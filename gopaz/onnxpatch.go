package main

import (
	"errors"
	"fmt"
)

type pbField struct {
	num  int
	wire int
	// raw holds the payload for wire type 2, varint bytes for 0, 8 bytes for 1,
	// and 4 bytes for 5.
	raw []byte
}

func readVarint(b []byte) (uint64, int) {
	var v uint64
	var s uint
	for i := range b {
		c := b[i]
		v |= uint64(c&0x7f) << s
		if c&0x80 == 0 {
			return v, i + 1
		}
		s += 7
		if s > 63 {
			return 0, 0
		}
	}
	return 0, 0
}

func appendVarint(b []byte, v uint64) []byte {
	for v >= 0x80 {
		b = append(b, byte(v)|0x80)
		v >>= 7
	}
	return append(b, byte(v))
}

func parseFields(b []byte) ([]pbField, error) {
	var out []pbField
	i := 0
	for i < len(b) {
		key, n := readVarint(b[i:])
		if n == 0 {
			return nil, errors.New("onnxpatch: bad varint")
		}
		i += n
		fn := int(key >> 3)
		wt := int(key & 7)
		if fn == 0 {
			return nil, errors.New("onnxpatch: field number is zero")
		}
		switch wt {
		case 0:
			st := i
			for i < len(b) && b[i]&0x80 != 0 {
				i++
			}
			if i >= len(b) {
				return nil, errors.New("onnxpatch: varint out of bounds")
			}
			i++
			out = append(out, pbField{fn, wt, b[st:i]})
		case 1:
			if i+8 > len(b) {
				return nil, errors.New("onnxpatch: fixed64 out of bounds")
			}
			out = append(out, pbField{fn, wt, b[i : i+8]})
			i += 8
		case 2:
			l, n := readVarint(b[i:])
			if n == 0 || i+n+int(l) > len(b) {
				return nil, errors.New("onnxpatch: length-delimited field out of bounds")
			}
			i += n
			out = append(out, pbField{fn, wt, b[i : i+int(l)]})
			i += int(l)
		case 5:
			if i+4 > len(b) {
				return nil, errors.New("onnxpatch: fixed32 out of bounds")
			}
			out = append(out, pbField{fn, wt, b[i : i+4]})
			i += 4
		default:
			return nil, fmt.Errorf("onnxpatch: unknown wire type %d", wt)
		}
	}
	return out, nil
}

func writeFields(fs []pbField) []byte {
	var buf []byte
	for _, f := range fs {
		buf = appendVarint(buf, uint64(f.num<<3|f.wire))
		if f.wire == 2 {
			buf = appendVarint(buf, uint64(len(f.raw)))
		}
		buf = append(buf, f.raw...)
	}
	return buf
}

func varintVal(raw []byte) uint64 {
	v, _ := readVarint(raw)
	return v
}

func strField(fs []pbField, num int) (string, bool) {
	for _, f := range fs {
		if f.num == num && f.wire == 2 {
			return string(f.raw), true
		}
	}
	return "", false
}

// setFirstString replaces the string value of the first length-delimited field
// with the given number.
func setFirstString(fs []pbField, num int, val string) []pbField {
	for i := range fs {
		if fs[i].num == num && fs[i].wire == 2 {
			fs[i].raw = []byte(val)
			return fs
		}
	}
	return append(fs, pbField{num, 2, []byte(val)})
}

const (
	// wdFloat is TensorProto.FLOAT.
	wdFloat = 1
	// wdFloat16 is TensorProto.FLOAT16.
	wdFloat16 = 10
	// atInt is AttributeProto.INT.
	atInt = 2
)

// PatchReduceMeanFp32 detects whether the model is fp16 and, if so, wraps every
// ReduceMean node in fp32 Cast nodes. An fp32 model is returned unchanged.
func PatchReduceMeanFp32(model []byte) ([]byte, bool, error) {
	mf, err := parseFields(model)
	if err != nil {
		return nil, false, err
	}
	gi := -1
	for i, f := range mf {
		if f.num == 7 && f.wire == 2 {
			gi = i
			break
		}
	}
	if gi < 0 {
		return nil, false, errors.New("onnxpatch: graph field not found")
	}
	gf, err := parseFields(mf[gi].raw)
	if err != nil {
		return nil, false, err
	}
	nameType := make(map[string]int)
	for _, f := range gf {
		if f.num == 11 || f.num == 12 || f.num == 13 {
			if nm, et, ok := valueInfoNameType(f.raw); ok {
				nameType[nm] = et
			}
		}
	}

	var newGraph []pbField
	count := 0
	idx := 0
	for _, f := range gf {
		if f.num != 1 || f.wire != 2 {
			newGraph = append(newGraph, f)
			continue
		}
		idx++
		nf, err := parseFields(f.raw)
		if err != nil {
			return nil, false, err
		}
		op, _ := strField(nf, 4)
		if op != "ReduceMean" {
			newGraph = append(newGraph, f)
			continue
		}
		in0, ok1 := firstString(nf, 1)
		out0, ok2 := firstString(nf, 2)
		if !ok1 || !ok2 {
			newGraph = append(newGraph, f)
			continue
		}
		if nameType[out0] != wdFloat16 {
			newGraph = append(newGraph, f)
			continue
		}
		count++
		midIn := fmt.Sprintf("%s__rmf32i%d", in0, idx)
		midOut := fmt.Sprintf("%s__rmf32o%d", out0, idx)

		c1, err := buildCastNode(in0, midIn, wdFloat)
		if err != nil {
			return nil, false, err
		}
		rm := setFirstString(copyFields(nf), 1, midIn)
		rm = setFirstString(rm, 2, midOut)
		c2, err := buildCastNode(midOut, out0, wdFloat16)
		if err != nil {
			return nil, false, err
		}
		newGraph = append(newGraph, pbField{1, 2, c1}, pbField{1, 2, writeFields(rm)}, pbField{1, 2, c2})
	}
	if count == 0 {
		return nil, false, nil
	}
	mf[gi].raw = writeFields(newGraph)
	return writeFields(mf), true, nil
}

// valueInfoNameType extracts the tensor name and elem_type from a ValueInfoProto.
func valueInfoNameType(vi []byte) (string, int, bool) {
	vf, err := parseFields(vi)
	if err != nil {
		return "", 0, false
	}
	name, _ := strField(vf, 1)
	for _, f := range vf {
		if f.num == 2 && f.wire == 2 {
			tf, err := parseFields(f.raw)
			if err != nil {
				return name, 0, false
			}
			for _, g := range tf {
				if g.num == 1 && g.wire == 2 {
					tt, err := parseFields(g.raw)
					if err != nil {
						return name, 0, false
					}
					for _, h := range tt {
						if h.num == 1 && h.wire == 0 {
							return name, int(varintVal(h.raw)), true
						}
					}
				}
			}
		}
	}
	return name, 0, false
}

// PatchOutputFp16 rewrites the final Cast before a graph output to fp16 and
// updates the declared output type.
func PatchOutputFp16(model []byte) ([]byte, bool, error) {
	mf, err := parseFields(model)
	if err != nil {
		return nil, false, err
	}
	gi := -1
	for i, f := range mf {
		if f.num == 7 && f.wire == 2 {
			gi = i
			break
		}
	}
	if gi < 0 {
		return nil, false, errors.New("onnxpatch: graph field not found")
	}
	gf, err := parseFields(mf[gi].raw)
	if err != nil {
		return nil, false, err
	}
	nameType := make(map[string]int)
	outputs := make(map[string]bool)
	for _, f := range gf {
		if f.num == 11 || f.num == 12 || f.num == 13 {
			if nm, et, ok := valueInfoNameType(f.raw); ok {
				nameType[nm] = et
			}
		}
		if f.num == 12 && f.wire == 2 {
			if nm, _, ok := valueInfoNameType(f.raw); ok {
				outputs[nm] = true
			}
		}
	}
	changed := false
	for i := range gf {
		if gf[i].num != 1 || gf[i].wire != 2 {
			continue
		}
		nf, err := parseFields(gf[i].raw)
		if err != nil {
			continue
		}
		op, _ := strField(nf, 4)
		if op != "Cast" {
			continue
		}
		out0, ok := strField(nf, 2)
		if !ok || !outputs[out0] {
			continue
		}
		in0, _ := strField(nf, 1)
		if nameType[in0] != wdFloat16 {
			continue
		}
		patched := false
		for j := range nf {
			if nf[j].num != 5 || nf[j].wire != 2 {
				continue
			}
			af, err := parseFields(nf[j].raw)
			if err != nil {
				continue
			}
			if nm, _ := strField(af, 1); nm != "to" {
				continue
			}
			for k := range af {
				if af[k].num == 3 && af[k].wire == 0 && varintVal(af[k].raw) == wdFloat {
					af[k].raw = appendVarint(nil, wdFloat16)
					nf[j].raw = writeFields(af)
					patched = true
				}
			}
		}
		if !patched {
			continue
		}
		gf[i].raw = writeFields(nf)
		changed = true
	}
	if !changed {
		return nil, false, nil
	}
	for i := range gf {
		if (gf[i].num != 12 && gf[i].num != 13) || gf[i].wire != 2 {
			continue
		}
		vi, err := parseFields(gf[i].raw)
		if err != nil {
			continue
		}
		if nm, _, ok := valueInfoNameType(gf[i].raw); !ok || !outputs[nm] {
			continue
		}
		if nvi, ok := setValueInfoElemType(vi, wdFloat16); ok {
			gf[i].raw = writeFields(nvi)
		}
	}
	mf[gi].raw = writeFields(gf)
	return writeFields(mf), true, nil
}

func setValueInfoElemType(vi []pbField, et int) ([]pbField, bool) {
	for i := range vi {
		if vi[i].num != 2 || vi[i].wire != 2 {
			continue
		}
		tf, err := parseFields(vi[i].raw)
		if err != nil {
			return vi, false
		}
		for j := range tf {
			if tf[j].num != 1 || tf[j].wire != 2 {
				continue
			}
			tt, err := parseFields(tf[j].raw)
			if err != nil {
				return vi, false
			}
			for k := range tt {
				if tt[k].num == 1 && tt[k].wire == 0 {
					tt[k].raw = appendVarint(nil, uint64(et))
					tf[j].raw = writeFields(tt)
					vi[i].raw = writeFields(tf)
					return vi, true
				}
			}
		}
	}
	return vi, false
}

// buildCastNode builds the NodeProto bytes for a Cast node.
func buildCastNode(in, out string, toType int) ([]byte, error) {
	attr := []pbField{
		{1, 2, []byte("to")},
		{3, 0, appendVarint(nil, uint64(toType))},
		{20, 0, appendVarint(nil, atInt)},
	}
	node := []pbField{
		{1, 2, []byte(in)},
		{2, 2, []byte(out)},
		{4, 2, []byte("Cast")},
		{5, 2, writeFields(attr)},
	}
	return writeFields(node), nil
}

// firstString returns the first length-delimited string field with the given number.
func firstString(fs []pbField, num int) (string, bool) {
	return strField(fs, num)
}

func copyFields(fs []pbField) []pbField {
	out := make([]pbField, len(fs))
	copy(out, fs)
	return out
}
