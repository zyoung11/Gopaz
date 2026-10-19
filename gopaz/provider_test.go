package main

import "testing"

// TestNormalizeProvider checks provider aliases and API versions.
func TestNormalizeProvider(t *testing.T) {
	cases := []struct {
		value string
		want  Provider
		bad   bool
	}{
		{"", ProviderCUDA, false},
		{"auto", ProviderCUDA, false},
		{"cuda", ProviderCUDA, false},
		{"tensorrt", ProviderTensorRT, false},
		{"trt", ProviderTensorRT, false},
		{"dml", ProviderDML, false},
		{"directml", ProviderDML, false},
		{"openvino", ProviderOpenVINO, false},
		{"ov", ProviderOpenVINO, false},
		{"cpu", ProviderCPU, false},
		{"unknown", "", true},
	}
	for _, c := range cases {
		got, err := normalizeProvider(c.value)
		if c.bad {
			if err == nil {
				t.Fatalf("%q: expected an error", c.value)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Fatalf("%q: got %q, %v", c.value, got, err)
		}
	}
	if providerAPIVersion(ProviderDML) != 24 || providerAPIVersion(ProviderOpenVINO) != 24 || providerAPIVersion(ProviderCUDA) != 31 {
		t.Fatal("unexpected provider API versions")
	}
}

// TestProviderDevices checks DirectML and OpenVINO device parsing.
func TestProviderDevices(t *testing.T) {
	if id, err := dmlDeviceID(""); err != nil || id != 0 {
		t.Fatalf("empty DML device = %d, %v", id, err)
	}
	if id, err := dmlDeviceID("1"); err != nil || id != 1 {
		t.Fatalf("DML device = %d, %v", id, err)
	}
	if _, err := dmlDeviceID("-1"); err == nil {
		t.Fatal("negative DML device should fail")
	}
	if dev, err := openVINODevice(""); err != nil || dev != "GPU.0" {
		t.Fatalf("default OpenVINO device = %q, %v", dev, err)
	}
	if dev, err := openVINODevice("npu"); err != nil || dev != "NPU" {
		t.Fatalf("OpenVINO NPU device = %q, %v", dev, err)
	}
	if _, err := openVINODevice("TPU"); err == nil {
		t.Fatal("unknown OpenVINO device should fail")
	}
}

// TestProviderSingleThreaded checks which providers require serial inference.
func TestProviderSingleThreaded(t *testing.T) {
	cases := []struct {
		provider Provider
		want     bool
	}{
		{ProviderCUDA, false},
		{ProviderTensorRT, true},
		{ProviderDML, true},
		{ProviderOpenVINO, true},
		{ProviderCPU, false},
	}
	for _, c := range cases {
		if got := providerSingleThreaded(c.provider); got != c.want {
			t.Fatalf("%s: got %v, want %v", c.provider, got, c.want)
		}
	}
}
