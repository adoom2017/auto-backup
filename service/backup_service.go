package service

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/alexmullins/zip"
	"github.com/robfig/cron/v3"

	"auto-backup/db"
	"auto-backup/log"
	"auto-backup/uploader"
)

// 存储文件信息的结构
type FileInfo struct {
	Path    string
	ModTime time.Time
	IsDir   bool
}

type BackupInfo struct {
	SrcDir    string
	OutputDir string
	Password  string
	Cron      string
	ForceFull bool
	Uploader  uploader.Uploader
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
		log.Error("获取文件列表失败: %v", err)
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
	records, err := db.LoadFileRecords(backupID)
	if err != nil {
		log.Error("获取上次备份的记录失败: %v", err)
		return nil, err
	}

	lastBackup := make(map[string]time.Time)
	for _, record := range records {
		lastBackup[record.Path] = record.ModTime
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
		log.Error("开始事务失败: %v", err)
		return err
	}
	defer tx.Rollback()

	// 删除旧记录
	err = db.DeleteFileRecord(backupID)
	if err != nil {
		log.Error("删除旧记录失败: %v", err)
		return err
	}

	log.Info("删除旧记录完成")

	// 批量插入新记录
	const batchSize = 1000
	records := make([]*db.FileRecord, 0, len(files))
	
	for _, info := range files {
		records = append(records, &db.FileRecord{
			Path:     info.Path,
			ModTime:  info.ModTime,
			BackupID: backupID,
		})
	}

	// 分批插入
	for i := 0; i < len(records); i += batchSize {
		end := i + batchSize
		if end > len(records) {
			end = len(records)
		}
		
		err = db.BatchSaveFileRecords(records[i:end])
		if err != nil {
			log.Error("批量插入记录失败: %v", err)
			return err
		}
		
		log.Info("成功插入 %d 条记录", end-i)
	}

	log.Info("插入新记录完成")

	return tx.Commit()
}

// 将文件压缩逻辑抽取为独立函数
func compressFile(archive *zip.Writer, srcDir, filePath, password string) error {
	fullPath := filepath.Join(srcDir, filePath)
	info, err := os.Stat(fullPath)
	if err != nil {
		log.Error("获取文件信息失败: %v", err)
		return err
	}

	if info.IsDir() {
		return nil
	}

	header, err := zip.FileInfoHeader(info)
	if err != nil {
		log.Error("获取文件头失败: %v", err)
		return err
	}

	relPath, err := filepath.Rel(srcDir, fullPath)
	if err != nil {
		log.Error("获取相对路径失败: %v", err)
		return err
	}
	header.Name = filepath.ToSlash(relPath)
	header.SetModTime(info.ModTime())
	header.Method = zip.Deflate
	header.SetPassword(password)

	writer, err := archive.CreateHeader(header)
	if err != nil {
		log.Error("创建文件头失败: %v", err)
		return err
	}

	file, err := os.Open(fullPath)
	if err != nil {
		log.Error("打开文件失败: %v", err)
		return err
	}
	defer file.Close()

	_, err = io.Copy(writer, file)
	if err != nil {
		log.Error("添加压缩文件失败: %v", err)
		return err
	}

	return nil
}

// 增量压缩文件夹
func (b *BackupInfo) Backup() error {
	// 添加输入参数验证
	if b.SrcDir == "" || b.OutputDir == "" {
		log.Error("源目录和输出目录不能为空")
		return fmt.Errorf("源目录和输出目录不能为空")
	}

	defer func() {
		if err := recover(); err != nil {
			log.Error("压缩过程发生严重错误: %v", err)
		}
	}()

	// 检查需要更新的文件
	backupID := filepath.Base(b.SrcDir)

	// 检查需要更新的文件
	filesToUpdate, err := needsBackup(b.SrcDir, backupID, b.ForceFull)
	if err != nil {
		log.Error("检查需要更新的文件失败: %v", err)
		return err
	}

	if len(filesToUpdate) == 0 {
		log.Info("没有文件需要更新")
		return nil
	}

	log.Debug("开始压缩目录: %s", b.SrcDir)

	// 确保输出目录存在
	if err := os.MkdirAll(b.OutputDir, 0755); err != nil {
		log.Error("创建输出目录失败: %v", err)
		return fmt.Errorf("创建输出目录失败: %v", err)
	}

	timestamp := time.Now().Format("20060102_150405")
	destZip := filepath.Join(b.OutputDir, fmt.Sprintf("%s_%s.zip", backupID, timestamp))

	// 创建新的zip文件
	zipfile, err := os.Create(destZip)
	if err != nil {
		log.Error("创建zip文件失败: %v", err)
		return fmt.Errorf("创建zip文件失败: %v", err)
	}
	defer zipfile.Close()

	archive := zip.NewWriter(zipfile)
	defer archive.Close()

	// 改为单线程顺序处理文件
	for filePath := range filesToUpdate {
		err := compressFile(archive, b.SrcDir, filePath, b.Password)
		if err != nil {
			log.Error("压缩文件失败: %v", err)
			return fmt.Errorf("压缩文件失败: %v", err)
		}
	}

	log.Info("压缩文件完成")

	// 备份完成后，更新数据库记录
	currentFiles, err := getFilesList(b.SrcDir)
	if err != nil {
		return fmt.Errorf("获取文件列表失败: %v", err)
	}

	err = updateFileRecords(currentFiles, backupID)
	if err != nil {
		return fmt.Errorf("更新文件记录失败: %v", err)
	}

	log.Info("开始上传文件: %s", destZip)

	// 上传文件
	err = b.Uploader.UploadBigFile("backup", destZip)
	if err != nil {
		return fmt.Errorf("上传文件失败: %v", err)
	}

	log.Info("上传文件完成")

	return nil
}

// 启动定时备份任务
func (b *BackupInfo) StartScheduledBackup() {
	c := cron.New()

	// 每天0点执行备份
	_, err := c.AddFunc(b.Cron, func() {
		log.Info("开始执行定时备份任务: %s", b.Cron)

		err := b.Backup()
		if err != nil {
			log.Error("定时备份失败: %v", err)
		} else {
			log.Info("定时备份完成")
		}
	})

	if err != nil {
		log.Error("添加定时任务失败: %v", err)
		return
	}

	c.Start()
	log.Info("定时备份任务已启动,将在每天0点执行")
}
