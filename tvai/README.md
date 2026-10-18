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

- **超分/修复**：`proteus`（6 参数）、`gaia` / `gaia-cg`（5 帧时序，无参数）
- **插帧/慢放**：`apollo`（8 子帧）、`aion`（16 子帧，两级结构），支持 2-16 倍

| 模型 | 倍率 | 参数 |
|---|---|---|
| proteus | 1/2/4 | 6 |
| gaia / gaia-cg | 1/2/4 | 无 |
| apollo / aion | 插帧 | 无 |

> 2/4/8/16 是 2 的幂，能精确对齐模型子帧；3/6/10/12 取最接近子帧（时间轴有轻微近似）。

## 依赖

1. **ffmpeg / ffprobe** —— 在 `PATH` 上，或用 `--ffmpeg`/`--ffprobe` 指定
2. **onnxruntime.dll 1.31+** —— 放在可执行文件同目录的 `ort-dll/`，或用环境变量 `TVAI_ORT_DLL` 指定。需 onnxruntime-gpu（CUDA）版本
3. **模型文件**（`.tz`）—— 用环境变量 `TVAI_MODEL_DIR` 指向 Topaz 的 **.tz 模型目录**，默认探测 `C:\zy\APP\Big_APPs\TopazVideoAI\models`。注意：Topaz 自己的 `TVAI_MODEL_DIR` 是元数据 JSON 目录，本工具的语义不同，见下表
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
- 参数（仅 proteus，取值 **-100~100**，默认 0）：
  - `--denoise` 降噪、`--detail` 恢复细节、`--dehalo` 去光晕
  - `--sharpen` 锐化、`--fix-compression` 修复压缩、`--deblur` 抗锯齿/去模糊
- `--kcolor 1`：keepColor 色彩校正（默认开）
- `--scene-threshold 6.5`：模型内部场景切换阈值（Topaz 常量）

### 慢放 / 插帧

```bash
tvai interpolate -i 输入.mp4 -o 输出.mp4 --model apollo --factor 4
```

- `--model`：`apollo` | `aion`
- `--factor`：慢放倍数 `2 3 4 6 8 10 12 16`
- `--sensitivity 10`：重复帧替换灵敏度 0-100；`rdt = sensitivity/1000`，0 关闭
- `--rdt -1`：原始 rdt 覆盖；-1 表示使用 `--sensitivity` 的映射
- `--instances 0`：插值并发会话数；0=自动（默认 1 个共享会话）
- `--workers 0`：插值并行 worker 数；0=自动（默认 2，空闲显存 <12GB 时降为 1）。多个 worker 共享同一会话，显存不翻倍
- `--gpu-vram 0`：Aion tile 预算 GB；0=约 2.4M 像素的整宽条带，显式值越小 tile 越小。增大 tile 时可能需要同时提高 `--vram`。
- `--vram 0`：每会话显存上限 GB；0=按 tile 面积自动（4K 约 3.2GB）

慢放输出保持源 FPS（30fps 输入仍是 30fps），帧数和时长都是 N 倍；音频会按同一倍率减速。`--audio-codec copy` 在慢放时会自动改为 AAC 重编码，`none` 可关闭音频。

重复帧替换的行为与 Topaz `tvai_fi` 一致：相邻源帧差异低于 `rdt` 时，不复制该重复帧，
而是在前后两个非重复帧之间生成新的插值帧替换它；输出帧数/FPS 保持不变。
Apollo 与 Aion 都支持该行为。

### 输出设置（两个命令通用）

```bash
--encoder h265-main-win-nvidia   # 编码器（见 tvai encoders）
--quality High                   # 质量档 High/Mid/Low（CQ 模式）
--qp 17                          # 直接指定 constqp 值（优先于 --quality）
--rate-control cq                # cq / cbr / vbr
--bitrate 20                     # cbr/vbr 码率 (Mbps)
--audio-codec copy               # copy / aac / ac3 / pcm / vorbis / none（慢放时 copy 自动转 AAC 并减速）
--audio-bitrate 320              # 音频码率 kbps（aac/ac3）
```

默认编码器 `h265-main-win-nvidia`、质量 `High`、码率模式 `cq`、音频 `copy`（直接复制原音轨；插帧慢放时会自动改为 AAC 并减速）。

### 其他

```bash
--gpu -2          # -2 自动 / -1 CPU / 0+ 设备号
--threads 4       # 推理线程数
--instances 0     # 插值并发会话数（0=自动，默认 1）
--workers 0       # 插值并行 worker 数（0=自动，默认 2，共享会话）
--sensitivity 10  # 重复帧替换灵敏度 0-100（rdt=sensitivity/1000，0 关闭）
--rdt -1          # 原始 rdt 覆盖（-1=使用 --sensitivity）
--gpu-vram 0      # Aion tile 预算 GB（0=约 2.4M 像素；越小越省显存）
--vram 0          # 每会话显存上限 GB（0=按 tile 自动；Topaz 的 vram 是显存比例 0.1-1）
--overlap 0       # 分块重叠，0=自动（增强/Apollo 48，Aion 32）；Aion 接缝明显时可调大
--kcolor 1        # keepColor 色彩校正 0/1
--scene-threshold 6.5  # 场景切换阈值
--start 0 --end 99  # 只处理指定帧范围
--workdir <dir>   # 临时 ONNX 解包目录
```

## 测试

```bash
go test ./...
```

端到端回归默认跳过，需要模型目录和 onnxruntime：

```bash
TVAI_E2E=1 go test ./internal/pipeline -run E2E -v
TVAI_E2E=1 TVAI_E2E_GPU=1 TVAI_E2E_AION=1 go test ./internal/pipeline -run TestE2EAionDuplicate -v
```

- `TVAI_E2E=1` 运行 Proteus 帧范围/分块和 Apollo 重复帧替换
- `TVAI_E2E_GPU=1` 使用 GPU 0，否则走 CPU
- `TVAI_E2E_AION=1` 额外运行 Aion 重复帧替换，需要 GPU
- 缺少 ffmpeg/ffprobe、模型或 ORT 时测试自动 skip

## 性能（4K 8x）

测试片：3840×2160 / 30fps / 377 帧，Aion 8x，Sensitivity 65，输出 3009 帧（30fps，约 100 秒）。

| 实现 | 耗时 | GPU 占用 | 显存峰值（整卡） |
|---|---|---|---|
| 优化前（每子帧重跑推理，1 会话） | 约 3.7 小时 | 约 40% | — |
| 当前（单区间一次推理，1 共享会话 + 2 worker） | **438s（7.3 分钟）** | **约 77%** | **约 10.2GB** |
| Topaz 同片同设置 | 约 14 分钟 | 85-90% | — |

Sensitivity 0（不替换重复帧）全片实测 1110s（18.5 分钟），GPU 约 92%，显存约 11.9GB；Sensitivity 65 为 438s（7.3 分钟）。持续满载时笔记本 GPU 会降频，短片段折算会偏乐观。
中间尝试过多会话方案（3 会话峰值约 22GB），最终改为 1 共享会话 + 2 worker，显存降到约 10GB。
`--instances 2` 可再快约 5%，但显存接近翻倍；`--workers 1 --vram 2` 为低显存模式，4K 峰值约 7.8GB，速度约慢 5-10%。

## 环境变量

| 变量 | 说明 |
|---|---|
| `TVAI_MODEL_DIR` | `.tz` 模型目录（本工具语义；Topaz 的 `TVAI_MODEL_DIR` 是元数据 JSON 目录） |
| `TVAI_ORT_DLL` | onnxruntime.dll 路径 |
| `TVAI_DEBUG=1` | 输出诊断信息 |

## 实现要点

- **纯 Go 调用 onnxruntime C API**：通过 `syscall` 直接调用 DLL 导出的 `OrtGetApiBase`，50 个 `OrtApi` 结构体偏移由官方 `onnxruntime_c_api.h`（ORT_API_VERSION 31）解析生成，见 `internal/ort/offsets.go`。
- **`.tz` 解包**：`.tz` 是 ZipCrypto 加密的 ZIP，内含标准 ONNX。密码硬编码于 `aiengine.dll`，见 `internal/tz`。
- **CUDA fp16 ReduceMean 溢出修复**：onnxruntime CUDA 上 fp16 大张量归约会累加溢出（超过 65504）导致 NaN。`internal/onnxpatch` 在模型加载时对每个输出为 fp16 的 ReduceMean 节点插入 `Cast(fp32) → ReduceMean → Cast(fp16)`，改用 fp32 累加。protobuf 级字段解析/重写，仅改节点列表其余原样保留。
- **Aion 两级插帧**：Aion 需要 `in8x`（8 帧预测，尺寸为 tile/4）。先用底部模型 `aiob` 从 4 帧产出 8 帧，块平均降采样到 tile/4 后喂给 Aion。`in8x` 为零时输出亮度仅 71%，两级后 100%。
- **插帧单次推理产出全部子帧**：每个源帧区间只跑一次 `aiob + aion`，把 16 个子帧一次性取回；按当前慢放倍数需要的子帧索引融合，不再为每个输出帧重跑推理。
- **Aion 条带切块**：动态模型优先整帧单块；超过约 2.4M 像素时用整宽（或整高）条带，4K 为 3840×576 ×4 块，重叠浪费约 7%。`--gpu-vram` 控制 tile 像素预算，值越小 tile 越小、显存越低。
- **共享会话 + 多 worker**：默认 1 个主会话 + 1 个底部会话，2 个 worker 共享它们并发调用 `Run`。模型权重和 CUDA arena 只保留一份，显存约为多会话方案的一半；`--instances` 可显式增加会话数。
- **CUDA EP 调优**：启用 `prefer_nhwc=1`（模型为 NHWC）和 `use_tf32=1`；推理输出在图层面改为 fp16，省一半输出带宽和显存，CPU 侧按 fp16 直接融合。
- **按 tile 自动限显存**：每会话 `gpu_mem_limit` 按 tile 像素面积估算（4K 条带约 3.2GB），避免 ORT arena 无界增长；`--vram` 可覆盖。
- **直接融合**：推理输出不拷贝成 `Frame`，而是按子帧通道直接从 ORT 张量视图加权累加进画布；融合权重和归一化倒数图在任务开始时预计算，归一化合入 RGB24 转换。
- **三阶段流水线**：解码在独立 goroutine 中预读，编码在独立 goroutine 中完成 RGB24 转换和 ffmpeg 写入；通过有界 channel 与推理并行。
- **keepColor**：当模型 `keepColor=1` 且 `--kcolor 1` 时，在输出帧上做 Lab 均值/方差匹配到输入帧（Reinhard 色彩迁移）；Proteus/Gaia 启用。
- **重复帧替换**：`--sensitivity` 映射到 `rdt`；相邻重复帧不复制，而是在前后非重复帧之间生成插值帧替换，保持输出帧数/FPS。
- **固定块模型选块**：Proteus/Gaia/Apollo 按 Topaz 的评分选块：
  `n = ceil((W-o)/(Wc-o)) * ceil((H-o)/(Hc-o))`，
  `score = n*0.05 + (n*Wc*Hc - W*H)/(W*H)`，取最小；允许候选块大于输入帧。
  增强/Apollo 默认重叠 48，Aion 32。
- **场景切换**：`scene_change_threshold` 传入 6.5；模型内部用 fnet 光流幅度做软混合，外部不再按像素差重置 `hr_prev`。

## 已知限制

- 插帧模型在部分 Blackwell GPU 上曾有 CUDA 兼容问题（fp16 ReduceMean 溢出），已通过 ONNX 补丁解决；若仍异常可 `--gpu -1` 退回 CPU（正确但慢）。
- **Aion 的 tile 形状**：为降低重叠开销，Aion 使用整宽/整高条带而非方形块。若在条带接缝处看到明显差异，可增大 `--overlap`（默认 32）；代价是每块像素变多。
- **显存与速度的取舍**：默认 1 共享会话 + 2 worker 已接近 Topaz 速度；`--instances 2` 约再快 5%，但显存接近翻倍。`--vram` 设得过低会因单次分配失败而报错，4K 双 worker 建议不低于 3GB，单 worker 约 2GB 可运行。
- 重复帧替换的输出帧数/FPS 保持不变；Topaz 的实现在部分边界会少 1-3 帧，本工具不模拟该细节。
- keepColor 是 Lab 均值/方差匹配的近似实现，与 Topaz 的像素差异约在 1 个 8bit 色阶量级。
- 非 2 幂倍数（3/6/10/12）的时间位置为最近子帧近似。
- 当前管线全程 RGB24，10bit 编码器实际从 8bit 转出；YUV/10bit 端到端尚未实现。
- Topaz 在 RTX 5090（cc 12.0）上因 TensorRT 后端不支持 Blackwell 而回退 onnxruntime，其速度来自软件优化（批处理/流/去重）而非 TRT，见调查文档。
