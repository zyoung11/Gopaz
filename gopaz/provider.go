package main

import (
	"fmt"
	"strconv"
	"strings"
)

// Provider identifies the ONNX Runtime execution provider.
type Provider string

const (
	ProviderAuto     Provider = "auto"
	ProviderCPU      Provider = "cpu"
	ProviderCUDA     Provider = "cuda"
	ProviderTensorRT Provider = "tensorrt"
	ProviderDML      Provider = "dml"
	ProviderOpenVINO Provider = "openvino"
)

// normalizeProvider validates a provider name and resolves auto.
func normalizeProvider(value string) (Provider, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "auto":
		return ProviderCUDA, nil
	case "cpu":
		return ProviderCPU, nil
	case "cuda":
		return ProviderCUDA, nil
	case "tensorrt", "trt":
		return ProviderTensorRT, nil
	case "dml", "directml":
		return ProviderDML, nil
	case "openvino", "ov":
		return ProviderOpenVINO, nil
	default:
		return "", fmt.Errorf("unknown provider %q; use cuda, tensorrt, dml, openvino, or cpu", value)
	}
}

// providerAPIVersion returns the ORT C API version supported by the provider runtime.
func providerAPIVersion(p Provider) uint32 {
	switch p {
	case ProviderDML, ProviderOpenVINO:
		return 24
	default:
		return 31
	}
}

// providerNeedsCUDA reports whether the provider uses the CUDA execution provider.
func providerNeedsCUDA(p Provider) bool {
	return p == ProviderCUDA || p == ProviderTensorRT
}

// providerSingleThreaded reports whether a provider requires serial inference.
func providerSingleThreaded(p Provider) bool {
	return p == ProviderTensorRT || p == ProviderDML
}

// providerDefaultWorkers returns the automatic worker count for a provider.
func providerDefaultWorkers(p Provider, kind Kind, device string) int {
	if p != ProviderOpenVINO {
		return 0
	}
	if strings.EqualFold(device, "NPU") || strings.EqualFold(device, "CPU") || kind == KindInterp {
		return 1
	}
	return 4
}

// dmlDeviceID parses a DirectML adapter index.
func dmlDeviceID(value string) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	id, err := strconv.Atoi(value)
	if err != nil || id < 0 {
		return 0, fmt.Errorf("invalid DirectML device %q", value)
	}
	return id, nil
}

// openVINODevice returns a validated OpenVINO device string.
func openVINODevice(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "GPU.0", nil
	}
	upper := strings.ToUpper(value)
	switch {
	case upper == "NPU", upper == "CPU", upper == "GPU", strings.HasPrefix(upper, "GPU."):
		return upper, nil
	default:
		return "", fmt.Errorf("invalid OpenVINO device %q; use GPU.0, GPU.1, NPU, or CPU", value)
	}
}
