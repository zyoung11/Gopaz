package main

import (
	"os"
	"path/filepath"
	"testing"
)

func hasTile(tiles [][2]int, w, h int) bool {
	for _, t := range tiles {
		if t[0] == w && t[1] == h {
			return true
		}
	}
	return false
}

// TestModelDefaults checks model ranges, tile tables, and default overlap.
func TestModelDefaults(t *testing.T) {
	if len(ModelKeys()) != 5 {
		t.Fatalf("模型数量 = %d, want 5", len(ModelKeys()))
	}
	if !hasTile(ModelRegistry()["proteus"].TileHW, 672, 576) {
		t.Fatalf("proteus 缺少 672x576")
	}
	if !hasTile(ModelRegistry()["apollo"].TileHW, 1344, 1152) {
		t.Fatalf("apollo 缺少 1344x1152")
	}
	if got := ModelRegistry()["aion"].BlockOverlapOrDefault(); got != 32 {
		t.Fatalf("aion overlap = %d, want 32", got)
	}
	if got := ModelRegistry()["proteus"].BlockOverlapOrDefault(); got != 48 {
		t.Fatalf("proteus overlap = %d, want 48", got)
	}
}

// TestModelNPUFlags checks the OpenVINO NPU capability flags.
func TestModelNPUFlags(t *testing.T) {
	if !ModelRegistry()["aion"].NPUUnsupported {
		t.Fatal("aion should be NPUUnsupported")
	}
	for _, key := range []string{"proteus", "gaia", "gaia-cg", "apollo"} {
		if ModelRegistry()[key].NPUUnsupported {
			t.Fatalf("%s should run on the NPU", key)
		}
	}
}

// TestModelAvailable checks drop-in model folder detection.
func TestModelAvailable(t *testing.T) {
	dir := t.TempDir()
	m := ModelRegistry()["apollo"]
	if m.Available(dir) {
		t.Fatal("empty dir should not be available")
	}
	if err := os.MkdirAll(filepath.Join(dir, "apollo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if m.Available(dir) {
		t.Fatal("empty subdirectory should not be available")
	}
	p := filepath.Join(dir, "apollo", "apo-v8-fp16-576x672-ox.onnx")
	if err := os.WriteFile(p, []byte("onnx"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !m.Available(dir) {
		t.Fatal("subdirectory model should be available")
	}
	flat := t.TempDir()
	p = filepath.Join(flat, "apo-v8-fp16-576x672-ox.tz")
	if err := os.WriteFile(p, []byte("tz"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !m.Available(flat) {
		t.Fatal("flat layout model should be available")
	}
	aion := ModelRegistry()["aion"]
	if aion.Available(t.TempDir()) {
		t.Fatal("missing aion should not be available")
	}
}

// TestModelAutoFile checks the parameter estimation model wiring.
func TestModelAutoFile(t *testing.T) {
	if ModelRegistry()["proteus"].AutoFile == "" {
		t.Fatal("proteus should declare an auto parameter model")
	}
	for _, key := range []string{"gaia", "gaia-cg", "apollo", "aion"} {
		if ModelRegistry()[key].AutoFile != "" {
			t.Fatalf("%s should not declare an auto parameter model", key)
		}
	}
}

// TestProteusParamRanges checks that every Proteus parameter allows negative values.
func TestProteusParamRanges(t *testing.T) {
	for _, p := range ModelRegistry()["proteus"].Params {
		if p.Min != -1 || p.Max != 1 {
			t.Fatalf("%s range = %v..%v, want -1..1", p.Key, p.Min, p.Max)
		}
	}
}
