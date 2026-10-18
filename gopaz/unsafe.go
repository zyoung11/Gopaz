package main

import "unsafe"

// ptrOf returns a pointer to the backing array of a float32 slice for creating
// ORT tensors.
func ptrOf(s []float32) unsafe.Pointer {
	return unsafe.Pointer(unsafe.SliceData(s))
}

// ptrOfU16 returns a pointer to the backing array of a uint16 slice.
func ptrOfU16(s []uint16) unsafe.Pointer {
	return unsafe.Pointer(unsafe.SliceData(s))
}

// unsafeSlice32 reinterprets a raw memory pointer as a float32 slice without copying.
func unsafeSlice32(p unsafe.Pointer, n int) []float32 {
	return unsafe.Slice((*float32)(p), n)
}

// unsafeSliceU16 reinterprets a raw memory pointer as a uint16 slice without copying.
func unsafeSliceU16(p unsafe.Pointer, n int) []uint16 {
	return unsafe.Slice((*uint16)(p), n)
}
