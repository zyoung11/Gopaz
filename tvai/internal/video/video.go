// Package video 通过 ffmpeg 子进程完成视频的解码（原始 RGB 帧）与编码。
package video

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"math"
	"os/exec"
	"strconv"
	"strings"
)

// Info 是 ffprobe 得到的视频基础信息。
type Info struct {
	Width    int
	Height   int
	FPS      float64
	Frames   int
	Duration float64
	HasAudio bool
	Rotation int
}

// Probe 用 ffprobe 读取视频信息。
func Probe(ffprobe, path string) (*Info, error) {
	cmd := exec.Command(ffprobe, "-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=width,height,r_frame_rate,nb_frames,duration:stream_side_data=rotation:format=duration",
		"-of", "default=noprint_wrappers=1", path)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe 失败: %w", err)
	}
	info := &Info{FPS: 30}
	for _, line := range splitLines(string(out)) {
		switch {
		case hasPrefix(line, "width="):
			info.Width, _ = strconv.Atoi(line[6:])
		case hasPrefix(line, "height="):
			info.Height, _ = strconv.Atoi(line[7:])
		case hasPrefix(line, "r_frame_rate="):
			num, den := parseRate(line[13:])
			if den > 0 {
				info.FPS = float64(num) / float64(den)
			}
		case hasPrefix(line, "nb_frames="):
			info.Frames, _ = strconv.Atoi(line[10:])
		case hasPrefix(line, "duration="):
			info.Duration, _ = strconv.ParseFloat(line[9:], 64)
		case hasPrefix(line, "rotation="):
			info.Rotation, _ = strconv.Atoi(line[9:])
		}
	}
	// 检测音频流。
	if info.Frames <= 0 && info.Duration > 0 && info.FPS > 0 {
		info.Frames = int(math.Round(info.Duration * info.FPS))
	}
	acmd := exec.Command(ffprobe, "-v", "error", "-select_streams", "a",
		"-show_entries", "stream=codec_type", "-of", "csv=p=0", path)
	if aout, err := acmd.Output(); err == nil && len(trimSpace(string(aout))) > 0 {
		info.HasAudio = true
	}
	return info, nil
}

// Decoder 从 ffmpeg 管道中顺序读出原始 RGB24 帧。
type Decoder struct {
	cmd    *exec.Cmd
	stdout *bufio.Reader
}

// NewDecoder 启动 ffmpeg 解码，输出 rawvideo rgb24 到 stdout 管道。
// startFrame/endFrame 为 -1 表示该端不限制。
func NewDecoder(ffmpeg, path string, w, h int, startFrame, endFrame int) (*Decoder, error) {
	filter := fmt.Sprintf("scale=%d:%d:flags=bicubic", w, h)
	selecting := false
	switch {
	case startFrame >= 0 && endFrame >= 0:
		filter = fmt.Sprintf("select='between(n\\,%d\\,%d)',scale=%d:%d:flags=bicubic", startFrame, endFrame, w, h)
		selecting = true
	case startFrame >= 0:
		filter = fmt.Sprintf("select='gte(n\\,%d)',scale=%d:%d:flags=bicubic", startFrame, w, h)
		selecting = true
	case endFrame >= 0:
		filter = fmt.Sprintf("select='lte(n\\,%d)',scale=%d:%d:flags=bicubic", endFrame, w, h)
		selecting = true
	}
	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-y",
		"-i", path,
		"-vf", filter}
	if selecting {
		args = append(args, "-vsync", "0")
	}
	args = append(args, "-f", "rawvideo", "-pix_fmt", "rgb24", "-")
	cmd := exec.Command(ffmpeg, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("启动 ffmpeg 解码失败: %w", err)
	}
	return &Decoder{cmd: cmd, stdout: bufio.NewReaderSize(stdout, 1<<20)}, nil
}

// ReadFrame 读出一帧到 buf（长度需 >= w*h*3）。到达末尾返回 io.EOF。
func (d *Decoder) ReadFrame(buf []byte) error {
	_, err := io.ReadFull(d.stdout, buf)
	if err == io.ErrUnexpectedEOF || err == io.EOF {
		return io.EOF
	}
	return err
}

// Close 结束解码进程。
func (d *Decoder) Close() {
	if d.cmd.Process != nil {
		d.cmd.Process.Kill()
	}
	d.cmd.Wait()
}

// Encoder 把原始 RGB24 帧编码为视频。
type Encoder struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stderr *bytes.Buffer
}

// NewEncoder 启动 ffmpeg 编码，从 stdin 读 rawvideo rgb24。
// extra 为追加在输出路径前的编码参数（如 -c:v ...）。
func NewEncoder(ffmpeg, outPath string, w, h int, fps float64, extra []string) (*Encoder, error) {
	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-y",
		"-f", "rawvideo", "-pix_fmt", "rgb24", "-s", fmt.Sprintf("%dx%d", w, h),
		"-framerate", strconv.FormatFloat(fps, 'f', 6, 64), "-i", "-"}
	args = append(args, extra...)
	args = append(args, outPath)
	cmd := exec.Command(ffmpeg, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stdout = nil
	var se bytes.Buffer
	cmd.Stderr = &se
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("启动 ffmpeg 编码失败: %w", err)
	}
	return &Encoder{cmd: cmd, stdin: stdin, stderr: &se}, nil
}

// WriteFrame 写入一帧。
func (e *Encoder) WriteFrame(buf []byte) error {
	_, err := e.stdin.Write(buf)
	return err
}

// Stderr 返回 ffmpeg 的标准错误输出（诊断用）。
func (e *Encoder) Stderr() string {
	return strings.TrimSpace(e.stderr.String())
}

// Close 结束编码并等待其退出。
func (e *Encoder) Close() error {
	e.stdin.Close()
	if err := e.cmd.Wait(); err != nil {
		return fmt.Errorf("ffmpeg 编码失败: %w, stderr: %s", err, strings.TrimSpace(e.stderr.String()))
	}
	return nil
}

// ParseRate 解析 "30000/1001" 形式的帧率。
func parseRate(s string) (int, int) {
	slash := -1
	for i := 0; i < len(s); i++ {
		if s[i] == '/' {
			slash = i
			break
		}
	}
	if slash < 0 {
		v, _ := strconv.Atoi(trimSpace(s))
		return v, 1
	}
	num, _ := strconv.Atoi(trimSpace(s[:slash]))
	den, _ := strconv.Atoi(trimSpace(s[slash+1:]))
	return num, den
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			line := trimSpace(s[start:i])
			if line != "" {
				out = append(out, line)
			}
			start = i + 1
		}
	}
	if start < len(s) {
		if line := trimSpace(s[start:]); line != "" {
			out = append(out, line)
		}
	}
	return out
}

func hasPrefix(s, p string) bool {
	return len(s) >= len(p) && s[:len(p)] == p
}

func trimSpace(s string) string {
	i, j := 0, len(s)
	for i < j && (s[i] == ' ' || s[i] == '\t' || s[i] == '\r' || s[i] == '\n') {
		i++
	}
	for j > i && (s[j-1] == ' ' || s[j-1] == '\t' || s[j-1] == '\r' || s[j-1] == '\n') {
		j--
	}
	return s[i:j]
}
