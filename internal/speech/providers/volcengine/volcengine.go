package volcengine

import (
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"github.com/lipipi/game-ai/internal/errors"
	"github.com/lipipi/game-ai/internal/log"
	"github.com/lipipi/game-ai/internal/speech"
)

// Config Volcengine ASR 配置（从 speech.VolcengineConfig 映射）
type Config struct {
	APIKey     string `json:"api_key"`
	Endpoint   string `json:"endpoint"`
	ResourceID string `json:"resource_id"`
}

type volcengineRecognizer struct {
	config    Config
	conn      *websocket.Conn
	audioCh   <-chan []byte
	callback  speech.TranscriptCallback
	done      chan struct{}
	sessionID string
	mu        sync.Mutex
	ended     bool
}

func init() {
	speech.RegisterProvider("volcengine", New)
}

// New 创建火山引擎流式 ASR Provider
func New(cfg any) (speech.SpeechRecognizer, errors.AppError) {
	var config Config
	switch v := cfg.(type) {
	case speech.VolcengineConfig:
		config = Config{
			APIKey:     v.APIKey,
			Endpoint:   v.Endpoint,
			ResourceID: v.ResourceID,
		}
	case Config:
		config = v
	default:
		return nil, errors.New(errors.CodeInvalidConfig, "invalid config type for volcengine speech provider")
	}

	if config.Endpoint == "" {
		config.Endpoint = "wss://openspeech.bytedance.com/api/v3/sauc/bigmodel_async"
	}
	if config.ResourceID == "" {
		config.ResourceID = "volc.seedasr.sauc.duration"
	}

	return &volcengineRecognizer{
		config: config,
	}, nil
}

func (v *volcengineRecognizer) Name() string {
	return "volcengine"
}

func (v *volcengineRecognizer) IsConfigured() bool {
	return v.config.APIKey != ""
}

func (v *volcengineRecognizer) StartSession(audioCh <-chan []byte, callback speech.TranscriptCallback) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	if !v.IsConfigured() {
		return errors.New(errors.CodeSpeechFailed, "volcengine ASR API key not configured")
	}

	v.audioCh = audioCh
	v.callback = callback
	v.sessionID = uuid.New().String()
	v.done = make(chan struct{})
	v.ended = false

	// 建立 WebSocket 连接
	if err := v.connect(); err != nil {
		return err
	}

	// 发送初始配置消息
	if err := v.sendFullRequest(); err != nil {
		v.conn.Close()
		return err
	}

	// 读取服务端 ack
	if err := v.readAck(); err != nil {
		v.conn.Close()
		return err
	}

	// 启动发送和接收 goroutine
	go v.sendAudioLoop()
	go v.recvLoop()

	log.Infof("Volcengine ASR session started: %s", v.sessionID)
	return nil
}

func (v *volcengineRecognizer) EndSession() error {
	v.mu.Lock()
	if v.ended {
		v.mu.Unlock()
		return nil
	}
	v.ended = true
	v.mu.Unlock()

	// 通知发送循环结束（通过关闭 done channel）
	// 发送循环会发送最后一个 negative seq 段
	if v.done != nil {
		select {
		case <-v.done:
			// 已经关闭
		default:
			close(v.done)
		}
	}

	// 等待 WebSocket 关闭（recvLoop 会处理）
	// 给一些时间让最终结果返回
	time.Sleep(500 * time.Millisecond)

	v.cleanup()
	return nil
}

func (v *volcengineRecognizer) CancelSession() error {
	v.mu.Lock()
	if v.ended {
		v.mu.Unlock()
		return nil
	}
	v.ended = true
	v.mu.Unlock()

	// 立即关闭
	if v.done != nil {
		select {
		case <-v.done:
		default:
			close(v.done)
		}
	}

	v.cleanup()
	return nil
}

// connect 建立 WebSocket 连接
func (v *volcengineRecognizer) connect() error {
	reqID := uuid.New().String()

	header := http.Header{}
	header.Add("X-Api-Key", v.config.APIKey)
	header.Add("X-Api-Resource-Id", v.config.ResourceID)
	header.Add("X-Api-Request-Id", reqID)

	conn, resp, err := websocket.DefaultDialer.Dial(v.config.Endpoint, header)
	if err != nil {
		if resp != nil {
			// 读取服务端返回的错误信息
			body := make([]byte, 1024)
			n, _ := resp.Body.Read(body)
			respBody := string(body[:n])
			resp.Body.Close()
			log.Errorf("ASR WebSocket handshake failed: status=%d, body=%s, endpoint=%s",
				resp.StatusCode, respBody, v.config.Endpoint)
			return errors.Newf(errors.CodeSpeechFailed,
				"websocket handshake failed: HTTP %d - %s", resp.StatusCode, respBody)
		}
		return errors.Wrap(errors.CodeSpeechFailed, "websocket connect failed", err)
	}

	logID := resp.Header.Get("X-Tt-Logid")
	log.Debugf("Volcengine ASR connected, logid: %s", logID)

	v.conn = conn
	return nil
}

// sendFullRequest 发送初始配置消息
func (v *volcengineRecognizer) sendFullRequest() error {
	data, err := buildFullClientRequest()
	if err != nil {
		return errors.Wrap(errors.CodeSpeechFailed, "build full request", err)
	}

	if err := v.conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
		return errors.Wrap(errors.CodeSpeechFailed, "send full request", err)
	}

	log.Debug("Sent full client request")
	return nil
}

// readAck 读取服务端对初始配置的确认
func (v *volcengineRecognizer) readAck() error {
	_, msg, err := v.conn.ReadMessage()
	if err != nil {
		return errors.Wrap(errors.CodeSpeechFailed, "read ack", err)
	}

	resp, err := parseServerResponse(msg)
	if err != nil {
		return errors.Wrap(errors.CodeSpeechFailed, "parse ack", err)
	}

	if resp.Code != 0 {
		return errors.Newf(errors.CodeSpeechFailed, "server error on ack: code=%d, error=%s",
			resp.Code, resp.PayloadMsg.Error)
	}

	log.Debug("Received server ack")
	return nil
}

// sendAudioLoop 音频发送循环
// 从 audioCh 读取 PCM 数据，每 200ms 发送一个音频段
func (v *volcengineRecognizer) sendAudioLoop() {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("sendAudioLoop panic: %v", r)
		}
	}()

	seq := int32(2) // 从 2 开始（1 是 full request）
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	firstSegment := true // 第一个音频段需要加 WAV 头

	for {
		select {
		case <-v.done:
			// 发送最后一段（negative sequence 表示结束）
			log.Debug("Sending last audio segment (negative seq)")
			// 读取剩余音频数据
			for {
				select {
				case data := <-v.audioCh:
					if v.conn == nil {
						return
					}
					msg, err := buildAudioRequest(seq, data)
					if err != nil {
						log.Errorf("Build last audio request: %v", err)
						return
					}
					if err := v.conn.WriteMessage(websocket.BinaryMessage, msg); err != nil {
						log.Debugf("Send last audio segment: %v", err)
						return
					}
					seq++
				default:
					// 没有更多数据，发送空的最后段
					if v.conn == nil {
						return
					}
					lastMsg, err := buildAudioRequest(-seq, []byte{})
					if err != nil {
						log.Errorf("Build final segment: %v", err)
						return
					}
					if err := v.conn.WriteMessage(websocket.BinaryMessage, lastMsg); err != nil {
						log.Debugf("Send final segment: %v", err)
					}
					return
				}
			}

		case <-ticker.C:
			// 从 audioCh 读取一段音频
			select {
			case data := <-v.audioCh:
				if v.conn == nil {
					return
				}
				// 第一个音频段前面加 WAV 头（服务端需要识别格式）
				if firstSegment {
					wavHeader := buildWavHeader(0) // dataSize=0 表示流式
					data = append(wavHeader, data...)
					firstSegment = false
				}
				log.Debugf("Sending audio segment seq=%d, bytes=%d", seq, len(data))
				msg, err := buildAudioRequest(seq, data)
				if err != nil {
					log.Errorf("Build audio request seq=%d: %v", seq, err)
					return
				}
				if v.conn == nil {
					return
				}
				if err := v.conn.WriteMessage(websocket.BinaryMessage, msg); err != nil {
					log.Debugf("Send audio segment seq=%d: %v", seq, err)
					return
				}
				seq++
			default:
				// 没有音频数据，跳过本周期
			}
		}
	}
}

// recvLoop 接收循环
// 读取服务端响应，解析识别结果
func (v *volcengineRecognizer) recvLoop() {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("recvLoop panic: %v", r)
		}
		v.cleanup()
	}()

	for {
		_, msg, err := v.conn.ReadMessage()
		if err != nil {
			// 连接关闭或被取消
			select {
			case <-v.done:
				// 正常结束
			default:
				log.Debugf("WebSocket read error: %v", err)
			}
			return
		}

		resp, err := parseServerResponse(msg)
		if err != nil {
			log.Errorf("Parse server response: %v, raw_len=%d, first_bytes=%x", err, len(msg), safeBytes(msg, 20))
			continue
		}

		// 调试日志：显示服务端返回内容
		if resp.PayloadMsg != nil {
			log.Debugf("ASR response: code=%d, isLast=%v, text=%q, utterances=%d",
				resp.Code, resp.IsLastPackage, resp.PayloadMsg.Result.Text,
				len(resp.PayloadMsg.Result.Utterances))
		} else {
			log.Debugf("ASR response: code=%d, isLast=%v, no payload, raw=%x",
				resp.Code, resp.IsLastPackage, safeBytes(msg, 40))
		}

		// 错误响应
		if resp.Code != 0 {
			log.Errorf("ASR server error: code=%d, raw_msg_len=%d", resp.Code, len(msg))
			if resp.PayloadMsg != nil && resp.PayloadMsg.Error != "" {
				log.Errorf("ASR error detail: %s", resp.PayloadMsg.Error)
			}
			if v.callback != nil {
				v.callback(speech.Transcript{
					Text:      "",
					IsFinal:   true,
					SessionID: v.sessionID,
				})
			}
			return
		}

		// 提取识别结果
		if resp.PayloadMsg != nil && v.callback != nil {
			result := resp.PayloadMsg.Result

			// 判断是否为最终结果
			isFinal := resp.IsLastPackage
			if !isFinal && len(result.Utterances) > 0 {
				// 检查最后一个 utterance 是否 definite
				isFinal = result.Utterances[len(result.Utterances)-1].Definite
			}

			v.callback(speech.Transcript{
				Text:      result.Text,
				IsFinal:   isFinal,
				SessionID: v.sessionID,
			})
		}

		// 最后一个包，结束
		if resp.IsLastPackage {
			log.Debug("Received last package, session complete")
			return
		}
	}
}

// safeBytes 安全截取字节切片的前 n 个字节（防止越界）
func safeBytes(data []byte, n int) []byte {
	if len(data) <= n {
		return data
	}
	return data[:n]
}

// cleanup 清理资源
func (v *volcengineRecognizer) cleanup() {
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.conn != nil {
		v.conn.Close()
		v.conn = nil
	}
}
