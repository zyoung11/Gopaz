package pipeline

import "testing"

// TestChooseTileTopaz 覆盖运行日志里验证过的选块结果。
func TestChooseTileTopaz(t *testing.T) {
	enhance := [][2]int{
		{288, 288}, {352, 256}, {480, 384}, {384, 480}, {384, 576},
		{416, 576}, {576, 448}, {672, 384}, {672, 576},
	}
	apollo := [][2]int{
		{288, 288}, {352, 256}, {480, 384}, {384, 480}, {384, 576},
		{416, 576}, {672, 384}, {576, 448}, {672, 576},
		{704, 1152}, {1056, 1088}, {1344, 1152},
	}
	cases := []struct {
		name    string
		cands   [][2]int
		fw, fh  int
		overlap int
		wantW   int
		wantH   int
	}{
		{"prob-720p", enhance, 1280, 720, 48, 672, 384},
		{"prob-1080p", enhance, 1920, 1080, 48, 672, 576},
		{"prob-4k", enhance, 3840, 2160, 48, 672, 576},
		{"apo-1080p", apollo, 1920, 1080, 48, 1056, 1088},
		{"apo-4k", apollo, 3840, 2160, 48, 1344, 1152},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w, h := chooseTile(tc.cands, tc.fw, tc.fh, tc.overlap, TilePenalty)
			if w != tc.wantW || h != tc.wantH {
				t.Fatalf("chooseTile = %dx%d, want %dx%d", w, h, tc.wantW, tc.wantH)
			}
		})
	}
}

// TestDuplicateRatio 验证 Topaz 风格的显著差异像素比例。
func TestDuplicateRatio(t *testing.T) {
	const w, h = 100, 100
	base := NewFrame(w, h)
	for i := range base.Data {
		base.Data[i] = 0.5
	}
	if got := DuplicateRatio(base, base); got != 0 {
		t.Fatalf("identical ratio = %v, want 0", got)
	}
	global := NewFrame(w, h)
	for i := range global.Data {
		global.Data[i] = 0.5 + 20.0/255.0
	}
	if got := DuplicateRatio(base, global); got != 1 {
		t.Fatalf("global +20 ratio = %v, want 1", got)
	}
	small := NewFrame(w, h)
	for i := range small.Data {
		small.Data[i] = 0.5 + 5.0/255.0
	}
	if got := DuplicateRatio(base, small); got != 0 {
		t.Fatalf("global +5 ratio = %v, want 0", got)
	}
	block := base.Clone()
	for y := range 10 {
		for x := range 40 {
			i := (y*w + x) * 3
			block.Data[i], block.Data[i+1], block.Data[i+2] = 1, 1, 1
		}
	}
	if got := DuplicateRatio(base, block); got < 0.039 || got > 0.041 {
		t.Fatalf("4%% block ratio = %v, want 0.04", got)
	}
	if got := DuplicateRatio(base, NewFrame(10, 10)); got != 1 {
		t.Fatalf("size mismatch ratio = %v, want 1", got)
	}
}
