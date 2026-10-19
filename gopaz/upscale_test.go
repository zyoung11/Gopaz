package main

import "testing"

// TestUpscaleRunnerConfig checks that CLI options reach the runner configuration.
func TestUpscaleRunnerConfig(t *testing.T) {
	m, err := GetModel("proteus")
	if err != nil {
		t.Fatal(err)
	}
	job := UpscaleJob{
		Model: m, ModelDir: "models", Scale: 2, Params: map[string]float64{"denoise": 10},
		GPU: 1, Threads: 3, Workers: 5, VramGB: 6.5, Overlap: 48,
		SceneThreshold: 7,
	}
	cfg := job.runnerConfig(1920, 1080)
	if cfg.VramGB != 6.5 {
		t.Fatalf("VramGB = %v, want 6.5", cfg.VramGB)
	}
	if cfg.FW != 1920 || cfg.FH != 1080 {
		t.Fatalf("frame size = %dx%d, want 1920x1080", cfg.FW, cfg.FH)
	}
	if cfg.Threads != 3 || cfg.Workers != 5 || cfg.Overlap != 48 || cfg.GPU != 1 {
		t.Fatalf("runner options were not forwarded: %+v", cfg)
	}
	if cfg.SceneThreshold != 7 || cfg.Model != m {
		t.Fatalf("model options were not forwarded: %+v", cfg)
	}
}
