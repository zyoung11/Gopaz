package pipeline

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"tvai/internal/encode"
	"tvai/internal/log"
)

// TempVideoPath 生成保留原扩展名的临时视频路径（ffmpeg 需要扩展名推断容器）。
// 如 out.mp4 -> out.tmp.mp4。
func TempVideoPath(out string) string {
	ext := filepath.Ext(out)
	if ext == "" {
		return out + ".tmp.mp4"
	}
	return out[:len(out)-len(ext)] + ".tmp" + ext
}

// MuxAudio 把原视频的音频轨并入已编码的输出（对应 Topaz 的第二趟 cleanup）。
// audioMode 为 encode.AudioCodec 支持的模式，默认 copy；tempo 为音频倍速，1=不变。
func MuxAudio(ffmpeg, videoOut, origIn, finalOut, audioMode string, audioBitrate int, tempo float64) error {
	if audioMode == "none" {
		if finalOut != videoOut {
			if err := os.Rename(videoOut, finalOut); err != nil {
				return fmt.Errorf("重命名输出失败: %w", err)
			}
		}
		return nil
	}
	if tempo <= 0 {
		tempo = 1
	}
	acodec, err := encode.AudioCodec(audioMode, audioBitrate)
	if err != nil {
		return err
	}
	if tempo != 1 && (audioMode == "" || audioMode == "copy") {
		acodec, _ = encode.AudioCodec("aac", audioBitrate)
		log.Infof("慢放音频使用 AAC 并减速 %.3fx", 1/tempo)
	}
	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-y",
		"-i", videoOut, "-i", origIn,
		"-map", "0:v", "-map", "1:a?",
		"-c:v", "copy",
		"-map_metadata:s:a:0", "1:s:a:0", "-map_metadata", "0",
	}
	if tempo != 1 {
		if filter := atempoChain(tempo); filter != "" {
			args = append(args, "-filter:a", filter)
		}
	}
	args = append(args, acodec...)
	args = append(args, "-movflags", "+faststart", finalOut)
	cmd := exec.Command(ffmpeg, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		log.Warnf("音频并轨失败，输出将不含音轨: %v", err)
		log.Debugf("音频并轨 stderr: %s", string(out))
		if finalOut != videoOut {
			if rerr := os.Rename(videoOut, finalOut); rerr == nil {
				return nil
			}
		}
		return fmt.Errorf("音频并轨失败: %w", err)
	}
	if finalOut != videoOut {
		os.Remove(videoOut)
	}
	return nil
}

// atempoChain 把音频倍速拆成 atempo 支持的 [0.5, 100] 区间。
func atempoChain(rate float64) string {
	var parts []string
	for rate < 0.5 {
		parts = append(parts, "atempo=0.5")
		rate /= 0.5
	}
	if math.Abs(rate-1) > 1e-6 {
		parts = append(parts, fmt.Sprintf("atempo=%.6f", rate))
	}
	return strings.Join(parts, ",")
}
