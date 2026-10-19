//go:build windows

package main

import (
	"bytes"
	"fmt"
	"syscall"
	"unsafe"
)

// cudaDevices lists CUDA-capable GPUs through the NVIDIA driver API.
func cudaDevices() []deviceInfo {
	dll := syscall.NewLazyDLL("nvcuda.dll")
	if err := dll.Load(); err != nil {
		return nil
	}
	init := dll.NewProc("cuInit")
	count := dll.NewProc("cuDeviceGetCount")
	getName := dll.NewProc("cuDeviceGetName")
	totalMem := dll.NewProc("cuDeviceTotalMem_v2")
	if totalMem.Find() != nil {
		totalMem = dll.NewProc("cuDeviceTotalMem")
	}
	if init.Find() != nil || count.Find() != nil || getName.Find() != nil {
		return nil
	}
	if r, _, _ := init.Call(0); r != 0 {
		Debugf("cuda: cuInit failed: %d", r)
		return nil
	}
	var n int32
	if r, _, _ := count.Call(uintptr(unsafe.Pointer(&n))); r != 0 {
		Debugf("cuda: cuDeviceGetCount failed: %d", r)
		return nil
	}
	memOK := totalMem.Find() == nil
	var out []deviceInfo
	for i := int32(0); i < n; i++ {
		buf := make([]byte, 256)
		if r, _, _ := getName.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), uintptr(i)); r != 0 {
			continue
		}
		var mem uint64
		if memOK {
			totalMem.Call(uintptr(unsafe.Pointer(&mem)), uintptr(i))
		}
		name := buf
		if before, _, ok := bytes.Cut(buf, []byte{0}); ok {
			name = before
		}
		out = append(out, deviceInfo{
			ID:      fmt.Sprintf("%d", i),
			Display: cudaDeviceDisplay(string(name), mem),
		})
	}
	return out
}
