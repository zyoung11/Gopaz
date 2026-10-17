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
