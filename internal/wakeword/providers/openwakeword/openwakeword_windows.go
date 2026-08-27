//go:build windows

package openwakeword

import (
	"os"
	"path/filepath"

	"github.com/lipipi/aurora/internal/config"
	"github.com/lipipi/aurora/internal/errors"
	"github.com/lipipi/aurora/internal/log"
	"github.com/lipipi/aurora/internal/wakeword"
	oww "github.com/stanislaw-glogowski/openwakeword_go"
	ort "github.com/yalue/onnxruntime_go"
)

// This file handles Windows-specific ONNX Runtime DLL loading.

// New creates a new openWakeWordDetector for Windows from the given config.
func New(cfg any) (wakeword.WakeWordDetector, errors.AppError) {
	// Type assertion for config
	var wakeCfg wakeword.Config
	switch c := cfg.(type) {
	case wakeword.Config:
		wakeCfg = c
	default:
		return nil, errors.New(errors.CodeWakeWordFailed, "invalid config type for openwakeword")
	}

	d := &openWakeWordDetector{
		cfg:             wakeCfg,
		events:          make(chan wakeword.WakeEvent, 8), // buffered for multiple events
		accumulatedSamples: make([]float32, 0, oww.FrameSamples),
	}

	return d, nil
}

// setONNXLibraryPath tries to find and set the ONNX Runtime DLL path.
func (d *openWakeWordDetector) setONNXLibraryPath(customPath string) errors.AppError {
	// Try custom path first
	if customPath != "" {
		if _, err := os.Stat(customPath); err == nil {
			ort.SetSharedLibraryPath(customPath)
			// Initialize the ONNX environment after setting shared library path
			if err := ort.InitializeEnvironment(); err != nil {
				log.Errorf("openwakeword: failed to initialize ONNX Environment from custom path: %v", err)
				return errors.Wrap(errors.CodeOnnxRuntimeNotFound, "failed to initialize ONNX environment from custom path", err)
			}
			log.Infof("openwakeword: using custom ONNX Runtime from: %s", customPath)
			return nil
		}
		log.Warnf("openwakeword: custom ONNX path %s not found, trying default locations", customPath)
	}

	// Try app data directory: %APPDATA%\GameAI\onnxruntime\onnxruntime.dll
	appDataDir := config.GetAppDataDir()
	defaultOnnxPath := filepath.Join(appDataDir, "onnxruntime", "onnxruntime.dll")
	if _, err := os.Stat(defaultOnnxPath); err == nil {
		ort.SetSharedLibraryPath(defaultOnnxPath)
		// Initialize the ONNX environment after setting shared library path
		if err := ort.InitializeEnvironment(); err != nil {
			log.Errorf("openwakeword: failed to initialize ONNX Environment from app data: %v", err)
			return errors.Wrap(errors.CodeOnnxRuntimeNotFound, "failed to initialize ONNX environment from app data directory", err)
		}
		log.Infof("openwakeword: using ONNX Runtime from: %s", defaultOnnxPath)
		return nil
	}

	// Try executable directory
	exeDir := config.GetAppDir()
	exeOnnxPath := filepath.Join(exeDir, "onnxruntime.dll")
	if _, err := os.Stat(exeOnnxPath); err == nil {
		ort.SetSharedLibraryPath(exeOnnxPath)
		// Initialize the ONNX environment after setting shared library path
		if err := ort.InitializeEnvironment(); err != nil {
			log.Errorf("openwakeword: failed to initialize ONNX Environment from exe directory: %v", err)
			return errors.Wrap(errors.CodeOnnxRuntimeNotFound, "failed to initialize ONNX environment from executable directory", err)
		}
		log.Infof("openwakeword: using ONNX Runtime from exe directory: %s", exeOnnxPath)
		return nil
	}

	// Try system PATH
	if dllPath := searchPATH("onnxruntime.dll"); dllPath != "" {
		ort.SetSharedLibraryPath(dllPath)
		// Initialize the ONNX environment after setting shared library path
		if err := ort.InitializeEnvironment(); err != nil {
			log.Errorf("openwakeword: failed to initialize ONNX Environment from system PATH: %v", err)
			return errors.Wrap(errors.CodeOnnxRuntimeNotFound, "failed to initialize ONNX environment from system PATH", err)
		}
		log.Infof("openwakeword: using ONNX Runtime from system PATH: %s", dllPath)
		return nil
	}

	// None found
	return errors.New(errors.CodeOnnxRuntimeNotFound,
		"ONNX Runtime DLL not found. Please download ONNX Runtime and configure the path in settings.")
}

// searchPATH searches the PATH environment variable for the given DLL name
// and returns the full path if found.
func searchPATH(dllName string) string {
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == "" {
			continue
		}
		candidate := filepath.Join(dir, dllName)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

// initializeONNX does the actual ONNX initialization on Windows.
func (d *openWakeWordDetector) initializeONNX(
	onnxRuntimePath, melspectrogramPath, embeddingModelPath, vadModelPath string,
) errors.AppError {
	// Set the ONNX library path and load it
	if err := d.setONNXLibraryPath(onnxRuntimePath); err != nil {
		return err
	}

	// If user didn't specify a custom model path, use the default auto-downloaded one
	if d.cfg.ModelPath == "" {
		appDataDir := config.GetAppDataDir()
		modelsDir := filepath.Join(appDataDir, "wakeword-models")
		defaultModelPath := filepath.Join(modelsDir, "hey_mycroft_v0.1.onnx")
		d.cfg.ModelPath = defaultModelPath
		log.Infof("openwakeword: using default model: %s", d.cfg.ModelPath)
	}

	// Initialize audio features (melspectrogram + embedding)
	var err error
	d.audioFeatures, err = oww.NewAudioFeatures(melspectrogramPath, embeddingModelPath)
	if err != nil {
		return errors.Wrap(errors.CodeWakeWordFailed, "failed to initialize audio features", err)
	}

	// Initialize VAD if enabled
	if d.cfg.VADEnabled {
		d.vad, err = oww.NewVAD(vadModelPath)
		if err != nil {
			return errors.Wrap(errors.CodeWakeWordFailed, "failed to initialize VAD", err)
		}
	} else {
		d.vad = nil
	}

	// Create the engine
	d.engine, err = oww.New(d.audioFeatures, d.vad)
	if err != nil {
		return errors.Wrap(errors.CodeWakeWordFailed, "failed to create wake word engine", err)
	}

	// Add the wake word model with configured threshold
	modelName := filepath.Base(d.cfg.ModelPath)
	err = d.engine.AddModel(
		d.cfg.ModelPath,
		oww.WithModelThreshold(d.cfg.Threshold),
		oww.WithModelName(modelName),
	)
	if err != nil {
		return errors.Wrap(errors.CodeWakeWordFailed, "failed to add wake word model", err)
	}

	log.Infof("openwakeword: initialized successfully, model=%s, threshold=%.3f, vad=%v",
		d.cfg.ModelPath, d.cfg.Threshold, d.cfg.VADEnabled)

	return nil
}

func init() {
	// Register the provider with the factory
	wakeword.RegisterProvider("openwakeword", New)
}
