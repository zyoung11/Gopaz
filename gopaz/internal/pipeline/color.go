package pipeline

import (
	"math"
	"sync"
)

const (
	colorLUTSize = 4096
	labDelta     = 6.0 / 29.0
	labDelta3    = labDelta * labDelta * labDelta
	labSlope     = 3 * labDelta * labDelta
	labOffset    = 4.0 / 29.0
)

var (
	srgbToLinearLUT [colorLUTSize + 1]float64
	linearToSrgbLUT [colorLUTSize + 1]float64
	cbrtLUT         [colorLUTSize + 1]float64
)

func init() {
	for i := 0; i <= colorLUTSize; i++ {
		x := float64(i) / float64(colorLUTSize)
		srgbToLinearLUT[i] = srgbToLinearExact(x)
		linearToSrgbLUT[i] = linearToSrgbExact(x)
		cbrtLUT[i] = math.Cbrt(x)
	}
}

func srgbToLinearExact(v float64) float64 {
	if v <= 0.04045 {
		return v / 12.92
	}
	return math.Pow((v+0.055)/1.055, 2.4)
}

func linearToSrgbExact(v float64) float64 {
	if v <= 0.0031308 {
		return v * 12.92
	}
	return 1.055*math.Pow(v, 1.0/2.4) - 0.055
}

func lookupLUT(lut *[colorLUTSize + 1]float64, v float64) float64 {
	if v <= 0 {
		return lut[0]
	}
	if v >= 1 {
		return lut[colorLUTSize]
	}
	pos := v * colorLUTSize
	i := int(pos)
	f := pos - float64(i)
	return lut[i] + (lut[i+1]-lut[i])*f
}

func labF(t float64) float64 {
	if t > labDelta3 {
		if t >= 1 {
			return math.Cbrt(t)
		}
		return lookupLUT(&cbrtLUT, t)
	}
	return t/labSlope + labOffset
}

func labFInv(t float64) float64 {
	if t > labDelta {
		return t * t * t
	}
	return labSlope * (t - labOffset)
}

func rgbToLab(r, g, b float64) (float64, float64, float64) {
	lr := lookupLUT(&srgbToLinearLUT, r)
	lg := lookupLUT(&srgbToLinearLUT, g)
	lb := lookupLUT(&srgbToLinearLUT, b)
	x := (0.4124564*lr + 0.3575761*lg + 0.1804375*lb) / 0.95047
	y := 0.2126729*lr + 0.7151522*lg + 0.0721750*lb
	z := (0.0193339*lr + 0.1191920*lg + 0.9503041*lb) / 1.08883
	fx := labF(x)
	fy := labF(y)
	fz := labF(z)
	return 116*fy - 16, 500 * (fx - fy), 200 * (fy - fz)
}

func labToRGB(l, a, b float64) (float64, float64, float64) {
	fy := (l + 16) / 116
	fx := fy + a/500
	fz := fy - b/200
	x := labFInv(fx) * 0.95047
	y := labFInv(fy)
	z := labFInv(fz) * 1.08883
	lr := 3.2404542*x - 1.5371385*y - 0.4985314*z
	lg := -0.9692660*x + 1.8760108*y + 0.0415560*z
	lb := 0.0556434*x - 0.2040259*y + 1.0572252*z
	return lookupLUT(&linearToSrgbLUT, lr), lookupLUT(&linearToSrgbLUT, lg), lookupLUT(&linearToSrgbLUT, lb)
}

func labStats(f *Frame) (mean, std [3]float64) {
	var mu sync.Mutex
	var sum, sumsq [3]float64
	parallelRows(f.H, func(s, e int) {
		var ls, lsq [3]float64
		for j := s; j < e; j++ {
			for i := 0; i < f.W; i++ {
				idx := (j*f.W + i) * 3
				l, a, b := rgbToLab(float64(f.Data[idx]), float64(f.Data[idx+1]), float64(f.Data[idx+2]))
				ls[0] += l
				ls[1] += a
				ls[2] += b
				lsq[0] += l * l
				lsq[1] += a * a
				lsq[2] += b * b
			}
		}
		mu.Lock()
		for k := range 3 {
			sum[k] += ls[k]
			sumsq[k] += lsq[k]
		}
		mu.Unlock()
	})
	count := float64(f.W * f.H)
	if count == 0 {
		return
	}
	for k := range 3 {
		mean[k] = sum[k] / count
		v := sumsq[k]/count - mean[k]*mean[k]
		if v > 0 {
			std[k] = math.Sqrt(v)
		}
	}
	return
}

// ColorTransferLab matches the Lab mean and variance of dst to src, emulating
// Topaz keepColor.
func ColorTransferLab(src, dst *Frame) {
	if src == nil || dst == nil || src.W <= 0 || src.H <= 0 || dst.W <= 0 || dst.H <= 0 {
		return
	}
	srcMean, srcStd := labStats(src)
	dstMean, dstStd := labStats(dst)
	var scale, offset [3]float64
	for k := range 3 {
		if dstStd[k] > 1e-9 {
			scale[k] = srcStd[k] / dstStd[k]
			offset[k] = srcMean[k] - scale[k]*dstMean[k]
		} else {
			scale[k] = 0
			offset[k] = srcMean[k]
		}
	}
	parallelRows(dst.H, func(s, e int) {
		for j := s; j < e; j++ {
			for i := 0; i < dst.W; i++ {
				idx := (j*dst.W + i) * 3
				l, a, b := rgbToLab(float64(dst.Data[idx]), float64(dst.Data[idx+1]), float64(dst.Data[idx+2]))
				r, g, bb := labToRGB(l*scale[0]+offset[0], a*scale[1]+offset[1], b*scale[2]+offset[2])
				dst.Data[idx] = float32(clamp01(r))
				dst.Data[idx+1] = float32(clamp01(g))
				dst.Data[idx+2] = float32(clamp01(bb))
			}
		}
	})
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
