**Gopaz architecture and implementation**

Gopaz is a local inference runtime for Topaz Video AI models. It decodes with ffmpeg, runs ONNX models through ONNX Runtime and encodes with ffmpeg. The CLI and Web UI are thin clients over the same pipeline; the Web UI adds preview/compare and persisted state.

Implementation is based on the `.tz` container, ONNX graph structure, Topaz runtime logs and end-to-end comparisons. The repository contains no reverse-engineering scripts, Topaz installations, model JSON or runtime logs. Gopaz uses its own `GOPAZ_*` settings and is independent of Topaz `TVAI_*` settings.

## 1 Purpose and constraints

Five user-facing models are implemented. Aion also uses the aiob bottom model. Enhancement produces one output frame per input frame; interpolation produces subframes between source frames.

| Category | Models |
|---|---|
| Enhancement | Proteus, Gaia, Gaia-CG |
| Interpolation | Apollo, Aion |

Nyx, Iris, Rhea, Theia and Artemis are not implemented. They share the same `.tz` container, tiling and parameter mechanisms.

| Constraint | Value | Consequence |
|---|---|---|
| Language | Go 1.26, `package main` | Core code lives in the `gopaz/` root |
| cgo | Disabled | ONNX Runtime is called through `syscall` |
| Platform | Windows x64 | ORT DLLs, CUDA, NVENC and ffmpeg subprocesses |
| Backends | CUDA, TensorRT, DirectML, OpenVINO | CPU fallback is available |
| Video I/O | ffmpeg and ffprobe subprocesses | RGB24 decode, encode then mux audio |
| Pixel format | RGB24 end to end | 10-bit and YUV pipelines are not implemented |

Design principles:

1. Match Topaz model inputs, parameter order, tile scoring, temporal windows and duplicate-frame semantics.
2. Overlap decode, inference and encode so the GPU is not idle during I/O.
3. Bound VRAM and surface failures with contextual errors instead of silent fallbacks.

## 2 System architecture

The code is split by responsibility rather than by feature. Model contract, inference, frame processing and output can be tested independently.

| Layer | Files | Responsibility |
|---|---|---|
| CLI and jobs | `main.go`, `serve.go`, `jobs.go` | Command parsing, Web server, one active job |
| Pipelines | `upscale.go`, `interpolate.go`, `interp_worker.go` | Window assembly, frame loop, output |
| Model runtime | `model.go`, `tz.go`, `onnxpatch.go`, `runner.go`, `tiling.go`, `enhance_plan.go` | Registry, container, graph patches, sessions, tile plans |
| ORT binding | `ort.go`, `offsets.go`, `ortinit.go`, `provider.go` | C ABI, runtime discovery, providers |
| Media I/O | `video.go`, `audio.go`, `encode.go` | ffmpeg/ffprobe subprocesses |
| Support | `frame.go`, `color.go`, `stream.go`, `f16.go`, `unsafe.go`, `parallel.go`, `gpu.go`, `range.go`, `log.go` | Frames, color, channels, parallel loops, VRAM, ranges, logs |

Pipeline stages:

| Stage | Work | Output |
|---|---|---|
| Decode | ffmpeg decodes to RGB24; `frameReader` uses a channel capacity of 2 | Bounded frame stream |
| Assemble | Enhancement temporal window or interpolation interval | Model input window |
| Plan | Tile positions, blend weights and normalization map | Tile jobs |
| Infer | Worker pool calls shared or per-worker ORT sessions | Tile tensors |
| Blend | Weighted float32 accumulation | Output frame |
| Encode | `frameWriter` uses a channel capacity of 4 and feeds ffmpeg | Temporary video |
| Mux | Second ffmpeg pass combines video and source audio | Final file |

Bounded decode and encode channels apply backpressure while inference occupies the GPU. Memory use stays stable when decode runs ahead or encode falls behind.

Concurrency rules:

| Resource | Rule |
|---|---|
| `Frame.Data` | Read-only after entering the reader channel; windows and caches hold references |
| Temporal state | `prevHR` is owned by `Runner`; frames update it serially, while tiles may run in parallel |
| Interpolation blend | Serial merge stage; inference workers continue with the next batch |
| Output frame | Owned by the writer goroutine; not modified after writing |
| ORT sessions | CUDA and OpenVINO share sessions across workers; TensorRT and DirectML use one session and one worker |
| Errors | Close the writer, destroy sessions and remove temporary output |

Startup:

1. Resolve the model file. Prefer a plain `.onnx`; fall back to `.tz`.
2. Apply graph patches in memory, create the main session pool and, for Aion, the bottom-model pool concurrently. Destroy created sessions if either pool fails.
3. Build the tile plan and start decode, inference and encode.

## 3 Model contract

A `.tz` file is a single-entry ZIP using deflate and traditional ZipCrypto. It decompresses to an ONNX protobuf. `gopaz prepare` extracts it to a plain `.onnx`. `LoadRunner` prefers `.onnx`, applies patches only in memory and uses `.tz` only when the plain file is missing or empty.

| Model | Short name | Type | Temporal inputs | Output channels | Output frames | Input precision | keepColor |
|---|---|---|---|---|---|---|---|
| Proteus | prob-v4 | Enhancement | 2 | 3 | 1 | fp16 | Yes |
| Gaia | ghq-v5 | Enhancement | 5 | 3 | 1 | fp16 | Yes |
| Gaia-CG | gcg-v5 | Enhancement | 5 | 3 | 1 | fp16 | Yes |
| Apollo | apo-v8 | Interpolation | 4 | 24 | 8 | fp16 | No |
| Aion | aion-v1 | Interpolation | 4 + in8x | 48 | 16 | fp32 | No |
| aiob | aiob-v1 | Interpolation bottom | 4 | 24 | 8 | fp32 | No |

ONNX inputs:

| Model | Inputs |
|---|---|
| Proteus | `lr_prev[1,H,W,3]`, `lr_curr[1,H,W,3]`, `params[1,6,1,1]`, `hr_prev[1,S·H,S·W,3]`, `scene_change_threshold` |
| Gaia / Gaia-CG | `VideoSR_Unet/inputFrame0..4:0[1,H,W,3]` |
| Apollo | `in0..in3[1,H,W,3]` |
| Aion | `in0..in3[1,H,W,3]`, `in8x[1,H/4,W/4,24]` |
| aiob | `in0..in3[1,H,W,3]` |

Aion and aiob require width and height divisible by 32.

Model files:

| Type | Name |
|---|---|
| Enhancement | `<prefix>-fp16-<H>x<W>-<scale>x-ox.tz` |
| Interpolation | `<prefix>-fp16-<H>x<W>-ox.tz` |
| Aion / aiob | `<single>-fp16-ox.tz` |

Model JSON stores blocks as `[W, H]`; file names use `HxW`. The order must be preserved or the tile table is transposed.

Proteus parameters always use this order:

| Index | CLI | Topaz label | Model space |
|---|---|---|---|
| 0 | deblur | Antialias/DeBlur | -1..1 |
| 1 | denoise | Reduce Noise | -1..1 |
| 2 | detail | Recover Details | -1..1 |
| 3 | dehalo | Dehalo | -1..1 |
| 4 | sharpen | Sharpen | -1..1 |
| 5 | fix-compression | Fix Compression | -1..1 |

CLI and Topaz sliders use -100..100; values are divided by 100 and clamped to `[-1,1]`. Topaz default is 0. `default: 0.5` in model JSON is metadata, not a runtime default. All six parameters accept negative values.

Parameters with different semantics:

| Parameter | Topaz | Gopaz |
|---|---|---|
| `vram` | VRAM fraction 0.1–1 | Per-session limit in GB; 0 = auto |
| `instances` | Extra instances, default 1 | Session count; 0 = auto |
| `workers` | Not present | Parallel workers; 0 = auto |
| `sensitivity` | Slider 0–100 | `rdt = sensitivity/1000` |
| `rdt` | Raw threshold -0.01..0.2 | Same value; -1 uses `sensitivity` |

Graph patches are applied to the in-memory ONNX only:

| Patch | Problem | Action |
|---|---|---|
| `PatchReduceMeanFp32` | fp16 `ReduceMean` overflows to Inf/NaN in Apollo, Aion and aiob | Cast input to fp32, then output back to fp16 |
| `PatchOutputFp16` | Output bandwidth | Change the final Cast target to fp16 and update the graph output type |

Proteus Auto loads the prap model (`models/proteus/prap-v3-fp32-ox.onnx`). It downscales the previous and current temporal frames to a maximum side of 384, packs them as NHWC `[1,H,W,6]` and gets `[1,6]` model-space parameters. The prap session runs on CPU in the same ORT runtime; injection is inside the serial enhancement loop.

## 4 Inference runtime

| ORT concern | Implementation |
|---|---|
| API versions | CUDA/TensorRT use 31; DirectML/OpenVINO use 24 |
| Function offsets | Generated from `onnxruntime_c_api.h` into `offsets.go`; `genoffsets -check` verifies them |
| Calls | `apiFn` reads the offset table; `call` invokes through `syscall.SyscallN` |
| Strings | UTF-8 names and UTF-16 model paths to match Windows `ORTCHAR_T` |
| Errors | `check` reads the message and releases `OrtStatus` |

| Object | Owner and release | Constraint |
|---|---|---|
| `Env` | `CreateEnv` / `ReleaseEnv` | Shared logger and thread pool |
| `Session` | `CreateSession` / `ReleaseSession` | Holds model and CUDA arena |
| `Tensor` | `CreateTensor...` / `ReleaseValue` | `NewTensorRef` requires the caller's memory to stay alive |
| `IOInfo` | Session metadata | Names are copied from the allocator |

Default session options:

| Field | Default |
|---|---|
| `IntraOpThreads` | 4 |
| `GraphOptLevel` | 99 |
| `CUDA` | true |
| `CUDADeviceID` | 0 |
| `GPUMemLimit` | 2 GiB |
| `CopyDefaultStream` | true |

CUDA EP options:

| Option | Value |
|---|---|
| `prefer_nhwc` | `1`; models are NHWC |
| `use_tf32` | `1` for interpolation |
| `gpu_mem_limit` | Estimated from tile area |
| `cudnn_conv_algo_search` | `EXHAUSTIVE`; `GOPAZ_CUDNN_ALGO` can select `heuristic` or `default` |

VRAM limits:

| Case | Estimate |
|---|---|
| Interpolation | `base = clamp(tileW*tileH / 700000, 3, 10)`; `perSess = ceil(workers / instances)`; `limit = max(2, base * perSess / 2)` GB |
| Enhancement | `limit = clamp(tileW*tileH*scale^2 / 600000, 4, 12)` GB |

`--vram` overrides the estimate. An undersized limit fails allocation and returns an error.

| Provider | Runtime directories | API | Concurrency | Startup and cache |
|---|---|---|---|---|
| CUDA | `runtime/NVIDIA/CUDA`, `runtime/NVIDIA/Compute` | 31 | Shared session, two workers by default | None |
| TensorRT | `runtime/NVIDIA/CUDA`, `runtime/NVIDIA/TensorRT`, `runtime/NVIDIA/Compute` | 31 | One session, one worker | Fixed-block models build at session creation; Aion builds at warmup; engines cached in `cache/tensorrt/` |
| DirectML | `runtime/DirectML` | 24 | One session, one worker | Sessions are recreated per process; no compile cache |
| OpenVINO | `runtime/OpenVINO` | 24 | Shared session, concurrent runs | Compiles at session creation; blobs cached in `cache/openvino/`; Aion fixes tile dimensions |
| CPU | Any installed ONNX Runtime | - | Shared session | None |

ORT plugins must sit beside `onnxruntime.dll`; CUDA and TensorRT plugins therefore live in `runtime/NVIDIA/CUDA`. `Compute` holds CUDA math libraries and cuDNN; `TensorRT` holds the official libraries. Compute libraries may come from `PATH`. The legacy flat `ort-dll/` layout is still loadable.

Backends and models are optional. `ProviderAvailable` and `Model.Available` filter `/api/config` and `gopaz models` using existing runtime directories and model files. Each task is a separate process and loads one ORT runtime; its runtime directories are added to `PATH`.

The cache root defaults to `cache/` beside the executable. `GOPAZ_CACHE_DIR` overrides it; an unwritable executable directory falls back to the user cache directory. Deleting the cache rebuilds TensorRT engines and OpenVINO blobs.

A new TensorRT model/shape builds an engine, and a new OpenVINO model compiles, on first use; both can take minutes. Later runs reuse the cache. Logs report build and ready times.

Device names are display-only; `--device` keeps stable IDs. `--device` takes precedence over `--gpu`; CUDA/TensorRT/DML use numeric indexes and OpenVINO uses `GPU.0`/`NPU`/`CPU`. `--gpu -1` forces CPU.

OpenVINO can silently fall back to its CPU plugin on compile failure with only a WARNING. Gopaz installs an ORT logger at WARNING severity and stops the task when `Falling back to OV CPU` appears.

## 5 Tiling and blending

Fixed tile selection is used by enhancement models and Apollo. For input `[W,H]`, candidate `[Wc,Hc]` and overlap `o`:

| Item | Formula |
|---|---|
| Horizontal tiles | `nx = ceil((W - o) / (Wc - o))` |
| Vertical tiles | `ny = ceil((H - o) / (Hc - o))` |
| Total tiles | `n = nx * ny` |
| Score | `score = n * 0.05 + (n * Wc * Hc - W * H) / (W * H)` |

Candidates with `Wc <= o` or `Hc <= o` are skipped. The lowest score wins. Candidates may exceed the input frame; extraction replicates edge pixels.

| Model | Candidates |
|---|---|
| Enhancement | 288×288, 352×256, 480×384, 384×480, 384×576, 416×576, 576×448, 672×384, 672×576 |
| Apollo | The nine above plus 704×1152, 1056×1088, 1344×1152 |

Aion uses a pixel budget instead of a tile table:

1. Default `maxPixels = 2.4M`.
2. With `budgetGB > 0`, `maxPixels = clamp(budgetGB * 600000, 262144, 4000000)`.
3. If `W*H <= maxPixels`, use `align32(W) × align32(H)`.
4. Otherwise split the long edge into `n` strips from 2 to 32. `span = align32(ceil((short + (n-1)*overlap) / n))`; use the first span with `long * span <= maxPixels`.

4K defaults to four 3840×576 strips.

Tiles start at `(0,0)` with stride `tile - overlap` and continue until the tile covers the frame. Each tile contributes a 2D ramp:

| Region | Weight |
|---|---|
| Left edge | `wx = (i+1)/(ramp+1)` |
| Middle | `wx = 1` |
| Right edge | `wx = (w-i)/(ramp+1)` |
| Vertical | `wy` by the same rule |
| Final | `weight = wx * wy` |

| Path | Ramp |
|---|---|
| Enhancement | `min(overlap*scale, tileW/4, tileH/4)` |
| Interpolation | `min(overlap, tileW/4, tileH/4)` |

The pipeline accumulates weights and stores the reciprocal:

| Item | Formula |
|---|---|
| Accumulate | `acc[x,y] = Σ weight_tile[x,y]` |
| Reciprocal | `inv[x,y] = 1 / acc[x,y]` |
| Output | `out[x,y] = (Σ weight_tile[x,y] * tile[x,y]) * inv[x,y]` |

Blending uses float32. Enhancement applies `inv` when writing RGB24; interpolation applies it in `WriteNorm`.

Interpolation batches tiles:

| Tensor | Shape |
|---|---|
| `dims` | `[batch, tileH, tileW, 3]` |
| `in0..in3` | Four temporal positions |
| `in8x` | `[batch, tileH/4, tileW/4, 24]` for Aion |
| `out` | `[batch, tileH, tileW, 24 or 48]` |

Default batch is 4; it drops to 1 when a tile covers the full width or height. The model batch dimension and tile count are additional limits.

## 6 Temporal processing

Enhancement uses a ring window of length `Temporal`; `CenterIdx` marks the current frame.

| Model | Temporal | CenterIdx | Window |
|---|---|---|---|
| Proteus | 2 | 1 | Previous and current |
| Gaia | 5 | 2 | Two before, previous, current, next, two after |

Reading preloads `lookAhead = Temporal - CenterIdx - 1` frames and clamps at boundaries.

Proteus temporal feedback:

| Item | Behavior |
|---|---|
| `hr_prev` | Previous frame's inference output, not the input frame |
| First frame | Initialized from a bilinear upscale of the current frame |
| Update order | Serial per frame; tiles may run in parallel; whole-frame tiles cannot be reordered across frames |
| Scene threshold | Fixed 6.5; the model compares fnet optical-flow magnitude and softly blends warped `hr_prev` |

Interpolation uses four source frames in order `u0-1`, `u0`, `u1`, `u1+1`; boundaries clamp. `u1-u0 = gap`.

Each interval outputs `span = gap * factor` subframe steps. The model produces `subdiv` subframes per inference: Apollo 8, Aion 16. For output position `i`:

1. `t = i / span`
2. `k = round(t * subdiv) - 1`
3. `k = clamp(k, 0, subdiv - 1)`

Equal `k` values are deduplicated, inferred once and repeated. Output frame count is `outCount = (total - 1) * factor + 1`.

| Model | Direct factors | Other factors |
|---|---|---|
| Apollo | 2/4/8 | 10/12/16 reuse subframes |
| Aion | 2/4/8/16 | 3/6/10/12 use the nearest subframe |

Duplicate frames use the fraction of significantly changed pixels:

| Item | Formula |
|---|---|
| Changed pixels | `changed = count(max(abs(dR), abs(dG), abs(dB)) > 10/255)` |
| Ratio | `ratio = changed / (W*H)` |
| Duplicate | `ratio < rdt` |
| Threshold | `rdt = sensitivity / 1000` |

If the next source frame is still a duplicate, the interval expands until a non-duplicate frame appears. Original frames inside the interval are replaced by interpolated frames; output count and FPS stay unchanged.

Aion runs two stages:

1. `aiob(in0..in3)` produces `[1,H,W,24]`, eight coarse frames.
2. A per-channel 4×4 mean downsamples it to `[1,H/4,W/4,24]`.
3. The result enters `aion` as `in8x` together with four frames.
4. `aion` produces `[1,H,W,48]`, sixteen frames.

Feeding zero to `in8x` is structurally valid but produces about 71% of normal luminance. Both models use the same input tiles.

## 7 Output pipeline

keepColor applies global Lab mean/variance matching when the model has `keepColor=1` and `--kcolor 1`:

| Item | Formula |
|---|---|
| Scale | `scale[k] = σ_src[k] / σ_dst[k]` |
| Offset | `offset[k] = μ_src[k] - scale[k] * μ_dst[k]` |
| Output | `dst'[k] = dst[k] * scale[k] + offset[k]` |

`k` covers L/a/b. `src` is the current input frame; `dst` is the model output. Values are clamped to `[0,1]`. Models with `keepColor=0` ignore the flag.

Encoding writes a temporary video, then a second ffmpeg pass muxes the source audio. Slow motion slows audio by `1/factor`; `atempo` is split into 0.5..100 stages. `copy` becomes AAC for slow motion. With a frame range, audio uses the same `-ss`/`-t` interval. A mux failure logs a warning and keeps the video without audio.

The encoder registry covers H.264, software H.264, H.265, H.265 Main10, AV1, AV1 10-bit, ProRes 422 HQ/422/422 LT, FFV1 10-bit 4:4:4/4:2:0 and VP9.

| Rate control | Behavior |
|---|---|
| `cq` | NVENC uses `constqp`; H.264 High/Mid/Low map to 18/25/28, H.265 to 17/25/28; encoders without CQP use defaults |
| `cbr` | Constant bitrate at `-b:v <N>M` |
| `vbr` | Variable bitrate with a quality target and maximum rate |

Encoder input is RGB24; 10-bit encoders convert from 8-bit. Default `auto` probes H.265 NVENC, H.264 NVENC, then software H.264. NVENC requires an NVIDIA GPU; other GPUs use software encoders.

Frame ranges:

| Input | Range |
|---|---|
| Both -1 | Full video |
| Start only | Start to last frame |
| End only | 0 to end |
| Out of bounds | Clamp |
| Invalid | Error |

Decode uses ffmpeg `select='between(n,start,end)'` and `-vsync 0`; the mux pass trims audio to the same source interval. Rotation metadata is applied during probe, so tiling and output sizes use display orientation.

Output names are `<input>_<model short><version>`, for example `_prob4`, `_ghq5`, `_gcg5`, `_apo8` or `_aion1`. Sequential processing appends tags carried by the input name; collisions append `(N)` before the tag. Custom names use the same deduplication.

Preview processes a short source range at the current playback position. Duration comes from `previewDuration` (default 2 seconds). Files are written to `cache/preview/`, kept for the session and cleared when the Web UI starts. After preview or a full run, the UI enters compare mode: interpolation uses side-by-side, enhancement uses a divider. Processed video loops and the four modes switch without interrupting playback except when switching to original, which pauses. `--video-device` selects software, NVDEC or Quick Sync decode; hardware decode downloads with `hwdownload`, and zero decoded frames is an error.

## 8 Runtime state and failure policy

`config.json` sits beside the executable and is written with defaults on first run. `GOPAZ_CONFIG` overrides the path. Program options are `saveState`, `restoreVideo` and `port`.

`state` stores language, model, per-model parameters, backend and device, encoding and audio, advanced options, output directory, file name and log panel open state. The Web client debounces `POST /api/settings` by 700 ms and uses `sendBeacon` on page close. `/api/config` returns `app` for startup restore.

`params` is the current model's parameter copy; `modelParams` stores parameters per model. Loading an old config migrates `params` into the current model. `saveState=false` disables writes; `restoreVideo` reloads the last video; `--port` overrides the config port.

Failure policy:

| Failure | Handling |
|---|---|
| CUDA output NaN | Startup tile probe detects it; destroy sessions, reload CPU sessions with one instance and one worker |
| TensorRT warmup failure | Return an error; no CPU fallback |
| Non-CUDA inference failure | Return the original error; no CPU fallback |
| Graph patch parse failure | Continue with the original model; CUDA probe decides fallback |
| Model or session creation failure | Destroy already-created sessions and return an error |
| Decode failure | Close writer, destroy sessions and remove temporary output |
| Encode failure | Return with ffmpeg stderr |
| Audio mux failure | Warn and keep the video without audio |
| VRAM allocation or invalid range | Return an error and do not start or continue the pipeline |

Only CUDA probes for NaN with a 128×128 input or one tile. TensorRT warms up with a real tile; DirectML and OpenVINO do not probe.

## 9 Validation

PSNR condition: 720p, 2x, 30 frames, default parameters, `kcolor=1`, FFV1 output on both sides, per-frame comparison.

| Model | PSNR |
|---|---|
| Proteus | 46.9 dB |
| Gaia | 43.7 dB |
| Gaia-CG | 45.5 dB |

Apollo 1080p, 2x, 30 frames with duplicate replacement disabled matches Topaz at 59 frames / 30 fps / 1.966 s and average PSNR 42.6 dB. Apollo 16x at 320×240 matches 465 frames / 15.5 s at 38.3 dB.

Tile selection, parameter order, scene threshold, duplicate-frame threshold, keepColor and Aion two-stage behavior were checked against Topaz logs, model tensors and end-to-end comparisons.

## 10 Known limitations

| Area | Limitation |
|---|---|
| Pixel format | RGB24 end to end; 10-bit encoders convert from 8-bit |
| Factors and boundaries | 3/6/10/12 use nearest-subframe approximation; Topaz drops 1–3 frames at some duplicate-frame boundaries, Gopaz keeps output count and FPS unchanged |
| keepColor | Lab mean/variance matching is approximate; residual is about one 8-bit level |
| Aion | Strip tiling wastes less overlap than square tiles but can show seams; increase `--overlap`. Aion is unsupported on OpenVINO NPU |
| Backend concurrency | TensorRT and DirectML force one session and one worker; DirectML forbids concurrent `Run`; OpenVINO supports concurrent `Run` |
| DirectML | `device_id` follows DXGI enumeration order; Intel NPU is not a D3D12 adapter, so NPU requires OpenVINO |
| OpenVINO NPU | Tiles are limited to 576×672; larger tiles are reduced automatically |
| Automatic parameters | `tvai_pe` parameter estimation is not integrated |
| Model scope | Only Proteus, Gaia, Gaia-CG, Apollo and Aion are implemented |

## 11 Key defaults

| Setting | Value |
|---|---|
| Default tile overlap | 48 |
| Aion tile overlap | 32 |
| Tile-selection penalty | 0.05 |
| Scene-change threshold | 6.5 |
| Duplicate-frame threshold | 10/255 |
| Duplicate-frame rdt mapping | `sensitivity/1000` |
| Default Aion tile budget | 2.4M pixels |
| Tile alignment | 32 |
| Default VRAM limit | 2 GiB |
| Instance range | 1..3 |
| Worker range | 1..8 |
| Frame reader / writer channels | 2 / 4 |
| ORT API versions | 31 / 24 |
