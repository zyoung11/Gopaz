package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestTRTCachePrefix checks engine cache prefixes are per model file.
func TestTRTCachePrefix(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"aion-v1-fp16-ox.tz", "aion-v1-fp16-ox"},
		{"aion-v1-fp16-ox.onnx", "aion-v1-fp16-ox"},
		{"apo-v8-fp16-1088x1056-ox.onnx", "apo-v8-fp16-1088x1056-ox"},
		{"model", "model"},
	}
	for _, c := range cases {
		if got := trtCachePrefix(filepath.Join("models", c.name)); got != c.want {
			t.Fatalf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
	a := trtCachePrefix(filepath.Join("models", "apo-v8-fp16-1088x1056-ox.onnx"))
	b := trtCachePrefix(filepath.Join("models", "apo-v8-fp16-384x672-ox.onnx"))
	if a == b {
		t.Fatalf("different tile models share prefix %q", a)
	}
}

// TestAionFreeDims checks fixed Aion dimensions for main and bottom models.
func TestAionFreeDims(t *testing.T) {
	main := aionFreeDims(3840, 576, true)
	want := map[string]int64{
		"bsize": 1, "fheight": 576, "fwidth": 3840,
		"fheight2": 144, "fwidth2": 960,
	}
	for k, v := range want {
		if main[k] != v {
			t.Fatalf("%s = %d, want %d", k, main[k], v)
		}
	}
	bottom := aionFreeDims(3840, 576, false)
	if _, ok := bottom["fheight2"]; ok {
		t.Fatal("bottom model should not set in8x dimensions")
	}
	if bottom["fheight"] != 576 || bottom["fwidth"] != 3840 {
		t.Fatalf("bottom dims = %v", bottom)
	}
}

// TestCacheRootOverride checks the cache directory environment override.
func TestCacheRootOverride(t *testing.T) {
	want := filepath.Join("portable", "cache")
	t.Setenv("GOPAZ_CACHE_DIR", want)
	if got := cacheRoot(); got != want {
		t.Fatalf("cacheRoot = %q, want %q", got, want)
	}
}

// TestCacheRootPortable checks that the cache lives next to the executable.
func TestCacheRootPortable(t *testing.T) {
	t.Setenv("GOPAZ_CACHE_DIR", "")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(filepath.Dir(exe), "cache")
	got := cacheRoot()
	if got != want {
		t.Fatalf("cacheRoot = %q, want %q", got, want)
	}
	if st, err := os.Stat(got); err != nil || !st.IsDir() {
		t.Fatalf("cache directory not created: %v", err)
	}
}

func TestTRTEngineCached(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GOPAZ_CACHE_DIR", root)
	dir := filepath.Join(root, "tensorrt")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	model := filepath.Join("models", "aion-v1-fp16-ox.onnx")
	if trtEngineCached(model) {
		t.Fatal("empty cache reported cached")
	}
	if err := os.WriteFile(filepath.Join(dir, "other_123_0_fp16_sm120.engine"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if trtEngineCached(model) {
		t.Fatal("unrelated engine reported cached")
	}
	engine := filepath.Join(dir, "aion-v1-fp16-ox_123_0_fp16_sm120.engine")
	if err := os.WriteFile(engine, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !trtEngineCached(model) {
		t.Fatal("cached engine not detected")
	}
	if err := os.WriteFile(engine, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if trtEngineCached(model) {
		t.Fatal("empty engine reported cached")
	}
}
