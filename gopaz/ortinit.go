package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// providerRuntimeDirs returns the runtime subdirectories a provider needs in
// the modular layout. CUDA, TensorRT and CPU share the CUDA ORT build.
func providerRuntimeDirs(provider Provider) []string {
	nvidia := filepath.Join("runtime", "NVIDIA")
	switch provider {
	case ProviderCUDA, ProviderCPU:
		return []string{filepath.Join(nvidia, "CUDA"), filepath.Join(nvidia, "Compute")}
	case ProviderTensorRT:
		return []string{
			filepath.Join(nvidia, "CUDA"),
			filepath.Join(nvidia, "TensorRT"),
			filepath.Join(nvidia, "TensorRT", "tensorrt_libs"),
			filepath.Join(nvidia, "Compute"),
		}
	case ProviderDML:
		return []string{filepath.Join("runtime", "DirectML")}
	case ProviderOpenVINO:
		return []string{filepath.Join("runtime", "OpenVINO"), filepath.Join("runtime", "OpenVINO", "libs")}
	}
	return nil
}

// ortDllCandidates returns onnxruntime.dll lookup paths for a provider.
// The CPU provider accepts any bundled ORT build.
func ortDllCandidates(provider Provider) []string {
	var cands []string
	if v := os.Getenv("GOPAZ_ORT_DLL"); v != "" {
		cands = append(cands, v)
	}
	var bases []string
	if exe, err := os.Executable(); err == nil {
		bases = append(bases, filepath.Dir(exe))
	}
	if wd, err := os.Getwd(); err == nil {
		bases = append(bases, wd)
	}
	for _, base := range bases {
		for _, sub := range providerRuntimeDirs(provider) {
			cands = append(cands, filepath.Join(base, sub, "onnxruntime.dll"))
		}
		if provider == ProviderCPU {
			for _, p := range []Provider{ProviderDML, ProviderOpenVINO} {
				for _, sub := range providerRuntimeDirs(p) {
					cands = append(cands, filepath.Join(base, sub, "onnxruntime.dll"))
				}
			}
		}
		if provider != ProviderCUDA && provider != ProviderCPU {
			cands = append(cands, filepath.Join(base, "ort-dll", string(provider), "onnxruntime.dll"))
		}
		cands = append(cands, filepath.Join(base, "ort-dll", "onnxruntime.dll"), filepath.Join(base, "onnxruntime.dll"))
	}
	return cands
}

// ProviderAvailable reports whether the runtime files of a provider are present.
// Bundled runtime folders and system-installed libraries both count.
func ProviderAvailable(provider Provider) bool {
	if strings.TrimSpace(os.Getenv("GOPAZ_ORT_DLL")) != "" {
		return true
	}
	if provider == ProviderTensorRT && !hasTensorRTRuntime() {
		return false
	}
	return providerORTPresent(provider)
}

// runtimeBases returns the directories searched for bundled runtime files.
func runtimeBases() []string {
	var bases []string
	if exe, err := os.Executable(); err == nil {
		bases = append(bases, filepath.Dir(exe))
	}
	if wd, err := os.Getwd(); err == nil {
		bases = append(bases, wd)
	}
	return bases
}

// providerORTPresent reports whether an onnxruntime build for the provider is installed.
func providerORTPresent(provider Provider) bool {
	subs := providerRuntimeDirs(provider)
	if provider == ProviderCPU {
		subs = append(subs, providerRuntimeDirs(ProviderDML)...)
		subs = append(subs, providerRuntimeDirs(ProviderOpenVINO)...)
	}
	if provider != ProviderCUDA && provider != ProviderCPU {
		subs = append(subs, filepath.Join("ort-dll", string(provider)))
	}
	if provider == ProviderCUDA || provider == ProviderCPU {
		subs = append(subs, "ort-dll", ".")
	}
	for _, base := range runtimeBases() {
		for _, sub := range subs {
			if fileExists(filepath.Join(base, sub, "onnxruntime.dll")) {
				return true
			}
		}
	}
	return false
}

// hasTensorRTRuntime reports whether the TensorRT libraries are installed.
func hasTensorRTRuntime() bool {
	for _, base := range runtimeBases() {
		for _, sub := range []string{
			filepath.Join("runtime", "NVIDIA", "TensorRT", "tensorrt_libs"),
			filepath.Join("ort-dll", "tensorrt", "tensorrt_libs"),
		} {
			if fileExists(filepath.Join(base, sub, "nvinfer_10.dll")) || fileExists(filepath.Join(base, sub, "nvinfer.dll")) {
				return true
			}
		}
	}
	if _, err := exec.LookPath("nvinfer_10.dll"); err == nil {
		return true
	}
	if _, err := exec.LookPath("nvinfer.dll"); err == nil {
		return true
	}
	return false
}

// initORT locates and initializes the onnxruntime library for a provider.
func initORT(provider Provider) error {
	exeDir := ""
	if exe, err := os.Executable(); err == nil {
		exeDir = filepath.Dir(exe)
	}
	paths := providerPaths(provider, exeDir)
	if len(paths) > 0 {
		os.Setenv("PATH", strings.Join(paths, string(os.PathListSeparator))+string(os.PathListSeparator)+os.Getenv("PATH"))
	}
	for _, c := range ortDllCandidates(provider) {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			if err := Init(c, providerAPIVersion(provider)); err != nil {
				return fmt.Errorf("failed to initialize onnxruntime for %s (%s): %w", provider, c, err)
			}
			Debugf("onnxruntime: %s (%s)", c, provider)
			return nil
		}
	}
	return fmt.Errorf("onnxruntime.dll for %s not found under %s or GOPAZ_ORT_DLL", provider, filepath.Join(exeDir, "runtime"))
}

// providerPaths returns the directories that must be searchable for provider DLLs.
func providerPaths(provider Provider, exeDir string) []string {
	var paths []string
	add := func(p string) {
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			paths = append(paths, p)
		}
	}
	if exeDir != "" {
		for _, sub := range providerRuntimeDirs(provider) {
			add(filepath.Join(exeDir, sub))
		}
		for _, sub := range []string{"runtime", "cuda", "cudnn"} {
			add(filepath.Join(exeDir, sub))
		}
		if provider != ProviderCUDA && provider != ProviderCPU {
			base := filepath.Join(exeDir, "ort-dll", string(provider))
			for _, sub := range []string{"", "tensorrt_libs", "libs"} {
				add(filepath.Join(base, sub))
			}
		}
		paths = append(paths, exeDir)
	}
	return paths
}
