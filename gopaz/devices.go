package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// deviceInfo describes a compute device with a stable id and a human readable name.
type deviceInfo struct {
	ID      string `json:"id"`
	Display string `json:"display"`
}

// formatVRAM formats a byte count as a compact memory label; empty for zero.
func formatVRAM(bytes uint64) string {
	const mb = 1024 * 1024
	const gb = mb * 1024
	switch {
	case bytes >= gb:
		tenths := (bytes*10 + gb/2) / gb
		if tenths%10 == 0 {
			return fmt.Sprintf("%dGB", tenths/10)
		}
		return fmt.Sprintf("%d.%dGB", tenths/10, tenths%10)
	case bytes >= mb:
		return fmt.Sprintf("%dMB", (bytes+mb/2)/mb)
	default:
		return ""
	}
}

// cudaDeviceDisplay builds the display label for a CUDA device.
func cudaDeviceDisplay(name string, bytes uint64) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "CUDA device"
	}
	if v := formatVRAM(bytes); v != "" {
		return fmt.Sprintf("%s (%s)", name, v)
	}
	return name
}

// openVINODisplay builds the display label for an OpenVINO device.
func openVINODisplay(id, name string) string {
	if strings.EqualFold(id, "CPU") {
		return "CPU"
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return id
	}
	return fmt.Sprintf("%s (%s)", name, id)
}

// openVINOIsIntelGPU reports whether an OpenVINO GPU device is Intel hardware.
func openVINOIsIntelGPU(arch, name string) bool {
	if arch != "" {
		return strings.Contains(arch, "vendor=0x8086")
	}
	return strings.HasPrefix(name, "Intel")
}

// openVINOSort orders OpenVINO devices as GPU, NPU, then CPU.
func openVINOSort(devices []deviceInfo) {
	sort.SliceStable(devices, func(i, j int) bool {
		ri, rj := openVINORank(devices[i].ID), openVINORank(devices[j].ID)
		if ri != rj {
			return ri < rj
		}
		return openVINOSuffix(devices[i].ID) < openVINOSuffix(devices[j].ID)
	})
}

// openVINOSuffix returns the numeric suffix of an OpenVINO device id.
func openVINOSuffix(id string) int {
	k := strings.LastIndexByte(id, '.')
	if k < 0 {
		return 0
	}
	n, err := strconv.Atoi(id[k+1:])
	if err != nil {
		return 0
	}
	return n
}

// openVINORank groups OpenVINO device ids for display ordering.
func openVINORank(id string) int {
	switch {
	case strings.HasPrefix(strings.ToUpper(id), "GPU"):
		return 0
	case strings.EqualFold(id, "NPU"):
		return 1
	default:
		return 2
	}
}
