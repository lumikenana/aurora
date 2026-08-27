package volcengine

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
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

	// 4-byte binary header (big-endian)
	//   byte 0: (version << 4) | headerSize  — version=1, headerSize=1 (=4 bytes)
	//   byte 1: (msgType << 4) | flags
	//   byte 2: (serialization << 4) | compression
	//   byte 3: reserved (= 0)
	msgTypeFullClientRequest  = 0b0001
	msgTypeAudioOnlyClient    = 0b0010
	msgTypeFullServerResponse = 0b1001
	msgTypeAudioOnlyServer    = 0b1011
	msgTypeServerError        = 0b1111

	flagNoSeq     = 0b0000 // No sequence, no event
	flagWithEvent = 0b0100 // WithEvent: 4B event int32 BE follows the header (used for control frames)

	serializationJSON = 0b0001
	compressionNone   = 0b0000
)

// Event 枚举（按 V3 协议约定）
const (
	evStartConnection   int32 = 1
	evFinishConnection  int32 = 2
	evConnectionStarted int32 = 50
	evConnectionFailed  int32 = 51
	evStartSession      int32 = 100
	evFinishSession     int32 = 102
	evSessionStarted    int32 = 150
	evSessionFinished   int32 = 152
	evSessionFailed     int32 = 153
	evTaskRequest       int32 = 200
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
			Speaker:      nonEmpty(v.Speaker, "zh_female_xiaohe_uranus_bigtts"),
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

// synthesizeAndPlay 建立 WebSocket → StartConnection → StartSession → TaskRequest → 收 audio → 关闭 → 播放
func (v *volcengineTTS) synthesizeAndPlay(text string) error {
	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
	headers := http.Header{}
	headers.Set("X-Api-Key", v.config.APIKey)
	headers.Set("X-Api-Resource-Id", v.config.ResourceId)
	headers.Set("X-Api-Connect-Id", uuid.NewString())

	conn, resp, err := dialer.Dial(ttsEndpoint, headers)
	if err != nil {
		status := -1
		bodySnippet := ""
		if resp != nil {
			status = resp.StatusCode
			bodyBytes, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			bodySnippet = truncate(string(bodyBytes), 300)
		}
		return fmt.Errorf("websocket dial: %w (status=%d, body=%q)", err, status, bodySnippet)
	}
	defer conn.Close()

	// 1. StartConnection（event=1，无 session_id）
	if err := sendEventConnLevel(conn, evStartConnection, []byte(`{}`)); err != nil {
		return fmt.Errorf("send StartConnection: %w", err)
	}
	if err := waitForResponse(conn, evConnectionStarted, 10*time.Second); err != nil {
		return fmt.Errorf("wait ConnectionStarted: %w", err)
	}

	// 2. StartSession（event=100，带 session_id，载荷结构按官方样例：
	//    {"event":100,"req_params":{"speaker":...,"audio_params":{...}}}）
	sessionID := uuid.NewString()
	sessionPayload, _ := json.Marshal(map[string]interface{}{
		"event": evStartSession,
		"req_params": map[string]interface{}{
			"speaker":      v.config.Speaker,
			"audio_params": v.buildAudioParams(),
		},
	})
	if err := sendEventSession(conn, evStartSession, sessionID, sessionPayload); err != nil {
		return fmt.Errorf("send StartSession: %w", err)
	}
	if err := waitForResponse(conn, evSessionStarted, 10*time.Second); err != nil {
		return fmt.Errorf("wait SessionStarted: %w", err)
	}

	// 3. TaskRequest（event=200，带 session_id；官方样例每次请求都带完整
	//    req_params（含 speaker）+ text，否则服务端可能落到与资源不匹配的
	//    默认音色，报 "resource ID is mismatched with speaker related resource"）
	taskPayload, _ := json.Marshal(map[string]interface{}{
		"event": evTaskRequest,
		"req_params": map[string]interface{}{
			"speaker":      v.config.Speaker,
			"audio_params": v.buildAudioParams(),
			"text":         text,
		},
	})
	if err := sendEventSession(conn, evTaskRequest, sessionID, taskPayload); err != nil {
		return fmt.Errorf("send TaskRequest: %w", err)
	}

	// 4. FinishSession：告知服务端文本发送完毕。
	//    服务端收到 FinishSession 后才合成剩余音频并回 SessionFinished；
	//    若不先发，collectAudio 会一直等不到结束帧而 i/o timeout
	//    （官方样例同样是发完文本 -> finish_session -> 再收音频）
	finishPayload, _ := json.Marshal(map[string]interface{}{
		"event": evFinishSession,
	})
	if err := sendEventSession(conn, evFinishSession, sessionID, finishPayload); err != nil {
		return fmt.Errorf("send FinishSession: %w", err)
	}

	// 5. 收集音频，直到 SessionFinished/SessionFailed
	audioBytes, err := v.collectAudio(conn, 60*time.Second)
	if err != nil {
		return err
	}
	if len(audioBytes) == 0 {
		return errors.New(errors.CodeTTSFailed, "TTS returned no audio data")
	}

	// 6. 优雅关闭
	_ = sendEventConnLevel(conn, evFinishConnection, []byte(`{}`))
	_ = conn.WriteMessage(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))

	// 6. 写临时文件 + 播放
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

// buildAudioParams 构造 audio_params（官方 V3 样例仅含 format/sample_rate；
// speech_rate/loudness_rate/pitch 属 V1 参数，仅在用户显式设置时携带）
func (v *volcengineTTS) buildAudioParams() map[string]interface{} {
	params := map[string]interface{}{
		"format":      v.config.Format,
		"sample_rate": v.config.SampleRate,
	}
	if v.config.SpeechRate != 0 {
		params["speech_rate"] = v.config.SpeechRate
	}
	if v.config.LoudnessRate != 0 {
		params["loudness_rate"] = v.config.LoudnessRate
	}
	if v.config.Pitch != 0 {
		params["pitch"] = v.config.Pitch
	}
	return params
}

// sendEventConnLevel 发送连接级事件帧（StartConnection/FinishConnection）：无 session_id
// Frame: [4B header][4B event int32 BE][4B payload size][payload]
func sendEventConnLevel(conn *websocket.Conn, event int32, payload []byte) error {
	var buf bytes.Buffer
	buf.Write(makeHeader(msgTypeFullClientRequest, flagWithEvent))
	binary.Write(&buf, binary.BigEndian, event)
	binary.Write(&buf, binary.BigEndian, uint32(len(payload)))
	buf.Write(payload)
	return conn.WriteMessage(websocket.BinaryMessage, buf.Bytes())
}

// sendEventSession 发送会话级事件帧（StartSession/TaskRequest/FinishSession）：带 session_id
// Frame: [4B header][4B event int32 BE][4B session_id size][session_id][4B payload size][payload]
func sendEventSession(conn *websocket.Conn, event int32, sessionID string, payload []byte) error {
	var buf bytes.Buffer
	buf.Write(makeHeader(msgTypeFullClientRequest, flagWithEvent))
	binary.Write(&buf, binary.BigEndian, event)
	sid := []byte(sessionID)
	binary.Write(&buf, binary.BigEndian, uint32(len(sid)))
	buf.Write(sid)
	binary.Write(&buf, binary.BigEndian, uint32(len(payload)))
	buf.Write(payload)
	return conn.WriteMessage(websocket.BinaryMessage, buf.Bytes())
}

// makeHeader 生成 4 字节二进制头：version=1, headerSize=1, msgType + flag, serialization + compression
func makeHeader(msgType, flags byte) []byte {
	return []byte{
		(1 << 4) | 1, // version=1, headerSize=1 (4 bytes total)
		(msgType << 4) | flags,
		(serializationJSON << 4) | compressionNone,
		0x00, // reserved
	}
}

// serverFrame 解析后的服务端帧
type serverFrame struct {
	msgType byte
	flags   byte
	event   int32
	payload []byte
}

// parseFrame 解析服务端 binary 帧
// 实测（抓包验证）FullServerResponse / AudioOnlyServer 与客户端帧结构镜像：
//   [4B hdr][4B event][4B sidLen][sid][4B payloadLen][payload]
//   （音频帧 event=352，TaskRequestAck=350，FinishSessionAck=351）
// ServerError: [4B hdr][4B errorCode][4B payloadLen][payload]（无 sid）
func parseFrame(data []byte) (*serverFrame, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("server frame too short: %d bytes", len(data))
	}
	f := &serverFrame{
		msgType: data[1] >> 4,
		flags:   data[1] & 0x0f,
	}
	pos := 4
	if f.msgType == msgTypeServerError {
		// Error: [4B errorCode][4B payloadLen][payload]
		if len(data) < pos+8 {
			return f, nil
		}
		pos += 4 // skip errorCode
	} else {
		// FullServerResponse / AudioOnlyServer: [4B event][4B sidLen][sid]...
		if len(data) < pos+4 {
			return f, nil
		}
		f.event = int32(binary.BigEndian.Uint32(data[pos:]))
		pos += 4
		if len(data) < pos+4 {
			return f, nil
		}
		sidLen := int(binary.BigEndian.Uint32(data[pos:]))
		pos += 4
		if sidLen > 0 && sidLen <= len(data)-pos {
			pos += sidLen // 跳过 session_id
		}
	}
	if len(data) < pos+4 {
		return f, nil
	}
	plSize := int(binary.BigEndian.Uint32(data[pos:]))
	pos += 4
	if plSize > 0 && plSize <= len(data)-pos {
		f.payload = data[pos : pos+plSize]
	}
	return f, nil
}

// waitForResponse 循环读帧直到出现指定 event
func waitForResponse(conn *websocket.Conn, wantEvent int32, timeout time.Duration) error {
	_ = conn.SetReadDeadline(time.Now().Add(timeout))
	for {
		mt, data, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		_ = conn.SetReadDeadline(time.Now().Add(timeout))
		if mt != websocket.BinaryMessage {
			continue
		}
		f, _ := parseFrame(data)
		if f.msgType == msgTypeServerError {
			return fmt.Errorf("server error: %s", string(f.payload))
		}
		if f.event == wantEvent {
			return nil
		}
		// 其他事件（如 StartSession 中间状态）忽略
	}
}

// collectAudio 持续读取 WebSocket 帧直到 SessionFinished / SessionFailed
// 累积 AudioOnlyServer 帧的 payload 字节
func (v *volcengineTTS) collectAudio(conn *websocket.Conn, timeout time.Duration) ([]byte, error) {
	_ = conn.SetReadDeadline(time.Now().Add(timeout))
	var audioBuf []byte
	for {
		mt, data, err := conn.ReadMessage()
		if err != nil {
			return audioBuf, fmt.Errorf("read message: %w", err)
		}
		_ = conn.SetReadDeadline(time.Now().Add(15 * time.Second))
		if mt != websocket.BinaryMessage {
			continue
		}
		f, _ := parseFrame(data)
		switch f.msgType {
		case msgTypeAudioOnlyServer:
			audioBuf = append(audioBuf, f.payload...)
		case msgTypeFullServerResponse:
			switch f.event {
			case evSessionFinished:
				return audioBuf, nil
			case evSessionFailed:
				return audioBuf, fmt.Errorf("TTS SessionFailed: %s", string(f.payload))
			}
		case msgTypeServerError:
			return audioBuf, fmt.Errorf("TTS server error: %s", string(f.payload))
		}
	}
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
