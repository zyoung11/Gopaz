package pipeline

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

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
// audioMode 为 encode.AudioCodec 支持的模式，默认 copy。
func MuxAudio(ffmpeg, videoOut, origIn, finalOut, audioMode string, audioBitrate int) error {
	if audioMode == "none" {
		if finalOut != videoOut {
			if err := os.Rename(videoOut, finalOut); err != nil {
				return fmt.Errorf("重命名输出失败: %w", err)
			}
		}
		return nil
	}
	acodec, err := encode.AudioCodec(audioMode, audioBitrate)
	if err != nil {
		return err
	}
	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-y",
		"-i", videoOut, "-i", origIn,
		"-map", "0:v", "-map", "1:a?",
		"-c:v", "copy",
		"-map_metadata:s:a:0", "1:s:a:0", "-map_metadata", "0",
	}
	args = append(args, acodec...)
	args = append(args, "-movflags", "+faststart", finalOut)
	cmd := exec.Command(ffmpeg, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		// 若无音频流等非致命情况，退回直接使用视频输出。
		log.Debugf("音频并轨失败: %v, %s", err, string(out))
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
