//go:build !windows

package main

// dmlDevices returns no adapters on platforms without DXGI.
func dmlDevices() []deviceInfo {
	return nil
}
