package audio

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"
)

// TestFormat_BytesPerSample 测试采样格式字节数计算
func TestFormat_BytesPerSample(t *testing.T) {
	tests := []struct {
		format   Format
		expected int
	}{
		{Format{SampleRate: 24000, Channels: 1, SampleFormat: SampleFormatInt16LE}, 2},
		{Format{SampleRate: 44100, Channels: 2, SampleFormat: SampleFormatFloat32LE}, 4},
		{Format{SampleRate: 8000, Channels: 1, SampleFormat: SampleFormatUInt8}, 1},
	}

	for i, tt := range tests {
		if got := tt.format.BytesPerSample(); got != tt.expected {
			t.Errorf("Test %d: BytesPerSample() = %d, want %d", i, got, tt.expected)
		}
	}
}

// TestFormat_BytesPerFrame 测试每帧字节数计算
func TestFormat_BytesPerFrame(t *testing.T) {
	tests := []struct {
		format   Format
		expected int
	}{
		{Format{SampleRate: 24000, Channels: 1, SampleFormat: SampleFormatInt16LE}, 2},  // mono, 16-bit
		{Format{SampleRate: 44100, Channels: 2, SampleFormat: SampleFormatInt16LE}, 4},  // stereo, 16-bit
		{Format{SampleRate: 48000, Channels: 2, SampleFormat: SampleFormatFloat32LE}, 8}, // stereo, 32-bit
	}

	for i, tt := range tests {
		if got := tt.format.BytesPerFrame(); got != tt.expected {
			t.Errorf("Test %d: BytesPerFrame() = %d, want %d", i, got, tt.expected)
		}
	}
}

// TestPCM16Mono24kHz 测试默认 PCM 格式
func TestPCM16Mono24kHz(t *testing.T) {
	if PCM16Mono24kHz.SampleRate != 24000 {
		t.Errorf("PCM16Mono24kHz.SampleRate = %d, want 24000", PCM16Mono24kHz.SampleRate)
	}
	if PCM16Mono24kHz.Channels != 1 {
		t.Errorf("PCM16Mono24kHz.Channels = %d, want 1", PCM16Mono24kHz.Channels)
	}
	if PCM16Mono24kHz.SampleFormat != SampleFormatInt16LE {
		t.Errorf("PCM16Mono24kHz.SampleFormat = %d, want %d", PCM16Mono24kHz.SampleFormat, SampleFormatInt16LE)
	}
}

// TestPlaybackService_StopWithoutPlay 测试未播放时调用 Stop 不会 panic
func TestPlaybackService_StopWithoutPlay(t *testing.T) {
	service := &PlaybackService{}
	// 不应该 panic
	service.Stop()
}

// TestPlaybackSession_StopMultipleTimes 测试多次调用 Stop 不会 panic
func TestPlaybackSession_StopMultipleTimes(t *testing.T) {
	session := &PlaybackSession{
		doneCh: make(chan struct{}),
		cancel: func() {},
	}

	// 多次调用不应该 panic
	session.Stop()
	session.Stop()
	session.Stop()
}

// TestStream_Close 测试 Stream 的 Close 函数
func TestStream_Close(t *testing.T) {
	closed := false
	stream := &Stream{
		Reader: bytes.NewReader([]byte{}),
		Format: PCM16Mono24kHz,
		Close: func() {
			closed = true
		},
	}

	if closed {
		t.Error("Close should not be called yet")
	}

	stream.Close()

	if !closed {
		t.Error("Close should have been called")
	}
}

// TestStream_NilClose 测试 Close 为 nil 时不会 panic
func TestStream_NilClose(t *testing.T) {
	stream := &Stream{
		Reader: bytes.NewReader([]byte{}),
		Format: PCM16Mono24kHz,
		Close:  nil,
	}

	// 不应该 panic
	if stream.Close != nil {
		stream.Close()
	}
}

// TestPlaybackSession_WaitWithTimeout 测试 Wait 可以正常返回
func TestPlaybackSession_WaitWithTimeout(t *testing.T) {
	session := &PlaybackSession{
		doneCh: make(chan struct{}),
	}

	// 异步关闭 doneCh
	go func() {
		time.Sleep(10 * time.Millisecond)
		close(session.doneCh)
	}()

	// Wait 应该在 doneCh 关闭后返回
	done := make(chan struct{})
	go func() {
		session.Wait()
		close(done)
	}()

	select {
	case <-done:
		// 成功
	case <-time.After(1 * time.Second):
		t.Error("Wait() did not return after doneCh was closed")
	}
}

// TestPlaybackSession_WaitWithContext 测试 context 取消时 Wait 的行为
func TestPlaybackSession_WaitWithContext(t *testing.T) {
	session := &PlaybackSession{
		doneCh: make(chan struct{}),
		cancel: func() {},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Wait 应该在 context 超时后返回（通过 doneCh 关闭）
	done := make(chan struct{})
	go func() {
		session.Wait()
		close(done)
	}()

	// 等待一段时间后关闭 doneCh
	go func() {
		<-ctx.Done()
		time.Sleep(10 * time.Millisecond)
		close(session.doneCh)
	}()

	select {
	case <-done:
		// 成功
	case <-time.After(1 * time.Second):
		t.Error("Wait() did not return")
	}
}

// TestOtoPlayer_IsFormatSupported 测试格式支持检查
func TestOtoPlayer_IsFormatSupported(t *testing.T) {
	player := &OtoPlayer{}

	// 支持的格式
	if !player.isFormatSupported(PCM16Mono24kHz) {
		t.Error("PCM16Mono24kHz should be supported")
	}

	// 不支持的格式
	unsupported := Format{
		SampleRate:   44100,
		Channels:     2,
		SampleFormat: SampleFormatFloat32LE,
	}
	if player.isFormatSupported(unsupported) {
		t.Error("44100Hz stereo float32 should not be supported")
	}
}

// TestPlaybackService_PlayStreamWithNilService 测试 nil service 时 PlayStream 的行为
func TestPlaybackService_PlayStreamWithNilService(t *testing.T) {
	var service *PlaybackService

	stream := &Stream{
		Reader: bytes.NewReader([]byte{}),
		Format: PCM16Mono24kHz,
	}

	// 应该返回 error
	err := service.PlayStream(context.Background(), stream, nil)
	if err == nil {
		t.Error("PlayStream with nil service should return error")
	}
}

// TestPlaybackService_FallbackToMCI 测试 fallback 机制（模拟）
func TestPlaybackService_FallbackToMCI(t *testing.T) {
	// 创建一个 mock MCI player
	mockMCI := &mockMCIPlayer{
		playFunc: func(filePath string) error {
			return nil
		},
	}

	service := &PlaybackService{
		otoPlayer: nil, // Oto 不可用
		mciPlayer: mockMCI,
	}

	// 创建测试 PCM 数据
	pcmData := make([]byte, 1000)
	stream := &Stream{
		Reader: bytes.NewReader(pcmData),
		Format: PCM16Mono24kHz,
	}

	// 应该 fallback 到 MCI
	err := service.PlayStream(context.Background(), stream, nil)
	if err != nil {
		t.Errorf("PlayStream should fallback to MCI: %v", err)
	}

	if !mockMCI.playCalled {
		t.Error("MCI play should have been called")
	}
}

// mockMCIPlayer 模拟 MCI 播放器
type mockMCIPlayer struct {
	playFunc    func(filePath string) error
	stopFunc    func() error
	setOnEndCB  func(cb func())
	playCalled  bool
	stopCalled  bool
	onEndSet    bool
}

func (m *mockMCIPlayer) Play(filePath string) error {
	m.playCalled = true
	if m.playFunc != nil {
		return m.playFunc(filePath)
	}
	return nil
}

func (m *mockMCIPlayer) Stop() error {
	m.stopCalled = true
	if m.stopFunc != nil {
		return m.stopFunc()
	}
	return nil
}

func (m *mockMCIPlayer) SetOnEnd(cb func()) {
	m.onEndSet = true
	if m.setOnEndCB != nil {
		m.setOnEndCB(cb)
	}
}

// TestStream_ReaderEOF 测试 Stream Reader 到达 EOF 时的行为
func TestStream_ReaderEOF(t *testing.T) {
	data := []byte{1, 2, 3, 4, 5}
	stream := &Stream{
		Reader: bytes.NewReader(data),
		Format: PCM16Mono24kHz,
	}

	buf := make([]byte, 10)
	n, err := stream.Reader.Read(buf)
	if n != 5 {
		t.Errorf("Read %d bytes, want 5", n)
	}
	if err != nil {
		t.Errorf("Read error: %v", err)
	}

	// 再次读取应该返回 EOF
	n, err = stream.Reader.Read(buf)
	if n != 0 {
		t.Errorf("Read %d bytes after EOF, want 0", n)
	}
	if err != io.EOF {
		t.Errorf("Expected EOF, got %v", err)
	}
}
