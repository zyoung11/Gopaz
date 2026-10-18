# Gopaz 架构与实现

> 本文汇总 Topaz Video AI 的逆向调查、知识核验与当前实现，替代早期的《TOPAZ逆向复刻-调查与方案》和《TOPAZ逆向核验报告》。文中数据来自静态分析、本机运行日志、ONNX 图检查和端到端实测。

## 1. 结论

Topaz Video AI 的 GUI 只负责拼 ffmpeg 命令行，推理由 `avfilter-10.dll` 里的 `tvai_up` / `tvai_fi` 等自定义滤镜完成，滤镜再调用 `aiengine.dll` 加载 ONNX 模型。

复刻不需要重写网络结构，只需要一条与 Topaz 等价的流水线：

```
解码 → 选块 → 分块推理 → 融合/色彩校正 → 编码 → 音轨并回
```

Gopaz 用 Go 直接调用 onnxruntime C API，实现了其中 5 个模型：Proteus、Gaia、Gaia-CG、Apollo、Aion。

## 2. 逆向结论

### 2.1 `.tz` 模型格式

- Topaz 模型目录下 622 个 `.tz` 全部是标准 ZIP，每个包恰好一个条目，压缩方法为 deflate，通用标志的 bit0 为 1，即 ZipCrypto 加密，没有 AES。
- 解包密码硬编码在 `aiengine.dll`，已在 `tz.go` 中使用。
- 解包后是标准 ONNX，可以直接交给 onnxruntime。
- 模型按 `短名-版本` 分家族，文件名形如 `<prefix>-fp16-<H>x<W>-<scale>x-ox.tz`。注意 JSON 的 `blocks` 是 `[W, H]`，文件名是 `HxW`。
- 增强模型每个家族有 9 个 block × 3 个 scale；Apollo 有 12 个块；Aion 和 aiob 是单文件动态尺寸。

### 2.2 支持的模型

| 模型 | 短名 | 类型 | 输入时序 | 输出 | 精度 | keepColor |
|---|---|---|---|---|---|---|
| Proteus | prob-v4 | 增强 | 2 帧（当前 + 前一） | 1 帧 | fp16 | 是 |
| Gaia | ghq-v5 | 增强 | 5 帧 | 1 帧 | fp16 | 是 |
| Gaia-CG | gcg-v5 | 增强 | 5 帧 | 1 帧 | fp16 | 是 |
| Apollo | apo-v8 | 插帧 | 4 帧 | 8 子帧（24 通道） | fp16 | 否 |
| Aion | aion-v1 | 插帧 | 4 帧 + in8x | 16 子帧（48 通道） | 图输入 fp32 | 否 |
| Aion 底部 | aiob-v1 | 插帧 | 4 帧 | 8 子帧（24 通道） | 图输入 fp32 | 否 |

Proteus 的输入为 `lr_prev`、`lr_curr`、`params[1,6,1,1]`、`hr_prev`、`scene_change_threshold`。Gaia / Gaia-CG 的输入为 `VideoSR_Unet/inputFrame0..4:0`。Apollo 为 `in0..in3`。Aion 为 `in0..in3` 加 `in8x[1,H/4,W/4,24]`。

Topaz 还有 Nyx、Iris、Rhea、Theia 等模型，本工具不实现，只保留上述 5 个。

### 2.3 参数系统

Proteus 的 6 个参数按固定顺序打包进 `params`：

| 张量下标 | CLI | GUI 名 |
|---|---|---|
| 0 | deblur | Antialias/DeBlur |
| 1 | denoise | Reduce Noise |
| 2 | detail | Recover Details |
| 3 | dehalo | Dehalo |
| 4 | sharpen | Sharpen |
| 5 | fix-compression | Fix Compression |

GUI 滑杆为 -100..100，模型空间为 -1..1，换算为 `值 / 100`。GUI 默认值是 0，模型 JSON 里的 `default: 0.5` 只是元数据基准，不是实际默认。六个参数都允许负值，Gopaz 统一 clamp 到 `[-1, 1]`。

### 2.4 分块与选块

默认分块重叠：增强模型和 Apollo 为 48，Aion 为 32。模型 JSON 里的 `blockOverlap` 可覆盖。

固定块模型的选块使用 Topaz 的评分：

```
nx = ceil((W - o) / (Wc - o))
ny = ceil((H - o) / (Hc - o))
n  = nx * ny
score = n * 0.05 + (n * Wc * Hc - W * H) / (W * H)
```

取 score 最小的候选。`Wc <= o` 或 `Hc <= o` 的候选跳过。候选块允许大于输入帧，超出部分用边缘填充。

实测选块结果：Proteus 720p 选 672×384，1080p/4K 选 672×576；Apollo 1080p 选 1056×1088，4K 选 1344×1152。

### 2.5 场景切换

Topaz 传给模型 `scene_change_threshold = 6.5`。模型内部用 fnet 光流幅度的均值乘 32 与阈值做 `Greater`，得到 mask 后对 `hr_prev` 的 warp 结果和当前帧做软混合。

Gopaz 只传阈值，不在外部按像素差重置时序状态。

### 2.6 重复帧与 Sensitivity

Topaz 的 `rdt` 不是整帧平均差，而是“显著变化像素的比例”阈值。显著变化的判定约为 10 个 8bit 色阶，即 `max(|dR|,|dG|,|dB|) > 10/255`。

```
rdt = sensitivity / 1000
```

Sensitivity 默认 10，对应 `rdt = 0.01`；日志里的 0.065 对应滑杆 65。低于阈值时视为重复帧，在前后非重复帧之间生成插值帧替换，输出帧数/FPS 不变。

### 2.7 keepColor

模型 JSON 的 `keepColor` 决定是否需要色彩校正。运行验证表明，`kcolor=1` 时对输出帧做逐帧、全局的 Lab 均值/方差匹配，把输出统计量对齐到输入帧，属于 Reinhard 色彩迁移。`keepColor=0` 的模型即使传 `kcolor=1` 也没有任何像素变化。

Gopaz 对 Proteus / Gaia / Gaia-CG 实现 Lab 均值/方差匹配，与 Topaz 的差异约 1 个 8bit 色阶。

### 2.8 编码与音频

Topaz 用 NVENC 编码，`cqpValues` 是数组（例如 `High: [17]`），cbr/vbr 走不同模板。输出完成后还有第二趟 ffmpeg，把原视频音轨并回。

Gopaz 注册了 11 个常用编码器：H.264 / H.265 / H.265 Main10 / AV1 / AV1 10bit / ProRes 422 HQ、422、422 LT / FFV1 10bit 4:4:4、4:2:0 / VP9。音轨支持 copy、AAC、AC3、PCM、Vorbis 和关闭。

### 2.9 参数语义差异

Topaz 与 Gopaz 的 CLI 语义不同，不能直接对照：

| 参数 | Topaz | Gopaz |
|---|---|---|
| `vram` | 显存使用比例 0.1-1，默认 1.0 | 每会话显存上限 GB，0=自动 |
| `instances` | 额外实例数，默认 1（共 2 个会话） | 并发会话数，0=自动（默认 1 个共享会话） |
| `workers` | 不存在 | 并行 worker 数，默认 2 |
| `sensitivity` | GUI 滑杆 0-100 | 同义，映射为 rdt |
| `rdt` | 原始阈值 -0.01..0.2 | 同义，-1 表示使用 sensitivity |

## 3. 实现

### 3.1 源文件组织

全部 Go 文件位于 `gopaz/` 根目录，同属 `package main`，按职责分文件：

| 文件 | 职责 |
|---|---|
| `main.go` | CLI 参数解析、依赖路径探测 |
| `ort.go` `offsets.go` | 纯 Go 的 onnxruntime C API 绑定，50 个 `OrtApi` 偏移由官方头文件生成 |
| `tz.go` | ZipCrypto 解密与 deflate 解压 |
| `onnxpatch.go` | protobuf 级 ONNX 修改：fp16 ReduceMean 补丁、输出 fp16 化 |
| `model.go` | 模型元数据：文件名、tile 表、参数、时序帧数、keepColor |
| `runner.go` `upscale.go` `interpolate.go` `interp_worker.go` | 切块、推理、融合、插帧、色彩校正 |
| `audio.go` `stream.go` `frame.go` `tiling.go` `enhance_plan.go` `parallel.go` `f16.go` `gpu.go` `range.go` `color.go` `unsafe.go` | 管线辅助逻辑 |
| `encode.go` | 编码器注册表与 ffmpeg 参数构造 |
| `video.go` | ffmpeg / ffprobe 子进程封装 |
| `log.go` | 终端输出格式 |

### 3.2 数据流

```
ffmpeg 解码 ──> 帧读取 goroutine ──> 有界 channel
                                        │
                                        ▼
                              分块 / 多 worker 推理
                                        │
                                        ▼
                             融合 + keepColor（可选）
                                        │
                                        ▼
                        帧写入 goroutine ──> RGB24 ──> ffmpeg 编码
                                        │
                                        ▼
                              第二趟 ffmpeg 并回音轨
```

解码、推理、编码三个阶段通过有界 channel 重叠进行，避免 GPU 在等 CPU 时闲置。

### 3.3 会话与 worker

- 默认 1 个主会话 + 1 个底部会话，2 个 worker 共享；模型权重和 CUDA arena 只保留一份。
- `--instances` 可显式增加会话数；插帧时每个主会话配对同下标的底部会话。
- `--workers` 控制并行 worker 数，默认 2；空闲显存低于 12GB 时自动降为 1。
- 增强模型的 `hr_prev` 按帧串行反馈，tile 之间并行；Proteus 的时序反馈不会因并行而错乱。

### 3.4 Aion 两级插帧

Aion 的 `in8x` 是 8 帧预测，形状 `[1, H/4, W/4, 24]`，由 `unshuffle_lr` 送入图内。喂零也能得到结构正确的插值，但亮度只有 71%。

正确做法是两级：

```
aiob(4 帧) → 8 帧全分辨率 → 块平均降采样到 tile/4 → in8x
           → aion 精化为 16 帧
```

Aion 和 aiob 要求输入尺寸被 32 整除。

### 3.5 fp16 ReduceMean 溢出修复

Apollo / Aion / aiob 在 CUDA 上会输出全 NaN，根因是 fp16 `ReduceMean` 对大张量归约时累加器溢出：求和超过 65504 变成 Inf，再变 NaN。Apollo 有 40 个 ReduceMean，aiob 40 个，Aion 54 个，而 Proteus 只有 1 个，这解释了为什么超分正常、插帧全挂。

`onnxpatch.go` 在加载时对每个输出为 fp16 的 ReduceMean 插入：

```
Cast(fp32) → ReduceMean → Cast(fp16)
```

只按输出类型修补，因为 Aion 是混合精度，全局强转 fp16 会破坏 fp32 分支。

修复后插值从 CPU 6 分 9 秒降到 CUDA 33 秒。

### 3.6 插帧融合

每个源帧区间只跑一次 `aiob + aion`，一次取回全部 16 个子帧，再按慢放倍数需要的索引融合。推理输出不拷贝成 `Frame`，而是按子帧通道直接从 ORT 张量视图加权累加进画布。

融合权重和归一化倒数图在任务开始时预计算，归一化并入 RGB24 转换。Apollo 的 8 子帧和 Aion 的 16 子帧共用同一套映射。

### 3.7 Aion 条带切块

Aion 优先整帧单块；超过约 2.4M 像素时改用整宽或整高条带。4K 使用 3840×576 × 4 块，重叠浪费从方形小块的约 18% 降到约 7%。

`--gpu-vram` 控制 tile 像素预算，值越小 tile 越小、显存越低；`--overlap` 可增大以缓解接缝。

### 3.8 显存策略

- 每会话的 `gpu_mem_limit` 按 tile 像素面积估算，插帧 4K 约 3.2GB，增强模型按输出 tile 面积设 4-12GB。
- Aion tile 预算默认约 2.4M 像素，可显式提高；提高 tile 时可能需要同时提高 `--vram`。
- `--vram` 设得过低会因单次分配失败而报错。4K 双 worker 建议不低于 3GB，单 worker 约 2GB 可运行。

### 3.9 CUDA EP 调优

- `prefer_nhwc=1`：模型为 NHWC。
- `use_tf32=1`：插帧模型启用 TF32。
- 插帧输出在图层面改为 fp16，输出带宽和显存减半。
- 推理输出按 fp16 直接融合，CPU 侧不做多余转换。

## 4. 质量对比

PSNR 条件：720p 2x、30 帧、默认参数、kcolor=1，双方 FFV1 输出逐帧比较。

| 模型 | 与 Topaz 的 PSNR |
|---|---|
| Proteus | 46.9 dB |
| Gaia | 43.7 dB |
| Gaia-CG | 45.5 dB |

Apollo 1080p 2x、30 帧、rdt 关闭：tile 1056×1088、59 帧/30fps/1.966s 均与 Topaz 一致，平均 PSNR 42.6dB，最低 36.7dB。最低值出现在高运动插值帧。16x 下双方都复用 8 个子帧，帧数和时长一致，平均 PSNR 38.3dB。

## 5. 性能

### 5.1 Aion 4K 8x

测试片：3840×2160 / 30fps / 377 帧，Aion 8x，Sensitivity 65，输出 3009 帧 / 30fps / 约 100 秒。

| 实现 | 耗时 | GPU 占用 | 显存峰值 |
|---|---|---|---|
| 优化前（每子帧重跑推理，1 会话） | 约 3.7-3.8 小时 | 约 40% | 未测 |
| 当前（单区间一次推理，1 共享会话 + 2 worker） | **1047.7s（17.5 分钟）** | **约 91%** | **约 10.5GB** |
| Topaz 同片同设置 | 约 14 分钟 | 85-90% | 未测 |

Sensitivity 0 全片实测 1110s，GPU 约 92%，显存约 11.9GB。测试片本身没有判定出重复帧，所以两者耗时接近；重复帧多的素材会更快。

中间尝试过 3 会话，显存峰值约 22GB，最终改为 1 共享会话 + 2 worker。`--instances 2` 可再快约 5%，显存接近翻倍；`--workers 1 --vram 2` 的 4K 峰值约 7.8GB，速度慢 5-10%。

### 5.2 增强模型

短片段推理耗时，含模型加载和启动：

| 模型 | 720p 2x 30 帧 | 1080p 2x 30 帧 |
|---|---|---|
| Proteus | 22s → **10s** | 24s（1 worker）→ **17s**（2 worker） |
| Gaia | 24s → **13s** | **22s** |

并行融合与旧串行路径的 720p 输出 PSNR 约 51dB，差异来自浮点累加顺序。

## 6. 已知限制

- **10bit / YUV**：当前管线全程 RGB24，10bit 编码器实际从 8bit 转出，YUV / 10bit 端到端尚未实现。
- **Aion 接缝**：条带切块的重叠小于方形块，若在接缝处看到差异可增大 `--overlap`。
- **非 2 幂倍数**：3/6/10/12 的时间位置是最近子帧近似；Apollo 的 10/12/16 倍会复用 8 个子帧，Topaz 同样会重复。
- **重复帧边界**：Gopaz 保持输出帧数/FPS 不变；Topaz 在部分边界会少 1-3 帧，本工具不模拟。
- **keepColor**：Lab 均值/方差匹配是近似实现，与 Topaz 的像素差异约 1 个 8bit 色阶。
- **参数估计**：Topaz 的 `tvai_pe` 自动参数估计尚未集成。
- **CUDA 兼容**：插帧模型在部分 Blackwell GPU 上曾有 fp16 ReduceMean 溢出，已通过 ONNX 补丁解决；若仍异常可用 `--gpu -1` 退回 CPU。

## 7. TODO

- [ ] Apollo 10/12/16 子帧复用策略优化
- [ ] 增强模型 4K/4x 长片段基准
- [ ] 自动参数估计 `tvai_pe` 集成
- [ ] 重复帧替换与 Topaz 的像素级对比
- [ ] 10bit/YUV 色彩管线与 keepColor 精确实现
- [ ] 子帧顺序与时间戳验证
- [ ] 评估 TensorRT 后端可行性

## 8. 逆向方法与产物

| 证据 | 位置 |
|---|---|
| AVFilter / AVClass / AVOption 结构解析 | `work/avopt3.py` |
| 5 个 `tvai_*` 滤镜的完整选项表 | `work/tvai_avoptions.txt` |
| PE + capstone 交叉引用与反汇编 | `work/xref.py` |
| 分块、选块、重叠函数反汇编 | `work/tile_funcs.txt`、`work/score_raw.txt`、`work/scoring.txt` |
| exe 内嵌 QML/JS 解压结果 | `work/app_res/` |
| 68 个模型 JSON + 4 个非模型 JSON | `work/modelmeta/` |
| 真实 ffmpeg 命令行与引擎日志 | Topaz 的 `Data/Roaming/.../logs/*.tzlog` |

独立运行 Topaz 滤镜时的环境变量命名与实际含义相反：

```
TVAI_MODEL_DIR      = ...\App\Topaz Labs LLC\Topaz Video AI\models   # 元数据 JSON + auth.tpz
TVAI_MODEL_DATA_DIR = ...\TopazVideoAI\models                        # .tz 模型文件
```

执行 `-filter_complex "tvai_up=..."` 的 `ffmpeg.exe` 来自 Topaz 自带构建，`--enable-tvai` 是定制编译开关。原始 Topaz 目录只读访问，未做修改。

README 中的 `GOPAZ_*` 是本工具自己的环境变量，与 Topaz 的 `TVAI_*` 无关。
