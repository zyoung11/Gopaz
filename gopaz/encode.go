package main

import (
	"fmt"
	"strings"
)

// EncoderSpec describes one video encoder.
type EncoderSpec struct {
	ID   string
	Desc string
	// Base holds the base video encoding arguments.
	Base []string
	// Extensions lists the container extensions this encoder accepts.
	Extensions []string
	// CQP maps a quality tier (High/Mid/Low) to a constqp value.
	CQP map[string]int
	// HasCQP reports whether the encoder supports constqp (CQ) mode.
	HasCQP bool
	// RateControl reports whether the encoder supports cbr/vbr rate control.
	RateControl bool
}

// EncoderRegistry returns every encoder indexed by ID.
func EncoderRegistry() map[string]*EncoderSpec {
	return map[string]*EncoderSpec{
		"h264-main-win-nvidia": {
			ID: "h264-main-win-nvidia", Desc: "H.264 High, NVENC, 8bit",
			Base:       strings.Fields("-c:v h264_nvenc -profile:v high -pix_fmt yuv420p -g 30"),
			Extensions: []string{"mov", "mkv", "mp4"},
			CQP:        map[string]int{"High": 18, "Mid": 25, "Low": 28}, HasCQP: true, RateControl: true,
		},
		"h265-main-win-nvidia": {
			ID: "h265-main-win-nvidia", Desc: "H.265 Main, NVENC, 8bit",
			Base:       strings.Fields("-c:v hevc_nvenc -profile:v main -pix_fmt yuv420p -b_ref_mode disabled -tag:v hvc1 -g 30"),
			Extensions: []string{"mov", "mkv", "mp4"},
			CQP:        map[string]int{"High": 17, "Mid": 25, "Low": 28}, HasCQP: true, RateControl: true,
		},
		"h265-main10-win-nvidia": {
			ID: "h265-main10-win-nvidia", Desc: "H.265 Main10, NVENC, 10bit",
			Base:       strings.Fields("-c:v hevc_nvenc -profile:v main10 -pix_fmt p010le -b_ref_mode disabled -tag:v hvc1 -g 30"),
			Extensions: []string{"mov", "mkv", "mp4"},
			CQP:        map[string]int{"High": 17, "Mid": 25, "Low": 28}, HasCQP: true, RateControl: true,
		},
		"av1-win-nvidia": {
			ID: "av1-win-nvidia", Desc: "AV1, NVENC, 8bit",
			Base:       strings.Fields("-c:v av1_nvenc -preset p6 -tile-columns 2 -tile-rows 2 -pix_fmt yuv420p"),
			Extensions: []string{"mkv", "webm", "mp4"}, HasCQP: false, RateControl: true,
		},
		"av1-10bit-win-nvidia": {
			ID: "av1-10bit-win-nvidia", Desc: "AV1, NVENC, 10bit",
			Base:       strings.Fields("-c:v av1_nvenc -preset p6 -tile-columns 2 -tile-rows 2 -pix_fmt p010le"),
			Extensions: []string{"mkv", "webm", "mp4"}, HasCQP: false, RateControl: true,
		},
		"prores-422-hq-win": {
			ID: "prores-422-hq-win", Desc: "ProRes 422 HQ, 10bit, mov",
			Base:       strings.Fields("-c:v prores_ks -profile:v 3 -vendor apl0 -quant_mat hq -bits_per_mb 1350 -pix_fmt yuv422p10le"),
			Extensions: []string{"mov"},
		},
		"prores-422-std-win": {
			ID: "prores-422-std-win", Desc: "ProRes 422, 10bit, mov",
			Base:       strings.Fields("-c:v prores_ks -profile:v 2 -vendor apl0 -quant_mat hq -bits_per_mb 1350 -pix_fmt yuv422p10le"),
			Extensions: []string{"mov"},
		},
		"prores-422-lt-win": {
			ID: "prores-422-lt-win", Desc: "ProRes 422 LT, 10bit, mov",
			Base:       strings.Fields("-c:v prores_ks -profile:v 1 -vendor apl0 -quant_mat lt -bits_per_mb 525 -pix_fmt yuv422p10le"),
			Extensions: []string{"mov"},
		},
		"ffv1-10bit-444": {
			ID: "ffv1-10bit-444", Desc: "FFV1 无损, 10bit 4:4:4",
			Base:       strings.Fields("-level 3 -c:v ffv1 -pix_fmt yuv444p10le -slices 4 -slicecrc 1 -g 1"),
			Extensions: []string{"mkv", "mov", "avi"},
		},
		"ffv1-10bit-420": {
			ID: "ffv1-10bit-420", Desc: "FFV1 无损, 10bit 4:2:0",
			Base:       strings.Fields("-level 3 -c:v ffv1 -pix_fmt yuv420p10le -slices 4 -slicecrc 1 -g 1"),
			Extensions: []string{"mkv", "mov", "avi"},
		},
		"vp9-best": {
			ID: "vp9-best", Desc: "VP9 best, 软编",
			Base:       strings.Fields("-strict -2 -c:v libvpx-vp9 -pix_fmt yuv420p -row-mt 1 -deadline best"),
			Extensions: []string{"mp4", "mkv", "webm"}, RateControl: true,
		},
	}
}

// GetEncoder returns the encoder with the given ID.
func GetEncoder(id string) (*EncoderSpec, error) {
	e := EncoderRegistry()[id]
	if e == nil {
		var ids []string
		for _, k := range EncoderIDs() {
			ids = append(ids, k)
		}
		return nil, fmt.Errorf("未知编码器 %q，可用: %s", id, strings.Join(ids, ", "))
	}
	return e, nil
}

// EncoderIDs returns the encoder IDs in a stable order.
func EncoderIDs() []string {
	return []string{
		"h264-main-win-nvidia", "h265-main-win-nvidia", "h265-main10-win-nvidia",
		"av1-win-nvidia", "av1-10bit-win-nvidia",
		"prores-422-hq-win", "prores-422-std-win", "prores-422-lt-win",
		"ffv1-10bit-444", "ffv1-10bit-420", "vp9-best",
	}
}

// EncodeOptions holds the tunable encoding settings.
type EncodeOptions struct {
	EncoderID string
	// Quality is High/Mid/Low and applies to CQ mode.
	Quality string
	// QP overrides Quality when non-zero.
	QP int
	// RateControl is cq, cbr, or vbr, defaulting to cq.
	RateControl string
	// Bitrate is the target bitrate in Mbps for cbr/vbr.
	Bitrate int
}

// BuildArgs builds the full video encoding argument list.
func BuildArgs(opt EncodeOptions) ([]string, error) {
	e, err := GetEncoder(opt.EncoderID)
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
			args = append(args, strings.Fields("-preset p7 -tune hq -rc constqp -rc-lookahead 20 -spatial_aq 1 -aq-strength 15 -b:v 0")...)
			args = append(args, "-qp", fmt.Sprintf("%d", qp))
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
	if len(e.Extensions) > 0 {
		args = append(args, "-movflags", "+faststart")
	}
	return args, nil
}

// AudioCodec returns the ffmpeg arguments for the given audio mode.
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
