// Package model 描述 Topaz 模型家族的元数据：文件命名规则、tile 尺寸、
// 可调参数及其顺序。这些信息取自 App/Topaz Labs LLC/.../models/*.json。
package model

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Kind 是模型的处理类别。
type Kind int

const (
	KindEnhance Kind = iota // 超分 / 修复
	KindInterp              // 插帧 / 慢放
)

// DefaultBlockOverlap 是模型未声明 blockOverlap 时的默认分块重叠。
const DefaultBlockOverlap = 48

// Param 描述一个可调参数在 params 张量中的位置与取值范围。
type Param struct {
	Key     string  // CLI 名（如 "denoise"）
	Tensor  int     // 在 params[1,6,1,1] 中的索引；-1 表示不进张量
	GUIName string  // GUI 显示名
	Min     float64 // 模型空间下限
	Max     float64 // 模型空间上限
}

// Model 是一个模型家族的静态描述。
type Model struct {
	Key     string // CLI 用的短名，如 "proteus"
	Short   string // 文件名中的短码，如 "prob"
	Version string // 版本，如 "4"
	Kind    Kind
	Display string
	Scales  []int    // 支持的放大倍率；插帧模型为空
	TileHW  [][2]int // 可用 tile 尺寸 (W,H)
	Params  []Param  // 可调参数（按张量顺序）
	// 模型是否要求 keepColor 色彩校正（JSON keepColor）。
	KeepColor bool
	// 模型 JSON 的 blockOverlap；0 表示使用默认 48。
	BlockOverlap int
	// 文件名前缀，用于拼接 <prefix>-fp16-<H>x<W>[-<S>x]-ox.tz
	FilePrefix string
	// 是否在文件名中含 scale 段（增强模型含，插帧模型不含）
	FileHasScale bool
	// 是否为单文件动态尺寸（Aion/Aion Bottom）
	SingleFile string
	// 时序输入帧数（增强模型：Proteus=2 当前+前一；Gaia=5）
	Temporal int
	// 当前帧在时序窗口中的下标
	CenterIdx int
	// 插帧：一次推理产出的中间帧数
	OutFrames int
	// 插帧：两级结构时的底部模型文件（如 Aion 需先用 aiob 产 8 帧作 in8x）
	BottomFile string
}

// Pro Params 是 Proteus 的 6 维参数（张量顺序 preBlur,noise,details,halo,blur,compression）。
func proteusParams() []Param {
	return []Param{
		{Key: "deblur", Tensor: 0, GUIName: "Antialias/DeBlur", Min: -1, Max: 1},
		{Key: "denoise", Tensor: 1, GUIName: "Reduce Noise", Min: 0, Max: 1},
		{Key: "detail", Tensor: 2, GUIName: "Recover Details", Min: 0, Max: 1},
		{Key: "dehalo", Tensor: 3, GUIName: "Dehalo", Min: 0, Max: 1},
		{Key: "sharpen", Tensor: 4, GUIName: "Sharpen", Min: 0, Max: 1},
		{Key: "fix-compression", Tensor: 5, GUIName: "Fix Compression", Min: 0, Max: 1},
	}
}

// 增强模型共用的 tile 尺寸。每个元素为 {W, H}（与 JSON blocks 顺序一致）。
var enhanceTiles = [][2]int{
	{288, 288}, {352, 256}, {480, 384}, {384, 480}, {384, 576},
	{416, 576}, {576, 448}, {672, 384}, {672, 576},
}

// Apollo 的 tile 尺寸（含更大块）。元素为 {W, H}。
var apolloTiles = [][2]int{
	{288, 288}, {352, 256}, {480, 384}, {384, 480}, {384, 576},
	{416, 576}, {672, 384}, {576, 448}, {672, 576},
	{704, 1152}, {1056, 1088}, {1344, 1152},
}

// 插帧模型（Aion）的 tile 尺寸，均为 32 的倍数（模型内部约束）。实测小 tile（接近
// 训练尺寸）比大 tile 更快，故以中小尺寸为主。元素为 {W, H}。
var interpTiles = [][2]int{
	{256, 256}, {384, 384}, {384, 480}, {480, 384}, {384, 576}, {576, 384}, {512, 512},
}

// Registry 返回全部可用模型家族，按 CLI Key 索引。
func Registry() map[string]*Model {
	return map[string]*Model{
		"proteus": {
			Key: "proteus", Short: "prob", Version: "4", Kind: KindEnhance,
			Display: "Proteus", Scales: []int{1, 2, 4}, TileHW: enhanceTiles,
			Params: proteusParams(), FilePrefix: "prob-v4-fgnet", FileHasScale: true,
			Temporal: 2, CenterIdx: 1, KeepColor: true,
		},
		"gaia": {
			Key: "gaia", Short: "ghq", Version: "5", Kind: KindEnhance,
			Display: "Gaia (High Quality)", Scales: []int{1, 2, 4}, TileHW: enhanceTiles,
			Params: nil, FilePrefix: "ghq-v5-gaia", FileHasScale: true,
			Temporal: 5, CenterIdx: 2, KeepColor: true,
		},
		"gaia-cg": {
			Key: "gaia-cg", Short: "gcg", Version: "5", Kind: KindEnhance,
			Display: "Gaia (Computer Generated)", Scales: []int{1, 2, 4}, TileHW: enhanceTiles,
			Params: nil, FilePrefix: "gcg-v5-gaia", FileHasScale: true,
			Temporal: 5, CenterIdx: 2, KeepColor: true,
		},
		"apollo": {
			Key: "apollo", Short: "apo", Version: "8", Kind: KindInterp,
			Display: "Apollo", Scales: nil, TileHW: apolloTiles,
			Params: nil, FilePrefix: "apo-v8", FileHasScale: false,
			OutFrames: 8,
		},
		"aion": {
			Key: "aion", Short: "aion", Version: "1", Kind: KindInterp,
			Display: "Aion", Scales: nil, TileHW: interpTiles,
			Params: nil, SingleFile: "aion-v1-fp16-ox.tz",
			OutFrames: 16, BottomFile: "aiob-v1-fp16-ox.tz", BlockOverlap: 32,
		},
	}
}

// Get 返回指定 key 的模型，不存在则报错。
func Get(key string) (*Model, error) {
	m := Registry()[key]
	if m == nil {
		return nil, fmt.Errorf("未知模型 %q，可用: %s", key, strings.Join(Keys(), ", "))
	}
	return m, nil
}

// Keys 返回全部 CLI 模型名（有序）。
func Keys() []string {
	return []string{"proteus", "gaia", "gaia-cg", "apollo", "aion"}
}

// TZPath 在 modelDir 中定位该模型在给定 tile 尺寸与倍率下的 .tz 文件路径。
// tileW/tileH 传 0 表示单文件模型（Aion）。文件名按 HxW 约定生成。
func (m *Model) TZPath(modelDir string, tileW, tileH, scale int) (string, error) {
	return m.tzPathNamed(m.SingleFile, modelDir, tileW, tileH, scale)
}

// BottomTZPath 定位底部模型（两级插帧）的 .tz 路径，无则返回空。
func (m *Model) BottomTZPath(modelDir string) (string, error) {
	if m.BottomFile == "" {
		return "", nil
	}
	return m.tzPathNamed(m.BottomFile, modelDir, 0, 0, 0)
}

func (m *Model) tzPathNamed(single string, modelDir string, tileW, tileH, scale int) (string, error) {
	if single != "" {
		p := filepath.Join(modelDir, single)
		if _, err := os.Stat(p); err != nil {
			return "", fmt.Errorf("模型文件缺失: %s", p)
		}
		return p, nil
	}
	var name string
	if m.FileHasScale {
		name = fmt.Sprintf("%s-fp16-%dx%d-%dx-ox.tz", m.FilePrefix, tileH, tileW, scale)
	} else {
		name = fmt.Sprintf("%s-fp16-%dx%d-ox.tz", m.FilePrefix, tileH, tileW)
	}
	p := filepath.Join(modelDir, name)
	if _, err := os.Stat(p); err != nil {
		return "", fmt.Errorf("模型文件缺失: %s", p)
	}
	return p, nil
}

// HasParam 判断某参数 key 是否被该模型支持。
func (m *Model) HasParam(key string) bool {
	for _, p := range m.Params {
		if p.Key == key {
			return true
		}
	}
	return false
}

// BlockOverlapOrDefault 返回模型的分块重叠，未声明时使用默认值。
func (m *Model) BlockOverlapOrDefault() int {
	if m.BlockOverlap > 0 {
		return m.BlockOverlap
	}
	return DefaultBlockOverlap
}
