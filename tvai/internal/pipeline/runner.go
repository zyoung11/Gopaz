package pipeline

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
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
	Instances      int     // 插值并发实例数（会话池大小）
	VramGB         float64 // 每会话显存上限(GB)，0=自动
	DeviceVRAMGB   float64 // 动态模型使用的总显存(GB)，0=自动检测
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
	// 两级插帧的底部模型会话（如 Aion 的 aiob）。
	bottom *ort.Session
	// 并发实例池（多会话以保持 GPU 喂饱）。sess=pool[0], bottom=bottomPool[0]。
	pool       []*ort.Session
	bottomPool []*ort.Session
	// 累计推理纳秒（诊断用）。
	inferNanos int64
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
	detectedVRAM := 0.0
	if m.PixelsPerGB > 0 && cfg.GPU != -1 {
		detectedVRAM = detectVRAMGB(cfg.GPU)
	}
	tileVRAM := cfg.DeviceVRAMGB
	if tileVRAM <= 0 {
		tileVRAM = detectedVRAM
		if tileVRAM > defaultAionTileBudget {
			tileVRAM = defaultAionTileBudget
		}
	}
	sessionVRAM := detectedVRAM
	if sessionVRAM <= 0 {
		sessionVRAM = cfg.DeviceVRAMGB
	}
	nInst := cfg.Instances
	if nInst <= 0 {
		nInst = 2
	}
	if nInst > 6 {
		nInst = 6
	}
	overlap := cfg.Overlap
	if overlap <= 0 {
		overlap = m.BlockOverlapOrDefault()
	}
	candidates := m.TileHW
	var tw, th int
	if m.SingleFile != "" {
		if m.PixelsPerGB > 0 && tileVRAM > 0 {
			tw, th = chooseDynamicTile(m.BaseGB, m.PixelsPerGB, tileVRAM, cfg.FW, cfg.FH)
		}
		if tw <= 0 || th <= 0 {
			tw, th = chooseTileMax(candidates, cfg.FW, cfg.FH)
		}
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
	if cfg.VramGB > 0 {
		so.GPUMemLimit = uint64(cfg.VramGB * 1024 * 1024 * 1024)
	} else if m.PixelsPerGB > 0 && sessionVRAM > 0 {
		autoGB := sessionVRAM - m.BaseGB
		if autoGB < 0.1 {
			autoGB = 0.1
		}
		if nInst > 1 {
			autoGB /= float64(nInst)
		}
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
	sess, err := ort.NewSession(env, onnxPath, so)
	if err != nil {
		return nil, err
	}
	pool := []*ort.Session{sess}
	for i := 1; i < nInst; i++ {
		ps, err := ort.NewSession(env, onnxPath, so)
		if err != nil {
			for _, s := range pool {
				s.Destroy()
			}
			return nil, fmt.Errorf("创建实例 %d 失败: %w", i, err)
		}
		pool = append(pool, ps)
	}
	inputs, err := sess.Inputs()
	if err != nil {
		return nil, err
	}
	outputs, err := sess.Outputs()
	if err != nil {
		return nil, err
	}
	sceneThreshold := cfg.SceneThreshold
	if sceneThreshold <= 0 {
		sceneThreshold = DefaultSceneChangeThreshold
	}
	r := &Runner{cfg: cfg, sess: sess, tileH: th, tileW: tw, inputs: inputs, outputs: outputs, paramVec: make([]float64, 6), sceneThreshold: sceneThreshold, overlap: overlap, pool: pool}

	// 两级插帧：加载底部模型（如 Aion 的 aiob），同样建实例池。
	if bpath, _ := m.BottomTZPath(cfg.ModelDir); bpath != "" {
		bonnx, err := prepareOnnx(bpath, cfg.WorkDir, m.Short+"_bottom")
		if err != nil {
			for _, s := range pool {
				s.Destroy()
			}
			return nil, fmt.Errorf("准备底部模型失败: %w", err)
		}
		var bpool []*ort.Session
		for i := 0; i < nInst; i++ {
			bs, err := ort.NewSession(env, bonnx, so)
			if err != nil {
				for _, s := range pool {
					s.Destroy()
				}
				for _, s := range bpool {
					s.Destroy()
				}
				return nil, fmt.Errorf("加载底部模型失败: %w", err)
			}
			bpool = append(bpool, bs)
		}
		r.bottom = bpool[0]
		r.bottomPool = bpool
	}

	for _, p := range m.Params {
		v := cfg.Params[p.Key]
		r.paramVec[p.Tensor] = math.Max(-1, math.Min(1, v/100.0))
	}
	return r, nil
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

// runBottom 用底部模型（aiob）对 4 帧 tile 推理，返回 [tw,th,24] 原始数据（8 帧）。
func (r *Runner) runBottom(tiles []*Frame) (*Frame, error) {
	tw, th := r.tileW, r.tileH
	var names []string
	var tensors []*ort.Tensor
	defer func() {
		for _, t := range tensors {
			t.Destroy()
		}
	}()
	for i := range 4 {
		t, err := frameTensor(tiles[i], ort.Float)
		if err != nil {
			return nil, err
		}
		names = append(names, fmt.Sprintf("in%d", i))
		tensors = append(tensors, t)
	}
	t0 := time.Now()
	outs, err := r.bottom.Run(names, tensors, []string{"out"})
	r.inferNanos += time.Since(t0).Nanoseconds()
	if err != nil {
		return nil, err
	}
	defer outs[0].Destroy()
	vals, err := readTensorFloats(outs[0], tw*th*24, ort.Float)
	if err != nil {
		return nil, err
	}
	return &Frame{W: tw, H: th, Data: vals}, nil
}

// downsampleRaw 对 [W,H,ch] 原始数据按 factor 做块平均降采样。
func downsampleRaw(raw *Frame, factor int) *Frame {
	ch := len(raw.Data) / (raw.W * raw.H)
	if ch <= 0 {
		ch = 1
	}
	ow, oh := raw.W/factor, raw.H/factor
	if ow < 1 {
		ow = 1
	}
	if oh < 1 {
		oh = 1
	}
	out := &Frame{W: ow, H: oh, Data: make([]float32, ow*oh*ch)}
	inv := float32(1) / float32(factor*factor)
	parallelRows(oh, func(js, je int) {
		for j := js; j < je; j++ {
			for i := 0; i < ow; i++ {
				for c := 0; c < ch; c++ {
					var sum float32
					for dy := range factor {
						for dx := range factor {
							si := ((j*factor+dy)*raw.W + (i*factor + dx)) * ch
							sum += raw.Data[si+c]
						}
					}
					out.Data[(j*ow+i)*ch+c] = sum * inv
				}
			}
		}
	})
	return out
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
	r.bottom = nil
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
	if len(tiles) < 4 {
		return nil, fmt.Errorf("%s 需要 4 帧输入", m.Key)
	}
	for i := range 4 {
		name := fmt.Sprintf("in%d", i)
		t, err := frameTensor(tiles[i], r.inputType(name))
		if err != nil {
			return nil, err
		}
		inputNames = append(inputNames, name)
		tensors = append(tensors, t)
	}
	if m.SingleFile != "" {
		// Aion 两级：先用底部模型 aiob 产 8 帧，降采样到 tile/4 作 in8x。
		h8, w8 := th/4, tw/4
		if h8 < 1 {
			h8 = 1
		}
		if w8 < 1 {
			w8 = 1
		}
		var in8 []float32
		if r.bottom != nil {
			bo, err := r.runBottom(tiles)
			if err != nil {
				return nil, err
			}
			down := downsampleRaw(bo, 4)
			in8 = down.Data
			if len(in8) != h8*w8*24 {
				// 尺寸不符时退回零（保守）。
				in8 = make([]float32, h8*w8*24)
			}
		} else {
			in8 = make([]float32, h8*w8*24)
		}
		zd := []int64{1, int64(h8), int64(w8), 24}
		zt, err := ort.NewTensor(zd, ort.Float, ptrOf(in8), len(in8))
		if err != nil {
			return nil, err
		}
		inputNames = append(inputNames, "in8x")
		tensors = append(tensors, zt)
	}
	t0 := time.Now()
	outs, err := r.sess.Run(inputNames, tensors, []string{r.outputName()})
	r.inferNanos += time.Since(t0).Nanoseconds()
	if err != nil {
		return nil, err
	}
	defer outs[0].Destroy()
	// 输出 [1, th, tw, frames*3]：Apollo 24 通道(8帧)，Aion 48 通道(16帧)。
	n := tw * th * m.OutFrames * 3
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
		for i := range n {
			out[i] = f16tof32(src[i])
		}
	} else {
		src := unsafeSlice32(p, n)
		copy(out, src)
	}
	return out, nil
}

// ExtractSubFrame 从插帧原始输出 [H, W, frames*3] 中提取第 k 个子帧。
// 帧数由数据长度推断（Apollo 8 帧 / Aion 16 帧）。
func ExtractSubFrame(raw *Frame, k int) *Frame {
	perFrame := raw.W * raw.H * 3
	frames := len(raw.Data) / perFrame
	if frames <= 0 {
		frames = 1
	}
	if k >= frames {
		k = frames - 1
	}
	stride := frames * 3
	out := NewFrame(raw.W, raw.H)
	for j := 0; j < raw.H; j++ {
		for i := 0; i < raw.W; i++ {
			si := (j*raw.W+i)*stride + k*3
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
	r.inferNanos += time.Since(t1).Nanoseconds()
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

// stackFrames 把 B 个 [H,W,3] 帧堆叠成 [B,H,W,3] 张量（按 elemType 转换）。
func stackFrames(frames []*Frame, elemType int) (*ort.Tensor, error) {
	B := len(frames)
	if B == 0 {
		return nil, fmt.Errorf("stackFrames: 空批次")
	}
	H, W := frames[0].H, frames[0].W
	n := B * H * W * 3
	dims := []int64{int64(B), int64(H), int64(W), 3}
	if elemType == ort.Float16 {
		buf := make([]uint16, n)
		off := 0
		for _, f := range frames {
			for i, v := range f.Data {
				buf[off+i] = f32tof16(v)
			}
			off += H * W * 3
		}
		return ort.NewTensor(dims, ort.Float16, ptrOfU16(buf), n)
	}
	buf := make([]float32, n)
	off := 0
	for _, f := range frames {
		copy(buf[off:], f.Data)
		off += H * W * 3
	}
	return ort.NewTensor(dims, ort.Float, ptrOf(buf), n)
}

// splitBatchFrames 把 [B,H,W,ch] 原始输出拆成 B 个 [H,W,ch] 帧。
func splitBatchFrames(vals []float32, B, H, W, ch int) []*Frame {
	per := H * W * ch
	out := make([]*Frame, B)
	for b := range B {
		out[b] = &Frame{W: W, H: H, Data: vals[b*per : (b+1)*per]}
	}
	return out
}

// runInterpBatch 在指定会话对（主+底）上对一批 tile 做插帧推理。
// tilesBatch[i] 是第 i 个 tile 的 4 帧窗口。返回每个 tile 的 [tw,th,OutFrames*3] 结果。
func (r *Runner) runInterpBatch(main, bot *ort.Session, tilesBatch [][]*Frame) ([]*Frame, error) {
	m := r.cfg.Model
	tw, th := r.tileW, r.tileH
	B := len(tilesBatch)
	if B == 0 {
		return nil, nil
	}
	var names []string
	var tensors []*ort.Tensor
	defer func() {
		for _, t := range tensors {
			t.Destroy()
		}
	}()
	// in0..in3：各 [B,th,tw,3]
	for i := range 4 {
		frames := make([]*Frame, B)
		for b := range B {
			frames[b] = tilesBatch[b][i]
		}
		name := fmt.Sprintf("in%d", i)
		t, err := stackFrames(frames, r.inputType(name))
		if err != nil {
			return nil, err
		}
		names = append(names, name)
		tensors = append(tensors, t)
	}
	if m.SingleFile != "" && bot != nil {
		// Aion 两级：底部批量推理 → 降采样 → in8x 批量
		var bn []string
		bn = make([]string, 0, 4)
		var bt []*ort.Tensor
		bt0 := time.Now()
		for i := range 4 {
			frames := make([]*Frame, B)
			for b := range B {
				frames[b] = tilesBatch[b][i]
			}
			name := fmt.Sprintf("in%d", i)
			t, err := stackFrames(frames, ort.Float)
			if err != nil {
				for _, x := range bt {
					x.Destroy()
				}
				return nil, err
			}
			bn = append(bn, name)
			bt = append(bt, t)
		}
		bouts, err := bot.Run(bn, bt, []string{"out"})
		r.addInfer(time.Since(bt0).Nanoseconds())
		for _, x := range bt {
			x.Destroy()
		}
		if err != nil {
			return nil, err
		}
		bvals, err := readTensorFloats(bouts[0], B*tw*th*24, ort.Float)
		bouts[0].Destroy()
		if err != nil {
			return nil, err
		}
		// 每个 tile 降采样 4x 作 in8x
		h8, w8 := th/4, tw/4
		in8 := make([]float32, B*h8*w8*24)
		for b := range B {
			raw := &Frame{W: tw, H: th, Data: bvals[b*tw*th*24 : (b+1)*tw*th*24]}
			down := downsampleRaw(raw, 4)
			copy(in8[b*h8*w8*24:], down.Data)
		}
		zt, err := ort.NewTensor([]int64{int64(B), int64(h8), int64(w8), 24}, ort.Float, ptrOf(in8), len(in8))
		if err != nil {
			return nil, err
		}
		names = append(names, "in8x")
		tensors = append(tensors, zt)
	}
	t0 := time.Now()
	outs, err := main.Run(names, tensors, []string{r.outputName()})
	r.addInfer(time.Since(t0).Nanoseconds())
	if err != nil {
		return nil, err
	}
	defer outs[0].Destroy()
	ch := m.OutFrames * 3
	vals, err := readTensorFloats(outs[0], B*tw*th*ch, r.outputType())
	if err != nil {
		return nil, err
	}
	return splitBatchFrames(vals, B, th, tw, ch), nil
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
