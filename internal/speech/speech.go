// Package speech 定义语音识别 (ASR) Provider 接口
package speech

// Transcript 识别结果
type Transcript struct {
	Text      string `json:"text"`       // 识别文本
	IsFinal   bool   `json:"is_final"`   // true=最终结果, false=部分结果
	SessionID string `json:"session_id"` // 会话 ID
}

// TranscriptCallback 识别结果回调
type TranscriptCallback func(transcript Transcript)

// SpeechRecognizer 语音识别 Provider 接口
// Agent 依赖此接口，不依赖具体 ASR 实现
type SpeechRecognizer interface {
	// StartSession 开始一次识别会话
	// audioCh: 音频数据输入通道（PCM 16-bit, 16kHz, mono）
	// callback: 识别结果回调（partial + final）
	StartSession(audioCh <-chan []byte, callback TranscriptCallback) error

	// EndSession 结束识别会话（发送最后音频段并等待最终结果）
	EndSession() error

	// CancelSession 取消识别会话（立即中断，丢弃结果）
	CancelSession() error

	// IsConfigured 检查是否已配置 API Key
	IsConfigured() bool

	// Name 返回 Provider 名称
	Name() string
}

// Config ASR 模块配置
type Config struct {
	Provider   string         `json:"provider"`   // "volcengine"
	Volcengine VolcengineConfig `json:"volcengine"`
}

// VolcengineConfig 火山引擎 ASR 配置
type VolcengineConfig struct {
	APIKey     string `json:"api_key"`     // API Key (never logged)
	Endpoint   string `json:"endpoint"`    // WebSocket endpoint URL
	ResourceID string `json:"resource_id"` // 资源 ID（在火山引擎控制台查看）
}

// DefaultConfig 返回默认 ASR 配置
func DefaultConfig() Config {
	return Config{
		Provider: "volcengine",
		Volcengine: VolcengineConfig{
			Endpoint:   "wss://openspeech.bytedance.com/api/v3/sauc/bigmodel",
			ResourceID: "volc.bigasr.sauc.duration",
		},
	}
}
