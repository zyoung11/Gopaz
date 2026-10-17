package pipeline

import (
	"fmt"
	"os"
	"path/filepath"

	"tvai/internal/log"
	"tvai/internal/ort"
)

// initORT 定位并初始化 onnxruntime.dll。搜索顺序：
//  1. 环境变量 TVAI_ORT_DLL
//  2. 可执行文件同目录的 ort-dll/onnxruntime.dll
//  3. 当前工作目录的 ort-dll/onnxruntime.dll
func initORT() error {
	cands := []string{}
	if v := os.Getenv("TVAI_ORT_DLL"); v != "" {
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
	return fmt.Errorf("找不到 onnxruntime.dll，可用 TVAI_ORT_DLL 指定（需 onnxruntime 1.31+）")
}
