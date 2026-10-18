package main

import "math"

// f32tof16 converts a float32 to its IEEE 754 half-precision bit pattern.
func f32tof16(f float32) uint16 {
	b := math.Float32bits(f)
	sign := uint16(b>>31) & 1
	exp := int((b>>23)&0xff) - 127 + 15
	mant := b & 0x7fffff
	if exp <= 0 {
		if exp < -10 {
			return sign << 15
		}
		mant |= 0x800000
		shift := uint(14 - exp)
		return sign<<15 | uint16(mant>>shift)
	}
	if exp >= 31 {
		return sign<<15 | 0x7c00
	}
	return sign<<15 | uint16(exp)<<10 | uint16(mant>>13)
}

// f16tof32 converts an IEEE 754 half-precision bit pattern to float32.
func f16tof32(h uint16) float32 {
	sign := uint32(h&0x8000) << 16
	exp := uint32(h>>10) & 0x1f
	mant := uint32(h & 0x3ff)
	switch exp {
	case 0:
		if mant == 0 {
			return math.Float32frombits(sign)
		}
		for mant&0x400 == 0 {
			mant <<= 1
			exp--
		}
		mant &= 0x3ff
		return math.Float32frombits(sign | (exp+127-15)<<23 | mant<<13)
	case 31:
		return math.Float32frombits(sign | 0x7f800000 | mant<<13)
	}
	return math.Float32frombits(sign | (exp+127-15)<<23 | mant<<13)
}

// f32SliceToF16 converts a float32 slice to half-precision bit patterns.
func f32SliceToF16(s []float32) []uint16 {
	out := make([]uint16, len(s))
	for i, v := range s {
		out[i] = f32tof16(v)
	}
	return out
}

// hasNaNF16 reports whether the half-precision slice contains NaN.
func hasNaNF16(s []uint16) bool {
	for _, v := range s {
		f := f16tof32(v)
		if f != f {
			return true
		}
	}
	return false
}
