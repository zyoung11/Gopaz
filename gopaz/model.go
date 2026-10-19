package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Kind is the processing category of a model.
type Kind int

const (
	KindEnhance Kind = iota
	KindInterp
)

// DefaultBlockOverlap is the fallback block overlap when a model declares none.
const DefaultBlockOverlap = 48

// Param describes one tunable parameter's position and range in the params tensor.
type Param struct {
	// Key is the CLI name, for example "denoise".
	Key string
	// Tensor is the index in params[1,6,1,1]; -1 means the value is not packed into the tensor.
	Tensor int
	// GUIName is the display name used by the original GUI.
	GUIName string
	// Min is the lower bound in model space.
	Min float64
	// Max is the upper bound in model space.
	Max float64
}

// Model is the static description of one model family.
type Model struct {
	// Key is the short CLI name, for example "proteus".
	Key string
	// Short is the code used in file names, for example "prob".
	Short string
	// Version is the model version, for example "4".
	Version string
	Kind    Kind
	Display string
	// Scales lists supported upscale factors; empty for interpolation models.
	Scales []int
	// TileHW lists available tile sizes as (W, H).
	TileHW [][2]int
	// Params lists tunable parameters in tensor order.
	Params []Param
	// KeepColor reports whether the model requires keepColor correction.
	KeepColor bool
	// BlockOverlap is the blockOverlap from model JSON; 0 means use the default 48.
	BlockOverlap int
	// FilePrefix is the file name prefix used to build <prefix>-fp16-<H>x<W>[-<S>x]-ox.tz.
	FilePrefix string
	// FileHasScale reports whether the file name includes the scale segment.
	FileHasScale bool
	// SingleFile is the fixed file name for dynamic-size models such as Aion.
	SingleFile string
	// Temporal is the number of temporal input frames (Proteus=2, Gaia=5).
	Temporal int
	// CenterIdx is the index of the current frame inside the temporal window.
	CenterIdx int
	// OutFrames is the number of intermediate frames produced per inference.
	OutFrames int
	// BottomFile is the bottom model used by two-stage interpolation, such as aiob for Aion.
	BottomFile string
}

// proteusParams returns the six Proteus dimensions in tensor order.
func proteusParams() []Param {
	return []Param{
		{Key: "deblur", Tensor: 0, GUIName: "Antialias/DeBlur", Min: -1, Max: 1},
		{Key: "denoise", Tensor: 1, GUIName: "Reduce Noise", Min: -1, Max: 1},
		{Key: "detail", Tensor: 2, GUIName: "Recover Details", Min: -1, Max: 1},
		{Key: "dehalo", Tensor: 3, GUIName: "Dehalo", Min: -1, Max: 1},
		{Key: "sharpen", Tensor: 4, GUIName: "Sharpen", Min: -1, Max: 1},
		{Key: "fix-compression", Tensor: 5, GUIName: "Fix Compression", Min: -1, Max: 1},
	}
}

// enhanceTiles is the tile table shared by the enhancement models, as {W, H}.
var enhanceTiles = [][2]int{
	{288, 288}, {352, 256}, {480, 384}, {384, 480}, {384, 576},
	{416, 576}, {576, 448}, {672, 384}, {672, 576},
}

// apolloTiles is the Apollo tile table, which includes larger blocks, as {W, H}.
var apolloTiles = [][2]int{
	{288, 288}, {352, 256}, {480, 384}, {384, 480}, {384, 576},
	{416, 576}, {672, 384}, {576, 448}, {672, 576},
	{704, 1152}, {1056, 1088}, {1344, 1152},
}

// ModelRegistry returns every available model family indexed by CLI key.
func ModelRegistry() map[string]*Model {
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
			Display: "Aion", Scales: nil,
			Params: nil, SingleFile: "aion-v1-fp16-ox.tz",
			OutFrames: 16, BottomFile: "aiob-v1-fp16-ox.tz", BlockOverlap: 32,
		},
	}
}

// GetModel returns the model with the given key.
func GetModel(key string) (*Model, error) {
	m := ModelRegistry()[key]
	if m == nil {
		return nil, fmt.Errorf("unknown model %q, available: %s", key, strings.Join(ModelKeys(), ", "))
	}
	return m, nil
}

// ModelKeys returns every CLI model name in a stable order.
func ModelKeys() []string {
	return []string{"proteus", "gaia", "gaia-cg", "apollo", "aion"}
}

// TZPath locates the .onnx or .tz file for the given tile size and scale.
// Prepared ONNX files take precedence over encrypted .tz files.
// Passing tileW and tileH as zero selects a single-file model such as Aion.
func (m *Model) TZPath(modelDir string, tileW, tileH, scale int) (string, error) {
	return m.tzPathNamed(m.SingleFile, modelDir, tileW, tileH, scale)
}

// BottomTZPath locates the bottom model of a two-stage interpolation pipeline.
func (m *Model) BottomTZPath(modelDir string) (string, error) {
	if m.BottomFile == "" {
		return "", nil
	}
	return m.tzPathNamed(m.BottomFile, modelDir, 0, 0, 0)
}

func (m *Model) tzPathNamed(single string, modelDir string, tileW, tileH, scale int) (string, error) {
	if single != "" {
		return modelPathNamed(modelDir, single)
	}
	var name string
	if m.FileHasScale {
		name = fmt.Sprintf("%s-fp16-%dx%d-%dx-ox.tz", m.FilePrefix, tileH, tileW, scale)
	} else {
		name = fmt.Sprintf("%s-fp16-%dx%d-ox.tz", m.FilePrefix, tileH, tileW)
	}
	return modelPathNamed(modelDir, name)
}

// modelPathNamed prefers a prepared .onnx file over its .tz source.
func modelPathNamed(modelDir, name string) (string, error) {
	tz := filepath.Join(modelDir, name)
	onnx := strings.TrimSuffix(tz, ".tz") + ".onnx"
	if st, err := os.Stat(onnx); err == nil && st.Size() > 0 {
		return onnx, nil
	}
	if _, err := os.Stat(tz); err == nil {
		return tz, nil
	}
	return "", fmt.Errorf("model file missing: %s or %s", onnx, tz)
}

// HasParam reports whether the model supports the given parameter key.
func (m *Model) HasParam(key string) bool {
	for _, p := range m.Params {
		if p.Key == key {
			return true
		}
	}
	return false
}

// BlockOverlapOrDefault returns the model's block overlap or the default.
func (m *Model) BlockOverlapOrDefault() int {
	if m.BlockOverlap > 0 {
		return m.BlockOverlap
	}
	return DefaultBlockOverlap
}
