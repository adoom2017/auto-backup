package uploader

import (
	"context"
	"io"
	"time"
)

// UploadConfig 上传配置接口
type UploadConfig interface {
	Validate() error
}

// UploadResult 上传结果
type UploadResult struct {
	URL       string    // 文件访问URL
	Path      string    // 文件在存储服务中的路径
	Timestamp time.Time // 上传时间
}

// Uploader 文件上传接口
type Uploader interface {
	// Upload 上传文件
	Upload(ctx context.Context, filePath string, reader io.Reader) (*UploadResult, error)

	// Delete 删除文件
	Delete(ctx context.Context, path string) error

	// List 列出指定路径下的文件
	List(ctx context.Context, path string) ([]string, error)
}

// UploaderFactory 上传器工厂接口
type UploaderFactory interface {
	Create(config UploadConfig) (Uploader, error)
}
