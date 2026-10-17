# Topaz Video AI 逆向知识核验报告

> 日期：2026-10-17 ｜ 对象：`TOPAZ逆向复刻-调查与方案.md`
> 方法：只读静态分析 + 本机已有运行日志 + 一次授权的 `ffmpeg.exe` 探针（仅 1 帧/30 帧测试片）。
> 目的：先验证既有结论，再补充此前未知的信息。

---

## 0. 结论摘要

原有文档的**主干是正确的**：`.tz` 是 ZipCrypto ZIP、密码有效、内含标准 ONNX；
`tvai_up` 通过 6 维 `params` 张量控制模型；分块推理；Aion 是两级结构；
GUI 拼 ffmpeg 命令行。这些都用独立证据复核通过了。

但细节层有**大量不准确**，主要集中在四处：

1. **滤镜选项表**：旧表的选项名混入了帮助文本/单位，且漏项、错项（如 `sharpen`、`denoise`、`threads`、`rsc`、`fullframe` 并不存在）。
2. **模型能力表**：Nyx / Nyx Fast / Iris / Rhea / Theia 的 6 参数、Thd/Thf 的子集参数、各模型的真实输入输出与精度，旧表缺得多。
3. **参数默认值与语义**：GUI 默认是 0 而不是 0.5；`vram` 是显存比例不是 GB；`instances` 是"额外实例数"不是总会话数。
4. **分块与场景阈值**：默认重叠是 **48**（Aion 32）不是 16；场景切换阈值是 **6.5** 不是 0.04；Topaz 的选块是一个带 penalty 的评分，不是简单"最大可放下"。

此外，这次从 `Topaz Video AI.exe` 的 Qt 资源里**完整提取出了 GUI 的 QML/JS 源码**，
参数归一化、`rdt` 映射、`estimate=8`、Aion 强制 `instances=0` 等都能直接读到源码，不再需要猜测。

---

## 1. 方法与证据

| 证据 | 位置 / 工具 |
|---|---|
| PE 节表、AVFilter / AVClass / AVOption 结构解析 | `work/avopt3.py`（旧 `avopt.py`/`avopt2.py` 的 AVClass option 偏移与 AVOption 步长都算错了） |
| x86-64 反汇编与字符串交叉引用 | `work/xref.py`（capstone + `.pdata` 函数边界） |
| videoai.dll 分块/阈值函数原文 | `work/tile_funcs.txt`、`work/score_raw.txt`、`work/scoring.txt` |
| exe 内嵌 QML/JS（zlib 解压） | `work/app_res/`（123 个资源文件，含 EnhancementFilter.qml 等） |
| exe 明文字符串区 | `work/app_text.txt` |
| 模型元数据 | `work/modelmeta/`（68 个模型 JSON + 4 个非模型 JSON） |
| 真实 ffmpeg 命令行与引擎日志 | `Data/Roaming/.../logs/2026-10-17-19-02-7-Main.tzlog` |
| ONNX 图结构 | `work/prob_576x672_2x.onnx` 等，onnx 1.19 解析 |
| `.tz` 全量清单 | 622 个文件逐个读中央目录 + 抽样解包 |
| 运行探针 | Topaz 自带 `ffmpeg.exe` + 自造 30 帧测试片；产物已清理，结论见 §2.12–§2.14 |

---

## 2. 逐项核验

### 2.1 `.tz` 格式与密码：✅ 完全确认，且范围更大

对 `C:\zy\APP\Big_APPs\TopazVideoAI\models` 下 **622 个 `.tz`** 全量扫描：

- 全部是标准 ZIP，**每个包恰好 1 个条目**；
- 压缩方法全部为 deflate（8）；
- 通用标志只有 `0x9`、`0x3`、`0x7` 三种，**bit0 全为 1**（ZipCrypto 加密），无 AES（bit6）；
- 按 35 个（短名, 版本）家族各抽 1 个用密码解包，**35/35 成功**，内容均以 ONNX protobuf 的 `0x08` 开头。

旧结论正确，且"622 个包只有一个 ONNX、全部 ZipCrypto"现在有全量证据。

### 2.2 滤镜选项：❌ 旧表需要重写

`avopt3.py` 现在能正确解析：AVFilter 48 字节（`name/desc/inputs/outputs/priv_class/flags`），
AVClass 的 `option` 指针在 +16，AVOption 步长 **64 字节**，
类型枚举从 **1** 开始（1=FLAGS, 2=INT, 3=INT64, 4=DOUBLE, 5=FLOAT, 6=STRING, 7=RATIONAL, 16=DURATION…）。
完整表已保存到 `work/tvai_avoptions.txt`。

`tvai_up` 实际 20 个选项（顺序即私有结构体偏移）：

| 选项 | 类型 | 默认 | 范围 | 说明 |
|---|---|---|---|---|
| `model` | STRING | `amq-13` | — | 模型短名（GUI 总会覆盖） |
| `scale` | INT | 1 | 0–4 | 0 = 按 w/h 自动 |
| `w` / `h` | INT | 0 | 0–100000 | 输出尺寸，用于自动 scale |
| `device` | INT | -2 | -2–8 | 自动/CPU/GPU |
| `instances` | INT | 0 | 0–3 | **额外**模型实例数 |
| `download` | INT | 1 | 0–1 | 允许下载模型 |
| `vram` | DOUBLE | 1.0 | 0.1–1 | **显存使用比例**，不是 GB |
| `estimate` | INT | 0 | 0–1e6 | 自动参数估计帧间隔；GUI 传 8 |
| `preblur` | DOUBLE | 0 | -1–1 | Anti-Alias/Deblur |
| `noise` | DOUBLE | 0 | -1–1 | Reduce Noise |
| `details` | DOUBLE | 0 | -1–1 | Recover Details |
| `halo` | DOUBLE | 0 | -1–1 | Dehalo |
| `blur` | DOUBLE | 0 | -1–1 | **Sharpen**（名字叫 blur） |
| `compression` | DOUBLE | 0 | -1–1 | Fix Compression |
| `prenoise` | DOUBLE | 0 | 0–0.1 | 输入加噪 |
| `grain` | DOUBLE | 0 | 0–0.1 | 输出加颗粒 |
| `gsize` | DOUBLE | 0 | 0–5 | 颗粒尺寸 |
| `kcolor` | INT | 1 | 0–1 | "Run extra color correction if required by model" |
| `blend` | DOUBLE | 0 | 0–1 | 与原帧混合 |

`tvai_fi`：`model`（默认 `chr-1`）、`device`、`instances`、`download`、`vram`、
`slowmo`（默认 1，0.1–16）、`rdt`（默认 **0.01**，-0.01–0.2）、`fps`（DURATION）。
**没有 `threads`**。

`tvai_stb`：`model`（默认 `ref-2`）、`device`、`instances`、`download`、`vram`、
`full`（不是 `fullframe`）、`filename`（默认 `cpe.json`）、`rst`、`wst`、`postFlight`、
`ws`（默认 64，0–512）、`csx`/`csy`（默认 2，1–8）、`smoothness`（默认 6，0–16）、
`cache`（默认 128，0–256）、`dof`（默认 1111）、`roll`（不是 `rsc`）、`reduce`（0–5）。

`tvai_pe`：**只有** `model`（默认 `prap-3`）和 `download`；没有 output/device。
`tvai_cpe`：`model`（默认 `cpe-1`）、`filename`（`cpe.json`）、`device`、`download`。

> 旧文档 §1.3 的 `sharpen, denoise, threads, rsc, fullframe, output` 均不存在。

### 2.3 GUI 参数归一化：✅ 已从 QML 源码直接确认

`work/app_res/047_1d9922.txt`（= `EnhancementFilter.qml`）第 267–273 行：

```js
preblur     = deblurValue / 100
noise       = reduceNoiseValue / 100
details     = recoverDetailsValue / 100
halo        = dehaloValue / 100
blur        = sharpenValue / 100
compression = revertCompressionValue / 100
```

`EnhancementFilter.qml` 的默认值全部为 **0**；滑杆范围 `-100 … 100`（`EnhancementSlidersViewModel.qml`）。
所以：

- GUI 0 → 模型 0；+100 → 1.0；-100 → -1.0；
- 模型 JSON 里的 `default: 0.5` 不是 GUI 默认，只是元数据默认（自动估计/相对模式的基准）；
- 旧文档 §2.4 把 JSON 默认 0.5 当作实际默认，不准确。

**另一个关键点**：六个参数的**有效范围都是 [-1, 1]**。
JSON 里 `noise/details/halo/blur/compression` 写的是 `0…1`，但滤镜的 AVOption 和 GUI 滑杆都允许负值；
QML 的自动估计日志也出现负数（如 `-0.350764`）。
当前 `tvai` 用 JSON 的 `Min/Max` 做 clamp，会把 `--denoise -50` 之类的负值截成 0，与 Topaz 不一致。

同时确认：

- Proteus / Rhea / Iris / Nyx / Nyx Fast 都传完整的 6 个参数；
- Theia（Thd/Thf）只传 `noise`、`blur`（sharpen）、`compression`；
- `autoParams != Manual` 时追加 `:estimate=8`；
- `addNoise > 0` 时传 `:prenoise=`；`recoverOriginalDetail > 0` 时传 `:blend=`；颗粒走 `:grain=:gsize=`。

### 2.4 模型能力：❌ 旧表缺了多组参数模型

对 68 个模型 JSON 汇总：

- **32 个短名**、35 个（短名, 版本）家族，不是"31 个家族 / 73 个元数据"。
  `modelmeta/` 里另有 `audio-codecs.json`、`video-encoders.json`、`benchmarks.json`、`proxy.json`。
- 带 6 参数（`preBlur, noise, details, halo, blur, compression`，顺序固定）的模型：
  **prob, iris, nyx, nxf, rhea, thd, thf, nap, prap**。
  旧文档说 Nyx/Nyx Fast/Iris/Rhea "无参数"、Theia 只有 3 个，均不准确。
- 无参数：`aaa, ahq, alq, amq, alqs, amqs, ddv, dtd, dtds, dtv, dtvs, ghq, gcg, thm, ref, cpe` 及各插帧模型。
- `keepColor=1`：prob/iris/rhea/thd/thf/aaa/ahq/alq/amq/ddv/dtd/dtds/dtv/dtvs/alqs/amqs/gaia；
  `keepColor=0`：nyx/nxf/thm/所有插帧模型/ref/cpe。
- `frames`：Gaia=5，Rhea=3，Thd/Thf=5，nap/prap=2；`frameCount`：Apollo/Aion/aiob=4，Apf=2。
- `blockOverlap`：只有 Aion=32；其余模型没有该键。

### 2.5 模型文件与 tile：✅ 代码是对的，文档方向写反了

JSON 的 `blocks` 是 **`[W, H]` 成对**；文件名是 **`HxW`**。
例如 `prob-4` 的 `[672, 576]` 对应文件 `prob-v4-fgnet-fp16-576x672-*.tz`。
旧文档 §2.5 写的是 `(H,W)` 对，方向反了；`tvai/internal/model` 的实现与之相符。

每个增强家族有 9 个 block × 3 个 scale = 27 个文件；Apollo 12 个块；
Aion/Aiob 是单文件动态尺寸。

### 2.6 真实 IO：大幅补充（旧表只有 7 行，且部分精度写错）

从 **32 个短名家族**各抽一个最小模型，用 `.tz` 密码解包后直接读 ONNX：

| 家族 | 输入 | 输出 | 输入精度 |
|---|---|---|---|
| prob-v4 | `lr_prev/lr_curr[1,H,W,3]`, `params[1,6,1,1]`, `hr_prev[1,S·H,S·W,3]`, `scene_change_threshold` | `out[1,S·H,S·W,3]` | fp16 |
| nyx-3 / nxf-1 | 同上但无 scene，`hr_prev[1,H,W,3]`（1x） | 同尺寸 | fp16 |
| iris-2/3 | 同上，`hr_prev[1,S·H,S·W,3]`；v2 采样无 scene | 同尺寸 | fp16 |
| rhea-1 | 多一个 `lr_next[1,H,W,3]` | 同尺寸 | fp16 |
| apo-8 | `in0..in3[1,H,W,3]` | `out[1,H,W,24]` = 8 帧 | fp16 |
| apf-2 | `in0,in1[1,H,W,3]` | `out[1,H,W,9]` = 3 帧 | fp16 |
| aion-1 | `in0..in3`, `in8x[1,H/4,W/4,24]` | `out[1,H,W,48]` = 16 帧 | **fp32** |
| aiob-1 | `in0..in3` | `out[1,H,W,24]` = 8 帧 | **fp32** |
| chf-3 / ifi-1 / chr-2 | `tfinetr2_input:0[1,H,W,7]` | 1 帧 `[1,H,W,3]` | fp32（chr 为 fp16） |
| aaa-9 fnet | `fnet/input:0[1,H,W,6]` + 权重输入 | `fnet/output:0[1,H,W,2]` | fp16 |
| ahq/alq/amq/ddv/dtd/dtv/… | `fgwnet/input:0[1,H,W,9]` | `generator/output:0[1,H,W,3]` | **fp32** |
| ghq/gcg（Gaia） | `VideoSR_Unet/inputFrame0..4:0` + 权重输入 | 1 帧 | fp16 |
| thd/thf fnet | `tlflow/input:0[1,H,W,6]` + 权重输入 | `tlflow/.../flow[1,H,W,2]` | fp16 |
| thm | `in0..in3` | 1 帧 | fp16 |
| ref-2 | `input[1,H,W,28]` | `output[1,H,W,3]` | fp16 |
| cpe-1/2 | `input_10..12[1,H,W,3]` | `output[1,4,1]` | fp32 |
| nap/prap | `input[1,H,W,6]` | `params[1,6]` | fp32 |

其他修正：

- `producer_name`：prob/nyx/apo 是 **pytorch**（1.12/1.13），只有 Gaia 是 **tf2onnx 1.7.1**。
  旧文档"全部由 tf2onnx 1.7.1 转出"不成立；metadata_props 为空。
- 部分模型（Artemis/Dione/Gaia/Theia）把权重也声明为 graph input（同时有 initializer 默认值）。
  用 ORT 直接跑没问题，但这解释了为什么这些图的 input 列表很长。
- `apf` 输出 9 通道 = 3×3，即一次产出 3 帧；JSON `outputFrameCount=4` 可能把输入帧也算入。
  Chronos/IFI 一次只产 1 帧，靠多次调用实现倍速。

### 2.7 场景切换阈值：❌ 旧实现差了两个数量级

在 `videoai.dll` 的增强执行函数（0x180092d80）里，设置
`scene_change_threshold` 前有一条：

```
180093161  mov dword ptr [rax], 0x40d00000   ; float 6.5
180093180  lea rdx, [rip+...]                ; "scene_change_threshold"
```

即 **Topaz 传 6.5**，不是当前 `tvai` 用的 `0.04`。

再看 `prob` 的 ONNX 图：

- 该输入只被 `Greater` 使用一次；
- `Greater` 的左操作数是 `ReduceMean(Abs(Mul(fnet_flow, 32.0)))`；
- mask = `Cast(Greater(...))`，随后参与的实际表达式是：

```
curr       = Resize(lr_curr, S·H, S·W)
prev       = GridSample(hr_prev, fnet_flow)      # 用光流 warp 上一帧输出
blended    = mask*curr + (1 - mask*curr) * prev
```

即不是"场景切换时清空 hr_prev"，而是一个依赖当前帧像素值的软混合。

也就是说：

1. 阈值作用在"fnet 光流幅度的均值 ×32"上，量纲与像素差完全不同；
2. 模型自己会处理场景切换，`tvai` 外部再用 `SceneDiff > 0.35` 重置 `prevHR` 属于另一种近似；
3. 当前 0.04 远低于 6.5，会让 mask=1 的比例明显偏高，时序融合行为与 Topaz 不同。

### 2.8 分块重叠与选块：❌ 默认重叠错，选块算法也被简化了

`videoai.dll` 0x18008e620 读取模型 JSON 的 `blockOverlap`；
**找不到时默认 `0x30 = 48`**（0x18008e863）。Aion 的 JSON 明确写 32。
真实日志里 Apollo 8x / 4K 也打印：

```
Checking block sizes 3840x2160 penalty 0.05 overlap 48
Make backend 6 input size 3840x2160 block 1344x1152 onnx16 device 0 memory 1
```

选块评分函数（0x1800873f0）已完整还原：
对每个候选 `(Wc, Hc)`，设输入 `(W, H)`、重叠 `o`、`penalty p`：

```
nx = ceil((W - o) / (Wc - o))
ny = ceil((H - o) / (Hc - o))
n  = nx * ny
score = n * p + (n * Wc * Hc - W * H) / (W * H)
```

取 score 最小者；`Wc <= o` 或 `Hc <= o` 的候选直接跳过。
`penalty` 默认 **0.05**（JSON 的 `penalty` 可覆盖）。

这解释了为什么 4K 选 672x576 / 1344x1152（面积浪费小、块数少），
而 720p 可能选 480x384（最大块反而浪费大）。
当前 `tvai` 的 `chooseTile` 只取"能放下的最大面积块"，在非 4K 分辨率会与 Topaz 不一致。

> 固定块模型（prob/apo/…）的日志里只出现 "Checking block sizes"，没有
> "Estimated block size"，说明它们直接走上面的评分函数；
> 目前只有 Aion/Aiob 的 onnx16 后端、以及 Gaia 的 openvino8 后端带 `pixelsPerGB`，
> 这类动态模型才用 `maxProcPixels()` 合成候选块。
> 这一点由 Apollo 4K 的日志与评分结果吻合支持。

另有 `maxProcPixels()`（0x180098b40）：

```
maxArea = clamp((int(deviceGB) - baseGB) * pixelsPerGB, 20000, 4_000_000)
```

- Aion/Aiob：`baseGB=1.2, pixelsPerGB=120000`，23 GB 卡 → 约 2.6M 像素预算；
- Gaia：只有 `openvino8` 后端的 scale 4 标了 `baseGB=0.5, pixelsPerGB=80000`；
  onnx16 的 Gaia 元数据没有这两个字段，仍走固定块评分。
- 其余固定块模型没有这两个字段，预算落到下限 20000，
  但固定块最终仍按上面的评分选，不会被这个小预算直接压到最小块。

> 旧文档 §6 的"分块重叠宽度未知"现在可以关闭：**增强/Apollo = 48，Aion = 32**。

### 2.9 设备与并发参数：❌ 语义需要改

QML `getDeviceSettings()`（`work/app_res/051_1dfc34.txt` 278–289 行）：

```js
:device=<gpu>
:vram=<maxMemoryUsage >= 10 ? maxMemoryUsage / 100.0 : 0.5>
:instances=<... ? 0 : 1>
```

- `maxMemoryUsage` 默认 **100** → `vram=1.0`（比例）；
- `exportInstances` 默认 1；只有 `exportInstances > 1`（非 macOS）或省电模式才传 `instances=0`；
- 因此默认是 `instances=1`，即 **1 个额外实例 = 2 个会话**；
- Aion 在 Windows 上被 `FrameInterpolationFilter.qml` 强制改成 `instances=0`；
- 用户日志里的 `instances=0` 是因为设置里 `exportInstances=2`。

旧文档把 `vram` 说成"每会话显存上限 GB"、把 `instances` 0–3 说成实例总数，都不对。
`tvai` 的 `--vram GB` / `--instances` 是自定义语义，与 Topaz 不可直接对照。

### 2.10 `rdt` / Sensitivity：✅ 映射已确认

`FrameInterpolationFilter.qml`：

```js
replaceDuplicateFrames = replaceDuplicateFramesValue / 1000   // 滑杆 0-100
```

默认 `replaceDuplicateFramesValue = 10` → `rdt = 0.01`；
日志里的 `0.065` 对应滑杆 65，不是默认值。
关闭时传 `-0.000001`（过滤器认为 <=0 即不删帧）。

### 2.11 编码器与音频：✅ 基本一致，补充两点

- 真实日志的 NVENC 参数、`-sws_flags spline+accurate_rnd+full_chroma_int`、
  第二遍 cleanup、`-metadata videoai=...` 都与复刻版一致；
- Topaz 的 `video-encoders.json` 有 50 项，复刻版只注册了 11 项；
  `cqpValues` 是数组（如 `High:[17]`），`bitrateOpts` 的 cbr/vbr 模板与复刻版一致；
- `tvai_pe` 的估计结果通过 **stderr** 打印：
  `Parameter values:[ -0.350764, ... ]`，QML 取 6 个参数、每帧取中位数，忽略后 4 个零。

### 2.12 颜色处理：✅ 算法已定位（运行验证）

静态部分：

- 滤镜 `kcolor` 默认 1，帮助文本："Run extra color correction if required by model"；
- 模型 JSON `keepColor` 决定是否"required"（prob/iris/rhea/theia/artemis 等为 1，nyx/nxf/thm/插帧/ref 为 0）；
- `videoai.dll` 读取 `keepColor` 后创建一个 0x1c8 字节的后处理器。

运行验证（720p testsrc2 + noise，30 帧，GPU）：

- 对 **prob-4（keepColor=1）** 同一帧跑 `kcolor=0/1`：
  - 差异是**全局近似线性**的：RGB 通道 `k1 ≈ 0.9915/0.9953/0.9931 * k0 + 0.26/0.08/0.41`；
  - 3×3 RGB 拟合的交叉项只有 ~0.001，基本是对角阵；
  - 转 Lab 后，`k1` 的均值/标准差几乎等于**输入帧**的 Lab 统计（差 ~0.1），
    Reinhard 预测残差约 `L=0.23, a=0.52, b=0.44`；
  - RGB 空间做同样的均值/方差匹配残差明显更大（~1.17/0.71/1.03）；
  - 按输出 tile 分块统计残差均值≈0，说明是**逐帧全局**校正，不是逐 tile。
- 对 **nyx-3（keepColor=0）** 跑 `kcolor=0/1`：解码后的帧**逐像素完全相同**；
  同一参数重复跑两次也完全相同（GPU 路径确定性良好），说明 `kcolor` 被模型标志门控。

结论：`keepColor` 约等于 **Reinhard 色彩迁移：在 Lab（或非常接近的空间）把输出帧的
均值/标准差匹配到输入帧**，逐帧、全局、模型级开关；不是固定 3×3 矩阵，也不是直方图匹配。
残差 <0.5 Lab 单位，可能来自 Lab 转换/取整或目标统计的采样方式；作为复刻可以用
"Lab 均值/方差匹配"近似到肉眼不可分的程度。

### 2.13 运行验证：独立跑 Topaz 滤镜的正确姿势

`TVAI_MODEL_DIR` 和 `TVAI_MODEL_DATA_DIR` 的命名与实际含义相反：

```
TVAI_MODEL_DIR      = ...\App\Topaz Labs LLC\Topaz Video AI\models   # 元数据 JSON + auth.tpz
TVAI_MODEL_DATA_DIR = C:\zy\APP\Big_APPs\TopazVideoAI\models        # .tz 模型文件
TVAI_ALWAYS_LOG=1
```

- `auth.tpz` 缺失只会打印 `Invalid auth file`，**不影响**本地已安装模型运行（GUI 日志同样如此）；
- 输入帧数必须大于模型的 preflight 缓冲：3 帧输入会得到 `No filtered frames`，30 帧正常；
- 同一模型/同一帧在 GPU 上可复现（nyx 对照逐像素相同），适合做 A/B；
- `ffmpeg -h filter=tvai_up` 的输出与 §2.2 的 AVOption 表**逐字一致**（模型默认 `amq-13` 等）。

### 2.14 运行验证：选块结果

用 30 帧/2 帧测试片，读取 `Checking block sizes ...` 与 `Make backend ... block WxH`：

| 模型 | 输入 | 实际重叠 | penalty | 选中的 block (W×H) |
|---|---|---|---|---|
| prob-4 2x | 1280×720 | 48 | 0.05 | **672×384** |
| prob-4 2x | 1920×1080 | 48 | 0.05 | **672×576** |
| prob-4 2x | 3840×2160 | 48 | 0.05 | **672×576** |
| nyx-3 1x | 1280×720 | 48 | 0.05 | **480×384** |
| nyx-3 1x | 1920×1080 | 48 | 0.05 | **1024×1088** |
| nyx-3 1x | 3840×2160 | 48 | 0.05 | **1024×1088** |
| rhea-1 4x | 1920×1080 | 48 | 0.05 | **672×576** |
| rhea-1 4x | 3840×2160 | 48 | 0.05 | **672×576** |
| apo-8 slowmo=2 | 1920×1080 | 48 | 0.05 | **1056×1088** |
| apo-8 slowmo=2 | 3840×2160 | 48 | 0.05 | **1344×1152**（旧日志） |

两点关键结论：

1. 720p 下 `prob`/`nyx` 都**不选最大可放下的块**（672×576），而选 672×384 / 480×384，
   与 §2.8 的评分公式吻合；当前 `chooseTile` 的"最大面积"策略在 720p 会选错。
2. Apollo 1080p 选了 **1056×1088**，高度 1088 > 输入 1080：
   Topaz **允许候选块大于输入帧**（边缘 padding），而当前 `chooseTile` 的"必须完整放进帧"
   会把它排除，退而选 1152×704。

另外，Nyx 的块表与通用 9 块不同：

- nyx-3 1x：`288×288, 480×384, 672×576, 1024×1088`；
- nyx-3 2x：`288×288, 480×384, 672×576, 1024×768`；
- iris-3 1x 也有 `384×288, 480×576, 576×480` 等专属块。

当前 `internal/model` 对所有增强模型共用一组 9 块，补 Nyx/Iris 时必须按 JSON 分 scale 读取。

---

## 3. 对 `TOPAZ逆向复刻-调查与方案.md` 的具体修正

| 文档位置 | 原说法 | 修正 |
|---|---|---|
| §1.3 `tvai_up` 选项 | 含 `sharpen, denoise, threads` | 实际是 `preblur, noise, details, halo, blur, compression`，无 `threads` |
| §1.3 `tvai_stb` | `rsc`、`fullframe` | `roll`、`full`，另有 `ws`、`filename` |
| §1.3 `tvai_pe` | 有 `output, device` | 只有 `model, download` |
| §1.3 `tvai_cpe` | 默认 cpe-1、device、download | 另有 `filename`，默认 `cpe.json` |
| §2.3 | 模型全部 fp16、tf2onnx 1.7.1 | aion/aiob/ahq 系输入 fp32；prob/nyx/apo 由 PyTorch 导出；仅 Gaia 是 tf2onnx |
| §2.3 | Aion 为 fp16 | 图输入为 fp32，权重混合精度 |
| §2.4 | 参数默认 0.5 | GUI 默认 0；0.5 只是 JSON 元数据 |
| §2.5 | blocks 是 (H,W) | 是 (W,H)；文件名才是 HxW |
| §3 | Nyx/Nyx Fast/Iris/Rhea 无参数 | 均有 6 参数；Thd/Thf 也有 6 参数（GUI 只用 3 个） |
| §3 | 73 个模型 / 31 家族 | 68 个模型 / 32 短名 / 35 家族；其余 4 个 JSON 是编码器/音频/benchmark/proxy |
| §6 未知项 1 | 重叠宽度未知 | 增强/插帧默认 48，Aion 32 |
| §6 未知项 2 | 色彩处理未知 | `kcolor` 门控 + 逐帧全局 Lab 均值/方差匹配（Reinhard） |
| §6 未知项 3 | 场景阈值未知 | 6.5 |
| §6 未知项 4 | params 顺序待验证 | JSON 顺序、AVOption 内存布局、QML 映射三方一致；ONNX 侧确认 params 作为 6 通道整体广播 |
| §4/§6 | `vram` 是 GB | 是显存比例 0.1–1，默认 1.0 |
| §4/§6 | `instances` 0–3 是实例数 | 是**额外**实例数；默认 GUI 传 1 |
| §7 待决策 2 | 需要跑 `ffmpeg -h filter=tvai_up` | 已不需要：AVOption 表已完整解出 |

---

## 4. 新挖到的、旧文档没有的信息

1. **完整 AVOption 表**（20+8+18+2+4 项）及默认值/范围/帮助文本：`work/tvai_avoptions.txt`。
2. **GUI 的 QML/JS 源码可读**：`work/app_res/` 123 个文件，包含 `EnhancementFilter.qml`、
   `FrameInterpolationFilter.qml`、`EnhancementSlidersViewModel.qml`、`AppSettings` 等。
   以后不需要猜参数映射、默认值、命令拼接。
3. **场景切换的真实机制**：模型内部用 fnet 光流幅度做 `Greater`，mask 软混合 flow 变形后的
   `hr_prev` 与当前帧；阈值 6.5。
4. **选块评分公式**、`penalty=0.05`、默认重叠 48/32、`maxProcPixels` 公式。
5. **各家族真实 ONNX IO 表**（30 个家族，含精度、通道数、动态尺寸）。
6. **`tvai_pe` 的实际协议**：输入 `[1,H,W,6]` → 输出 `[1,6]`，stderr 打印 6+4 个浮点，
   GUI 取中位数。
7. **Aion 的 `in8x` 尺寸** `[1,H/4,W/4,24]` 与 fp32 输入、48 通道输出。
8. **Rhea 的 `lr_next`**、Nyx/Nyx Fast 的 1x `hr_prev`、Ref 的 28 通道输入、
   CPE 的 4 元四元数输出等。
9. **模型权重作为 graph input** 的现象（Artemis/Dione/Gaia/Theia）。
10. **Aion 在 Windows 上强制 `instances=0`**（QML 里有明确注释）。
11. **keepColor 的运行级算法**：逐帧全局 Lab 均值/方差匹配（Reinhard 风格），
    模型 `keepColor=0` 时 `kcolor` 完全无效（像素级相同）。
12. **选块的真实验证结果**：prob 720p→672×384、1080p/4K→672×576；
    nyx 720p→480×384、1080p/4K→1024×1088；apo 1080p→1056×1088、4K→1344×1152。
    验证了 §2.8 的评分公式，并发现候选块可以大于输入帧。
13. **独立运行 Topaz 滤镜的 env 配方**：见 §2.13（`TVAI_MODEL_DIR` 是元数据目录，
    `TVAI_MODEL_DATA_DIR` 才是 `.tz` 目录，和名字直觉相反）。

---

## 5. 对当前 `tvai` 代码的直接影响（已实施，见 §8）

按影响排序：

1. `internal/pipeline/runner.go`：`sceneThreshold` 0.04 → **6.5**；
   并考虑移除/弱化 `upscale.go` 里基于像素差的 `ResetTemporal`，改为完全交给模型。
2. `internal/pipeline/runner.go`：参数 clamp 应允许 **[-1, 1]**（当前五个参数被 JSON 的 0…1 截掉负值）。
3. `internal/pipeline/upscale.go` / `interpolate.go`：默认 `overlap` 16 → **48**
   （Aion 32），或至少让默认值与模型 JSON 的 `blockOverlap` 对齐。
4. `internal/pipeline/frame.go` `chooseTile`：改成 §2.8 的 `ceil` + penalty 评分；
   不要要求候选块完整放进帧（Apollo 1080p 会选 1056×1088 > 1080）。
5. 模型范围保持四模型：Proteus / Gaia / Apollo / Aion；Nyx/NxF/Iris/Rhea 不实现。
6. `keepColor`：模型 `keepColor=1` 且 `kcolor=1` 时，对输出帧做 Lab 均值/方差匹配到输入帧。
7. CLI 的 `--vram` / `--instances` 语义与 Topaz 不同，文档应明确区分；
   `--sensitivity` 映射 Topaz 滑杆（10 → rdt 0.01），`--rdt` 保留原始覆盖。
8. 颜色/位深：Topaz 走 `-sws_flags spline+accurate_rnd+full_chroma_int` 且滤镜支持
   `kcolor`；当前 `tvai` 全程 rgb24，10bit 编码器实际从 8bit 转出。

---

## 6. 仍未确定 / 建议的下一步实验

1. **keepColor 的精确实现**：已确认是逐帧全局 Lab 均值/方差匹配，但残差 ~0.5 Lab 单位；
   还需要确定它用的是哪种 Lab 转换、统计采样方式（全帧/中心裁剪）、以及是否对
   模型输出先做模糊/降采样再统计。
2. **场景阈值 6.5 的运行时效果**：静态值已确认，但引擎日志不打印它；
   可以对比“同一帧在 0.04/6.5 下的输出”量化差异（需要 Go 侧支持传入阈值）。
3. **Aion/Gaia 的动态 tile**：`maxProcPixels` 会给出约 2.6M 像素的大块，
   与当前 `tvai` 的小块策略不同；需要实测哪种更快/画质更接近。
4. **子帧顺序与时间戳**：Apollo 24 通道 / Aion 48 通道的帧序尚未用真实输出验证。
5. **Aion 的 in8x 构造**：JSON 的 `baseModel/dependencies/in8x` 与当前两级实现一致，
   但没有从 engine 代码直接确认 aiob→降采样→aion 的每一步。
6. **自动参数估计**：`tvai_pe` 的 6 值输出与 GUI 中位数逻辑已确认，
   但 Go 侧的 estimate 集成尚未实现。

---

## 7. 本次产物

```
work/avopt3.py           正确的 AVFilter/AVClass/AVOption 解析器
work/tvai_avoptions.txt  5 个 tvai_* 滤镜的完整选项表
work/xref.py             PE + capstone 交叉引用/函数反汇编工具
work/tile_funcs.txt      分块/选块/重叠相关函数反汇编
work/score_raw.txt       选块评分函数原始反汇编
work/app_res/            exe 内嵌 QML/JS 解压结果（123 个）
work/app_text.txt        exe 明文资源串
work/modelmeta/          68 个模型 JSON + 4 个非模型 JSON
```

原始 Topaz 目录仍为只读访问，未修改任何文件。


---

## 8. 代码实施记录（2026-10-17）

已按 §5 落地到 `tvai/`：

| 项 | 实现 |
|---|---|
| 场景阈值 | `DefaultSceneChangeThreshold = 6.5`，新增 `--scene-threshold`；移除基于像素差的 `prevIn` 重置 |
| 参数范围 | 六个参数统一 clamp 到 `[-1,1]`，允许负值 |
| 分块重叠 | 默认 0=自动（增强/Apollo 48，Aion 32）；`--overlap` 可覆盖 |
| 选块 | 改为 Topaz 的 `ceil` + 0.05 penalty 评分；允许候选块大于输入帧 |
| keepColor | 新增 `--kcolor`；Proteus/Gaia 做 Lab 均值/方差匹配 |
| 模型范围 | 仅保留 Proteus / Gaia / Apollo / Aion；Nyx/NxF/Iris/Rhea 不实现 |
| 重复帧替换 | Apollo/Aion 新增真正的 Replace：检测相邻重复帧后用前后非重复帧生成插值帧替换，输出帧数/FPS 不变；`--sensitivity` 0-100 映射 rdt，`--rdt` 原始覆盖 |
| 稳定性 | 修复 Apollo tile 批处理固定 batch=1 的维度错误；修复 `Probe` 缺 `nb_frames` 的帧数兜底；修复 `parallelRows` 行数不整除时等待挂起 |
| 文档/测试 | README 更新；新增选块、Lab 色彩迁移、模型块表的单元测试 |

验证：

- `go test ./...` 通过；`go build -ldflags="-s -w" ./cmd/tvai` 通过；`gopls check -severity=hint` 对改动文件无提示（`go vet` 仅剩 `ort` 的既有 unsafe 提示）。
- 端到端运行通过：proteus 720p 2x 30 帧；gaia 5 帧；apollo/aion 重复帧替换各 59 帧。
- 与 Topaz 同源输出对比：proteus 720p 第 10 帧 vs Topaz `kcolor=1` PSNR **37.2 dB**（vs `kcolor=0` 36.6 dB）。
- 选块实测与代码一致：proteus 720p→672x384、1080p/4K→672x576；apo 1080p→1056x1088、4K→1344x1152。
- 重复帧替换实测：30 帧含 15 组重复的测试片，Apollo slowmo=2 输出 59 帧并替换 28 帧（Sensitivity 100）；Aion 输出 59 帧并替换 14 帧（Sensitivity 10）。

未纳入本次改动：

- Nyx/NxF/Iris/Rhea 不在工具范围内（逆向结论保留在 §2–§4）。
- 流水线三阶段重叠、Aion 的 Topaz 动态大 tile 策略未改。
- 重复帧替换保留输出帧数/FPS；Topaz 的实现在部分边界会少 1-3 帧，本工具不模拟该细节。
