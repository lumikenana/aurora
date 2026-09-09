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

// AudioDevice 音频设备信息
//
// ID 当前使用 WinMM 设备名称（szPname）作为匹配标识。
// 注意：设备名称不保证唯一（同名设备可能共存）。
// 未来迁移 WASAPI 后，ID 可升级为 Endpoint ID（GUID 字符串），
// 配置格式（string）保持不变，解析逻辑按值格式自动区分。
type AudioDevice struct {
	ID    string `json:"id"`    // 匹配标识（当前 = 设备名称）
	Name  string `json:"name"`  // 显示名称（当前 == ID）
	Index int    `json:"-"`     // 运行时索引，仅用于 waveInOpen/waveOutOpen，不持久化
}

// DeviceConfig 音频设备配置（持久化到 config.json）
//
// InputDeviceID / OutputDeviceID:
//   - "" 或 "default" → 跟随 Windows 系统默认设备（WAVE_MAPPER）
//   - 其他字符串 → 设备名称精确匹配
//   - 匹配失败 → log 警告 + fallback WAVE_MAPPER
type DeviceConfig struct {
	InputDeviceID  string `json:"input_device_id"`
	OutputDeviceID string `json:"output_device_id"`
}

// DefaultDeviceConfig 返回默认设备配置（跟随系统默认）
func DefaultDeviceConfig() DeviceConfig {
	return DeviceConfig{
		InputDeviceID:  "default",
		OutputDeviceID: "default",
	}
}

// Config 音频采集模块配置
type Config struct {
	Devices DeviceConfig `json:"devices"`
	Format  AudioFormat  `json:"format"`
}

// DefaultConfig 返回默认音频配置
func DefaultConfig() Config {
	return Config{
		Devices: DefaultDeviceConfig(),
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
