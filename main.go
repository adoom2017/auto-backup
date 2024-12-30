package main

import (
	"fmt"
	"time"

	"auto-backup/config"
	"auto-backup/process"
	"auto-backup/restful"
	"auto-backup/utils/db"
	"auto-backup/utils/logger"

	_ "github.com/mattn/go-sqlite3"
)

// 使用导入的logger
var log *logger.Logger

func main() {
	// 加载配置文件
	err := config.LoadConfig("./config.yaml")
	if err != nil {
		fmt.Printf("加载配置失败: %v\n", err)
		return
	}

	// 初始化日志
	logger.InitLogger(
		config.GlobalConfig.Log.BaseName,
		config.GlobalConfig.Log.Level,
		config.GlobalConfig.Log.MaxDays,
	)
	log = logger.GetLogger() // 获取全局日志记录器

	// 初始化数据库
	db.InitDB()
	defer db.CloseDB()

	// 启动http服务
	server := restful.NewOneDriveServer(8080)
	server.Start()
	defer server.Stop()

	// 加载认证信息
	firstTime := false
	for {
		authInfo, err := db.LoadAuthInfo()
		if err != nil {
			if !firstTime {
				log.Error("加载认证信息失败: %v\n", err)
				log.Info("请先进行认证, 将下面的URL复制到浏览器中进行认证:")
				log.Info("%s", server.GetAuthUrl())
				firstTime = true
			}

			// 等待5秒后重试
			time.Sleep(5 * time.Second)
			continue
		}

		config.GlobalAuthInfo = authInfo
		break
	}

	log.Info("认证信息加载成功, %v", config.GlobalAuthInfo)

	rootDir := config.GlobalConfig.Backup.RootDir                 // 要备份的根目录
	outputDir := config.GlobalConfig.Backup.OutputDir             // 备份文件的输出目录
	password := config.GlobalConfig.Backup.Password               // 压缩密码
	forceFullBackup := config.GlobalConfig.Backup.ForceFullBackup // 是否强制全量备份

	err = process.IncrementalCompress(rootDir, outputDir, password, forceFullBackup)
	if err != nil {
		log.Error("备份错误: %v", err)
		return
	}

	log.Info("备份完成")

	// 还原示例
	// err = process.RestoreBackup("./backups", "./restored", "123456")
	// if err != nil {
	// 	fmt.Printf("还原错误: %v\n", err)
	// 	return
	// }
	// fmt.Printf("还原完成\n")

	// 等待信号
	server.WaitForSignal()
}
