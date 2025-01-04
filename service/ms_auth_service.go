package service

import (
	"auto-backup/db"
	"auto-backup/log"
	"auto-backup/model"
	"bytes"
	"fmt"
	"time"

	"io"
	"net/http"
	"net/url"
)

var TOKEN_URL = "https://login.microsoftonline.com/common/oauth2/v2.0/token"

type OneDriveAuth struct {
	ClientID     string
	ClientSecret string
	RedirectURI  string
	done         chan bool
	client       *http.Client
}

func NewOneDriveAuth(clientID, clientSecret, redirectURI string, done chan bool) *OneDriveAuth {
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	return &OneDriveAuth{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURI:  redirectURI,
		done:         done,
		client:       client,
	}
}

func (o OneDriveAuth) GetToken(code string) error {
	// 构建请求参数
	formData := url.Values{
		"client_id":     {o.ClientID},
		"redirect_uri":  {o.RedirectURI},
		"client_secret": {o.ClientSecret},
		"code":          {code},
		"grant_type":    {"authorization_code"},
	}

	req, err := http.NewRequest("POST", TOKEN_URL, bytes.NewBufferString(formData.Encode()))
	if err != nil {
		log.Error("创建获取token请求失败: %v", err)
		return err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := o.client.Do(req)
	if err != nil {
		log.Error("发送获取token请求失败: %v", err)
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Error("获取token失败，状态码: %d", resp.StatusCode)
		return fmt.Errorf("获取token失败，状态码: %d", resp.StatusCode)
	}

	// 读取响应内容
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error("读取获取token响应失败: %v", err)
		return err
	}

	log.Info("token响应: %s", string(body))

	// 解析响应JSON
	var tokenResponse model.TokenResponse

	err = tokenResponse.UnmarshalJSON(body)
	if err != nil {
		log.Error("解析token响应失败: %v", err)
		return err
	}

	// 保存认证信息
	err = db.SaveAuthInfo(&db.AuthInfo{
		AccessToken:  tokenResponse.AccessToken,
		RefreshToken: tokenResponse.RefreshToken,
		UserID:       tokenResponse.UserID,
		ExpiresIn:    time.Now().Add(time.Duration(tokenResponse.ExpiresIn) * time.Second).Unix(),
	})

	if err != nil {
		log.Error("保存认证信息失败: %v", err)
		return err
	}

	// 通知完成
	o.done <- true

	return nil
}

func (o OneDriveAuth) RefreshToken(refreshToken string) error {

	formData := url.Values{
		"client_id":     {o.ClientID},
		"redirect_uri":  {o.RedirectURI},
		"client_secret": {o.ClientSecret},
		"refresh_token": {refreshToken},
		"grant_type":    {"refresh_token"},
	}

	req, err := http.NewRequest("POST", TOKEN_URL, bytes.NewBufferString(formData.Encode()))
	if err != nil {
		log.Error("创建刷新token请求失败: %v", err)
		return err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := o.client.Do(req)
	if err != nil {
		log.Error("发送刷新token请求失败: %v", err)
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Error("刷新token失败，状态码: %d", resp.StatusCode)
		return fmt.Errorf("刷新token失败，状态码: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error("读取刷新token响应失败: %v", err)
		return err
	}

	var tokenResponse model.TokenResponse
	if err := tokenResponse.UnmarshalJSON(body); err != nil {
		log.Error("解析刷新token响应失败: %v", err)
		return err
	}

	log.Info("刷新token响应: %v", tokenResponse)

	// 保存认证信息
	err = db.SaveAuthInfo(&db.AuthInfo{
		AccessToken:  tokenResponse.AccessToken,
		RefreshToken: tokenResponse.RefreshToken,
		UserID:       tokenResponse.UserID,
		ExpiresIn:    time.Now().Add(time.Duration(tokenResponse.ExpiresIn) * time.Second).Unix(),
	})

	if err != nil {
		log.Error("保存认证信息失败: %v", err)
		return err
	}

	return nil
}
