package volcengine

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"github.com/lipipi/aurora/internal/errors"
	"github.com/lipipi/aurora/internal/log"
	"github.com/lipipi/aurora/internal/tts"
)

// Config 火山引擎 TTS V3 双向流式 WebSocket 配置（从 tts.VolcengineConfig 映射）
type Config struct {
	APIKey       string
	ResourceId   string
	Speaker      string
	Format       string
	SampleRate   int
	SpeechRate   int
	LoudnessRate int
	Pitch        int
}

func init() {
	tts.RegisterProvider("volcengine", New)
}

const (
	ttsEndpoint = "wss://openspeech.bytedance.com/api/v3/tts/bidirection"

	// Binary header: 4 bytes big-endian
	// bits[0:4]   = protocol version (1)
	// bits[4:8]   = header size in 4-byte words (1 = 4 bytes)
	// bits[8:16]  = message type (0x01 = FullServerResponse, 0x02 = AudioOnlyServer)
	// bits[16:20] = message type flags
	// bits[20:24] = serialization (0 = raw, 1 = JSON)
	// bits[24:28] = compression
	// bits[28:32] = reserved
	msgTypeFullServerResponse = 0x01
	msgTypeAudioOnlyServer    = 0x02
	headerSizeBytes           = 4
)

// volcengineTTS 火山引擎 TTS V3 Provider（WebSocket 双向流式）
type volcengineTTS struct {
	config Config
	player tts.Player

	mu        sync.Mutex
	playEndCB func() // 播放完成回调
}

// New 创建火山引擎 TTS V3 Provider
func New(cfg any) (tts.TTSProvider, errors.AppError) {
	var config Config
	switch v := cfg.(type) {
	case tts.VolcengineConfig:
		config = Config{
			APIKey:       v.APIKey,
			ResourceId:   nonEmpty(v.ResourceId, "seed-tts-2.0"),
			Speaker:      nonEmpty(v.Speaker, "saturn_zh_female_keainvsheng_tob"),
			Format:       nonEmpty(v.Format, "mp3"),
			SampleRate:   defaultInt(v.SampleRate, 24000),
			SpeechRate:   v.SpeechRate,
			LoudnessRate: v.LoudnessRate,
			Pitch:        v.Pitch,
		}
	case Config:
		config = v
	default:
		return nil, errors.New(errors.CodeInvalidConfig, "invalid config type for volcengine TTS provider")
	}

	player := tts.NewPlatformPlayer()

	v := &volcengineTTS{
		config: config,
		player: player,
	}

	player.SetOnEnd(func() {
		v.onPlaybackEnd()
	})

	log.Debugf("Created volcengine TTS V3 provider, speaker=%s, format=%s, sample_rate=%d",
		config.Speaker, config.Format, config.SampleRate)
	return v, nil
}

func (v *volcengineTTS) Name() string {
	return "volcengine"
}

func (v *volcengineTTS) IsConfigured() bool {
	return v.config.APIKey != ""
}

func (v *volcengineTTS) SetOnPlaybackEnd(cb func()) {
	v.mu.Lock()
	v.playEndCB = cb
	v.mu.Unlock()
}

func (v *volcengineTTS) IsPlaying() bool {
	if v.player == nil {
		return false
	}
	return v.player.IsPlaying()
}

// Synthesize 异步合成并播放
func (v *volcengineTTS) Synthesize(text string) error {
	if !v.IsConfigured() {
		return errors.New(errors.CodeTTSFailed, "volcengine TTS api key not configured")
	}
	if text == "" {
		return nil
	}
	go func() {
		if err := v.synthesizeAndPlay(text); err != nil {
			log.Errorf("TTS synthesize failed: %v", err)
			v.onPlaybackEnd()
		}
	}()
	return nil
}

// synthesizeAndPlay 建立 WebSocket → 发送 StartConnection/StartSession/TaskRequest → 收 audio → 写临时文件 → 播放
func (v *volcengineTTS) synthesizeAndPlay(text string) error {
	// 1. 连接 WebSocket
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}
	headers := http.Header{}
	headers.Set("X-Api-Key", v.config.APIKey)
	headers.Set("X-Api-Resource-Id", v.config.ResourceId)
	headers.Set("X-Api-Connect-Id", uuid.NewString())

	conn, _, err := dialer.Dial(ttsEndpoint, headers)
	if err != nil {
		return fmt.Errorf("websocket dial: %w", err)
	}
	defer conn.Close()

	// 2. StartConnection
	if err := sendEvent(conn, map[string]interface{}{
		"event": EventStartConnection,
	}); err != nil {
		return fmt.Errorf("send StartConnection: %w", err)
	}
	if err := expectEvent(conn, EventConnectionStarted, 5*time.Second); err != nil {
		return fmt.Errorf("wait ConnectionStarted: %w", err)
	}

	// 3. StartSession
	sessionID := uuid.NewString()
	if err := sendEvent(conn, map[string]interface{}{
		"event":      EventStartSession,
		"session_id": sessionID,
		"req_params": map[string]interface{}{
			"model":  "seed-tts-2.0-standard",
			"text":   text, // 部分实现要求 StartSession 就传 text，我们也在此传
			"speaker": v.config.Speaker,
			"audio_params": map[string]interface{}{
				"format":           v.config.Format,
				"sample_rate":      v.config.SampleRate,
				"speech_rate":      v.config.SpeechRate,
				"loudness_rate":    v.config.LoudnessRate,
				"pitch":            v.config.Pitch,
				"enable_subtitle":  false,
				"disable_markdown_filter": true,
			},
		},
	}); err != nil {
		return fmt.Errorf("send StartSession: %w", err)
	}
	if err := expectEvent(conn, EventSessionStarted, 10*time.Second); err != nil {
		return fmt.Errorf("wait SessionStarted: %w", err)
	}

	// 4. TaskRequest（如果 StartSession 已传 text，部分实现可不发；为安全仍发送一次）
	if err := sendEvent(conn, map[string]interface{}{
		"event":      EventTaskRequest,
		"session_id": sessionID,
		"text":       text,
	}); err != nil {
		return fmt.Errorf("send TaskRequest: %w", err)
	}

	// 5. 收 audio frames 直到 SessionFinished
	audioBytes, err := v.collectAudio(conn, 60*time.Second)
	if err != nil {
		return err
	}
	if len(audioBytes) == 0 {
		return errors.New(errors.CodeTTSFailed, "TTS returned no audio data")
	}

	// 6. 关闭会话
	_ = sendEvent(conn, map[string]interface{}{
		"event":      EventFinishSession,
		"session_id": sessionID,
	})
	_ = sendEvent(conn, map[string]interface{}{
		"event": EventFinishConnection,
	})
	_ = conn.WriteMessage(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	_ = conn.Close()

	// 7. 写临时文件 + 播放
	tmpFile, err := writeTempFile(audioBytes, v.config.Format)
	if err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}
	defer os.Remove(tmpFile)

	log.Debugf("TTS synthesized %d chars → %d bytes %s", len(text), len(audioBytes), v.config.Format)
	if err := v.player.Play(tmpFile); err != nil {
		return fmt.Errorf("play: %w", err)
	}
	return nil
}

// collectAudio 持续读取 WebSocket 帧：binary → audio payload；text → 控制事件
// 返回所有 audio 字节，遇到 SessionFinished / SessionFailed / SessionCanceled 后退出
func (v *volcengineTTS) collectAudio(conn *websocket.Conn, timeout time.Duration) ([]byte, error) {
	deadline := time.Now().Add(timeout)
	_ = conn.SetReadDeadline(deadline)

	var audioBuf []byte
	for {
		msgType, data, err := conn.ReadMessage()
		if err != nil {
			return audioBuf, fmt.Errorf("read message: %w", err)
		}
		_ = conn.SetReadDeadline(time.Now().Add(15 * time.Second))

		if msgType == websocket.BinaryMessage {
			// binary 帧：前 4 字节 header，剩下是 audio payload
			if len(data) <= headerSizeBytes {
				continue
			}
			header := binary.BigEndian.Uint32(data[:4])
			msgType2 := byte((header >> 8) & 0xFF)
			if msgType2 == msgTypeAudioOnlyServer {
				audioBuf = append(audioBuf, data[headerSizeBytes:]...)
			} else if msgType2 == msgTypeFullServerResponse {
				// FullServerResponse 走 JSON 路径
				var resp TTSResponse
				if err := json.Unmarshal(data[headerSizeBytes:], &resp); err == nil {
					if isTerminalEvent(resp.Event) {
						return audioBuf, checkTerminalError(resp)
					}
				}
			}
			continue
		}

		// text 帧：JSON 事件
		var resp TTSResponse
		if err := json.Unmarshal(data, &resp); err != nil {
			log.Warnf("TTS: unparseable text frame: %v (data=%s)", err, truncate(string(data), 200))
			continue
		}
		log.Debugf("TTS event: %s", resp.Event)
		if isTerminalEvent(resp.Event) {
			return audioBuf, checkTerminalError(resp)
		}
	}
}

func isTerminalEvent(event string) bool {
	switch event {
	case EventSessionFinished, EventSessionFailed, EventSessionCanceled,
		EventConnectionFailed, EventConnectionFinished:
		return true
	}
	return false
}

func checkTerminalError(resp TTSResponse) error {
	switch resp.Event {
	case EventSessionFailed, EventSessionCanceled, EventConnectionFailed:
		return fmt.Errorf("TTS session error: event=%s, error=%s", resp.Event, resp.ErrorMessage)
	}
	return nil
}

// onPlaybackEnd 播放结束回调
func (v *volcengineTTS) onPlaybackEnd() {
	v.mu.Lock()
	cb := v.playEndCB
	v.mu.Unlock()
	if cb != nil {
		cb()
	}
}

// Stop 停止播放
func (v *volcengineTTS) Stop() {
	if v.player != nil {
		_ = v.player.Stop()
	}
}

// ---- 事件常量 ----

const (
	EventStartConnection   = "StartConnection"
	EventConnectionStarted = "ConnectionStarted"
	EventConnectionFailed  = "ConnectionFailed"
	EventConnectionFinished = "ConnectionFinished"
	EventStartSession      = "StartSession"
	EventSessionStarted    = "SessionStarted"
	EventSessionFinished   = "SessionFinished"
	EventSessionFailed     = "SessionFailed"
	EventSessionCanceled   = "SessionCanceled"
	EventTaskRequest       = "TaskRequest"
	EventFinishSession     = "FinishSession"
	EventFinishConnection  = "FinishConnection"
)

// TTSResponse WebSocket 响应（JSON 形式）
type TTSResponse struct {
	Event        string                 `json:"event"`
	SessionID    string                 `json:"session_id,omitempty"`
	ConnectID    string                 `json:"connect_id,omitempty"`
	ErrorCode    int                    `json:"error_code,omitempty"`
	ErrorMessage string                 `json:"error_message,omitempty"`
	Payload      map[string]interface{} `json:"payload,omitempty"`
	Usage        map[string]interface{} `json:"usage,omitempty"`
}

// sendEvent 发送 JSON 控制事件
func sendEvent(conn *websocket.Conn, payload map[string]interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return conn.WriteMessage(websocket.TextMessage, data)
}

// expectEvent 读直到指定事件出现（用于建连后会话启动）
func expectEvent(conn *websocket.Conn, event string, timeout time.Duration) error {
	_ = conn.SetReadDeadline(time.Now().Add(timeout))
	for {
		msgType, data, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		_ = conn.SetReadDeadline(time.Now().Add(timeout))
		if msgType == websocket.BinaryMessage {
			continue
		}
		var resp TTSResponse
		if err := json.Unmarshal(data, &resp); err != nil {
			continue
		}
		if resp.Event == event {
			return nil
		}
		if isTerminalEvent(resp.Event) && resp.Event != EventConnectionFinished && resp.Event != EventSessionFinished {
			return fmt.Errorf("unexpected terminal event: %s, err=%s", resp.Event, resp.ErrorMessage)
		}
	}
}

// writeTempFile 写入临时音频文件
func writeTempFile(data []byte, format string) (string, error) {
	tmpDir := os.TempDir()
	ext := format
	if ext == "" {
		ext = "mp3"
	}
	tmpFile := filepath.Join(tmpDir, fmt.Sprintf("gameai_tts_%d.%s", time.Now().UnixNano(), ext))
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return "", err
	}
	return tmpFile, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func nonEmpty(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func defaultInt(v, def int) int {
	if v == 0 {
		return def
	}
	return v
}
