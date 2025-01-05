package utils

var (
	// 需要过滤的系统文件和目录列表
	ExcludedFiles = map[string]bool{
		"System Volume Information": true, // Windows系统文件
		"$RECYCLE.BIN":             true, // Windows回收站
		"lost+found":               true, // Linux系统文件
	}
)