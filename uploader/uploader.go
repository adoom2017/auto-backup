package uploader

// Uploader 定义了文件上传器的接口
type Uploader interface {
	// UploadBigFile 上传大文件
	UploadBigFile(folderPath, localFilePath string) error
}
