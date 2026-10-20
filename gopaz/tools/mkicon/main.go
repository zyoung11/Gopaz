// Command mkicon renders the Gopaz brand mark into a multi-size .ico file.
package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
)

var (
	bgColor  = color.NRGBA{R: 0x17, G: 0x17, B: 0x17, A: 0xff}
	fgColor  = color.NRGBA{R: 0xa6, G: 0xff, B: 0x00, A: 0xff}
	icoSizes = []int{16, 24, 32, 48, 64, 128, 256}
)

// insideRounded reports whether a point lies inside the rounded brand square.
func insideRounded(x, y, size, radius float64) bool {
	if x < 0 || y < 0 || x >= size || y >= size {
		return false
	}
	half := size / 2
	dx := math.Max(0, math.Abs(x-half)-(half-radius))
	dy := math.Max(0, math.Abs(y-half)-(half-radius))
	return dx*dx+dy*dy <= radius*radius
}

// insidePlay reports whether a point lies inside the play triangle.
func insidePlay(x, y, size float64) bool {
	s := size / 32
	ax, ay := 12*s, 9*s
	bx, by := 24*s, 16*s
	cx, cy := 12*s, 23*s
	d1 := (x-bx)*(ay-by) - (ax-bx)*(y-by)
	d2 := (x-cx)*(by-cy) - (bx-cx)*(y-cy)
	d3 := (x-ax)*(cy-ay) - (cx-ax)*(y-ay)
	neg := d1 < 0 || d2 < 0 || d3 < 0
	pos := d1 > 0 || d2 > 0 || d3 > 0
	return !(neg && pos)
}

// render draws one antialiased icon frame at the given size.
func render(size int) *image.NRGBA {
	const ss = 6
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	radius := float64(size) * 6.0 / 32.0
	step := 1.0 / ss
	for py := range size {
		for px := range size {
			var rectHits, fgHits int
			for sy := range ss {
				for sx := range ss {
					x := float64(px) + (float64(sx)+0.5)*step
					y := float64(py) + (float64(sy)+0.5)*step
					if !insideRounded(x, y, float64(size), radius) {
						continue
					}
					rectHits++
					if insidePlay(x, y, float64(size)) {
						fgHits++
					}
				}
			}
			if rectHits == 0 {
				continue
			}
			total := float64(ss * ss)
			bg := float64(rectHits-fgHits) / total
			fg := float64(fgHits) / total
			a := float64(rectHits) / total
			out := img.NRGBAAt(px, py)
			out.R = uint8(math.Round(float64(bgColor.R)*bg+float64(fgColor.R)*fg) / a)
			out.G = uint8(math.Round(float64(bgColor.G)*bg+float64(fgColor.G)*fg) / a)
			out.B = uint8(math.Round(float64(bgColor.B)*bg+float64(fgColor.B)*fg) / a)
			out.A = uint8(math.Round(a * 255))
			img.SetNRGBA(px, py, out)
		}
	}
	return img
}

// encodeICO packs PNG-encoded frames into the Windows .ico container.
func encodeICO(frames [][]byte, sizes []int) []byte {
	var buf bytes.Buffer
	_ = binary.Write(&buf, binary.LittleEndian, uint16(0))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(len(frames)))
	offset := 6 + 16*len(frames)
	for i, blob := range frames {
		dim := sizes[i]
		if dim >= 256 {
			dim = 0
		}
		buf.WriteByte(byte(dim))
		buf.WriteByte(byte(dim))
		buf.WriteByte(0)
		buf.WriteByte(0)
		_ = binary.Write(&buf, binary.LittleEndian, uint16(1))
		_ = binary.Write(&buf, binary.LittleEndian, uint16(32))
		_ = binary.Write(&buf, binary.LittleEndian, uint32(len(blob)))
		_ = binary.Write(&buf, binary.LittleEndian, uint32(offset))
		offset += len(blob)
	}
	for _, blob := range frames {
		buf.Write(blob)
	}
	return buf.Bytes()
}

func main() {
	out := flag.String("o", "gopaz.ico", "output icon path")
	flag.Parse()
	var frames [][]byte
	for _, size := range icoSizes {
		var buf bytes.Buffer
		if err := png.Encode(&buf, render(size)); err != nil {
			panic(err)
		}
		frames = append(frames, buf.Bytes())
	}
	if err := os.WriteFile(*out, encodeICO(frames, icoSizes), 0o644); err != nil {
		panic(err)
	}
}
