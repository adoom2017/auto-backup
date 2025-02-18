package service

import (
	"auto-backup/log"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/alexmullins/zip"
)

type RestoreInfo struct {
	ZipDir    string // 压缩文件所在目录
	OutputDir string // 解压目标目录
	Password  string // 解压密码
	BackupID  string // 备份ID
	Timestamp string // 可选，指定要还原的备份时间
}

// 备份分片信息
type BackupPart struct {
	Path      string    // 文件路径
	Timestamp time.Time // 备份时间
	PartNum   int       // 分片序号
}

func (r *RestoreInfo) Restore() error {
	// 1. 查找所有分片文件并解析信息
	pattern := fmt.Sprintf("%s_*.zip", r.BackupID)
	matches, err := filepath.Glob(filepath.Join(r.ZipDir, pattern))
	if err != nil {
		return fmt.Errorf("查找分片文件失败: %v", err)
	}

	if len(matches) == 0 {
		return fmt.Errorf("未找到备份文件: %s", pattern)
	}

	// 2. 解析所有备份文件信息
	backupFiles := make(map[string][]BackupPart) // key: timestamp
	for _, path := range matches {
		timestamp, partNum, err := parseBackupFileName(path)
		if err != nil {
			log.Warn("跳过无效的备份文件: %s, 错误: %v", path, err)
			continue
		}

		timeStr := timestamp.Format("20060102_150405")
		backupFiles[timeStr] = append(backupFiles[timeStr], BackupPart{
			Path:      path,
			Timestamp: timestamp,
			PartNum:   partNum,
		})
	}

	// 3. 如果没有指定时间，列出所有可用的备份时间
	if r.Timestamp == "" {
		timestamps := make([]string, 0, len(backupFiles))
		for ts := range backupFiles {
			timestamps = append(timestamps, ts)
		}
		sort.Sort(sort.Reverse(sort.StringSlice(timestamps))) // 最新的在前面

		log.Info("可用的备份时间:")
		for _, ts := range timestamps {
			log.Info("- %s (共%d个分片)", ts, len(backupFiles[ts]))
		}
		return fmt.Errorf("请指定要还原的备份时间")
	}

	// 4. 获取指定时间的备份文件
	parts, exists := backupFiles[r.Timestamp]
	if !exists {
		return fmt.Errorf("未找到指定时间(%s)的备份文件", r.Timestamp)
	}

	// 5. 按分片序号排序
	sort.Slice(parts, func(i, j int) bool {
		return parts[i].PartNum < parts[j].PartNum
	})

	// 6. 确保输出目录存在
	if err := os.MkdirAll(r.OutputDir, 0755); err != nil {
		return fmt.Errorf("创建输出目录失败: %v", err)
	}

	// 7. 依次解压每个分片
	for i, part := range parts {
		log.Info("正在解压第%d/%d个分片: %s", i+1, len(parts), filepath.Base(part.Path))
		if err := r.extractZipFile(part.Path); err != nil {
			return fmt.Errorf("解压文件 %s 失败: %v", part.Path, err)
		}
	}

	log.Info("还原完成")
	return nil
}

// 解析备份文件名
// 文件名格式: backupID_20060102_150405_partN.zip
func parseBackupFileName(filename string) (time.Time, int, error) {
	base := filepath.Base(filename)
	parts := strings.Split(base, "_")
	if len(parts) != 4 {
		return time.Time{}, 0, fmt.Errorf("无效的文件名格式")
	}

	// 解析时间戳
	timeStr := parts[1] + "_" + strings.TrimSuffix(parts[2], "_part")
	timestamp, err := time.ParseInLocation("20060102_150405", timeStr, time.Local)
	if err != nil {
		return time.Time{}, 0, fmt.Errorf("无效的时间格式: %v", err)
	}

	// 解析分片序号
	var partNum int
	_, err = fmt.Sscanf(parts[3], "part%d.zip", &partNum)
	if err != nil {
		return time.Time{}, 0, fmt.Errorf("无效的分片序号: %v", err)
	}

	return timestamp, partNum, nil
}

func (r *RestoreInfo) extractZipFile(zipPath string) error {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("打开zip文件失败: %v", err)
	}
	defer reader.Close()

	// 遍历压缩文件中的每个文件
	for _, file := range reader.File {
		if file.IsEncrypted() {
			file.SetPassword(r.Password)
		}

		// 构建完整的输出路径
		outPath := filepath.Join(r.OutputDir, file.Name)

		if file.FileInfo().IsDir() {
			// 创建目录
			if err := os.MkdirAll(outPath, file.Mode()); err != nil {
				return fmt.Errorf("创建目录失败: %v", err)
			}
			continue
		}

		// 确保父目录存在
		if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
			return fmt.Errorf("创建父目录失败: %v", err)
		}

		// 创建输出文件
		outFile, err := os.OpenFile(outPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.Mode())
		if err != nil {
			return fmt.Errorf("创建输出文件失败: %v", err)
		}

		// 打开压缩文件
		rc, err := file.Open()
		if err != nil {
			outFile.Close()
			return fmt.Errorf("打开压缩文件失败: %v", err)
		}

		// 复制文件内容
		_, err = io.Copy(outFile, rc)
		rc.Close()
		outFile.Close()

		if err != nil {
			return fmt.Errorf("解压文件内容失败: %v", err)
		}

		// 保持文件修改时间
		if err := os.Chtimes(outPath, file.ModTime(), file.ModTime()); err != nil {
			log.Warn("设置文件时间失败: %v", err)
		}
	}

	return nil
}
