package pipeline

import "testing"

// TestChooseAionTile covers the Aion strip tiling strategy.
func TestChooseAionTile(t *testing.T) {
	cases := []struct {
		name         string
		fw, fh       int
		overlap      int
		budgetGB     float64
		wantW, wantH int
	}{
		{"4k", 3840, 2160, 32, 0, 3840, 576},
		{"4k-big-budget", 3840, 2160, 32, 8, 3840, 768},
		{"1080p", 1920, 1080, 32, 0, 1920, 1088},
		{"1440p", 2560, 1440, 32, 0, 2560, 736},
		{"720p", 1280, 720, 32, 0, 1280, 736},
		{"portrait-4k", 2160, 3840, 32, 0, 576, 3840},
		{"tiny", 256, 160, 32, 0, 256, 160},
		{"unknown", 0, 2160, 32, 0, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w, h := chooseAionTile(tc.fw, tc.fh, tc.overlap, tc.budgetGB)
			if w != tc.wantW || h != tc.wantH {
				t.Fatalf("chooseAionTile = %dx%d, want %dx%d", w, h, tc.wantW, tc.wantH)
			}
		})
	}
}

// TestRoundUp checks tile alignment.
func TestRoundUp(t *testing.T) {
	cases := []struct {
		in, want int
	}{
		{0, 32}, {1, 32}, {31, 32}, {32, 32}, {33, 64}, {100, 128}, {1632, 1632},
	}
	for _, tc := range cases {
		if got := roundUp(tileAlign, tc.in); got != tc.want {
			t.Fatalf("roundUp(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

// TestSubFrames checks the slowdown interval to model subframe mapping.
func TestSubFrames(t *testing.T) {
	p := &interpPlan{}
	unique, order := p.subFrames(8, 16)
	wantUnique := []int{1, 3, 5, 7, 9, 11, 13}
	if len(unique) != len(wantUnique) {
		t.Fatalf("unique = %v, want %v", unique, wantUnique)
	}
	for i := range wantUnique {
		if unique[i] != wantUnique[i] {
			t.Fatalf("unique = %v, want %v", unique, wantUnique)
		}
	}
	if len(order) != 7 {
		t.Fatalf("order = %v, want 7 项", order)
	}
	unique, order = p.subFrames(2, 8)
	if len(unique) != 1 || unique[0] != 3 || len(order) != 1 || order[0] != 0 {
		t.Fatalf("2x apollo: unique=%v order=%v", unique, order)
	}
	unique, order = p.subFrames(80, 16)
	if len(order) != 79 {
		t.Fatalf("span=80 order=%d, want 79", len(order))
	}
	if len(unique) > 16 {
		t.Fatalf("unique=%d, want <=16", len(unique))
	}
}
