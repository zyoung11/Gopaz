package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// initORT locates and initializes onnxruntime.dll. The search order is:
//  1. the GOPAZ_ORT_DLL environment variable
//  2. ort-dll/onnxruntime.dll next to the executable
//  3. ort-dll/onnxruntime.dll in the current working directory
//
// Before loading, the runtime, cuda, cudnn, and program directories next to the
// executable are prepended to PATH so a portable layout needs no launcher script.
func initORT() error {
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		paths := make([]string, 0, 4)
		for _, sub := range []string{"runtime", "cuda", "cudnn"} {
			p := filepath.Join(dir, sub)
			if st, err := os.Stat(p); err == nil && st.IsDir() {
				paths = append(paths, p)
			}
		}
		paths = append(paths, dir)
		os.Setenv("PATH", strings.Join(paths, string(os.PathListSeparator))+string(os.PathListSeparator)+os.Getenv("PATH"))
	}
	cands := []string{}
	if v := os.Getenv("GOPAZ_ORT_DLL"); v != "" {
		cands = append(cands, v)
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		cands = append(cands,
			filepath.Join(dir, "ort-dll", "onnxruntime.dll"),
			filepath.Join(dir, "onnxruntime.dll"),
		)
	}
	if wd, err := os.Getwd(); err == nil {
		cands = append(cands, filepath.Join(wd, "ort-dll", "onnxruntime.dll"))
	}
	for _, c := range cands {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			if err := Init(c); err != nil {
				return fmt.Errorf("failed to initialize onnxruntime (%s): %w", c, err)
			}
			Debugf("onnxruntime: %s", c)
			return nil
		}
	}
	return fmt.Errorf("onnxruntime.dll not found; set GOPAZ_ORT_DLL (requires onnxruntime 1.31+)")
}
