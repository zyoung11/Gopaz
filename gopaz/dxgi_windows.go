//go:build windows

package main

import (
	"fmt"
	"strconv"
	"syscall"
	"unsafe"
)

const (
	dxgiEnumAdapters = 7
	dxgiGetDesc      = 8
	comReleaseMethod = 2
)

type dxgiGUID struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

type dxgiAdapterDesc struct {
	Description           [128]uint16
	VendorID              uint32
	DeviceID              uint32
	SubSysID              uint32
	Revision              uint32
	DedicatedVideoMemory  uintptr
	DedicatedSystemMemory uintptr
	SharedSystemMemory    uintptr
	AdapterLuid           uint64
}

var iidIDXGIFactory1 = dxgiGUID{
	Data1: 0x770aae78,
	Data2: 0xf26f,
	Data3: 0x4dba,
	Data4: [8]byte{0xa8, 0x29, 0x25, 0x3c, 0x83, 0xd1, 0xb3, 0x87},
}

// dmlDevices lists DXGI hardware adapters in DirectML device_id order.
func dmlDevices() []deviceInfo {
	factory, err := createDXGIFactory1()
	if err != nil {
		return nil
	}
	defer comRelease(factory)
	var out []deviceInfo
	for i := 0; ; i++ {
		var adapter uintptr
		hr, _, _ := comCall(factory, dxgiEnumAdapters, uintptr(i), uintptr(unsafe.Pointer(&adapter)))
		if int32(hr) < 0 || adapter == 0 {
			break
		}
		var desc dxgiAdapterDesc
		hr, _, _ = comCall(adapter, dxgiGetDesc, uintptr(unsafe.Pointer(&desc)))
		if int32(hr) >= 0 {
			name := syscall.UTF16ToString(desc.Description[:])
			Debugf("dml adapter %d: name=%q dedicated=%d system=%d shared=%d vendor=0x%04x", i, name, uint64(desc.DedicatedVideoMemory), uint64(desc.DedicatedSystemMemory), uint64(desc.SharedSystemMemory), desc.VendorID)
			if desc.VendorID == 0x1414 {
				comRelease(adapter)
				continue
			}
			if name == "" {
				name = fmt.Sprintf("GPU %d", i)
			}
			if desc.DedicatedVideoMemory >= 1<<30 {
				name += " (" + formatVRAM(uint64(desc.DedicatedVideoMemory)) + ")"
			}
			out = append(out, deviceInfo{ID: strconv.Itoa(i), Display: name})
		}
		comRelease(adapter)
	}
	return out
}

// createDXGIFactory1 creates the DXGI factory used for adapter enumeration.
func createDXGIFactory1() (uintptr, error) {
	proc := syscall.NewLazyDLL("dxgi.dll").NewProc("CreateDXGIFactory1")
	var factory uintptr
	hr, _, _ := proc.Call(
		uintptr(unsafe.Pointer(&iidIDXGIFactory1)),
		uintptr(unsafe.Pointer(&factory)),
	)
	if int32(hr) < 0 || factory == 0 {
		return 0, fmt.Errorf("CreateDXGIFactory1 failed: 0x%08x", uint32(hr))
	}
	return factory, nil
}

// comCall invokes one COM vtable method.
//
//go:uintptrescapes
//go:nocheckptr
func comCall(iface, method uintptr, args ...uintptr) (uintptr, uintptr, syscall.Errno) {
	vtable := *(*uintptr)(unsafe.Pointer(iface))
	proc := *(*uintptr)(unsafe.Add(unsafe.Pointer(vtable), method*unsafe.Sizeof(uintptr(0))))
	all := make([]uintptr, 0, len(args)+1)
	all = append(all, iface)
	all = append(all, args...)
	return syscall.SyscallN(proc, all...)
}

// comRelease releases a COM interface.
func comRelease(iface uintptr) {
	if iface != 0 {
		comCall(iface, comReleaseMethod)
	}
}
