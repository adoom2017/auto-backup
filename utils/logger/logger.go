package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// 日志级别
const (
	LogDebug = iota
	LogInfo
	LogWarn
	LogError
)

// 日志记录器
type Logger struct {
	debug    *log.Logger
	info     *log.Logger
	warn     *log.Logger
	error    *log.Logger
	level    int
	logDir   string   // 日志目录
	baseFile string   // 基础文件名
	curFile  *os.File // 当前日志文件
	curDate  string   // 当前日期
	maxDays  int      // 保留的最大天数
	mu       sync.RWMutex
	ticker   *time.Ticker
	done     chan bool
}

// 全局日志记录器
var logger *Logger

func InitLogger(baseName string, level int, maxDays int) {
	var err error
	logger, err = newLogger(
		baseName, // 基础文件名
		level,    // 日志级别
		maxDays,  // 保留最近7天的日志
	)
	if err != nil {
		panic("无法创建日志记录器: " + err.Error())
	}
}

func GetLogger() *Logger {
	if logger == nil {
		panic("日志记录器未初始化")
	}

	return logger
}

// 创建新的日志记录器
func newLogger(baseFile string, level int, maxDays int) (*Logger, error) {

	logDir := "./logs"

	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("创建日志目录失败: %v", err)
	}

	logger := &Logger{
		level:    level,
		logDir:   logDir,
		baseFile: baseFile,
		maxDays:  maxDays,
		done:     make(chan bool),
	}

	// 初始化日志文件
	if err := logger.rotateLog(); err != nil {
		return nil, err
	}

	// 清理旧日志
	if err := logger.cleanOldLogs(); err != nil {
		return nil, err
	}

	// 启动定时检查
	logger.startScheduler()

	return logger, nil
}

// 启动定时检查调度器
func (l *Logger) startScheduler() {
	l.ticker = time.NewTicker(1 * time.Hour)
	go func() {
		for {
			select {
			case <-l.ticker.C:
				l.mu.Lock()
				_ = l.cleanOldLogs()
				l.mu.Unlock()
			case <-l.done:
				l.ticker.Stop()
				return
			}
		}
	}()
}

// 关闭日志记录器
func (l *Logger) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.done != nil {
		close(l.done)
	}
	if l.curFile != nil {
		l.curFile.Close()
	}
}

// 轮转日志文件
func (l *Logger) rotateLog() error {
	curDate := time.Now().Format("20060102")

	if curDate == l.curDate && l.curFile != nil {
		return nil
	}

	if l.curFile != nil {
		l.curFile.Close()
	}

	fileName := fmt.Sprintf("%s.%s.log", l.baseFile, curDate)
	filePath := filepath.Join(l.logDir, fileName)

	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return err
	}

	writers := io.MultiWriter(os.Stdout, file)

	l.debug = log.New(writers, "[DEBUG] ", log.Ldate|log.Ltime|log.Lshortfile)
	l.info = log.New(writers, "[INFO] ", log.Ldate|log.Ltime|log.Lshortfile)
	l.warn = log.New(writers, "[WARN] ", log.Ldate|log.Ltime|log.Lshortfile)
	l.error = log.New(writers, "[ERROR] ", log.Ldate|log.Ltime|log.Lshortfile)

	l.curFile = file
	l.curDate = curDate

	return nil
}

// 清理旧日志
func (l *Logger) cleanOldLogs() error {
	files, err := os.ReadDir(l.logDir)
	if err != nil {
		return err
	}

	var logFiles []string
	for _, file := range files {
		if strings.HasPrefix(file.Name(), l.baseFile) && strings.HasSuffix(file.Name(), ".log") {
			logFiles = append(logFiles, file.Name())
		}
	}

	sort.Strings(logFiles)

	if len(logFiles) > l.maxDays {
		for _, file := range logFiles[:len(logFiles)-l.maxDays] {
			if err := os.Remove(filepath.Join(l.logDir, file)); err != nil {
				return err
			}
		}
	}

	return nil
}

// 快速检查是否需要轮转日志
func (l *Logger) checkDate() error {
	curDate := time.Now().Format("20060102")
	l.mu.RLock()
	if curDate == l.curDate {
		l.mu.RUnlock()
		return nil
	}
	l.mu.RUnlock()

	// 如果日期变化，则获取写锁并执行轮转
	l.mu.Lock()
	defer l.mu.Unlock()

	// 双重检查，避免在获取写锁期间其他协程已经完成轮转
	if curDate == l.curDate {
		return nil
	}

	return l.rotateLog()
}

func (l *Logger) Debug(format string, v ...interface{}) {
	if err := l.checkDate(); err != nil {
		fmt.Printf("日志轮转错误: %v\n", err)
		return
	}
	if l.level <= LogDebug {
		l.debug.Output(3, fmt.Sprintf(format, v...))
	}
}

func (l *Logger) Info(format string, v ...interface{}) {
	if err := l.checkDate(); err != nil {
		fmt.Printf("日志轮转错误: %v\n", err)
		return
	}
	if l.level <= LogInfo {
		l.info.Output(3, fmt.Sprintf(format, v...))
	}
}

func (l *Logger) Warn(format string, v ...interface{}) {
	if err := l.checkDate(); err != nil {
		fmt.Printf("日志轮转错误: %v\n", err)
		return
	}
	if l.level <= LogWarn {
		l.warn.Output(3, fmt.Sprintf(format, v...))
	}
}

func (l *Logger) Error(format string, v ...interface{}) {
	if err := l.checkDate(); err != nil {
		fmt.Printf("日志轮转错误: %v\n", err)
		return
	}
	if l.level <= LogError {
		l.error.Output(3, fmt.Sprintf(format, v...))
	}
}
