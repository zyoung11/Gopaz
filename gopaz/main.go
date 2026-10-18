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

func main() {
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
	case "models":
		cmdModels()
	case "encoders":
		cmdEncoders()
	case "help", "-h", "--help":
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

func usage() {
	fmt.Println(`gopaz - Topaz Video AI 本地复刻 CLI

用法:
  gopaz              启动本地 Web 界面
  gopaz serve        -port 8765 -no-open  仅启动 Web 界面
  gopaz upscale      -i <输入> -o <输出> --model <模型> --scale <1|2|4> [参数...]
  gopaz interpolate  -i <输入> -o <输出> --model <模型> --factor <N> [参数...]
  gopaz models       列出可用模型
  gopaz encoders     列出可用编码器

模型:
  超分:  proteus  gaia  gaia-cg
  插帧:  apollo  aion

超分参数 (proteus 支持，取值 -100~100):
  --denoise --detail --dehalo --sharpen --fix-compression --deblur

插帧参数:
  --factor  慢放倍数 (2,3,4,6,8,10,12,16；输出保持源 FPS，时长变为 N 倍)
  --sensitivity 10  重复帧替换灵敏度 0-100 (rdt=Sensitivity/1000，0 关闭)
  --rdt -1          原始 rdt 阈值覆盖 (-0.01~0.2，-1=用 sensitivity)

通用:
  --gpu     -2自动 -1 CPU 0+设备号 (默认 -2)
  --threads 推理线程数
  --overlap 分块重叠，0=自动 (增强/插帧 48, Aion 32)
  --kcolor  keepColor 色彩校正 0/1 (默认 1)
  --scene-threshold 场景切换阈值 (默认 6.5)
  --instances 0 插值并发会话数 (0=自动，默认 1 个共享会话)
  --workers 0   插值/增强并行 worker 数 (0=自动，默认 2)
  --gpu-vram 0  Aion tile 预算 GB (0=约 2.4M 像素；越小越省显存)
  --vram    每会话显存上限 GB (0=按 tile 自动)
  --start --end  处理帧范围
  --ffmpeg --ffprobe  可执行文件路径 (默认用 exe 同目录或 PATH 上的版本)`)
}

func cmdUpscale(args []string) error {
	fs := flag.NewFlagSet("upscale", flag.ExitOnError)
	in := fs.String("i", "", "输入视频")
	out := fs.String("o", "", "输出视频")
	mk := fs.String("model", "proteus", "模型")
	scale := fs.Int("scale", 1, "倍率 1/2/4")
	gpu := fs.Int("gpu", -2, "GPU 设备")
	threads := fs.Int("threads", 0, "线程数")
	workers := fs.Int("workers", 0, "并行 worker 数，0=自动")
	vram := fs.Float64("vram", 0, "每会话显存上限 GB，0=自动")
	overlap := fs.Int("overlap", 0, "分块重叠，0=自动")
	kcolor := fs.Int("kcolor", 1, "keepColor 色彩校正 0/1")
	sceneThreshold := fs.Float64("scene-threshold", 6.5, "场景切换阈值")
	ffmpeg := fs.String("ffmpeg", "", "ffmpeg 路径")
	ffprobe := fs.String("ffprobe", "", "ffprobe 路径")
	work := fs.String("workdir", "", "临时目录")
	start := fs.Int("start", -1, "起始帧")
	end := fs.Int("end", -1, "结束帧")
	encID := fs.String("encoder", "h265-main-win-nvidia", "视频编码器")
	quality := fs.String("quality", "High", "质量档 High/Mid/Low")
	qp := fs.Int("qp", 0, "直接指定 constqp")
	rc := fs.String("rate-control", "cq", "码率模式 cq/cbr/vbr")
	bitrate := fs.Int("bitrate", 0, "码率 Mbps")
	audioCodec := fs.String("audio-codec", "copy", "音频 copy/aac/ac3/pcm/vorbis/none")
	audioBitrate := fs.Int("audio-bitrate", 0, "音频码率 kbps")
	denoise := fs.Float64("denoise", 0, "降噪")
	detail := fs.Float64("detail", 0, "细节")
	dehalo := fs.Float64("dehalo", 0, "去光晕")
	sharpen := fs.Float64("sharpen", 0, "锐化")
	fixComp := fs.Float64("fix-compression", 0, "修复压缩")
	deblur := fs.Float64("deblur", 0, "去模糊")
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
		KColor: *kcolor != 0, SceneThreshold: *sceneThreshold,
		WorkDir: *work, StartFrame: *start, EndFrame: *end,
		Encode:    EncodeOptions{EncoderID: *encID, Quality: *quality, QP: *qp, RateControl: *rc, Bitrate: *bitrate},
		AudioMode: *audioCodec, AudioBitrate: *audioBitrate,
	}
	return RunUpscale(job)
}

func cmdInterpolate(args []string) error {
	fs := flag.NewFlagSet("interpolate", flag.ExitOnError)
	in := fs.String("i", "", "输入视频")
	out := fs.String("o", "", "输出视频")
	mk := fs.String("model", "apollo", "模型")
	factor := fs.Int("factor", 2, "慢放倍数")
	instances := fs.Int("instances", 0, "插值并发会话数，0=自动（默认 1 个共享会话）")
	workers := fs.Int("workers", 0, "插值并行 worker 数，0=自动")
	rdt := fs.Float64("rdt", -1, "原始去重阈值，-1=使用 --sensitivity")
	sensitivity := fs.Int("sensitivity", 10, "重复帧替换灵敏度 0-100，0 关闭")
	gpuVRAM := fs.Float64("gpu-vram", 0, "Aion tile 预算 GB，0=约 2.4M 像素")
	vram := fs.Float64("vram", 0, "每会话显存上限 GB，0=按 tile 自动")
	gpu := fs.Int("gpu", -2, "GPU 设备")
	threads := fs.Int("threads", 0, "线程数")
	overlap := fs.Int("overlap", 0, "分块重叠，0=自动")
	ffmpeg := fs.String("ffmpeg", "", "ffmpeg 路径")
	ffprobe := fs.String("ffprobe", "", "ffprobe 路径")
	work := fs.String("workdir", "", "临时目录")
	start := fs.Int("start", -1, "起始帧")
	end := fs.Int("end", -1, "结束帧")
	encID := fs.String("encoder", "h265-main-win-nvidia", "视频编码器")
	quality := fs.String("quality", "High", "质量档 High/Mid/Low")
	qp := fs.Int("qp", 0, "直接指定 constqp")
	rc := fs.String("rate-control", "cq", "码率模式 cq/cbr/vbr")
	bitrate := fs.Int("bitrate", 0, "码率 Mbps")
	audioCodec := fs.String("audio-codec", "copy", "音频 copy/aac/ac3/pcm/vorbis/none")
	audioBitrate := fs.Int("audio-bitrate", 0, "音频码率 kbps")
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
	job := InterpJob{
		Input: *in, Output: *out, Ffmpeg: ff, Ffprobe: fp,
		ModelDir: resolveModelDir(), Model: m, Factor: *factor,
		GPU: *gpu, Threads: *threads, Overlap: *overlap, WorkDir: *work,
		Instances: *instances, Workers: *workers, RDT: *rdt, Sensitivity: *sensitivity, VramGB: *vram, DeviceVRAMGB: *gpuVRAM,
		StartFrame: *start, EndFrame: *end,
		Encode:    EncodeOptions{EncoderID: *encID, Quality: *quality, QP: *qp, RateControl: *rc, Bitrate: *bitrate},
		AudioMode: *audioCodec, AudioBitrate: *audioBitrate,
	}
	return RunInterpolate(job)
}

func cmdModels() {
	reg := ModelRegistry()
	fmt.Printf("%-10s %-24s %-8s %-12s %s\n", "KEY", "DISPLAY", "KIND", "SCALES", "PARAMS")
	for _, k := range ModelKeys() {
		m := reg[k]
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
		for _, sub := range []string{"", "bin"} {
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
	cands = append(cands,
		`C:\zy\APP\Big_APPs\TopazVideoAI\models`,
		filepath.Join(os.Getenv("ProgramData"), `Topaz Labs LLC\Topaz Video AI\models`),
	)
	for _, c := range cands {
		if st, err := os.Stat(c); err == nil && st.IsDir() {
			return c
		}
	}
	return cands[0]
}
