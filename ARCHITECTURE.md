# Gopaz 架构与实现

> 文档范围：模型接口、推理后端、分块算法、时序处理、后处理和质量验证。
>
> CLI 参数与使用方式见 `README.md`。
>
> 本文不记录具体机器、绝对路径或基准成绩；实际性能取决于 GPU、驱动、运行时版本和模型尺寸。

## 1. 范围与约束

### 1.1 实现范围

Gopaz 复刻 Topaz Video AI 的本地推理链路。本文只描述推理管线，交互层不在本文范围内；项目管理和云端功能未实现。当前支持 5 个模型：

- 增强：Proteus、Gaia、Gaia-CG
- 插帧：Apollo、Aion

Topaz 的 Nyx、Iris、Rhea、Theia、Artemis 等模型不在实现范围内，但它们共用相同的 `.tz` 容器、分块和参数机制。

### 1.2 工程约束

| 约束 | 取值 | 影响 |
|---|---|---|
| 语言 | Go 1.26，`package main` | 所有核心代码位于 `gopaz/` 根目录 |
| cgo | 禁用 | ORT 通过 `syscall` 直接调用 C ABI |
| 平台 | Windows x64 | ORT DLL、CUDA、NVENC、ffmpeg 子进程 |
| 推理后端 | CUDA / TensorRT / DirectML / OpenVINO | 支持 CPU 回退 |
| 视频 I/O | ffmpeg / ffprobe 子进程 | 解码 RGB24，编码后二次并轨音频 |
| 像素格式 | 全程 RGB24 | 10bit / YUV 端到端未实现 |

### 1.3 设计目标

1. 与 Topaz 的模型输入输出、参数顺序、分块评分、时序窗口和重复帧语义一致。
2. GPU 利用率优先于单帧延迟：解码、推理、编码三阶段重叠。
3. 显存有界：每个会话设置 `gpu_mem_limit`，避免 ORT arena 随运行时间增长。
4. 失败可诊断：模型加载、推理、编码、并轨分别返回带上下文的错误。

## 2. 系统结构

### 2.1 组件划分

| 文件 | 职责 |
|---|---|
| `main.go` | CLI 参数解析、模型/工具路径解析 |
| `model.go` | 模型注册表、文件名、tile 表、参数、keepColor、时序帧数 |
| `tz.go` | `.tz` 的 ZipCrypto 解密与 deflate 解压 |
| `ort.go` `offsets.go` `ortinit.go` | ONNX Runtime C API 绑定、运行时选择与 provider 加载 |
| `provider.go` | Provider 名称、API 版本、设备参数校验 |
| `onnxpatch.go` | protobuf 级 ONNX 图修改：fp16 ReduceMean 补丁、输出 fp16 化 |
| `runner.go` | 模型加载、会话池、tile 推理、输出张量读取 |
| `upscale.go` | 增强管线：时序窗口、逐帧处理、keepColor、编码 |
| `interpolate.go` | 插帧管线：区间缓存、重复帧合并、子帧输出 |
| `interp_worker.go` | 插帧批推理、底部模型、降采样、子帧融合 |
| `tiling.go` | 固定块评分、Aion 动态切块、权重与子帧映射 |
| `enhance_plan.go` | 增强模型的 output-space tile 计划与权重 |
| `frame.go` | `Frame` 类型、RGB 转换、tile 提取、重复帧比例 |
| `stream.go` | 解码读取 goroutine、编码写入 goroutine |
| `color.go` | sRGB / Lab 转换与 Reinhard 色彩迁移 |
| `audio.go` | 第二趟 ffmpeg 并轨与 atempo 链 |
| `encode.go` | 编码器注册表与 ffmpeg 参数构造 |
| `video.go` | ffmpeg / ffprobe 子进程封装 |
| `f16.go` `unsafe.go` `parallel.go` `gpu.go` `range.go` | 数值转换、零拷贝视图、并行行处理、显存探测、帧范围 |
| `log.go` | 终端日志与进度输出 |

### 2.2 数据流

| 阶段 | 输入 | 处理 | 输出 |
|---|---|---|---|
| 1 | 输入视频 | ffmpeg 解码为 RGB24 rawvideo | 原始帧流 |
| 2 | 原始帧流 | `frameReader` goroutine，channel 容量 2 | 有界帧流 |
| 3 | 帧流 | 时序窗口或插帧区间缓存 | 待推理窗口 |
| 4 | 窗口 | 生成 tile 计划、融合权重和归一化图 | tile 任务 |
| 5 | tile 任务 | worker 池调用共享 ORT 会话 | 各 tile 输出张量 |
| 6 | tile 输出 | 按权重融合到输出画布 | 完整输出帧 |
| 7 | 输出帧 | `frameWriter` goroutine，channel 容量 4 | RGB24 帧流 |
| 8 | RGB24 帧流 | ffmpeg 编码为临时视频 | 临时视频 |
| 9 | 临时视频 + 原音轨 | 第二趟 ffmpeg 并轨 | 最终输出 |

两个 channel 都是有界的，分别对解码和编码施加背压。推理阶段占用 GPU 时，解码器可以继续预读，编码器可以继续转换和写盘。

### 2.3 并发契约

- `Frame.Data` 一旦进入 reader channel 即视为只读，窗口和 cache 中只保存引用。
- `prevHR` 属于 `Runner`，保存上一帧的推理输出；后续帧的 `hr_prev` 从该帧提取。
- 输出帧由 writer goroutine 独占，写入后不再修改。
- 错误通过 `reader.Err()`、`writer.Close()` 和 `Run*` 返回值传播；任一阶段失败会关闭 writer、销毁会话并删除临时输出。
- 插帧的 tile 融合由单独的 merge 阶段串行执行，推理 worker 可以继续处理下一个 batch。
- ORT 会话在多个 worker 间共享。代码依赖 onnxruntime 的 `Run` 并发安全性；同一会话不修改输入输出元数据。

### 2.4 启动顺序

1. 解析模型文件路径：优先同名明文 `.onnx`，否则按 tile 尺寸、scale 和 `.tz` 命名规则定位。
2. 读取明文 ONNX，或解包 `.tz`；应用 ONNX 补丁后直接从内存创建会话，不写临时 ONNX。
3. 初始化 ORT 环境，创建会话池；Aion 额外创建底部模型会话池。
4. 读取输入/输出元数据，构造参数张量范围。
5. 构造增强或插帧 plan，启动解码、推理、编码。

会话池和底部会话池并发创建。底部模型加载失败时销毁已创建的主会话，避免半初始化状态。

## 3. 模型接口

### 3.1 `.tz` 容器

`.tz` 是一个 ZIP，压缩方法为 deflate，通用标志 bit0=1，即传统 ZipCrypto 加密。每个包恰好包含一个条目，解包后是标准 ONNX protobuf。密码硬编码在 `aiengine.dll`，实现位于 `tz.go`。

`gopaz prepare` 可以把 `.tz` 转成同名明文 `.onnx`。`LoadRunner` 优先读取 `.onnx`，补丁只在内存应用；只有缺少 `.onnx` 时才解包 `.tz`。同名的 `.onnx` 必须是非空文件，否则回退到 `.tz`。

解析流程：

1. 从末尾向前搜索 EOCD，得到中央目录偏移。
2. 读取中央目录条目，得到 flags、method、压缩大小、本地头偏移。
3. 从本地文件头计算数据起点。
4. 用 ZipCrypto 解密 12 字节头，再逐字节解密数据。
5. method=0 直接返回，method=8 走 `compress/flate`。

模型文件名规则：

| 类型 | 规则 |
|---|---|
| 增强模型 | `<prefix>-fp16-<H>x<W>-<scale>x-ox.tz` |
| 插帧模型 | `<prefix>-fp16-<H>x<W>-ox.tz` |
| Aion / aiob | `<single>-fp16-ox.tz` |

模型 JSON 的 `blocks` 是 `[W, H]`，文件名中的顺序是 `HxW`。实现中必须保持这个方向，否则 tile 表会转置。

### 3.2 支持的模型

| 模型 | 短名 | 类型 | 时序输入 | 输出通道 | 输出帧 | 图输入精度 | keepColor |
|---|---|---|---|---|---|---|---|
| Proteus | prob-v4 | 增强 | 2 | 3 | 1 | fp16 | 是 |
| Gaia | ghq-v5 | 增强 | 5 | 3 | 1 | fp16 | 是 |
| Gaia-CG | gcg-v5 | 增强 | 5 | 3 | 1 | fp16 | 是 |
| Apollo | apo-v8 | 插帧 | 4 | 24 | 8 | fp16 | 否 |
| Aion | aion-v1 | 插帧 | 4 + in8x | 48 | 16 | fp32 | 否 |
| aiob | aiob-v1 | 插帧底部 | 4 | 24 | 8 | fp32 | 否 |

ONNX 输入：

| 模型 | 输入 |
|---|---|
| Proteus | `lr_prev[1,H,W,3]`、`lr_curr[1,H,W,3]`、`params[1,6,1,1]`、`hr_prev[1,S·H,S·W,3]`、`scene_change_threshold` |
| Gaia / Gaia-CG | `VideoSR_Unet/inputFrame0..4:0[1,H,W,3]` |
| Apollo | `in0..in3[1,H,W,3]` |
| Aion | `in0..in3[1,H,W,3]`、`in8x[1,H/4,W/4,24]` |
| aiob | `in0..in3[1,H,W,3]` |

Aion 和 aiob 要求输入宽高被 32 整除。

### 3.3 参数系统

Proteus 的 6 个参数按固定顺序打包：

| 下标 | CLI | Topaz 标签 | 模型空间 |
|---|---|---|---|
| 0 | deblur | Antialias/DeBlur | -1..1 |
| 1 | denoise | Reduce Noise | -1..1 |
| 2 | detail | Recover Details | -1..1 |
| 3 | dehalo | Dehalo | -1..1 |
| 4 | sharpen | Sharpen | -1..1 |
| 5 | fix-compression | Fix Compression | -1..1 |

CLI 与 Topaz 滑杆使用 -100..100，写入模型前除以 100 并 clamp 到 `[-1,1]`。Topaz 默认值为 0；模型 JSON 中的 `default: 0.5` 只是元数据基准，不是实际默认。六个参数都允许负值。

### 3.4 参数语义差异

| 参数 | Topaz | Gopaz |
|---|---|---|
| `vram` | 显存使用比例 0.1-1 | 每会话显存上限 GB，0=自动 |
| `instances` | 额外实例数，默认 1 | 会话数，0=自动 |
| `workers` | 无 | 并行 worker 数，0=自动 |
| `sensitivity` | Topaz 滑杆 0-100 | 映射为 `rdt = sensitivity/1000` |
| `rdt` | 原始阈值 -0.01..0.2 | 同义，-1 表示使用 sensitivity |

## 4. ONNX Runtime 后端

### 4.1 C API 绑定

`ort.go` 不依赖 cgo，通过 `syscall.NewLazyDLL` 加载 `onnxruntime.dll`，调用导出的 `OrtGetApiBase`。CUDA/TensorRT 使用 API 版本 31，DirectML/OpenVINO 使用 API 版本 24；现有函数表位置在这两个版本之间保持稳定。

- `OrtApi` 的函数指针偏移由 `onnxruntime_c_api.h` 解析生成，见 `offsets.go`；生成器为 `gopaz/tools/genoffsets`，可用 `-check` 校验。
- `apiFn` 根据偏移读取函数地址，`call` 统一通过 `syscall.SyscallN` 调用。
- 字符串参数使用 UTF-8 `cstr`，模型路径使用 UTF-16 `wstr`，与 Windows `ORTCHAR_T` 对齐。
- 错误状态由 `check` 读取错误消息并释放 `OrtStatus`。

### 4.2 对象生命周期

| 对象 | 创建 | 释放 | 约束 |
|---|---|---|---|
| `Env` | `CreateEnv` | `ReleaseEnv` | 全局日志和线程池 |
| `Session` | `CreateSession` 或 `CreateSessionFromArray` | `ReleaseSession` | 持有模型和 CUDA arena |
| `Tensor` | `CreateTensorAsOrtValue` 或 `CreateTensorWithDataAsOrtValue` | `ReleaseValue` | `NewTensorRef` 依赖外部内存存活 |
| `IOInfo` | 会话元数据 | 无 | 名称从 allocator 复制后释放原字符串 |

`NewTensor` 会复制数据；`NewTensorRef` 不复制，要求调用方保持底层切片存活，并用 `ref` 字段防止 GC 提前回收。`Run` 调用前后对名称指针和张量引用执行 `runtime.KeepAlive`。

### 4.3 会话配置

默认会话配置：

| 字段 | 默认值 |
|---|---|
| `IntraOpThreads` | 4 |
| `GraphOptLevel` | 99 |
| `CUDA` | true |
| `CUDADeviceID` | 0 |
| `GPUMemLimit` | 2 GiB |
| `CopyDefaultStream` | true |

CUDA EP 选项：

- `prefer_nhwc=1`：模型为 NHWC。
- `use_tf32=1`：插帧模型启用 TF32。
- `gpu_mem_limit`：按 tile 面积估算。
- `cudnn_conv_algo_search`：默认 EXHAUSTIVE；部分 GPU 组合可能产出 NaN，可用 `GOPAZ_CUDNN_ALGO=heuristic` 或 `default` 切换。

ProviderOpts 为空时走旧版 CUDA EP 结构体，否则走 CUDA EP V2 的 key/value 接口。

显存上限估算：

| 场景 | 估算 |
|---|---|
| 插帧 | `base = clamp(tileW*tileH / 700000, 3, 10)`，`perSess = ceil(workers / instances)`，`limit = max(2, base * perSess / 2)` GB |
| 增强 | `limit = clamp(tileW*tileH*scale^2 / 600000, 4, 12)` GB |

`--vram` 显式覆盖该估算。上限过低时单次分配会失败并返回错误，不会静默降级。

### 4.4 会话池与并发

- `instances` clamp 到 1..3；增强模型强制为 1。
- `workers` clamp 到 1..8；CUDA 默认 2，空闲显存低于 12GB 时自动降为 1；OpenVINO GPU 增强默认 4，Aion/NPU/CPU 默认 1。
- TensorRT 和 DirectML 强制单会话单 worker：DirectML 不允许同一 Session 并发 Run；TensorRT 多会话没有稳定收益。OpenVINO 允许同一 Session 并发 Run，多个 worker 共享一个会话。

### 4.5 执行后端选择

| Provider | 运行时目录 | ORT API | 备注 |
|---|---|---|---|
| CUDA | `ort-dll/` | 31 | 默认后端 |
| TensorRT | `ort-dll/tensorrt/` | 31 | 需要 TensorRT 10 和 CUDA provider 作为回退 |
| DirectML | `ort-dll/dml/` | 24 | DML 编入运行时，NPU 不可用 |
| OpenVINO | `ort-dll/openvino/` | 24 | GPU/NPU 默认 FP16，CPU 保持 FP32；NPU 不支持 Aion |

每个任务仍是独立子进程，因此一个进程只会加载一个 ORT 运行时。Provider 通过 `--provider` 和 `--device` 选择，子进程启动时把对应目录加入 PATH。DirectML 会预先加载同目录的 `DirectML.dll`；OpenVINO 会把 `libs/` 加入 PATH。

DirectML 的 `device_id` 对应 `IDXGIFactory::EnumAdapters` 的顺序，`gopaz devices` 用 DXGI 枚举显示实际适配器名称。TensorRT 在固定块模型上于会话创建时构建 engine，Aion 的动态 shape 在预热时构建；日志会打印 `TensorRT session ready` 或 `Warming up TensorRT engine`。engine 缓存的文件名前缀包含模型文件名和 tile 尺寸，避免不同 tile 变体共享同一 graph hash。同一前缀和 shape 再次运行直接复用，换分辨率或 `--gpu-vram` 会触发重建。OpenVINO 在会话创建时编译模型，NPU 首次编译可能较慢。OpenVINO 下 Aion 通过 `AddFreeDimensionOverrideByName` 把 `bsize`、`fheight`、`fwidth` 和 `in8x` 的尺寸固定成当前 tile，启用静态 shape 优化；DirectML 保持动态 shape。

缓存目录默认为可执行文件旁的 `cache/`（首次使用 TensorRT 或 OpenVINO 时自动创建），删除该文件夹即可清空缓存；可用 `GOPAZ_CACHE_DIR` 指向其他位置，可执行目录不可写时回退到系统用户缓存目录。

设备名称探测：`gopaz devices` 和 Web 界面显示真实设备名称，`--device` 参数仍使用稳定 ID，名称只做显示。CUDA/TensorRT 通过 `nvcuda.dll` 的驱动 API（`cuDeviceGetCount`、`cuDeviceGetName`、`cuDeviceTotalMem_v2`）枚举，附带显存；DirectML 用 DXGI 适配器名，专用显存低于 1GB 的 carve-out 不显示，VendorID `0x1414` 的软件适配器（WARP，即 Microsoft Basic Render Driver）直接过滤，原始枚举保留在 `GOPAZ_DEBUG=1` 日志里，设备 ID 仍是原始 DXGI 索引。OpenVINO 先把 `libs/` 加入 PATH 再加载 `openvino_c.dll`（否则传递依赖 `tbb12.dll` 无法解析），用 `ov_core_get_available_devices` 枚举真实存在的设备，再用 `FULL_DEVICE_NAME` 查出设备全名；GPU 插件会把 NVIDIA/AMD 等非 Intel GPU 也列成 OpenCL 设备（如 `GPU.1`），这些不受官方支持，用 `DEVICE_ARCHITECTURE` 的 `vendor=0x8086` 过滤掉；CPU 直接标注为 `CPU`。Web 设置面板的 GPU 下拉按 CUDA 顺序回退 DXGI 顺序显示名称；探测不到任何运行时就退回固定 `Device N` 标签。

设备选择：四个后端在 Web 的 Compute 面板都提供设备下拉，`--device` 优先于 `--gpu`；CUDA/TensorRT/DML 使用数字索引（`gopaz devices` 的 ID 列），OpenVINO 使用 `GPU.0`/`NPU`/`CPU`。`--gpu` 仅在 `--device` 省略时作为设备号兜底，`-1` 强制 CPU；空闲显存探测和 worker 自动值跟随最终选定的设备。

## 5. ONNX 图修补

### 5.1 fp16 ReduceMean 溢出

Apollo / Aion / aiob 在 CUDA 上输出全 NaN。根因是 fp16 `ReduceMean` 对大张量求和时累加器超过 65504，产生 Inf，再产生 NaN。

模型中的 ReduceMean 数量：

| 模型 | ReduceMean |
|---|---|
| Apollo | 存在并触发溢出 |
| Aion | 存在并触发溢出 |
| aiob | 存在并触发溢出 |

### 5.2 补丁算法

`PatchReduceMeanFp32` 在 protobuf 层定位 ReduceMean 节点并插入 Cast：

1. 解析 `GraphProto` 的 node、initializer 和 value_info。
2. 找到 ReduceMean 的输入。
3. 在输入前插入 Cast 到 float32。
4. 在输出后插入 Cast 回 float16。
5. 更新节点输入输出名称和类型声明。

补丁只修改内存中的 ONNX，不写回磁盘。

### 5.3 输出 fp16 化

`PatchOutputFp16` 把图末尾的 Cast 目标从 float32 改为 float16，并更新 graph output 的类型声明。插帧模型使用该补丁，减少输出张量带宽；参数和输入仍按模型原类型处理。

## 6. 分块与融合

### 6.1 固定块选块

增强模型和 Apollo 使用固定候选块。候选块为 `[Wc,Hc]`，输入 `[W,H]`，重叠 `o`。

| 项目 | 计算 |
|---|---|
| 水平块数 | `nx = ceil((W - o) / (Wc - o))` |
| 垂直块数 | `ny = ceil((H - o) / (Hc - o))` |
| 总块数 | `n = nx * ny` |
| 评分 | `score = n * 0.05 + (n * Wc * Hc - W * H) / (W * H)` |

规则：

- `Wc <= o` 或 `Hc <= o` 的候选跳过。
- 取 score 最小的候选。
- 候选块允许大于输入帧，超出部分由 tile 提取做边缘复制。
- penalty 默认 0.05，对应 `TilePenalty`。

候选块：

| 模型 | 候选块 |
|---|---|
| 增强 | 288×288、352×256、480×384、384×480、384×576、416×576、576×448、672×384、672×576 |
| Apollo | 上述 9 块加 704×1152、1056×1088、1344×1152 |

选块结果示例：

| 模型 | 输入 | 选中块 |
|---|---|---|
| Proteus 2x | 1280×720 | 672×384 |
| Proteus 2x | 1920×1080 / 3840×2160 | 672×576 |
| Apollo 2x | 1920×1080 | 1056×1088 |
| Apollo 2x | 3840×2160 | 1344×1152 |

### 6.2 Aion 动态切块

Aion 没有固定块表，按像素预算选择整帧或条带：

1. 默认预算 `maxPixels = 2.4M`。
2. 如果设置 `budgetGB > 0`，`maxPixels = clamp(budgetGB * 600000, 262144, 4000000)`。
3. 如果 `W*H <= maxPixels`，返回 `align32(W) × align32(H)`。
4. 否则沿长边切成 `n` 条，`n` 从 2 到 32：
   - `span = ceil((short + (n-1)*overlap) / n)`
   - `span = align32(span)`
   - 如果 `long * span <= maxPixels`，取该 span。

4K 默认使用 3840×576 的 4 条，重叠浪费小于方形小块。

### 6.3 权重与归一化

tile 网格从 `(0,0)` 开始，步长为 `tile - overlap`，直到 tile 右/下边界覆盖帧边界。每个 tile 的融合权重为二维斜坡。

| 区域 | 水平权重 |
|---|---|
| 左边缘 | `wx = (i+1)/(ramp+1)` |
| 中间 | `wx = 1` |
| 右边缘 | `wx = (w-i)/(ramp+1)` |
| 垂直方向 | `wy` 同理 |
| 最终权重 | `weight = wx * wy` |

增强模型的 ramp 在输出空间：

| 路径 | ramp |
|---|---|
| 增强 | `min(overlap*scale, tileW/4, tileH/4)` |
| 插帧 | `min(overlap, tileW/4, tileH/4)` |

归一化图预先累加所有 tile 覆盖处的权重，再取倒数：

| 项目 | 计算 |
|---|---|
| 权重累加 | `acc[x,y] = Σ weight_tile[x,y]` |
| 倒数 | `inv[x,y] = 1 / acc[x,y]` |
| 输出 | `out[x,y] = (Σ weight_tile[x,y] * tile[x,y]) * inv[x,y]` |

融合使用 float32 累加。增强路径在写 RGB24 时应用 `inv`；插帧路径在 `WriteNorm` 时应用。

### 6.4 tile 批处理

插帧 worker 把多个 tile 打包为一个批次。

| 张量 | 形状 |
|---|---|
| `dims` | `[batch, tileH, tileW, 3]` |
| `in0..in3` | 4 个时间位置的 tile |
| `in8x` | `[batch, tileH/4, tileW/4, 24]`（Aion） |
| `out` | `[batch, tileH, tileW, 24 或 48]` |

默认 batch=4；当 tile 覆盖整个宽度或高度时降为 1。最终 batch 还受模型输入 batch 维度限制和 tile 总数限制。

## 7. 时序处理

### 7.1 增强模型窗口

增强模型使用长度为 `Temporal` 的环形窗口，`CenterIdx` 指定当前帧：

| 模型 | Temporal | CenterIdx | 窗口 |
|---|---|---|---|
| Proteus | 2 | 1 | 前一帧 + 当前帧 |
| Gaia | 5 | 2 | 前二、前一、当前、后一、后二 |

读取时先预读 `lookAhead = Temporal - CenterIdx - 1` 帧。边界处 `frameAt` 对索引做 clamp，首帧和末帧用最近的合法帧填充。

Proteus 的时序反馈：

- `hr_prev` 来自上一帧的推理输出，不是输入帧。
- 首帧没有上一帧输出时，用当前帧的双线性放大结果初始化。
- `hr_prev` 按帧串行更新，tile 之间可以并行；因此不能把整帧 tile 跨帧重排。
- `scene_change_threshold` 固定传 6.5。模型内部用 fnet 光流幅度做 `Greater`，对 warp 后的 `hr_prev` 与当前帧做软混合。

### 7.2 插帧窗口

插帧以 4 帧为输入窗口，顺序是前一帧、当前帧、区间端点、区间后一帧，即 `u0-1`、`u0`、`u1`、`u1+1`。边界 clamp 到首帧/末帧。`u0` 和 `u1` 是当前区间的源帧端点；`u1-u0 = gap`。

### 7.3 子帧映射

每个源帧区间输出 `span = gap * factor` 个子帧间隔。模型一次产出 `subdiv` 个子帧（Apollo 8，Aion 16）。对第 `i` 个输出位置：

1. `t = i / span`
2. `k = round(t * subdiv) - 1`
3. `k = clamp(k, 0, subdiv - 1)`

相同 `k` 去重后只推理/融合一次，再按 `order` 重复写出。倍数与子帧的对应关系：

| 模型 | 直接对齐的倍数 | 其他倍数 |
|---|---|---|
| Apollo | 2/4/8 | 10/12/16 复用子帧 |
| Aion | 2/4/8/16 | 3/6/10/12 取最近子帧 |

输出帧数：`outCount = (total - 1) * factor + 1`。

### 7.4 重复帧合并

重复帧判定不是整帧平均差，而是显著变化像素的比例：

| 项目 | 计算 |
|---|---|
| 变化像素 | `changed = count(max(|dR|,|dG|,|dB|) > 10/255)` |
| 比例 | `ratio = changed / (W*H)` |
| 重复帧 | `duplicate = ratio < rdt` |
| 阈值 | `rdt = sensitivity / 1000` |

若下一个源帧与当前区间末端仍为重复帧，区间继续扩展，直到遇到非重复帧。区间内部不复制原帧，而是在 `u0` 与 `u1` 之间生成插值帧替换。输出帧数/FPS 保持不变。

### 7.5 Aion 两级推理

Aion 的 `in8x` 是 8 帧预测，形状 `[1,H/4,W/4,24]`，在图中经 `unshuffle_lr` 送入。喂零可以得到结构正确的插值，但输出亮度约为正常值的 71%。

正确流程：

1. `aiob(in0..in3)` 输出 `[1,H,W,24]`，表示 8 帧全分辨率粗结果。
2. 对每个通道做 4×4 均值降采样，得到 `[1,H/4,W/4,24]`。
3. 把结果作为 `in8x` 与 4 帧一起送入 `aion`。
4. `aion` 输出 `[1,H,W,48]`，表示 16 帧。

降采样是逐通道的 4×4 均值，除以 16。底部模型和主模型使用同一组输入 tile。

## 8. 后处理与输出

### 8.1 keepColor

当模型 `keepColor=1` 且 `--kcolor 1` 时，对输出帧做逐帧、全局的 Lab 均值/方差匹配：

| 项目 | 计算 |
|---|---|
| 缩放 | `scale[k] = σ_src[k] / σ_dst[k]` |
| 偏移 | `offset[k] = μ_src[k] - scale[k] * μ_dst[k]` |
| 输出 | `dst'[k] = dst[k] * scale[k] + offset[k]` |

`k` 为 L/a/b 三个通道；`src` 是当前输入帧，`dst` 是模型输出帧。实现使用 sRGB LUT 和 Lab 转换，最后 clamp 到 `[0,1]`。`keepColor=0` 的模型即使传 `--kcolor 1` 也不做处理。

### 8.2 音频并轨

编码先写临时视频，第二趟 ffmpeg 将原音轨并回：

```text
-map 0:v -map 1:a? -c:v copy
-map_metadata:s:a:0 1:s:a:0 -map_metadata 0
```

慢放时音频按 `1/factor` 减速。`atempo` 支持的区间为 0.5..100，低于 0.5 时拆成多段。`audio-codec copy` 在慢放时会自动改为 AAC。指定 `--start`/`--end` 时，音频输入用 `-ss` 和 `-t` 读取同一源区间，避免音轨长度与画面不一致。并轨失败时记录警告，保留无音轨输出，而不是丢弃整个视频。

### 8.3 编码

编码器注册表包含 H.264、H.264 软件、H.265、H.265 Main10、AV1、AV1 10bit、ProRes 422 HQ、ProRes 422、ProRes 422 LT、FFV1 10bit 4:4:4、FFV1 10bit 4:2:0、VP9。

码率控制分三种：

| 模式 | 行为 |
|---|---|
| `cq` | NVENC 走 `constqp`，附加 `-preset p7 -tune hq -rc-lookahead 20 -spatial_aq 1 -aq-strength 15 -b:v 0`；H.264 的 High/Mid/Low 对应 18/25/28，H.265 对应 17/25/28 |
| `cbr` | `-rc cbr -preset p6 -b:v <N>M` |
| `vbr` | `-preset p7 -tune hq -rc vbr -b:v 0 -cq 19 -maxrate <N>M` |

不支持 CQP 的编码器在 `cq` 模式下只使用自身默认参数；ProRes 不支持码率控制。所有输出统一追加 `-movflags +faststart`。

编码输入是 RGB24 rawvideo，10bit 编码器会从 8bit 转换后输出，这是当前管线的精度限制。默认 `auto` 会探测 NVENC：可用时用 H.265 NVENC，否则回退到软件 H.264。NVENC 编码器需要 NVIDIA GPU；其他 GPU 使用软件编码器。

### 8.4 帧范围

`--start`/`--end` 由 `ResolveFrameRange` 解析：

| 输入 | 范围 |
|---|---|
| 两端都为 -1 | 全片 |
| 只给 start | 从 start 到末帧 |
| 只给 end | 从 0 到 end |
| 越界 | 裁剪到有效范围 |
| 无效范围 | 返回错误 |

解码端通过 ffmpeg `select='between(n,start,end)'` 过滤，并设置 `-vsync 0`，输出帧数与范围一致。指定范围时，第二趟并轨用相同的源时间区间裁剪音轨。

带 rotation 元数据的视频在 `ProbeVideo` 阶段把宽高转换为显示方向，ffmpeg 解码时自动旋转，后续分块和输出尺寸都基于这一方向。

### 8.5 输出命名

自动命名对齐 Topaz 约定：输出文件名为 `<输入名>_<模型缩写><版本>`，例如 `_prob4`（Proteus）、`_ghq5`（Gaia）、`_gcg5`（Gaia-CG）、`_apo8`（Apollo）、`_aion1`（Aion），不记录倍率或倍数。连续处理时输入名自然携带已有标签，新标签继续追加，例如 `10月20日 (1)_ghq5_apo8.mp4`。输出重名时在名字后追加 `(N)` 再接标签，例如 `11月18日 (3)(1)_apo8.mp4`；自定义输出名同样用 `(N)` 去重。

## 9. 验证与质量

### 9.1 PSNR 条件

720p 2x、30 帧、默认参数、`kcolor=1`，双方 FFV1 输出逐帧比较：

| 模型 | PSNR |
|---|---|
| Proteus | 46.9 dB |
| Gaia | 43.7 dB |
| Gaia-CG | 45.5 dB |

Apollo 1080p 2x、30 帧、rdt 关闭：tile 1056×1088、59 帧 / 30fps / 1.966s 与 Topaz 一致，平均 PSNR 42.6dB，最低 36.7dB。最低值出现在高运动插值帧。

Apollo 16x（320×240、30 帧）：双方都是 465 帧 / 15.5s，平均 PSNR 38.3dB；Apollo 只有 8 个子帧，双方都会复用子帧。

### 9.2 一致性验证

| 项目 | 验证方式 | 结论 |
|---|---|---|
| 选块 | 读取 Topaz 运行日志中的 block size 记录 | 720p/1080p/4K 选择一致 |
| 参数顺序 | QML 映射、AVOption 内存布局、ONNX `params` 张量 | 三方一致 |
| 场景阈值 | `videoai.dll` 立即数 `0x40d00000` | 6.5 |
| 重复帧 | 合成片段 + Topaz `tvai_fi` 对照 | 10/255 像素比例阈值 |
| keepColor | 同帧 kcolor=0/1 + Lab 统计拟合 | 逐帧全局 Lab 均值/方差匹配 |
| Aion 两级 | in8x 零输入亮度对照 | 零输入亮度 71%，aiob 输入 100% |

## 10. 性能特征

- Aion 是卷积密集型模型。主干和底部模型都包含大量 3×3 dense 卷积、SE 通道注意力和 1×1 fuse 卷积；输出头和 Resize 在总耗时中占比很小。
- Aion 每个区间先运行底部模型，再把 8 帧粗结果降采样后送入主模型；两个模型都使用同一组输入 tile。
- TensorRT 为每个模型和输入 shape 构建 engine；首次构建较慢，之后从缓存加载。OpenVINO 在会话创建时编译并缓存；DirectML 没有跨进程编译缓存。
- CUDA 默认两个 worker 共享会话；TensorRT 和 DirectML 强制单 worker；OpenVINO 在增强模型上可以使用多个 worker。
- 绝对帧率取决于 GPU 算力、显存带宽、驱动、运行时版本和模型尺寸，建议在目标机器上用实际素材测量。

## 11. 失败模式与恢复

| 失败 | 检测 | 处理 |
|---|---|---|
| CUDA 输出 NaN | 启动时 tile 探测 | 销毁会话，重载 CPU 会话，`instances=1`、`workers=1` |
| TensorRT 预热失败 | 实际 tile 预热 | 返回错误，不回退 CPU |
| 非 CUDA 推理失败 | 管线返回错误 | 返回原始错误，不回退 CPU |
| ONNX 补丁解析失败 | `PatchReduceMeanFp32` 返回错误 | 使用原模型继续，由 NaN 探测决定是否回退 |
| 模型文件缺失 | `os.Stat` | 返回包含路径的错误 |
| 主/底部会话创建失败 | 创建结果 | 销毁已创建的会话，返回错误 |
| 解码失败 | `reader.Err()` | 关闭 writer，销毁会话，删除临时输出 |
| 编码失败 | `writer.Close()` / `enc.Close()` | 附带 ffmpeg stderr 返回 |
| 音频并轨失败 | ffmpeg 退出码 | 记录警告，保留无音轨输出 |
| 显存分配失败 | ORT 错误状态 | 返回错误，建议提高 `--vram` |
| 帧范围无效 | `ResolveFrameRange` | 返回错误，不启动管线 |

推理探测使用 128×128 输入（单文件模型）或单个 tile（固定块模型），输出含 NaN 即判定 CUDA 路径不可用。增强模型使用整块 tile 探测。该探测只对 CUDA 执行；TensorRT 改为用实际 tile 预热并构建 engine，DirectML/OpenVINO 不做探测。

## 12. 已知限制

- 10bit / YUV：当前管线全程 RGB24，10bit 编码器实际从 8bit 转出。
- Aion 接缝：条带切块的重叠小于方形块；接缝明显时可增大 `--overlap`。
- 非 2 幂倍数：3/6/10/12 的时间位置是最近子帧近似。
- 重复帧边界：保持输出帧数/FPS 不变；Topaz 在部分边界会少 1-3 帧，本工具不模拟。
- keepColor：Lab 均值/方差匹配是近似实现，残差约 1 个 8bit 色阶。
- 参数估计：`tvai_pe` 自动参数估计尚未集成。
- CUDA 兼容：部分 GPU 的 fp16 ReduceMean 溢出已通过图补丁解决；异常时可用 `--gpu -1`。
- 后端并发：TensorRT 和 DirectML 强制单会话单 worker；DirectML 不允许同一 Session 并发 Run，TensorRT 多会话没有稳定收益。OpenVINO 支持同一 Session 并发。
- Aion 输入尺寸：OpenVINO 用 `AddFreeDimensionOverrideByName` 固定当前 tile 尺寸，首次每个尺寸需要重新编译；DirectML 保持动态尺寸。
- 后端启动：TensorRT 每个新 tile shape 首次构建 engine；固定块模型在会话创建时构建，动态模型在预热时构建；OpenVINO NPU 首次编译可能较慢。日志会分别提示构建和就绪耗时。
- TensorRT 缓存：engine 按模型文件名和 tile 尺寸加 cache prefix。Apollo 的不同 tile 模型 graph hash 相同，缺少 prefix 时会错误复用其他尺寸的 engine 并报 `setInputShape` 失败。
- TensorRT 构建：默认 builder level 1，冷启动较快；level 3/5 不提升 Aion 推理速度，level 0 会明显变慢。workspace 上限为 8GiB；2GiB 时 Aion 的候选 tactic 因内存不足被全部跳过，报 `Could not find any implementation` 导致 engine 构建失败。
- DirectML 设备：`device_id` 是 DXGI 枚举顺序，索引不保证对应独显或核显；用 `gopaz devices` 查看名称。
- DirectML NPU：DirectML 基于 D3D12，当前 Intel NPU 不作为 D3D12 适配器暴露，NPU 只能走 OpenVINO。
- OpenVINO NPU：VPUX 编译器只接受 576x672 以内的 tile，更大 tile 无法编译（`tiling schema [1,1,1,1]`）；任务会自动缩小 tile 并提示 `NPU limits tile size`。Proteus/Gaia/Apollo 实测在 NPU 上真跑，首次编译可能很久（Proteus 4x 实测约 30 分钟，之后复用 blob 缓存）。Aion 在 NPU 上直接崩溃，模型标记 `NPUUnsupported`，Web 不显示 NPU，CLI 直接报错提示换 `GPU.0`。
- 静默回退防护：ORT OpenVINO EP 在编译失败时会静默改用 OV CPU 只留一条 WARNING。gopaz 用 `CreateEnvWithCustomLogger` 接管 ORT 日志（severity WARNING），检测到 `Falling back to OV CPU` 立即停止任务并报错，不会默默用 CPU 算完。
- 模型范围：只实现 Proteus、Gaia、Gaia-CG、Apollo、Aion。

## 13. TODO

- [ ] Apollo 10/12/16 子帧复用策略优化
- [ ] 增强模型长片段基准
- [ ] 自动参数估计 `tvai_pe` 集成
- [ ] 重复帧替换与 Topaz 的像素级对比
- [ ] 10bit/YUV 色彩管线与 keepColor 精确实现
- [ ] 子帧顺序与时间戳验证
- [ ] TensorRT、DirectML、OpenVINO 的多会话并发与性能基准
- [ ] 解决 OpenVINO NPU 上 Aion 的兼容问题
- [ ] 评估 DirectML/OpenVINO 多运行时目录的自动探测

## 附录 A：逆向依据

实现依据来自 `.tz` 容器、ONNX 图结构、Topaz 运行日志和端到端对照测试。仓库不包含逆向脚本、Topaz 安装目录、模型 JSON 或运行日志；这些内容只用于本机分析，不作为运行依赖。

Gopaz 使用自己的 `GOPAZ_MODEL_DIR`、`GOPAZ_ORT_DLL`、`GOPAZ_CACHE_DIR`、`GOPAZ_DEBUG`，与 Topaz 的 `TVAI_*` 无关。

## 附录 B：关键常量

| 常量 | 值 | 位置 |
|---|---|---|
| 默认分块重叠 | 48 | `model.go` |
| Aion 分块重叠 | 32 | `model.go` |
| 选块 penalty | 0.05 | `frame.go` |
| 场景切换阈值 | 6.5 | `runner.go` |
| 重复帧像素阈值 | 10/255 | `frame.go` |
| 重复帧 rdt 映射 | sensitivity/1000 | `interpolate.go` |
| 默认 Aion tile 预算 | 2.4M 像素 | `tiling.go` |
| tile 对齐 | 32 | `tiling.go` |
| 默认显存上限 | 2 GiB | `ort.go` |
| TensorRT engine cache | `GOPAZ_CACHE_DIR` 或用户缓存目录下的 `tensorrt/` | `runner.go` |
| OpenVINO cache | `GOPAZ_CACHE_DIR` 或用户缓存目录下的 `openvino/` | `runner.go` |
| 帧读取 channel | 2 | `stream.go` |
| 帧写入 channel | 4 | `stream.go` |
