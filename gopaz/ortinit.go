package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

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

	cands := []string{}
	if v := os.Getenv("GOPAZ_ORT_DLL"); v != "" {
		cands = append(cands, v)
	}
	if exeDir != "" {
		if provider != ProviderCUDA && provider != ProviderCPU {
			cands = append(cands, filepath.Join(exeDir, "ort-dll", string(provider), "onnxruntime.dll"))
		}
		cands = append(cands, filepath.Join(exeDir, "ort-dll", "onnxruntime.dll"), filepath.Join(exeDir, "onnxruntime.dll"))
	}
	if wd, err := os.Getwd(); err == nil {
		if provider != ProviderCUDA && provider != ProviderCPU {
			cands = append(cands, filepath.Join(wd, "ort-dll", string(provider), "onnxruntime.dll"))
		}
		cands = append(cands, filepath.Join(wd, "ort-dll", "onnxruntime.dll"))
	}
	for _, c := range cands {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			if err := Init(c, providerAPIVersion(provider)); err != nil {
				return fmt.Errorf("failed to initialize onnxruntime for %s (%s): %w", provider, c, err)
			}
			Debugf("onnxruntime: %s (%s)", c, provider)
			return nil
		}
	}
	expected := filepath.Join(exeDir, "ort-dll", string(provider), "onnxruntime.dll")
	return fmt.Errorf("onnxruntime.dll for %s not found; expected %s or GOPAZ_ORT_DLL", provider, expected)
}

// providerPaths returns the directories that must be searchable for provider DLLs.
func providerPaths(provider Provider, exeDir string) []string {
	var paths []string
	if exeDir != "" {
		for _, sub := range []string{"runtime", "cuda", "cudnn"} {
			p := filepath.Join(exeDir, sub)
			if st, err := os.Stat(p); err == nil && st.IsDir() {
				paths = append(paths, p)
			}
		}
		if provider != ProviderCUDA && provider != ProviderCPU {
			base := filepath.Join(exeDir, "ort-dll", string(provider))
			for _, sub := range []string{"", "tensorrt_libs", "libs"} {
				p := filepath.Join(base, sub)
				if st, err := os.Stat(p); err == nil && st.IsDir() {
					paths = append(paths, p)
				}
			}
		}
		paths = append(paths, exeDir)
	}
	return paths
}
