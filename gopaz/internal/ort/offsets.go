package ort

// Code generated from onnxruntime_c_api.h (ORT_API_VERSION 31). DO NOT EDIT.

const (
	ortIdxCreateStatus                                  = 0
	ortIdxGetErrorCode                                  = 1
	ortIdxGetErrorMessage                               = 2
	ortIdxReleaseStatus                                 = 93
	ortIdxCreateEnv                                     = 3
	ortIdxDisableTelemetryEvents                        = 6
	ortIdxReleaseEnv                                    = 92
	ortIdxCreateSession                                 = 7
	ortIdxCreateSessionFromArray                        = 8
	ortIdxRun                                           = 9
	ortIdxReleaseSession                                = 95
	ortIdxCreateSessionOptions                          = 10
	ortIdxReleaseTypeInfo                               = 98
	ortIdxSetIntraOpNumThreads                          = 24
	ortIdxSetInterOpNumThreads                          = 25
	ortIdxSetSessionExecutionMode                       = 13
	ortIdxSetSessionGraphOptimizationLevel              = 23
	ortIdxEnableCpuMemArena                             = 18
	ortIdxSessionOptionsAppendExecutionProvider_CUDA    = 152
	ortIdxSessionOptionsAppendExecutionProvider_CUDA_V2 = 204
	ortIdxCreateCUDAProviderOptions                     = 205
	ortIdxUpdateCUDAProviderOptions                     = 206
	ortIdxReleaseCUDAProviderOptions                    = 208
	ortIdxReleaseSessionOptions                         = 100
	ortIdxCreateCpuMemoryInfo                           = 69
	ortIdxReleaseMemoryInfo                             = 94
	ortIdxGetAllocatorWithDefaultOptions                = 78
	ortIdxCreateAllocator                               = 131
	ortIdxReleaseAllocator                              = 132
	ortIdxCreateTensorWithDataAsOrtValue                = 49
	ortIdxCreateTensorAsOrtValue                        = 48
	ortIdxGetTensorMutableData                          = 51
	ortIdxGetTensorTypeAndShape                         = 65
	ortIdxReleaseValue                                  = 96
	ortIdxGetDimensionsCount                            = 61
	ortIdxGetDimensions                                 = 62
	ortIdxGetTensorElementType                          = 60
	ortIdxCastTypeInfoToTensorInfo                      = 55
	ortIdxGetTypeInfo                                   = 66
	ortIdxGetOnnxTypeFromTypeInfo                       = 56
	ortIdxSessionGetInputCount                          = 30
	ortIdxSessionGetOutputCount                         = 31
	ortIdxSessionGetInputName                           = 36
	ortIdxSessionGetOutputName                          = 37
	ortIdxSessionGetInputTypeInfo                       = 33
	ortIdxSessionGetOutputTypeInfo                      = 34
	ortIdxCreateRunOptions                              = 39
	ortIdxRunOptionsSetTerminate                        = 46
	ortIdxReleaseRunOptions                             = 97
	ortIdxAllocatorFree                                 = 76
)
