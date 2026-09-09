package agent

import (
	"github.com/lipipi/aurora/internal/audio"
	"github.com/lipipi/aurora/internal/config"
	"github.com/lipipi/aurora/internal/log"
)

// --- Audio Device bindings (Phase 1) ---

// AudioDeviceResponse 返回给前端的设备信息
type AudioDeviceResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ListAudioDevices 枚举所有音频输入/输出设备
func (b *Bindings) ListAudioDevices() map[string]interface{} {
	inputDevs := audio.ListInputDevices()
	outputDevs := audio.ListOutputDevices()

	input := make([]AudioDeviceResponse, 0, len(inputDevs))
	for _, d := range inputDevs {
		input = append(input, AudioDeviceResponse{ID: d.ID, Name: d.Name})
	}

	output := make([]AudioDeviceResponse, 0, len(outputDevs))
	for _, d := range outputDevs {
		output = append(output, AudioDeviceResponse{ID: d.ID, Name: d.Name})
	}

	return map[string]interface{}{
		"input":  input,
		"output": output,
	}
}

// GetAudioDeviceConfig 获取当前音频设备配置
func (b *Bindings) GetAudioDeviceConfig() map[string]interface{} {
	cfg := config.Global()
	return map[string]interface{}{
		"input_device_id":  cfg.Audio.Devices.InputDeviceID,
		"output_device_id": cfg.Audio.Devices.OutputDeviceID,
	}
}

// SaveAudioDeviceConfig 保存音频设备配置
//
// inputDeviceID / outputDeviceID:
//   - "default" 或 "" → 跟随 Windows 系统默认设备
//   - 其他 → 设备名称（精确匹配）
//
// 输入设备在下次 Start() 时生效。
// 输出设备当前仅保存配置，实际播放设备在 Phase 2（waveOut 替代 Oto）后生效。
func (b *Bindings) SaveAudioDeviceConfig(inputDeviceID string, outputDeviceID string) error {
	log.Debugf("SaveAudioDeviceConfig: input=%q, output=%q", inputDeviceID, outputDeviceID)

	cfg := config.Global()

	// 规范化：空字符串统一为 "default"
	if inputDeviceID == "" {
		inputDeviceID = "default"
	}
	if outputDeviceID == "" {
		outputDeviceID = "default"
	}

	cfg.Audio.Devices.InputDeviceID = inputDeviceID
	cfg.Audio.Devices.OutputDeviceID = outputDeviceID

	if err := config.Save(); err != nil {
		log.Errorf("SaveAudioDeviceConfig: failed to save: %v", err)
		return err
	}

	log.Debugf("SaveAudioDeviceConfig: saved (input device takes effect on next recording start)")
	return nil
}
