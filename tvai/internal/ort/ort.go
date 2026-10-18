// Package ort 是一个纯 Go 的 ONNX Runtime 绑定，通过 syscall 直接调用
// onnxruntime.dll 导出的 OrtGetApiBase，不依赖 cgo 和 C 编译器。
package ort

import (
	"fmt"
	"maps"
	"runtime"
	"strconv"
	"sync"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

// Element type 常量，对应 ONNXTensorElementDataType。
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
	once       sync.Once
	apiPtr     uintptr
	initErr    error
	dllPath    string
	cpuMemInfo uintptr
)

// Init 加载 onnxruntime 动态库并初始化全局 API 表。path 为 onnxruntime.dll 路径。
func Init(path string) error {
	once.Do(func() {
		dllPath = path
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		dll := syscall.NewLazyDLL(path)
		proc := dll.NewProc("OrtGetApiBase")
		base, _, _ := proc.Call()
		if base == 0 {
			initErr = fmt.Errorf("OrtGetApiBase 返回 nil，无法加载 %s", path)
			return
		}
		getAPI := *(*uintptr)(unsafe.Pointer(base))
		tab, _, _ := syscall.SyscallN(getAPI, 31)
		if tab == 0 {
			initErr = fmt.Errorf("GetApi(31) 返回 nil")
			return
		}
		apiPtr = tab
		var mi uintptr
		if st := call(ortIdxCreateCpuMemoryInfo, 1, 0, uintptr(unsafe.Pointer(&mi))); st != 0 {
			initErr = fmt.Errorf("CreateCpuMemoryInfo 失败")
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

// Env 表示 ORT 环境（日志、全局状态）。
type Env struct{ ptr uintptr }

// NewEnv 创建环境。
func NewEnv() (*Env, error) {
	var p uintptr
	if err := check(call(ortIdxCreateEnv, 3, uintptr(unsafe.Pointer(cstr("tvai"))), uintptr(unsafe.Pointer(&p))), "CreateEnv"); err != nil {
		return nil, err
	}
	return &Env{ptr: p}, nil
}

// Destroy 释放环境。
func (e *Env) Destroy() {
	if e.ptr != 0 {
		call(ortIdxReleaseEnv, e.ptr)
		e.ptr = 0
	}
}

// appendCUDA 追加 CUDA 执行提供器，ProviderOpts 非空时走 V2 接口。
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

// Session 表示一个已加载的模型会话。
type Session struct {
	ptr uintptr
}

// SessionOptions 配置会话。
type SessionOptions struct {
	IntraOpThreads    int
	GraphOptLevel     int
	CUDA              bool
	CUDADeviceID      int
	GPUMemLimit       uint64
	CopyDefaultStream bool
	// CUDNNAlgo: 0=Exhaustive(默认), 1=Heuristic, 2=Default。某些模型在
	// Blackwell 上 exhaustive 选出的算法会产出 NaN，此时可调为 1 或 2。
	CUDNNAlgo int
	// ProviderOpts 通过 CUDA EP V2 接口追加或覆盖 provider 选项。
	ProviderOpts map[string]string
}

// DefaultSessionOptions 返回默认配置：CUDA 开启，全部图优化。
// GPUMemLimit 限制单个会话的 CUDA arena 上限（防止无界增长吃满显存）。
func DefaultSessionOptions() SessionOptions {
	return SessionOptions{
		IntraOpThreads:    4,
		GraphOptLevel:     99,
		CUDA:              true,
		CUDADeviceID:      0,
		GPUMemLimit:       2 * 1024 * 1024 * 1024, // 2GB/会话，防止无界增长吃满显存
		CopyDefaultStream: true,
	}
}

// NewSession 加载 ONNX 模型（路径为 UTF-16 宽字符，Windows ORTCHAR_T）。
func NewSession(env *Env, modelPath string, opt SessionOptions) (*Session, error) {
	var so uintptr
	if err := check(call(ortIdxCreateSessionOptions, uintptr(unsafe.Pointer(&so))), "CreateSessionOptions"); err != nil {
		return nil, err
	}
	defer call(ortIdxReleaseSessionOptions, so)

	call(ortIdxSetIntraOpNumThreads, so, uintptr(opt.IntraOpThreads))
	call(ortIdxSetSessionGraphOptimizationLevel, so, uintptr(opt.GraphOptLevel))

	if opt.CUDA {
		if err := appendCUDA(so, opt); err != nil {
			return nil, err
		}
	}

	var p uintptr
	pathPtr := wstr(modelPath)
	st := call(ortIdxCreateSession, uintptr(unsafe.Pointer(env.ptr)), uintptr(unsafe.Pointer(pathPtr)), so, uintptr(unsafe.Pointer(&p)))
	runtime.KeepAlive(pathPtr)
	if err := check(st, "CreateSession"); err != nil {
		return nil, err
	}
	return &Session{ptr: p}, nil
}

// Destroy 释放会话。
func (s *Session) Destroy() {
	if s.ptr != 0 {
		call(ortIdxReleaseSession, s.ptr)
		s.ptr = 0
	}
}

// IOInfo 描述一个输入或输出张量。
type IOInfo struct {
	Name string
	Type int
	Dim  []int64
}

// Inputs 返回模型全部输入的元数据。
func (s *Session) Inputs() ([]IOInfo, error) { return s.ioMeta(true) }

// Outputs 返回模型全部输出的元数据。
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

// Tensor 是一个 ORT 张量值。
type Tensor struct {
	ptr uintptr
	ref any
}

// NewTensor 创建一个 ORT 自有内存的张量并填入数据。data 会被拷贝。
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

// NewTensorRef 用调用方提供的内存创建张量，不拷贝数据。
// ref 用于保持 data 的生命周期，必须在张量使用期间有效。
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

// Data 返回张量数据指针（只读用途请勿越界写）。
func (t *Tensor) Data() unsafe.Pointer {
	var dp uintptr
	check(call(ortIdxGetTensorMutableData, t.ptr, uintptr(unsafe.Pointer(&dp))), "GetTensorMutableData")
	return unsafe.Pointer(dp)
}

// Destroy 释放张量。
func (t *Tensor) Destroy() {
	if t.ptr != 0 {
		call(ortIdxReleaseValue, t.ptr)
		t.ptr = 0
	}
}

// Run 执行推理。inputs 的顺序必须与 inputNames 一致。
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
