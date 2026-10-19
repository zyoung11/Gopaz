package main

import (
	"strings"
	"testing"
)

func TestFormatVRAM(t *testing.T) {
	cases := []struct {
		bytes uint64
		want  string
	}{
		{0, ""},
		{1024, ""},
		{512 * 1024 * 1024, "512MB"},
		{100 * 1024 * 1024, "100MB"},
		{4 * 1024 * 1024 * 1024, "4GB"},
		{6 * 1024 * 1024 * 1024, "6GB"},
		{8 * 1024 * 1024 * 1024, "8GB"},
		{12 * 1024 * 1024 * 1024, "12GB"},
		{24 * 1024 * 1024 * 1024, "24GB"},
		{5905580032, "5.5GB"},
		{4293918720, "4GB"},
	}
	for _, c := range cases {
		if got := formatVRAM(c.bytes); got != c.want {
			t.Errorf("formatVRAM(%d) = %q, want %q", c.bytes, got, c.want)
		}
	}
}

func TestCudaDeviceDisplay(t *testing.T) {
	cases := []struct {
		name  string
		bytes uint64
		want  string
	}{
		{"NVIDIA GeForce RTX 4090", 24 * 1024 * 1024 * 1024, "NVIDIA GeForce RTX 4090 (24GB)"},
		{"NVIDIA GeForce GTX 1080", 0, "NVIDIA GeForce GTX 1080"},
		{"", 8 * 1024 * 1024 * 1024, "CUDA device (8GB)"},
		{"  NVIDIA A100  ", 40 * 1024 * 1024 * 1024, "NVIDIA A100 (40GB)"},
		{"", 0, "CUDA device"},
	}
	for _, c := range cases {
		if got := cudaDeviceDisplay(c.name, c.bytes); got != c.want {
			t.Errorf("cudaDeviceDisplay(%q, %d) = %q, want %q", c.name, c.bytes, got, c.want)
		}
	}
}

func TestOpenVINODisplay(t *testing.T) {
	cases := []struct {
		id, name, want string
	}{
		{"GPU.0", "Intel(R) Arc(TM) Graphics", "Intel(R) Arc(TM) Graphics (GPU.0)"},
		{"NPU", "Intel(R) NPU", "Intel(R) NPU (NPU)"},
		{"CPU", "13th Gen Intel(R) Core(TM) i7-13700", "CPU"},
		{"cpu", "13th Gen Intel(R) Core(TM) i7-13700", "CPU"},
		{"GPU.1", "", "GPU.1"},
		{"GPU.0", "   ", "GPU.0"},
	}
	for _, c := range cases {
		if got := openVINODisplay(c.id, c.name); got != c.want {
			t.Errorf("openVINODisplay(%q, %q) = %q, want %q", c.id, c.name, got, c.want)
		}
	}
}

func TestOpenVINOIsIntelGPU(t *testing.T) {
	cases := []struct {
		arch, name string
		want       bool
	}{
		{"GPU: vendor=0x8086 arch=v12.74.4", "Intel(R) Arc(TM) Graphics (iGPU)", true},
		{"GPU: vendor=0x10de arch=v12.0.0", "NVIDIA GeForce RTX 5090 Laptop GPU (dGPU)", false},
		{"GPU: vendor=0x1022 arch=v12.0.0", "AMD Radeon Graphics", false},
		{"", "Intel(R) UHD Graphics", true},
		{"", "AMD Radeon Graphics", false},
	}
	for _, c := range cases {
		if got := openVINOIsIntelGPU(c.arch, c.name); got != c.want {
			t.Errorf("openVINOIsIntelGPU(%q, %q) = %v, want %v", c.arch, c.name, got, c.want)
		}
	}
}

func TestOpenVINOSort(t *testing.T) {
	devices := []deviceInfo{
		{ID: "CPU", Display: "CPU"},
		{ID: "NPU", Display: "Intel(R) AI Boost (NPU)"},
		{ID: "GPU.1", Display: "Second (GPU.1)"},
		{ID: "GPU.0", Display: "First (GPU.0)"},
	}
	openVINOSort(devices)
	order := []string{devices[0].ID, devices[1].ID, devices[2].ID, devices[3].ID}
	want := []string{"GPU.0", "GPU.1", "NPU", "CPU"}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("order = %v, want %v", order, want)
		}
	}
}

func TestDeviceEntriesValid(t *testing.T) {
	for _, devices := range [][]deviceInfo{cudaDevices(), dmlDevices(), openVINODevices()} {
		for _, d := range devices {
			if strings.TrimSpace(d.ID) == "" || strings.TrimSpace(d.Display) == "" {
				t.Errorf("empty device entry: %+v", d)
			}
			if strings.Contains(d.Display, "\x00") {
				t.Errorf("display contains null byte: %q", d.Display)
			}
		}
	}
}
