package main

import (
	"fmt"
	"time"
)

// interpWorker owns a pair of sessions and reusable input buffers.
type interpWorker struct {
	r     *Runner
	main  *Session
	bot   *Session
	plan  *interpPlan
	buf32 [4][]float32
	buf16 [4][]uint16
	in8   []float32
}

// newInterpWorker creates an interpolation worker.
func newInterpWorker(r *Runner, main, bot *Session, plan *interpPlan) *interpWorker {
	return &interpWorker{r: r, main: main, bot: bot, plan: plan}
}

// run executes one full inference over a batch of tiles and returns the main
// model output tensor.
func (w *interpWorker) run(window []*Frame, tiles []tilePos) (*Tensor, error) {
	p := w.plan
	b := len(tiles)
	tw, th := p.tw, p.th
	elemType := w.r.inputType("in0")
	dims := []int64{int64(b), int64(th), int64(tw), 3}
	names := make([]string, 0, 5)
	tensors := make([]*Tensor, 0, 5)
	defer func() {
		for _, t := range tensors {
			t.Destroy()
		}
	}()
	for fi := range 4 {
		name := fmt.Sprintf("in%d", fi)
		if elemType == Float16 {
			n := b * tw * th * 3
			if cap(w.buf16[fi]) < n {
				w.buf16[fi] = make([]uint16, n)
			}
			buf := w.buf16[fi][:n]
			fillTileRows16(buf, window[fi], tiles, tw, th)
			t, err := NewTensorRef(dims, Float16, ptrOfU16(buf), n, buf)
			if err != nil {
				return nil, err
			}
			names = append(names, name)
			tensors = append(tensors, t)
			continue
		}
		n := b * tw * th * 3
		if cap(w.buf32[fi]) < n {
			w.buf32[fi] = make([]float32, n)
		}
		buf := w.buf32[fi][:n]
		fillTileRows32(buf, window[fi], tiles, tw, th)
		t, err := NewTensorRef(dims, Float, ptrOf(buf), n, buf)
		if err != nil {
			return nil, err
		}
		names = append(names, name)
		tensors = append(tensors, t)
	}
	if w.bot != nil {
		t0 := time.Now()
		outs, err := w.bot.Run(names, tensors, []string{"out"})
		w.r.addInfer(time.Since(t0).Nanoseconds())
		if err != nil {
			return nil, err
		}
		h8, w8 := th/4, tw/4
		n8 := b * h8 * w8 * 24
		if cap(w.in8) < n8 {
			w.in8 = make([]float32, n8)
		}
		in8 := w.in8[:n8]
		if err := downsampleBottom(outs[0], in8, b, tw, th, w.r.bottomOutType); err != nil {
			outs[0].Destroy()
			return nil, err
		}
		outs[0].Destroy()
		zt, err := NewTensorRef([]int64{int64(b), int64(h8), int64(w8), 24}, Float, ptrOf(in8), n8, in8)
		if err != nil {
			return nil, err
		}
		names = append(names, "in8x")
		tensors = append(tensors, zt)
	}
	t0 := time.Now()
	outs, err := w.main.Run(names, tensors, []string{w.r.outputName()})
	w.r.addInfer(time.Since(t0).Nanoseconds())
	if err != nil {
		return nil, err
	}
	return outs[0], nil
}

// fillTileRows32 copies source frame tiles directly into the batch buffer.
func fillTileRows32(buf []float32, f *Frame, tiles []tilePos, tw, th int) {
	parallelRows(len(tiles)*th, func(s, e int) {
		for row := s; row < e; row++ {
			ti := row / th
			j := row - ti*th
			tp := tiles[ti]
			sy := clampInt(tp.y+j, 0, f.H-1)
			dst := buf[row*tw*3 : (row+1)*tw*3]
			srcRow := sy * f.W
			for i := range tw {
				sx := clampInt(tp.x+i, 0, f.W-1)
				si := (srcRow + sx) * 3
				di := i * 3
				dst[di] = f.Data[si]
				dst[di+1] = f.Data[si+1]
				dst[di+2] = f.Data[si+2]
			}
		}
	})
}

// fillTileRows16 converts source frame tiles to fp16 and fills the batch buffer.
func fillTileRows16(buf []uint16, f *Frame, tiles []tilePos, tw, th int) {
	parallelRows(len(tiles)*th, func(s, e int) {
		for row := s; row < e; row++ {
			ti := row / th
			j := row - ti*th
			tp := tiles[ti]
			sy := clampInt(tp.y+j, 0, f.H-1)
			dst := buf[row*tw*3 : (row+1)*tw*3]
			srcRow := sy * f.W
			for i := range tw {
				sx := clampInt(tp.x+i, 0, f.W-1)
				si := (srcRow + sx) * 3
				di := i * 3
				dst[di] = f32tof16(f.Data[si])
				dst[di+1] = f32tof16(f.Data[si+1])
				dst[di+2] = f32tof16(f.Data[si+2])
			}
		}
	})
}

// downsampleBottom block-averages the 24-channel bottom model output into in8x.
func downsampleBottom(t *Tensor, out []float32, b, tw, th, elemType int) error {
	raw, err := tensorFloatsView(t, b*tw*th*24, elemType)
	if err != nil {
		return err
	}
	w8, h8 := tw/4, th/4
	parallelRows(b*h8, func(s, e int) {
		for row := s; row < e; row++ {
			bi := row / h8
			j := row - bi*h8
			for i := range w8 {
				for c := range 24 {
					var sum float32
					for dy := range 4 {
						for dx := range 4 {
							si := ((bi*th+j*4+dy)*tw + i*4 + dx) * 24
							sum += raw[si+c]
						}
					}
					out[(row*w8+i)*24+c] = sum * (1.0 / 16.0)
				}
			}
		}
	})
	return nil
}

// rawView is a float32 or float16 view of the main model output.
type rawView struct {
	f32 []float32
	f16 []uint16
}

// tensorFloatsView returns a float32 view of the tensor. An fp16 tensor is
// converted into a new buffer.
func tensorFloatsView(t *Tensor, n, elemType int) ([]float32, error) {
	p := t.Data()
	if p == nil {
		return nil, fmt.Errorf("张量数据为空")
	}
	if elemType == Float16 {
		src := unsafeSliceU16(p, n)
		out := make([]float32, n)
		for i := range n {
			out[i] = f16tof32(src[i])
		}
		return out, nil
	}
	return unsafeSlice32(p, n), nil
}

// outputView returns a raw view of the main model output without copying.
func (r *Runner) outputView(t *Tensor, n int) (rawView, error) {
	p := t.Data()
	if p == nil {
		return rawView{}, fmt.Errorf("张量数据为空")
	}
	if r.outputType() == Float16 {
		return rawView{f16: unsafeSliceU16(p, n)}, nil
	}
	return rawView{f32: unsafeSlice32(p, n)}, nil
}

// mergeSubframe accumulates the tiles of one batch into the given subframe canvas.
func mergeSubframe(canvas *Frame, raw rawView, tiles []tilePos, k, stride int, p *interpPlan) {
	if raw.f32 != nil {
		for bi, tp := range tiles {
			mergeTileF32(canvas, raw.f32, bi, tp, k, stride, p)
		}
		return
	}
	for bi, tp := range tiles {
		mergeTileF16(canvas, raw.f16, bi, tp, k, stride, p)
	}
}

func mergeTileF32(canvas *Frame, raw []float32, bi int, tp tilePos, k, stride int, p *interpPlan) {
	tw, th := p.tw, p.th
	ny := min(th, p.fh-tp.y)
	nx := min(tw, p.fw-tp.x)
	if ny <= 0 || nx <= 0 {
		return
	}
	base := bi * th * tw * stride
	for j := range ny {
		di := ((tp.y+j)*p.fw + tp.x) * 3
		wrow := j * tw
		si := base + j*tw*stride + k*3
		for i := range nx {
			w := p.weight[wrow+i]
			canvas.Data[di] += raw[si] * w
			canvas.Data[di+1] += raw[si+1] * w
			canvas.Data[di+2] += raw[si+2] * w
			di += 3
			si += stride
		}
	}
}

func mergeTileF16(canvas *Frame, raw []uint16, bi int, tp tilePos, k, stride int, p *interpPlan) {
	tw, th := p.tw, p.th
	ny := min(th, p.fh-tp.y)
	nx := min(tw, p.fw-tp.x)
	if ny <= 0 || nx <= 0 {
		return
	}
	base := bi * th * tw * stride
	for j := range ny {
		di := ((tp.y+j)*p.fw + tp.x) * 3
		wrow := j * tw
		si := base + j*tw*stride + k*3
		for i := range nx {
			w := p.weight[wrow+i]
			canvas.Data[di] += f16tof32(raw[si]) * w
			canvas.Data[di+1] += f16tof32(raw[si+1]) * w
			canvas.Data[di+2] += f16tof32(raw[si+2]) * w
			di += 3
			si += stride
		}
	}
}
