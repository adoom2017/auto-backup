package uploader

import (
	"auto-backup/utils"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3Config AWS S3配置
type S3Config struct {
	Region          string
	Bucket          string
	AccessKeyID     string
	SecretAccessKey string
	Endpoint        string // 可选，用于兼容S3协议的其他存储服务
	BasePath        string
}

func (c *S3Config) Validate() error {
	if c.Bucket == "" || c.AccessKeyID == "" || c.SecretAccessKey == "" {
		return utils.ErrInvalidConfig
	}
	return nil
}

// S3Uploader S3上传实现
type S3Uploader struct {
	config *S3Config
	client *s3.Client
}

func NewS3Uploader(config *S3Config) (*S3Uploader, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	client, err := createS3Client(config)
	if err != nil {
		return nil, err
	}

	return &S3Uploader{
		config: config,
		client: client,
	}, nil
}

func (u *S3Uploader) Upload(ctx context.Context, filePath string, reader io.Reader) (*UploadResult, error) {
	uploadPath := filepath.Join(u.config.BasePath, filePath)

	input := &s3.PutObjectInput{
		Bucket: &u.config.Bucket,
		Key:    &uploadPath,
		Body:   reader,
	}

	_, err := u.client.PutObject(ctx, input)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s",
		u.config.Bucket, u.config.Region, uploadPath)

	return &UploadResult{
		URL:       url,
		Path:      uploadPath,
		Timestamp: time.Now(),
	}, nil
}

func (u *S3Uploader) Delete(ctx context.Context, path string) error {
	input := &s3.DeleteObjectInput{
		Bucket: &u.config.Bucket,
		Key:    &path,
	}

	_, err := u.client.DeleteObject(ctx, input)
	return err
}

func (u *S3Uploader) List(ctx context.Context, path string) ([]string, error) {
	var files []string

	input := &s3.ListObjectsV2Input{
		Bucket: &u.config.Bucket,
		Prefix: &path,
	}

	result, err := u.client.ListObjectsV2(ctx, input)
	if err != nil {
		return nil, err
	}

	for _, obj := range result.Contents {
		files = append(files, *obj.Key)
	}

	return files, nil
}

// createS3Client 创建S3客户端
func createS3Client(config *S3Config) (*s3.Client, error) {
	// 创建凭证提供者
	creds := credentials.NewStaticCredentialsProvider(
		config.AccessKeyID,
		config.SecretAccessKey,
		"",
	)

	// 创建AWS配置
	cfg := aws.Config{
		Region:      config.Region,
		Credentials: creds,
	}

	// 如果提供了自定义endpoint，使用它
	if config.Endpoint != "" {
		cfg.EndpointResolver = aws.EndpointResolverFunc(func(service, region string) (aws.Endpoint, error) {
			return aws.Endpoint{
				URL: config.Endpoint,
			}, nil
		})
	}

	// 创建S3客户端
	return s3.NewFromConfig(cfg), nil
}
