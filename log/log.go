package log

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"

	"gopkg.in/natefinch/lumberjack.v2"
)

var logger *slog.Logger
var logConfig = LogConfig{
	Level:      INFO,
	Filename:   "",
	MaxSize:    10,
	MaxBackups: 3,
	Compress:   true,
}

type LogConfig struct {
	Level      LogLevel
	Filename   string
	MaxSize    int
	MaxBackups int
	Compress   bool
}

type multiHandler struct {
	handlers []slog.Handler
}

func (h *multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (h *multiHandler) Handle(ctx context.Context, r slog.Record) error {
	for _, handler := range h.handlers {
		if err := handler.Handle(ctx, r); err != nil {
			return err
		}
	}
	return nil
}

func (h *multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	handlers := make([]slog.Handler, len(h.handlers))
	for i, handler := range h.handlers {
		handlers[i] = handler.WithAttrs(attrs)
	}
	return &multiHandler{handlers: handlers}
}

func (h *multiHandler) WithGroup(name string) slog.Handler {
	handlers := make([]slog.Handler, len(h.handlers))
	for i, handler := range h.handlers {
		handlers[i] = handler.WithGroup(name)
	}
	return &multiHandler{handlers: handlers}
}

func init() {
	opts := &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}
	handler := slog.NewTextHandler(os.Stdout, opts)
	logger = slog.New(handler)
	slog.SetDefault(logger)
}

func GetLogConfig() LogConfig {
	return logConfig
}

func SetLogConfig(config LogConfig) {
	handlers := []slog.Handler{}

	// 添加标准输出处理器
	stdoutOpts := &slog.HandlerOptions{
		Level:     convertLevel(config.Level),
		AddSource: false,
	}
	stdoutHandler := slog.NewTextHandler(os.Stdout, stdoutOpts)
	handlers = append(handlers, stdoutHandler)

	// 如果配置了文件输出，添加文件处理器
	if config.Filename != "" {
		dir := filepath.Dir(config.Filename)
		if err := os.MkdirAll(dir, os.ModePerm); err != nil {
			logger.Error("Failed to create log directory", "dir", dir, "error", err)
			return
		}

		// 使用 lumberjack 进行日志轮转
		writer := &lumberjack.Logger{
			Filename:   config.Filename,
			MaxSize:    config.MaxSize,
			MaxBackups: config.MaxBackups,
			Compress:   config.Compress,
			LocalTime:  true,
		}

		fileOpts := &slog.HandlerOptions{
			Level:     convertLevel(config.Level),
			AddSource: false,
		}
		fileHandler := slog.NewJSONHandler(writer, fileOpts)
		handlers = append(handlers, fileHandler)
	}

	// 创建多重处理器
	multiHandler := &multiHandler{handlers: handlers}
	logger = slog.New(multiHandler)
	slog.SetDefault(logger)

	logConfig = config
}

func Info(format string, v ...any) {
	print(INFO, fmt.Sprintf(format, v...))
}

func Warn(format string, v ...any) {
	print(WARNING, fmt.Sprintf(format, v...))
}

func Error(format string, v ...any) {
	print(ERROR, fmt.Sprintf(format, v...))
}

func Debug(format string, v ...any) {
	print(DEBUG, fmt.Sprintf(format, v...))
}

func Fatal(format string, v ...any) {
	logger.Error(fmt.Sprintf(format, v...))
	os.Exit(1)
}

func print(level LogLevel, data string) {
	if level < logConfig.Level {
		return
	}

	// 获取调用者信息
	_, file, line, ok := runtime.Caller(2) // 使用 2 来跳过当前函数和调用者函数
	if !ok {
		file = "unknown"
		line = 0
	}
	_, fileName := filepath.Split(file)
	source := fmt.Sprintf("%s:%d", fileName, line)

	switch level {
	case INFO:
		logger.Info(data, "source", source)
	case WARNING:
		logger.Warn(data, "source", source)
	case ERROR:
		logger.Error(data, "source", source)
	case DEBUG:
		logger.Debug(data, "source", source)
	}
}

// 将自定义日志级别转换为 slog 级别
func convertLevel(level LogLevel) slog.Level {
	switch level {
	case DEBUG:
		return slog.LevelDebug
	case INFO:
		return slog.LevelInfo
	case WARNING:
		return slog.LevelWarn
	case ERROR:
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
