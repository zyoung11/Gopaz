package pipeline

// FrameRange is the frame range actually processed. A Start or End of -1 leaves
// that end unbounded.
type FrameRange struct {
	Start int
	End   int
	Count int
}

// ResolveFrameRange turns CLI start and end frames into an effective range and count.
func ResolveFrameRange(start, end, total int) FrameRange {
	if start < 0 && end < 0 {
		return FrameRange{Start: -1, End: -1, Count: max(total, 0)}
	}
	s := max(start, 0)
	if total > 0 {
		if s >= total {
			return FrameRange{Start: s, End: -1, Count: 0}
		}
		e := end
		if e < 0 || e >= total {
			e = total - 1
		}
		if e < s {
			return FrameRange{Start: s, End: -1, Count: 0}
		}
		return FrameRange{Start: s, End: e, Count: e - s + 1}
	}
	if end < 0 || end < s {
		return FrameRange{Start: s, End: -1, Count: 0}
	}
	return FrameRange{Start: s, End: end, Count: end - s + 1}
}
