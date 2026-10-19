//go:build !windows

package main

// cudaDevices returns no devices on platforms without the NVIDIA driver API.
func cudaDevices() []deviceInfo {
	return nil
}
