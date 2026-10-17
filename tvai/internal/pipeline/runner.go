package pipeline

import (
	"fmt"
	"math"
	"os"
	"path/filepath"

	"tvai/internal/log"
	"tvai/internal/model"
	"tvai/internal/onnxpatch"
	"tvai/internal/ort"
	"tvai/internal/tz"
)

// UpscaleConfig 是超分/修复的完整配置。
type UpscaleConfig struct {
	Model      *model.Model
	ModelDir   string // .tz 文件目录
	Scale      int    // 1/2/4
	FW, FH     int    // 输入帧尺寸
	Params     map[string]float64 // GUI 空间参数（0-100 或 -100-100）
	Overlap    int    // 分块重叠（输入空间）
	GPU        int    // -2 自动 / -1 CPU / 0+ 设备号
	VRAM       float64
	Threads    int
	WorkDir    string // 临时解包目录
}

// Runner 持有一个已加载的模型会话。
type Runner struct {
	cfg  UpscaleConfig
	sess *ort.Session
	inputs  []ort.IOInfo
	outputs []ort.IOInfo
	tileH, tileW int
	paramVec []float64 // 长度 6
	sceneThreshold float64
	// 时序状态：上一帧的输出（输出分辨率）与上一帧输入（输入分辨率）。
	prevHR   *Frame
	prevIn   *Frame
}

// outputName 返回模型首个输出的张量名。
func (r *Runner) outputName() string {
	if len(r.outputs) > 0 {
		return r.outputs[0].Name
	}
	return "out"
}

// SetPrevOutput 记录当前帧输出，供下一帧的 hr_prev 使用。
func (r *Runner) SetPrevOutput(f *Frame) { r.prevHR = f }

// ResetTemporal 重置时序状态（场景切换时调用）。base 为当前帧的双线性放大结果。
func (r *Runner) ResetTemporal(base *Frame) {
	r.prevHR = base
}

// LoadRunner 解包并加载模型，读取 IO 元数据。
func LoadRunner(cfg UpscaleConfig) (*Runner, error) {
	m := cfg.Model
	tw, th := chooseTile(m.TileHW, cfg.FW, cfg.FH)
	if len(m.TileHW) == 0 {
		tw, th = cfg.FW, cfg.FH
	}
	tzPath, err := m.TZPath(cfg.ModelDir, tw, th, cfg.Scale)
	if err != nil {
		return nil, err
	}
	onnxBytes, err := os.ReadFile(tzPath)
	if err != nil {
		return nil, fmt.Errorf("读取模型失败: %w", err)
	}
	raw, err := tz.Extract(onnxBytes, tz.Password)
	if err != nil {
		return nil, fmt.Errorf("解包模型失败: %w", err)
	}
	// 修复 fp16 ReduceMean 在 CUDA 上的归约溢出 NaN。
	if patched, changed, perr := onnxpatch.PatchReduceMeanFp32(raw); perr == nil && changed {
		log.Debugf("已为 fp16 ReduceMean 插入 fp32 累加补丁")
		raw = patched
	}
	if cfg.WorkDir == "" {
		cfg.WorkDir = os.TempDir()
	}
	os.MkdirAll(cfg.WorkDir, 0o755)
	onnxPath := filepath.Join(cfg.WorkDir, fmt.Sprintf("%s_%dx%d_%dx.onnx", m.Short, th, tw, cfg.Scale))
	if err := os.WriteFile(onnxPath, raw, 0o644); err != nil {
		return nil, fmt.Errorf("写出 ONNX 失败: %w", err)
	}

	if err := initORT(); err != nil {
		return nil, err
	}

	so := ort.DefaultSessionOptions()
	so.CUDA = cfg.GPU != -1
	if cfg.GPU >= 0 {
		so.CUDADeviceID = cfg.GPU
	}
	if cfg.Threads > 0 {
		so.IntraOpThreads = cfg.Threads
	}
	// TVAI_GRAPH_OPT 可覆盖图优化等级（0=关闭, 99=全部）。
	if v := os.Getenv("TVAI_GRAPH_OPT"); v != "" {
		var g int
		if _, err := fmt.Sscanf(v, "%d", &g); err == nil {
			so.GraphOptLevel = g
		}
	}
	// TVAI_CUDNN_ALGO 可覆盖 cuDNN conv 算法搜索策略（0/1/2）。
	if v := os.Getenv("TVAI_CUDNN_ALGO"); v != "" {
		var a int
		if _, err := fmt.Sscanf(v, "%d", &a); err == nil && a >= 0 && a <= 2 {
			so.CUDNNAlgo = a
		}
	}
	env, err := ort.NewEnv()
	if err != nil {
		return nil, err
	}
	sess, err := ort.NewSession(env, onnxPath, so)
	if err != nil {
		return nil, err
	}
	inputs, err := sess.Inputs()
	if err != nil {
		return nil, err
	}
	outputs, err := sess.Outputs()
	if err != nil {
		return nil, err
	}
	r := &Runner{cfg: cfg, sess: sess, tileH: th, tileW: tw, inputs: inputs, outputs: outputs, paramVec: make([]float64, 6), sceneThreshold: 0.04}

	// 默认参数：GUI 空间 0-100 → 模型空间 Min..Max。
	for _, p := range m.Params {
		v, ok := cfg.Params[p.Key]
		if !ok {
			v = 0 // GUI 默认：不处理
		}
		// GUI 归一化到 [0,1]（deblur 是 [-100,100]→[-1,1]）。
		var norm float64
		if p.Min < 0 {
			norm = v / 100.0
		} else {
			norm = v / 100.0
		}
		clamped := math.Max(p.Min, math.Min(p.Max, norm))
		r.paramVec[p.Tensor] = clamped
	}
	return r, nil
}

// Close 释放会话。
func (r *Runner) Close() {
	if r.sess != nil {
		r.sess.Destroy()
		r.sess = nil
	}
}

// TileSize 返回当前使用的 tile 尺寸（输入空间）。
func (r *Runner) TileSize() (int, int) { return r.tileH, r.tileW }

// ProcessTile 对一个输入 tile 做推理，返回放大后的 tile。
// ox/oy 是该 tile 在输出画布中的坐标（用于从 prevHR 提取 hr_prev）。
func (r *Runner) ProcessTile(tiles []*Frame, scale, ox, oy int) (*Frame, error) {
	switch r.cfg.Model.Kind {
	case model.KindEnhance:
		return r.runEnhance(tiles, scale, ox, oy)
	case model.KindInterp:
		return r.runInterpTile(tiles)
	default:
		return nil, fmt.Errorf("ProcessTile 不支持该模型类别")
	}
}

// runInterpTile 对插帧模型的 4 帧 tile 做推理，返回 8 帧的拼合结果（24 通道）。
// 返回的 Frame.Data 布局为 [H, W, 24]，即每像素 24 值 = 8 帧 × 3 通道。
func (r *Runner) runInterpTile(tiles []*Frame) (*Frame, error) {
	m := r.cfg.Model
	tw, th := r.tileW, r.tileH
	var inputNames []string
	var tensors []*ort.Tensor
	defer func() {
		for _, t := range tensors {
			t.Destroy()
		}
	}()
	if m.SingleFile == "" {
		// Apollo：in0..in3
		if len(tiles) < 4 {
			return nil, fmt.Errorf("%s 需要 4 帧输入", m.Key)
		}
		for i := 0; i < 4; i++ {
			name := fmt.Sprintf("in%d", i)
			t, err := frameTensor(tiles[i], r.inputType(name))
			if err != nil {
				return nil, err
			}
			inputNames = append(inputNames, name)
			tensors = append(tensors, t)
		}
	} else {
		return nil, fmt.Errorf("模型 %s 的插帧输入构造尚未实现", m.Key)
	}
	outs, err := r.sess.Run(inputNames, tensors, []string{r.outputName()})
	if err != nil {
		return nil, err
	}
	defer outs[0].Destroy()
	// 输出 [1, th, tw, 24]，读为 24 通道原始数据。
	n := tw * th * 24
	vals, err := readTensorFloats(outs[0], n, r.outputType())
	if err != nil {
		return nil, err
	}
	var mn, mx float32 = 1e9, -1e9
	var sum float64
	for _, v := range vals {
		if v < mn {
			mn = v
		}
		if v > mx {
			mx = v
		}
		sum += float64(v)
	}
	log.Debugf("interp raw out type=%d range=[%.3f,%.3f] mean=%.4f n=%d", r.outputType(), mn, mx, sum/float64(len(vals)), len(vals))
	raw := &Frame{W: tw, H: th, Data: vals}
	return raw, nil
}

// readTensorFloats 把张量数据读为 float32 切片（按元素类型转换）。
func readTensorFloats(t *ort.Tensor, n, elemType int) ([]float32, error) {
	p := t.Data()
	if p == nil {
		return nil, fmt.Errorf("张量数据为空")
	}
	out := make([]float32, n)
	if elemType == ort.Float16 {
		src := unsafeSliceU16(p, n)
		for i := 0; i < n; i++ {
			out[i] = f16tof32(src[i])
		}
	} else {
		src := unsafeSlice32(p, n)
		copy(out, src)
	}
	return out, nil
}

// ExtractSubFrame 从插帧原始输出 [H, W, 24] 中提取第 k 个子帧（k=0..7）。
func ExtractSubFrame(raw *Frame, k int) *Frame {
	out := NewFrame(raw.W, raw.H)
	for j := 0; j < raw.H; j++ {
		for i := 0; i < raw.W; i++ {
			si := (j*raw.W+i)*24 + k*3
			di := (j*raw.W + i) * 3
			out.Data[di] = raw.Data[si]
			out.Data[di+1] = raw.Data[si+1]
			out.Data[di+2] = raw.Data[si+2]
		}
	}
	return out
}

// inputType 返回指定输入名的元素类型（默认 float32）。
func (r *Runner) inputType(name string) int {
	for _, in := range r.inputs {
		if in.Name == name {
			return in.Type
		}
	}
	return ort.Float
}

// outputType 返回第一个输出的元素类型。
func (r *Runner) outputType() int {
	if len(r.outputs) > 0 {
		return r.outputs[0].Type
	}
	return ort.Float
}

// frameTensor 按指定元素类型构造帧张量。
func frameTensor(f *Frame, elemType int) (*ort.Tensor, error) {
	dims := []int64{1, int64(f.H), int64(f.W), 3}
	if elemType == ort.Float16 {
		h := f32SliceToF16(f.Data)
		return ort.NewTensor(dims, ort.Float16, ptrOfU16(h), len(h))
	}
	return ort.NewTensor(dims, ort.Float, ptrOf(f.Data), len(f.Data))
}

func (r *Runner) runEnhance(tiles []*Frame, scale, ox, oy int) (*Frame, error) {
	m := r.cfg.Model
	tw, th := r.tileW, r.tileH
	var inputNames []string
	var tensors []*ort.Tensor
	defer func() {
		for _, t := range tensors {
			t.Destroy()
		}
	}()

	mk := func(name string, f *Frame) error {
		t, err := frameTensor(f, r.inputType(name))
		if err != nil {
			return err
		}
		inputNames = append(inputNames, name)
		tensors = append(tensors, t)
		return nil
	}

	switch m.Key {
	case "proteus":
		if len(tiles) < 2 {
			return nil, fmt.Errorf("proteus 需要 2 帧时序输入")
		}
		if err := mk("lr_prev", tiles[0]); err != nil {
			return nil, err
		}
		if err := mk("lr_curr", tiles[1]); err != nil {
			return nil, err
		}
		pt, err := r.paramTensor(r.paramVec)
		if err != nil {
			return nil, err
		}
		inputNames = append(inputNames, "params")
		tensors = append(tensors, pt)
		hr := r.extractHRTile(tw*scale, th*scale, ox, oy)
		if err := mk("hr_prev", hr); err != nil {
			return nil, err
		}
		st, err := r.scalarTensor(r.sceneThreshold)
		if err != nil {
			return nil, err
		}
		inputNames = append(inputNames, "scene_change_threshold")
		tensors = append(tensors, st)

	case "gaia", "gaia-cg":
		if len(tiles) < m.Temporal {
			return nil, fmt.Errorf("%s 需要 %d 帧时序输入", m.Key, m.Temporal)
		}
		// 输入名形如 VideoSR_Unet/inputFrameN:0。
		for j := 0; j < m.Temporal; j++ {
			name := fmt.Sprintf("VideoSR_Unet/inputFrame%d:0", j)
			if err := mk(name, tiles[j]); err != nil {
				return nil, err
			}
		}
	default:
		return nil, fmt.Errorf("模型 %s 的增强输入构造尚未实现", m.Key)
	}

	outs, err := r.sess.Run(inputNames, tensors, []string{r.outputName()})
	if err != nil {
		return nil, err
	}
	defer outs[0].Destroy()
	return tensorToFrameTyped(outs[0], tw*scale, th*scale, r.outputType())
}

// extractHRTile 从上一帧输出中提取对应 tile 作为 hr_prev。
// prevHR 为空时返回零帧（首帧）。
func (r *Runner) extractHRTile(w, h, ox, oy int) *Frame {
	if r.prevHR == nil {
		return NewFrame(w, h)
	}
	return r.prevHR.ExtractTile(ox, oy, w, h)
}

// paramTensor 按声明类型构造 params 张量。
func (r *Runner) paramTensor(v []float64) (*ort.Tensor, error) {
	f := make([]float32, len(v))
	for i, x := range v {
		f[i] = float32(x)
	}
	dims := []int64{1, int64(len(v)), 1, 1}
	et := r.inputType("params")
	if et == ort.Float16 {
		h := f32SliceToF16(f)
		return ort.NewTensor(dims, ort.Float16, ptrOfU16(h), len(h))
	}
	return ort.NewTensor(dims, ort.Float, ptrOf(f), len(f))
}

// scalarTensor 按声明类型构造标量张量。
func (r *Runner) scalarTensor(v float64) (*ort.Tensor, error) {
	f := []float32{float32(v)}
	dims := []int64{1, 1, 1, 1}
	et := r.inputType("scene_change_threshold")
	if et == ort.Float16 {
		h := f32SliceToF16(f)
		return ort.NewTensor(dims, ort.Float16, ptrOfU16(h), 1)
	}
	return ort.NewTensor(dims, ort.Float, ptrOf(f), 1)
}

// tensorToFrameTyped 按元素类型读取输出张量为 float32 帧。
func tensorToFrameTyped(t *ort.Tensor, w, h int, elemType int) (*Frame, error) {
	p := t.Data()
	if p == nil {
		return nil, fmt.Errorf("张量数据为空")
	}
	n := w * h * 3
	f := &Frame{W: w, H: h, Data: make([]float32, n)}
	if elemType == ort.Float16 {
		src := unsafeSliceU16(p, n)
		for i := 0; i < n; i++ {
			f.Data[i] = f16tof32(src[i])
		}
	} else {
		src := unsafeSlice32(p, n)
		copy(f.Data, src)
	}
	return f, nil
}

// LogTile 输出 tile 信息。
func (r *Runner) LogTile() {
	th, tw := r.tileH, r.tileW
	log.Infof("tile %dx%d, scale %dx, params %v", th, tw, r.cfg.Scale, r.paramVec)
	_ = math.Max
}
