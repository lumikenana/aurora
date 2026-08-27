//go:build windows

// Package openwakeword implements the WakeWordDetector interface using the
// github.com/stanislaw-glogowski/openwakeword_go library.
package openwakeword

import (
	"context"
	"path/filepath"
	"sync"
	"time"

	"github.com/lipipi/aurora/internal/config"
	"github.com/lipipi/aurora/internal/errors"
	"github.com/lipipi/aurora/internal/log"
	"github.com/lipipi/aurora/internal/wakeword"
	oww "github.com/stanislaw-glogowski/openwakeword_go"
)

// openWakeWordDetector implements wakeword.WakeWordDetector.
// The actual initializeONNX implementation is platform-specific.
type openWakeWordDetector struct {
	cfg             wakeword.Config
	engine          *oww.Engine
	audioFeatures   *oww.AudioFeatures
	vad             *oww.VAD
	events          chan wakeword.WakeEvent
	ctx             context.Context
	cancel          context.CancelFunc
	running         bool
	mu              sync.Mutex
	accumulatedSamples []float32 // accumulate 16-bit samples to 1280-sample frames
	lastWakeTime    time.Time
}

// Start implements WakeWordDetector.Start.
func (d *openWakeWordDetector) Start(ctx context.Context, audioCh <-chan []byte) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.running {
		return nil // already running
	}

	if !d.IsConfigured() {
		return errors.New(errors.CodeWakeWordFailed, "openwakeword not configured: missing model path")
	}

	// We need to do ONNX initialization here after the library path is set
	// by platform-specific code
	appDataDir := config.GetAppDataDir()
	modelsDir := filepath.Join(appDataDir, "wakeword-models")

	melspectrogramPath := filepath.Join(modelsDir, "melspectrogram.onnx")
	embeddingModelPath := filepath.Join(modelsDir, "embedding_model.onnx")
	vadModelPath := filepath.Join(modelsDir, "silero_vad.onnx")

	if err := d.initializeONNX(d.cfg.OnnxRuntimePath, melspectrogramPath, embeddingModelPath, vadModelPath); err != nil {
		return err
	}

	d.ctx, d.cancel = context.WithCancel(ctx)
	d.running = true

	// Start the processing goroutine
	go d.processLoop(audioCh)

	log.Infof("openwakeword detector started, model: %s", d.cfg.ModelPath)
	return nil
}

// Stop implements WakeWordDetector.Stop.
func (d *openWakeWordDetector) Stop() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.running {
		return nil
	}

	if d.cancel != nil {
		d.cancel()
	}

	// Close ONNX sessions
	if d.engine != nil {
		d.engine.Close()
	}
	if d.audioFeatures != nil {
		d.audioFeatures.Close()
	}
	if d.vad != nil {
		d.vad.Close()
	}

	d.engine = nil
	d.audioFeatures = nil
	d.vad = nil
	d.running = false
	close(d.events)

	log.Info("openwakeword detector stopped")
	return nil
}

// Events implements WakeWordDetector.Events.
func (d *openWakeWordDetector) Events() <-chan wakeword.WakeEvent {
	return d.events
}

// IsConfigured implements WakeWordDetector.IsConfigured.
// If ModelPath is empty, we still consider it configured because we'll automatically
// use the default pre-downloaded hey_mycroft_v0.1.onnx model.
func (d *openWakeWordDetector) IsConfigured() bool {
	return d.cfg.Provider == "openwakeword"
}

// Name implements WakeWordDetector.Name.
func (d *openWakeWordDetector) Name() string {
	return "openwakeword"
}

func (d *openWakeWordDetector) processLoop(audioCh <-chan []byte) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("openwakeword processLoop panicked: %v", r)
			select {
			case d.events <- wakeword.WakeEvent{
				Type:    wakeword.WakeEventError,
				Message: "panic in detection loop",
			}:
			default:
			}
		}
	}()

	for {
		select {
		case <-d.ctx.Done():
			return
		case data, ok := <-audioCh:
			if !ok {
				return
			}
			d.processAudioChunk(data)
		}
	}
}

// processAudioChunk converts 16-bit PCM bytes to float32 samples, accumulates
// to 1280-sample frames, and runs prediction.
func (d *openWakeWordDetector) processAudioChunk(data []byte) {
	// Input is 16-bit mono PCM at 16kHz: each sample is 2 bytes
	// data length must be even
	if len(data)%2 != 0 {
		log.Warn("openwakeword: odd number of bytes in audio chunk, skipping")
		return
	}

	// Convert bytes to float32 samples (range -1.0 to 1.0)
	numSamples := len(data) / 2
	samples := make([]float32, numSamples)
	for i := 0; i < numSamples; i++ {
		// little-endian 16-bit signed integer
		sample := int16(data[i*2]) | int16(data[i*2+1])<<8
		// Convert to float32 in [-1, 1]
		samples[i] = float32(sample) / 32768.0
	}

	// Accumulate samples until we have at least FrameSamples (1280)
	d.mu.Lock()
	defer d.mu.Unlock()

	d.accumulatedSamples = append(d.accumulatedSamples, samples...)

	// Process full frames
	for len(d.accumulatedSamples) >= oww.FrameSamples {
		// Extract the next frame
		frame := d.accumulatedSamples[:oww.FrameSamples]
		d.accumulatedSamples = d.accumulatedSamples[oww.FrameSamples:]

		d.predictFrame(frame)
	}
}

// predictFrame runs prediction on a 1280-sample frame and checks for detection.
func (d *openWakeWordDetector) predictFrame(frame oww.Samples) {
	if d.engine == nil {
		return
	}

	// Run detection
	detections, err := d.engine.Detect(frame)
	if err != nil {
		log.Errorf("openwakeword prediction failed: %v", err)
		return
	}

	// Check each model for detection
	for name, detected := range detections {
		if detected {
			scores, _ := d.engine.Predict(frame)
			score := scores[name]

			// Check cooldown
			now := time.Now()
			cooldown := time.Duration(d.cfg.CooldownMs) * time.Millisecond
			if now.Sub(d.lastWakeTime) < cooldown {
				log.Debugf("openwakeword: %s detected but in cooldown, skipping", name)
				continue
			}

			// Detection passed all checks
			log.Infof("openwakeword: wake word detected: %s, score: %.3f", name, score)
			d.lastWakeTime = now

			// Send detection event (non-blocking in case channel is full)
			select {
			case d.events <- wakeword.WakeEvent{
				Type:      wakeword.WakeEventDetected,
				ModelName: name,
				Score:     score,
			}:
			default:
				log.Warn("openwakeword: event channel full, dropping detection")
			}
		}
	}
}
