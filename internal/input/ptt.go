package input

import (
	"sync"

	"github.com/lipipi/game-ai/internal/audio"
	"github.com/lipipi/game-ai/internal/log"
	"github.com/lipipi/game-ai/internal/speech"
)

// PTTState PTT 状态
type PTTState int

const (
	// PTTIdle 空闲
	PTTIdle PTTState = iota
	// PTTRecording 录音中
	PTTRecording
)

// PTTManager Push-To-Talk 管理器
// 协调: 热键检测 → 音频采集 → ASR 识别 → 输入提交
type PTTManager struct {
	state      PTTState
	audioCap   audio.AudioCapture
	recognizer speech.SpeechRecognizer
	audioCh    chan []byte
	done       chan struct{}
	mu         sync.Mutex

	// Callbacks
	onStateChange   func(state PTTState)
	onTranscript    func(speech.Transcript)
	onPreStart      []func()  // Called before PTT starts, for preemption
	onFinish        []func()  // Called after PTT finishes
}

var pttManager *PTTManager

// InitPTT 初始化 PTT 管理器
func InitPTT(audioCap audio.AudioCapture, recognizer speech.SpeechRecognizer) {
	pttManager = &PTTManager{
		state:      PTTIdle,
		audioCap:   audioCap,
		recognizer: recognizer,
	}
	log.Info("PTT manager initialized")
}

// GetPTT 获取 PTT 管理器
func GetPTT() *PTTManager {
	return pttManager
}

// StartRecording 开始录音（PTT 按下时调用）
func (m *PTTManager) StartRecording() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.state != PTTIdle {
		// 已在录音中，静默忽略（热键可能重复触发）
		return nil
	}

	if m.recognizer == nil || !m.recognizer.IsConfigured() {
		log.Warn("PTT: speech recognizer not configured")
		return nil
	}

	if m.audioCap == nil || !m.audioCap.IsAvailable() {
		log.Warn("PTT: audio capture not available")
		return nil
	}

	// Trigger pre-start callbacks (wake word preemption)
	for _, cb := range m.onPreStart {
		cb()
	}

	log.Info("PTT: starting recording")

	// 1. 创建音频通道
	m.audioCh = make(chan []byte, 16)
	m.done = make(chan struct{})

	// 2. 启动 ASR 会话
	err := m.recognizer.StartSession(m.audioCh, m.handleTranscript)
	if err != nil {
		log.Errorf("PTT: failed to start ASR session: %v", err)
		close(m.done)
		return err
	}

	// 3. 启动音频采集
	err = m.audioCap.Start(func(data []byte) {
		select {
		case m.audioCh <- data:
		default:
			// 丢弃帧（ASR 跟不上时保护，避免阻塞 waveIn 回调线程）
		}
	})
	if err != nil {
		log.Errorf("PTT: failed to start audio capture: %v", err)
		m.recognizer.CancelSession()
		close(m.done)
		return err
	}

	m.state = PTTRecording
	if m.onStateChange != nil {
		m.onStateChange(PTTRecording)
	}

	// 4. 启动按键释放轮询
	go m.pollKeyRelease()

	return nil
}

// StopRecording 停止录音（PTT 释放时调用）
func (m *PTTManager) StopRecording() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.state != PTTRecording {
		return
	}

	log.Info("PTT: stopping recording")

	// 1. 停止音频采集
	m.audioCap.Stop()

	// 2. 结束 ASR 会话（等待最终结果）
	m.recognizer.EndSession()

	// 3. 停止轮询
	if m.done != nil {
		select {
		case <-m.done:
		default:
			close(m.done)
		}
	}

	m.state = PTTIdle
	if m.onStateChange != nil {
		m.onStateChange(PTTIdle)
	}
	// Trigger finish callbacks
	for _, cb := range m.onFinish {
		cb()
	}
}

// CancelRecording 取消录音（错误/退出时调用）
func (m *PTTManager) CancelRecording() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.state != PTTRecording {
		return
	}

	log.Info("PTT: cancelling recording")

	m.audioCap.Stop()
	m.recognizer.CancelSession()

	if m.done != nil {
		select {
		case <-m.done:
		default:
			close(m.done)
		}
	}

	m.state = PTTIdle
	if m.onStateChange != nil {
		m.onStateChange(PTTIdle)
	}
	// Trigger finish callbacks
	for _, cb := range m.onFinish {
		cb()
	}
}

// handleTranscript 处理 ASR 回调
func (m *PTTManager) handleTranscript(t speech.Transcript) {
	// 转发到注册的回调（用于前端显示）
	if m.onTranscript != nil {
		m.onTranscript(t)
	}

	// 如果是最终结果，提交为 UserInput
	if t.IsFinal && t.Text != "" {
		log.Infof("PTT: final transcript: %s", t.Text)
		input := UserInput{
			Text:   t.Text,
			Source: SourceVoice,
		}
		Current().NotifyInput(input)
	}
}

// IsRecording 是否正在录音
func (m *PTTManager) IsRecording() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state == PTTRecording
}

// RegisterStateChangeCallback 注册状态变化回调
func (m *PTTManager) RegisterStateChangeCallback(cb func(PTTState)) {
	m.onStateChange = cb
}

// RegisterTranscriptCallback 注册识别结果回调
func (m *PTTManager) RegisterTranscriptCallback(cb func(speech.Transcript)) {
	m.onTranscript = cb
}

// RegisterPreStartCallback 注册 PTT 开始前的回调
// Used for wake word to preempt when PTT is activated
func (m *PTTManager) RegisterPreStartCallback(cb func()) {
	m.onPreStart = append(m.onPreStart, cb)
}

// RegisterFinishCallback 注册 PTT 完成后的回调
// Used for wake word to resume listening after PTT
func (m *PTTManager) RegisterFinishCallback(cb func()) {
	m.onFinish = append(m.onFinish, cb)
}
