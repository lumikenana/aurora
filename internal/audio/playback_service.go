package audio

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/lipipi/aurora/internal/log"
)

// PlaybackService 音频播放服务
//
// 负责选择播放器并播放音频流。
// 支持 Oto PCM streaming（优先）和 MCI file playback（fallback）。
type PlaybackService struct {
	otoPlayer *OtoPlayer
	mciPlayer MCIPlayer // Windows only, interface for testability
}

// MCIPlayer MCI 播放器接口（用于 fallback）
type MCIPlayer interface {
	Play(filePath string) error
	Stop() error
	SetOnEnd(cb func())
}

// NewPlaybackService 创建播放服务
//
// 尝试初始化 Oto Player。失败则使用 MCI fallback。
func NewPlaybackService(mciPlayer MCIPlayer) *PlaybackService {
	service := &PlaybackService{
		mciPlayer: mciPlayer,
	}

	// 尝试创建 Oto Player
	oto, err := NewOtoPlayer()
	if err != nil {
		log.Warnf("Oto player unavailable, will use MCI fallback: %v", err)
	} else {
		service.otoPlayer = oto
		log.Debugf("PlaybackService: Oto PCM streaming available")
	}

	return service
}

// PlayStream 播放音频流
//
// 优先使用 Oto PCM streaming。如果 Oto 不可用（初始化失败），
// 则 fallback 到 MCI file playback。
//
// 注意：如果 Oto 已经开始消费 stream 后发生 playback error，
// 不会尝试 fallback（stream 已被消费），直接结束 session 并报告 error。
func (s *PlaybackService) PlayStream(ctx context.Context, stream *Stream, onEnd func()) error {
	// 检查 service 是否为 nil
	if s == nil {
		return fmt.Errorf("playback service is nil")
	}

	// 优先尝试 Oto PCM streaming
	if s.otoPlayer != nil {
		_, err := s.otoPlayer.PlayStream(ctx, stream, onEnd)
		if err == nil {
			return nil
		}
		// PlayStream 返回 error 说明格式不支持或其他初始化问题
		// 此时 stream 尚未被消费，可以尝试 fallback
		log.Warnf("Oto PlayStream failed, falling back to MCI: %v", err)
	}

	// Fallback: 收集完整音频，写入临时文件，MCI 播放
	return s.fallbackToMCI(ctx, stream, onEnd)
}

// Stop 停止当前播放
func (s *PlaybackService) Stop() {
	if s.otoPlayer != nil {
		s.otoPlayer.Stop()
	}
	if s.mciPlayer != nil {
		s.mciPlayer.Stop()
	}
}

// Close 释放资源
func (s *PlaybackService) Close() error {
	if s.otoPlayer != nil {
		return s.otoPlayer.Close()
	}
	return nil
}

// fallbackToMCI MCI fallback 实现
//
// 收集完整 PCM 数据，转换为 WAV，写入临时文件，MCI 播放。
// 注意：这会等待完整音频，失去流式播放的低延迟优势。
func (s *PlaybackService) fallbackToMCI(ctx context.Context, stream *Stream, onEnd func()) error {
	if s.mciPlayer == nil {
		return fmt.Errorf("no player available (Oto unavailable, MCI not configured)")
	}

	startTime := time.Now()
	log.Debugf("TTS fallback to MCI: collecting audio")

	// 收集所有 PCM 数据
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, stream.Reader); err != nil {
		if stream.Close != nil {
			stream.Close()
		}
		return fmt.Errorf("collect PCM data: %w", err)
	}

	// 清理 stream
	if stream.Close != nil {
		stream.Close()
	}

	log.Debugf("TTS fallback: collected %d bytes PCM in %v",
		buf.Len(), time.Since(startTime))

	// 转换为 WAV
	wavData, err := pcmToWAV(buf.Bytes(), stream.Format)
	if err != nil {
		return fmt.Errorf("convert PCM to WAV: %w", err)
	}

	// 写入临时文件
	tmpFile, err := writeTempWAV(wavData)
	if err != nil {
		return fmt.Errorf("write temp WAV: %w", err)
	}

	// 设置 MCI OnEnd 回调（包含清理临时文件）
	s.mciPlayer.SetOnEnd(func() {
		// 删除临时文件
		if err := os.Remove(tmpFile); err != nil {
			log.Debugf("Temp WAV cleanup: %v", err)
		}
		// 调用原始 onEnd
		if onEnd != nil {
			onEnd()
		}
	})

	// MCI 播放
	log.Debugf("TTS fallback: playing WAV via MCI")
	if err := s.mciPlayer.Play(tmpFile); err != nil {
		// 播放失败，清理临时文件
		os.Remove(tmpFile)
		return fmt.Errorf("MCI play: %w", err)
	}

	return nil
}

// pcmToWAV 将 PCM 数据转换为 WAV 格式
func pcmToWAV(pcm []byte, format Format) ([]byte, error) {
	var buf bytes.Buffer

	// RIFF header
	buf.WriteString("RIFF")
	binary.Write(&buf, binary.LittleEndian, uint32(36+len(pcm)))
	buf.WriteString("WAVE")

	// fmt chunk
	buf.WriteString("fmt ")
	binary.Write(&buf, binary.LittleEndian, uint32(16)) // chunk size
	binary.Write(&buf, binary.LittleEndian, uint16(1))  // PCM format
	binary.Write(&buf, binary.LittleEndian, uint16(format.Channels))
	binary.Write(&buf, binary.LittleEndian, uint32(format.SampleRate))
	bytesPerSec := format.SampleRate * format.Channels * format.BytesPerSample()
	binary.Write(&buf, binary.LittleEndian, uint32(bytesPerSec))
	binary.Write(&buf, binary.LittleEndian, uint16(format.Channels*format.BytesPerSample()))
	binary.Write(&buf, binary.LittleEndian, uint16(format.BytesPerSample()*8))

	// data chunk
	buf.WriteString("data")
	binary.Write(&buf, binary.LittleEndian, uint32(len(pcm)))
	buf.Write(pcm)

	return buf.Bytes(), nil
}

// writeTempWAV 写入临时 WAV 文件
func writeTempWAV(wavData []byte) (string, error) {
	tmpDir := os.TempDir()
	tmpFile := filepath.Join(tmpDir, fmt.Sprintf("aurora_tts_fallback_%d.wav", time.Now().UnixNano()))

	if err := os.WriteFile(tmpFile, wavData, 0644); err != nil {
		return "", err
	}

	return tmpFile, nil
}
