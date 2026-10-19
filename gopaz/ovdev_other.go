//go:build !windows

package main

// openVINODevices returns no devices on platforms without the OpenVINO runtime.
func openVINODevices() []deviceInfo {
	return nil
}
