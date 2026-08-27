package wakeword

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"

	"github.com/lipipi/aurora/internal/errors"
	"github.com/lipipi/aurora/internal/log"
)

// DownloadProgress is the callback for reporting download progress.
type DownloadProgress func(percent int, message string)

// AutoDownloadIfNeeded checks if ONNX Runtime and models are already downloaded,
// downloads them if missing. Returns whether download was successful.
func AutoDownloadIfNeeded(
	ctx context.Context,
	cfg Config,
	appDataDir string,
	progress DownloadProgress,
) errors.AppError {
	onnxDir := filepath.Join(appDataDir, "onnxruntime")
	onnxPath := filepath.Join(onnxDir, "onnxruntime.dll")

	if _, err := os.Stat(onnxPath); os.IsNotExist(err) {
		progress(0, "Downloading ONNX Runtime...")
		if err := downloadONNXRuntime(ctx, onnxDir, onnxPath); err != nil {
			return errors.Wrap(errors.CodeDownloadFailed, "failed to download ONNX Runtime", err)
		}
		progress(100, "ONNX Runtime downloaded")
	} else if err != nil {
		return errors.Wrap(errors.CodeDownloadFailed, "failed to check ONNX Runtime", err)
	}

	// Check models
	modelsDir := filepath.Join(appDataDir, "wakeword-models")
	if err := os.MkdirAll(modelsDir, 0755); err != nil {
		return errors.Wrap(errors.CodeDownloadFailed, "failed to create models directory", err)
	}

	// Download required base models
	// These base models are hosted on the openWakeWord releases
	baseModels := []struct {
		name string
		url  string
	}{
		{"melspectrogram.onnx", "https://github.com/dscripka/openWakeWord/releases/download/v0.5.1/melspectrogram.onnx"},
		{"embedding_model.onnx", "https://github.com/dscripka/openWakeWord/releases/download/v0.5.1/embedding_model.onnx"},
		{"silero_vad.onnx", "https://github.com/dscripka/openWakeWord/releases/download/v0.5.1/silero_vad.onnx"},
	}

	defaultWakeModel := struct {
		name string
		url  string
	}{
		"hey_mycroft_v0.1.onnx",
		"https://github.com/dscripka/openWakeWord/releases/download/v0.5.1/hey_mycroft_v0.1.onnx",
	}

	total := len(baseModels) + 1
	downloaded := 0

	for _, m := range baseModels {
		modelPath := filepath.Join(modelsDir, m.name)
		if _, err := os.Stat(modelPath); os.IsNotExist(err) {
			progress((downloaded*100)/total, fmt.Sprintf("Downloading %s...", m.name))
			if err := downloadFile(ctx, m.url, modelPath); err != nil {
				return errors.Wrap(errors.CodeDownloadFailed, fmt.Sprintf("failed to download %s", m.name), err)
			}
			log.Infof("Downloaded %s to %s", m.name, modelPath)
		}
		downloaded++
	}

	// Download default wake word model if model path not set
	if cfg.ModelPath == "" {
		defaultModelPath := filepath.Join(modelsDir, defaultWakeModel.name)
		if _, err := os.Stat(defaultModelPath); os.IsNotExist(err) {
			progress((downloaded*100)/total, fmt.Sprintf("Downloading default wake word model..."))
			if err := downloadFile(ctx, defaultWakeModel.url, defaultModelPath); err != nil {
				return errors.Wrap(errors.CodeDownloadFailed, "failed to download default wake word model", err)
			}
			log.Infof("Downloaded default wake word model to %s", defaultModelPath)
		}
	}

	progress(100, "All files downloaded successfully")
	log.Info("openwakeword: all required files are ready")
	return nil
}

// downloadONNXRuntime downloads ONNX Runtime for Windows and extracts onnxruntime.dll.
func downloadONNXRuntime(ctx context.Context, targetDir, targetPath string) errors.AppError {
	// Current ONNX Runtime version for Windows x64
	version := "1.16.3"
	url := fmt.Sprintf("https://github.com/microsoft/onnxruntime/releases/download/v%s/onnxruntime-win-x64-%s.zip", version, version)

	if runtime.GOARCH != "amd64" {
		return errors.New(errors.CodeDownloadFailed, "only amd64 architecture is supported for ONNX Runtime on Windows")
	}

	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return errors.Wrap(errors.CodeDownloadFailed, "failed to create onnxruntime directory", err)
	}

	// Download the zip
	log.Infof("Downloading ONNX Runtime from: %s", url)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return errors.Wrap(errors.CodeDownloadFailed, "failed to create request", err)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return errors.Wrap(errors.CodeDownloadFailed, "failed to download ONNX Runtime", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errors.Newf(errors.CodeDownloadFailed, "bad status: %s", resp.Status)
	}

	// Read the zip into memory
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, resp.Body); err != nil {
		return errors.Wrap(errors.CodeDownloadFailed, "failed to read ONNX Runtime zip", err)
	}

	// Extract onnxruntime.dll
	reader, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		return errors.Wrap(errors.CodeDownloadFailed, "failed to read zip", err)
	}

	// Find onnxruntime.dll in the zip
	dllFound := false
	for _, file := range reader.File {
		// Path is like onnxruntime-win-x64-1.16.3/lib/onnxruntime.dll
		if filepath.Base(file.Name) == "onnxruntime.dll" {
			if err := extractZipFile(file, targetPath); err != nil {
				return errors.Wrap(errors.CodeDownloadFailed, "failed to extract onnxruntime.dll", err)
			}
			dllFound = true
			break
		}
	}

	if !dllFound {
		return errors.New(errors.CodeDownloadFailed, "onnxruntime.dll not found in downloaded zip")
	}

	log.Infof("ONNX Runtime extracted to: %s", targetPath)
	return nil
}

func extractZipFile(zf *zip.File, targetPath string) error {
	rc, err := zf.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	outFile, err := os.Create(targetPath)
	if err != nil {
		return err
	}
	defer outFile.Close()

	_, err = io.Copy(outFile, rc)
	return err
}

// progressReader wraps an io.Reader to report download progress.
type progressReader struct {
	reader     io.Reader
	total      int64
	downloaded int64
	onProgress func(percent int)
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	pr.downloaded += int64(n)
	if pr.total > 0 {
		percent := int(float64(pr.downloaded) / float64(pr.total) * 100)
		pr.onProgress(percent)
	}
	return n, err
}

// downloadFile downloads a single file directly with progress reporting.
func downloadFile(ctx context.Context, url, targetPath string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	out, err := os.Create(targetPath)
	if err != nil {
		return err
	}
	defer out.Close()

	// Wrap the response body to track download progress
	// We don't report to the main progress callback here because AutoDownloadIfNeeded
	// already reports overall file progress. This just helps with CPU profiling.
	total := resp.ContentLength
	var reader io.Reader = resp.Body
	if total > 0 {
		reader = &progressReader{
			reader:     resp.Body,
			total:      total,
			downloaded: 0,
			onProgress: func(percent int) {
				// Don't need to do anything for now - overall progress is tracked at the file level
			},
		}
	}

	_, err = io.Copy(out, reader)
	return err
}

// IsAlreadyDownloaded checks if all required files are already downloaded.
func IsAlreadyDownloaded(cfg Config, appDataDir string) bool {
	modelsDir := filepath.Join(appDataDir, "wakeword-models")

	requiredFiles := []string{
		filepath.Join(modelsDir, "melspectrogram.onnx"),
		filepath.Join(modelsDir, "embedding_model.onnx"),
		filepath.Join(modelsDir, "silero_vad.onnx"),
	}

	if cfg.ModelPath == "" {
		requiredFiles = append(requiredFiles, filepath.Join(modelsDir, "hey_mycroft_v0.1.onnx"))
	} else {
		requiredFiles = append(requiredFiles, cfg.ModelPath)
	}

	// ONNX check is different location on Windows
	if runtime.GOOS == "windows" {
		onnxPath := filepath.Join(appDataDir, "onnxruntime", "onnxruntime.dll")
		requiredFiles = append(requiredFiles, onnxPath)
	}

	for _, path := range requiredFiles {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			log.Debugf("openwakeword: missing file: %s", path)
			return false
		}
	}

	return true
}
