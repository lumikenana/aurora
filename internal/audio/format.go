package audio

// SampleFormat 音频采样格式
type SampleFormat int

const (
	SampleFormatInt16LE SampleFormat = iota // 有符号 16-bit 小端序 PCM
	SampleFormatFloat32LE                   // 32-bit 浮点小端序
	SampleFormatUInt8                       // 无符号 8-bit
)

// Format 音频格式定义
type Format struct {
	SampleRate   int          // 采样率 (Hz)
	Channels     int          // 声道数 (1=mono, 2=stereo)
	SampleFormat SampleFormat // 采样格式
}

// PCM16Mono24kHz Volcengine TTS 默认 PCM 格式
// 24000 Hz, mono, signed 16-bit little endian
var PCM16Mono24kHz = Format{
	SampleRate:   24000,
	Channels:     1,
	SampleFormat: SampleFormatInt16LE,
}

// BytesPerSample 每个采样的字节数
func (f Format) BytesPerSample() int {
	switch f.SampleFormat {
	case SampleFormatInt16LE:
		return 2
	case SampleFormatFloat32LE:
		return 4
	case SampleFormatUInt8:
		return 1
	default:
		return 0
	}
}

// BytesPerFrame 每帧的字节数（所有声道）
func (f Format) BytesPerFrame() int {
	return f.BytesPerSample() * f.Channels
}
