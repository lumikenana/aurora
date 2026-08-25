// Package audio 定义音频采集 Provider 接口
package audio

// AudioCallback 音频数据回调
// data: PCM 16-bit signed, 16kHz, mono, raw (no header)
type AudioCallback func(data []byte)

// AudioFormat 音频格式配置
type AudioFormat struct {
	SampleRate    int `json:"sample_rate"`     // 采样率 (16000)
	BitsPerSample int `json:"bits_per_sample"` // 位深 (16)
	Channels      int `json:"channels"`        // 声道数 (1)
}

// Config 音频采集模块配置
type Config struct {
	DeviceIndex int         `json:"device_index"` // 设备索引，-1 = 默认设备
	Format      AudioFormat `json:"format"`
}

// DefaultConfig 返回默认音频配置
func DefaultConfig() Config {
	return Config{
		DeviceIndex: -1,
		Format: AudioFormat{
			SampleRate:    16000,
			BitsPerSample: 16,
			Channels:      1,
		},
	}
}

// AudioCapture 音频采集接口
// ASR/Agent 依赖此接口，不依赖具体平台实现
type AudioCapture interface {
	// Start 开始录音，音频数据通过回调返回
	// callback 每收到一段 PCM 数据调用一次
	Start(callback AudioCallback) error
	// Stop 停止录音，释放麦克风
	Stop()
	// IsRecording 是否正在录音
	IsRecording() bool
	// Name 返回 Provider 名称
	Name() string
	// IsAvailable 检查是否可用
	IsAvailable() bool
}
