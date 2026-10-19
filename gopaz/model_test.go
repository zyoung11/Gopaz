package main

import "testing"

func hasTile(tiles [][2]int, w, h int) bool {
	for _, t := range tiles {
		if t[0] == w && t[1] == h {
			return true
		}
	}
	return false
}

// TestModelDefaults checks model ranges, tile tables, and default overlap.
func TestModelDefaults(t *testing.T) {
	if len(ModelKeys()) != 5 {
		t.Fatalf("模型数量 = %d, want 5", len(ModelKeys()))
	}
	if !hasTile(ModelRegistry()["proteus"].TileHW, 672, 576) {
		t.Fatalf("proteus 缺少 672x576")
	}
	if !hasTile(ModelRegistry()["apollo"].TileHW, 1344, 1152) {
		t.Fatalf("apollo 缺少 1344x1152")
	}
	if got := ModelRegistry()["aion"].BlockOverlapOrDefault(); got != 32 {
		t.Fatalf("aion overlap = %d, want 32", got)
	}
	if got := ModelRegistry()["proteus"].BlockOverlapOrDefault(); got != 48 {
		t.Fatalf("proteus overlap = %d, want 48", got)
	}
}

// TestProteusParamRanges checks that every Proteus parameter allows negative values.
func TestProteusParamRanges(t *testing.T) {
	for _, p := range ModelRegistry()["proteus"].Params {
		if p.Min != -1 || p.Max != 1 {
			t.Fatalf("%s range = %v..%v, want -1..1", p.Key, p.Min, p.Max)
		}
	}
}
