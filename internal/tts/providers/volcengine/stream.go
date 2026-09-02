package volcengine

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"github.com/lipipi/aurora/internal/audio"
	"github.com/lipipi/aurora/internal/errors"
	"github.com/lipipi/aurora/internal/log"
	"github.com/lipipi/aurora/internal/tts"
)

// synthesizeAndStream 流式合成并播放（PCM streaming path）
//
// Phase 19: 使用 WebSocket 流式接收 PCM 数据，通过 io.Pipe 直接传给 Oto 播放器
// 不等待完整音频，不创建临时文件
func (v *volcengineTTS) synthesizeAndStream(text string) error {
	startTime := time.Now()
	log.Debugf("TTS streaming request start")

	// 创建 io.Pipe 连接 WebSocket 和播放器
	pr, pw := io.Pipe()

	// 创建音频流
	stream := &audio.Stream{
		Reader: pr,
		Format: audio.PCM16Mono24kHz,
		Close: func() {
			pw.Close()
		},
	}

	// 启动 WebSocket goroutine（写入 PCM 到 PipeWriter）
	wsErrCh := make(chan error, 1)
	go func() {
		defer pw.Close()
		if err := v.streamAudioFromWebSocket(text, pw, startTime); err != nil {
			wsErrCh <- err
		}
	}()

	// 启动播放器（从 PipeReader 读取）
	if v.playbackService == nil {
		return errors.New(errors.CodeTTSFailed, "playback service not initialized")
	}

	playbackErrCh := make(chan error, 1)
	go func() {
		err := v.playbackService.PlayStream(context.Background(), stream, func() {
			// 播放结束回调
			log.Debugf("TTS playback ended: %v", time.Since(startTime))
			v.onPlaybackEnd()
		})
		if err != nil {
			playbackErrCh <- err
		}
	}()

	// 等待 WebSocket 或播放器报错
	select {
	case err := <-wsErrCh:
		return fmt.Errorf("websocket streaming failed: %w", err)
	case err := <-playbackErrCh:
		return fmt.Errorf("playback failed: %w", err)
	default:
		// 两个 goroutine 都在正常运行
		return nil
	}
}

// streamAudioFromWebSocket 从 WebSocket 流式接收音频数据并写入 writer
func (v *volcengineTTS) streamAudioFromWebSocket(text string, writer io.Writer, startTime time.Time) error {
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

	// 1. StartConnection
	if err := sendEventConnLevel(conn, evStartConnection, []byte(`{}`)); err != nil {
		return fmt.Errorf("send StartConnection: %w", err)
	}
	if err := waitForResponse(conn, evConnectionStarted, 10*time.Second); err != nil {
		return fmt.Errorf("wait ConnectionStarted: %w", err)
	}

	// 2. StartSession
	sessionID := uuid.NewString()
	sessionPayload, _ := json.Marshal(map[string]interface{}{
		"event": evStartSession,
		"req_params": map[string]interface{}{
			"speaker":      v.config.Speaker,
			"audio_params": v.buildAudioParamsForStreaming(),
		},
	})
	if err := sendEventSession(conn, evStartSession, sessionID, sessionPayload); err != nil {
		return fmt.Errorf("send StartSession: %w", err)
	}
	if err := waitForResponse(conn, evSessionStarted, 10*time.Second); err != nil {
		return fmt.Errorf("wait SessionStarted: %w", err)
	}

	// 3. TaskRequest
	taskPayload, _ := json.Marshal(map[string]interface{}{
		"event": evTaskRequest,
		"req_params": map[string]interface{}{
			"speaker":      v.config.Speaker,
			"audio_params": v.buildAudioParamsForStreaming(),
			"text":         text,
		},
	})
	if err := sendEventSession(conn, evTaskRequest, sessionID, taskPayload); err != nil {
		return fmt.Errorf("send TaskRequest: %w", err)
	}

	// 4. FinishSession
	finishPayload, _ := json.Marshal(map[string]interface{}{
		"event": evFinishSession,
	})
	if err := sendEventSession(conn, evFinishSession, sessionID, finishPayload); err != nil {
		return fmt.Errorf("send FinishSession: %w", err)
	}

	// 5. 流式接收音频数据，直接写入 writer（不累积）
	firstChunkTime := time.Time{}
	totalBytes := 0
	for {
		mt, data, err := conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("read message: %w", err)
		}
		_ = conn.SetReadDeadline(time.Now().Add(15 * time.Second))
		if mt != websocket.BinaryMessage {
			continue
		}

		f, _ := parseFrame(data)
		switch f.msgType {
		case msgTypeAudioOnlyServer:
			// 首个音频块到达
			if firstChunkTime.IsZero() {
				firstChunkTime = time.Now()
				log.Debugf("TTS first audio chunk: %v", firstChunkTime.Sub(startTime))
			}
			// 直接写入 pipe，不累积
			if _, err := writer.Write(f.payload); err != nil {
				return fmt.Errorf("write to pipe: %w", err)
			}
			totalBytes += len(f.payload)

		case msgTypeFullServerResponse:
			switch f.event {
			case evSessionFinished:
				log.Debugf("TTS WebSocket complete: %d bytes in %v",
					totalBytes, time.Since(startTime))
				// 优雅关闭
				_ = sendEventConnLevel(conn, evFinishConnection, []byte(`{}`))
				_ = conn.WriteMessage(websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
				return nil
			case evSessionFailed:
				return fmt.Errorf("TTS SessionFailed: %s", string(f.payload))
			}

		case msgTypeServerError:
			return fmt.Errorf("TTS server error: %s", string(f.payload))
		}
	}
}

// buildAudioParamsForStreaming 构造流式播放的 audio_params（PCM 格式）
func (v *volcengineTTS) buildAudioParamsForStreaming() map[string]interface{} {
	params := map[string]interface{}{
		"format":      "pcm",
		"sample_rate": 24000,
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

// 确保 volcengineTTS 实现了 TTSProvider 接口
var _ tts.TTSProvider = (*volcengineTTS)(nil)
