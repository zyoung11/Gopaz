// Package onnxpatch 对 ONNX 模型做最小化的 protobuf 级修改。
//
// 目标：修复 onnxruntime CUDA 上 fp16 ReduceMean 大张量归约时累加器溢出
// 导致的 NaN（求和超过 fp16 上限 65504）。做法是在每个 fp16 ReduceMean
// 前后插入 Cast，使其以 fp32 累加后再转回 fp16。
//
// 实现方式为字段级 protobuf 解析/重写：只改动节点列表，其余字段按原样
// 保留，避免破坏模型的其它部分。
package onnxpatch

import (
	"errors"
	"fmt"
)

// ---- protobuf 基础 ----

type pbField struct {
	num  int
	wire int
	raw  []byte // wire2: 负载内容；wire0: varint 字节；wire1: 8字节；wire5: 4字节
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
			return nil, errors.New("onnxpatch: varint 解析失败")
		}
		i += n
		fn := int(key >> 3)
		wt := int(key & 7)
		if fn == 0 {
			return nil, errors.New("onnxpatch: 字段号为 0")
		}
		switch wt {
		case 0:
			st := i
			for i < len(b) && b[i]&0x80 != 0 {
				i++
			}
			if i >= len(b) {
				return nil, errors.New("onnxpatch: varint 越界")
			}
			i++
			out = append(out, pbField{fn, wt, b[st:i]})
		case 1:
			if i+8 > len(b) {
				return nil, errors.New("onnxpatch: fixed64 越界")
			}
			out = append(out, pbField{fn, wt, b[i : i+8]})
			i += 8
		case 2:
			l, n := readVarint(b[i:])
			if n == 0 || i+n+int(l) > len(b) {
				return nil, errors.New("onnxpatch: 长度域越界")
			}
			i += n
			out = append(out, pbField{fn, wt, b[i : i+int(l)]})
			i += int(l)
		case 5:
			if i+4 > len(b) {
				return nil, errors.New("onnxpatch: fixed32 越界")
			}
			out = append(out, pbField{fn, wt, b[i : i+4]})
			i += 4
		default:
			return nil, fmt.Errorf("onnxpatch: 未知 wire type %d", wt)
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

// setFirstString 替换指定编号的首个长度域字段的字符串值。
func setFirstString(fs []pbField, num int, val string) []pbField {
	for i := range fs {
		if fs[i].num == num && fs[i].wire == 2 {
			fs[i].raw = []byte(val)
			return fs
		}
	}
	return append(fs, pbField{num, 2, []byte(val)})
}

// ---- 顶层：ReduceMean fp32 化 ----

const (
	wdFloat   = 1  // TensorProto.FLOAT
	wdFloat16 = 10 // TensorProto.FLOAT16
	atInt     = 2  // AttributeProto.INT
)

// PatchReduceMeanFp32 检测模型是否为 fp16；若是，则为每个 ReduceMean 节点
// 插入 fp32 Cast 以避免 CUDA 归约溢出。fp32 模型直接原样返回（无溢出问题）。
func PatchReduceMeanFp32(model []byte) ([]byte, bool, error) {
	mf, err := parseFields(model)
	if err != nil {
		return nil, false, err
	}
	// 定位 graph（field 7, wire 2）
	gi := -1
	for i, f := range mf {
		if f.num == 7 && f.wire == 2 {
			gi = i
			break
		}
	}
	if gi < 0 {
		return nil, false, errors.New("onnxpatch: 未找到 graph 字段")
	}
	gf, err := parseFields(mf[gi].raw)
	if err != nil {
		return nil, false, err
	}
	// 建立 name -> elem_type 映射（来自 input/output/value_info）。
	nameType := make(map[string]int)
	for _, f := range gf {
		if f.num == 11 || f.num == 12 || f.num == 13 {
			if nm, et, ok := valueInfoNameType(f.raw); ok {
				nameType[nm] = et
			}
		}
	}

	// 重建节点列表：field 1 (node)。
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
		// 仅修补输出为 fp16 的 ReduceMean（fp16 累加溢出）。fp32 输出无需修补。
		if nameType[out0] != wdFloat16 {
			newGraph = append(newGraph, f)
			continue
		}
		count++
		midIn := fmt.Sprintf("%s__rmf32i%d", in0, idx)
		midOut := fmt.Sprintf("%s__rmf32o%d", out0, idx)

		// 1) Cast(in0 -> midIn) to=FLOAT
		c1, err := buildCastNode(in0, midIn, wdFloat)
		if err != nil {
			return nil, false, err
		}
		// 2) ReduceMean(midIn -> midOut)：复用原节点，改写首输入/输出
		rm := setFirstString(copyFields(nf), 1, midIn)
		rm = setFirstString(rm, 2, midOut)
		// 3) Cast(midOut -> out0) to=FLOAT16（保持原输出名，下游无感知）
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

// valueInfoNameType 从 ValueInfoProto 中解析出张量名与 elem_type。
func valueInfoNameType(vi []byte) (string, int, bool) {
	vf, err := parseFields(vi)
	if err != nil {
		return "", 0, false
	}
	name, _ := strField(vf, 1)
	for _, f := range vf {
		if f.num == 2 && f.wire == 2 { // type: TypeProto
			tf, err := parseFields(f.raw)
			if err != nil {
				return name, 0, false
			}
			for _, g := range tf {
				if g.num == 1 && g.wire == 2 { // tensor_type
					tt, err := parseFields(g.raw)
					if err != nil {
						return name, 0, false
					}
					for _, h := range tt {
						if h.num == 1 && h.wire == 0 { // elem_type
							return name, int(varintVal(h.raw)), true
						}
					}
				}
			}
		}
	}
	return name, 0, false
}

// buildCastNode 构造一个 Cast 节点的 NodeProto 字节。
func buildCastNode(in, out string, toType int) ([]byte, error) {
	// AttributeProto: name(1)="to", i(3)=toType, type(20)=INT
	attr := []pbField{
		{1, 2, []byte("to")},
		{3, 0, appendVarint(nil, uint64(toType))},
		{20, 0, appendVarint(nil, atInt)},
	}
	node := []pbField{
		{1, 2, []byte(in)},        // input
		{2, 2, []byte(out)},       // output
		{4, 2, []byte("Cast")},    // op_type
		{5, 2, writeFields(attr)}, // attribute
	}
	return writeFields(node), nil
}

// firstString 取首个指定编号的长度域字符串。
func firstString(fs []pbField, num int) (string, bool) {
	return strField(fs, num)
}

func copyFields(fs []pbField) []pbField {
	out := make([]pbField, len(fs))
	copy(out, fs)
	return out
}
