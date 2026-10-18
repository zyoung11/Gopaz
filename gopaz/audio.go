package main

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// TempVideoPath builds a temporary video path that keeps the original extension,
// which ffmpeg needs to infer the container. For example, out.mp4 becomes
// out.tmp.mp4.
func TempVideoPath(out string) string {
	ext := filepath.Ext(out)
	if ext == "" {
		return out + ".tmp.mp4"
	}
	return out[:len(out)-len(ext)] + ".tmp" + ext
}

// MuxAudio merges the audio track of the source video into the encoded output,
// mirroring the second cleanup pass in Topaz. audioMode is a mode supported by
// AudioCodec, defaulting to copy; tempo is the audio speed factor, with 1
// meaning unchanged.
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
	acodec, err := AudioCodec(audioMode, audioBitrate)
	if err != nil {
		return err
	}
	if tempo != 1 && (audioMode == "" || audioMode == "copy") {
		acodec, _ = AudioCodec("aac", audioBitrate)
		Infof("慢放音频使用 AAC 并减速 %.3fx", 1/tempo)
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
		Warnf("音频并轨失败，输出将不含音轨: %v", err)
		Debugf("音频并轨 stderr: %s", string(out))
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

// atempoChain splits a speed factor into atempo steps within the supported
// range of 0.5 to 100.
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
