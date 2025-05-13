package utils

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"sync"
)

var (
    // 用于哈希计算的缓冲池
    hashBufPool = sync.Pool{
        New: func() interface{} {
            b := make([]byte, 32*1024) // 32KB 缓冲区
            return &b
        },
    }
)

// CalculateFileHash 计算文件的哈希值
// 默认使用SHA256算法，如果文件较大则使用MD5以提高性能
// 参数:
//   - filePath: 文件路径
//   - algorithm: 可选参数，指定算法 "md5" 或 "sha256"（默认）
//
// 返回:
//   - 哈希字符串
//   - 错误信息
func CalculateFileHash(filePath string, algorithm ...string) (string, error) {
    // 默认使用SHA256算法
    hashType := "sha256"
    if len(algorithm) > 0 && algorithm[0] == "md5" {
        hashType = "md5"
    }

    // 检查文件大小
    fileInfo, err := os.Stat(filePath)
    if err != nil {
        return "", fmt.Errorf("获取文件信息失败: %v", err)
    }

    // 如果文件太大（>100MB），自动切换为MD5
    if fileInfo.Size() > 50*1024*1024 && hashType == "sha256" {
        hashType = "md5"
    }

    // 打开文件
    file, err := os.Open(filePath)
    if err != nil {
        return "", fmt.Errorf("打开文件失败: %v", err)
    }
    defer file.Close()

    // 从池中获取缓冲区
    buf := *(hashBufPool.Get().(*[]byte))
    defer func() {
        hashBufPool.Put(&buf)
    }()

    var hash string
    if hashType == "md5" {
        h := md5.New()
        if _, err := io.CopyBuffer(h, file, buf); err != nil {
            return "", fmt.Errorf("计算MD5哈希失败: %v", err)
        }
        hash = hex.EncodeToString(h.Sum(nil))
    } else {
        h := sha256.New()
        if _, err := io.CopyBuffer(h, file, buf); err != nil {
            return "", fmt.Errorf("计算SHA256哈希失败: %v", err)
        }
        hash = hex.EncodeToString(h.Sum(nil))
    }

    return hash, nil
}

// QuickFileHash 快速计算小文件或仅部分内容的哈希值
// 只读取文件的前8KB和后8KB内容，适合快速比较
func QuickFileHash(filePath string) (string, error) {
    file, err := os.Open(filePath)
    if err != nil {
        return "", fmt.Errorf("打开文件失败: %v", err)
    }
    defer file.Close()

    // 获取文件大小
    fileInfo, err := file.Stat()
    if err != nil {
        return "", fmt.Errorf("获取文件信息失败: %v", err)
    }

    fileSize := fileInfo.Size()
    h := md5.New()

    // 文件很小时直接读取全部内容
    if fileSize <= 16*1024 {
        if _, err := io.Copy(h, file); err != nil {
            return "", fmt.Errorf("读取文件失败: %v", err)
        }
    } else {
        // 读取前8KB
        frontBuf := make([]byte, 8*1024)
        if _, err := file.Read(frontBuf); err != nil {
            return "", fmt.Errorf("读取文件头部失败: %v", err)
        }
        h.Write(frontBuf)

        // 读取后8KB
        if _, err := file.Seek(-8*1024, io.SeekEnd); err != nil {
            return "", fmt.Errorf("文件定位失败: %v", err)
        }
        endBuf := make([]byte, 8*1024)
        if _, err := file.Read(endBuf); err != nil && err != io.EOF {
            return "", fmt.Errorf("读取文件尾部失败: %v", err)
        }
        h.Write(endBuf)
    }

    return hex.EncodeToString(h.Sum(nil)), nil
}
