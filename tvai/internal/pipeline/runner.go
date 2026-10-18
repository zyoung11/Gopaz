package pipeline

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"tvai/internal/log"
	"tvai/internal/model"
	"tvai/internal/onnxpatch"
	"tvai/internal/ort"
	"tvai/internal/tz"
)

// DefaultSceneChangeThreshold 是 Topaz 内部传入模型的场景切换阈值。
const DefaultSceneChangeThreshold = 6.5

// UpscaleConfig 是超分/修复的完整配置。
type UpscaleConfig struct {
	Model          *model.Model
	ModelDir       string             // .tz 文件目录
	Scale          int                // 1/2/4
	FW, FH         int                // 输入帧尺寸
	Params         map[string]float64 // GUI 空间参数（-100-100）
	Overlap        int                // 分块重叠（输入空间），0=按模型默认
	SceneThreshold float64            // 场景切换阈值，0=默认 6.5
	GPU            int                // -2 自动 / -1 CPU / 0+ 设备号
	Threads        int
	Instances      int     // 插值并发会话数，0=自动（默认 1 个共享会话）
	Workers        int     // 插值并行 worker 数，0=自动
	VramGB         float64 // 每会话显存上限(GB)，0=自动
	DeviceVRAMGB   float64 // Aion tile 预算(GB)，0=按默认上限
	WorkDir        string  // 临时解包目录
}

// Runner 持有一个已加载的模型会话。
type Runner struct {
	cfg            UpscaleConfig
	sess           *ort.Session
	inputs         []ort.IOInfo
	outputs        []ort.IOInfo
	tileH, tileW   int
	paramVec       []float64 // 长度 6
	sceneThreshold float64
	overlap        int
	// 时序状态：上一帧的输出（输出分辨率）。
	prevHR *Frame
	// 并发实例池。sess=pool[0]，插帧时每个实例配对 bottomPool[i]。
	pool          []*ort.Session
	bottomPool    []*ort.Session
	plan          *interpPlan
	enhPlan       *enhancePlan
	bottomOutType int
	// 累计推理纳秒（诊断用）。
	inferNanos int64
	workers    int
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

// ResetTemporal 初始化时序状态，base 为当前帧的双线性放大结果。
func (r *Runner) ResetTemporal(base *Frame) {
	r.prevHR = base
}

// HasInput 判断模型是否声明了指定输入。
func (r *Runner) HasInput(name string) bool {
	for _, in := range r.inputs {
		if in.Name == name {
			return true
		}
	}
	return false
}

// Overlap 返回当前模型实际使用的分块重叠。
func (r *Runner) Overlap() int { return r.overlap }

// BatchLimit 返回插帧模型可用的 tile 批大小上限。
func (r *Runner) BatchLimit() int {
	for _, in := range r.inputs {
		if in.Name == "in0" && len(in.Dim) > 0 && in.Dim[0] == 1 {
			return 1
		}
	}
	return 4
}

// LoadRunner 解包并加载模型，读取 IO 元数据。
func LoadRunner(cfg UpscaleConfig) (*Runner, error) {
	m := cfg.Model
	nInst := cfg.Instances
	if nInst <= 0 {
		nInst = 1
	}
	if m.Kind == model.KindEnhance {
		nInst = 1
	}
	if nInst < 1 {
		nInst = 1
	}
	if nInst > 3 {
		nInst = 3
	}
	nWorkers := cfg.Workers
	if nWorkers <= 0 {
		nWorkers = 2
		if cfg.GPU != -1 {
			if free := detectFreeVRAMGB(cfg.GPU); free > 0 && free < 12 {
				nWorkers = 1
			}
		}
	}
	if nWorkers < 1 {
		nWorkers = 1
	}
	if nWorkers > 8 {
		nWorkers = 8
	}
	overlap := cfg.Overlap
	if overlap <= 0 {
		overlap = m.BlockOverlapOrDefault()
	}
	candidates := m.TileHW
	var tw, th int
	if m.SingleFile != "" {
		tw, th = chooseAionTile(cfg.FW, cfg.FH, overlap, cfg.DeviceVRAMGB)
	} else {
		tw, th = chooseTile(candidates, cfg.FW, cfg.FH, overlap, TilePenalty)
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
	if patched, changed, perr := onnxpatch.PatchReduceMeanFp32(raw); perr == nil && changed {
		log.Debugf("已为 fp16 ReduceMean 插入 fp32 累加补丁")
		raw = patched
	}
	if m.Kind == model.KindInterp {
		if patched, changed, perr := onnxpatch.PatchOutputFp16(raw); perr == nil && changed {
			log.Debugf("已把模型输出改为 fp16")
			raw = patched
		}
	}
	if cfg.WorkDir == "" {
		cfg.WorkDir = os.TempDir()
	}
	os.MkdirAll(cfg.WorkDir, 0o755)
	onnxPath := filepath.Join(cfg.WorkDir, fmt.Sprintf("%s_%dx%d_%dx.onnx", m.Short, th, tw, cfg.Scale))
	if err := os.WriteFile(onnxPath, raw, 0o644); err != nil {
		return nil, fmt.Errorf("写出 ONNX 失败: %w", err)
	}

	var botPath string
	if bp, _ := m.BottomTZPath(cfg.ModelDir); bp != "" {
		botPath, err = prepareOnnx(bp, cfg.WorkDir, m.Short+"_bottom")
		if err != nil {
			return nil, fmt.Errorf("准备底部模型失败: %w", err)
		}
	}

	if err := initORT(); err != nil {
		return nil, err
	}

	so := ort.DefaultSessionOptions()
	so.CUDA = cfg.GPU != -1
	if cfg.GPU != -1 {
		so.ProviderOpts = map[string]string{"prefer_nhwc": "1"}
		if m.Kind == model.KindInterp {
			so.ProviderOpts["use_tf32"] = "1"
		}
	}
	if cfg.VramGB > 0 {
		so.GPUMemLimit = uint64(cfg.VramGB * 1024 * 1024 * 1024)
	} else if m.Kind == model.KindInterp {
		baseGB := clampFloat(float64(tw*th)/700000, 3.0, 10.0)
		perSess := (nWorkers + nInst - 1) / nInst
		autoGB := baseGB * float64(perSess) / 2
		if autoGB < 2.0 {
			autoGB = 2.0
		}
		so.GPUMemLimit = uint64(autoGB * 1024 * 1024 * 1024)
	} else if m.Kind == model.KindEnhance {
		autoGB := clampFloat(float64(tw*th*cfg.Scale*cfg.Scale)/600000, 4.0, 12.0)
		so.GPUMemLimit = uint64(autoGB * 1024 * 1024 * 1024)
	}
	if cfg.GPU >= 0 {
		so.CUDADeviceID = cfg.GPU
	}
	if cfg.Threads > 0 {
		so.IntraOpThreads = cfg.Threads
	}
	env, err := ort.NewEnv()
	if err != nil {
		return nil, err
	}
	mainCh := make(chan poolResult, 1)
	go func() {
		p, e := createSessions(env, onnxPath, so, nInst)
		mainCh <- poolResult{pool: p, err: e}
	}()
	var botCh chan poolResult
	if botPath != "" {
		botCh = make(chan poolResult, 1)
		go func() {
			p, e := createSessions(env, botPath, so, nInst)
			botCh <- poolResult{pool: p, err: e}
		}()
	}
	mr := <-mainCh
	if mr.err != nil {
		if botCh != nil {
			br := <-botCh
			for _, s := range br.pool {
				s.Destroy()
			}
		}
		return nil, mr.err
	}
	pool := mr.pool
	sess := pool[0]
	var bpool []*ort.Session
	if botCh != nil {
		br := <-botCh
		if br.err != nil {
			for _, s := range pool {
				s.Destroy()
			}
			return nil, fmt.Errorf("加载底部模型失败: %w", br.err)
		}
		bpool = br.pool
	}
	inputs, err := sess.Inputs()
	if err != nil {
		for _, s := range pool {
			s.Destroy()
		}
		for _, s := range bpool {
			s.Destroy()
		}
		return nil, err
	}
	outputs, err := sess.Outputs()
	if err != nil {
		for _, s := range pool {
			s.Destroy()
		}
		for _, s := range bpool {
			s.Destroy()
		}
		return nil, err
	}
	sceneThreshold := cfg.SceneThreshold
	if sceneThreshold <= 0 {
		sceneThreshold = DefaultSceneChangeThreshold
	}
	r := &Runner{cfg: cfg, sess: sess, tileH: th, tileW: tw, inputs: inputs, outputs: outputs, paramVec: make([]float64, 6), sceneThreshold: sceneThreshold, overlap: overlap, pool: pool, bottomPool: bpool, workers: nWorkers}
	if len(bpool) > 0 {
		r.bottomOutType = ort.Float
		if bo, err := bpool[0].Outputs(); err == nil && len(bo) > 0 {
			r.bottomOutType = bo[0].Type
		}
	}

	for _, p := range m.Params {
		v := cfg.Params[p.Key]
		r.paramVec[p.Tensor] = math.Max(-1, math.Min(1, v/100.0))
	}
	if m.Kind == model.KindInterp {
		r.plan = newInterpPlan(cfg.FW, cfg.FH, tw, th, overlap, r.BatchLimit())
	}
	if m.Kind == model.KindEnhance {
		r.enhPlan = newEnhancePlan(cfg.FW, cfg.FH, tw, th, cfg.Scale, overlap)
	}
	return r, nil
}

type poolResult struct {
	pool []*ort.Session
	err  error
}

// createSessions 并发创建 n 个相同配置的会话。
func createSessions(env *ort.Env, path string, so ort.SessionOptions, n int) ([]*ort.Session, error) {
	out := make([]*ort.Session, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			out[i], errs[i] = ort.NewSession(env, path, so)
		}(i)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			for _, s := range out {
				if s != nil {
					s.Destroy()
				}
			}
			return nil, err
		}
	}
	return out, nil
}

// prepareOnnx 解包 .tz、应用 ReduceMean 补丁并写出 ONNX，返回路径。
func prepareOnnx(tzPath, workDir, name string) (string, error) {
	b, err := os.ReadFile(tzPath)
	if err != nil {
		return "", err
	}
	raw, err := tz.Extract(b, tz.Password)
	if err != nil {
		return "", err
	}
	if p, changed, perr := onnxpatch.PatchReduceMeanFp32(raw); perr == nil && changed {
		raw = p
	}
	os.MkdirAll(workDir, 0o755)
	p := filepath.Join(workDir, name+".onnx")
	if err := os.WriteFile(p, raw, 0o644); err != nil {
		return "", err
	}
	return p, nil
}

// Close 释放会话。
func (r *Runner) Close() {
	for _, s := range r.pool {
		s.Destroy()
	}
	r.pool = nil
	r.sess = nil
	for _, s := range r.bottomPool {
		s.Destroy()
	}
	r.bottomPool = nil
}

// TileSize 返回当前使用的 tile 尺寸（输入空间）。
func (r *Runner) TileSize() (int, int) { return r.tileH, r.tileW }

// ProcessTile 对一个输入 tile 做增强推理，返回放大后的 tile。
// ox/oy 是该 tile 在输出画布中的坐标（用于从 prevHR 提取 hr_prev）。
func (r *Runner) ProcessTile(tiles []*Frame, scale, ox, oy int) (*Frame, error) {
	if r.cfg.Model.Kind != model.KindEnhance {
		return nil, fmt.Errorf("ProcessTile 不支持该模型类别")
	}
	return r.runEnhance(tiles, scale, ox, oy)
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
	case "gaia", "gaia-cg":
		if len(tiles) < m.Temporal {
			return nil, fmt.Errorf("%s 需要 %d 帧时序输入", m.Key, m.Temporal)
		}
		for j := 0; j < m.Temporal; j++ {
			name := fmt.Sprintf("VideoSR_Unet/inputFrame%d:0", j)
			if err := mk(name, tiles[j]); err != nil {
				return nil, err
			}
		}
	default:
		center := m.CenterIdx
		tileAt := func(i int) *Frame {
			if i < 0 {
				i = 0
			}
			if i >= len(tiles) {
				i = len(tiles) - 1
			}
			return tiles[i]
		}
		if r.HasInput("lr_prev") {
			if err := mk("lr_prev", tileAt(center-1)); err != nil {
				return nil, err
			}
		}
		if r.HasInput("lr_curr") {
			if err := mk("lr_curr", tileAt(center)); err != nil {
				return nil, err
			}
		}
		if r.HasInput("params") {
			pt, err := r.paramTensor(r.paramVec)
			if err != nil {
				return nil, err
			}
			inputNames = append(inputNames, "params")
			tensors = append(tensors, pt)
		}
		if r.HasInput("hr_prev") {
			hr := r.extractHRTile(tw*scale, th*scale, ox, oy)
			if err := mk("hr_prev", hr); err != nil {
				return nil, err
			}
		}
		if r.HasInput("scene_change_threshold") {
			st, err := r.scalarTensor(r.sceneThreshold)
			if err != nil {
				return nil, err
			}
			inputNames = append(inputNames, "scene_change_threshold")
			tensors = append(tensors, st)
		}
	}
	if len(inputNames) == 0 {
		return nil, fmt.Errorf("模型 %s 的增强输入构造失败", m.Key)
	}

	t1 := time.Now()
	outs, err := r.sess.Run(inputNames, tensors, []string{r.outputName()})
	r.addInfer(time.Since(t1).Nanoseconds())
	if err != nil {
		return nil, err
	}
	defer outs[0].Destroy()
	return tensorToFrameTyped(outs[0], tw*scale, th*scale, r.outputType())
}

// addInfer 线程安全地累加推理纳秒（并发 worker 会同时调用）。
func (r *Runner) addInfer(ns int64) {
	atomic.AddInt64(&r.inferNanos, ns)
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
		for i := range n {
			f.Data[i] = f16tof32(src[i])
		}
	} else {
		src := unsafeSlice32(p, n)
		copy(f.Data, src)
	}
	return f, nil
}
