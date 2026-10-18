# Gopaz

Topaz Video AI 的本地复刻 CLI：输入视频，用本地 GPU 推理，输出超分/修复或慢放/插帧后的视频。纯 Go 实现，**无 cgo、无 C 编译器、无 Python**。

逆向依据、实现细节、性能数据与已知限制见 [ARCHITECTURE.md](ARCHITECTURE.md)。

## 功能

| 命令 | 说明 |
|---|---|
| `gopaz upscale` | 超分（2x/4x）或修复（1x） |
| `gopaz interpolate` | 慢放/插帧（2-16 倍） |
| `gopaz models` | 列出可用模型 |
| `gopaz encoders` | 列出可用编码器 |

| 模型 | 类型 | 倍率 | 参数 |
|---|---|---|---|
| proteus | 增强 | 1/2/4 | 6 |
| gaia / gaia-cg | 增强 | 1/2/4 | 无 |
| apollo | 插帧 | 2-16 | 无 |
| aion | 插帧 | 2-16 | 无 |

Apollo 单次推理产出 8 子帧，2/4/8 倍精确对齐；Aion 产出 16 子帧，2/4/8/16 倍精确对齐。其余倍数取最近子帧或复用子帧，与 Topaz 行为一致。

## 依赖

1. **ffmpeg / ffprobe** —— 优先用可执行文件同目录或同目录 `bin/` 下的版本，其次 PATH，也可用 `--ffmpeg`/`--ffprobe` 指定
2. **onnxruntime.dll 1.31+** —— 放在可执行文件同目录的 `ort-dll/`，或用 `GOPAZ_ORT_DLL` 指定；需要 onnxruntime-gpu（CUDA）
3. **模型文件（.tz）** —— 优先 `GOPAZ_MODEL_DIR`，其次可执行文件同目录的 `models/`，最后探测 Topaz 默认模型目录
4. **CUDA + cuDNN 9** —— 优先用可执行文件同目录的 `runtime/`（程序会自动加入 PATH），否则需要在系统 PATH 上

## 构建

```bash
CGO_ENABLED=0 go build -ldflags="-s -w" -o gopaz.exe ./cmd/gopaz
```

## 绿色便携版

把运行时文件放进同一个目录即可，不需要安装：

```
gopaz-portable\
├─ gopaz.exe
├─ ffmpeg.exe
├─ ffprobe.exe
├─ ort-dll\
│  ├─ onnxruntime.dll
│  ├─ onnxruntime_providers_cuda.dll
│  └─ onnxruntime_providers_shared.dll
├─ runtime\              # 可选，CUDA / cuDNN 9 的运行库
└─ models\               # 只放 .tz，不需要 Topaz 的 JSON
```

查找顺序：

- ffmpeg / ffprobe：exe 同目录 → exe 同目录 `bin\` → PATH
- 模型：`GOPAZ_MODEL_DIR` → exe 同目录 `models\` → Topaz 默认模型目录
- ORT：`GOPAZ_ORT_DLL` → exe 同目录 `ort-dll\` → 当前目录 `ort-dll\`
- CUDA / cuDNN：exe 同目录 `runtime\`、`cuda\`、`cudnn\` 会自动加入 PATH

## 用法

### 超分 / 修复

```bash
gopaz upscale -i 输入.mp4 -o 输出.mp4 --model proteus --scale 2 \
  --denoise 40 --sharpen 30 --detail 20 --dehalo 0 --fix-compression 15 --deblur 0
```

- `--model`：`proteus` | `gaia` | `gaia-cg`
- `--scale`：`1`（修复）| `2` | `4`
- Proteus 参数取值 -100~100，默认 0：`--denoise` `--detail` `--dehalo` `--sharpen` `--fix-compression` `--deblur`
- `--kcolor 1`：keepColor 色彩校正
- `--scene-threshold 6.5`：场景切换阈值
- `--workers 0`：tile 并行 worker 数，0=自动
- `--vram 0`：每会话显存上限 GB，0=自动

### 慢放 / 插帧

```bash
gopaz interpolate -i 输入.mp4 -o 输出.mp4 --model apollo --factor 4
gopaz interpolate -i 输入.mp4 -o 输出.mp4 --model aion --factor 8 --sensitivity 65
```

- `--model`：`apollo` | `aion`
- `--factor`：`2 3 4 6 8 10 12 16`
- `--sensitivity 10`：重复帧替换灵敏度 0-100，`rdt = sensitivity/1000`，0 关闭
- `--rdt -1`：原始 rdt 覆盖，-1 表示使用 `--sensitivity`
- `--instances 0`：插值并发会话数，0=自动（默认 1 个共享会话）
- `--workers 0`：插值/增强并行 worker 数，0=自动
- `--gpu-vram 0`：Aion tile 预算 GB，0=约 2.4M 像素
- `--vram 0`：每会话显存上限 GB，0=按 tile 自动

慢放输出保持源 FPS，帧数和时长都是 N 倍，音频按同一倍率减速。`--audio-codec copy` 在慢放时会自动改为 AAC。

重复帧替换与 Topaz `tvai_fi` 一致：相邻源帧中变化超过约 10 个 8bit 色阶的像素比例低于 `rdt` 时视为重复帧，在前后非重复帧之间生成插值帧替换，输出帧数/FPS 不变。

### 输出设置

```bash
--encoder h265-main-win-nvidia   # 见 gopaz encoders
--quality High                   # High/Mid/Low（CQ 模式）
--qp 17                          # 直接指定 constqp，优先于 quality
--rate-control cq                # cq / cbr / vbr
--bitrate 20                     # cbr/vbr 码率 Mbps
--audio-codec copy               # copy / aac / ac3 / pcm / vorbis / none
--audio-bitrate 320              # 音频码率 kbps
```

### 其他

```bash
--gpu -2          # -2 自动 / -1 CPU / 0+ 设备号
--threads 4       # 推理线程数
--overlap 0       # 分块重叠，0=自动（增强/Apollo 48，Aion 32）
--start 0 --end 99  # 只处理指定帧范围
--workdir <dir>   # 临时 ONNX 解包目录
```

## 环境变量

| 变量 | 说明 |
|---|---|
| `GOPAZ_MODEL_DIR` | `.tz` 模型目录 |
| `GOPAZ_ORT_DLL` | onnxruntime.dll 路径 |
| `GOPAZ_DEBUG=1` | 输出诊断信息 |

注意：Topaz 自己的 `TVAI_MODEL_DIR` 是元数据 JSON 目录，本工具的语义不同。

## 测试

```bash
go test ./...
```

端到端回归默认跳过，需要模型目录和 onnxruntime：

```bash
GOPAZ_E2E=1 go test ./internal/pipeline -run E2E -v
GOPAZ_E2E=1 GOPAZ_E2E_GPU=1 GOPAZ_E2E_AION=1 go test ./internal/pipeline -run TestE2EAionDuplicate -v
```

## 声明

`.tz` 模型来自本机安装的 Topaz Video AI，仅用于本机/个人使用，请勿再分发。
