package pipeline

// FrameRange 描述实际处理的帧范围。Start/End 为 -1 表示不限制。
type FrameRange struct {
	Start int
	End   int
	Count int
}

// ResolveFrameRange 把 CLI 的起止帧解析成有效范围和帧数。
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
