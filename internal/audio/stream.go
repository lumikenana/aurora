package audio

import "io"

// Stream 音频流抽象
type Stream struct {
	Reader io.Reader // PCM 数据读取器
	Format Format    // 音频格式
	Close  func()    // 清理资源的函数（可选）
}
