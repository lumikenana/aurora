package log

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"time"
)

var (
	logger *Logger
)

// LogLevel 日志级别
type LogLevel int

const (
	LevelDebug LogLevel = iota
	LevelInfo
	LevelWarn
	LevelError
	LevelFatal
)

// Logger 日志记录器
type Logger struct {
	level     LogLevel
	file      *os.File
	isConsole bool
	logDir    string
	onLog     func(level LogLevel, message string)
}

// Init 初始化日志系统
func Init() {
	level := LevelDebug
	isConsole := true

	logger = &Logger{
		level:     level,
		isConsole: isConsole,
	}

	Info("Aurora logger initialized")
}

// InitWithLogDir 初始化日志系统并指定日志目录
// 应该在config初始化后调用
func InitWithLogDir(logDir string) {
	if logger == nil {
		Init()
	}

	// 尝试打开日志文件
	if err := os.MkdirAll(logDir, 0755); err == nil {
		logPath := filepath.Join(logDir, fmt.Sprintf("aurora_%s.log", time.Now().Format("20060102")))
		file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err == nil {
			logger.file = file
			Info("Log file opened: %s", logPath)
		}
	}
}

// SetLevel 设置日志级别
func SetLevelFromString(levelStr string) {
	if logger == nil {
		return
	}
	switch levelStr {
	case "debug":
		logger.level = LevelDebug
	case "info":
		logger.level = LevelInfo
	case "warn":
		logger.level = LevelWarn
	case "error":
		logger.level = LevelError
	}
}

// Shutdown 关闭日志
func Shutdown() {
	if logger != nil && logger.file != nil {
		_ = logger.file.Close()
	}
}

// SetLevel 设置日志级别
func SetLevel(level LogLevel) {
	if logger != nil {
		logger.level = level
	}
}

// SetOnLogCallback 设置日志回调，用于将日志发送到前端
func SetOnLogCallback(callback func(level LogLevel, message string)) {
	if logger != nil {
		logger.onLog = callback
	}
}

// Debug 输出调试日志
func Debug(format string, args ...interface{}) {
	if logger != nil {
		logger.log(LevelDebug, format, args...)
	}
}

// Debugf 输出调试日志（别名）
func Debugf(format string, args ...interface{}) {
	Debug(format, args...)
}

// Info 输出信息日志
func Info(format string, args ...interface{}) {
	if logger != nil {
		logger.log(LevelInfo, format, args...)
	}
}

// Infof 输出信息日志（别名）
func Infof(format string, args ...interface{}) {
	Info(format, args...)
}

// Warn 输出警告日志
func Warn(format string, args ...interface{}) {
	if logger != nil {
		logger.log(LevelWarn, format, args...)
	}
}

// Warnf 输出警告日志（别名）
func Warnf(format string, args ...interface{}) {
	Warn(format, args...)
}

// Error 输出错误日志
func Error(format string, args ...interface{}) {
	if logger != nil {
		logger.log(LevelError, format, args...)
	}
}

// Errorf 输出错误日志（别名）
func Errorf(format string, args ...interface{}) {
	Error(format, args...)
}

// Fatal 输出致命错误并退出
func Fatal(format string, args ...interface{}) {
	if logger != nil {
		logger.log(LevelFatal, format, args...)
	}
	os.Exit(1)
}

// Fatalf 输出致命错误并退出（别名）
func Fatalf(format string, args ...interface{}) {
	Fatal(format, args...)
}

// debug 实例方法
func (l *Logger) debug(format string, args ...interface{}) {
	l.log(LevelDebug, format, args...)
}

// info 实例方法
func (l *Logger) info(format string, args ...interface{}) {
	l.log(LevelInfo, format, args...)
}

// warn 实例方法
func (l *Logger) warn(format string, args ...interface{}) {
	l.log(LevelWarn, format, args...)
}

// error 实例方法
func (l *Logger) error(format string, args ...interface{}) {
	l.log(LevelError, format, args...)
}

// log 核心日志方法
func (l *Logger) log(level LogLevel, format string, args ...interface{}) {
	if level < l.level {
		return
	}

	levelStr := getLevelString(level)
	timestamp := time.Now().Format("2006-01-02 15:04:05.000")
	msg := fmt.Sprintf(fmt.Sprintf("[%s] %s %s\n", timestamp, levelStr, format), args...)
	plainMsg := fmt.Sprintf(format, args...)

	if l.isConsole {
		fmt.Print(msg)
	}
	if l.file != nil {
		_, _ = l.file.WriteString(msg)
	}
	// 如果设置了回调，发送日志到前端
	if l.onLog != nil {
		l.onLog(level, plainMsg)
	}
}

// getLevelString 获取级别字符串
func getLevelString(level LogLevel) string {
	switch level {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return " INFO"
	case LevelWarn:
		return " WARN"
	case LevelError:
		return "ERROR"
	case LevelFatal:
		return "FATAL"
	default:
		return "?????"
	}
}

// RandomID 生成随机ID字符串
func RandomID(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	seededRand := rand.New(rand.NewSource(time.Now().UnixNano()))
	result := make([]byte, length)
	for i := range result {
		result[i] = charset[seededRand.Intn(len(charset))]
	}
	return string(result)
}
