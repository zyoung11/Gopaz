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
