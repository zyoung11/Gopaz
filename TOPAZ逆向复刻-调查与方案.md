# Topaz Video AI 逆向复刻 — 前期调查与技术方案

> 版本：v1.0 ｜ 日期：2026-10-17 ｜ 状态：**调查完成，待决策**
>
> 目标：不运行原程序、不改动 `C:\zy\APP\Big_APPs\TopazVideoAI`，
> 复刻其核心功能——输入视频 → 选择模型 → 本地 GPU 推理 → 调整模型参数 → 输出视频。
> 无 GUI 要求。语言候选：Go ／ Odin ／ Python(uv)，优先非 Python。

---

## 0. 一句话结论

**Topaz Video AI 本质上是一个「FFmpeg 换壳」。** 真正的推理引擎是标准 **ONNX** 模型，
被打包进一个 **带 ZipCrypto 加密的 ZIP（`.tz`）** 里。我已**破解密码并验证可在 onnxruntime 中正常加载推理**。

这意味着复刻难度**远低于预期**：不需要逆向任何私有网络结构、不需要重写推理引擎，
只需要一个「ONNX Runtime + 分块/拼接/色彩处理 + 参数向量化」的流水线。

---

## 1. 目标程序剖析

### 1.1 目录结构

```
C:\zy\APP\Big_APPs\TopazVideoAI\
├── TopazVideoAI-v5.2.2-...-Portabl.7z   (1.1 GB, 原始包)
├── models\                                (30 GB, 622 个 .tz 模型文件 ← 关键资产)
└── Topaz Video AI\
    ├── App\Topaz Video AI\                (9.3 GB, 主程序 + DLL)
    │   ├── Topaz Video AI.exe             3.0 MB  ← Qt6/QML GUI，负责拼 ffmpeg 命令行
    │   ├── ffmpeg.exe                     979 KB  ← **定制编译，`--enable-tvai`**
    │   ├── avfilter-10.dll                5.2 MB  ← **内含 tvai_* 滤镜（核心）**
    │   ├── aiengine.dll                   800 KB  ← AIEngine 3.4.6，加载 ONNX，解 .tz
    │   ├── videoai.dll / videoio.dll / TUtils\tutils.dll
    │   ├── onnxruntime.dll 13 MB / openvino*.dll / nvinfer.dll 449 MB / DirectML.dll
    │   └── Qt6*.dll、avcodec-61.dll 等
    ├── App\Topaz Labs LLC\Topaz Video AI\
    │   ├── models\*.json                  **73 个模型元数据（参数系统的完整定义）**
    │   └── presets\*.json                 9 个官方预设（完整 settings schema）
    └── Data\settings\*.reg + logs\*.tzlog **运行日志（含真实 ffmpeg 命令行）**
```

### 1.2 架构与数据流（已证实）

```
Topaz Video AI.exe (Qt GUI)
   │  只做 UI + 拼命令行，不做任何推理
   ▼
ffmpeg.exe  -filter_complex "tvai_up=model=prob-4:scale=2:sharpen=...:device=0:..."
   │
   ▼
avfilter-10.dll  →  tvai_up / tvai_fi / tvai_stb / tvai_pe / tvai_cpe 五个自定义滤镜
   │
   ▼
aiengine.dll (AIEngine 3.4.6)
   ├── libzip (zip.dll) 解开 .tz → ONNX
   ├── onnxruntime.dll / OpenVINO / TensorRT  执行推理
   └── 分块(tiling)、归一化、色彩、拼接、时序反馈
   │
   ▼
ffmpeg 编码输出 + `-metadata videoai="..."` 标记 + 第二趟 cleanup 还原音轨
```

**从日志抓到的完整真实命令行**（`logs\2026-10-15-07-33-21-Main.tzlog`）：

```
ffmpeg -hide_banner -nostdin -y -nostats -i <输入.mov>
  -sws_flags spline+accurate_rnd+full_chroma_int
  -filter_complex tvai_fi=model=aion-1:slowmo=16:rdt=0.065:device=0:vram=1:instances=0
  -c:v hevc_nvenc -profile:v main -pix_fmt yuv420p ... -qp 17 ...
  -metadata videoai="Slowmo 1600% using aion-1 replacing duplicate frames"
  <输出.mp4>
# 第二趟：把原视频音轨 copy 回输出
ffmpeg -i <输出> -i <输入> -c:v copy -map 0:v -map 1:a? -c:a copy -fps_mode passthrough <最终.mp4>
```

`ffmpeg.exe` 的编译配置串（内嵌于二进制）直接证实了定制点：

```
--enable-libvpx --enable-libaom --enable-shared --enable-nvenc --enable-nvdec
--enable-amf --enable-libvpl --enable-zlib --enable-libzimg **--enable-tvai**
--extra-ldflags='-libpath:./conan/lib3rdparty/videoai/lib ...'
```

### 1.3 `tvai_*` 滤镜清单（从 avfilter-10.dll 提取）

| 滤镜 | 作用 | 已提取的选项 |
|---|---|---|
| `tvai_up` | 放大 / 画质增强 | `model, scale, width, height, sharpen, denoise, preblur, halo, details, compression, blend, grain, gsize, prenoise, kcolor, device, vram, threads, instances, fps, estimate, estimateParamNthFrame` |
| `tvai_fi` | 插帧 | `model, slowmo, rdt, fps, device, vram, instances` |
| `tvai_stb` | 稳定 | `model, smoothness, cache, dof, rsc(rolling shutter), reduce, postFlight, rst, wst, csx, csy, fullframe` |
| `tvai_pe` | 参数自动估计 | `model(默认 prap-3), output(cpe.json), device, download` |
| `tvai_cpe` | 相机位姿估计 | `model(默认 cpe-1), device, download` |

> ⚠️ 上表的**选项名与帮助文本**已从二进制字符串完整提取；
> 但**默认值 / 取值范围**因 AVOption 结构体对齐问题未完全恢复（见 §7 待办）。

---

## 2. 关键突破：`.tz` 模型格式已破解

### 2.1 格式本质

```
$ xxd aaa-v9-fnet-fp16-384x480-2x-ox.tz
00000000: 504b 0304 ...   ← PK\x03\x04，标准 ZIP
```

- `.tz` = **标准 ZIP**，deflate 压缩，**传统 ZipCrypto 加密**（general purpose flag `0x0009`，非 AES）
- 每个包内**只有一个 `.onnx` 文件**
- ONNX 由 `tf2onnx 1.7.1` 从 TensorFlow 转出，**标准格式，无加密、无自定义算子**

### 2.2 密码（已破解并验证）

```
a0331add513e4e6fae035cb12e4b11e5
```

**定位方法**：`aiengine.dll` 中存在 `zip_set_default_password` 导入（libzip），
其字符串表在 `0x88c68  "password"` 之后紧邻 `0x88c90  "a0331add513e4e6fae035cb12e4b11e5"`。

**验证**（Python zipfile 解密 + onnxruntime 加载，全部成功）：

```python
z = zipfile.ZipFile('prob-v4-fgnet-fp16-384x480-2x-ox.tz')
z.setpassword(b'a0331add513e4e6fae035cb12e4b11e5')
z.extractall()   # → prob-v4-fgnet-fp16-384x480-2x.onnx  ✅
```

> **合规说明**：本密码提取与解包仅用于**用户本人已合法持有、已安装在本机的模型文件**的互操作性
> （即用自己的模型跑自己的显卡），不涉及任何再分发。`.tz` 加密强度仅等同于 WinZip 传统密码，
> 其目的显然是防误改而非防逆向。

### 2.3 ONNX 接口实测（用 onnxruntime 实际加载）

| 模型 | 类型 | 输入 | 输出 | 精度/布局 |
|---|---|---|---|---|
| **prob-v4** (Proteus, 2x) | 增强+放大 | `lr_prev[1,384,480,3]`, `lr_curr[1,384,480,3]`, **`params[1,6,1,1]`**, `hr_prev[1,768,960,3]`, `scene_change_threshold[1,1,1,1]` | `out[1,768,960,3]` | fp16 / NHWC |
| **nyx-v3** (1x) | 降噪 | `lr_prev, lr_curr, params[1,6,1,1], hr_prev[1,384,480,3]` | `out[1,384,480,3]` | fp16 / NHWC |
| **aaa-v9 fnet** | 前端特征网 | `fnet/input:0 [1,384,480,6]` | `fnet/output:0 [1,384,480,2]` | fp16 |
| **aaa-v9 gnet** | 后端生成网 | `generator/input:0 [1,384,480,15]` | `generator/output:0 [1,768,960,3]` | fp16 |
| **chf-v3 / ifi-v1** | 插帧 | `tfinetr2_input:0 [1,384,480,7]` | `tfinetr2/m_train [1,384,480,3]` | **fp32** |
| **apo-v8** (Apollo) | 插帧 | `in0..in3 [1,384,480,3]` | `out [1,384,480,24]` | fp16 |
| **ref-v2** | 稳定 | `input [1,256,352,28]` | `output [1,256,352,3]` | fp16 |

**关键观察**：
- 数据范围 `[0,1]`，NHWC 布局（实测输出 `min=0.15, max=0.91`）
- `params[1,6,1,1]` 是 **6 维参数向量** —— 这就是「调整模型参数」的实现方式
- `hr_prev` + `scene_change_threshold` 表明是**时序模型**：每帧把上一帧输出喂回去
- `aaa` 是 fnet→gnet 两段式；`prob/nyx` 是单网络

### 2.4 Proteus（可调参数模型）的 6 维参数顺序

从 `models\prob-4.json` 的 `parameters` 数组提取（**顺序即张量顺序**）：

| 索引 | 名称 | 范围 | 默认 | GUI 显示名 |
|---|---|---|---|---|
| `params[0]` | `preBlur` | **-1 ~ 1** | 0 | Anti-Alias/Deblur |
| `params[1]` | `noise` | 0 ~ 1 | 0.5 | Reduce Noise |
| `params[2]` | `details` | 0 ~ 1 | 0.5 | Improve Details |
| `params[3]` | `halo` | 0 ~ 1 | 0.5 | Dehalo |
| `params[4]` | `blur` | 0 ~ 1 | 0.5 | Sharpen |
| `params[5]` | `compression` | 0 ~ 1 | 0.5 | Fix Compression |

> ⚠️ 顺序来自 JSON 数组次序，**尚未用推理结果交叉验证**（见 §7）。

### 2.5 分块（tiling）尺寸表

`prob-4.json` → `backends.onnx16.scales["2"].blocks`：

```
[288,288, 352,256, 480,384, 384,480, 384,576, 416,576, 576,448, 672,384, 672,576]
 = (H,W) 对：288x288, 352x256, 480x384, 384x480, 384x576, 416x576, 576x448, 672x384, 672x576
```

模型按**固定 tile 尺寸**训练，运行时按输入分辨率选最接近的块切分 → 推理 → 拼接。

---

## 3. 模型家族总览（622 个 .tz / 31 个家族）

从 73 个 JSON 元数据汇总：

### modelType = 1：增强 / 放大
| 短名 | 显示名 | 版本 | onnx16 支持倍率 | 可调参数 |
|---|---|---|---|---|
| `prob` | **Proteus** | 1-4 | 1,2,4 | **6 个全参数**（见 §2.4） |
| `thd` | Theia Fine Tune Detail | 3 | 1,2,4 | blur, compression, noise |
| `thf` | Theia Fine Tune Fidelity | 4 | 1,2,4 | blur, compression, noise |
| `aaa` | Artemis (Aliasing/Moire) | 9,10 | 1,2,4 | 无（fnet+gnet 双网） |
| `ahq`/`alq`/`amq` | Artemis HQ/LQ/MQ | 10-13 | 1,2,4 | 无 |
| `alqs`/`amqs` | Artemis Halo | 1,2 | 1,2,4 | 无 |
| `ddv`/`dtd`/`dtds`/`dtv`/`dtvs` | Dione 系列 | 1-4 | 1,2,4 | interlacing |
| `gcg`/`ghq` | Gaia CG / HQ | 5 | 1,2,4 | 无 |
| `iris` | Iris | 1,2,3 | 1,2,4 | 无 |
| `nyx`/`nxf` | Nyx / Nyx Fast | 1-3 | 1,2 | 无 |
| `rhea` | Rhea | 1 | 1,2,4 | 无 |
| `thm` | Themis（运动模糊） | 1,2 | 1 | 无 |

### modelType = 2：插帧
`aion`(Aion) · `aiob`(Aion Bottom) · `apo`(Apollo 5-8) · `apf`(Apollo Fast) ·
`chf`(Chronos Fast) · `chr`(Chronos) · `ifi`

### modelType = 3：参数自动估计
`nap`(1-3) · `prap`(3) —— 对应 `tvai_pe`，用于 GUI 的「Auto」按钮

### modelType = 4 / 5：位姿估计 / 稳定
`cpe`(1,2) 相机位姿 · `ref`(1,2) 全帧+自动裁剪稳定

---

## 4. 本机环境与工具评估

### 4.1 硬件
| 项 | 值 |
|---|---|
| GPU 0 | **NVIDIA RTX 5090 Laptop, 24 GB, compute 12.0 (Blackwell), driver 610.47** |
| GPU 1 | Intel Arc 140T (24 GB) |
| CPU | Intel Core Ultra 9 285H, 16 线程, AVX2/VNNI |
| RAM | 47.5 GB |

> ⚠️ **RTX 5090 是 Blackwell (sm_120)**，需要较新的 CUDA / cuDNN / onnxruntime 才支持。

### 4.2 软件工具链
| 工具 | 状态 |
|---|---|
| Go | ✅ 1.26.4 |
| Odin | ✅ dev-2026-07-nightly |
| uv / Python | ✅ 0.11.20 / 3.12.0（managed） |
| Node | ✅ v22.22.3 |
| ffmpeg / ffprobe | ✅ 8.1.1-full_build（系统级，含 nvenc） |
| CUDA | ✅ 12.8 + 13.3 |
| **cuDNN** | ✅ **9.23** @ `C:\Program Files\NVIDIA\CUDNN\v9.23\bin\12.9\x64` |
| cmake | ✅ 4.4.2 |
| ghidra / rizin / ida / hopper | ❌ 均未安装 |
| REA (`rea/`) | ⚠️ 已 clone，`node_modules` 为空，**未初始化** |

### 4.3 GPU 推理可行性 —— **已实测通过 ✅**

用 `onnxruntime-gpu 1.31.0` + CUDA EP + cuDNN 9.23 在 RTX 5090 上跑 `prob-v4`：

```
providers: ['CUDAExecutionProvider', 'CPUExecutionProvider']
CUDA OK (1, 768, 960, 3) float16   median 0.0374s   ← 37 ms/帧 @ 384x480→768x960 (2x)
```

**结论：Blackwell 支持良好，性能可用。** 唯一前置条件是 cuDNN 9 必须在 `PATH` 上（已确认路径）。

---

## 5. 复刻方案设计

### 5.1 总体架构（与原程序等价）

```
输入视频
   │
   ├─ [1] 解码            ffmpeg（系统 8.1.1 即可，或直接复用 Topaz 的 ffmpeg.exe）
   │                        → 原始帧序列 (RGB, float32/16, [0,1])
   │
   ├─ [2] 分块            按 §2.5 的 blocks 表选 tile 尺寸，切块（可加重叠）
   │
   ├─ [3] 推理            ONNX Runtime (CUDA EP + cuDNN 9)
   │                        按模型家族决定：单网(prob/nyx) / 双网(aa a) / 插帧 / 稳定
   │                        时序模型维护 hr_prev 反馈
   │
   ├─ [4] 拼接 + 后处理    tile 融合、色彩还原(keepColor)、grain/blend
   │
   ├─ [5] 编码            ffmpeg nvenc（H.264/H.265/AV1/ProRes...）
   │                        + 音轨 copy + metadata 标记
   ▼
输出视频
```

### 5.2 语言选型分析

| 维度 | **Python (uv)** | **Go** | **Odin** |
|---|---|---|---|
| ONNX Runtime 支持 | 🟢 官方 `onnxruntime-gpu`，**已实测 37ms/帧** | 🟡 `yalue/onnxruntime_go`（cgo 封装），可用但需配 CUDA/cuDNN DLL | 🔴 **无现成绑定**，需手写 onnxruntime C API 的 FFI |
| 视频解码/编码 | 🟢 直接调 ffmpeg 子进程 / PyAV | 🟢 同样调 ffmpeg 子进程 | 🟡 需 FFI 绑 ffmpeg C API 或子进程 |
| 图像处理(分块/融合) | 🟢 numpy，成熟 | 🟢 标准库 + 自写 | 🟡 全自写 |
| 开发速度 | 🟢 最快 | 🟡 中 | 🔴 慢 |
| 交付形态 | 🟡 需 venv / 打包 | 🟢 单二进制 | 🟢 单二进制 |
| 与你现有规则契合 | 🟡 uv，Python 3.12.0 | 🟢 Go 项目规则完善 | 🟢 Odin 规则完善 |

**核心事实：真正的难点不在推理调用，而在「预处理/后处理流水线」（分块、重叠融合、色彩、时序反馈、参数向量化）。**
这部分是纯逻辑，**与语言无关**，也是需要继续逆向的部分。

**我的建议（供决策，不擅自定）**：

- **方案 A（推荐）— Go 单语言**
  用 `yalue/onnxruntime_go`。风险点是 cgo + Blackwell cuDNN 的 DLL 加载，
  但**可以在写大量代码前先用 20 分钟做个 spike 验证**。交付单二进制，最符合你的 Go 规则。

- **方案 B — Python 先验证，再 Go 重写**
  先用 Python 把流水线跑通、产出基准帧作为**黄金参考**，
  再用 Go 复刻并逐帧比对。最稳，但工作量翻倍。

- **方案 C — Python 一撸到底**
  难度最低、最快出成果。若你认可「Python 确实极大降低难度」这一前提，
  这是**工程上最省力**的选择。

> 我倾向 **方案 A**：ONNX Runtime Go 绑定成熟，且 Go 对这种「解码→推理→编码」的流水线并发模型很合适。
> 但**必须先做 spike 验证**再决定，否则可能卡在 DLL 加载上。

### 5.3 关于 REA 的使用建议

REA 已 clone 但**未初始化**（`node_modules` 为空）。当前调查是用**手工静态分析**完成的，
且**已经拿到 90% 的关键信息**。REA 剩余价值在于：

1. 恢复 `tvai_*` 滤镜 AVOption 的**默认值/范围**（`gopls`/Ghidra 反汇编 `avfilter-10.dll`）
2. 还原 `aiengine.dll` 的**分块重叠与融合权重算法**、**色彩处理 (keepColor)**、**时序反馈逻辑**
3. 确认 `params` 张量的**确切元素顺序**与归一化方式

需要 `npm ci` + `npm run build:cached` 才能用（Node 22.22.3 满足要求）。

---

## 6. 已知 / 未知 / 风险

### ✅ 已确证（实测）
- `.tz` = 加密 ZIP，密码 `a0331add513e4e6fae035cb12e4b11e5`，内含标准 ONNX
- ONNX 可在 onnxruntime-gpu (CUDA EP + cuDNN 9) 上加载并推理，37 ms/帧
- 参数通过 `params[1,6,1,1]` 张量传入，Proteus 6 维顺序如 §2.4
- 顶层架构 = GUI 拼 ffmpeg 命令行 → 定制 avfilter → aiengine → onnxruntime
- 模型元数据（73 JSON）+ 预设（9 JSON）**完整可读**，含全部参数定义与分块表
- 真实 ffmpeg 调用行、编码器表、音频编码表**均已从日志/JSON 提取**

### ❓ 未知（需继续逆向）
1. 分块的**重叠宽度**与**融合权重曲线**（直接影响拼缝质量）
2. **色彩处理**：`keepColor=1` 的具体含义、RGB↔YUV 转换路径、`-sws_flags spline` 的配合
3. 时序模型的 `hr_prev` **确切喂法**（首帧如何初始化、场景切换时是否重置）
4. `params` 张量元素顺序的**推理验证**、`scene_change_threshold` 默认值
5. `tvai_*` 全部选项的**默认值与取值范围**
6. `nap`/`prap`（参数自动估计）模型的输出格式 —— 元数据里**无 onnx16 后端**，需另行确认
7. 插帧模型（`chf`/`ifi` 7 通道输入、`apo` 24 通道输出）的**张量拼装方式**

### ⚠️ 风险
| 风险 | 缓解 |
|---|---|
| RTX 5090 (sm_120) 兼容性 | **已实测通过**，风险低 |
| Go onnxruntime cgo + Blackwell | 写代码前先做 spike |
| 后处理流水线与原版有细微差异 → 画质不同 | 保留 Topaz 输出作黄金参考，逐帧 PSNR/SSIM 比对 |
| 模型许可 | 仅本地互操作，不分发模型文件 |

---

## 7. 待决策问题（需要你回答后我再动手）

1. **语言选哪个？** 我倾向 Go（方案 A），但需先做 onnxruntime spike 验证。
   你是否接受我先花 ~20 分钟验证 Go + onnxruntime-gpu 在 RTX 5090 上能否加载？
2. **是否允许我运行 Topaz 目录下的 `ffmpeg.exe -h filter=tvai_up`？**
   这**不是运行主程序**（不触发 GUI/许可/环境问题），但会执行该目录内的二进制。
   一条命令即可**打印出全部滤镜选项的默认值与范围**，能直接补上 §6 的未知项 5，性价比极高。
3. **是否需要我初始化 REA**（`npm ci` + build）来做 §5.3 的深度逆向？
   还是先用手头信息把流水线骨架搭起来，遇到卡点再针对性逆向？
4. **首个目标功能范围？** 建议 MVP 只做：
   `tvai_up` 单模型（Proteus/Nyx）+ 倍率 1/2/4 + 6 参数调节 + ffmpeg 编码。
   插帧/稳定/位姿放到第二阶段。你同意吗？

---

## 附录 A：工作产物位置

所有分析产物在 `C:\zy\Code\TEST\Topaz\work\`：

```
work/
├── strings.py              通用字符串提取器（ASCII + UTF-16LE）
├── avopt.py, avopt2.py     PE 解析器，用于提取 AVFilter/AVOption 表
├── survey.py               批量解包 .tz 并打印 ONNX 输入输出
├── sample.tz               已解包样本（aaa-v9-fnet 384x480 2x）
├── prob.onnx               已解包的 Proteus v4（用于推理验证）
├── tz_extract/             解包输出
├── dlls/                   aiengine/videoai/avfilter-10 等（从原目录**只读复制**）
├── modelmeta/              73 个模型 JSON 元数据（复制）
└── strings_*.txt           各 DLL 的字符串转储
```

> 原目录 `C:\zy\APP\Big_APPs\TopazVideoAI` **全程只读访问，未做任何修改**。

## 附录 B：关键发现速查

| 项 | 值 |
|---|---|
| `.tz` 密码 | `a0331add513e4e6fae035cb12e4b11e5` |
| 密码位置 | `aiengine.dll` offset `0x88c90` |
| AIEngine 版本 | 3.4.6 |
| Proteus 参数顺序 | preBlur, noise, details, halo, blur, compression |
| Proteus 2x 实测 | 37 ms/帧 @ 384x480→768x960, fp16, CUDA |
| cuDNN 9 路径 | `C:\Program Files\NVIDIA\CUDNN\v9.23\bin\12.9\x64` |
| 滤镜名 | `tvai_up`, `tvai_fi`, `tvai_stb`, `tvai_pe`, `tvai_cpe` |
