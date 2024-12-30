package uploader

import (
	"auto-backup/config"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const ( // 替换为你要上传的文件路径
	uploadURLTemplate = "https://graph.microsoft.com/v1.0/me/drive/root:/%s/%s:/createUploadSession" // 替换为目标路径
	chunkSize         = 8 * 1024 * 1024                                                              // 每块大小设置为 8MB
	maxRetries        = 3
	retryDelay        = 5 * time.Second
	tokenURL          = "https://login.microsoftonline.com/common/oauth2/v2.0/token"
)

// OneDriveConfig OneDrive配置
type OneDriveConfig struct {
	AccessToken  string
	RefreshToken string
	ExpireTime   time.Time
	BasePath     string
}

type UploadSession struct {
	UploadURL string `json:"uploadUrl"`
}

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
}

// OneDriveUploader OneDrive上传实现
type OneDriveUploader struct {
	config *OneDriveConfig
	client *http.Client
}

func NewOneDriveUploader(config *OneDriveConfig) (*OneDriveUploader, error) {
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	return &OneDriveUploader{
		config: config,
		client: client,
	}, nil
}

// GetAuthUrl 获取授权URL
func (u *OneDriveUploader) GetAuthUrl() string {
	return fmt.Sprintf("https://login.live.com/oauth20_authorize.srf?client_id=%s&scope=%s&response_type=code&redirect_uri=%s",
		config.GlobalConfig.OneDrive.ClientID,
		config.GlobalConfig.OneDrive.Scope,
		config.GlobalConfig.OneDrive.RedirectURI)
}

func (u *OneDriveUploader) GetAccessTokenByCode(code string) error {
	// 构建请求参数
	formData := url.Values{
		"client_id":     {config.GlobalConfig.OneDrive.ClientID},
		"redirect_uri":  {config.GlobalConfig.OneDrive.RedirectURI},
		"client_secret": {config.GlobalConfig.OneDrive.ClientSecret},
		"code":          {code},
		"grant_type":    {"authorization_code"},
	}

	req, err := http.NewRequest("POST", tokenURL, bytes.NewBufferString(formData.Encode()))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := u.client.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	// 读取响应内容
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	// 解析响应JSON
	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return err
	}

	u.config.AccessToken = tokenResp.AccessToken
	u.config.RefreshToken = tokenResp.RefreshToken
	u.config.ExpireTime = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)

	return nil
}

// RefreshByRefreshToken 使用refreshToken刷新accessToken
func (u *OneDriveUploader) RefreshAccessToken() error {

	// 如果accessToken不为空且未过期，则直接返回
	if u.config.AccessToken != "" && time.Now().Before(u.config.ExpireTime.Add(-5*time.Minute)) {
		return nil
	}

	// 如果refreshToken不为空，则使用refreshToken获取accessToken
	if u.config.RefreshToken != "" {
		formData := url.Values{
			"client_id":     {config.GlobalConfig.OneDrive.ClientID},
			"redirect_uri":  {config.GlobalConfig.OneDrive.RedirectURI},
			"client_secret": {config.GlobalConfig.OneDrive.ClientSecret},
			"refresh_token": {u.config.RefreshToken},
			"grant_type":    {"refresh_token"},
		}

		req, err := http.NewRequest("POST", tokenURL, bytes.NewBufferString(formData.Encode()))
		if err != nil {
			return err
		}

		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		resp, err := u.client.Do(req)
		if err != nil {
			return err
		}

		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("获取accessToken失败，状态码: %d", resp.StatusCode)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}

		var tokenResponse TokenResponse
		if err := json.Unmarshal(body, &tokenResponse); err != nil {
			return err
		}

		u.config.AccessToken = tokenResponse.AccessToken
		u.config.RefreshToken = tokenResponse.RefreshToken
		u.config.ExpireTime = time.Now().Add(time.Duration(tokenResponse.ExpiresIn) * time.Second)

		return nil
	} else {
		// 用户手动认证

	}

	return nil
}

// 创建上传会话
func (u *OneDriveUploader) createUploadSession(uploadURL string) (*UploadSession, error) {
	payload := map[string]interface{}{
		"item": map[string]string{
			"@microsoft.graph.conflictBehavior": "rename", // 如果文件已存在，重命名
		},
	}

	payloadBytes, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", uploadURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+u.config.AccessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := u.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to create upload session: %s", string(body))
	}

	var session UploadSession
	err = json.NewDecoder(resp.Body).Decode(&session)
	if err != nil {
		return nil, err
	}

	return &session, nil
}

// 分块上传文件
func (u *OneDriveUploader) uploadFileChunks(session *UploadSession, localFilePath string) error {
	file, err := os.Open(localFilePath)
	if err != nil {
		return fmt.Errorf("无法打开文件: %w", err)
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		return fmt.Errorf("无法获取文件信息: %w", err)
	}
	fileSize := fileInfo.Size()

	client := &http.Client{
		Timeout: 30 * time.Second, // 添加超时设置
	}

	start := int64(0)
	for {
		end := start + chunkSize - 1
		if end >= fileSize {
			end = fileSize - 1
		}

		// 添加重试逻辑
		retryCount := 0
		for retryCount < maxRetries {
			chunk := make([]byte, end-start+1)
			_, err := file.ReadAt(chunk, start)
			if err != nil && err != io.EOF {
				return fmt.Errorf("读取文件块失败: %w", err)
			}

			req, err := http.NewRequest("PUT", session.UploadURL, bytes.NewReader(chunk))
			if err != nil {
				return fmt.Errorf("创建请求失败: %w", err)
			}

			contentRange := fmt.Sprintf("bytes %d-%d/%d", start, end, fileSize)
			req.Header.Set("Content-Range", contentRange)
			req.Header.Set("Content-Length", fmt.Sprintf("%d", len(chunk)))

			resp, err := client.Do(req)
			if err != nil {
				retryCount++
				if retryCount < maxRetries {
					time.Sleep(retryDelay)
					continue
				}
				return fmt.Errorf("上传块失败: %w", err)
			}

			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()

			// 处理响应
			if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated {
				fmt.Printf("文件上传完成: %d/%d 字节\n", end+1, fileSize)
				return nil
			} else if resp.StatusCode == http.StatusAccepted {
				var serverResponse map[string]interface{}
				if err := json.Unmarshal(body, &serverResponse); err != nil {
					return fmt.Errorf("解析响应失败: %w", err)
				}

				// 更新上传进度
				if nextRanges, ok := serverResponse["nextExpectedRanges"].([]interface{}); ok && len(nextRanges) > 0 {
					rangeParts := strings.Split(nextRanges[0].(string), "-")
					newStart, _ := strconv.ParseInt(rangeParts[0], 10, 64)
					if newStart > start {
						start = newStart
						fmt.Printf("上传进度: %.2f%%\n", float64(start)/float64(fileSize)*100)
						break // 成功上传当前块，继续下一块
					}
				}
			} else {
				retryCount++
				if retryCount < maxRetries {
					time.Sleep(retryDelay)
					continue
				}
				return fmt.Errorf("上传块失败，状态码: %d，响应: %s", resp.StatusCode, string(body))
			}
		}

		if start >= fileSize {
			break
		}
	}

	return nil
}

func (u *OneDriveUploader) UploadBigFile(folderPath, localFilePath string) error {

	// 从文件路径中获取文件名
	fileName := filepath.Base(localFilePath)
	uploadURL := fmt.Sprintf(uploadURLTemplate, folderPath, fileName)

	// Step 1: 获取上传会话
	uploadSession, err := u.createUploadSession(uploadURL)
	if err != nil {
		return err
	}

	// Step 2: 分块上传文件
	err = u.uploadFileChunks(uploadSession, localFilePath)
	if err != nil {
		return err
	}

	return nil
}
