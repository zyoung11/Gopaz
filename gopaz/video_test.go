package main

import "testing"

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
