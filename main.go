package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"auto-backup/config"
	"auto-backup/db"
	"auto-backup/handler"
	"auto-backup/log"
	"auto-backup/model"
	"auto-backup/service"
	"auto-backup/uploader"

	_ "github.com/mattn/go-sqlite3"
)

func initConfig() (*config.Config, bool, error) {
	cfg, err := config.LoadConfig("./config.yaml")
	if err != nil {
		log.Error("加载配置文件失败: %v", err)
		return nil, false, err
	}

	needUpload := true

	// 配置信息中client_id和client_secret从环境变量中获取，如果没有设置，则使用配置文件中的值
	if os.Getenv("CLIENT_ID") != "" {
		cfg.OneDrive.ClientID = os.Getenv("CLIENT_ID")
	} else {
		needUpload = false
	}
	if os.Getenv("CLIENT_SECRET") != "" {
		cfg.OneDrive.ClientSecret = os.Getenv("CLIENT_SECRET")
	} else {
		needUpload = false
	}
	if os.Getenv("REDIRECT_URI") != "" {
		cfg.OneDrive.RedirectURI = os.Getenv("REDIRECT_URI")
	} else {
		needUpload = false
	}
	if os.Getenv("BACKUP_PASSWORD") != "" {
		cfg.Backup.Password = os.Getenv("BACKUP_PASSWORD")
	}

	return cfg, needUpload, nil
}

func main() {
	// 加载配置文件
	config, needUpload, err := initConfig()
	if err != nil {
		fmt.Printf("加载配置失败: %v\n", err)
		return
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

	actionChan := make(chan model.TokenAction)
	doneChan := make(chan bool)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var store *uploader.OneDriveUploader = nil

	if needUpload {
		// 启动http服务
		server := handler.NewAuthHandlerServer(8080, actionChan)
		server.Start(ctx)

		onedriveConfig := &uploader.OneDriveConfig{
			ClientID:     config.OneDrive.ClientID,
			ClientSecret: config.OneDrive.ClientSecret,
			Scope:        config.OneDrive.Scope,
			RedirectURI:  config.OneDrive.RedirectURI,
		}

		store, err = uploader.NewOneDriveUploader(onedriveConfig, actionChan, doneChan, ctx)
		if err != nil {
			log.Error("初始化OneDrive上传器失败: %v", err)
			return
		}

		store.DoAuthInit()
	}

	backupInfo := service.BackupInfo{
		SrcDir:    config.Backup.RootDir,
		OutputDir: config.Backup.OutputDir,
		Password:  config.Backup.Password,
		ForceFull: config.Backup.ForceFullBackup,
		Cron:      config.Backup.Cron,
		Uploader:  store,
	}

	backupInfo.StartScheduledBackup()

	// 启动时执行一次备份
	backupInfo.Backup()

	// 等待退出信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info("收到退出信号,程序退出")
}
