package pipeline

import (
	"math"
	"testing"
)

func makePattern(w, h int, phase float64) *Frame {
	f := NewFrame(w, h)
	for j := range h {
		for i := range w {
			idx := (j*w + i) * 3
			x := float64(i) / float64(w)
			y := float64(j) / float64(h)
			f.Data[idx] = float32(0.5 + 0.2*math.Sin(6*x+phase))
			f.Data[idx+1] = float32(0.5 + 0.18*math.Sin(5*y+phase+1))
			f.Data[idx+2] = float32(0.45 + 0.15*math.Sin(4*(x+y)+phase+2))
		}
	}
	return f
}

// TestColorTransferLabMatchesStats checks that output Lab statistics match the input.
func TestColorTransferLabMatchesStats(t *testing.T) {
	src := makePattern(64, 48, 0.3)
	dst := makePattern(80, 60, 1.7)
	ColorTransferLab(src, dst)
	srcMean, srcStd := labStats(src)
	dstMean, dstStd := labStats(dst)
	tols := [3]float64{0.05, 0.25, 0.25}
	for k := range 3 {
		if math.Abs(dstMean[k]-srcMean[k]) > tols[k] {
			t.Fatalf("mean[%d] = %.4f, want %.4f", k, dstMean[k], srcMean[k])
		}
		if math.Abs(dstStd[k]-srcStd[k]) > tols[k] {
			t.Fatalf("std[%d] = %.4f, want %.4f", k, dstStd[k], srcStd[k])
		}
	}
}

// TestColorTransferLabFlat checks that flat frames cause no division by zero or overflow.
func TestColorTransferLabFlat(t *testing.T) {
	src := NewFrame(8, 8)
	dst := NewFrame(8, 8)
	for i := 0; i < len(src.Data); i += 3 {
		src.Data[i], src.Data[i+1], src.Data[i+2] = 0.3, 0.4, 0.5
		dst.Data[i], dst.Data[i+1], dst.Data[i+2] = 0.6, 0.5, 0.4
	}
	ColorTransferLab(src, dst)
	for i := 0; i < len(dst.Data); i += 3 {
		if math.Abs(float64(dst.Data[i])-0.3) > 0.02 ||
			math.Abs(float64(dst.Data[i+1])-0.4) > 0.02 ||
			math.Abs(float64(dst.Data[i+2])-0.5) > 0.02 {
			t.Fatalf("pixel %d = %.4f/%.4f/%.4f, want 0.3/0.4/0.5",
				i, dst.Data[i], dst.Data[i+1], dst.Data[i+2])
		}
	}
}
