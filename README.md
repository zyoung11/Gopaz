# Gopaz

Gopaz 是 Topaz Video AI 的本地推理工具，提供命令行和本地 Web 界面。输入视频，用本地 GPU 做超分/修复或慢放/插帧。纯 Go 实现，无 cgo、无 C 编译器、无 Python。

当前支持 Windows x64。

## 功能

| 命令 | 说明 |
|---|---|
| `gopaz` | 启动本地 Web 界面 |
| `gopaz serve` | 只启动 Web 界面 |
| `gopaz upscale` | 超分（2x/4x）或修复（1x） |
| `gopaz interpolate` | 慢放/插帧（2-16 倍） |
| `gopaz prepare` | 把 `.tz` 解压成明文 `.onnx` |
| `gopaz models` | 列出可用模型 |
| `gopaz encoders` | 列出可用编码器 |
| `gopaz devices` | 列出 CUDA、DirectML 和 OpenVINO 设备 |
| `gopaz help` / `gopaz -v` | 帮助（`-h`、`--help`）/ 版本（`--version`、`-V`） |

| 模型 | 类型 | 倍率 | 参数 |
|---|---|---|---|
| proteus | 增强 | 1/2/4 | 6 |
| gaia / gaia-cg | 增强 | 1/2/4 | 无 |
| apollo | 插帧 | 2-16 | 无 |
| aion | 插帧 | 2-16 | 无 |

## 依赖

1. **ffmpeg / ffprobe**：优先使用可执行文件同目录或 `bin/` 下的版本，其次 PATH，也可以用 `--ffmpeg`/`--ffprobe` 指定。
2. **ONNX Runtime**：CUDA/TensorRT 使用 API 31 的运行时；DirectML 和 OpenVINO 使用 API 24 的运行时。运行时目录结构见下面的便携目录。
3. **模型文件**：放在 `models/`，或用 `GOPAZ_MODEL_DIR` 指定；支持明文 `.onnx` 和加密 `.tz`，同名时优先 `.onnx`。
4. **CUDA + cuDNN**：CUDA/TensorRT 需要。优先使用可执行文件同目录 `runtime/` 中的运行库，否则需要在系统 PATH 上。
5. **可选后端**：TensorRT、DirectML、OpenVINO 分别需要对应的运行时目录；缺少时只影响对应 provider。

## 便携目录

可以把程序、运行时和模型放在同一个目录，直接运行，不需要安装。

| 路径 | 内容 |
|---|---|
| `gopaz.exe` | 主程序 |
| `config.json` | 首次运行自动生成的配置：UI 状态持久化和程序选项（见下） |
| `ffmpeg/` | `ffmpeg.exe` 和 `ffprobe.exe`，视频解码和编码 |
| `models/` | 按模型分子目录 `proteus/`、`gaia/`、`apollo/`、`aion/`；`.onnx` 或 `.tz` 模型，不需要 Topaz 的 JSON |
| `runtime/OpenVINO/` | ONNX Runtime + OpenVINO 后端，含 `libs/` |
| `runtime/DirectML/` | ONNX Runtime + DirectML 后端 |
| `runtime/NVIDIA/CUDA/` | ONNX Runtime + CUDA/TensorRT 后端插件，CUDA、TensorRT、CPU 三个后端共用 |
| `runtime/NVIDIA/TensorRT/` | TensorRT 运行库 `tensorrt_libs/` |
| `runtime/NVIDIA/Compute/` | CUDA 计算库 + cuDNN（cublas、cudnn、nvrtc 等），CUDA 和 TensorRT 共用 |
| `cache/` | 自动创建，TensorRT engine 和 OpenVINO 编译缓存；删除整个文件夹即可清缓存，下次运行重新构建；用 `GOPAZ_CACHE_DIR` 可改到其他位置 |

查找顺序：

| 文件 | 查找顺序 |
|---|---|
| ffmpeg / ffprobe | 可执行文件同目录 `ffmpeg/`，其次同目录、同目录 `bin/`，最后 PATH |
| 模型 | `GOPAZ_MODEL_DIR`，其次同目录 `models/<模型>/`，最后旧的平铺 `models/` |
| ORT | `GOPAZ_ORT_DLL`，其次 `runtime/<供应商>/<后端>/`，最后旧的 `ort-dll/` |

## 按需下载

运行时和模型都是可选的，缺什么下载什么，把文件夹放到便携目录对应位置即可，程序在启动时自动识别；Web 界面的后端和模型列表只显示实际存在的内容，放好后刷新页面即可。

| 想用 | 必需 | 可省略 |
|---|---|---|
| CUDA | `runtime/NVIDIA/CUDA` | `runtime/NVIDIA/Compute`——系统安装的 CUDA Toolkit + cuDNN 在 PATH 上即可 |
| TensorRT | `runtime/NVIDIA/CUDA` + `runtime/NVIDIA/TensorRT` | `runtime/NVIDIA/Compute` 同上；系统装了 TensorRT 在 PATH 上可省 `NVIDIA/TensorRT` |
| DirectML | `runtime/DirectML` | — |
| OpenVINO | `runtime/OpenVINO` | — |
| CPU | 任意一份 ONNX Runtime | — |

只放 `runtime/NVIDIA/CUDA` 不足以跑 CUDA：ORT 只是调度层，真正做计算的 cuBLAS/cuDNN 必须由 `NVIDIA/Compute` 文件夹或系统安装提供，二选一。

## 配置文件

`config.json` 在首次运行时自动生成在 `gopaz.exe` 同目录，包含程序选项和 UI 状态两部分：

```json
{
  "saveState": true,
  "restoreVideo": true,
  "port": 8765,
  "state": { "lang": "zh", "model": "proteus", "...": "上次的全部选择" }
}
```

| 选项 | 默认 | 说明 |
|---|---|---|
| `saveState` | `true` | 是否把 UI 状态持续写回本文件，关闭后 state 不再更新 |
| `restoreVideo` | `true` | 启动时自动加载上次选择的视频，置为 `false` 关闭 |
| `port` | `8765` | Web 端口，`--port` 参数优先于本项 |

`state` 保存使用过程中的全部选择：模型和参数、倍率/倍数、供应商和设备、编码器、容器、质量、码率控制、音频、线程/显存等高级参数、语言、输出目录和文件名。下次打开 Web 时自动恢复。

## 构建

```bash
go build -ldflags="-s -w" -o gopaz.exe .
```

Web 前端在 `web/`，通过 `//go:embed` 打包进可执行文件，不需要 Node 或构建步骤。

## CLI

### 超分 / 修复

```bash
gopaz upscale -i input.mp4 -o output.mp4 --model proteus --scale 2 --encoder h264-main-sw
```

- `--model`：`proteus`、`gaia`、`gaia-cg`
- `--scale`：`1`（修复）、`2`、`4`
- Proteus 参数：`--denoise`、`--detail`、`--dehalo`、`--sharpen`、`--fix-compression`、`--deblur`，取值 -100 到 100
- `--kcolor 1`：keepColor 色彩校正
- `--scene-threshold 6.5`：场景切换阈值

### 慢放 / 插帧

```bash
gopaz interpolate -i input.mp4 -o output.mp4 --model apollo --factor 4
gopaz interpolate -i input.mp4 -o output.mp4 --model aion --factor 8 --sensitivity 65
```

- `--model`：`apollo`、`aion`
- `--factor`：`2`、`3`、`4`、`6`、`8`、`10`、`12`、`16`
- `--sensitivity 10`：重复帧替换灵敏度 0 到 100，0 关闭
- `--rdt -1`：原始重复帧阈值覆盖，-1 表示使用 `--sensitivity`
- 慢放输出保持源 FPS，帧数和时长都是 N 倍，音频按同一倍率减速

### 通用选项

| 选项 | 说明 |
|---|---|
| `--provider cuda / tensorrt / dml / openvino / cpu` | 推理后端 |
| `--device <id>` | CUDA/TensorRT/DML 设备索引，或 OpenVINO `GPU.0`/`GPU.1`/`NPU`/`CPU` |
| `--gpu <id>` | -2 自动，-1 CPU；`--device` 省略时作为设备号兜底 |
| `--threads <n>` | 推理线程数 |
| `--workers <n>` | 并行 worker 数，0 自动 |
| `--instances <n>` | 插值并发会话数，0 自动 |
| `--overlap <px>` | 分块重叠，0 自动 |
| `--vram <GB>` | 每会话显存上限，0 自动 |
| `--gpu-vram <GB>` | Aion tile 预算，0 自动 |
| `--start <frame>`、`--end <frame>` | 只处理指定帧范围，音轨同步裁剪 |
| `--ffmpeg <path>`、`--ffprobe <path>` | 指定可执行文件路径 |

### 推理后端

| Provider | 设备 | 说明 |
|---|---|---|
| `cuda` | NVIDIA GPU | 默认后端，使用 CUDA 运行时 |
| `tensorrt` | NVIDIA GPU | 首次为每个模型和输入尺寸构建 engine，之后复用缓存 |
| `dml` | Direct3D 12 适配器 | 使用 DirectML 运行时，设备索引见 `gopaz devices` |
| `openvino` | Intel GPU / NPU / CPU | GPU 和 NPU 默认 FP16，CPU 使用 FP32；NPU 自动限制 tile ≤576x672，编译失败立即报错；Aion 不支持 NPU |

默认后端是 CUDA；非 NVIDIA 环境请显式选择 `--provider dml` 或 `--provider openvino`。

### 输出设置

```bash
--encoder h264-main-sw          # 见 gopaz encoders
--quality High                  # High/Mid/Low
--rate-control cq               # cq/cbr/vbr
--bitrate 20                    # cbr/vbr 码率 Mbps
--audio-codec copy              # copy/aac/ac3/pcm/vorbis/none
--audio-bitrate 320             # 音频码率 kbps
```

自动命名：`<输入名>_<模型缩写><版本>`（如 `_prob4`、`_ghq5`、`_apo8`、`_aion1`），连续处理时累加标签（如 `_ghq5_apo8`），重名时在名字后追加 `(N)`。

编码器与推理后端相互独立。默认 `auto` 会探测 NVENC：可用时用 H.265 NVENC，否则回退到软件 H.264。NVENC 编码器需要 NVIDIA GPU；其他环境也可以用 `--encoder` 手动选择 `h264-main-sw`、`vp9-best` 或 FFV1。

## Web 界面

```bash
gopaz
```

默认监听 `http://127.0.0.1:8765` 并自动打开浏览器。端口被占用时会自动顺延，也可以手动指定：

```bash
gopaz serve --port 9000 --no-open
```

在界面中选择视频、模型、倍率、参数和输出目录，然后开始处理。界面支持取消任务、查看进度和日志，以及从当前位置生成两秒预览。DirectML 的设备下拉框显示 DXGI 适配器名称；OpenVINO 显示设备类型。

Web 界面一次只运行一个任务；CLI 的全部功能仍然可用。

## 缓存与首次运行

- TensorRT 第一次为每个模型和输入尺寸构建 engine，之后从缓存加载。换分辨率或改变 tile 尺寸会重新构建。
- OpenVINO 在创建会话时编译模型并写入缓存；NPU 的首次编译通常比 GPU 更慢。Aion 在 OpenVINO 上会把输入尺寸固定为当前 tile，换 tiling 尺寸会重新编译。
- DirectML 没有持久编译缓存，每次进程启动都会重新创建会话。
- 首次构建/编译可能需要数十秒到数分钟，取决于模型和硬件；日志会显示构建或编译阶段。

缓存默认放在系统用户缓存目录。可以把缓存放到便携目录，例如：

```powershell
$env:GOPAZ_CACHE_DIR = "cache"
gopaz upscale ...
```

## 环境变量

| 变量 | 说明 |
|---|---|
| `GOPAZ_MODEL_DIR` | 模型目录 |
| `GOPAZ_ORT_DLL` | onnxruntime.dll 路径 |
| `GOPAZ_CACHE_DIR` | 缓存目录；相对路径相对于当前工作目录 |
| `GOPAZ_CUDNN_ALGO` | cuDNN 卷积算法：`exhaustive`、`heuristic`、`default` |
| `GOPAZ_DEBUG=1` | 输出诊断信息 |
| `GOPAZ_PROGRESS=json` | 以 JSON 行输出日志和进度 |

## 测试

```bash
go test .
```

端到端测试默认跳过，需要模型目录和 ONNX Runtime。未设置 `GOPAZ_MODEL_DIR` 或 `GOPAZ_ORT_DLL` 时会尝试 `portable/models`、`ort-dll/` 和 `portable/ort-dll/`。

```bash
GOPAZ_E2E=1 go test . -run E2E -v
GOPAZ_E2E=1 GOPAZ_E2E_GPU=1 GOPAZ_E2E_AION=1 go test . -run TestE2EAionDuplicate -v
```

## 声明

`.tz` 模型来自本机安装的 Topaz Video AI，仅用于本机/个人使用，请勿再分发。用 `gopaz prepare` 生成的明文 `.onnx` 同样不要分发。
