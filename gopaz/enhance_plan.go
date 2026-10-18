package main

// enhancePlan describes the tiling, blend weights, and normalization map of an
// enhancement model.
type enhancePlan struct {
	fw, fh       int
	tw, th       int
	scale        int
	outW, outH   int
	tileW, tileH int
	ramp         int
	pos          []tilePos
	weight       []float32
	inv          []float32
}

// newEnhancePlan computes the tile grid and precomputed weights for an
// enhancement model.
func newEnhancePlan(fw, fh, tw, th, scale, overlap int) *enhancePlan {
	tileW, tileH := tw*scale, th*scale
	ramp := min(overlap*scale, tileW/4, tileH/4)
	p := &enhancePlan{
		fw: fw, fh: fh, tw: tw, th: th, scale: scale,
		outW: fw * scale, outH: fh * scale, tileW: tileW, tileH: tileH, ramp: ramp,
	}
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
	p.weight = make([]float32, tileW*tileH)
	for j := range tileH {
		for i := range tileW {
			p.weight[j*tileW+i] = rampWeight(i, j, tileW, tileH, ramp)
		}
	}
	p.inv = make([]float32, p.outW*p.outH)
	for _, tp := range p.pos {
		for j := range tileH {
			dy := tp.y*scale + j
			if dy >= p.outH {
				break
			}
			for i := range tileW {
				dx := tp.x*scale + i
				if dx >= p.outW {
					break
				}
				p.inv[dy*p.outW+dx] += p.weight[j*tileW+i]
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
	return p
}

// addWeightedTile accumulates an output tile into the canvas with precomputed weights.
func addWeightedTile(canvas, tile *Frame, weight []float32, ox, oy int) {
	for j := 0; j < tile.H; j++ {
		dy := oy + j
		if dy < 0 || dy >= canvas.H {
			continue
		}
		for i := 0; i < tile.W; i++ {
			dx := ox + i
			if dx < 0 || dx >= canvas.W {
				continue
			}
			w := weight[j*tile.W+i]
			ti := (j*tile.W + i) * 3
			ci := (dy*canvas.W + dx) * 3
			canvas.Data[ci] += tile.Data[ti] * w
			canvas.Data[ci+1] += tile.Data[ti+1] * w
			canvas.Data[ci+2] += tile.Data[ti+2] * w
		}
	}
}

// normalizeCanvasInv normalizes the canvas with precomputed inverse weights.
func normalizeCanvasInv(canvas *Frame, inv []float32) {
	parallelRows(canvas.H, func(s, e int) {
		for p := s * canvas.W; p < e*canvas.W; p++ {
			w := inv[p]
			if w <= 0 {
				w = 1
			}
			i := p * 3
			canvas.Data[i] *= w
			canvas.Data[i+1] *= w
			canvas.Data[i+2] *= w
		}
	})
}
