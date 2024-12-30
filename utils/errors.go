package utils

import "errors"

// 上传错误类型
var (
	ErrInvalidConfig  = errors.New("invalid configuration")
	ErrUploadFailed   = errors.New("upload failed")
	ErrDeleteFailed   = errors.New("delete failed")
	ErrListFailed     = errors.New("list failed")
	ErrNotImplemented = errors.New("not implemented")
)
