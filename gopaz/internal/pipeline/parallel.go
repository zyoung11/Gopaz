package pipeline

import (
	"runtime"
	"sync"
)

// parallelRows splits the row range [0,n) across CPU cores and runs fn(start,end)
// in parallel. It accelerates per-pixel hot loops such as downsampling, tile
// extraction, blending, and type conversion.
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
