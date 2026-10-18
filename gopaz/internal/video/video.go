// Package video decodes raw RGB frames and encodes video through ffmpeg subprocesses.
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

// Info holds the basic video information reported by ffprobe.
type Info struct {
	Width    int
	Height   int
	FPS      float64
	Frames   int
	Duration float64
	HasAudio bool
	Rotation int
}

// Probe reads video information with ffprobe.
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

// Decoder reads raw RGB24 frames sequentially from an ffmpeg pipe.
type Decoder struct {
	cmd    *exec.Cmd
	stdout *bufio.Reader
}

// NewDecoder starts ffmpeg and streams rawvideo rgb24 to a stdout pipe.
// A startFrame or endFrame of -1 leaves that end unbounded.
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

// ReadFrame reads one frame into buf, which must hold at least w*h*3 bytes.
// It returns io.EOF at the end of the stream.
func (d *Decoder) ReadFrame(buf []byte) error {
	_, err := io.ReadFull(d.stdout, buf)
	if err == io.ErrUnexpectedEOF || err == io.EOF {
		return io.EOF
	}
	return err
}

// Close terminates the decoder process.
func (d *Decoder) Close() {
	if d.cmd.Process != nil {
		d.cmd.Process.Kill()
	}
	d.cmd.Wait()
}

// Encoder encodes raw RGB24 frames into a video file.
type Encoder struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stderr *bytes.Buffer
}

// NewEncoder starts ffmpeg and reads rawvideo rgb24 from stdin.
// extra holds encoding arguments appended before the output path.
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

// WriteFrame writes one frame.
func (e *Encoder) WriteFrame(buf []byte) error {
	_, err := e.stdin.Write(buf)
	return err
}

// Stderr returns the captured ffmpeg standard error output.
func (e *Encoder) Stderr() string {
	return strings.TrimSpace(e.stderr.String())
}

// Close finishes encoding and waits for ffmpeg to exit.
func (e *Encoder) Close() error {
	e.stdin.Close()
	if err := e.cmd.Wait(); err != nil {
		return fmt.Errorf("ffmpeg 编码失败: %w, stderr: %s", err, strings.TrimSpace(e.stderr.String()))
	}
	return nil
}

// parseRate parses a frame rate such as "30000/1001".
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
