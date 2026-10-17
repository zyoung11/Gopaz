package pipeline

import (
	"math"
	"os/exec"
	"strconv"
	"strings"
)

const (
	minProcPixels         = 20000
	maxProcPixels         = 4000000
	dynamicTileAlign      = 32
	defaultAionTileBudget = 4.0
)

// chooseDynamicTile 按 Topaz 的显存预算计算动态 tile，返回 W,H。
func chooseDynamicTile(baseGB, pixelsPerGB, deviceGB float64, fw, fh int) (int, int) {
	if pixelsPerGB <= 0 || deviceGB <= 0 {
		return 0, 0
	}
	area := (math.Trunc(deviceGB) - baseGB) * pixelsPerGB
	if area < minProcPixels {
		area = minProcPixels
	}
	if area > maxProcPixels {
		area = maxProcPixels
	}
	side := roundUp32(int(math.Sqrt(area)))
	other := roundUp32(int(area) / side)
	if fw >= fh {
		return side, other
	}
	return other, side
}

// roundUp32 把数值向上对齐到 32 的倍数。
func roundUp32(v int) int {
	if v < dynamicTileAlign {
		return dynamicTileAlign
	}
	return (v + dynamicTileAlign - 1) / dynamicTileAlign * dynamicTileAlign
}

// detectVRAMGB 用 nvidia-smi 读取指定设备的显存总量。
func detectVRAMGB(device int) float64 {
	out, err := exec.Command("nvidia-smi", "--query-gpu=memory.total", "--format=csv,noheader,nounits").Output()
	if err != nil {
		return 0
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 0 {
		return 0
	}
	idx := device
	if idx < 0 || idx >= len(lines) {
		idx = 0
	}
	fields := strings.Fields(lines[idx])
	if len(fields) == 0 {
		return 0
	}
	mib, err := strconv.ParseFloat(strings.TrimSpace(fields[0]), 64)
	if err != nil {
		return 0
	}
	return mib / 1024
}
