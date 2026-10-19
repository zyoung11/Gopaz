package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// cmdPrepare converts encrypted .tz models into prepared .onnx files.
func cmdPrepare(args []string) error {
	fs := flag.NewFlagSet("prepare", flag.ExitOnError)
	dir := fs.String("model-dir", "", "模型目录")
	force := fs.Bool("force", false, "覆盖已有的 .onnx")
	fs.Parse(args)

	modelDir := *dir
	if modelDir == "" {
		modelDir = resolveModelDir()
	}
	prepared, skipped, err := prepareModels(modelDir, *force)
	if err != nil {
		return err
	}
	if prepared == 0 && skipped == 0 {
		return fmt.Errorf("no .tz models found in %s", modelDir)
	}
	Successf("Prepared %d models, skipped %d", prepared, skipped)
	return nil
}

// prepareModels writes a plain .onnx next to every .tz file.
func prepareModels(modelDir string, force bool) (int, int, error) {
	paths, err := filepath.Glob(filepath.Join(modelDir, "*.tz"))
	if err != nil {
		return 0, 0, err
	}
	sort.Strings(paths)
	prepared, skipped := 0, 0
	for _, tzPath := range paths {
		target := strings.TrimSuffix(tzPath, ".tz") + ".onnx"
		if !force {
			targetInfo, targetErr := os.Stat(target)
			sourceInfo, sourceErr := os.Stat(tzPath)
			if targetErr == nil && targetInfo.Size() > 0 && sourceErr == nil && !sourceInfo.ModTime().After(targetInfo.ModTime()) {
				skipped++
				continue
			}
		}
		Infof("Preparing %s", filepath.Base(tzPath))
		data, err := os.ReadFile(tzPath)
		if err != nil {
			return prepared, skipped, err
		}
		raw, err := Extract(data, Password)
		if err != nil {
			return prepared, skipped, fmt.Errorf("%s: %w", filepath.Base(tzPath), err)
		}
		if err := writeFileAtomic(target, raw); err != nil {
			return prepared, skipped, err
		}
		prepared++
	}
	return prepared, skipped, nil
}

// writeFileAtomic writes data through a temporary file and renames it into place.
func writeFileAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
