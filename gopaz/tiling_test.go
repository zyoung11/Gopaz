package main

import "testing"

func TestNPUTiles(t *testing.T) {
	tiles := [][2]int{{288, 288}, {672, 576}, {704, 1152}, {1056, 1088}, {1344, 1152}}
	got := npuTiles(tiles)
	want := [][2]int{{288, 288}, {672, 576}}
	if len(got) != len(want) {
		t.Fatalf("npuTiles returned %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("npuTiles[%d] = %v, want %v", i, got[i], want[i])
		}
	}
	if len(npuTiles(nil)) != 0 {
		t.Fatal("npuTiles(nil) should be empty")
	}
}

func TestNPUTilesApolloRegistry(t *testing.T) {
	m := ModelRegistry()["apollo"]
	capped := npuTiles(m.TileHW)
	if len(capped) == 0 || len(capped) >= len(m.TileHW) {
		t.Fatalf("apollo tiles %v -> %v", m.TileHW, capped)
	}
	for _, tile := range capped {
		if tile[0]*tile[1] > npuMaxTileArea {
			t.Fatalf("tile %v exceeds NPU limit", tile)
		}
	}
}
