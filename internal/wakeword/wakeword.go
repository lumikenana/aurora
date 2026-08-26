// Package wakeword defines the wake word detector interface and types.
package wakeword

import (
	"context"
)

// WakeEventType defines the type of wake event.
type WakeEventType int

const (
	// WakeEventDetected means a wake word was detected.
	WakeEventDetected WakeEventType = iota
	// WakeEventError means an error occurred during detection.
	WakeEventError
)

// WakeEvent is sent when a wake event occurs.
type WakeEvent struct {
	Type        WakeEventType `json:"type"`
	ModelName   string        `json:"model_name"` // Name of the model that triggered
	Score       float32        `json:"score"`     // Detection confidence score
	Message     string        `json:"message"`   // Error message for error events
}

// WakeWordDetector defines the interface for wake word detection.
// Upper-level components (VoiceController) depend on this interface,
// not on concrete implementations.
type WakeWordDetector interface {
	// Start starts background detection, consuming audio chunks from audioCh.
	// Detection events are sent through the Events() channel.
	// The detector keeps running until ctx is canceled or Stop() is called.
	Start(ctx context.Context, audioCh <-chan []byte) error

	// Stop stops detection and releases all resources (ONNX sessions, etc.).
	Stop() error

	// Events returns the channel for receiving wake events.
	Events() <-chan WakeEvent

	// IsConfigured checks if the detector is properly configured with model path etc.
	IsConfigured() bool

	// Name returns the provider name for logging.
	Name() string
}

// Config holds the top-level configuration for wake word detection.
type Config struct {
	Provider        string  `json:"provider"`          // Provider name (currently only "openwakeword")
	Enabled         bool    `json:"enabled"`           // Whether to enable wake word detection (mutually exclusive with PTT mode? No, PTT can still work)
	ModelPath       string  `json:"model_path"`        // Path to the .onnx wake word model file
	Threshold       float32 `json:"threshold"`         // Detection threshold (0.0 - 1.0), higher = fewer false positives
	CooldownMs      int     `json:"cooldown_ms"`       // Cooldown in milliseconds after detection before next can trigger
	VADEnabled      bool    `json:"vad_enabled"`       // Whether to enable Silero VAD to skip silent frames
	OnnxRuntimePath string  `json:"onnx_runtime_path"` // Optional custom path to onnxruntime.dll (Windows)
}

// DefaultConfig returns sensible default configuration values.
func DefaultConfig() Config {
	return Config{
		Provider:   "openwakeword",
		Enabled:    false,
		Threshold:  0.5,
		CooldownMs: 1000,
		VADEnabled: true,
	}
}
