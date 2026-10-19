package main

import "math"

const (
	tileAlign            = 32
	defaultMaxTilePixels = 2400000.0
	minTilePixels        = 262144.0
	maxTilePixels        = 4000000.0
)

// tilePos is the top-left coordinate of a tile in the input frame.
type tilePos struct{ x, y int }

// interpPlan describes the tiling, blend weights, and subframe mapping of one
// interpolation task.
type interpPlan struct {
	fw, fh int
	tw, th int
	ramp   int
	pos    []tilePos
	weight []float32
	inv    []float32
	batch  int
}

// chooseAionTile picks the most efficient tile for dynamic-size Aion. It prefers
// a single full-frame tile or full-width/full-height strips over many square
// tiles with redundant overlap.
func chooseAionTile(fw, fh, overlap int, budgetGB float64) (int, int) {
	if fw <= 0 || fh <= 0 {
		return 0, 0
	}
	maxPixels := defaultMaxTilePixels
	if budgetGB > 0 {
		maxPixels = clampFloat(budgetGB*600000, minTilePixels, maxTilePixels)
	}
	full := float64(fw) * float64(fh)
	if full <= maxPixels {
		return roundUp(tileAlign, fw), roundUp(tileAlign, fh)
	}
	long, short := fw, fh
	horizontal := true
	if fh > fw {
		long, short = fh, fw
		horizontal = false
	}
	bestSpan := 0
	for n := 2; n <= 32; n++ {
		span := int(math.Ceil((float64(short) + float64(n-1)*float64(overlap)) / float64(n)))
		span = roundUp(tileAlign, span)
		area := float64(long) * float64(span)
		if area > maxPixels {
			continue
		}
		bestSpan = span
		break
	}
	if bestSpan == 0 {
		bestSpan = roundUp(tileAlign, short)
	}
	if horizontal {
		return roundUp(tileAlign, fw), bestSpan
	}
	return bestSpan, roundUp(tileAlign, fh)
}

// newInterpPlan computes the tile grid, blend weights, and normalization map.
func newInterpPlan(fw, fh, tw, th, overlap, batchLimit int) *interpPlan {
	if tw <= 0 {
		tw = fw
	}
	if th <= 0 {
		th = fh
	}
	ramp := min(overlap, tw/4, th/4)
	p := &interpPlan{fw: fw, fh: fh, tw: tw, th: th, ramp: ramp}
	stepX := tw - overlap
	stepY := th - overlap
	if stepX <= 0 {
		stepX = tw
	}
	if stepY <= 0 {
		stepY = th
	}
	for y := 0; ; {
		for x := 0; ; {
			p.pos = append(p.pos, tilePos{x: x, y: y})
			if x+tw >= fw {
				break
			}
			x += stepX
		}
		if y+th >= fh {
			break
		}
		y += stepY
	}
	p.weight = make([]float32, tw*th)
	for j := 0; j < th; j++ {
		for i := 0; i < tw; i++ {
			p.weight[j*tw+i] = rampWeight(i, j, tw, th, ramp)
		}
	}
	p.inv = make([]float32, fw*fh)
	for _, tp := range p.pos {
		for j := 0; j < th; j++ {
			dy := tp.y + j
			if dy >= fh {
				break
			}
			for i := 0; i < tw; i++ {
				dx := tp.x + i
				if dx >= fw {
					break
				}
				p.inv[dy*fw+dx] += p.weight[j*tw+i]
			}
		}
	}
	for i, v := range p.inv {
		if v > 0 {
			p.inv[i] = 1 / v
		} else {
			p.inv[i] = 1
		}
	}
	p.batch = 4
	if tw >= fw || th >= fh {
		p.batch = 1
	}
	if p.batch > len(p.pos) {
		p.batch = len(p.pos)
	}
	if batchLimit > 0 && p.batch > batchLimit {
		p.batch = batchLimit
	}
	if p.batch < 1 {
		p.batch = 1
	}
	return p
}

// subFrames maps a slowdown interval to model subframe indices and returns the
// deduplicated indices together with the output order.
func (p *interpPlan) subFrames(span, subdiv int) ([]int, []int) {
	order := make([]int, 0, max(span-1, 0))
	seen := make(map[int]int, subdiv)
	var unique []int
	for i := 1; i < span; i++ {
		t := float64(i) / float64(span)
		k := max(int(math.Round(t*float64(subdiv)))-1, 0)
		if k >= subdiv {
			k = subdiv - 1
		}
		oi, ok := seen[k]
		if !ok {
			oi = len(unique)
			seen[k] = oi
			unique = append(unique, k)
		}
		order = append(order, oi)
	}
	return unique, order
}

func roundUp(align, v int) int {
	if v <= 0 {
		return align
	}
	return (v + align - 1) / align * align
}

func clampFloat(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// npuMaxTileArea is the largest tile area the VPUX compiler accepts, 576x672.
const npuMaxTileArea = 576 * 672

// npuTiles drops tiles too large for the OpenVINO NPU compiler.
func npuTiles(tiles [][2]int) [][2]int {
	out := make([][2]int, 0, len(tiles))
	for _, t := range tiles {
		if t[0]*t[1] <= npuMaxTileArea {
			out = append(out, t)
		}
	}
	return out
}
