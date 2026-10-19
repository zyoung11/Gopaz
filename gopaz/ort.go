package main

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

// Element type constants matching ONNXTensorElementDataType.
const (
	Float   = 1
	Uint8   = 2
	Int8    = 3
	Uint16  = 4
	Int16   = 5
	Int32   = 6
	Int64   = 7
	String  = 8
	Bool    = 9
	Float16 = 10
	Double  = 11
	Uint32  = 12
	Uint64  = 13
)

var (
	once          sync.Once
	apiPtr        uintptr
	initErr       error
	dllPath       string
	ortDLL        *syscall.LazyDLL
	ortAPIVersion uint32
	cpuMemInfo    uintptr
)

// Init loads the onnxruntime shared library and initializes the global API table.
func Init(path string, apiVersion uint32) error {
	once.Do(func() {
		dllPath = path
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		dll := syscall.NewLazyDLL(path)
		proc := dll.NewProc("OrtGetApiBase")
		base, _, _ := proc.Call()
		if base == 0 {
			initErr = fmt.Errorf("OrtGetApiBase returned nil; cannot load %s", path)
			return
		}
		getAPI := *(*uintptr)(unsafe.Pointer(base))
		tab, _, _ := syscall.SyscallN(getAPI, uintptr(apiVersion))
		if tab == 0 {
			initErr = fmt.Errorf("GetApi(%d) returned nil for %s", apiVersion, path)
			return
		}
		apiPtr = tab
		ortDLL = dll
		ortAPIVersion = apiVersion
		var mi uintptr
		if st := call(ortIdxCreateCpuMemoryInfo, 1, 0, uintptr(unsafe.Pointer(&mi))); st != 0 {
			initErr = fmt.Errorf("CreateCpuMemoryInfo failed")
			return
		}
		cpuMemInfo = mi
	})
	return initErr
}

func apiFn(idx int) uintptr {
	return *(*uintptr)(unsafe.Pointer(apiPtr + uintptr(idx)*8))
}

func call(idx int, args ...uintptr) uintptr {
	r, _, _ := syscall.SyscallN(apiFn(idx), args...)
	return r
}

func check(st uintptr, what string) error {
	if st == 0 {
		return nil
	}
	msg := "unknown"
	if m := call(ortIdxGetErrorMessage, st); m != 0 {
		msg = byteStr(m)
	}
	call(ortIdxReleaseStatus, st)
	return fmt.Errorf("%s: %s", what, msg)
}

func byteStr(p uintptr) string {
	var b []byte
	for i := uintptr(0); ; i++ {
		c := *(*byte)(unsafe.Pointer(p + i))
		if c == 0 {
			break
		}
		b = append(b, c)
	}
	return string(b)
}

func cstr(s string) *byte {
	b := append([]byte(s), 0)
	return &b[0]
}

func wstr(s string) *uint16 {
	u := utf16.Encode([]rune(s))
	u = append(u, 0)
	return &u[0]
}

// Env represents an ORT environment holding logging and global state.
type Env struct{ ptr uintptr }

// NewEnv creates an environment.
func NewEnv() (*Env, error) {
	var p uintptr
	if err := check(call(ortIdxCreateEnv, 3, uintptr(unsafe.Pointer(cstr("gopaz"))), uintptr(unsafe.Pointer(&p))), "CreateEnv"); err != nil {
		return nil, err
	}
	return &Env{ptr: p}, nil
}

// Destroy releases the environment.
func (e *Env) Destroy() {
	if e.ptr != 0 {
		call(ortIdxReleaseEnv, e.ptr)
		e.ptr = 0
	}
}

// appendCUDA appends the CUDA execution provider, using the V2 API when
// ProviderOpts is set.
func appendCUDA(so uintptr, opt SessionOptions) error {
	if opt.ProviderOpts == nil {
		opts := make([]byte, 72)
		*(*int32)(unsafe.Pointer(&opts[0])) = int32(opt.CUDADeviceID)
		*(*int32)(unsafe.Pointer(&opts[4])) = int32(opt.CUDNNAlgo)
		*(*uint64)(unsafe.Pointer(&opts[8])) = opt.GPUMemLimit
		doCopy := int32(0)
		if opt.CopyDefaultStream {
			doCopy = 1
		}
		*(*int32)(unsafe.Pointer(&opts[20])) = doCopy
		return check(call(ortIdxSessionOptionsAppendExecutionProvider_CUDA, so, uintptr(unsafe.Pointer(&opts[0]))), "AppendCUDA")
	}
	var provider uintptr
	if err := check(call(ortIdxCreateCUDAProviderOptions, uintptr(unsafe.Pointer(&provider))), "CreateCUDAProviderOptions"); err != nil {
		return err
	}
	defer call(ortIdxReleaseCUDAProviderOptions, provider)
	algo := "EXHAUSTIVE"
	switch opt.CUDNNAlgo {
	case 1:
		algo = "HEURISTIC"
	case 2:
		algo = "DEFAULT"
	}
	doCopy := "0"
	if opt.CopyDefaultStream {
		doCopy = "1"
	}
	vals := map[string]string{
		"device_id":                 strconv.Itoa(opt.CUDADeviceID),
		"gpu_mem_limit":             strconv.FormatUint(opt.GPUMemLimit, 10),
		"cudnn_conv_algo_search":    algo,
		"do_copy_in_default_stream": doCopy,
	}
	maps.Copy(vals, opt.ProviderOpts)
	n := len(vals)
	keys := make([]*byte, 0, n)
	values := make([]*byte, 0, n)
	keyPtrs := make([]uintptr, 0, n)
	valPtrs := make([]uintptr, 0, n)
	for k, v := range vals {
		kp := cstr(k)
		vp := cstr(v)
		keys = append(keys, kp)
		values = append(values, vp)
		keyPtrs = append(keyPtrs, uintptr(unsafe.Pointer(kp)))
		valPtrs = append(valPtrs, uintptr(unsafe.Pointer(vp)))
	}
	st := call(ortIdxUpdateCUDAProviderOptions, provider,
		uintptr(unsafe.Pointer(&keyPtrs[0])), uintptr(unsafe.Pointer(&valPtrs[0])), uintptr(n))
	runtime.KeepAlive(keys)
	runtime.KeepAlive(values)
	if err := check(st, "UpdateCUDAProviderOptions"); err != nil {
		return err
	}
	return check(call(ortIdxSessionOptionsAppendExecutionProvider_CUDA_V2, so, provider), "AppendCUDAV2")
}

// keyValuePointers builds the C string arrays used by provider option APIs.
func keyValuePointers(vals map[string]string) ([]*byte, []*byte, []uintptr, []uintptr) {
	n := len(vals)
	keys := make([]*byte, 0, n)
	values := make([]*byte, 0, n)
	keyPtrs := make([]uintptr, 0, n)
	valPtrs := make([]uintptr, 0, n)
	for k, v := range vals {
		kp := cstr(k)
		vp := cstr(v)
		keys = append(keys, kp)
		values = append(values, vp)
		keyPtrs = append(keyPtrs, uintptr(unsafe.Pointer(kp)))
		valPtrs = append(valPtrs, uintptr(unsafe.Pointer(vp)))
	}
	return keys, values, keyPtrs, valPtrs
}

// appendTensorRT appends the TensorRT execution provider with CUDA fallback.
func appendTensorRT(so uintptr, opt SessionOptions) error {
	var provider uintptr
	if err := check(call(ortIdxCreateTensorRTProviderOptions, uintptr(unsafe.Pointer(&provider))), "CreateTensorRTProviderOptions"); err != nil {
		return err
	}
	defer call(ortIdxReleaseTensorRTProviderOptions, provider)
	vals := map[string]string{
		"device_id":                      strconv.Itoa(opt.CUDADeviceID),
		"trt_fp16_enable":                "1",
		"trt_builder_optimization_level": "3",
	}
	if opt.TRTCacheDir != "" {
		vals["trt_engine_cache_enable"] = "1"
		vals["trt_engine_cache_path"] = opt.TRTCacheDir
		vals["trt_timing_cache_enable"] = "1"
		vals["trt_timing_cache_path"] = opt.TRTCacheDir
	}
	if opt.TRTCachePrefix != "" {
		vals["trt_engine_cache_prefix"] = opt.TRTCachePrefix
	}
	keys, values, keyPtrs, valPtrs := keyValuePointers(vals)
	st := call(ortIdxUpdateTensorRTProviderOptions, provider,
		uintptr(unsafe.Pointer(&keyPtrs[0])), uintptr(unsafe.Pointer(&valPtrs[0])), uintptr(len(vals)))
	runtime.KeepAlive(keys)
	runtime.KeepAlive(values)
	if err := check(st, "UpdateTensorRTProviderOptions"); err != nil {
		return err
	}
	return check(call(ortIdxSessionOptionsAppendExecutionProvider_TensorRT_V2, so, provider), "AppendTensorRT")
}

// openVINOProviderOptions builds the OpenVINO execution provider options.
func openVINOProviderOptions(device, cacheDir string) map[string]string {
	if device == "" {
		device = "GPU.0"
	}
	vals := map[string]string{
		"device_type": device,
	}
	if !strings.EqualFold(device, "CPU") {
		vals["precision"] = "FP16"
	}
	if cacheDir != "" {
		vals["cache_dir"] = cacheDir
	}
	return vals
}

// appendOpenVINO appends the OpenVINO execution provider.
func appendOpenVINO(so uintptr, opt SessionOptions) error {
	vals := openVINOProviderOptions(opt.Device, opt.OVCacheDir)
	keys, values, keyPtrs, valPtrs := keyValuePointers(vals)
	st := call(ortIdxSessionOptionsAppendExecutionProvider_OpenVINO_V2, so,
		uintptr(unsafe.Pointer(&keyPtrs[0])), uintptr(unsafe.Pointer(&valPtrs[0])), uintptr(len(vals)))
	runtime.KeepAlive(keys)
	runtime.KeepAlive(values)
	return check(st, "AppendOpenVINO")
}

// appendDML appends the DirectML execution provider.
func appendDML(so uintptr, opt SessionOptions) error {
	if err := check(call(ortIdxDisableMemPattern, so), "DisableMemPattern"); err != nil {
		return err
	}
	if err := check(call(ortIdxSetSessionExecutionMode, so, 0), "SetSessionExecutionMode"); err != nil {
		return err
	}
	if ortDLL == nil {
		return fmt.Errorf("onnxruntime library is not initialized")
	}
	if dllPath != "" {
		dmlPath := filepath.Join(filepath.Dir(dllPath), "DirectML.dll")
		if _, err := os.Stat(dmlPath); err == nil {
			dmlDLL := syscall.NewLazyDLL(dmlPath)
			if err := dmlDLL.Load(); err != nil {
				return fmt.Errorf("load DirectML: %w", err)
			}
		}
	}
	proc := ortDLL.NewProc("OrtSessionOptionsAppendExecutionProvider_DML")
	st, _, _ := proc.Call(so, uintptr(opt.DMLDeviceID))
	return check(st, "AppendDML")
}

// Session represents a loaded model session.
type Session struct {
	ptr uintptr
}

// SessionOptions configures a session.
type SessionOptions struct {
	IntraOpThreads    int
	GraphOptLevel     int
	CUDA              bool
	CUDADeviceID      int
	GPUMemLimit       uint64
	CopyDefaultStream bool
	// CUDNNAlgo selects the cuDNN convolution algorithm: 0=Exhaustive (default),
	// 1=Heuristic, 2=Default. On Blackwell, exhaustive may pick an algorithm that
	// produces NaN, in which case 1 or 2 can be used.
	CUDNNAlgo int
	// ProviderOpts adds or overrides provider options through the CUDA EP V2 API.
	ProviderOpts   map[string]string
	Provider       Provider
	Device         string
	TRTCacheDir    string
	TRTCachePrefix string
	OVCacheDir     string
	DMLDeviceID    int
}

// DefaultSessionOptions returns the default configuration with CUDA enabled and
// all graph optimizations turned on. GPUMemLimit caps the CUDA arena of a single
// session so that it cannot grow without bound.
func DefaultSessionOptions() SessionOptions {
	return SessionOptions{
		IntraOpThreads:    4,
		GraphOptLevel:     99,
		CUDA:              true,
		CUDADeviceID:      0,
		GPUMemLimit:       2 * 1024 * 1024 * 1024,
		CopyDefaultStream: true,
	}
}

// buildSessionOptions creates the ORT session options used by both file and
// memory session creation.
func buildSessionOptions(opt SessionOptions) (uintptr, error) {
	var so uintptr
	if err := check(call(ortIdxCreateSessionOptions, uintptr(unsafe.Pointer(&so))), "CreateSessionOptions"); err != nil {
		return 0, err
	}
	call(ortIdxSetIntraOpNumThreads, so, uintptr(opt.IntraOpThreads))
	call(ortIdxSetSessionGraphOptimizationLevel, so, uintptr(opt.GraphOptLevel))
	switch opt.Provider {
	case ProviderTensorRT:
		if err := appendTensorRT(so, opt); err != nil {
			call(ortIdxReleaseSessionOptions, so)
			return 0, err
		}
		if err := appendCUDA(so, opt); err != nil {
			call(ortIdxReleaseSessionOptions, so)
			return 0, err
		}
	case ProviderDML:
		if err := appendDML(so, opt); err != nil {
			call(ortIdxReleaseSessionOptions, so)
			return 0, err
		}
	case ProviderOpenVINO:
		if err := appendOpenVINO(so, opt); err != nil {
			call(ortIdxReleaseSessionOptions, so)
			return 0, err
		}
	default:
		if opt.CUDA {
			if err := appendCUDA(so, opt); err != nil {
				call(ortIdxReleaseSessionOptions, so)
				return 0, err
			}
		}
	}
	return so, nil
}

// NewSession loads an ONNX model from a UTF-16 path, as required by ORTCHAR_T
// on Windows.
func NewSession(env *Env, modelPath string, opt SessionOptions) (*Session, error) {
	so, err := buildSessionOptions(opt)
	if err != nil {
		return nil, err
	}
	defer call(ortIdxReleaseSessionOptions, so)

	var p uintptr
	pathPtr := wstr(modelPath)
	st := call(ortIdxCreateSession, uintptr(unsafe.Pointer(env.ptr)), uintptr(unsafe.Pointer(pathPtr)), so, uintptr(unsafe.Pointer(&p)))
	runtime.KeepAlive(pathPtr)
	if err := check(st, "CreateSession"); err != nil {
		return nil, err
	}
	return &Session{ptr: p}, nil
}

// NewSessionFromBytes creates a session directly from ONNX bytes, avoiding a
// temporary model file.
func NewSessionFromBytes(env *Env, modelData []byte, opt SessionOptions) (*Session, error) {
	if len(modelData) == 0 {
		return nil, fmt.Errorf("empty ONNX model")
	}
	so, err := buildSessionOptions(opt)
	if err != nil {
		return nil, err
	}
	defer call(ortIdxReleaseSessionOptions, so)

	var p uintptr
	st := call(ortIdxCreateSessionFromArray, uintptr(unsafe.Pointer(env.ptr)), uintptr(unsafe.Pointer(&modelData[0])), uintptr(len(modelData)), so, uintptr(unsafe.Pointer(&p)))
	runtime.KeepAlive(modelData)
	if err := check(st, "CreateSessionFromArray"); err != nil {
		return nil, err
	}
	return &Session{ptr: p}, nil
}

// Destroy releases the session.
func (s *Session) Destroy() {
	if s.ptr != 0 {
		call(ortIdxReleaseSession, s.ptr)
		s.ptr = 0
	}
}

// IOInfo describes an input or output tensor.
type IOInfo struct {
	Name string
	Type int
	Dim  []int64
}

// Inputs returns the metadata of every model input.
func (s *Session) Inputs() ([]IOInfo, error) { return s.ioMeta(true) }

// Outputs returns the metadata of every model output.
func (s *Session) Outputs() ([]IOInfo, error) { return s.ioMeta(false) }

func (s *Session) ioMeta(input bool) ([]IOInfo, error) {
	var cnt uintptr
	var st uintptr
	if input {
		st = call(ortIdxSessionGetInputCount, s.ptr, uintptr(unsafe.Pointer(&cnt)))
	} else {
		st = call(ortIdxSessionGetOutputCount, s.ptr, uintptr(unsafe.Pointer(&cnt)))
	}
	if err := check(st, "GetCount"); err != nil {
		return nil, err
	}
	var alloc uintptr
	if err := check(call(ortIdxGetAllocatorWithDefaultOptions, uintptr(unsafe.Pointer(&alloc))), "GetAllocator"); err != nil {
		return nil, err
	}
	out := make([]IOInfo, 0, cnt)
	for i := uintptr(0); i < cnt; i++ {
		var namePtr *byte
		var ti uintptr
		if input {
			if err := check(call(ortIdxSessionGetInputName, s.ptr, i, alloc, uintptr(unsafe.Pointer(&namePtr))), "GetInputName"); err != nil {
				return nil, err
			}
			if err := check(call(ortIdxSessionGetInputTypeInfo, s.ptr, i, uintptr(unsafe.Pointer(&ti))), "GetInputTypeInfo"); err != nil {
				return nil, err
			}
		} else {
			if err := check(call(ortIdxSessionGetOutputName, s.ptr, i, alloc, uintptr(unsafe.Pointer(&namePtr))), "GetOutputName"); err != nil {
				return nil, err
			}
			if err := check(call(ortIdxSessionGetOutputTypeInfo, s.ptr, i, uintptr(unsafe.Pointer(&ti))), "GetOutputTypeInfo"); err != nil {
				return nil, err
			}
		}
		var tensorInfo uintptr
		call(ortIdxCastTypeInfoToTensorInfo, ti, uintptr(unsafe.Pointer(&tensorInfo)))
		var nd uintptr
		call(ortIdxGetDimensionsCount, tensorInfo, uintptr(unsafe.Pointer(&nd)))
		dims := make([]int64, nd)
		if nd > 0 {
			call(ortIdxGetDimensions, tensorInfo, uintptr(unsafe.Pointer(&dims[0])), nd)
		}
		var elem uintptr
		call(ortIdxGetTensorElementType, tensorInfo, uintptr(unsafe.Pointer(&elem)))
		call(ortIdxReleaseTypeInfo, ti)
		info := IOInfo{Name: byteStr(uintptr(unsafe.Pointer(namePtr))), Type: int(elem), Dim: dims}
		if alloc != 0 {
			call(ortIdxAllocatorFree, alloc, uintptr(unsafe.Pointer(namePtr)))
		}
		out = append(out, info)
	}
	return out, nil
}

// Tensor is an ORT tensor value.
type Tensor struct {
	ptr uintptr
	ref any
}

// NewTensor creates an ORT-owned tensor and copies data into it.
func NewTensor(dims []int64, elemType int, data unsafe.Pointer, nElems int) (*Tensor, error) {
	var alloc uintptr
	if err := check(call(ortIdxGetAllocatorWithDefaultOptions, uintptr(unsafe.Pointer(&alloc))), "GetAllocator"); err != nil {
		return nil, err
	}
	var p uintptr
	dimPtr := uintptr(unsafe.Pointer(&dims[0]))
	if err := check(call(ortIdxCreateTensorAsOrtValue, alloc, dimPtr, uintptr(len(dims)), uintptr(elemType), uintptr(unsafe.Pointer(&p))), "CreateTensor"); err != nil {
		return nil, err
	}
	t := &Tensor{ptr: p}
	if nElems > 0 {
		var dp uintptr
		if err := check(call(ortIdxGetTensorMutableData, p, uintptr(unsafe.Pointer(&dp))), "GetTensorMutableData"); err != nil {
			t.Destroy()
			return nil, err
		}
		dst := unsafe.Slice((*byte)(unsafe.Pointer(dp)), nElems*elemSize(elemType))
		src := unsafe.Slice((*byte)(data), nElems*elemSize(elemType))
		copy(dst, src)
	}
	return t, nil
}

// NewTensorRef creates a tensor backed by caller-provided memory without copying.
// ref keeps data alive and must remain valid while the tensor is in use.
func NewTensorRef(dims []int64, elemType int, data unsafe.Pointer, nElems int, ref any) (*Tensor, error) {
	var p uintptr
	dimPtr := uintptr(unsafe.Pointer(&dims[0]))
	st := call(ortIdxCreateTensorWithDataAsOrtValue, cpuMemInfo, uintptr(data),
		uintptr(nElems*elemSize(elemType)), dimPtr, uintptr(len(dims)), uintptr(elemType),
		uintptr(unsafe.Pointer(&p)))
	if err := check(st, "CreateTensorRef"); err != nil {
		return nil, err
	}
	return &Tensor{ptr: p, ref: ref}, nil
}

// Data returns the tensor data pointer. Do not write past the bounds.
func (t *Tensor) Data() unsafe.Pointer {
	var dp uintptr
	check(call(ortIdxGetTensorMutableData, t.ptr, uintptr(unsafe.Pointer(&dp))), "GetTensorMutableData")
	return unsafe.Pointer(dp)
}

// Destroy releases the tensor.
func (t *Tensor) Destroy() {
	if t.ptr != 0 {
		call(ortIdxReleaseValue, t.ptr)
		t.ptr = 0
	}
}

// Run executes inference. The order of inputs must match inputNames.
func (s *Session) Run(inputNames []string, inputs []*Tensor, outputNames []string) ([]*Tensor, error) {
	nIn := len(inputNames)
	namePtrs := make([]uintptr, nIn)
	valPtrs := make([]uintptr, nIn)
	inNames := make([]*byte, nIn)
	for i := range nIn {
		inNames[i] = cstr(inputNames[i])
		namePtrs[i] = uintptr(unsafe.Pointer(inNames[i]))
		valPtrs[i] = inputs[i].ptr
	}
	nOut := len(outputNames)
	outNamePtrs := make([]uintptr, nOut)
	outNames := make([]*byte, nOut)
	for i := range nOut {
		outNames[i] = cstr(outputNames[i])
		outNamePtrs[i] = uintptr(unsafe.Pointer(outNames[i]))
	}
	outVals := make([]uintptr, nOut)
	st := call(ortIdxRun, s.ptr, 0,
		uintptr(unsafe.Pointer(&namePtrs[0])), uintptr(unsafe.Pointer(&valPtrs[0])), uintptr(nIn),
		uintptr(unsafe.Pointer(&outNamePtrs[0])), uintptr(nOut),
		uintptr(unsafe.Pointer(&outVals[0])))
	runtime.KeepAlive(inNames)
	runtime.KeepAlive(outNames)
	if err := check(st, "Run"); err != nil {
		return nil, err
	}
	res := make([]*Tensor, nOut)
	for i := range nOut {
		res[i] = &Tensor{ptr: outVals[i]}
	}
	return res, nil
}

func elemSize(t int) int {
	switch t {
	case Float, Int32, Uint32:
		return 4
	case Double, Int64, Uint64:
		return 8
	case Float16, Int16, Uint16:
		return 2
	default:
		return 1
	}
}
