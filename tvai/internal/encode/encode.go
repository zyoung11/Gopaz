// Package encode 描述可用的视频编码器及其 ffmpeg 参数，用于构造编码命令行。
// 参数取自 Topaz 的 video-encoders.json 元数据。
package encode

import (
	"fmt"
	"strings"
)

// Encoder 描述一个视频编码器。
type Encoder struct {
	ID          string
	Desc        string
	Base        []string   // 基础视频编码参数（ffmpegOpts）
	Extensions  []string   // 允许的容器扩展名
	CQP         map[string]int // 质量档位 (High/Mid/Low) -> constqp 值
	HasCQP      bool       // 是否支持 constqp (CQ) 模式
	RateControl bool       // 是否支持 cbr/vbr 码率控制
}

// Registry 返回全部编码器，按 ID 索引。
func Registry() map[string]*Encoder {
	return map[string]*Encoder{
		"h264-main-win-nvidia": {
			ID: "h264-main-win-nvidia", Desc: "H.264 High, NVENC, 8bit",
			Base: strings.Fields("-c:v h264_nvenc -profile:v high -pix_fmt yuv420p -g 30"),
			Extensions: []string{"mov", "mkv", "mp4"},
			CQP:        map[string]int{"High": 18, "Mid": 25, "Low": 28}, HasCQP: true, RateControl: true,
			
		},
		"h265-main-win-nvidia": {
			ID: "h265-main-win-nvidia", Desc: "H.265 Main, NVENC, 8bit",
			Base: strings.Fields("-c:v hevc_nvenc -profile:v main -pix_fmt yuv420p -b_ref_mode disabled -tag:v hvc1 -g 30"),
			Extensions: []string{"mov", "mkv", "mp4"},
			CQP:        map[string]int{"High": 17, "Mid": 25, "Low": 28}, HasCQP: true, RateControl: true,
			
		},
		"h265-main10-win-nvidia": {
			ID: "h265-main10-win-nvidia", Desc: "H.265 Main10, NVENC, 10bit",
			Base: strings.Fields("-c:v hevc_nvenc -profile:v main10 -pix_fmt p010le -b_ref_mode disabled -tag:v hvc1 -g 30"),
			Extensions: []string{"mov", "mkv", "mp4"},
			CQP:        map[string]int{"High": 17, "Mid": 25, "Low": 28}, HasCQP: true, RateControl: true,
			
		},
		"av1-win-nvidia": {
			ID: "av1-win-nvidia", Desc: "AV1, NVENC, 8bit",
			Base: strings.Fields("-c:v av1_nvenc -preset p6 -tile-columns 2 -tile-rows 2 -pix_fmt yuv420p"),
			Extensions: []string{"mkv", "webm", "mp4"}, HasCQP: false, RateControl: true,
			
		},
		"av1-10bit-win-nvidia": {
			ID: "av1-10bit-win-nvidia", Desc: "AV1, NVENC, 10bit",
			Base: strings.Fields("-c:v av1_nvenc -preset p6 -tile-columns 2 -tile-rows 2 -pix_fmt p010le"),
			Extensions: []string{"mkv", "webm", "mp4"}, HasCQP: false, RateControl: true,
			
		},
		"prores-422-hq-win": {
			ID: "prores-422-hq-win", Desc: "ProRes 422 HQ, 10bit, mov",
			Base: strings.Fields("-c:v prores_ks -profile:v 3 -vendor apl0 -quant_mat hq -bits_per_mb 1350 -pix_fmt yuv422p10le"),
			Extensions: []string{"mov"},
		},
		"prores-422-std-win": {
			ID: "prores-422-std-win", Desc: "ProRes 422, 10bit, mov",
			Base: strings.Fields("-c:v prores_ks -profile:v 2 -vendor apl0 -quant_mat hq -bits_per_mb 1350 -pix_fmt yuv422p10le"),
			Extensions: []string{"mov"},
		},
		"prores-422-lt-win": {
			ID: "prores-422-lt-win", Desc: "ProRes 422 LT, 10bit, mov",
			Base: strings.Fields("-c:v prores_ks -profile:v 1 -vendor apl0 -quant_mat lt -bits_per_mb 525 -pix_fmt yuv422p10le"),
			Extensions: []string{"mov"},
		},
		"ffv1-10bit-444": {
			ID: "ffv1-10bit-444", Desc: "FFV1 无损, 10bit 4:4:4",
			Base: strings.Fields("-level 3 -c:v ffv1 -pix_fmt yuv444p10le -slices 4 -slicecrc 1 -g 1"),
			Extensions: []string{"mkv", "mov", "avi"},
		},
		"ffv1-10bit-420": {
			ID: "ffv1-10bit-420", Desc: "FFV1 无损, 10bit 4:2:0",
			Base: strings.Fields("-level 3 -c:v ffv1 -pix_fmt yuv420p10le -slices 4 -slicecrc 1 -g 1"),
			Extensions: []string{"mkv", "mov", "avi"},
		},
		"vp9-best": {
			ID: "vp9-best", Desc: "VP9 best, 软编",
			Base: strings.Fields("-strict -2 -c:v libvpx-vp9 -pix_fmt yuv420p -row-mt 1 -deadline best"),
			Extensions: []string{"mp4", "mkv", "webm"}, RateControl: true,
		},
	}
}

// Get 返回编码器。
func Get(id string) (*Encoder, error) {
	e := Registry()[id]
	if e == nil {
		var ids []string
		for _, k := range SortedIDs() {
			ids = append(ids, k)
		}
		return nil, fmt.Errorf("未知编码器 %q，可用: %s", id, strings.Join(ids, ", "))
	}
	return e, nil
}

// SortedIDs 返回有序的编码器 ID 列表。
func SortedIDs() []string {
	return []string{
		"h264-main-win-nvidia", "h265-main-win-nvidia", "h265-main10-win-nvidia",
		"av1-win-nvidia", "av1-10bit-win-nvidia",
		"prores-422-hq-win", "prores-422-std-win", "prores-422-lt-win",
		"ffv1-10bit-444", "ffv1-10bit-420", "vp9-best",
	}
}

// Options 是构造编码参数的可选项。
type Options struct {
	EncoderID   string
	Quality     string  // High/Mid/Low（CQ 模式用）
	QP          int     // 直接指定 qp（优先于 Quality）
	RateControl string  // cq/cbr/vbr（默认 cq）
	Bitrate     int     // cbr/vbr 时的码率 (Mbps)
}

// BuildArgs 构造完整的视频编码 ffmpeg 参数。
func BuildArgs(opt Options) ([]string, error) {
	e, err := Get(opt.EncoderID)
	if err != nil {
		return nil, err
	}
	args := append([]string{}, e.Base...)
	rc := opt.RateControl
	if rc == "" {
		rc = "cq"
	}
	switch rc {
	case "cq":
		if e.HasCQP {
			qp := opt.QP
			if qp == 0 {
				q := opt.Quality
				if q == "" {
					q = "High"
				}
				var ok bool
				qp, ok = e.CQP[q]
				if !ok {
					return nil, fmt.Errorf("编码器 %s 不支持质量档 %q（可用 High/Mid/Low）", e.ID, q)
				}
			}
			// NVENC constqp（与 Topaz vbr 分支一致）。
			args = append(args, strings.Fields("-preset p7 -tune hq -rc constqp -rc-lookahead 20 -spatial_aq 1 -aq-strength 15 -b:v 0")...)
			args = append(args, "-qp", fmt.Sprintf("%d", qp))
		} else {
			// 不支持 CQ 的编码器用其默认（CRF 等），不额外加码率参数。
		}
	case "cbr":
		if !e.RateControl {
			return nil, fmt.Errorf("编码器 %s 不支持 cbr", e.ID)
		}
		if opt.Bitrate <= 0 {
			return nil, fmt.Errorf("cbr 需要 --bitrate (Mbps)")
		}
		args = append(args, strings.Fields("-rc cbr -preset p6")...)
		args = append(args, "-b:v", fmt.Sprintf("%dM", opt.Bitrate))
	case "vbr":
		if !e.RateControl {
			return nil, fmt.Errorf("编码器 %s 不支持 vbr", e.ID)
		}
		if opt.Bitrate <= 0 {
			return nil, fmt.Errorf("vbr 需要 --bitrate (Mbps)")
		}
		args = append(args, strings.Fields("-preset p7 -tune hq -rc vbr -b:v 0")...)
		args = append(args, "-cq", "19", "-maxrate", fmt.Sprintf("%dM", opt.Bitrate))
	default:
		return nil, fmt.Errorf("未知码率模式 %q（可用 cq/cbr/vbr）", rc)
	}
	// 快启动。
	if ext := strings.Join(e.Extensions, ""); ext != "" {
		args = append(args, "-movflags", "+faststart")
	}
	return args, nil
}

// AudioCodec 返回音频编码的 ffmpeg 参数。mode 为 copy/aac/ac3/pcm/vorbis。
func AudioCodec(mode string, bitrate int) ([]string, error) {
	switch mode {
	case "", "copy":
		return []string{"-c:a", "copy"}, nil
	case "aac":
		if bitrate <= 0 {
			bitrate = 320
		}
		return []string{"-c:a", "aac", "-ac", "2", "-b:a", fmt.Sprintf("%dk", bitrate)}, nil
	case "ac3":
		if bitrate <= 0 {
			bitrate = 448
		}
		return []string{"-c:a", "ac3", "-b:a", fmt.Sprintf("%dk", bitrate)}, nil
	case "pcm":
		return []string{"-c:a", "pcm_s24le"}, nil
	case "vorbis":
		return []string{"-c:a", "vorbis", "-ac", "2"}, nil
	case "none":
		return []string{"-an"}, nil
	default:
		return nil, fmt.Errorf("未知音频编码 %q（可用 copy/aac/ac3/pcm/vorbis/none）", mode)
	}
}
