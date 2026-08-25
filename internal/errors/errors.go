package errors

import (
	"fmt"
	"runtime"
)

// AppError 应用错误接口
type AppError interface {
	error
	Code() int
	Message() string
	Cause() error
	Stack() string
}

// appError 应用错误实现
type appError struct {
	code    int
	message string
	cause   error
	stack   string
}

// New 创建一个新的应用错误
func New(code int, message string) AppError {
	return &appError{
		code:    code,
		message: message,
		stack:   getStack(),
	}
}

// Newf 创建一个新的格式化应用错误
func Newf(code int, format string, args ...interface{}) AppError {
	return &appError{
		code:    code,
		message: fmt.Sprintf(format, args...),
		stack:   getStack(),
	}
}

// Wrap 包装一个已有错误
func Wrap(code int, message string, cause error) AppError {
	return &appError{
		code:    code,
		message: message,
		cause:   cause,
		stack:   getStack(),
	}
}

// Error 实现error接口
func (e *appError) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("%s: %v", e.message, e.cause)
	}
	return e.message
}

// Code 返回错误码
func (e *appError) Code() int {
	return e.code
}

// Message 返回错误消息
func (e *appError) Message() string {
	return e.message
}

// Cause 返回根因
func (e *appError) Cause() error {
	return e.cause
}

// Stack 返回堆栈信息
func (e *appError) Stack() string {
	return e.stack
}

// Unwrap 支持errors.Unwrap
func (e *appError) Unwrap() error {
	return e.cause
}

// 错误码定义
const (
	CodeSuccess         = 0
	CodeUnknown         = 1000
	CodeConfigError     = 1001
	CodeIOError         = 1002
	CodePlatformError   = 1003
	CodeTrayError       = 1004
	CodeWindowError     = 1005
	CodeStorageError    = 1006
	CodePermissionError = 1007
	CodeInputError      = 1008
	CodeScreenshotFailed = 1009
	CodeInvalidConfig   = 1100
	CodeLLMRequest      = 1101
	CodeLLMNetwork      = 1102
	CodeLLMAPIError     = 1103
	CodeLLMResponse     = 1104
)

// Is 判断错误是否是指定类型
func Is(err error, code int) bool {
	if err == nil {
		return false
	}
	appErr, ok := err.(AppError)
	if !ok {
		return false
	}
	return appErr.Code() == code
}

// getStack 获取堆栈信息
func getStack() string {
	var stack []byte
	stack = make([]byte, 4096)
	n := runtime.Stack(stack, false)
	return string(stack[:n])
}
