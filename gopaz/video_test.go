package main

import (
	"os/exec"
	"path/filepath"
	"testing"
)

// TestOrientedSize checks that quarter-turn rotations swap display dimensions.
func TestOrientedSize(t *testing.T) {
	cases := []struct {
		rotation int
		w, h     int
	}{
		{0, 1920, 1080},
		{90, 1080, 1920},
		{180, 1920, 1080},
		{270, 1080, 1920},
		{-90, 1080, 1920},
		{360, 1920, 1080},
		{450, 1080, 1920},
	}
	for _, c := range cases {
		w, h := orientedSize(1920, 1080, c.rotation)
		if w != c.w || h != c.h {
			t.Fatalf("orientedSize(1920, 1080, %d) = %dx%d, want %dx%d", c.rotation, w, h, c.w, c.h)
		}
	}
}

// TestVideoDecoderStderr checks that ffmpeg diagnostics reach the error path.
func TestVideoDecoderStderr(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not found")
	}
	d, err := NewVideoDecoder(ffmpeg, filepath.Join(t.TempDir(), "missing.mp4"), 64, 64, -1, -1, "sw")
	if err != nil {
		t.Fatalf("start decoder: %v", err)
	}
	_ = d.ReadFrame(make([]byte, 64*64*3))
	d.Close()
	if d.Stderr() == "" {
		t.Fatal("expected ffmpeg stderr to be captured")
	}
}
