package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestAppConfigFirstRun(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("GOPAZ_CONFIG", path)
	cfg := loadAppConfig()
	if !cfg.SaveState || !cfg.RestoreVideo || cfg.Port != 8765 || cfg.PreviewDuration != 2 {
		t.Fatalf("defaults = %+v", cfg)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("config not created: %v", err)
	}
	var ondisk AppConfig
	if err := json.Unmarshal(b, &ondisk); err != nil {
		t.Fatal(err)
	}
	if ondisk.State.Model != "proteus" || ondisk.State.Provider != "cuda" || ondisk.State.Lang != "en" {
		t.Fatalf("state defaults = %+v", ondisk.State)
	}
}

func TestSaveAppStateRespectsFlag(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("GOPAZ_CONFIG", path)
	cfg := loadAppConfig()
	cfg.SaveState = false
	b, _ := json.MarshalIndent(cfg, "", "  ")
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
	saveAppState(AppState{Model: "apollo"})
	if got := loadAppConfig(); got.State.Model != "proteus" {
		t.Fatalf("state saved while disabled: %+v", got.State)
	}
	cfg.SaveState = true
	b, _ = json.MarshalIndent(cfg, "", "  ")
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
	saveAppState(AppState{Model: "apollo", Params: map[string]float64{"deblur": 5}})
	got := loadAppConfig()
	if got.State.Model != "apollo" || got.State.Params["deblur"] != 5 {
		t.Fatalf("state not saved: %+v", got.State)
	}
}
