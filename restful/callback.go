package restful

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"auto-backup/config"
	"auto-backup/utils/db"
	"auto-backup/utils/logger"

	"github.com/gin-gonic/gin"
)

// 定义响应结构
type TokenResponse struct {
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	UserID       string `json:"user_id"`
}

type OneDriveServer struct {
	port   int
	router *gin.Engine
	log    *logger.Logger
	srv    *http.Server
	quit   chan os.Signal
}

func NewOneDriveServer(port int) *OneDriveServer {
	router := gin.Default()
	return &OneDriveServer{
		port:   port,
		router: router,
		log:    logger.GetLogger(),
		quit:   make(chan os.Signal, 1),
	}
}

func (s *OneDriveServer) Start() {
	s.router.GET("/token", s.getToken)

	s.srv = &http.Server{
		Addr:    fmt.Sprintf(":%d", s.port),
		Handler: s.router,
	}

	// 在goroutine中启动服务器
	go func() {
		if err := s.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.log.Error("监听失败: %v", err)
		}
	}()

	// 监听中断信号
	signal.Notify(s.quit, syscall.SIGINT, syscall.SIGTERM)
}

func (s *OneDriveServer) Stop() {
	s.log.Info("正在关闭服务器...")

	// 设置5秒超时的上下文
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 优雅关闭
	if err := s.srv.Shutdown(ctx); err != nil {
		s.log.Error("服务器强制关闭: %v", err)
	}

	s.log.Info("服务器已退出")
}

func (s *OneDriveServer) WaitForSignal() {
	<-s.quit
}

func (s *OneDriveServer) GetAuthUrl() string {
	return fmt.Sprintf("https://login.live.com/oauth20_authorize.srf?client_id=%s&scope=%s&response_type=code&redirect_uri=%s",
		config.GlobalConfig.OneDrive.ClientID,
		config.GlobalConfig.OneDrive.Scope,
		config.GlobalConfig.OneDrive.RedirectURI)
}

func (s *OneDriveServer) getToken(c *gin.Context) {

	log := logger.GetLogger()

	// 获取code
	code := c.Query("code")

	// 返回响应
	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
	})

	// 构建请求参数
	formData := url.Values{
		"client_id":     {config.GlobalConfig.OneDrive.ClientID},
		"redirect_uri":  {config.GlobalConfig.OneDrive.RedirectURI},
		"client_secret": {config.GlobalConfig.OneDrive.ClientSecret},
		"code":          {code},
		"grant_type":    {"authorization_code"},
	}

	// 发送POST请求
	resp, err := http.PostForm("https://login.live.com/oauth20_token.srf", formData)
	if err != nil {
		log.Error("发送token请求失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}
	defer resp.Body.Close()

	// 读取响应内容
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error("读取响应失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	log.Info("token响应: %s", string(body))

	// 解析响应JSON
	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		log.Error("解析token响应失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	config.GlobalAuthInfo.SetAuthInfo(tokenResp.AccessToken, tokenResp.RefreshToken, tokenResp.UserID, tokenResp.ExpiresIn)

	err = db.SaveAuthInfo(config.GlobalAuthInfo)
	if err != nil {
		log.Error("保存认证信息失败: %v", err)
	}
}
