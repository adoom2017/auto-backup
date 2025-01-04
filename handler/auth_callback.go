package handler

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"auto-backup/log"
	"auto-backup/model"

	"github.com/gin-gonic/gin"
)

type AuthHandlerServer struct {
	port   int
	router *gin.Engine
	srv    *http.Server
	notify chan model.TokenAction
}

func NewAuthHandlerServer(port int, notify chan model.TokenAction) *AuthHandlerServer {
	router := gin.Default()
	return &AuthHandlerServer{
		port:   port,
		router: router,
		notify: notify,
	}
}

func (s *AuthHandlerServer) Start(ctx context.Context) {
	s.router.GET("/token", s.getToken)

	s.srv = &http.Server{
		Addr:    fmt.Sprintf(":%d", s.port),
		Handler: s.router,
	}

	// 在goroutine中启动服务器
	go func() {
		if err := s.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("监听失败: %v", err)
		}

		log.Info("服务器已成功启动")
	}()

	// 监听context的取消信号
	go func() {
		<-ctx.Done()
		log.Info("收到退出信号,开始关闭服务器...")

		// 设置5秒超时的关闭上下文
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := s.srv.Shutdown(shutdownCtx); err != nil {
			log.Error("服务器关闭出错: %v", err)
		}
		log.Info("服务器已成功关闭")
	}()
}

func (s *AuthHandlerServer) getToken(c *gin.Context) {
	// 获取code
	code := c.Query("code")

	// 返回响应
	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
	})

	// 发送code到channel中
	s.notify <- model.TokenAction{
		Action: "getToken",
		Code:   code,
	}
}
