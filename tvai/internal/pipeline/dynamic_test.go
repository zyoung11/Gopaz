package pipeline

import "testing"

// TestChooseDynamicTile 覆盖 Aion 的显存预算选块。
func TestChooseDynamicTile(t *testing.T) {
	cases := []struct {
		name                  string
		baseGB, ppg, deviceGB float64
		fw, fh                int
		wantW, wantH          int
	}{
		{"24gb", 1.2, 120000, 24, 3840, 2160, 1664, 1664},
		{"23.57gb", 1.2, 120000, 23.57, 3840, 2160, 1632, 1632},
		{"2gb", 1.2, 120000, 2, 256, 160, 320, 320},
		{"4gb-landscape", 1.2, 120000, 4, 3840, 2160, 608, 576},
		{"4gb-portrait", 1.2, 120000, 4, 2160, 3840, 576, 608},
		{"unknown", 1.2, 120000, 0, 3840, 2160, 0, 0},
		{"no-ppg", 0, 0, 24, 3840, 2160, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w, h := chooseDynamicTile(tc.baseGB, tc.ppg, tc.deviceGB, tc.fw, tc.fh)
			if w != tc.wantW || h != tc.wantH {
				t.Fatalf("chooseDynamicTile = %dx%d, want %dx%d", w, h, tc.wantW, tc.wantH)
			}
		})
	}
}

// TestRoundUp32 验证 tile 对齐。
func TestRoundUp32(t *testing.T) {
	cases := []struct {
		in, want int
	}{
		{0, 32}, {1, 32}, {31, 32}, {32, 32}, {33, 64}, {100, 128}, {1632, 1632},
	}
	for _, tc := range cases {
		if got := roundUp32(tc.in); got != tc.want {
			t.Fatalf("roundUp32(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}
