package model

import "testing"

func hasTile(tiles [][2]int, w, h int) bool {
	for _, t := range tiles {
		if t[0] == w && t[1] == h {
			return true
		}
	}
	return false
}

// TestModelDefaults 验证四模型范围、tile 表与默认重叠。
func TestModelDefaults(t *testing.T) {
	if len(Keys()) != 5 {
		t.Fatalf("模型数量 = %d, want 5", len(Keys()))
	}
	if !hasTile(Registry()["proteus"].TileHW, 672, 576) {
		t.Fatalf("proteus 缺少 672x576")
	}
	if !hasTile(Registry()["apollo"].TileHW, 1344, 1152) {
		t.Fatalf("apollo 缺少 1344x1152")
	}
	if got := Registry()["aion"].BlockOverlapOrDefault(); got != 32 {
		t.Fatalf("aion overlap = %d, want 32", got)
	}
	if got := Registry()["proteus"].BlockOverlapOrDefault(); got != 48 {
		t.Fatalf("proteus overlap = %d, want 48", got)
	}
	aion := Registry()["aion"]
	if aion.BaseGB != 1.2 || aion.PixelsPerGB != 120000 {
		t.Fatalf("aion budget = %.1f/%.0f, want 1.2/120000", aion.BaseGB, aion.PixelsPerGB)
	}
}
