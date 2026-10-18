package pipeline

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopaz/internal/log"
	"gopaz/internal/ort"
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
			if err := ort.Init(c); err != nil {
				return fmt.Errorf("初始化 onnxruntime (%s) 失败: %w", c, err)
			}
			log.Debugf("onnxruntime: %s", c)
			return nil
		}
	}
	return fmt.Errorf("找不到 onnxruntime.dll，可用 GOPAZ_ORT_DLL 指定（需 onnxruntime 1.31+）")
}
