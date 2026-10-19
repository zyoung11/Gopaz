//go:build windows

package main

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

// ovAvailableDevices mirrors the ov_available_devices_t C struct.
type ovAvailableDevices struct {
	Devices uintptr
	Size    uintptr
}

// openVINODLL locates the OpenVINO C API library in the portable layout.
func openVINODLL() *syscall.LazyDLL {
	var bases []string
	if exe, err := os.Executable(); err == nil {
		bases = append(bases, filepath.Dir(exe))
	}
	if wd, err := os.Getwd(); err == nil {
		bases = append(bases, wd)
	}
	for _, base := range bases {
		for _, sub := range []string{filepath.Join("ort-dll", "openvino", "libs"), filepath.Join("ort-dll", "openvino")} {
			path := filepath.Join(base, sub, "openvino_c.dll")
			if _, err := os.Stat(path); err != nil {
				continue
			}
			dir := filepath.Dir(path)
			os.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
			dll := syscall.NewLazyDLL(path)
			err := dll.Load()
			if err == nil {
				Debugf("openvino runtime: %s", path)
				return dll
			}
			Debugf("openvino: load %s: %v", path, err)
		}
	}
	return nil
}

// openVINODevices lists OpenVINO devices with their full device names.
func openVINODevices() []deviceInfo {
	dll := openVINODLL()
	if dll == nil {
		return nil
	}
	coreCreate := dll.NewProc("ov_core_create")
	getAvailable := dll.NewProc("ov_core_get_available_devices")
	getProperty := dll.NewProc("ov_core_get_property")
	devicesFree := dll.NewProc("ov_available_devices_free")
	coreFree := dll.NewProc("ov_core_free")
	valueFree := dll.NewProc("ov_free")
	procs := []*syscall.LazyProc{coreCreate, getAvailable, getProperty, devicesFree, coreFree, valueFree}
	for _, p := range procs {
		if p.Find() != nil {
			Debugf("openvino: missing C API export in openvino_c.dll")
			return nil
		}
	}
	var core uintptr
	if st, _, _ := coreCreate.Call(uintptr(unsafe.Pointer(&core))); st != 0 || core == 0 {
		Debugf("openvino: core create failed: %d", st)
		return nil
	}
	defer coreFree.Call(core)
	var avail ovAvailableDevices
	if st, _, _ := getAvailable.Call(core, uintptr(unsafe.Pointer(&avail))); st != 0 || avail.Devices == 0 || avail.Size == 0 {
		Debugf("openvino: device query failed: %d", st)
		return nil
	}
	defer devicesFree.Call(uintptr(unsafe.Pointer(&avail)))
	names := unsafe.Slice((*uintptr)(unsafe.Pointer(avail.Devices)), avail.Size)
	var out []deviceInfo
	for _, slot := range names {
		id := byteStr(slot)
		if id == "" {
			continue
		}
		name := openVINOProperty(core, getProperty, valueFree, id, "FULL_DEVICE_NAME")
		if strings.HasPrefix(id, "GPU") {
			arch := openVINOProperty(core, getProperty, valueFree, id, "DEVICE_ARCHITECTURE")
			if !openVINOIsIntelGPU(arch, name) {
				Debugf("openvino: skip non-Intel device %s (%s)", id, name)
				continue
			}
		}
		out = append(out, deviceInfo{ID: id, Display: openVINODisplay(id, name)})
	}
	openVINOSort(out)
	return out
}

// openVINOProperty queries a string property of one OpenVINO device.
func openVINOProperty(core uintptr, getProperty, valueFree *syscall.LazyProc, id, key string) string {
	var value uintptr
	st, _, _ := getProperty.Call(
		core,
		uintptr(unsafe.Pointer(cstr(id))),
		uintptr(unsafe.Pointer(cstr(key))),
		uintptr(unsafe.Pointer(&value)),
	)
	if st != 0 || value == 0 {
		return ""
	}
	name := byteStr(value)
	valueFree.Call(value)
	return name
}
