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
| `serve.go` `jobs.go` | 本地 Web 服务、任务管理与进度 |
| `picker_windows.go` `picker_other.go` | Windows 原生文件与文件夹选择器 |
| `proc_windows.go` `proc_other.go` | 子进程树终止 |
| `web/` | 纯 HTML/CSS/JS 前端，编译时嵌入 |

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

### 3.10 Web 界面

`web/` 下的 `index.html`、`style.css`、`app.js` 和 Inter 字体通过 `//go:embed web` 嵌入可执行文件，不需要 Node 或打包步骤。

- `gopaz` 不带参数启动服务，默认监听 `127.0.0.1:8765`；端口占用时顺延 10 个，随后自动打开浏览器。`gopaz serve` 可显式启动，`--port` 和 `--no-open` 控制端口与浏览器。
- 服务只绑定回环地址。前端通过 `/api/config`、`/api/probe`、`/api/preview-output`、`/api/jobs`、`/api/media` 和 `/api/reveal` 与后端交互。`/api/media` 使用白名单，只允许探测过的输入和生成的预览文件，支持 Range 请求。
- 输入视频通过 `GO-WinFilePicker` 的原生对话框或路径输入框导入，只传绝对路径，不复制文件。浏览器拖拽无法获得绝对路径，因此不提供拖拽导入。
- 输出只要求文件夹路径。文件名按 Topaz 规则生成：增强模型用 `短名+版本`，如 `prob4`、`ghq5`、`gcg5`；插帧用 `tvai`；冲突时插入 `_1_`、`_2_`。文件名输入框也可以改成自定义名字，留空恢复自动命名；非法字符会被拒绝并在输入框下方显示错误。
- 任务通过子进程调用同一个可执行文件的 `upscale` / `interpolate`，环境变量 `GOPAZ_PROGRESS=json` 让日志和进度以 JSON 行输出。服务端解析这些事件，按进度事件的时间差计算处理速度和剩余时间，通过轮询 API 暴露给前端。
- 同一时间只允许一个任务。取消时使用 `taskkill /T /F` 终止子进程树，连同 ffmpeg 子进程一起结束。
- 预览使用带 `preview` 标记的任务：服务端把当前播放位置换算成帧，取 2 秒，输出到临时目录，并强制 H.264 MP4 和无音频，保证浏览器可播放；预览文件同时加入媒体白名单。
- 播放器用完整输入视频作为原画，用预览片段作为处理后画面。Original 模式使用完整时间轴；Processed、左右对比和分割对比使用预览片段时间轴。插帧预览时原画按倍率反向设置 `playbackRate`，两侧对应同一个源区间。分割对比由两条叠加的 video 和 `clip-path` 实现，拖动分隔线只改变裁剪比例。
- 前端默认英文，右上角可切换中文；只提供暗色主题。日志由 CLI 以英文输出，不受界面语言影响。
- 前端主题取自 `WonderStyle.qml` 的暗色 token：`#171717` 页面、`#212121` 面板、`#A6FF00` 主色、Inter 字体、4px 圆角。

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
- **Web 播放与对比**：Web 界面暂不提供视频播放、前后对比和实时预览。
- **Web 任务并发**：Web 界面一次只运行一个任务，没有任务队列。
- **拖拽导入**：浏览器无法提供本地绝对路径，Web 界面不提供拖拽，需使用原生选择器或路径输入。
- **原生选择器**：依赖 Windows COM，仅在 Windows 上可用。
- **Web 播放编码**：输入视频需要浏览器支持其编码；预览强制 H.264 MP4 且无音频，不代表最终输出编码。
- **Web 预览范围**：Preview 只处理当前播放位置起 2 秒；预览片段留在系统临时目录，不自动清理。

## 7. TODO

- [ ] Apollo 10/12/16 子帧复用策略优化
- [ ] 增强模型 4K/4x 长片段基准
- [ ] 自动参数估计 `tvai_pe` 集成
- [ ] 重复帧替换与 Topaz 的像素级对比
- [ ] 10bit/YUV 色彩管线与 keepColor 精确实现
- [ ] 子帧顺序与时间戳验证
- [ ] 评估 TensorRT 后端可行性
- [ ] Web 视频播放与前后对比
- [ ] Web 任务队列与多任务并发
- [ ] Web 输出缩略图与处理前后预览

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
