package main

import "testing"

// TestApplyCUDNNAlgoFromEnv checks the cuDNN algorithm override values.
func TestApplyCUDNNAlgoFromEnv(t *testing.T) {
	cases := []struct {
		value string
		want  int
		bad   bool
	}{
		{"", 0, false},
		{"exhaustive", 0, false},
		{"heuristic", 1, false},
		{"DEFAULT", 2, false},
		{"invalid", 0, true},
	}
	for _, c := range cases {
		t.Setenv("GOPAZ_CUDNN_ALGO", c.value)
		so := DefaultSessionOptions()
		err := applyCUDNNAlgoFromEnv(&so)
		if c.bad {
			if err == nil {
				t.Fatalf("%q: expected an error", c.value)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%q: %v", c.value, err)
		}
		if so.CUDNNAlgo != c.want {
			t.Fatalf("%q: CUDNNAlgo = %d, want %d", c.value, so.CUDNNAlgo, c.want)
		}
	}
}

// TestOpenVINOProviderOptions checks OpenVINO precision and cache options.
func TestOpenVINOProviderOptions(t *testing.T) {
	vals := openVINOProviderOptions("", "/cache/ov")
	if vals["device_type"] != "GPU.0" {
		t.Fatalf("default device = %q", vals["device_type"])
	}
	if vals["precision"] != "FP16" {
		t.Fatalf("precision = %q", vals["precision"])
	}
	if vals["cache_dir"] != "/cache/ov" {
		t.Fatalf("cache_dir = %q", vals["cache_dir"])
	}
	vals = openVINOProviderOptions("NPU", "")
	if vals["device_type"] != "NPU" {
		t.Fatalf("device = %q", vals["device_type"])
	}
	if vals["precision"] != "FP16" {
		t.Fatalf("NPU precision = %q", vals["precision"])
	}
	if _, ok := vals["cache_dir"]; ok {
		t.Fatal("empty cache dir should be omitted")
	}
	vals = openVINOProviderOptions("CPU", "")
	if _, ok := vals["precision"]; ok {
		t.Fatal("CPU should not set inference precision")
	}
}
