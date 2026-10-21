# Gopaz

Reverse-engineered local inference for Topaz Video AI in pure Go.

[中文](README_CN.md)

<p align="center">
  <img src="images/web-ui.jpg" alt="Gopaz web UI showing an Aion 10x side-by-side comparison" width="100%">
</p>
<p align="center"><sub>Aion 10x slow motion with side-by-side comparison and live logs</sub></p>

Gopaz runs Topaz Video AI models locally on Windows. The core is pure Go: no cgo, no C compiler and no Python. The ONNX Runtime C API is bound directly through `syscall`, and video I/O uses ffmpeg subprocesses.

## Features

- **Local web UI** — import a video, preview a short range, compare the original and processed video side by side or with a divider, loop playback, and follow progress, speed, ETA and live logs.
- **Enhancement** — Proteus with six parameters and automatic per-frame parameter prediction; Gaia and Gaia-CG for live action and computer-generated content. 1x, 2x and 4x.
- **Interpolation** — Apollo and Aion, from 2x to 16x slow motion, with scene-aware windowing and duplicate-frame replacement.
- **Backends** — CUDA, TensorRT and DirectML on GPU, OpenVINO on GPU or NPU, plus CPU fallback. The inference backend is independent of the video encoder.
- **Encoding** — NVIDIA NVENC and Intel Quick Sync H.264/H.265, plus software H.264, VP9 and FFV1; audio copy, AAC, AC3, PCM or Vorbis. Slow-motion audio is retimed automatically.
- **Portable** — one executable plus optional runtime and model packs. Missing backends and models are simply hidden from the UI; add the folders and refresh.
- **Persistence** — model, per-model parameters, backend, encoder, output options and the log panel state are restored from `config.json`.

## Models

| Model | Task | Scale or factor | Notes |
|---|---|---|---|
| Proteus | Enhance | 1x / 2x / 4x | Six parameters plus Auto parameter estimation |
| Gaia | Enhance | 1x / 2x / 4x | High quality, live action |
| Gaia-CG | Enhance | 1x / 2x / 4x | Computer-generated content |
| Apollo | Interpolate | 2x–16x | Up to 8 subframes per inference |
| Aion | Interpolate | 2x–16x | 16 subframes, two-stage inference |

Gopaz implements these five models. Other Topaz models such as Nyx, Iris, Rhea, Theia and Artemis are not yet supported. See [ARCHITECTURE.md](ARCHITECTURE.md) for the inference pipeline.

## Download

Grab `gopaz.exe` and the packs you need from [Releases](https://github.com/zyoung11/Gopaz/releases/latest). There is no installer; build the portable folder as shown below.

| Asset | Location | Required for |
|---|---|---|
| `gopaz.exe` | Gopaz folder | everything |
| `ffmpeg.zip` | Gopaz folder | everything |
| `CUDA.zip` | `runtime/NVIDIA/` | CUDA, TensorRT and CPU |
| `Compute.zip` | `runtime/NVIDIA/` | CUDA and TensorRT; optional when the CUDA Toolkit and cuDNN are on `PATH` |
| `TensorRT.zip` | `runtime/NVIDIA/` | TensorRT; optional when TensorRT is on `PATH` |
| `DirectML.zip` | `runtime/` | DirectML |
| `OpenVINO.zip` | `runtime/` | OpenVINO |
| `proteus.zip` | `models/` | Proteus |
| `gaia-1.zip` | `models/` | Gaia-CG |
| `gaia-2.zip` | `models/` | Gaia |
| `apollo-1.zip`, `apollo-2.zip` | `models/` | Apollo; both cover all tile sizes |
| `aion.zip` | `models/` | Aion |

CUDA also needs the NVIDIA driver. `CUDA.zip` contains the ONNX Runtime build with the CUDA and TensorRT execution providers; `Compute.zip` contains the CUDA math libraries and cuDNN. TensorRT, DirectML and OpenVINO each add one runtime folder. The CPU provider can use any of these ONNX Runtime builds.

The folder layout after extraction:

```
Gopaz/
├─ gopaz.exe
├─ config.json              generated on first run
├─ ffmpeg/
│  ├─ ffmpeg.exe
│  └─ ffprobe.exe
├─ models/
│  ├─ proteus/
│  ├─ gaia/
│  ├─ apollo/
│  └─ aion/
├─ runtime/
│  ├─ NVIDIA/
│  │  ├─ CUDA/
│  │  ├─ Compute/
│  │  └─ TensorRT/
│  ├─ DirectML/
│  └─ OpenVINO/
└─ cache/                   generated, safe to delete
```

## Quick start

1. Put `gopaz.exe` and extract `ffmpeg.zip` into an empty folder.
2. Add a runtime, for example `CUDA.zip` and `Compute.zip` into `runtime/NVIDIA/`.
3. Extract the model packs you need into `models/`.
4. Run `gopaz.exe`. The web UI opens at `http://127.0.0.1:8765`.

Select a video, choose a model and settings, then start. Use **Preview** to render a short range at the current playback position, and **Logs** to follow the pipeline.

## Usage

### Web UI

```bash
gopaz
gopaz serve --port 9000 --no-open
```

The UI provides native file and folder pickers, per-model settings, encoder and audio options, frame ranges, task cancel, progress with speed and ETA, live logs, and preview/compare modes. One task runs at a time; the CLI remains available.

Settings, per-model parameters and the log panel state are saved to `config.json` next to `gopaz.exe`. Selecting a model restores its own parameters.

### CLI

| Command | Purpose |
|---|---|
| `gopaz upscale` | Upscale or restore a video |
| `gopaz interpolate` | Slow motion or frame interpolation |
| `gopaz prepare` | Convert `.tz` models to plain `.onnx` |
| `gopaz models` | List installed models |
| `gopaz encoders` | List available encoders |
| `gopaz devices` | List CUDA, DirectML and OpenVINO devices |
| `gopaz help` | Show all options |
| `gopaz -v` | Show the version |

Examples:

```bash
gopaz upscale -i input.mp4 -o output.mp4 --model proteus --scale 2 --encoder h264-main-sw
gopaz interpolate -i input.mp4 -o output.mp4 --model aion --factor 4 --sensitivity 65
```

Proteus parameters are `--deblur`, `--denoise`, `--detail`, `--dehalo`, `--sharpen` and `--fix-compression`, each ranging from -100 to 100. `--auto` lets the parameter model choose them per frame; manual values are ignored. Interpolation uses `--factor`, `--sensitivity` and `--rdt`.

Common options:

| Option | Description |
|---|---|
| `--provider cuda / tensorrt / dml / openvino / cpu` | Inference backend |
| `--device <id>` | Device index, or OpenVINO `GPU.0` / `GPU.1` / `NPU` / `CPU` |
| `--video-device sw / cuda / qsv` | Video decoder: software, NVDEC or Quick Sync |
| `--encoder <id>` | Encoder from `gopaz encoders`; auto by default |
| `--quality`, `--rate-control`, `--bitrate`, `--qp` | Encoding quality and rate control |
| `--start`, `--end` | Process a frame range |
| `--ffmpeg`, `--ffprobe` | Override tool paths |

Run `gopaz help` for the full list, including threads, workers, VRAM limits, tiling overlap and audio options.

Output files are named `<input>_<model tag>` by default, for example `_prob4`, `_ghq5`, `_gcg5`, `_apo8` or `_aion1`. Existing files get a `(N)` suffix.

## Configuration

`config.json` is created next to the executable on first run.

| Option | Default | Description |
|---|---|---|
| `saveState` | `true` | Persist UI state |
| `restoreVideo` | `true` | Reload the last video on startup |
| `previewDuration` | `2` | Source seconds rendered by Preview |
| `port` | `8765` | Web port; `--port` overrides it |

The `state` object stores the current model, backend and device, encoder, output settings, language, per-model parameters, and the log panel's open state. Use the UI settings; manual editing is not required.

Environment variables:

| Variable | Description |
|---|---|
| `GOPAZ_MODEL_DIR` | Model directory |
| `GOPAZ_ORT_DLL` | Path to `onnxruntime.dll` |
| `GOPAZ_CACHE_DIR` | Cache directory; relative paths use the working directory |
| `GOPAZ_CONFIG` | Override the `config.json` path |
| `GOPAZ_CUDNN_ALGO` | cuDNN convolution algorithm: `exhaustive`, `heuristic` or `default` |
| `GOPAZ_DEBUG=1` | Diagnostic logs |
| `GOPAZ_PROGRESS=json` | JSON-lines logs and progress |

TensorRT engines and OpenVINO compiled models are cached under `cache/tensorrt/` and `cache/openvino/`. The first run for a model and tile shape can take minutes; later runs reuse the cache. Delete `cache/` to rebuild. Preview videos live in `cache/preview/` and are cleared when the web UI starts.

## Building from source

```bash
git clone https://github.com/zyoung11/Gopaz.git
cd Gopaz/gopaz
go build -ldflags="-s -w" .
go test ./...
```

The web UI is embedded with `//go:embed`; no Node.js or separate frontend build is needed. Optional end-to-end tests need ffmpeg, model files and ONNX Runtime:

```bash
GOPAZ_E2E=1 go test . -run E2E -v
```

## Limitations

- Windows x64 only.
- The pipeline is RGB24 end to end; 10-bit and YUV output are not implemented.
- Apollo 10/12/16 subframe reuse and exact Topaz duplicate-frame boundaries are approximations.
- TensorRT and DirectML use one session and one worker; OpenVINO supports concurrent runs.
- Performance depends on the GPU, driver, runtime version and model size.

See [ARCHITECTURE.md](ARCHITECTURE.md) for backends, tiling, color handling, failure modes and known limits.

## License

Code is released under the [MIT License](LICENSE).

Gopaz is not affiliated with Topaz Labs. Model files, including `.tz` files and `.onnx` files produced by `gopaz prepare`, belong to Topaz and are not covered by this license; use them only on your own machine and do not redistribute them.
