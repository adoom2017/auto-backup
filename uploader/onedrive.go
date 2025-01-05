package uploader

import (
	"auto-backup/db"
	"auto-backup/log"
	"auto-backup/model"
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
	ClientID     string
	ClientSecret string
	RedirectURI  string
	Scope        string
	AccessToken  string
	RefreshToken string
	ExpireTime   time.Time
}

type UploadSession struct {
	UploadURL string `json:"uploadUrl"`
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

// 设置认证信息
func (u *OneDriveUploader) SetAuthInfo(authInfo *db.AuthInfo) {
	u.config.AccessToken = authInfo.AccessToken
	u.config.RefreshToken = authInfo.RefreshToken
	u.config.ExpireTime = time.Unix(authInfo.ExpiresIn, 0)
}

func (u *OneDriveUploader) GetAccessTokenByCode(code string) error {
	// 构建请求参数
	formData := url.Values{
		"client_id":     {u.config.ClientID},
		"redirect_uri":  {u.config.RedirectURI},
		"client_secret": {u.config.ClientSecret},
		"code":          {code},
		"grant_type":    {"authorization_code"},
	}

	req, err := http.NewRequest("POST", tokenURL, bytes.NewBufferString(formData.Encode()))
	if err != nil {
		log.Error("创建请求失败: %v", err)
		return err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := u.client.Do(req)
	if err != nil {
		log.Error("发送请求失败: %v", err)
		return err
	}

	defer resp.Body.Close()

	// 读取响应内容
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error("读取响应内容失败: %v", err)
		return err
	}

	// 解析响应JSON
	var tokenResp model.TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		log.Error("解析响应JSON失败: %v", err)
		return err
	}

	u.config.AccessToken = tokenResp.AccessToken
	u.config.RefreshToken = tokenResp.RefreshToken
	u.config.ExpireTime = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)

	// 保存认证信息
	db.SaveAuthInfo(&db.AuthInfo{
		AccessToken:  u.config.AccessToken,
		RefreshToken: u.config.RefreshToken,
		ExpiresIn:    u.config.ExpireTime.Unix(),
		UserID:       tokenResp.UserID,
	})

	log.Info("认证成功: code: %s, 过期时间: %s", code, u.config.ExpireTime.Format(time.RFC3339))

	return nil
}

// RefreshByRefreshToken 使用refreshToken刷新accessToken
func (u *OneDriveUploader) RefreshAccessToken() error {

	formData := url.Values{
		"client_id":     {u.config.ClientID},
		"redirect_uri":  {u.config.RedirectURI},
		"client_secret": {u.config.ClientSecret},
		"refresh_token": {u.config.RefreshToken},
		"grant_type":    {"refresh_token"},
	}

	req, err := http.NewRequest("POST", tokenURL, bytes.NewBufferString(formData.Encode()))
	if err != nil {
		log.Error("创建请求失败: %v", err)
		return err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := u.client.Do(req)
	if err != nil {
		log.Error("发送请求失败: %v", err)
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Error("获取accessToken失败，状态码: %d", resp.StatusCode)
		return fmt.Errorf("获取accessToken失败，状态码: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error("读取响应内容失败: %v", err)
		return err
	}

	var tokenResponse model.TokenResponse
	if err := json.Unmarshal(body, &tokenResponse); err != nil {
		log.Error("解析响应JSON失败: %v", err)
		return err
	}

	u.config.AccessToken = tokenResponse.AccessToken
	u.config.RefreshToken = tokenResponse.RefreshToken
	u.config.ExpireTime = time.Now().Add(time.Duration(tokenResponse.ExpiresIn) * time.Second)

	// 保存认证信息
	db.SaveAuthInfo(&db.AuthInfo{
		AccessToken:  u.config.AccessToken,
		RefreshToken: u.config.RefreshToken,
		ExpiresIn:    u.config.ExpireTime.Unix(),
		UserID:       tokenResponse.UserID,
	})

	log.Info("刷新accessToken成功，过期时间: %s", u.config.ExpireTime.Format(time.RFC3339))

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
		log.Error("创建请求失败: %v", err)
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+u.config.AccessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := u.client.Do(req)
	if err != nil {
		log.Error("发送请求失败: %v", err)
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Error("创建上传会话失败: %s", string(body))
		return nil, fmt.Errorf("failed to create upload session: %s", string(body))
	}

	var session UploadSession
	err = json.NewDecoder(resp.Body).Decode(&session)
	if err != nil {
		log.Error("解析响应JSON失败: %v", err)
		return nil, err
	}

	return &session, nil
}

// 分块上传文件
func (u *OneDriveUploader) uploadFileChunks(session *UploadSession, localFilePath string) error {
	file, err := os.Open(localFilePath)
	if err != nil {
		log.Error("无法打开文件: %v", err)
		return fmt.Errorf("无法打开文件: %w", err)
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		log.Error("无法获取文件信息: %v", err)
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
				log.Error("读取文件块失败: %v", err)
				return fmt.Errorf("读取文件块失败: %w", err)
			}

			req, err := http.NewRequest("PUT", session.UploadURL, bytes.NewReader(chunk))
			if err != nil {
				log.Error("创建请求失败: %v", err)
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
				log.Error("上传块失败: %v", err)
				return fmt.Errorf("上传块失败: %w", err)
			}

			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()

			// 处理响应
			if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated {
				log.Info("文件上传完成: %d/%d 字节", end+1, fileSize)
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
						log.Info("上传进度: %.2f%%", float64(start)/float64(fileSize)*100)
						break // 成功上传当前块，继续下一块
					}
				}
			} else {
				retryCount++
				if retryCount < maxRetries {
					time.Sleep(retryDelay)
					continue
				}
				log.Error("上传块失败，状态码: %d，响应: %s", resp.StatusCode, string(body))
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

	log.Info("开始上传文件: %s", localFilePath)

	// 从文件路径中获取文件名
	fileName := filepath.Base(localFilePath)
	uploadURL := fmt.Sprintf(uploadURLTemplate, folderPath, fileName)

	log.Info("上传URL: %s", uploadURL)

	// Step 1: 获取上传会话
	uploadSession, err := u.createUploadSession(uploadURL)
	if err != nil {
		log.Error("创建上传会话失败: %v", err)
		return err
	}

	log.Info("上传会话: %v", uploadSession)

	// Step 2: 分块上传文件
	err = u.uploadFileChunks(uploadSession, localFilePath)
	if err != nil {
		log.Error("分块上传文件失败: %v", err)
		return err
	}

	log.Info("文件上传完成")

	return nil
}
