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
// meaning unchanged. trimStart and trimDuration select the source range in
// seconds; zero values keep the whole track.
func MuxAudio(ffmpeg, videoOut, origIn, finalOut, audioMode string, audioBitrate int, tempo, trimStart, trimDuration float64) error {
	if audioMode == "none" {
		if finalOut != videoOut {
			if err := os.Rename(videoOut, finalOut); err != nil {
				return fmt.Errorf("failed to rename output: %w", err)
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
		Infof("slow-motion audio uses AAC at %.3fx", 1/tempo)
	}
	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-y",
		"-i", videoOut}
	args = append(args, audioTrimArgs(trimStart, trimDuration)...)
	args = append(args, "-i", origIn,
		"-map", "0:v", "-map", "1:a?",
		"-c:v", "copy",
		"-map_metadata:s:a:0", "1:s:a:0", "-map_metadata", "0")
	if tempo != 1 {
		if filter := atempoChain(tempo); filter != "" {
			args = append(args, "-filter:a", filter)
		}
	}
	args = append(args, acodec...)
	args = append(args, "-movflags", "+faststart", finalOut)
	cmd := exec.Command(ffmpeg, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		Warnf("audio mux failed, output will have no audio track: %v", err)
		Debugf("audio mux stderr: %s", string(out))
		if finalOut != videoOut {
			if rerr := os.Rename(videoOut, finalOut); rerr == nil {
				return nil
			}
		}
		return fmt.Errorf("audio mux failed: %w", err)
	}
	if finalOut != videoOut {
		os.Remove(videoOut)
	}
	return nil
}

// audioTrimArgs returns input-side options that trim the source audio to the
// processed range.
func audioTrimArgs(startSec, durationSec float64) []string {
	var args []string
	if startSec > 0 {
		args = append(args, "-ss", fmt.Sprintf("%.6f", startSec))
	}
	if durationSec > 0 {
		args = append(args, "-t", fmt.Sprintf("%.6f", durationSec))
	}
	return args
}

// sourceAudioTrim maps a resolved frame range to a source-time offset and
// duration for the audio track.
func sourceAudioTrim(info *VideoInfo, fr FrameRange, total int, rangeSpecified bool) (float64, float64) {
	if !rangeSpecified || info == nil || info.FPS <= 0 || fr.Start < 0 {
		return 0, 0
	}
	start := float64(fr.Start) / info.FPS
	duration := 0.0
	if total > 0 {
		duration = float64(total) / info.FPS
	}
	return start, duration
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
