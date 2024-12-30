package process

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/alexmullins/zip"

	"auto-backup/utils/db"
	"auto-backup/utils/logger"
)

// 存储文件信息的结构
type FileInfo struct {
	Path    string
	ModTime time.Time
	IsDir   bool
}

// 文件记录结构
type FileRecord struct {
	Path     string
	ModTime  time.Time
	BackupID string
}

// 获取目录下所有文件的信息
func getFilesList(srcDir string) (map[string]FileInfo, error) {
	files := make(map[string]FileInfo)

	err := filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}

		if relPath == "." {
			return nil
		}

		files[relPath] = FileInfo{
			Path:    relPath,
			ModTime: info.ModTime(),
			IsDir:   info.IsDir(),
		}
		return nil
	})

	return files, err
}

// 检查文件是否需要更新
func needsBackup(srcPath string, backupID string, forceFullBackup bool) (map[string]bool, error) {
	needsUpdate := make(map[string]bool)

	// 获取当前目录下的所有文件
	currentFiles, err := getFilesList(srcPath)
	if err != nil {
		return nil, err
	}

	// 如果是强制全量备份，直接返回所有文件
	if forceFullBackup {
		for path := range currentFiles {
			needsUpdate[path] = true
		}
		return needsUpdate, nil
	}

	// 获取上次备份的记录
	rows, err := db.DB().Query("SELECT path, mod_time FROM file_records WHERE backup_id = ?", backupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	lastBackup := make(map[string]time.Time)
	for rows.Next() {
		var path string
		var modTime time.Time
		if err := rows.Scan(&path, &modTime); err != nil {
			return nil, err
		}
		lastBackup[path] = modTime
	}

	// 比较文件
	for path, info := range currentFiles {
		if lastModTime, exists := lastBackup[path]; !exists || info.ModTime.After(lastModTime) {
			needsUpdate[path] = true
		}
	}

	return needsUpdate, nil
}

// 更新文件记录
func updateFileRecords(files map[string]FileInfo, backupID string) error {
	tx, err := db.DB().Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 删除旧记录
	_, err = tx.Exec("DELETE FROM file_records WHERE backup_id = ?", backupID)
	if err != nil {
		return err
	}

	// 插入新记录
	stmt, err := tx.Prepare("INSERT INTO file_records (path, mod_time, backup_id) VALUES (?, ?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, info := range files {
		_, err = stmt.Exec(info.Path, info.ModTime, backupID)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// 将文件压缩逻辑抽取为独立函数
func compressFile(archive *zip.Writer, srcDir, filePath, password string) error {
	fullPath := filepath.Join(srcDir, filePath)
	info, err := os.Stat(fullPath)
	if err != nil {
		return err
	}

	if info.IsDir() {
		return nil
	}

	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}

	relPath, err := filepath.Rel(srcDir, fullPath)
	if err != nil {
		return err
	}
	header.Name = filepath.ToSlash(relPath)
	header.SetModTime(info.ModTime())
	header.Method = zip.Deflate
	header.SetPassword(password)

	writer, err := archive.CreateHeader(header)
	if err != nil {
		return err
	}

	file, err := os.Open(fullPath)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(writer, file)
	return err
}

// 增量压缩文件夹
func IncrementalCompress(srcDir, outputDir, password string, forceFullBackup bool) error {
	log := logger.GetLogger()

	// 添加输入参数验证
	if srcDir == "" || outputDir == "" {
		log.Error("源目录和输出目录不能为空")
		return fmt.Errorf("源目录和输出目录不能为空")
	}

	defer func() {
		if err := recover(); err != nil {
			log.Error("压缩过程发生严重错误: %v", err)
		}
	}()

	// 检查需要更新的文件
	backupID := filepath.Base(srcDir)

	// 检查需要更新的文件
	filesToUpdate, err := needsBackup(srcDir, backupID, forceFullBackup)
	if err != nil {
		log.Error("检查需要更新的文件失败: %v", err)
		return err
	}

	if len(filesToUpdate) == 0 {
		log.Info("没有文件需要更新")
		return nil
	}

	log.Debug("开始压缩目录: %s", srcDir)

	// 确保输出目录存在
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		log.Error("创建输出目录失败: %v", err)
		return fmt.Errorf("创建输出目录失败: %v", err)
	}

	timestamp := time.Now().Format("20060102_150405")
	destZip := filepath.Join(outputDir, fmt.Sprintf("%s_%s.zip", backupID, timestamp))

	// 创建新的zip文件
	zipfile, err := os.Create(destZip)
	if err != nil {
		log.Error("创建zip文件失败: %v", err)
		return err
	}
	defer zipfile.Close()

	archive := zip.NewWriter(zipfile)
	defer archive.Close()

	// 使用 worker pool 处理文件压缩
	const maxWorkers = 4
	sem := make(chan struct{}, maxWorkers)
	errChan := make(chan error, len(filesToUpdate))

	for filePath := range filesToUpdate {
		sem <- struct{}{} // 获取信号量
		go func(path string) {
			defer func() { <-sem }() // 释放信号量
			if err := compressFile(archive, srcDir, path, password); err != nil {
				errChan <- err
			}
		}(filePath)
	}

	// 等待所有 worker 完成
	for i := 0; i < cap(sem); i++ {
		sem <- struct{}{}
	}

	// 备份完成后，更新数据库记录
	currentFiles, err := getFilesList(srcDir)
	if err != nil {
		return err
	}

	return updateFileRecords(currentFiles, backupID)
}

// 还原备份文件到指定目录
func RestoreBackup(backupDir, targetDir, password string) error {
	// 确保目标目录存在
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("创建还原目录失败: %v", err)
	}

	// 获取所有备份文件
	files, err := os.ReadDir(backupDir)
	if err != nil {
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
		backupPath := filepath.Join(backupDir, backupFile)
		file, err := os.Open(backupPath)
		if err != nil {
			return fmt.Errorf("打开备份文件失败: %v", err)
		}
		defer file.Close()

		fileInfo, err := file.Stat()
		if err != nil {
			return fmt.Errorf("获取文件信息失败: %v", err)
		}

		reader, err := zip.NewReader(file, fileInfo.Size())
		if err != nil {
			return fmt.Errorf("打开备份文件 %s 失败: %v", backupFile, err)
		}

		for _, file := range reader.File {
			// 设置密码
			file.SetPassword(password)

			// 解密文件
			rc, err := file.Open()
			if err != nil {
				return fmt.Errorf("解密文件 %s 失败: %v", file.Name, err)
			}
			defer rc.Close()

			// 构建目标路径
			targetPath := filepath.Join(targetDir, file.Name)

			if file.FileInfo().IsDir() {
				// 创建目录
				if err := os.MkdirAll(targetPath, file.Mode()); err != nil {
					return fmt.Errorf("创建目录 %s 失败: %v", targetPath, err)
				}
				continue
			}

			// 确保父目录存在
			if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
				return fmt.Errorf("创建父目录失败: %v", err)
			}

			// 创建目标文件
			f, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.Mode())
			if err != nil {
				return fmt.Errorf("创建文件 %s 失败: %v", targetPath, err)
			}

			// 复制内容
			_, err = io.Copy(f, rc)
			f.Close()
			if err != nil {
				return fmt.Errorf("写入文件 %s 失败: %v", targetPath, err)
			}

			// 设置修改时间
			if err := os.Chtimes(targetPath, file.ModTime(), file.ModTime()); err != nil {
				return fmt.Errorf("设置文件时间 %s 失败: %v", targetPath, err)
			}
		}
		fmt.Printf("已还原备份文件: %s\n", backupFile)
	}

	return nil
}
