package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// AppState holds the persisted UI selections.
type AppState struct {
	Lang           string             `json:"lang"`
	Input          string             `json:"input"`
	OutputDir      string             `json:"outputDir"`
	OutputName     string             `json:"outputName"`
	CustomName     bool               `json:"customName"`
	Mode           string             `json:"mode"`
	Model          string             `json:"model"`
	Scale          int                `json:"scale"`
	Factor         int                `json:"factor"`
	Params         map[string]float64 `json:"params"`
	Provider       string             `json:"provider"`
	Device         string             `json:"device"`
	VideoDevice    string             `json:"videoDevice"`
	GPU            int                `json:"gpu"`
	Encoder        string             `json:"encoder"`
	Container      string             `json:"container"`
	Quality        string             `json:"quality"`
	RateControl    string             `json:"rateControl"`
	Bitrate        int                `json:"bitrate"`
	QP             int                `json:"qp"`
	AudioCodec     string             `json:"audioCodec"`
	AudioBitrate   int                `json:"audioBitrate"`
	Threads        int                `json:"threads"`
	Workers        int                `json:"workers"`
	VramGB         float64            `json:"vram"`
	DeviceVRAMGB   float64            `json:"gpuVram"`
	Overlap        int                `json:"overlap"`
	KColor         int                `json:"kcolor"`
	SceneThreshold float64            `json:"sceneThreshold"`
	StartFrame     int                `json:"startFrame"`
	EndFrame       int                `json:"endFrame"`
	Instances      int                `json:"instances"`
	Sensitivity    int                `json:"sensitivity"`
	RDT            float64            `json:"rdt"`
	LogHeight      int                `json:"logHeight"`
}

// AppConfig is the persistent application configuration stored in config.json
// next to the executable.
type AppConfig struct {
	SaveState        bool     `json:"saveState"`
	RestoreVideo     bool     `json:"restoreVideo"`
	PreviewDuration  float64  `json:"previewDuration"`
	Port             int      `json:"port"`
	State            AppState `json:"state"`
}

var appCfgMu sync.Mutex

// defaultAppConfig returns the first-run configuration.
func defaultAppConfig() AppConfig {
	return AppConfig{
		SaveState:       true,
		RestoreVideo:    true,
		PreviewDuration: 2,
		Port:            8765,
		State: AppState{
			Lang: "en", Model: "proteus", Mode: "enhance",
			Scale: 2, Factor: 2, Params: map[string]float64{},
			Provider: "cuda", GPU: -2,
			Quality: "High", RateControl: "cq", Bitrate: 20,
			AudioCodec: "copy", AudioBitrate: 320,
			SceneThreshold: 6.5, StartFrame: -1, EndFrame: -1,
			Sensitivity: 10,
		},
	}
}

// appConfigPath returns the config.json location next to the executable.
// GOPAZ_CONFIG overrides the location.
func appConfigPath() string {
	if p := strings.TrimSpace(os.Getenv("GOPAZ_CONFIG")); p != "" {
		return p
	}
	if exe, err := os.Executable(); err == nil {
		return filepath.Join(filepath.Dir(exe), "config.json")
	}
	return "config.json"
}

// loadAppConfig reads config.json and writes it with defaults on first run.
func loadAppConfig() AppConfig {
	appCfgMu.Lock()
	defer appCfgMu.Unlock()
	cfg := defaultAppConfig()
	path := appConfigPath()
	b, err := os.ReadFile(path)
	if err == nil {
		_ = json.Unmarshal(b, &cfg)
		return cfg
	}
	writeAppConfigLocked(cfg)
	return cfg
}

// saveAppState persists the UI state when saving is enabled.
func saveAppState(st AppState) {
	appCfgMu.Lock()
	defer appCfgMu.Unlock()
	cfg := defaultAppConfig()
	if b, err := os.ReadFile(appConfigPath()); err == nil {
		_ = json.Unmarshal(b, &cfg)
	}
	if !cfg.SaveState {
		return
	}
	cfg.State = st
	writeAppConfigLocked(cfg)
}

func writeAppConfigLocked(cfg AppConfig) {
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(appConfigPath(), append(b, '\n'), 0o644)
}
