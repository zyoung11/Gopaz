package main

import (
	"slices"
	"testing"
)

// TestSoftwareEncoderArgs checks the libx264 fallback encoder used by previews.
func TestSoftwareEncoderArgs(t *testing.T) {
	e, err := GetEncoder("h264-main-sw")
	if err != nil {
		t.Fatal(err)
	}
	if e.HasCQP || e.RateControl {
		t.Fatalf("software encoder should not advertise NVENC rate control")
	}
	args, err := BuildArgs(EncodeOptions{EncoderID: e.ID, Quality: "High", RateControl: "cq"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(args, "libx264") {
		t.Fatalf("args missing libx264: %v", args)
	}
	if slices.Contains(args, "h264_nvenc") {
		t.Fatalf("args unexpectedly contain h264_nvenc: %v", args)
	}
}
