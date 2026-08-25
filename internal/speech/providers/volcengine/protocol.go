// Package volcengine 火山引擎流式语音识别 2.0 实现
// 协议参考: sauc_go SDK (Volcengine official demo)
package volcengine

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
)

// 协议常量
const (
	protocolVersion = 0b0001
	headerSize      = 1 // in 4-byte units (total 4 bytes)

	// Message types (byte 1, high nibble)
	msgTypeClientFullRequest  = 0b0001
	msgTypeClientAudioRequest = 0b0010
	msgTypeServerFullResponse = 0b1001
	msgTypeServerError        = 0b1111

	// Message type specific flags (byte 1, low nibble)
	flagPosSequence     = 0b0001
	flagNegWithSequence = 0b0011

	// Serialization (byte 2, high nibble)
	serializationJSON = 0b0001

	// Compression (byte 2, low nibble)
	compressionGZIP = 0b0001
)

// --- Request payload types ---

type userMeta struct {
	UID string `json:"uid,omitempty"`
}

type audioMeta struct {
	Format  string `json:"format,omitempty"`
	Codec   string `json:"codec,omitempty"`
	Rate    int    `json:"rate,omitempty"`
	Bits    int    `json:"bits,omitempty"`
	Channel int    `json:"channel,omitempty"`
}

type requestMeta struct {
	ModelName       string `json:"model_name,omitempty"`
	EnableITN       bool   `json:"enable_itn,omitempty"`
	EnablePUNC      bool   `json:"enable_punc,omitempty"`
	EnableDDC       bool   `json:"enable_ddc,omitempty"`
	ShowUtterances  bool   `json:"show_utterances"`
	EnableNonstream bool   `json:"enable_nonstream"`
}

type asrRequestPayload struct {
	User    userMeta    `json:"user"`
	Audio   audioMeta   `json:"audio"`
	Request requestMeta `json:"request"`
}

// --- Response payload types ---

// ServerResponse 服务端响应 JSON 结构
type ServerResponse struct {
	Code          int              `json:"code"`
	Event         int              `json:"event"`
	IsLastPackage bool             `json:"is_last_package"`
	PayloadMsg    *responsePayload `json:"payload_msg"`
}

type responsePayload struct {
	AudioInfo struct {
		Duration int `json:"duration"`
	} `json:"audio_info"`
	Result struct {
		Text       string `json:"text"`
		Utterances []struct {
			Definite  bool   `json:"definite"`
			EndTime   int    `json:"end_time"`
			StartTime int    `json:"start_time"`
			Text      string `json:"text"`
		} `json:"utterances,omitempty"`
	} `json:"result"`
	Error string `json:"error,omitempty"`
}

// --- Protocol functions ---

// buildHeader 构建 4 字节协议头
func buildHeader(msgType byte, flags byte) []byte {
	header := make([]byte, 4)
	header[0] = (protocolVersion << 4) | headerSize
	header[1] = (msgType << 4) | flags
	header[2] = (serializationJSON << 4) | compressionGZIP
	header[3] = 0x00
	return header
}

// gzipCompress gzip 压缩
func gzipCompress(input []byte) []byte {
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	w.Write(input)
	w.Close()
	return b.Bytes()
}

// gzipDecompress gzip 解压
func gzipDecompress(input []byte) ([]byte, error) {
	b := bytes.NewBuffer(input)
	r, err := gzip.NewReader(b)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

// buildFullClientRequest 构建初始配置消息 (CLIENT_FULL_REQUEST)
// 这是连接后发送的第一条消息，包含音频格式和识别参数
func buildFullClientRequest() ([]byte, error) {
	payload := asrRequestPayload{
		User: userMeta{
			UID: "gameai_user",
		},
		Audio: audioMeta{
			Format:  "wav",
			Codec:   "raw",
			Rate:    16000,
			Bits:    16,
			Channel: 1,
		},
		Request: requestMeta{
			ModelName:       "bigmodel",
			EnableITN:       true,
			EnablePUNC:      true,
			EnableDDC:       true,
			ShowUtterances:  true,
			EnableNonstream: false,
		},
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	compressed := gzipCompress(jsonData)

	var buf bytes.Buffer
	// 4 字节 header
	buf.Write(buildHeader(msgTypeClientFullRequest, flagPosSequence))
	// int32(1) — payload sequence (固定为 1)
	binary.Write(&buf, binary.BigEndian, int32(1))
	// uint32 — compressed payload size
	binary.Write(&buf, binary.BigEndian, uint32(len(compressed)))
	// compressed payload
	buf.Write(compressed)

	return buf.Bytes(), nil
}

// buildAudioRequest 构建音频段消息 (CLIENT_AUDIO_ONLY_REQUEST)
// seq > 0: 正常段, seq < 0: 最后段 (negative sequence 表示结束)
func buildAudioRequest(seq int32, audioData []byte) ([]byte, error) {
	compressed := gzipCompress(audioData)

	var flags byte
	if seq < 0 {
		flags = flagNegWithSequence
	} else {
		flags = flagPosSequence
	}

	var buf bytes.Buffer
	// 4 字节 header
	buf.Write(buildHeader(msgTypeClientAudioRequest, flags))
	// int32 — sequence number
	binary.Write(&buf, binary.BigEndian, seq)
	// uint32 — compressed audio size
	binary.Write(&buf, binary.BigEndian, uint32(len(compressed)))
	// compressed audio data
	buf.Write(compressed)

	return buf.Bytes(), nil
}

// parseServerResponse 解析服务端响应
// 返回解析后的 ServerResponse 结构
func parseServerResponse(data []byte) (*ServerResponse, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("response too short: %d bytes", len(data))
	}

	// 解析 header
	headerSizeInBytes := int(data[0]&0x0f) * 4
	if headerSizeInBytes < 4 {
		headerSizeInBytes = 4
	}

	msgType := data[1] >> 4
	flags := data[1] & 0x0f
	compression := data[2] & 0x0f

	payload := data[headerSizeInBytes:]

	result := &ServerResponse{}

	// 解析 flags
	if flags&0x01 != 0 && len(payload) >= 4 {
		// payload sequence
		payload = payload[4:]
	}
	if flags&0x02 != 0 {
		result.IsLastPackage = true
	}
	if flags&0x04 != 0 && len(payload) >= 4 {
		// event
		payload = payload[4:]
	}

	// 解析 message type specific data
	switch msgType {
	case msgTypeServerFullResponse:
		if len(payload) < 4 {
			return result, nil
		}
		payloadSize := int(binary.BigEndian.Uint32(payload[:4]))
		payload = payload[4:]
		if len(payload) > payloadSize {
			payload = payload[:payloadSize]
		}
	case msgTypeServerError:
		if len(payload) < 8 {
			return result, nil
		}
		result.Code = int(binary.BigEndian.Uint32(payload[:4]))
		payloadSize := int(binary.BigEndian.Uint32(payload[4:8]))
		payload = payload[8:]
		if len(payload) > payloadSize {
			payload = payload[:payloadSize]
		}
	}

	if len(payload) == 0 {
		return result, nil
	}

	// 解压
	if compression == compressionGZIP {
		decompressed, err := gzipDecompress(payload)
		if err != nil {
			return nil, fmt.Errorf("gzip decompress: %w", err)
		}
		payload = decompressed
	}

	// 解析 JSON
	if err := json.Unmarshal(payload, result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return result, nil
}
