// gopaz is a local reimplementation of Topaz Video AI CLI for upscaling and frame interpolation.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// version is the CLI version.
const version = "v0.1.0"

// globalFlagAction classifies global help and version flags anywhere in the CLI args.
func globalFlagAction(args []string) string {
	for _, a := range args {
		switch a {
		case "-h", "--help":
			return "help"
		case "-v", "--version", "-V":
			return "version"
		}
	}
	return ""
}

// printVersion prints the CLI version.
func printVersion() {
	fmt.Println(wrap(cBlue, "gopaz") + " " + version)
}

func main() {
	switch globalFlagAction(os.Args[1:]) {
	case "help":
		usage()
		return
	case "version":
		printVersion()
		return
	}
	if len(os.Args) < 2 {
		if err := cmdServe(nil); err != nil {
			Error(err.Error())
			os.Exit(1)
		}
		return
	}
	var err error
	switch os.Args[1] {
	case "upscale":
		err = cmdUpscale(os.Args[2:])
	case "interpolate":
		err = cmdInterpolate(os.Args[2:])
	case "serve":
		err = cmdServe(os.Args[2:])
	case "prepare":
		err = cmdPrepare(os.Args[2:])
	case "models":
		cmdModels()
	case "encoders":
		cmdEncoders()
	case "devices":
		cmdDevices()
	case "help":
		usage()
	default:
		Errorf("unknown subcommand %q", os.Args[1])
		usage()
		os.Exit(2)
	}
	if err != nil {
		Error(err.Error())
		os.Exit(1)
	}
}

const usageLabelWidth = 30

func helpHeader(text string) {
	fmt.Println(wrap(cBold+cMauve, text))
}

func helpPadded(rendered string, visible int, desc string) {
	pad := max(usageLabelWidth-visible, 0)
	fmt.Printf("  %s%s%s\n", rendered, strings.Repeat(" ", pad), desc)
}

func helpCommand(label, desc string) {
	rendered := wrap(cBlue, label)
	if i := strings.IndexByte(label, ' '); i > 0 {
		rendered = wrap(cBlue, label[:i]) + wrap(cGreen, label[i:])
	}
	helpPadded(rendered, len(label), desc)
}

func helpOption(label, desc string) {
	helpPadded(wrap(cYellow, label), len(label), desc)
}

func helpModel(label, desc string) {
	helpPadded(wrap(cGreen, label), len(label), desc)
}

func usage() {
	fmt.Println(wrap(cBlue, "gopaz") + " " + version + " - local Topaz Video AI CLI")
	fmt.Println()
	helpHeader("Usage:")
	helpCommand("gopaz", "Start the local web UI")
	helpCommand("gopaz serve", "Start the web UI only")
	helpCommand("gopaz upscale", "Upscale or restore a video")
	helpCommand("gopaz interpolate", "Slow motion or frame interpolation")
	helpCommand("gopaz prepare", "Convert .tz models to plain .onnx")
	helpCommand("gopaz models", "List available models")
	helpCommand("gopaz encoders", "List available encoders")
	helpCommand("gopaz devices", "List CUDA, DirectML, and OpenVINO devices")
	helpCommand("gopaz help", "Show this help; also -h, --help")
	helpCommand("gopaz -v", "Show version; also --version, -V")
	fmt.Println()
	helpHeader("Models:")
	helpModel("enhancement", "proteus, gaia, gaia-cg")
	helpModel("interpolation", "apollo, aion")
	fmt.Println()
	helpHeader("Enhancement parameters (Proteus only, -100 to 100):")
	helpOption("--denoise", "Reduce noise")
	helpOption("--detail", "Recover details")
	helpOption("--dehalo", "Remove halos")
	helpOption("--sharpen", "Sharpen")
	helpOption("--fix-compression", "Fix compression artifacts")
	helpOption("--deblur", "Deblur")
	fmt.Println()
	helpHeader("Interpolation parameters:")
	helpOption("--factor <N>", "Slow motion factor 2,3,4,6,8,10,12,16; source FPS is kept and duration becomes N times")
	helpOption("--sensitivity <n>", "Duplicate replacement sensitivity 0-100; rdt = sensitivity/500000, 0 disables")
	helpOption("--rdt <v>", "Raw rdt override -0.01 to 0.2; -1 uses sensitivity")
	fmt.Println()
	helpHeader("Common options:")
	helpOption("--gpu <id>", "-2 auto, -1 CPU, 0+ device index fallback when --device is omitted (default -2)")
	helpOption("--provider <name>", "cuda, tensorrt, dml, openvino (default cuda)")
	helpOption("--device <id>", "CUDA/TensorRT/DML device index from gopaz devices, or OpenVINO GPU.0/NPU/CPU")
	helpOption("--threads <n>", "Inference thread count")
	helpOption("--overlap <px>", "Block overlap, 0 = auto (enhancement/interp 48, Aion 32)")
	helpOption("--kcolor <0|1>", "keepColor correction (default 1)")
	helpOption("--scene-threshold <v>", "Scene change threshold (default 6.5)")
	helpOption("--instances <n>", "Interpolation sessions, 0 = auto (one shared session)")
	helpOption("--workers <n>", "Interpolation/enhancement workers, 0 = auto (OpenVINO enhancement 4, Aion/NPU 1)")
	helpOption("--gpu-vram <GB>", "Aion tile budget in GB, 0 = about 2.4M pixels")
	helpOption("--vram <GB>", "Per-session VRAM limit in GB, 0 = auto per tile")
	helpOption("--start <frame>", "First frame to process")
	helpOption("--end <frame>", "Last frame to process")
	helpOption("--ffmpeg <path>", "ffmpeg path (defaults to the executable directory or PATH)")
	helpOption("--ffprobe <path>", "ffprobe path (defaults to the executable directory or PATH)")
	fmt.Println()
	helpHeader("Output options:")
	helpOption("--encoder <id>", "Video encoder from gopaz encoders (default auto)")
	helpOption("--quality <name>", "Quality preset High, Mid, Low (default High)")
	helpOption("--qp <n>", "Constant QP override")
	helpOption("--rate-control <mode>", "Rate control cq, cbr, vbr (default cq)")
	helpOption("--bitrate <Mbps>", "Bitrate for cbr/vbr")
	helpOption("--audio-codec <name>", "Audio codec copy, aac, ac3, pcm, vorbis, none (default copy)")
	helpOption("--audio-bitrate <kbps>", "Audio bitrate (default 320)")
}

func cmdUpscale(args []string) error {
	fs := flag.NewFlagSet("upscale", flag.ExitOnError)
	in := fs.String("i", "", "input video")
	out := fs.String("o", "", "output video")
	mk := fs.String("model", "proteus", "model name")
	scale := fs.Int("scale", 1, "scale factor 1/2/4")
	gpu := fs.Int("gpu", -2, "GPU device")
	provider := fs.String("provider", "cuda", "inference provider")
	device := fs.String("device", "", "provider device")
	videoDevice := fs.String("video-device", "sw", "video decode acceleration sw, cuda or qsv")
	auto := fs.Bool("auto", false, "let the AI model pick Proteus parameters per frame")
	threads := fs.Int("threads", 0, "inference thread count")
	workers := fs.Int("workers", 0, "parallel workers, 0 = auto")
	vram := fs.Float64("vram", 0, "per-session VRAM limit in GB, 0 = auto")
	overlap := fs.Int("overlap", 0, "block overlap, 0 = auto")
	kcolor := fs.Int("kcolor", 1, "keepColor correction 0/1")
	sceneThreshold := fs.Float64("scene-threshold", 6.5, "scene change threshold")
	ffmpeg := fs.String("ffmpeg", "", "ffmpeg path")
	ffprobe := fs.String("ffprobe", "", "ffprobe path")
	start := fs.Int("start", -1, "first frame")
	end := fs.Int("end", -1, "last frame")
	encID := fs.String("encoder", "auto", "video encoder, auto selects NVENC or software")
	quality := fs.String("quality", "High", "quality preset High/Mid/Low")
	qp := fs.Int("qp", 0, "constant QP override")
	rc := fs.String("rate-control", "cq", "rate control cq/cbr/vbr")
	bitrate := fs.Int("bitrate", 0, "bitrate in Mbps")
	audioCodec := fs.String("audio-codec", "copy", "audio codec copy/aac/ac3/pcm/vorbis/none")
	audioBitrate := fs.Int("audio-bitrate", 0, "audio bitrate in kbps")
	denoise := fs.Float64("denoise", 0, "reduce noise")
	detail := fs.Float64("detail", 0, "recover details")
	dehalo := fs.Float64("dehalo", 0, "remove halos")
	sharpen := fs.Float64("sharpen", 0, "sharpen")
	fixComp := fs.Float64("fix-compression", 0, "fix compression artifacts")
	deblur := fs.Float64("deblur", 0, "deblur")
	fs.Parse(args)

	if *in == "" || *out == "" {
		return fmt.Errorf("-i and -o are required")
	}
	m, err := GetModel(*mk)
	if err != nil {
		return err
	}
	if m.Kind != KindEnhance {
		return fmt.Errorf("model %s is not an enhancement model", *mk)
	}
	if !containsInt(m.Scales, *scale) {
		return fmt.Errorf("model %s does not support scale %d; available: %v", *mk, *scale, m.Scales)
	}
	ff, fp := resolveFfmpeg(*ffmpeg, *ffprobe)
	if *encID == "auto" {
		*encID = defaultEncoder(ff)
	}
	params := map[string]float64{
		"denoise": *denoise, "detail": *detail, "dehalo": *dehalo,
		"sharpen": *sharpen, "fix-compression": *fixComp, "deblur": *deblur,
	}
	for k := range params {
		if !m.HasParam(k) {
			delete(params, k)
		}
	}
	job := UpscaleJob{
		Input: *in, Output: *out, Ffmpeg: ff, Ffprobe: fp,
		ModelDir: resolveModelDir(), Model: m, Scale: *scale,
		Params: params, GPU: *gpu, Threads: *threads, Workers: *workers, VramGB: *vram, Overlap: *overlap,
		Auto:     *auto,
		Provider: Provider(*provider), Device: *device, VideoDevice: *videoDevice,
		KColor: *kcolor != 0, SceneThreshold: *sceneThreshold,
		StartFrame: *start, EndFrame: *end,
		Encode:    EncodeOptions{EncoderID: *encID, Quality: *quality, QP: *qp, RateControl: *rc, Bitrate: *bitrate},
		AudioMode: *audioCodec, AudioBitrate: *audioBitrate,
	}
	return RunUpscale(job)
}

func cmdInterpolate(args []string) error {
	fs := flag.NewFlagSet("interpolate", flag.ExitOnError)
	in := fs.String("i", "", "input video")
	out := fs.String("o", "", "output video")
	mk := fs.String("model", "apollo", "model name")
	factor := fs.Int("factor", 2, "slow motion factor")
	instances := fs.Int("instances", 0, "interpolation sessions, 0 = auto (one shared session)")
	workers := fs.Int("workers", 0, "interpolation workers, 0 = auto")
	rdt := fs.Float64("rdt", -1, "raw duplicate threshold, -1 uses --sensitivity")
	sensitivity := fs.Int("sensitivity", 10, "duplicate replacement sensitivity 0-100, 0 disables")
	gpuVRAM := fs.Float64("gpu-vram", 0, "Aion tile budget in GB, 0 = about 2.4M pixels")
	vram := fs.Float64("vram", 0, "per-session VRAM limit in GB, 0 = auto per tile")
	gpu := fs.Int("gpu", -2, "GPU device")
	provider := fs.String("provider", "cuda", "inference provider")
	device := fs.String("device", "", "provider device")
	videoDevice := fs.String("video-device", "sw", "video decode acceleration sw, cuda or qsv")
	threads := fs.Int("threads", 0, "inference thread count")
	overlap := fs.Int("overlap", 0, "block overlap, 0 = auto")
	ffmpeg := fs.String("ffmpeg", "", "ffmpeg path")
	ffprobe := fs.String("ffprobe", "", "ffprobe path")
	start := fs.Int("start", -1, "first frame")
	end := fs.Int("end", -1, "last frame")
	encID := fs.String("encoder", "auto", "video encoder, auto selects NVENC or software")
	quality := fs.String("quality", "High", "quality preset High/Mid/Low")
	qp := fs.Int("qp", 0, "constant QP override")
	rc := fs.String("rate-control", "cq", "rate control cq/cbr/vbr")
	bitrate := fs.Int("bitrate", 0, "bitrate in Mbps")
	audioCodec := fs.String("audio-codec", "copy", "audio codec copy/aac/ac3/pcm/vorbis/none")
	audioBitrate := fs.Int("audio-bitrate", 0, "audio bitrate in kbps")
	fs.Parse(args)
	if *in == "" || *out == "" {
		return fmt.Errorf("-i and -o are required")
	}
	m, err := GetModel(*mk)
	if err != nil {
		return err
	}
	if m.Kind != KindInterp {
		return fmt.Errorf("model %s is not an interpolation model", *mk)
	}
	ff, fp := resolveFfmpeg(*ffmpeg, *ffprobe)
	if *encID == "auto" {
		*encID = defaultEncoder(ff)
	}
	job := InterpJob{
		Input: *in, Output: *out, Ffmpeg: ff, Ffprobe: fp,
		ModelDir: resolveModelDir(), Model: m, Factor: *factor,
		GPU: *gpu, Threads: *threads, Overlap: *overlap,
		Provider: Provider(*provider), Device: *device, VideoDevice: *videoDevice,
		Instances: *instances, Workers: *workers, RDT: *rdt, Sensitivity: *sensitivity, VramGB: *vram, DeviceVRAMGB: *gpuVRAM,
		StartFrame: *start, EndFrame: *end,
		Encode:    EncodeOptions{EncoderID: *encID, Quality: *quality, QP: *qp, RateControl: *rc, Bitrate: *bitrate},
		AudioMode: *audioCodec, AudioBitrate: *audioBitrate,
	}
	return RunInterpolate(job)
}

func cmdModels() {
	reg := ModelRegistry()
	modelDir := resolveModelDir()
	fmt.Printf("%-10s %-24s %-8s %-12s %s\n", "KEY", "DISPLAY", "KIND", "SCALES", "PARAMS")
	for _, k := range ModelKeys() {
		m := reg[k]
		if !m.Available(modelDir) {
			continue
		}
		kind := "enhance"
		if m.Kind == KindInterp {
			kind = "interp"
		}
		scales := "-"
		if len(m.Scales) > 0 {
			var s []string
			for _, v := range m.Scales {
				s = append(s, strconv.Itoa(v))
			}
			scales = strings.Join(s, ",")
		}
		params := "-"
		if len(m.Params) > 0 {
			var s []string
			for _, p := range m.Params {
				s = append(s, p.Key)
			}
			params = strings.Join(s, ",")
		}
		fmt.Printf("%-10s %-24s %-8s %-12s %s\n", m.Key, m.Display, kind, scales, params)
	}
}

func cmdEncoders() {
	fmt.Println("Video encoders (id -> description):")
	reg := EncoderRegistry()
	for _, id := range EncoderIDs() {
		e := reg[id]
		fmt.Printf("  %-26s %s\n", e.ID, e.Desc)
	}
}

func cmdDevices() {
	printDeviceSection("CUDA / TensorRT devices (index -> name):", cudaDevices(), "NVIDIA driver (nvcuda.dll) not found")
	printDeviceSection("DML / DirectML devices (DXGI order):", dmlDevices(), "no DXGI hardware adapters found")
	printDeviceSection("OpenVINO devices (id -> name):", openVINODevices(), "OpenVINO runtime (ort-dll/openvino) not found")
}

// printDeviceSection prints one provider section of gopaz devices.
func printDeviceSection(title string, devices []deviceInfo, empty string) {
	fmt.Println(title)
	if len(devices) == 0 {
		fmt.Printf("  (%s)\n", empty)
		return
	}
	for _, d := range devices {
		if d.Display == d.ID {
			fmt.Printf("  %s\n", d.ID)
			continue
		}
		fmt.Printf("  %-6s %s\n", d.ID, d.Display)
	}
}

func containsInt(s []int, v int) bool {
	return slices.Contains(s, v)
}

func resolveFfmpeg(ff, fp string) (string, string) {
	if ff == "" {
		ff = localTool("ffmpeg")
	}
	if fp == "" {
		fp = localTool("ffprobe")
	}
	return ff, fp
}

func localTool(name string) string {
	dirs := make([]string, 0, 2)
	if exe, err := os.Executable(); err == nil {
		dirs = append(dirs, filepath.Dir(exe))
	}
	if wd, err := os.Getwd(); err == nil {
		dirs = append(dirs, wd)
	}
	for _, d := range dirs {
		for _, sub := range []string{"", "bin", "ffmpeg"} {
			for _, ext := range []string{".exe", ""} {
				p := filepath.Join(d, sub, name+ext)
				if st, err := os.Stat(p); err == nil && !st.IsDir() {
					return p
				}
			}
		}
	}
	return name
}

// resolveModelDir returns the first existing model directory. The portable
// layout expects models/ next to the executable; GOPAZ_MODEL_DIR overrides it.
func resolveModelDir() string {
	var cands []string
	if d := os.Getenv("GOPAZ_MODEL_DIR"); d != "" {
		cands = append(cands, d)
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		cands = append(cands, filepath.Join(dir, "models"), filepath.Join(dir, "..", "models"))
	}
	if wd, err := os.Getwd(); err == nil {
		cands = append(cands, filepath.Join(wd, "models"))
	}
	for _, c := range cands {
		if st, err := os.Stat(c); err == nil && st.IsDir() {
			return c
		}
	}
	return cands[0]
}
