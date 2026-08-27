// Package tts 定义语音合成 (TTS) Provider 接口
//
// TTS（Text-To-Speech）用于将 Agent 文字回复转为语音并播放。
// 与 ASR/Provider 类似，TTS 也走 Provider 抽象：核心依赖接口，
// 具体厂商实现在 providers 子包。当前内置火山引擎 HTTP TTS。
package tts

import (
	"github.com/lipipi/aurora/internal/errors"
)

// TTSProvider 语音合成 Provider 接口
//
// 合成是非阻塞的，调用 Synthesize 后立即返回。
// 播放完成时通过 SetOnPlaybackEnd 注册的回调通知。
// 调用方在播放期间可通过 Stop() 中断（用于 PTT 抢占）。
type TTSProvider interface {
	// Synthesize 异步合成并播放文本
	// text: 要朗读的文本
	// 返回 nil 表示已提交（合成/播放是异步的）
	Synthesize(text string) error

	// Stop 停止当前播放（合成阶段无法取消，只能停播放）
	Stop()

	// IsPlaying 是否正在播放
	IsPlaying() bool

	// IsConfigured 检查 API Key 等是否已配置
	IsConfigured() bool

	// Name 返回 Provider 名称
	Name() string

	// SetOnPlaybackEnd 设置播放结束回调（播放完成或被 Stop 时调用）
	SetOnPlaybackEnd(cb func())
}

// Config TTS 模块顶层配置
type Config struct {
	Provider   string             `json:"provider"`    // "volcengine"
	Volcengine VolcengineConfig   `json:"volcengine"`
}

// VolcengineConfig 火山引擎 TTS V3（豆包语音合成大模型 2.0，双向流式 WebSocket）配置
type VolcengineConfig struct {
	// APIKey 控制台 API Key 管理获取（火山控制台 → 语音技术 → API Key 管理）
	APIKey string `json:"api_key"`
	// ResourceId 模型版本：seed-tts-2.0 / seed-icl-2.0
	ResourceId string `json:"resource_id"`
	// Speaker 音色 ID（火山控制台 → 音色库 复制）
	Speaker string `json:"voice_type"`
	// Format 音频格式：mp3 / pcm / ogg_opus / wav
	Format string `json:"format"`
	// SampleRate 采样率 Hz，mp3/pcm 默认 24000，可选 8000/16000/22050/24000/32000/44100/48000
	SampleRate int `json:"sample_rate"`
	// SpeechRate 语速，范围 [-50, 100]；100 = 2.0x，-50 = 0.5x
	SpeechRate int `json:"speech_rate"`
	// LoudnessRate 音量，范围 [-50, 100]；100 = 2.0x，-50 = 0.5x
	LoudnessRate int `json:"loudness_rate"`
	// Pitch 音调，范围 [-12, 12]
	Pitch int `json:"pitch"`
}

// DefaultConfig 返回默认 TTS 配置
func DefaultConfig() Config {
	return Config{
		Provider: "volcengine",
		Volcengine: VolcengineConfig{
			ResourceId: "seed-tts-2.0",
			Speaker:    "saturn_zh_female_keainvsheng_tob",
			Format:     "mp3",
			SampleRate: 24000,
			SpeechRate: 0,
			LoudnessRate: 0,
			Pitch:      0,
		},
	}
}

// ProviderFactory Provider 工厂函数类型
type ProviderFactory func(cfg any) (TTSProvider, errors.AppError)

// 全局 Provider 工厂注册表
var providerFactories = make(map[string]ProviderFactory)

// RegisterProvider 注册 TTS Provider 工厂
func RegisterProvider(providerType string, factory ProviderFactory) {
	providerFactories[providerType] = factory
}

// CreateProvider 根据配置创建 TTS Provider
func CreateProvider(cfg Config) (TTSProvider, errors.AppError) {
	providerType := cfg.Provider
	if providerType == "" {
		providerType = "volcengine"
	}

	factory, exists := providerFactories[providerType]
	if !exists {
		return nil, errors.New(errors.CodeTTSFailed, "unsupported TTS provider: "+providerType)
	}

	// 传递 provider 特定的配置
	var providerCfg any
	switch providerType {
	case "volcengine":
		providerCfg = cfg.Volcengine
	default:
		providerCfg = cfg
	}

	p, err := factory(providerCfg)
	if err != nil {
		return nil, err
	}

	return p, nil
}
