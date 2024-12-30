package uploader_test

import (
	"auto-backup/uploader"
	"testing"
)

// 测试onedrive上传

func TestOneDriveUploader(t *testing.T) {
	config := &uploader.OneDriveConfig{
		AccessToken:  "your_access_token",
		RefreshToken: "your_refresh_token",
	}

	onedriveUploader, err := uploader.NewOneDriveUploader(config)
	if err != nil {
		t.Fatalf("Failed to create uploader: %v", err)
	}

	err = onedriveUploader.UploadBigFile("Album", "E:\\paopao-gateway.iso")
	if err != nil {
		t.Fatalf("Failed to upload file: %v", err)
	}
}
