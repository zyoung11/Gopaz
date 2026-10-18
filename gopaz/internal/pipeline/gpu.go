package pipeline

import (
	"os/exec"
	"strconv"
	"strings"
)

// detectFreeVRAMGB reads the free memory of the given device through nvidia-smi.
func detectFreeVRAMGB(device int) float64 {
	out, err := exec.Command("nvidia-smi", "--query-gpu=memory.free", "--format=csv,noheader,nounits").Output()
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
