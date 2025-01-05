package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"auto-backup/config"
	"auto-backup/db"
	"auto-backup/handler"
	"auto-backup/log"
	"auto-backup/model"
	"auto-backup/service"
	"auto-backup/uploader"

	_ "github.com/mattn/go-sqlite3"
)

func GetAuthUrl(config *config.Config) string {
	return fmt.Sprintf("https://login.live.com/oauth20_authorize.srf?client_id=%s&scope=%s&response_type=code&redirect_uri=%s",
		config.OneDrive.ClientID,
		config.OneDrive.Scope,
		config.OneDrive.RedirectURI)
}

func main() {
	// 加载配置文件
	config, err := config.LoadConfig("./config.yaml")
	if err != nil {
		fmt.Printf("加载配置失败: %v\n", err)
		return
	}

	// 配置信息中client_id和client_secret从环境变量中获取，如果没有设置，则使用配置文件中的值
	if os.Getenv("CLIENT_ID") != "" {
		config.OneDrive.ClientID = os.Getenv("CLIENT_ID")
	}
	if os.Getenv("CLIENT_SECRET") != "" {
		config.OneDrive.ClientSecret = os.Getenv("CLIENT_SECRET")
	}
	if os.Getenv("REDIRECT_URI") != "" {
		config.OneDrive.RedirectURI = os.Getenv("REDIRECT_URI")
	}
	if os.Getenv("BACKUP_PASSWORD") != "" {
		config.Backup.Password = os.Getenv("BACKUP_PASSWORD")
	}

	// 初始化日志
	log.SetLogConfig(log.LogConfig{
		Level:      log.LogLevel(config.Log.Level),
		Filename:   config.Log.Path,
		MaxSize:    config.Log.MaxSize,
		MaxBackups: config.Log.MaxBackups,
		Compress:   config.Log.Compress,
	})

	// 初始化数据库
	db.InitDB()
	defer db.CloseDB()

	action := make(chan model.TokenAction)
	ctx, cancel := context.WithCancel(context.Background())

	// 启动http服务
	server := handler.NewAuthHandlerServer(8080, action)
	server.Start(ctx)
	defer cancel()

	onedriveConfig := &uploader.OneDriveConfig{
		ClientID:     config.OneDrive.ClientID,
		ClientSecret: config.OneDrive.ClientSecret,
		Scope:        config.OneDrive.Scope,
		RedirectURI:  config.OneDrive.RedirectURI,
	}

	uploader, err := uploader.NewOneDriveUploader(onedriveConfig)
	if err != nil {
		log.Error("初始化OneDrive上传器失败: %v", err)
		return
	}

	done := make(chan bool)

	go func() {
		for {
			select {
			case act := <-action:
				if act.Action == "getToken" {
					err = uploader.GetAccessTokenByCode(act.Code)
					if err != nil {
						log.Error("获取访问令牌失败: %v", err)
						continue
					}
					log.Info("认证成功, 可以进行备份")
					done <- true
				} else if act.Action == "refreshToken" {
					err = uploader.RefreshAccessToken()
					if err != nil {
						log.Error("刷新访问令牌失败: %v", err)
						continue
					}
					log.Info("刷新令牌成功, 可以进行备份")
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// 加载认证信息
	authInfo, err := db.LoadAuthInfo()
	if err != nil {
		log.Error("加载认证信息失败: %v\n", err)
		log.Info("请先进行认证, 将下面的URL复制到浏览器中进行认证:")
		log.Info("%s", GetAuthUrl(config))

		<-done

		authInfo, _ = db.LoadAuthInfo()
	}

	// 判断认证信息是否过期
	if authInfo.ExpiresIn < time.Now().Unix() {
		log.Info("认证信息已过期, 请重新认证")
		log.Info("请先进行认证, 将下面的URL复制到浏览器中进行认证:")
		log.Info("%s", GetAuthUrl(config))

		<-done
	}

	uploader.SetAuthInfo(authInfo)

	// 启动定时刷新Token
	go func() {
		for {
			time.Sleep(time.Duration(30 * time.Minute))
			uploader.RefreshAccessToken()
		}
	}()

	backupInfo := service.BackupInfo{
		SrcDir:    config.Backup.RootDir,
		OutputDir: config.Backup.OutputDir,
		Password:  config.Backup.Password,
		ForceFull: config.Backup.ForceFullBackup,
		Cron:      config.Backup.Cron,
		Uploader:  uploader,
	}

	backupInfo.StartScheduledBackup()

	// 启动时执行一次备份
	backupInfo.Backup()

	// 等待退出信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info("收到退出信号,程序退出")

	// 还原示例
	// err = process.RestoreBackup("./backups", "./restored", "123456")
	// if err != nil {
	// 	fmt.Printf("还原错误: %v\n", err)
	// 	return
	// }
	// fmt.Printf("还原完成\n")
}
