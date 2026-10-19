package main

import (
	"slices"
	"testing"
)

// TestAudioTrimArgs checks the input-side trim options for the audio pass.
func TestAudioTrimArgs(t *testing.T) {
	cases := []struct {
		name     string
		start    float64
		duration float64
		want     []string
	}{
		{"none", 0, 0, nil},
		{"start", 2.5, 0, []string{"-ss", "2.500000"}},
		{"duration", 0, 3, []string{"-t", "3.000000"}},
		{"both", 1.25, 2.5, []string{"-ss", "1.250000", "-t", "2.500000"}},
	}
	for _, c := range cases {
		got := audioTrimArgs(c.start, c.duration)
		if !slices.Equal(got, c.want) {
			t.Fatalf("%s: audioTrimArgs(%v, %v) = %v, want %v", c.name, c.start, c.duration, got, c.want)
		}
	}
}

// TestSourceAudioTrim checks the mapping from a frame range to source seconds.
func TestSourceAudioTrim(t *testing.T) {
	info := &VideoInfo{FPS: 30}
	fr := FrameRange{Start: 15, End: 44, Count: 30}

	start, duration := sourceAudioTrim(info, fr, 30, true)
	if start != 0.5 || duration != 1 {
		t.Fatalf("trim = %v, %v; want 0.5, 1", start, duration)
	}
	if start, duration := sourceAudioTrim(info, fr, 30, false); start != 0 || duration != 0 {
		t.Fatalf("untrimmed range = %v, %v; want 0, 0", start, duration)
	}
	if start, duration := sourceAudioTrim(&VideoInfo{FPS: 0}, fr, 30, true); start != 0 || duration != 0 {
		t.Fatalf("unknown fps = %v, %v; want 0, 0", start, duration)
	}
	if start, duration := sourceAudioTrim(info, FrameRange{Start: -1}, 30, true); start != 0 || duration != 0 {
		t.Fatalf("unresolved start = %v, %v; want 0, 0", start, duration)
	}
	if start, duration := sourceAudioTrim(info, fr, 0, true); start != 0.5 || duration != 0 {
		t.Fatalf("unknown total = %v, %v; want 0.5, 0", start, duration)
	}
}
