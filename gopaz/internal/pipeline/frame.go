// Package pipeline implements the upscaling and frame interpolation pipelines.
package pipeline

import (
	"math"
	"runtime"
)

// Frame is one RGB frame with NHWC data in the range [0,1].
type Frame struct {
	W, H int
	// Data has W*H*3 interleaved RGB values.
	Data []float32
}

// NewFrame allocates a blank frame.
func NewFrame(w, h int) *Frame {
	return &Frame{W: w, H: h, Data: make([]float32, w*h*3)}
}

// FromRGB24 converts raw RGB24 bytes in 0..255 to a float frame in [0,1].
func FromRGB24(b []byte, w, h int) *Frame {
	f := &Frame{W: w, H: h, Data: make([]float32, w*h*3)}
	parallelRows(w*h, func(s, e int) {
		for i := s * 3; i < e*3; i++ {
			f.Data[i] = float32(b[i]) / 255.0
		}
	})
	return f
}

// ToRGB24 converts the frame back to RGB24 bytes, clamping out-of-range values.
func (f *Frame) ToRGB24() []byte {
	out := make([]byte, f.W*f.H*3)
	parallelRows(f.W*f.H, func(s, e int) {
		for i := s * 3; i < e*3; i++ {
			out[i] = toByte(f.Data[i])
		}
	})
	return out
}

// ToRGB24Norm normalizes by inv before converting to RGB24, avoiding an extra pass.
func (f *Frame) ToRGB24Norm(inv []float32) []byte {
	out := make([]byte, f.W*f.H*3)
	parallelRows(f.H, func(s, e int) {
		for y := s; y < e; y++ {
			base := y * f.W
			for x := 0; x < f.W; x++ {
				p := base + x
				w := inv[p]
				if w <= 0 {
					w = 1
				}
				i := p * 3
				out[i] = toByte(f.Data[i] * w)
				out[i+1] = toByte(f.Data[i+1] * w)
				out[i+2] = toByte(f.Data[i+2] * w)
			}
		}
	})
	return out
}

// Clone returns a deep copy of the frame.
func (f *Frame) Clone() *Frame {
	c := &Frame{W: f.W, H: f.H, Data: make([]float32, len(f.Data))}
	copy(c.Data, f.Data)
	return c
}

// ExtractTile extracts a tile in input frame space, replicating edge pixels when
// the coordinates fall outside the frame.
func (f *Frame) ExtractTile(x, y, tw, th int) *Frame {
	t := NewFrame(tw, th)
	parallelRows(th, func(js, je int) {
		for j := js; j < je; j++ {
			sy := clampInt(y+j, 0, f.H-1)
			for i := range tw {
				sx := clampInt(x+i, 0, f.W-1)
				si := (sy*f.W + sx) * 3
				di := (j*tw + i) * 3
				t.Data[di] = f.Data[si]
				t.Data[di+1] = f.Data[si+1]
				t.Data[di+2] = f.Data[si+2]
			}
		}
	})
	return t
}

// rampWeight computes the blend weight of one pixel inside a tile. Edges ramp
// linearly while the interior is 1. ramp is the ramp width in output pixels.
func rampWeight(i, j, w, h, ramp int) float32 {
	if ramp <= 0 {
		return 1
	}
	wx := float32(1)
	wy := float32(1)
	if i < ramp {
		wx = float32(i+1) / float32(ramp+1)
	} else if i >= w-ramp {
		wx = float32(w-i) / float32(ramp+1)
	}
	if j < ramp {
		wy = float32(j+1) / float32(ramp+1)
	} else if j >= h-ramp {
		wy = float32(h-j) / float32(ramp+1)
	}
	return wx * wy
}

// UpscaleBilinear scales the frame to nw x nh, used to initialize hr_prev.
func (f *Frame) UpscaleBilinear(nw, nh int) *Frame {
	out := NewFrame(nw, nh)
	sxRatio := float64(f.W) / float64(nw)
	syRatio := float64(f.H) / float64(nh)
	for j := range nh {
		sy := (float64(j)+0.5)*syRatio - 0.5
		y0 := clampInt(int(math.Floor(sy)), 0, f.H-1)
		y1 := clampInt(y0+1, 0, f.H-1)
		fy := sy - math.Floor(sy)
		if sy < 0 {
			fy = 0
		}
		for i := range nw {
			sx := (float64(i)+0.5)*sxRatio - 0.5
			x0 := clampInt(int(math.Floor(sx)), 0, f.W-1)
			x1 := clampInt(x0+1, 0, f.W-1)
			fx := sx - math.Floor(sx)
			if sx < 0 {
				fx = 0
			}
			for c := range 3 {
				v00 := f.Data[(y0*f.W+x0)*3+c]
				v01 := f.Data[(y0*f.W+x1)*3+c]
				v10 := f.Data[(y1*f.W+x0)*3+c]
				v11 := f.Data[(y1*f.W+x1)*3+c]
				ffx := float32(fx)
				ffy := float32(fy)
				top := v00*(1-ffx) + v01*ffx
				bot := v10*(1-ffx) + v11*ffx
				out.Data[(j*nw+i)*3+c] = top*(1-ffy) + bot*ffy
			}
		}
	}
	return out
}

// hasNaN reports whether the slice contains a NaN value.
func hasNaN(s []float32) bool {
	for _, v := range s {
		if math.IsNaN(float64(v)) {
			return true
		}
	}
	return false
}

// TilePenalty is the per-tile penalty used by the Topaz tile scoring.
const TilePenalty = 0.05

// chooseTile selects the tile for a fixed-tile model using the Topaz score.
func chooseTile(candidates [][2]int, fw, fh, overlap int, penalty float64) (int, int) {
	if len(candidates) == 0 {
		return fw, fh
	}
	area := float64(fw) * float64(fh)
	bestW, bestH := candidates[0][0], candidates[0][1]
	best := math.MaxFloat64
	found := false
	for _, c := range candidates {
		tw, th := c[0], c[1]
		if tw <= overlap || th <= overlap {
			continue
		}
		nx := ceilDiv(fw-overlap, tw-overlap)
		ny := ceilDiv(fh-overlap, th-overlap)
		if nx < 1 {
			nx = 1
		}
		if ny < 1 {
			ny = 1
		}
		n := float64(nx * ny)
		score := n*penalty + (n*float64(tw)*float64(th)-area)/area
		if !found || score < best {
			found = true
			best = score
			bestW, bestH = tw, th
		}
	}
	if !found {
		return candidates[0][0], candidates[0][1]
	}
	return bestW, bestH
}

func ceilDiv(a, b int) int {
	if a <= 0 {
		return 0
	}
	return (a + b - 1) / b
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// duplicatePixelThreshold marks a significant pixel change, about 10 8-bit levels.
const duplicatePixelThreshold = 10.0 / 255.0

// DuplicateRatio returns the fraction of pixels that changed significantly
// between two frames, used for duplicate frame detection.
func DuplicateRatio(a, b *Frame) float64 {
	if a.W != b.W || a.H != b.H {
		return 1
	}
	total := a.W * a.H
	if total <= 0 {
		return 0
	}
	n := a.H
	nw := max(min(runtime.NumCPU(), n), 1)
	chunk := (n + nw - 1) / nw
	parts := make([]int, (n+chunk-1)/chunk)
	parallelRows(n, func(s, e int) {
		cnt := 0
		for p := s * a.W; p < e*a.W; p++ {
			i := p * 3
			dr := a.Data[i] - b.Data[i]
			if dr < 0 {
				dr = -dr
			}
			dg := a.Data[i+1] - b.Data[i+1]
			if dg < 0 {
				dg = -dg
			}
			db := a.Data[i+2] - b.Data[i+2]
			if db < 0 {
				db = -db
			}
			if max(dr, dg, db) > duplicatePixelThreshold {
				cnt++
			}
		}
		parts[s/chunk] = cnt
	})
	sum := 0
	for _, v := range parts {
		sum += v
	}
	return float64(sum) / float64(total)
}

func toByte(v float32) byte {
	if v < 0 {
		v = 0
	}
	if v > 1 {
		v = 1
	}
	return byte(v*255 + 0.5)
}
