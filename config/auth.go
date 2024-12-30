package config

import (
	"sync"
)

var (
	// 全局认证信息实例
	GlobalAuthInfo = &AuthInfo{}
)

type AuthInfo struct {
	AccessToken  string       `db:"access_token"`
	RefreshToken string       `db:"refresh_token"`
	ExpiresIn    int          `db:"expires_in"`
	UserID       string       `db:"user_id"`
	mu           sync.RWMutex `db:"-"`
}

// 设置认证信息
func (a *AuthInfo) SetAuthInfo(accessToken, refreshToken, userID string, expiresIn int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.AccessToken = accessToken
	a.RefreshToken = refreshToken
	a.UserID = userID
	a.ExpiresIn = expiresIn
}

// 获取 access token
func (a *AuthInfo) GetAccessToken() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.AccessToken
}

// 获取 refresh token
func (a *AuthInfo) GetRefreshToken() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.RefreshToken
}
