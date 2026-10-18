package pipeline

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"gopaz/internal/encode"
	"gopaz/internal/model"
	"gopaz/internal/video"
)

type e2eEnv struct {
	ffmpeg   string
	ffprobe  string
	modelDir string
	gpu      int
}

func newE2E(t *testing.T) e2eEnv {
	t.Helper()
	if testing.Short() {
		t.Skip("short mode")
	}
	if os.Getenv("GOPAZ_E2E") != "1" {
		t.Skip("set GOPAZ_E2E=1 to run e2e tests")
	}
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not found")
	}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe not found")
	}
	modelDir := os.Getenv("GOPAZ_MODEL_DIR")
	if modelDir == "" {
		modelDir = `C:\zy\APP\Big_APPs\TopazVideoAI\models`
	}
	if st, err := os.Stat(modelDir); err != nil || !st.IsDir() {
		t.Skipf("model dir not found: %s", modelDir)
	}
	if os.Getenv("GOPAZ_ORT_DLL") == "" {
		cand, err := filepath.Abs(filepath.Join("..", "..", "ort-dll", "onnxruntime.dll"))
		if err == nil {
			if _, err := os.Stat(cand); err == nil {
				os.Setenv("GOPAZ_ORT_DLL", cand)
			}
		}
	}
	if os.Getenv("GOPAZ_ORT_DLL") == "" {
		t.Skip("GOPAZ_ORT_DLL not set")
	}
	gpu := -1
	if os.Getenv("GOPAZ_E2E_GPU") == "1" {
		gpu = 0
	}
	return e2eEnv{ffmpeg: ffmpeg, ffprobe: ffprobe, modelDir: modelDir, gpu: gpu}
}

func requireModel(t *testing.T, dir, pattern string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, pattern))
	if err != nil {
		t.Fatalf("glob %s: %v", pattern, err)
	}
	if len(matches) == 0 {
		t.Skipf("model files not found: %s", pattern)
	}
}

func makeClip(t *testing.T, ffmpeg, dir, name string, w, h, fps, frames int) string {
	t.Helper()
	out := filepath.Join(dir, name)
	args := []string{
		"-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", fmt.Sprintf("testsrc2=size=%dx%d:rate=%d", w, h, fps),
		"-frames:v", strconv.Itoa(frames),
		"-pix_fmt", "yuv420p", "-c:v", "libx264", "-qp", "0", out,
	}
	if msg, err := exec.Command(ffmpeg, args...).CombinedOutput(); err != nil {
		t.Fatalf("make clip: %v: %s", err, msg)
	}
	return out
}

func makeDuplicateClip(t *testing.T, ffmpeg, dir string, w, h, unique int) string {
	t.Helper()
	base := makeClip(t, ffmpeg, dir, "unique.mp4", w, h, 15, unique)
	out := filepath.Join(dir, "dup.mp4")
	args := []string{
		"-hide_banner", "-loglevel", "error", "-y",
		"-i", base, "-vf", "fps=30",
		"-pix_fmt", "yuv420p", "-c:v", "libx264", "-qp", "0", out,
	}
	if msg, err := exec.Command(ffmpeg, args...).CombinedOutput(); err != nil {
		t.Fatalf("make duplicate clip: %v: %s", err, msg)
	}
	return out
}

func e2eUpscale(t *testing.T, env e2eEnv, src, out, key string, scale, start, end int) {
	t.Helper()
	m, err := model.Get(key)
	if err != nil {
		t.Fatal(err)
	}
	job := UpscaleJob{
		Input: src, Output: out, Ffmpeg: env.ffmpeg, Ffprobe: env.ffprobe,
		ModelDir: env.modelDir, Model: m, Scale: scale,
		Params: map[string]float64{}, GPU: env.gpu, Threads: 4, Overlap: 0,
		KColor: true, SceneThreshold: 6.5, WorkDir: t.TempDir(),
		StartFrame: start, EndFrame: end,
		Encode:    encode.Options{EncoderID: "ffv1-10bit-420", RateControl: "cq", Quality: "High"},
		AudioMode: "none",
	}
	if err := RunUpscale(job); err != nil {
		t.Fatalf("upscale: %v", err)
	}
}

func e2eInterpolate(t *testing.T, env e2eEnv, src, out, key string, sensitivity, start, end int) {
	t.Helper()
	m, err := model.Get(key)
	if err != nil {
		t.Fatal(err)
	}
	job := InterpJob{
		Input: src, Output: out, Ffmpeg: env.ffmpeg, Ffprobe: env.ffprobe,
		ModelDir: env.modelDir, Model: m, Factor: 2,
		RDT: -1, Sensitivity: sensitivity, Instances: 1, VramGB: 2, DeviceVRAMGB: 2,
		GPU: env.gpu, Threads: 4, Overlap: 0, WorkDir: t.TempDir(),
		StartFrame: start, EndFrame: end,
		Encode:    encode.Options{EncoderID: "ffv1-10bit-420", RateControl: "cq", Quality: "High"},
		AudioMode: "none",
	}
	if err := RunInterpolate(job); err != nil {
		t.Fatalf("interpolate: %v", err)
	}
}

func probeFrameCount(t *testing.T, ffprobe, path string) int {
	t.Helper()
	cmd := exec.Command(ffprobe, "-v", "error", "-count_frames", "-select_streams", "v:0",
		"-show_entries", "stream=nb_read_frames", "-of", "default=nw=1:nk=1", path)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("ffprobe frames: %v", err)
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		t.Fatalf("ffprobe frames %q: %v", out, err)
	}
	return n
}

func assertFrames(t *testing.T, ffprobe, path string, want int) {
	t.Helper()
	if got := probeFrameCount(t, ffprobe, path); got != want {
		t.Fatalf("frames = %d, want %d", got, want)
	}
}

func assertSize(t *testing.T, ffprobe, path string, wantW, wantH int) {
	t.Helper()
	info, err := video.Probe(ffprobe, path)
	if err != nil {
		t.Fatalf("probe output: %v", err)
	}
	if info.Width != wantW || info.Height != wantH {
		t.Fatalf("size = %dx%d, want %dx%d", info.Width, info.Height, wantW, wantH)
	}
}

func extractFrame(t *testing.T, ffmpeg, path string, idx, w, h int) []byte {
	t.Helper()
	args := []string{
		"-hide_banner", "-loglevel", "error", "-y",
		"-i", path, "-vf", fmt.Sprintf("select=eq(n\\,%d)", idx),
		"-vsync", "0", "-frames:v", "1",
		"-f", "rawvideo", "-pix_fmt", "rgb24", "-",
	}
	out, err := exec.Command(ffmpeg, args...).Output()
	if err != nil {
		t.Fatalf("extract frame %d: %v", idx, err)
	}
	if len(out) != w*h*3 {
		t.Fatalf("frame bytes = %d, want %d", len(out), w*h*3)
	}
	return out
}

func meanAbsDiff(a, b []byte) float64 {
	var sum float64
	for i := range a {
		d := float64(a[i]) - float64(b[i])
		if d < 0 {
			d = -d
		}
		sum += d
	}
	return sum / float64(len(a))
}

// TestE2EProteusUpscale checks frame range, scale, and output size for Proteus.
func TestE2EProteusUpscale(t *testing.T) {
	env := newE2E(t)
	requireModel(t, env.modelDir, "prob-v4-fgnet-*.tz")
	dir := t.TempDir()
	src := makeClip(t, env.ffmpeg, dir, "src.mp4", 320, 240, 10, 12)
	cases := []struct {
		name       string
		scale      int
		start, end int
		frames     int
		w, h       int
	}{
		{"range", 1, 5, 9, 5, 320, 240},
		{"start", 1, 5, -1, 7, 320, 240},
		{"end", 1, -1, 4, 5, 320, 240},
		{"scale2", 2, 5, 9, 5, 640, 480},
		{"scale4", 4, 5, 9, 5, 1280, 960},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := filepath.Join(dir, tc.name+".mkv")
			e2eUpscale(t, env, src, out, "proteus", tc.scale, tc.start, tc.end)
			assertFrames(t, env.ffprobe, out, tc.frames)
			assertSize(t, env.ffprobe, out, tc.w, tc.h)
		})
	}
}

// TestE2EProteusTile checks the Topaz tile choice for 720p 2x.
func TestE2EProteusTile(t *testing.T) {
	env := newE2E(t)
	requireModel(t, env.modelDir, "prob-v4-fgnet-*.tz")
	m, err := model.Get("proteus")
	if err != nil {
		t.Fatal(err)
	}
	r, err := LoadRunner(UpscaleConfig{
		Model: m, ModelDir: env.modelDir, Scale: 2, FW: 1280, FH: 720,
		Params: map[string]float64{}, Overlap: 0, GPU: env.gpu, Threads: 4,
		Instances: 1, WorkDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("load runner: %v", err)
	}
	defer r.Close()
	th, tw := r.TileSize()
	if th != 384 || tw != 672 {
		t.Fatalf("tile = %dx%d, want 384x672", th, tw)
	}
}

// TestE2EGaiaUpscale checks the five-frame temporal input and output size for Gaia.
func TestE2EGaiaUpscale(t *testing.T) {
	env := newE2E(t)
	requireModel(t, env.modelDir, "ghq-v5-gaia-*.tz")
	dir := t.TempDir()
	src := makeClip(t, env.ffmpeg, dir, "src.mp4", 320, 240, 10, 12)
	out := filepath.Join(dir, "gaia.mkv")
	e2eUpscale(t, env, src, out, "gaia", 2, -1, -1)
	assertFrames(t, env.ffprobe, out, 12)
	assertSize(t, env.ffprobe, out, 640, 480)
}

// TestE2EGaiaCGUpscale checks the five-frame temporal input and output size for Gaia-CG.
func TestE2EGaiaCGUpscale(t *testing.T) {
	env := newE2E(t)
	requireModel(t, env.modelDir, "gcg-v5-gaia-*.tz")
	dir := t.TempDir()
	src := makeClip(t, env.ffmpeg, dir, "src.mp4", 320, 240, 10, 12)
	out := filepath.Join(dir, "gaia-cg.mkv")
	e2eUpscale(t, env, src, out, "gaia-cg", 2, -1, -1)
	assertFrames(t, env.ffprobe, out, 12)
	assertSize(t, env.ffprobe, out, 640, 480)
}

// TestE2EApolloDuplicate checks duplicate frame replacement for Apollo.
func TestE2EApolloDuplicate(t *testing.T) {
	env := newE2E(t)
	requireModel(t, env.modelDir, "apo-v8-*.tz")
	dir := t.TempDir()
	src := makeDuplicateClip(t, env.ffmpeg, dir, 256, 160, 4)
	off := filepath.Join(dir, "off.mkv")
	on := filepath.Join(dir, "on.mkv")
	e2eInterpolate(t, env, src, off, "apollo", 0, -1, -1)
	e2eInterpolate(t, env, src, on, "apollo", 10, -1, -1)
	assertFrames(t, env.ffprobe, off, 15)
	assertFrames(t, env.ffprobe, on, 15)
	assertSize(t, env.ffprobe, off, 256, 160)
	assertSize(t, env.ffprobe, on, 256, 160)
	diff := meanAbsDiff(
		extractFrame(t, env.ffmpeg, off, 1, 256, 160),
		extractFrame(t, env.ffmpeg, on, 1, 256, 160),
	)
	t.Logf("duplicate frame diff = %.4f", diff)
	if diff < 0.01 {
		t.Fatalf("duplicate replacement did not change frame 1: %.4f", diff)
	}
}

// TestE2EAionDuplicate checks duplicate frame replacement for Aion and is skipped by default.
func TestE2EAionDuplicate(t *testing.T) {
	if os.Getenv("GOPAZ_E2E_AION") != "1" {
		t.Skip("set GOPAZ_E2E_AION=1 to run Aion e2e")
	}
	if os.Getenv("GOPAZ_E2E_GPU") != "1" {
		t.Skip("Aion e2e requires GOPAZ_E2E_GPU=1")
	}
	env := newE2E(t)
	requireModel(t, env.modelDir, "aion-v1-*.tz")
	requireModel(t, env.modelDir, "aiob-v1-*.tz")
	dir := t.TempDir()
	src := makeDuplicateClip(t, env.ffmpeg, dir, 256, 160, 4)
	off := filepath.Join(dir, "off.mkv")
	on := filepath.Join(dir, "on.mkv")
	e2eInterpolate(t, env, src, off, "aion", 0, -1, -1)
	e2eInterpolate(t, env, src, on, "aion", 10, -1, -1)
	assertFrames(t, env.ffprobe, off, 15)
	assertFrames(t, env.ffprobe, on, 15)
	diff := meanAbsDiff(
		extractFrame(t, env.ffmpeg, off, 1, 256, 160),
		extractFrame(t, env.ffmpeg, on, 1, 256, 160),
	)
	t.Logf("duplicate frame diff = %.4f", diff)
	if diff < 0.01 {
		t.Fatalf("duplicate replacement did not change frame 1: %.4f", diff)
	}
}
