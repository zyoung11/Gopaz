package pipeline

import "unsafe"

// ptrOf 返回 float32 切片底层数组的指针，供 ORT 张量创建使用。
func ptrOf(s []float32) unsafe.Pointer {
	return unsafe.Pointer(unsafe.SliceData(s))
}

// ptrOfU16 返回 uint16 切片底层数组的指针。
func ptrOfU16(s []uint16) unsafe.Pointer {
	return unsafe.Pointer(unsafe.SliceData(s))
}

// unsafeSlice32 把原始内存指针解释为 float32 切片（不拷贝）。
func unsafeSlice32(p unsafe.Pointer, n int) []float32 {
	return unsafe.Slice((*float32)(p), n)
}

// unsafeSliceU16 把原始内存指针解释为 uint16 切片（不拷贝）。
func unsafeSliceU16(p unsafe.Pointer, n int) []uint16 {
	return unsafe.Slice((*uint16)(p), n)
}
