package main

import (
	"math"
	"testing"
)

func TestDuplicateThreshold(t *testing.T) {
	cases := []struct {
		sensitivity int
		want        float64
	}{
		{-10, 0},
		{0, 0},
		{60, 0.00012},
		{100, 0.0002},
	}
	for _, c := range cases {
		got := duplicateThreshold(c.sensitivity)
		if math.Abs(got-c.want) > 1e-12 {
			t.Fatalf("duplicateThreshold(%d) = %v, want %v", c.sensitivity, got, c.want)
		}
	}
}
