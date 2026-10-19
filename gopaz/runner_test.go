package main

import (
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
