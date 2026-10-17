// tvai 是 Topaz Video AI 的本地复刻 CLI：超分/修复与插帧/慢放。
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"tvai/internal/encode"
	"tvai/internal/log"
	"tvai/internal/model"
	"tvai/internal/pipeline"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "upscale":
		err = cmdUpscale(os.Args[2:])
	case "interpolate":
		err = cmdInterpolate(os.Args[2:])
	case "models":
		cmdModels()
	case "encoders":
		cmdEncoders()
	case "help", "-h", "--help":
		usage()
	default:
		log.Errorf("未知子命令 %q", os.Args[1])
		usage()
		os.Exit(2)
	}
	if err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}
}

func usage() {
	fmt.Println(`tvai - Topaz Video AI 本地复刻 CLI

用法:
  tvai upscale      -i <输入> -o <输出> --model <模型> --scale <1|2|4> [参数...]
  tvai interpolate  -i <输入> -o <输出> --model <模型> --factor <N> [参数...]
  tvai models       列出可用模型
  tvai encoders     列出可用编码器

模型:
  超分:  proteus(可调参数)  gaia  gaia-cg
  插帧:  apollo  aion

超分参数 (proteus 支持，取值 0-100，deblur 为 -100~100):
  --denoise --detail --dehalo --sharpen --fix-compression --deblur

插帧参数:
  --factor  慢放倍数 (2,3,4,6,8,10,12,16)
  --rdt     去重阈值 (-0.01~0.2, 默认 0.01)

通用:
  --gpu     -2自动 -1 CPU 0+设备号 (默认 -2)
  --threads 推理线程数
  --overlap 分块重叠像素 (默认 16)
  --start --end  处理帧范围
  --ffmpeg --ffprobe  可执行文件路径 (默认从 PATH 或 Topaz 目录)`)
}

func cmdUpscale(args []string) error {
	fs := flag.NewFlagSet("upscale", flag.ExitOnError)
	in := fs.String("i", "", "输入视频")
	out := fs.String("o", "", "输出视频")
	mk := fs.String("model", "proteus", "模型")
	scale := fs.Int("scale", 1, "倍率 1/2/4")
	gpu := fs.Int("gpu", -2, "GPU 设备")
	threads := fs.Int("threads", 0, "线程数")
	overlap := fs.Int("overlap", 16, "重叠")
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
		return fmt.Errorf("必须指定 -i 和 -o")
	}
	m, err := model.Get(*mk)
	if err != nil {
		return err
	}
	if m.Kind != model.KindEnhance {
		return fmt.Errorf("模型 %s 不是超分模型", *mk)
	}
	if !containsInt(m.Scales, *scale) {
		return fmt.Errorf("模型 %s 不支持倍率 %d，可用 %v", *mk, *scale, m.Scales)
	}
	ff, fp := resolveFfmpeg(*ffmpeg, *ffprobe)
	params := map[string]float64{
		"denoise": *denoise, "detail": *detail, "dehalo": *dehalo,
		"sharpen": *sharpen, "fix-compression": *fixComp, "deblur": *deblur,
	}
	// 校验模型支持的参数，不支持的丢弃并提示。
	for k := range params {
		if !m.HasParam(k) {
			delete(params, k)
		}
	}
	job := pipeline.UpscaleJob{
		Input: *in, Output: *out, Ffmpeg: ff, Ffprobe: fp,
		ModelDir: resolveModelDir(), Model: m, Scale: *scale,
		Params: params, GPU: *gpu, Threads: *threads, Overlap: *overlap,
		WorkDir: *work, StartFrame: *start, EndFrame: *end,
		Encode: encode.Options{EncoderID: *encID, Quality: *quality, QP: *qp, RateControl: *rc, Bitrate: *bitrate},
		AudioMode: *audioCodec, AudioBitrate: *audioBitrate,
	}
	return pipeline.RunUpscale(job)
}

func cmdInterpolate(args []string) error {
	fs := flag.NewFlagSet("interpolate", flag.ExitOnError)
	in := fs.String("i", "", "输入视频")
	out := fs.String("o", "", "输出视频")
	mk := fs.String("model", "apollo", "模型")
	factor := fs.Int("factor", 2, "慢放倍数")
	gpu := fs.Int("gpu", -2, "GPU 设备")
	threads := fs.Int("threads", 0, "线程数")
	overlap := fs.Int("overlap", 16, "重叠")
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
		return fmt.Errorf("必须指定 -i 和 -o")
	}
	m, err := model.Get(*mk)
	if err != nil {
		return err
	}
	if m.Kind != model.KindInterp {
		return fmt.Errorf("模型 %s 不是插帧模型", *mk)
	}
	ff, fp := resolveFfmpeg(*ffmpeg, *ffprobe)
	job := pipeline.InterpJob{
		Input: *in, Output: *out, Ffmpeg: ff, Ffprobe: fp,
		ModelDir: resolveModelDir(), Model: m, Factor: *factor,
		GPU: *gpu, Threads: *threads, Overlap: *overlap, WorkDir: *work,
		StartFrame: *start, EndFrame: *end,
		Encode: encode.Options{EncoderID: *encID, Quality: *quality, QP: *qp, RateControl: *rc, Bitrate: *bitrate},
		AudioMode: *audioCodec, AudioBitrate: *audioBitrate,
	}
	return pipeline.RunInterpolate(job)
}

func cmdModels() {
	reg := model.Registry()
	fmt.Printf("%-10s %-24s %-8s %-12s %s\n", "KEY", "DISPLAY", "KIND", "SCALES", "PARAMS")
	for _, k := range model.Keys() {
		m := reg[k]
		kind := "enhance"
		if m.Kind == model.KindInterp {
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
	fmt.Println("视频编码器 (encoder id → 说明):")
	for _, e := range encodersList() {
		fmt.Printf("  %-26s %s\n", e.ID, e.Desc)
	}
}

type encoderInfo struct{ ID, Desc string }

func encodersList() []encoderInfo {
	return []encoderInfo{
		{"h264-main-win-nvidia", "H.264 High, NVENC, 8bit, ≤4096x4096"},
		{"h265-main-win-nvidia", "H.265 Main, NVENC, 8bit, ≤8192x8192"},
		{"h265-main10-win-nvidia", "H.265 Main10, NVENC, 10bit, ≤8192x8192"},
		{"av1-win-nvidia", "AV1, NVENC, 8bit"},
		{"av1-10bit-win-nvidia", "AV1, NVENC, 10bit"},
		{"prores-422-hq-win", "ProRes 422 HQ, 10bit, mov"},
		{"prores-422-std-win", "ProRes 422, 10bit, mov"},
		{"ffv1-10bit-444", "FFV1 无损, 10bit 4:4:4"},
		{"vp9-best", "VP9 best, 软编"},
	}
}

func containsInt(s []int, v int) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func resolveFfmpeg(ff, fp string) (string, string) {
	if ff == "" {
		ff = "ffmpeg"
	}
	if fp == "" {
		fp = "ffprobe"
	}
	return ff, fp
}

func resolveModelDir() string {
	if d := os.Getenv("TVAI_MODEL_DIR"); d != "" {
		return d
	}
	// 默认探测常见位置。
	cands := []string{
		`C:\zy\APP\Big_APPs\TopazVideoAI\models`,
		filepath.Join(os.Getenv("ProgramData"), `Topaz Labs LLC\Topaz Video AI\models`),
	}
	for _, c := range cands {
		if st, err := os.Stat(c); err == nil && st.IsDir() {
			return c
		}
	}
	return cands[0]
}
