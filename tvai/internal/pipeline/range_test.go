package pipeline

import "testing"

// TestResolveFrameRange 覆盖全片、起始、结束、越界和未知总帧数。
func TestResolveFrameRange(t *testing.T) {
	cases := []struct {
		name      string
		start     int
		end       int
		total     int
		wantStart int
		wantEnd   int
		wantCount int
	}{
		{"full", -1, -1, 100, -1, -1, 100},
		{"start-only", 10, -1, 100, 10, 99, 90},
		{"end-only", -1, 19, 100, 0, 19, 20},
		{"both", 10, 19, 100, 10, 19, 10},
		{"end-clamped", 10, 200, 100, 10, 99, 90},
		{"start-out", 200, -1, 100, 200, -1, 0},
		{"start-after-end", 50, 10, 100, 50, -1, 0},
		{"unknown-end", -1, 19, 0, 0, 19, 20},
		{"unknown-start-only", 10, -1, 0, 10, -1, 0},
		{"unknown-full", -1, -1, 0, -1, -1, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveFrameRange(tc.start, tc.end, tc.total)
			if got.Start != tc.wantStart || got.End != tc.wantEnd || got.Count != tc.wantCount {
				t.Fatalf("ResolveFrameRange = %+v, want {%d %d %d}",
					got, tc.wantStart, tc.wantEnd, tc.wantCount)
			}
		})
	}
}
