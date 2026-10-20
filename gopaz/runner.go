package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// DefaultSceneChangeThreshold is the scene change threshold that Topaz passes to the model.
const DefaultSceneChangeThreshold = 6.5

// UpscaleConfig is the full configuration for upscaling and restoration.
type UpscaleConfig struct {
	Model *Model
	// ModelDir is the directory containing the .tz files.
	ModelDir string
	// Scale is the upscale factor: 1, 2, or 4.
	Scale int
	// FW and FH are the input frame dimensions.
	FW, FH int
	// Params holds GUI-space parameter values in the range -100..100.
	Params map[string]float64
	// Overlap is the block overlap in input space; 0 uses the model default.
	Overlap int
	// SceneThreshold is the scene change threshold; 0 uses the default 6.5.
	SceneThreshold float64
	// GPU is -2 for auto, -1 for CPU, or a device index.
	GPU     int
	Threads int
	// Instances is the number of interpolation sessions; 0 means auto, which is
	// one shared session.
	Instances int
	// Workers is the number of parallel workers; 0 means auto.
	Workers int
	// VramGB caps GPU memory per session in GB; 0 means auto.
	VramGB float64
	// DeviceVRAMGB is the Aion tile budget in GB; 0 uses the default cap.
	DeviceVRAMGB float64
	Provider     Provider
	Device       string
}

// Runner holds a loaded model session and its tile plan.
type Runner struct {
	cfg            UpscaleConfig
	sess           *Session
	inputs         []IOInfo
	outputs        []IOInfo
	tileH, tileW   int
	paramVec       []float64
	sceneThreshold float64
	overlap        int
	// prevHR is the previous frame output at output resolution.
	prevHR *Frame
	// pool holds concurrent sessions. sess is pool[0]; interpolation pairs each
	// instance with bottomPool[i].
	pool          []*Session
	bottomPool    []*Session
	plan          *interpPlan
	enhPlan       *enhancePlan
	bottomOutType int
	// inferNanos accumulates inference time in nanoseconds for diagnostics.
	inferNanos int64
	// bottomNanos and mainNanos split interpolation inference time.
	bottomNanos int64
	mainNanos   int64
	mergeNanos  int64
	workers     int
}

// outputName returns the tensor name of the model's first output.
func (r *Runner) outputName() string {
	if len(r.outputs) > 0 {
		return r.outputs[0].Name
	}
	return "out"
}

// SetPrevOutput stores the current frame output for the next frame's hr_prev.
func (r *Runner) SetPrevOutput(f *Frame) { r.prevHR = f }

// ResetTemporal initializes the temporal state from a bilinear upscale of the current frame.
func (r *Runner) ResetTemporal(base *Frame) {
	r.prevHR = base
}

// HasInput reports whether the model declares the named input.
func (r *Runner) HasInput(name string) bool {
	for _, in := range r.inputs {
		if in.Name == name {
			return true
		}
	}
	return false
}

// Overlap returns the block overlap in use.
func (r *Runner) Overlap() int { return r.overlap }

// BatchLimit returns the maximum tile batch size for an interpolation model.
func (r *Runner) BatchLimit() int {
	for _, in := range r.inputs {
		if in.Name == "in0" && len(in.Dim) > 0 && in.Dim[0] == 1 {
			return 1
		}
	}
	return 4
}

// LoadRunner unpacks and loads the model, then reads its IO metadata.
func LoadRunner(cfg UpscaleConfig) (*Runner, error) {
	m := cfg.Model
	provider, err := normalizeProvider(string(cfg.Provider))
	if err != nil {
		return nil, err
	}
	if cfg.GPU == -1 {
		provider = ProviderCPU
	}
	cfg.Provider = provider
	device := strings.TrimSpace(cfg.Device)
	if provider == ProviderOpenVINO {
		device, err = openVINODevice(device)
		if err != nil {
			return nil, err
		}
		if strings.EqualFold(device, "NPU") && m.NPUUnsupported {
			return nil, fmt.Errorf("Aion is not supported on the OpenVINO NPU; use GPU.0 instead")
		}
	}
	cfg.Device = device
	gpuID := cfg.GPU
	if device != "" && (providerNeedsCUDA(provider) || provider == ProviderDML) {
		gpuID, err = deviceIndex(device)
		if err != nil {
			return nil, err
		}
	}
	nInst := cfg.Instances
	if nInst <= 0 {
		nInst = 1
	}
	if m.Kind == KindEnhance || providerSingleThreaded(provider) {
		nInst = 1
	}
	if nInst < 1 {
		nInst = 1
	}
	if nInst > 3 {
		nInst = 3
	}
	nWorkers := cfg.Workers
	if providerSingleThreaded(provider) {
		nWorkers = 1
	} else if nWorkers <= 0 {
		nWorkers = providerDefaultWorkers(provider, m.Kind, device)
		if nWorkers <= 0 {
			nWorkers = 2
			if gpuID != -1 {
				if free := detectFreeVRAMGB(gpuID); free > 0 && free < 12 {
					nWorkers = 1
				}
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
		if provider == ProviderOpenVINO && strings.EqualFold(device, "NPU") {
			capped := npuTiles(candidates)
			if len(capped) == 0 {
				return nil, fmt.Errorf("model %s has no tile size supported by the OpenVINO NPU", m.Key)
			}
			if len(capped) < len(candidates) {
				Infof("NPU limits tile size to 576x672 for the VPUX compiler")
				candidates = capped
			}
		}
		tw, th = chooseTile(candidates, cfg.FW, cfg.FH, overlap, TilePenalty)
	}
	modelPath, err := m.TZPath(cfg.ModelDir, tw, th, cfg.Scale)
	if err != nil {
		return nil, err
	}
	raw, err := readModel(modelPath)
	if err != nil {
		return nil, err
	}
	if patched, changed, perr := PatchReduceMeanFp32(raw); perr == nil && changed {
		Debugf("inserted fp32 accumulation patch for fp16 ReduceMean")
		raw = patched
	}
	if m.Kind == KindInterp {
		if patched, changed, perr := PatchOutputFp16(raw); perr == nil && changed {
			Debugf("converted model output to fp16")
			raw = patched
		}
	}

	var bottomData []byte
	var bottomPath string
	if bp, _ := m.BottomTZPath(cfg.ModelDir); bp != "" {
		bottomPath = bp
		bottomData, err = readModel(bp)
		if err != nil {
			return nil, fmt.Errorf("failed to prepare bottom model: %w", err)
		}
		if patched, changed, perr := PatchReduceMeanFp32(bottomData); perr == nil && changed {
			bottomData = patched
		}
	}

	if err := initORT(provider); err != nil {
		return nil, err
	}

	so := DefaultSessionOptions()
	so.Provider = provider
	so.Device = device
	so.CUDA = providerNeedsCUDA(provider)
	if providerNeedsCUDA(provider) {
		if err := applyCUDNNAlgoFromEnv(&so); err != nil {
			return nil, err
		}
		so.ProviderOpts = map[string]string{"prefer_nhwc": "1"}
		if m.Kind == KindInterp {
			so.ProviderOpts["use_tf32"] = "1"
		}
	}
	if provider == ProviderTensorRT {
		cacheDir := filepath.Join(cacheRoot(), "tensorrt")
		if err := os.MkdirAll(cacheDir, 0o755); err == nil {
			so.TRTCacheDir = cacheDir
		}
		so.TRTCachePrefix = trtCachePrefix(modelPath)
	}
	if provider == ProviderOpenVINO {
		cacheDir := filepath.Join(cacheRoot(), "openvino")
		if err := os.MkdirAll(cacheDir, 0o755); err == nil {
			so.OVCacheDir = cacheDir
		}
	}
	if provider == ProviderDML {
		so.DMLDeviceID = max(gpuID, 0)
	}
	if providerNeedsCUDA(provider) {
		if cfg.VramGB > 0 {
			so.GPUMemLimit = uint64(cfg.VramGB * 1024 * 1024 * 1024)
		} else if m.Kind == KindInterp {
			baseGB := clampFloat(float64(tw*th)/700000, 3.0, 10.0)
			perSess := (nWorkers + nInst - 1) / nInst
			autoGB := baseGB * float64(perSess) / 2
			if autoGB < 2.0 {
				autoGB = 2.0
			}
			so.GPUMemLimit = uint64(autoGB * 1024 * 1024 * 1024)
		} else if m.Kind == KindEnhance {
			autoGB := clampFloat(float64(tw*th*cfg.Scale*cfg.Scale)/600000, 4.0, 12.0)
			so.GPUMemLimit = uint64(autoGB * 1024 * 1024 * 1024)
		}
		if gpuID >= 0 {
			so.CUDADeviceID = gpuID
		}
	}
	if cfg.Threads > 0 {
		so.IntraOpThreads = cfg.Threads
	}
	if m.Key == "aion" && provider == ProviderOpenVINO {
		so.FreeDims = aionFreeDims(tw, th, true)
	}
	if provider == ProviderTensorRT || provider == ProviderOpenVINO {
		Phase("compile")
		defer Phase("")
	}
	sessionStart := time.Now()
	if provider == ProviderOpenVINO {
		if strings.EqualFold(device, "NPU") {
			Infof("Compiling OpenVINO model for NPU; the first run per model and shape may take several minutes")
		} else {
			Infof("Preparing OpenVINO model for %s (compiling if not cached)", device)
		}
	}
	if provider == ProviderTensorRT {
		Infof("Preparing TensorRT engine for %s; the first run per tile shape may take several minutes", trtCachePrefix(modelPath))
	}
	env, err := NewEnv()
	if err != nil {
		return nil, err
	}
	botSO := so
	if bottomPath != "" {
		botSO.TRTCachePrefix = trtCachePrefix(bottomPath)
	}
	if m.Key == "aion" && provider == ProviderOpenVINO {
		botSO.FreeDims = aionFreeDims(tw, th, false)
	}
	mainCh := make(chan poolResult, 1)
	go func() {
		p, e := createSessions(env, raw, so, nInst)
		mainCh <- poolResult{pool: p, err: e}
	}()
	var botCh chan poolResult
	if bottomData != nil {
		botCh = make(chan poolResult, 1)
		go func() {
			p, e := createSessions(env, bottomData, botSO, nInst)
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
	var bpool []*Session
	if botCh != nil {
		br := <-botCh
		if br.err != nil {
			for _, s := range pool {
				s.Destroy()
			}
			return nil, fmt.Errorf("failed to load bottom model: %w", br.err)
		}
		bpool = br.pool
	}
	if provider == ProviderOpenVINO {
		if msg := cpuFallbackMessage(); msg != "" {
			for _, s := range pool {
				s.Destroy()
			}
			for _, s := range bpool {
				s.Destroy()
			}
			return nil, fmt.Errorf("OpenVINO failed to compile for %s and fell back to CPU; stopped instead of running on CPU, use another device or provider (%s)", device, msg)
		}
		Infof("OpenVINO model ready in %.1fs", time.Since(sessionStart).Seconds())
	}
	if provider == ProviderTensorRT {
		Infof("TensorRT session ready in %.1fs", time.Since(sessionStart).Seconds())
	}
	Phase("")
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
		r.bottomOutType = Float
		if bo, err := bpool[0].Outputs(); err == nil && len(bo) > 0 {
			r.bottomOutType = bo[0].Type
		}
	}

	for _, p := range m.Params {
		v := cfg.Params[p.Key]
		r.paramVec[p.Tensor] = clampFloat(v/100.0, p.Min, p.Max)
	}
	if m.Kind == KindInterp {
		r.plan = newInterpPlan(cfg.FW, cfg.FH, tw, th, overlap, r.BatchLimit())
	}
	if m.Kind == KindEnhance {
		r.enhPlan = newEnhancePlan(cfg.FW, cfg.FH, tw, th, cfg.Scale, overlap)
	}
	return r, nil
}

type poolResult struct {
	pool []*Session
	err  error
}

// applyCUDNNAlgoFromEnv selects the cuDNN convolution algorithm from
// GOPAZ_CUDNN_ALGO, which accepts exhaustive, heuristic, or default.
func applyCUDNNAlgoFromEnv(so *SessionOptions) error {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("GOPAZ_CUDNN_ALGO"))) {
	case "", "exhaustive":
		so.CUDNNAlgo = 0
	case "heuristic":
		so.CUDNNAlgo = 1
	case "default":
		so.CUDNNAlgo = 2
	default:
		return fmt.Errorf("GOPAZ_CUDNN_ALGO must be exhaustive, heuristic, or default")
	}
	return nil
}

// readModel reads a prepared .onnx file or unpacks a .tz file.
func readModel(path string) ([]byte, error) {
	Debugf("model file: %s", path)
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read model: %w", err)
	}
	if strings.HasSuffix(strings.ToLower(path), ".onnx") {
		return b, nil
	}
	raw, err := Extract(b, Password)
	if err != nil {
		return nil, fmt.Errorf("failed to unpack model: %w", err)
	}
	return raw, nil
}

// createSessions creates n identically configured sessions concurrently.
func createSessions(env *Env, modelData []byte, so SessionOptions, n int) ([]*Session, error) {
	out := make([]*Session, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			out[i], errs[i] = NewSessionFromBytes(env, modelData, so)
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

// Close releases every session.
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

// TileSize returns the tile size in input space.
func (r *Runner) TileSize() (int, int) { return r.tileH, r.tileW }

// ProcessTile runs enhancement inference on one input tile and returns the
// upscaled tile. ox and oy are the tile coordinates on the output canvas.
func (r *Runner) ProcessTile(tiles []*Frame, scale, ox, oy int) (*Frame, error) {
	if r.cfg.Model.Kind != KindEnhance {
		return nil, fmt.Errorf("ProcessTile does not support this model kind")
	}
	return r.runEnhance(tiles, scale, ox, oy)
}

// inputType returns the element type of the named input, defaulting to float32.
func (r *Runner) inputType(name string) int {
	for _, in := range r.inputs {
		if in.Name == name {
			return in.Type
		}
	}
	return Float
}

// outputType returns the element type of the first output.
func (r *Runner) outputType() int {
	if len(r.outputs) > 0 {
		return r.outputs[0].Type
	}
	return Float
}

// frameTensor builds a frame tensor with the given element type.
func frameTensor(f *Frame, elemType int) (*Tensor, error) {
	dims := []int64{1, int64(f.H), int64(f.W), 3}
	if elemType == Float16 {
		h := f32SliceToF16(f.Data)
		return NewTensor(dims, Float16, ptrOfU16(h), len(h))
	}
	return NewTensor(dims, Float, ptrOf(f.Data), len(f.Data))
}

func (r *Runner) runEnhance(tiles []*Frame, scale, ox, oy int) (*Frame, error) {
	m := r.cfg.Model
	tw, th := r.tileW, r.tileH
	var inputNames []string
	var tensors []*Tensor
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
			return nil, fmt.Errorf("%s requires %d temporal frames", m.Key, m.Temporal)
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
		return nil, fmt.Errorf("failed to build enhancement inputs for model %s", m.Key)
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

// addInfer accumulates inference nanoseconds atomically.
func (r *Runner) addInfer(ns int64) {
	atomic.AddInt64(&r.inferNanos, ns)
}

// addBottomInfer accumulates bottom-model inference nanoseconds.
func (r *Runner) addBottomInfer(ns int64) {
	atomic.AddInt64(&r.inferNanos, ns)
	atomic.AddInt64(&r.bottomNanos, ns)
}

// addMainInfer accumulates main-model inference nanoseconds.
func (r *Runner) addMainInfer(ns int64) {
	atomic.AddInt64(&r.inferNanos, ns)
	atomic.AddInt64(&r.mainNanos, ns)
}

// addMerge accumulates batch merge nanoseconds.
func (r *Runner) addMerge(ns int64) {
	atomic.AddInt64(&r.mergeNanos, ns)
}

// ResetInferTiming clears accumulated inference time after a warmup.
func (r *Runner) ResetInferTiming() {
	atomic.StoreInt64(&r.inferNanos, 0)
	atomic.StoreInt64(&r.bottomNanos, 0)
	atomic.StoreInt64(&r.mainNanos, 0)
	atomic.StoreInt64(&r.mergeNanos, 0)
}

// extractHRTile extracts the matching tile from the previous output as hr_prev.
// It returns a zero frame for the first frame, when prevHR is nil.
func (r *Runner) extractHRTile(w, h, ox, oy int) *Frame {
	if r.prevHR == nil {
		return NewFrame(w, h)
	}
	return r.prevHR.ExtractTile(ox, oy, w, h)
}

// paramTensor builds the params tensor using the declared element type.
func (r *Runner) paramTensor(v []float64) (*Tensor, error) {
	f := make([]float32, len(v))
	for i, x := range v {
		f[i] = float32(x)
	}
	dims := []int64{1, int64(len(v)), 1, 1}
	et := r.inputType("params")
	if et == Float16 {
		h := f32SliceToF16(f)
		return NewTensor(dims, Float16, ptrOfU16(h), len(h))
	}
	return NewTensor(dims, Float, ptrOf(f), len(f))
}

// scalarTensor builds a scalar tensor using the declared element type.
func (r *Runner) scalarTensor(v float64) (*Tensor, error) {
	f := []float32{float32(v)}
	dims := []int64{1, 1, 1, 1}
	et := r.inputType("scene_change_threshold")
	if et == Float16 {
		h := f32SliceToF16(f)
		return NewTensor(dims, Float16, ptrOfU16(h), 1)
	}
	return NewTensor(dims, Float, ptrOf(f), 1)
}

// tensorToFrameTyped reads an output tensor as a float32 frame.
func tensorToFrameTyped(t *Tensor, w, h int, elemType int) (*Frame, error) {
	p := t.Data()
	if p == nil {
		return nil, fmt.Errorf("tensor data is nil")
	}
	n := w * h * 3
	f := &Frame{W: w, H: h, Data: make([]float32, n)}
	if elemType == Float16 {
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

// cacheRoot returns the provider cache directory next to the executable.
// GOPAZ_CACHE_DIR overrides the location; unwritable executable directories
// fall back to the per-user cache directory.
func cacheRoot() string {
	if dir := strings.TrimSpace(os.Getenv("GOPAZ_CACHE_DIR")); dir != "" {
		return dir
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Join(filepath.Dir(exe), "cache")
		if err := os.MkdirAll(dir, 0o755); err == nil {
			return dir
		}
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "Gopaz")
}

// trtCachePrefix derives a stable per-model engine cache prefix.
func trtCachePrefix(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// aionFreeDims returns the fixed Aion dimensions for the current tile.
func aionFreeDims(tw, th int, withIn8x bool) map[string]int64 {
	dims := map[string]int64{
		"bsize":   1,
		"fheight": int64(th),
		"fwidth":  int64(tw),
	}
	if withIn8x {
		dims["fheight2"] = int64(th / 4)
		dims["fwidth2"] = int64(tw / 4)
	}
	return dims
}
