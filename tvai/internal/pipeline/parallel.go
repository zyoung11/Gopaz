package pipeline

import (
	"runtime"
	"sync"
)

// parallelRows 把 [0,n) 行区间按 CPU 核数切分并行执行 fn(start,end)。
// 用于加速逐像素热循环（降采样、tile 提取、融合、类型转换）。
func parallelRows(n int, fn func(start, end int)) {
	nw := runtime.NumCPU()
	if nw <= 1 || n <= 1 {
		fn(0, n)
		return
	}
	if nw > n {
		nw = n
	}
	chunk := (n + nw - 1) / nw
	var wg sync.WaitGroup
	for s := 0; s < n; s += chunk {
		e := min(s+chunk, n)
		wg.Go(func() {
			fn(s, e)
		})
	}
	wg.Wait()
}
