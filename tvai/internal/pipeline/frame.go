// Package pipeline 实现超分与插帧的处理管线。
package pipeline

import (
	"fmt"
	"math"
)

// Frame 是一帧 RGB，数据为 NHWC 布局的 [0,1] 浮点。
type Frame struct {
	W, H int
	Data []float32 // 长度 W*H*3，RGB 交织
}

// NewFrame 分配空白帧。
func NewFrame(w, h int) *Frame {
	return &Frame{W: w, H: h, Data: make([]float32, w*h*3)}
}

// FromRGB24 从原始 RGB24 字节（0-255）转为 [0,1] 浮点帧。
func FromRGB24(b []byte, w, h int) *Frame {
	f := &Frame{W: w, H: h, Data: make([]float32, w*h*3)}
	for i := 0; i < w*h*3; i++ {
		f.Data[i] = float32(b[i]) / 255.0
	}
	return f
}

// ToRGB24 转回 RGB24 字节，越界值做截断。
func (f *Frame) ToRGB24() []byte {
	out := make([]byte, f.W*f.H*3)
	for i, v := range f.Data {
		if v < 0 {
			v = 0
		}
		if v > 1 {
			v = 1
		}
		out[i] = byte(v*255 + 0.5)
	}
	return out
}

// Clone 深拷贝。
func (f *Frame) Clone() *Frame {
	c := &Frame{W: f.W, H: f.H, Data: make([]float32, len(f.Data))}
	copy(c.Data, f.Data)
	return c
}

// ExtractTile 提取一个 tile，坐标为输入帧空间。若越界则用边缘像素填充（replicate）。
func (f *Frame) ExtractTile(x, y, tw, th int) *Frame {
	t := NewFrame(tw, th)
	for j := 0; j < th; j++ {
		sy := clampInt(y+j, 0, f.H-1)
		for i := 0; i < tw; i++ {
			sx := clampInt(x+i, 0, f.W-1)
			si := (sy*f.W + sx) * 3
			di := (j*tw + i) * 3
			t.Data[di] = f.Data[si]
			t.Data[di+1] = f.Data[si+1]
			t.Data[di+2] = f.Data[si+2]
		}
	}
	return t
}

// FillRampWeight 填充一个 tile 的融合权重图：边缘线性斜坡，内部 1。
func FillRampWeight(w *Frame, ramp int) {
	for j := 0; j < w.H; j++ {
		for i := 0; i < w.W; i++ {
			w.Data[j*w.W+i] = rampWeight(i, j, w.W, w.H, ramp)
		}
	}
}

// MergeTileWeighted 把放大后的 tile 按 tileW 权重累加进画布，
// 同时把权重累加到全局权重图。
func MergeTileWeighted(canvas, canvasW, tile, tileW *Frame, ox, oy int) {
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
			ti := (j*tile.W + i) * 3
			ci := (dy*canvas.W + dx) * 3
			w := tileW.Data[j*tile.W+i]
			canvas.Data[ci] += tile.Data[ti] * w
			canvas.Data[ci+1] += tile.Data[ti+1] * w
			canvas.Data[ci+2] += tile.Data[ti+2] * w
			canvasW.Data[dy*canvasW.W+dx] += w
		}
	}
}

// AddWeight 给权重图的一个矩形区域累加权重。
func AddWeight(weight *Frame, x, y, w, h int, ramp int) {
	for j := 0; j < h; j++ {
		dy := y + j
		if dy < 0 || dy >= weight.H {
			continue
		}
		for i := 0; i < w; i++ {
			dx := x + i
			if dx < 0 || dx >= weight.W {
				continue
			}
			wt := rampWeight(i, j, w, h, ramp)
			weight.Data[dy*weight.W+dx] += wt
		}
	}
}

// rampWeight 计算一个 tile 内某像素的融合权重：边缘用线性斜坡过渡，内部为 1。
// ramp 是斜坡宽度（输出空间像素数）。
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

// NormalizeCanvas 用权重图归一化画布（就地）。
func NormalizeCanvas(canvas, weight *Frame) {
	n := len(canvas.Data)
	// RGB 交织，权重按像素存，需对每个通道除以同一权重。
	for p := 0; p < canvas.W*canvas.H; p++ {
		w := weight.Data[p]
		if w <= 0 {
			w = 1
		}
		inv := float32(1) / w
		ci := p * 3
		canvas.Data[ci] *= inv
		canvas.Data[ci+1] *= inv
		canvas.Data[ci+2] *= inv
	}
	_ = n
}

// UpscaleBilinear 把帧双线性缩放到 nw x nh（用于初始化 hr_prev 等）。
func (f *Frame) UpscaleBilinear(nw, nh int) *Frame {
	out := NewFrame(nw, nh)
	sxRatio := float64(f.W) / float64(nw)
	syRatio := float64(f.H) / float64(nh)
	for j := 0; j < nh; j++ {
		sy := (float64(j)+0.5)*syRatio - 0.5
		y0 := clampInt(int(math.Floor(sy)), 0, f.H-1)
		y1 := clampInt(y0+1, 0, f.H-1)
		fy := sy - math.Floor(sy)
		if sy < 0 {
			fy = 0
		}
		for i := 0; i < nw; i++ {
			sx := (float64(i)+0.5)*sxRatio - 0.5
			x0 := clampInt(int(math.Floor(sx)), 0, f.W-1)
			x1 := clampInt(x0+1, 0, f.W-1)
			fx := sx - math.Floor(sx)
			if sx < 0 {
				fx = 0
			}
			for c := 0; c < 3; c++ {
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

// hasNaN 检测切片中是否存在 NaN（CUDA 推理不兼容时会产出）。
func hasNaN(s []float32) bool {
	for _, v := range s {
		if math.IsNaN(float64(v)) {
			return true
		}
	}
	return false
}

// chooseTile 为给定帧尺寸挑选合适的 tile。candidates 每个元素为 {W, H}。
// 返回 (tileW, tileH)。策略：在能完整放进帧（W<=fw 且 H<=fh）的候选中选面积最大者。
func chooseTile(candidates [][2]int, fw, fh int) (int, int) {
	if len(candidates) == 0 {
		return fw, fh
	}
	bestW, bestH := candidates[0][0], candidates[0][1]
	bestArea := -1.0
	anyFit := false
	for _, c := range candidates {
		tw, th := c[0], c[1]
		area := float64(tw * th)
		if tw <= fw && th <= fh {
			anyFit = true
			if area > bestArea {
				bestArea = area
				bestW, bestH = tw, th
			}
		}
	}
	if !anyFit {
		for _, c := range candidates {
			if a := float64(c[0] * c[1]); a > bestArea {
				bestArea = a
				bestW, bestH = c[0], c[1]
			}
		}
	}
	return bestW, bestH
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

// SceneDiff 计算两帧的平均绝对差（用于场景切换检测）。
func SceneDiff(a, b *Frame) float64 {
	if a.W != b.W || a.H != b.H {
		return 1
	}
	var sum float64
	n := len(a.Data)
	for i := 0; i < n; i++ {
		d := float64(a.Data[i]) - float64(b.Data[i])
		if d < 0 {
			d = -d
		}
		sum += d
	}
	return sum / float64(n)
}

// ValidateTileHW 校验 tile 尺寸能被倍率整除关系正确（占位，实际推理按输出尺寸）。
func ValidateTileHW(th, tw int) error {
	if th <= 0 || tw <= 0 {
		return fmt.Errorf("非法 tile 尺寸 %dx%d", th, tw)
	}
	return nil
}
