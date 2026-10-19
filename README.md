# Gopaz

Topaz Video AI 的本地复刻工具：提供命令行和本地 Web 界面。输入视频，用本地 GPU 推理，输出超分/修复或慢放/插帧后的视频。纯 Go 实现，**无 cgo、无 C 编译器、无 Python**。

当前只支持 Windows x64。仓库里的非 Windows 分支仅保留给后续移植，不能直接构建。

逆向依据、实现细节、性能数据与已知限制见 [ARCHITECTURE.md](ARCHITECTURE.md)。

## 功能

| 命令 | 说明 |
|---|---|
| `gopaz` | 启动本地 Web 界面并打开浏览器 |
| `gopaz serve` | 只启动 Web 界面 |
| `gopaz upscale` | 超分（2x/4x）或修复（1x） |
| `gopaz interpolate` | 慢放/插帧（2-16 倍） |
| `gopaz prepare` | 把 `.tz` 解压成明文 `.onnx` |
| `gopaz models` | 列出可用模型 |
| `gopaz encoders` | 列出可用编码器 |
| `gopaz devices` | 列出 DML 适配器和 OpenVINO 设备 |

| 模型 | 类型 | 倍率 | 参数 |
|---|---|---|---|
| proteus | 增强 | 1/2/4 | 6 |
| gaia / gaia-cg | 增强 | 1/2/4 | 无 |
| apollo | 插帧 | 2-16 | 无 |
| aion | 插帧 | 2-16 | 无 |

Apollo 单次推理产出 8 子帧，2/4/8 倍精确对齐；Aion 产出 16 子帧，2/4/8/16 倍精确对齐。其余倍数取最近子帧或复用子帧，与 Topaz 行为一致。

带 rotation 元数据的视频按显示方向处理，竖屏素材的输出仍为竖屏。

## 依赖

1. **ffmpeg / ffprobe** —— 优先用可执行文件同目录或同目录 `bin/` 下的版本，其次 PATH，也可用 `--ffmpeg`/`--ffprobe` 指定
2. **ONNX Runtime** —— CUDA/TensorRT 使用 `ort-dll/onnxruntime.dll` 1.31；DirectML 和 OpenVINO 使用各自子目录里的 1.24 运行时；也可用 `GOPAZ_ORT_DLL` 指定
3. **模型文件** —— 放在可执行文件同目录的 `models/`，或用 `GOPAZ_MODEL_DIR` 指定；支持明文 `.onnx` 和加密 `.tz`，同名时优先 `.onnx`
4. **CUDA + cuDNN 9** —— 优先用可执行文件同目录的 `runtime/`（程序会自动加入 PATH），否则需要在系统 PATH 上
5. **可选后端** —— TensorRT 需要 `ort-dll/tensorrt/`，DirectML 需要 `ort-dll/dml/`，OpenVINO 需要 `ort-dll/openvino/`；缺少时只影响对应 provider

## 构建

```bash
go build -ldflags="-s -w" -o gopaz.exe .
```

Web 前端在 `web/`，通过 `//go:embed` 打包进可执行文件，不需要 Node 或构建步骤。

## 绿色便携版

把运行时文件放进同一个目录即可，不需要安装：

```
gopaz-portable\
├─ gopaz.exe
├─ ffmpeg.exe
├─ ffprobe.exe
├─ ort-dll\
│  ├─ onnxruntime.dll                 # CUDA 1.31
│  ├─ onnxruntime_providers_cuda.dll
│  ├─ onnxruntime_providers_shared.dll
│  ├─ tensorrt\                      # TensorRT + CUDA fallback + tensorrt_libs
│  ├─ dml\                           # DirectML 1.24.4 + DirectML.dll
│  └─ openvino\                      # OpenVINO 1.24.1 + provider + libs
├─ runtime\              # 可选，CUDA / cuDNN 9 的运行库
└─ models\               # .onnx 或 .tz，不需要 Topaz 的 JSON
```

查找顺序：

- ffmpeg / ffprobe：exe 同目录 → exe 同目录 `bin\` → PATH
- 模型：`GOPAZ_MODEL_DIR` → exe 同目录 `models\` → exe 同目录 `..\models\`；同名 `.onnx` 优先于 `.tz`
- ORT：`GOPAZ_ORT_DLL` → exe 同目录 `ort-dll\`；DirectML/OpenVINO 优先 `ort-dll\dml\`、`ort-dll\openvino\`
- CUDA / cuDNN：exe 同目录 `runtime\`、`cuda\`、`cudnn\` 会自动加入 PATH

### 预解压模型

把 `.tz` 转成明文 `.onnx`，启动时不再解密和解压：

```bash
gopaz prepare --model-dir .\models
gopaz prepare --model-dir .\models --force
```

转换后会得到同名 `.onnx`，程序优先使用它；`.tz` 保留作为回退。`prepare` 输出的是未打补丁的原始 ONNX，Patch 只在加载时于内存完成，不要手动预打补丁。模型更新后重新运行 `prepare`。明文 ONNX 只适合本机使用，不要分发。

## Web 界面

```bash
gopaz
```

不带参数启动时会监听 `http://127.0.0.1:8765` 并自动打开浏览器。端口被占用时自动顺延，也可以手动指定：

```bash
gopaz serve --port 9000 --no-open
```

使用方式：

- 点击中间的「选择视频」按钮，通过 Windows 原生文件选择器（GO-WinFilePicker）导入，直接把本地路径交给程序，不复制视频
- 选择模型、倍率、参数和输出设置，选择输出文件夹
- 左侧模型列表下方可以选择 Provider 和 Device，DML 设备会显示 DXGI 适配器名称，选择结果会同步到 CLI 命令
- 悬停模型按钮或参数标签会显示对应说明，中文界面使用对应翻译
- 点「开始处理」，底部显示进度、处理速度 fps、剩余时间和日志；可以取消当前任务
- 输出文件名默认自动生成，也可以点击文件名输入框改成自定义名字；留空恢复自动命名
- 界面默认英文，右上角可切换中文；只提供暗色主题

自动命名与 Topaz 一致，只提供文件夹路径：

```
clip_prob4.mp4        Proteus 输出
clip_ghq5.mp4         Gaia 输出
clip_gcg5.mp4         Gaia-CG 输出
clip_tvai.mp4         插帧输出
clip_1_prob4.mp4      同名文件已存在时的第二个输出
myvideo.mp4           自定义文件名
```

未指定输出文件夹时默认保存到输入视频所在目录。

浏览器拖拽无法获取本地文件的绝对路径，这是浏览器安全限制，所以 Web 界面不提供拖拽导入，请使用选择按钮。

### 播放与预览

- 中间播放器直接播放输入视频，支持播放/暂停和拖拽进度条
- 右侧模式切换为 Original / Processed；另有左右对比和分割对比，分割线可以左右拖动
- Preview 按钮从当前播放位置开始处理 2 秒，输入不足 2 秒时处理剩余部分
- 预览使用临时 H.264 MP4 以便浏览器播放，优先 NVENC，不可用时回退 libx264，不改变最终输出编码设置
- 预览文件保存在系统临时目录，下一次预览或服务退出时会清理旧文件
- 预览区间会在原始进度条上高亮；对比模式下的进度条显示预览片段时间轴，并标注对应的源区间
- 日志面板可展开，显示任务设置、完整命令、帧范围、分块、耗时和警告
- 输入视频需要浏览器支持的编码；如果原视频无法播放，仍可以查看处理后的预览

Web 界面一次只运行一个任务。CLI 的全部功能仍然可用。

## CLI 用法

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

慢放输出保持源 FPS，帧数和时长都是 N 倍，音频按同一倍率减速。`--audio-codec copy` 在慢放时会自动改为 AAC。指定 `--start`/`--end` 时，音轨按相同的源区间裁剪。

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
--provider cuda      # cuda / tensorrt / dml / openvino / cpu
--device 0           # DML 索引见 gopaz devices，OpenVINO 用 GPU.0/NPU/CPU
--gpu -2             # -2 自动 / -1 CPU / 0+ 设备号，适用于 CUDA/TensorRT
--threads 4          # 推理线程数
--overlap 0          # 分块重叠，0=自动（增强/Apollo 48，Aion 32）
--start 0 --end 99   # 只处理指定帧范围，音轨同步裁剪
```

### 推理后端

| Provider | 设备 | 说明 |
|---|---|---|
| `cuda` | NVIDIA GPU | 默认后端，使用 `ort-dll/onnxruntime.dll` |
| `tensorrt` | NVIDIA GPU | 使用 TensorRT 10，首次运行构建 engine，之后复用缓存 |
| `dml` | NVIDIA / Intel 核显 | 使用 DirectML 1.24.4，NPU 不支持；设备索引见 `gopaz devices` |
| `openvino` | Intel 核显 / NPU | GPU.0 支持全部模型；NPU 不支持 Aion |

TensorRT、DirectML 和 OpenVINO 当前强制单线程推理，`--workers` 和 `--instances` 会被限制为 1。OpenVINO NPU 上 Aion 会直接报错，建议换回 `GPU.0`。CUDA/TensorRT 的 GPU 设备仍用高级设置里的 GPU 选择；DirectML/OpenVINO 用左侧栏的 Device 选择。

### 首次运行与缓存

- TensorRT 首次为当前模型文件和 tile 尺寸构建 engine：固定块模型在会话创建时构建，Aion 这类动态尺寸模型在预热时构建。Aion 4K 首次通常需要 1-2 分钟，日志会分别显示 `TensorRT session ready` 或 `Warming up TensorRT engine` 的耗时。engine 按模型文件名和 tile 尺寸分开缓存，位于 `%LOCALAPPDATA%\Gopaz\tensorrt`；换分辨率或 `--gpu-vram` 会改变 tile 尺寸并重建。升级到带 cache prefix 的版本后旧缓存不会命中，首次运行会重建一次。
- OpenVINO NPU 在创建会话时编译模型，首次每个模型和尺寸可能需要数分钟，日志会在编译前提示；之后从 `%LOCALAPPDATA%\Gopaz\openvino` 加载。
- DirectML 的 `device_id` 按 DXGI 适配器顺序排列，`gopaz devices` 会显示实际名称。0 通常是默认显示适配器，不保证是核显。

## 环境变量

| 变量 | 说明 |
|---|---|
| `GOPAZ_MODEL_DIR` | 模型目录，包含 `.onnx` 或 `.tz` |
| `GOPAZ_ORT_DLL` | onnxruntime.dll 路径 |
| `GOPAZ_CUDNN_ALGO` | cuDNN 卷积算法 exhaustive、heuristic 或 default |
| `GOPAZ_DEBUG=1` | 输出诊断信息 |
| `GOPAZ_PROGRESS=json` | 以 JSON 行输出日志和进度，供 Web 界面读取 |

注意：Topaz 自己的 `TVAI_MODEL_DIR` 是元数据 JSON 目录，本工具的语义不同。

## 测试

```bash
go test .
```

端到端回归默认跳过，需要模型目录和 onnxruntime。未设置 `GOPAZ_MODEL_DIR` 或 `GOPAZ_ORT_DLL` 时会尝试 `portable/models`、`ort-dll/` 和 `portable/ort-dll/`：

```bash
GOPAZ_E2E=1 go test . -run E2E -v
GOPAZ_E2E=1 GOPAZ_E2E_GPU=1 GOPAZ_E2E_AION=1 go test . -run TestE2EAionDuplicate -v
```

## 声明

`.tz` 模型来自本机安装的 Topaz Video AI，仅用于本机/个人使用，请勿再分发。用 `gopaz prepare` 生成的明文 `.onnx` 同样不要分发。
