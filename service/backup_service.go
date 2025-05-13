package service

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/alexmullins/zip"
	"github.com/robfig/cron/v3"

	"auto-backup/db"
	"auto-backup/log"
	"auto-backup/uploader"
	"auto-backup/utils"
)

// 存储文件信息的结构
type FileInfo struct {
	Path    string
	ModTime time.Time
	IsDir   bool
	Hash    string
}

type BackupInfo struct {
	SrcDir    string
	OutputDir string
	Password  string
	Cron      string
	ForceFull bool
	BasePath  string
	Uploader  uploader.Uploader
}

// 添加缓冲区大小常量
const (
	defaultBufferSize = 4 * 1024 * 1024        // 4MB 缓冲区
	maxZipSize        = 1 * 1024 * 1024 * 1024 // 1GB 每个压缩包最大大小
)

// 包级别的缓冲池
var bufPool = sync.Pool{
	New: func() interface{} {
		b := make([]byte, defaultBufferSize)
		return &b
	},
}

// 添加一个变量和互斥锁来跟踪备份状态
var (
	isRunning bool
	mu        sync.Mutex
)

// 获取目录下所有文件的信息
func getFilesList(srcDir string) (map[string]FileInfo, error) {
	files := make(map[string]FileInfo)

	err := filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 过滤隐藏文件和系统文件
		filename := info.Name()
		if filename[0] == '.' { // 隐藏文件
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// 检查是否在排除列表中
		if utils.ExcludedFiles[filename] {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}

		if relPath == "." {
			return nil
		}

		if !info.IsDir() {
			// 计算文件哈希（仅对文件进行）
			hash, err := utils.QuickFileHash(path) // 使用快速哈希算法
			if err != nil {
				log.Warn("计算文件哈希失败: %s, %v", path, err)
				// 失败时使用空哈希，继续处理
				hash = ""
			}

			files[relPath] = FileInfo{
				Path:    relPath,
				ModTime: info.ModTime(),
				IsDir:   info.IsDir(),
				Hash:    hash,
			}
		} else {
			files[relPath] = FileInfo{
				Path:    relPath,
				ModTime: info.ModTime(),
				IsDir:   info.IsDir(),
				Hash:    "", // 目录没有哈希值
			}
		}

		return nil
	})

	return files, err
}

// 检查文件是否需要更新（需要修改db.FileRecord结构添加Hash字段）
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

	lastBackup := make(map[string]*db.FileRecord)
	for _, record := range records {
		lastBackup[record.Path] = record
	}

	// 比较文件
	for path, info := range currentFiles {
		lastRecord, exists := lastBackup[path]

		if !exists {
			// 文件是新增的
			log.Debug("新增文件: %s", path)
			needsUpdate[path] = true
		} else if info.ModTime.After(lastRecord.ModTime) {
			// 时间变化了，检查哈希值
			if info.Hash != "" && lastRecord.Hash != "" && info.Hash != lastRecord.Hash {
				// 哈希值不同，内容确实变化了
				log.Debug("文件内容已变更: %s", path)
				needsUpdate[path] = true
			} else if info.ModTime.Sub(lastRecord.ModTime).Seconds() > 1 {
				// 如果哈希值为空，但修改时间差大于1秒，认为文件变更
				log.Debug("文件时间已变更: %s", path)
				needsUpdate[path] = true
			}
		}
	}

	// 可选：处理已删除的文件（如果需要）
	// for path := range lastBackup {
	// 	if _, exists := currentFiles[path]; !exists {
	// 		log.Info("文件已删除: %s", path)
	// 	}
	// }

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
			Hash:     info.Hash,
			BackupID: backupID,
		})
	}

	// 分批插入
	for i := 0; i < len(records); i += batchSize {
		end := i + batchSize
		if end > len(records) {
			end = len(records)
		}

		err = db.BatchSaveFileRecordsOptimized(records[i:end])
		if err != nil {
			log.Error("批量插入记录失败: %v", err)
			return err
		}

		log.Info("成功插入 %d 条记录", end-i)
	}

	log.Info("插入新记录完成")

	return tx.Commit()
}

// 根据文件类型选择最佳压缩方式
func selectCompressionMethod(filename string) uint16 {
	ext := strings.ToLower(filepath.Ext(filename))

	// 已压缩的文件类型使用 STORE 方法（不压缩）
	noCompressionExts := map[string]bool{
		".jpg": true, ".jpeg": true, ".png": true,
		".mp3": true, ".mp4": true, ".zip": true,
		".rar": true, ".7z": true, ".gz": true,
	}

	if noCompressionExts[ext] {
		return zip.Store
	}

	// 其他文件使用 DEFLATE 方法
	return zip.Deflate
}

// 将文件压缩逻辑抽取为独立函数
func (b *BackupInfo) compressFile(archive *zip.Writer, srcDir, filePath, password string) error {
	fullPath := filepath.Join(srcDir, filePath)
	info, err := os.Stat(fullPath)
	if err != nil {
		log.Error("获取文件信息失败: %v", err)
		return fmt.Errorf("获取文件信息失败: %v", err)
	}

	if info.IsDir() {
		return nil
	}

	// 创建带缓冲的读取器
	file, err := os.Open(fullPath)
	if err != nil {
		log.Error("打开文件失败: %v", err)
		return fmt.Errorf("打开文件失败: %v", err)
	}
	defer file.Close()

	bufferedReader := bufio.NewReaderSize(file, defaultBufferSize)

	header, err := zip.FileInfoHeader(info)
	if err != nil {
		log.Error("创建文件头失败: %v", err)
		return fmt.Errorf("创建文件头失败: %v", err)
	}

	relPath, err := filepath.Rel(srcDir, fullPath)
	if err != nil {
		log.Error("获取相对路径失败: %v", err)
		return fmt.Errorf("获取相对路径失败: %v", err)
	}

	header.Name = filepath.ToSlash(relPath)
	header.SetModTime(info.ModTime())

	// 根据文件类型选择压缩方法
	header.Method = selectCompressionMethod(filePath)
	header.SetPassword(password)

	writer, err := archive.CreateHeader(header)
	if err != nil {
		log.Error("创建文件头失败: %v", err)
		return fmt.Errorf("创建文件头失败: %v", err)
	}

	// 从池中获取缓冲区
	buf := *(bufPool.Get().(*[]byte))
	defer func() {
		bufPool.Put(&buf)
	}()

	_, err = io.CopyBuffer(writer, bufferedReader, buf)
	if err != nil {
		log.Error("复制文件内容失败: %v", err)
		return fmt.Errorf("复制文件内容失败: %v", err)
	}

	return nil
}

// 增量压缩文件夹
func (b *BackupInfo) Backup() error {
	// 检查是否已经在运行
	mu.Lock()
	if isRunning {
		mu.Unlock()
		log.Warn("备份任务已经在运行中，不能重复运行")
		return fmt.Errorf("备份任务已经在运行中")
	}
	isRunning = true
	mu.Unlock()

	// 确保在函数结束时重置运行状态
	defer func() {
		mu.Lock()
		isRunning = false
		mu.Unlock()
	}()

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

	// 用于跟踪当前压缩文件的大小
	var currentZipSize int64 = 0
	var zipIndex = 1
	var currentArchive *zip.Writer
	var currentZipFile *os.File

	// 创建新的zip文件函数
	createNewZipFile := func() error {
		if currentZipFile != nil {
			currentArchive.Close()
			currentZipFile.Close()
		}

		destZip := filepath.Join(b.OutputDir, fmt.Sprintf("%s_%s_part%d.zip", backupID, timestamp, zipIndex))
		zipfile, err := os.Create(destZip)
		if err != nil {
			log.Error("创建zip文件失败: %v", err)
			return fmt.Errorf("创建zip文件失败: %v", err)
		}
		currentZipFile = zipfile
		currentArchive = zip.NewWriter(zipfile)
		currentZipSize = 0
		zipIndex++
		return nil
	}

	// 创建第一个zip文件
	if err := createNewZipFile(); err != nil {
		return err
	}
	defer func() {
		if currentArchive != nil {
			currentArchive.Close()
		}
		if currentZipFile != nil {
			currentZipFile.Close()
		}
	}()

	// 修改文件压缩逻辑
	for filePath := range filesToUpdate {
		fullPath := filepath.Join(b.SrcDir, filePath)
		info, err := os.Stat(fullPath)
		if err != nil {
			log.Error("获取文件信息失败: %v", err)
			continue
		}

		// 如果当前文件加上当前zip大小超过限制，创建新的zip文件
		if !info.IsDir() && currentZipSize+info.Size() > maxZipSize {
			if err := createNewZipFile(); err != nil {
				return err
			}

			// 如果有上传器，上传前一个文件
			if b.Uploader != nil {
				prevZipPath := filepath.Join(b.OutputDir, fmt.Sprintf("%s_%s_part%d.zip", backupID, timestamp, zipIndex-2))
				log.Info("开始上传文件: %s", prevZipPath)
				err = b.Uploader.UploadBigFile(b.BasePath, prevZipPath)
				if err != nil {
					return fmt.Errorf("上传文件失败: %v", err)
				}
				// 上传成功后删除本地文件
				os.Remove(prevZipPath)
			}
		}

		// 压缩文件
		err = b.compressFile(currentArchive, b.SrcDir, filePath, b.Password)
		if err != nil {
			log.Error("压缩文件失败: %v", err)
			return fmt.Errorf("压缩文件失败: %v", err)
		}

		if !info.IsDir() {
			currentZipSize += info.Size()
		}
	}

	// 关闭最后一个压缩文件
	currentArchive.Close()
	currentZipFile.Close()

	// 上传最后的文件
	if b.Uploader != nil {
		for i := 1; i < zipIndex; i++ {
			lastZipPath := filepath.Join(b.OutputDir, fmt.Sprintf("%s_%s_part%d.zip", backupID, timestamp, i))
			if _, err := os.Stat(lastZipPath); err == nil {
				log.Info("开始上传文件: %s", lastZipPath)
				err = b.Uploader.UploadBigFile(b.BasePath, lastZipPath)
				if err != nil {
					return fmt.Errorf("上传文件失败: %v", err)
				}
				// 上传成功后删除本地文件
				os.Remove(lastZipPath)
			}
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
