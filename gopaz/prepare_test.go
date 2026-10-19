package main

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeFakeTZ(t *testing.T, path string, payload []byte) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	w, err := zw.CreateHeader(&zip.FileHeader{Name: "model.onnx", Method: zip.Store})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

// TestPrepareModels checks extraction, skipping, and regeneration.
func TestPrepareModels(t *testing.T) {
	dir := t.TempDir()
	tzPath := filepath.Join(dir, "model.tz")
	onnxPath := filepath.Join(dir, "model.onnx")
	first := []byte("first model")
	second := []byte("second model")
	writeFakeTZ(t, tzPath, first)

	prepared, skipped, err := prepareModels(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if prepared != 1 || skipped != 0 {
		t.Fatalf("first run: prepared=%d skipped=%d", prepared, skipped)
	}
	if data, err := os.ReadFile(onnxPath); err != nil || !bytes.Equal(data, first) {
		t.Fatalf("first ONNX = %q, %v", data, err)
	}

	prepared, skipped, err = prepareModels(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if prepared != 0 || skipped != 1 {
		t.Fatalf("second run: prepared=%d skipped=%d", prepared, skipped)
	}

	writeFakeTZ(t, tzPath, second)
	if err := os.Chtimes(tzPath, time.Now().Add(2*time.Second), time.Now().Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	prepared, skipped, err = prepareModels(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if prepared != 1 || skipped != 0 {
		t.Fatalf("updated run: prepared=%d skipped=%d", prepared, skipped)
	}
	if data, err := os.ReadFile(onnxPath); err != nil || !bytes.Equal(data, second) {
		t.Fatalf("updated ONNX = %q, %v", data, err)
	}
}

// TestModelPathPrefersONNX checks the .onnx-first model lookup.
func TestModelPathPrefersONNX(t *testing.T) {
	dir := t.TempDir()
	tzPath := filepath.Join(dir, "model.tz")
	onnxPath := filepath.Join(dir, "model.onnx")
	if err := os.WriteFile(tzPath, []byte("tz"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := modelPathNamed(dir, "model.tz"); err != nil || got != tzPath {
		t.Fatalf("tz lookup = %q, %v", got, err)
	}
	if err := os.WriteFile(onnxPath, []byte("onnx"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := modelPathNamed(dir, "model.tz"); err != nil || got != onnxPath {
		t.Fatalf("onnx lookup = %q, %v", got, err)
	}
	if err := os.WriteFile(onnxPath, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := modelPathNamed(dir, "model.tz"); err != nil || got != tzPath {
		t.Fatalf("empty onnx lookup = %q, %v", got, err)
	}
}

// TestReadModel checks both prepared ONNX and packed TZ input.
func TestReadModel(t *testing.T) {
	dir := t.TempDir()
	payload := []byte("onnx payload")
	onnxPath := filepath.Join(dir, "model.onnx")
	tzPath := filepath.Join(dir, "model.tz")
	if err := os.WriteFile(onnxPath, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	writeFakeTZ(t, tzPath, payload)
	for _, path := range []string{onnxPath, tzPath} {
		data, err := readModel(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(data, payload) {
			t.Fatalf("%s: got %q", path, data)
		}
	}
}
