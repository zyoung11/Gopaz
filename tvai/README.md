# tvai

Topaz Video AI 的本地复刻 CLI。输入视频，用本地 GPU 对模型推理，输出超分/修复或慢放/插帧后的视频。纯 Go 实现，**无需 cgo、无需 C 编译器、无需 Python**。

## 功能

| 命令 | 说明 |
|---|---|
| `tvai upscale` | 超分（2x/4x）或修复（1x） |
| `tvai interpolate` | 慢放/插帧（2-16 倍） |
| `tvai models` | 列出可用模型 |
| `tvai encoders` | 列出可用编码器 |

### 模型

- **超分/修复**：`proteus`（6 个可调参数）、`gaia`、`gaia-cg`（多帧时序，无参数），支持 1x/2x/4x
- **插帧/慢放**：`apollo`（8 子帧）、`aion`（16 子帧，两级结构），支持 2-16 倍

> 2/4/8/16 是 2 的幂，能精确对齐模型子帧；3/6/10/12 取最接近子帧（时间轴有轻微近似）。

## 依赖

1. **ffmpeg / ffprobe** —— 在 `PATH` 上，或用 `--ffmpeg`/`--ffprobe` 指定
2. **onnxruntime.dll 1.31+** —— 放在可执行文件同目录的 `ort-dll/`，或用环境变量 `TVAI_ORT_DLL` 指定。需 onnxruntime-gpu（CUDA）版本
3. **模型文件**（`.tz`）—— 用环境变量 `TVAI_MODEL_DIR` 指向 Topaz 的 models 目录，默认探测 `C:\zy\APP\Big_APPs\TopazVideoAI\models`
4. **CUDA + cuDNN 9** —— 在 `PATH` 上（GPU 推理）

## 构建

```bash
CGO_ENABLED=0 go build -ldflags="-s -w" -o tvai.exe ./cmd/tvai
```

## 用法

### 超分 / 修复

```bash
tvai upscale -i 输入.mp4 -o 输出.mp4 --model proteus --scale 2 \
  --denoise 40 --sharpen 30 --detail 20 --dehalo 0 --fix-compression 15 --deblur 0
```

- `--model`：`proteus` | `gaia` | `gaia-cg`
- `--scale`：`1`（修复）| `2` | `4`
- 参数（仅 proteus 支持，取值 0-100，`--deblur` 为 -100~100）：
  - `--denoise` 降噪、`--detail` 恢复细节、`--dehalo` 去光晕
  - `--sharpen` 锐化、`--fix-compression` 修复压缩、`--deblur` 抗锯齿/去模糊

### 慢放 / 插帧

```bash
tvai interpolate -i 输入.mp4 -o 输出.mp4 --model apollo --factor 4
```

- `--model`：`apollo` | `aion`
- `--factor`：慢放倍数 `2 3 4 6 8 10 12 16`

### 输出设置（两个命令通用）

```bash
--encoder h265-main-win-nvidia   # 编码器（见 tvai encoders）
--quality High                   # 质量档 High/Mid/Low（CQ 模式）
--qp 17                          # 直接指定 constqp 值（优先于 --quality）
--rate-control cq                # cq / cbr / vbr
--bitrate 20                     # cbr/vbr 码率 (Mbps)
--audio-codec copy               # copy / aac / ac3 / pcm / vorbis / none
--audio-bitrate 320              # 音频码率 kbps（aac/ac3）
```

默认编码器 `h265-main-win-nvidia`、质量 `High`、码率模式 `cq`、音频 `copy`（直接复制原音轨）。

### 其他

```bash
--gpu -2          # -2 自动 / -1 CPU / 0+ 设备号
--threads 4       # 推理线程数
--instances 1     # 插值并发实例数（单 GPU 上 1 通常最优，多实例显存翻倍且吞吐增益有限）
--rdt 0.03        # 去重阈值 Sensitivity，相邻帧差异低于此值时复制跳过推理，0 关闭
--vram 2          # 每会话显存上限 GB（默认 2，防止无界增长吃满显存）
--overlap 16      # 分块重叠像素
--start 0 --end 99  # 只处理指定帧范围
--workdir <dir>   # 临时 ONNX 解包目录
```

## 环境变量

| 变量 | 说明 |
|---|---|
| `TVAI_MODEL_DIR` | `.tz` 模型目录 |
| `TVAI_ORT_DLL` | onnxruntime.dll 路径 |
| `TVAI_DEBUG=1` | 输出诊断信息 |

## 实现要点

- **纯 Go 调用 onnxruntime C API**：通过 `syscall` 直接调用 DLL 导出的 `OrtGetApiBase`，46 个 `OrtApi` 结构体偏移由官方 `onnxruntime_c_api.h`（ORT_API_VERSION 31）解析生成，见 `internal/ort/offsets.go`。
- **`.tz` 解包**：`.tz` 是 ZipCrypto 加密的 ZIP，内含标准 ONNX。密码硬编码于 `aiengine.dll`，见 `internal/tz`。
- **CUDA fp16 ReduceMean 溢出修复**：onnxruntime CUDA 上 fp16 大张量归约会累加溢出（超过 65504）导致 NaN。`internal/onnxpatch` 在模型加载时对每个输出为 fp16 的 ReduceMean 节点插入 `Cast(fp32) → ReduceMean → Cast(fp16)`，改用 fp32 累加。protobuf 级字段解析/重写，仅改节点列表其余原样保留。
- **Aion 两级插帧**：Aion 需要 `in8x`（8 帧预测，尺寸为 tile/4）。先用底部模型 `aiob` 从 4 帧产出 8 帧，块平均降采样到 tile/4 后喂给 Aion。`in8x` 为零时输出亮度仅 71%，两级后 100%。

## 已知限制

- 插帧模型在部分 Blackwell GPU 上曾有 CUDA 兼容问题（fp16 ReduceMean 溢出），已通过 ONNX 补丁解决；若仍异常可 `--gpu -1` 退回 CPU（正确但慢）。
- **Aion 大视频较慢**：Aion 是两级推理（aiob+aion），每帧 4K 约数十秒。当前实现 GPU 占用在 0-100% 间横跳（未做流水线重叠，GPU 约 40% 时间在空转等 CPU）。Topaz 靠流水线重叠 + TensorRT + Sensitivity 去重跳帧达到稳定 80% GPU，这些尚未复刻，见调查文档的性能分析。
- `rdt` 与 `instances` 已实现；流水线重叠（解码‖推理‖编码）尚未做，GPU 仍有空转。
- Aion 按训练尺寸（32 的倍数）分块推理，实测小 tile 比大 tile 更快。
- 非 2 幂倍数（3/6/10/12）的时间位置为最近子帧近似。
- Topaz 在 RTX 5090（cc 12.0）上因 TensorRT 后端不支持 Blackwell 而回退 onnxruntime，其速度来自软件优化（批处理/流/去重）而非 TRT，见调查文档。
