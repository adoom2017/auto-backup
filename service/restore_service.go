package service

import (
	"auto-backup/log"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/alexmullins/zip"
)

type RestoreInfo struct {
	BackupDir string
	TargetDir string
	Password  string
}

// 还原备份文件到指定目录
func (r *RestoreInfo) Restore() error {
	// 确保目标目录存在
	if err := os.MkdirAll(r.TargetDir, 0755); err != nil {
		log.Error("创建还原目录失败: %v", err)
		return fmt.Errorf("创建还原目录失败: %v", err)
	}

	// 获取所有备份文件
	files, err := os.ReadDir(r.BackupDir)
	if err != nil {
		log.Error("读取备份目录失败: %v", err)
		return fmt.Errorf("读取备份目录失败: %v", err)
	}

	// 过滤并排序zip文件
	var backupFiles []string
	for _, f := range files {
		if filepath.Ext(f.Name()) == ".zip" {
			backupFiles = append(backupFiles, f.Name())
		}
	}
	// 按时间戳排序（最早的在前）
	sort.Strings(backupFiles)

	// 按顺序解压每个备份文件
	for _, backupFile := range backupFiles {
		backupPath := filepath.Join(r.BackupDir, backupFile)
		file, err := os.Open(backupPath)
		if err != nil {
			log.Error("打开备份文件失败: %v", err)
			return fmt.Errorf("打开备份文件失败: %v", err)
		}
		defer file.Close()

		fileInfo, err := file.Stat()
		if err != nil {
			log.Error("获取文件信息失败: %v", err)
			return fmt.Errorf("获取文件信息失败: %v", err)
		}

		reader, err := zip.NewReader(file, fileInfo.Size())
		if err != nil {
			log.Error("打开备份文件 %s 失败: %v", backupFile, err)
			return fmt.Errorf("打开备份文件 %s 失败: %v", backupFile, err)
		}

		for _, file := range reader.File {
			// 设置密码
			file.SetPassword(r.Password)

			// 解密文件
			rc, err := file.Open()
			if err != nil {
				log.Error("解密文件 %s 失败: %v", file.Name, err)
				return fmt.Errorf("解密文件 %s 失败: %v", file.Name, err)
			}
			defer rc.Close()

			// 构建目标路径
			targetPath := filepath.Join(r.TargetDir, file.Name)

			if file.FileInfo().IsDir() {
				// 创建目录
				if err := os.MkdirAll(targetPath, file.Mode()); err != nil {
					log.Error("创建目录 %s 失败: %v", targetPath, err)
					return fmt.Errorf("创建目录 %s 失败: %v", targetPath, err)
				}
				continue
			}

			// 确保父目录存在
			if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
				log.Error("创建父目录 %s 失败: %v", filepath.Dir(targetPath), err)
				return fmt.Errorf("创建父目录 %s 失败: %v", filepath.Dir(targetPath), err)
			}

			// 创建目标文件
			f, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.Mode())
			if err != nil {
				log.Error("创建文件 %s 失败: %v", targetPath, err)
				return fmt.Errorf("创建文件 %s 失败: %v", targetPath, err)
			}

			// 复制内容
			_, err = io.Copy(f, rc)
			f.Close()
			if err != nil {
				log.Error("写入文件 %s 失败: %v", targetPath, err)
				return fmt.Errorf("写入文件 %s 失败: %v", targetPath, err)
			}

			// 设置修改时间
			if err := os.Chtimes(targetPath, file.ModTime(), file.ModTime()); err != nil {
				log.Error("设置文件时间 %s 失败: %v", targetPath, err)
				return fmt.Errorf("设置文件时间 %s 失败: %v", targetPath, err)
			}
		}
		fmt.Printf("已还原备份文件: %s\n", backupFile)
	}

	return nil
}
