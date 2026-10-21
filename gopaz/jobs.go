package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

type jobRequest struct {
	Input          string             `json:"input"`
	OutputDir      string             `json:"outputDir"`
	OutputName     string             `json:"outputName"`
	Mode           string             `json:"mode"`
	Model          string             `json:"model"`
	Scale          int                `json:"scale"`
	Factor         int                `json:"factor"`
	Params         map[string]float64 `json:"params"`
	Encoder        string             `json:"encoder"`
	Container      string             `json:"container"`
	Quality        string             `json:"quality"`
	RateControl    string             `json:"rateControl"`
	Bitrate        int                `json:"bitrate"`
	QP             int                `json:"qp"`
	Instances      int                `json:"instances"`
	AudioCodec     string             `json:"audioCodec"`
	AudioBitrate   int                `json:"audioBitrate"`
	GPU            int                `json:"gpu"`
	Provider       string             `json:"provider"`
	Device         string             `json:"device"`
	VideoDevice    string             `json:"videoDevice"`
	Auto           bool               `json:"auto"`
	Threads        int                `json:"threads"`
	Workers        int                `json:"workers"`
	VramGB         float64            `json:"vram"`
	DeviceVRAMGB   float64            `json:"gpuVram"`
	Overlap        int                `json:"overlap"`
	KColor         bool               `json:"kcolor"`
	SceneThreshold float64            `json:"sceneThreshold"`
	StartFrame     int                `json:"startFrame"`
	EndFrame       int                `json:"endFrame"`
	Sensitivity    int                `json:"sensitivity"`
	RDT            float64            `json:"rdt"`
	Preview        bool               `json:"preview"`
	PreviewStart   float64            `json:"previewStart"`
}

func defaultJobRequest() jobRequest {
	return jobRequest{
		Mode: "enhance", Model: "proteus", Scale: 2, Factor: 2,
		Encoder: "", Quality: "High", RateControl: "cq",
		AudioCodec: "copy", GPU: -2, Provider: "cuda", KColor: true, SceneThreshold: 6.5,
		StartFrame: -1, EndFrame: -1, Sensitivity: 10, RDT: -1,
	}
}

type jobPlan struct {
	Mode                  string
	Model                 *Model
	Encoder               *EncoderSpec
	OutputPath            string
	Total                 int
	Info                  *VideoInfo
	Preview               bool
	PreviewStart          float64
	PreviewSourceDuration float64
	PreviewDuration       float64
}

type jobLog struct {
	Time  string `json:"time"`
	Level string `json:"level"`
	Msg   string `json:"msg"`
}

type jobProgress struct {
	Label string  `json:"label"`
	Cur   int     `json:"cur"`
	Total int     `json:"total"`
	Pct   float64 `json:"pct"`
	Speed float64 `json:"speed"`
	ETA   float64 `json:"eta"`
	Phase string  `json:"phase,omitempty"`
}

type job struct {
	ID       string
	State    string
	Input    string
	Output   string
	Total    int
	Progress jobProgress
	Logs     []jobLog
	Error    string
	Started  time.Time
	Finished time.Time

	mu                    sync.Mutex
	cmd                   *exec.Cmd
	cancelled             bool
	lastCur               int
	lastProgressAt        time.Time
	Preview               bool
	PreviewStart          float64
	PreviewSourceDuration float64
	PreviewDuration       float64
}

type jobView struct {
	ID                    string      `json:"id"`
	State                 string      `json:"state"`
	Input                 string      `json:"input"`
	Output                string      `json:"output"`
	Progress              jobProgress `json:"progress"`
	Logs                  []jobLog    `json:"logs"`
	Error                 string      `json:"error"`
	StartedAt             string      `json:"startedAt"`
	FinishedAt            string      `json:"finishedAt"`
	Elapsed               float64     `json:"elapsed"`
	Preview               bool        `json:"preview"`
	PreviewStart          float64     `json:"previewStart"`
	PreviewSourceDuration float64     `json:"previewSourceDuration"`
	PreviewDuration       float64     `json:"previewDuration"`
}

type childEvent struct {
	Type  string  `json:"type"`
	Level string  `json:"level"`
	Msg   string  `json:"msg"`
	Label string  `json:"label"`
	Cur   int     `json:"cur"`
	Total int     `json:"total"`
	Pct   float64 `json:"pct"`
	Phase string  `json:"phase"`
}

var (
	jobsMu    sync.Mutex
	jobs      = map[string]*job{}
	activeJob *job
)

func resolveOutput(req *jobRequest) (*jobPlan, error) {
	if req.Input == "" {
		return nil, errors.New("select an input video")
	}
	input := normalizePath(req.Input)
	st, err := os.Stat(input)
	if err != nil {
		return nil, errors.New("input video does not exist or is not accessible")
	}
	if st.IsDir() {
		return nil, errors.New("input path is a folder")
	}
	mode := strings.ToLower(strings.TrimSpace(req.Mode))
	if mode == "" {
		mode = "enhance"
	}
	if mode != "enhance" && mode != "interp" {
		return nil, errors.New("unknown processing mode")
	}
	m, err := GetModel(req.Model)
	if err != nil {
		return nil, err
	}
	if mode == "enhance" && m.Kind != KindEnhance {
		return nil, fmt.Errorf("%s is not an enhancement model", m.Display)
	}
	if mode == "interp" && m.Kind != KindInterp {
		return nil, fmt.Errorf("%s is not an interpolation model", m.Display)
	}
	if mode == "enhance" && !slices.Contains(m.Scales, req.Scale) {
		return nil, fmt.Errorf("%s does not support %dx", m.Display, req.Scale)
	}
	if mode == "interp" && req.Factor < 2 {
		return nil, errors.New("slow motion factor must be at least 2")
	}
	if req.Encoder == "" {
		req.Encoder = defaultEncoder(localTool("ffmpeg"))
	}
	e, err := GetEncoder(req.Encoder)
	if err != nil {
		return nil, err
	}
	ext := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(req.Container)), ".")
	if ext == "" {
		if slices.Contains(e.Extensions, "mp4") {
			ext = "mp4"
		} else if len(e.Extensions) > 0 {
			ext = e.Extensions[0]
		}
	}
	if ext == "" || !slices.Contains(e.Extensions, ext) {
		return nil, fmt.Errorf("%s does not support the %s container", e.ID, ext)
	}
	rc := req.RateControl
	if rc == "" {
		rc = "cq"
	}
	if rc != "cq" && rc != "cbr" && rc != "vbr" {
		return nil, fmt.Errorf("unknown rate control %q", rc)
	}
	if rc != "cq" && !e.RateControl {
		return nil, fmt.Errorf("%s does not support %s", e.ID, rc)
	}
	if (rc == "cbr" || rc == "vbr") && req.Bitrate <= 0 {
		return nil, errors.New("cbr/vbr requires a bitrate")
	}
	req.RateControl = rc
	if req.Quality == "" {
		req.Quality = "High"
	}
	if _, err := BuildArgs(EncodeOptions{
		EncoderID: e.ID, Quality: req.Quality, RateControl: rc, Bitrate: req.Bitrate,
	}); err != nil {
		return nil, err
	}
	if _, err := AudioCodec(req.AudioCodec, req.AudioBitrate); err != nil {
		return nil, err
	}

	dir := normalizePath(req.OutputDir)
	if dir == "" {
		dir = filepath.Dir(input)
	}
	if st, err := os.Stat(dir); err == nil && !st.IsDir() {
		return nil, errors.New("output path is not a folder")
	}

	if name := strings.TrimSpace(req.OutputName); name != "" {
		base := strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
		if base == "" || base == "." || base == ".." {
			return nil, errors.New("invalid output file name")
		}
		if invalidFilename(base) {
			return nil, errors.New("output file name contains invalid characters")
		}
		return &jobPlan{Mode: mode, Model: m, Encoder: e, OutputPath: dedupePath(dir, base, "", ext)}, nil
	}

	base := strings.TrimSuffix(filepath.Base(input), filepath.Ext(input))
	return &jobPlan{Mode: mode, Model: m, Encoder: e, OutputPath: dedupePath(dir, base, "_"+m.Short+m.Version, ext)}, nil
}

// dedupePath appends (N) to base until the output path is free.
func dedupePath(dir, base, suffix, ext string) string {
	p := filepath.Join(dir, base+suffix+"."+ext)
	for i := 1; fileExists(p); i++ {
		p = filepath.Join(dir, fmt.Sprintf("%s(%d)%s.%s", base, i, suffix, ext))
	}
	return p
}

func invalidFilename(name string) bool {
	if name == "" || strings.HasSuffix(name, " ") || strings.HasSuffix(name, ".") {
		return true
	}
	if strings.ContainsAny(name, `<>:"/\\|?*`) {
		return true
	}
	for _, r := range name {
		if r < 32 {
			return true
		}
	}
	upper := strings.ToUpper(name)
	switch upper {
	case "CON", "PRN", "AUX", "NUL":
		return true
	}
	if len(upper) == 4 && (strings.HasPrefix(upper, "COM") || strings.HasPrefix(upper, "LPT")) && upper[3] >= '1' && upper[3] <= '9' {
		return true
	}
	return false
}

func planJob(req *jobRequest) (*jobPlan, error) {
	plan, err := resolveOutput(req)
	if err != nil {
		return nil, err
	}
	info, err := ProbeVideo(localTool("ffprobe"), normalizePath(req.Input))
	if err != nil {
		return nil, fmt.Errorf("failed to read video info: %w", err)
	}
	plan.Info = info
	plan.Total = estimateTotal(plan.Mode, req, info)
	dir := filepath.Dir(plan.OutputPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create output folder: %w", err)
	}
	return plan, nil
}

func planPreview(req *jobRequest) (*jobPlan, error) {
	input := normalizePath(req.Input)
	info, err := ProbeVideo(localTool("ffprobe"), input)
	if err != nil {
		return nil, fmt.Errorf("failed to read video info: %w", err)
	}
	if info.Frames <= 0 || info.FPS <= 0 {
		return nil, errors.New("cannot determine frame count for preview")
	}
	seconds := loadAppConfig().PreviewDuration
	if seconds <= 0 {
		seconds = 2
	}
	span := max(1, int(math.Round(seconds*info.FPS)))
	span = min(span, info.Frames)
	start := int(math.Round(req.PreviewStart * info.FPS))
	start = min(start, info.Frames-span)
	start = max(start, 0)
	end := start + span - 1

	dir := previewCacheDir()
	req.StartFrame = start
	req.EndFrame = end
	req.OutputDir = dir
	req.OutputName = "preview"
	outW, outH := info.Width, info.Height
	if req.Scale > 1 {
		outW, outH = outW*req.Scale, outH*req.Scale
	}
	req.Encoder = choosePreviewEncoder(localTool("ffmpeg"), outW, outH)
	req.Container = "mp4"
	req.Quality = "High"
	req.RateControl = "cq"
	req.Bitrate = 0
	req.QP = 0
	req.AudioCodec = "none"
	req.AudioBitrate = 0

	plan, err := planJob(req)
	if err != nil {
		return nil, err
	}
	plan.Preview = true
	plan.PreviewStart = float64(start) / info.FPS
	sourceFrames := end - start + 1
	plan.PreviewSourceDuration = float64(sourceFrames) / info.FPS
	if plan.Total > 0 {
		plan.PreviewDuration = float64(plan.Total) / info.FPS
	} else {
		plan.PreviewDuration = plan.PreviewSourceDuration
	}
	return plan, nil
}

// defaultEncoder selects NVENC when a probe encode succeeds and falls back to
// software H.264 otherwise.
func defaultEncoder(ffmpeg string) string {
	if probeEncoder(ffmpeg, "hevc_nvenc") {
		return "h265-main-win-nvidia"
	}
	if probeEncoder(ffmpeg, "h264_nvenc") {
		return "h264-main-win-nvidia"
	}
	return "h264-main-sw"
}

// probeEncoder checks whether an ffmpeg encoder can encode one small frame.
func probeEncoder(ffmpeg, name string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, ffmpeg, "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "color=c=black:s=256x256:d=0.04",
		"-frames:v", "1", "-c:v", name, "-pix_fmt", "yuv420p", "-f", "null", "-")
	return cmd.Run() == nil
}

// choosePreviewEncoder picks a fast encoder for previews that fits the output
// size, re-probing every call so a busy GPU encoder falls back to software.
func choosePreviewEncoder(ffmpeg string, w, h int) string {
	if w <= 4096 && h <= 4096 && probeEncoder(ffmpeg, "h264_nvenc") {
		return "h264-main-win-nvidia"
	}
	if probeEncoder(ffmpeg, "hevc_nvenc") {
		return "h265-main-win-nvidia"
	}
	return "h264-main-sw"
}

// previewCacheDir returns the persistent preview folder inside the cache root.
func previewCacheDir() string {
	return filepath.Join(cacheRoot(), "preview")
}

// resetPreviewCache clears the persistent preview folder and recreates it.
func resetPreviewCache() error {
	dir := previewCacheDir()
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	return os.MkdirAll(dir, 0o755)
}

func estimateTotal(mode string, req *jobRequest, info *VideoInfo) int {
	fr := ResolveFrameRange(req.StartFrame, req.EndFrame, info.Frames)
	count := fr.Count
	if req.StartFrame < 0 && req.EndFrame < 0 {
		count = info.Frames
	}
	if count <= 0 {
		return 0
	}
	if mode == "interp" {
		return (count-1)*req.Factor + 1
	}
	return count
}

func buildJobArgs(req *jobRequest, plan *jobPlan) []string {
	cmdName := "upscale"
	if plan.Mode == "interp" {
		cmdName = "interpolate"
	}
	args := []string{
		cmdName, "-i", normalizePath(req.Input), "-o", plan.OutputPath,
		"--model", plan.Model.Key, "--encoder", plan.Encoder.ID,
		"--quality", req.Quality, "--rate-control", req.RateControl,
		"--audio-codec", req.AudioCodec,
		"--provider", req.Provider, "--device", req.Device,
		"--video-device", req.VideoDevice,
		"--gpu", itoa(req.GPU), "--threads", itoa(req.Threads),
		"--workers", itoa(req.Workers), "--vram", ftoa(req.VramGB),
		"--overlap", itoa(req.Overlap),
		"--start", itoa(req.StartFrame), "--end", itoa(req.EndFrame),
	}
	if req.Bitrate > 0 {
		args = append(args, "--bitrate", itoa(req.Bitrate))
	}
	if req.AudioBitrate > 0 {
		args = append(args, "--audio-bitrate", itoa(req.AudioBitrate))
	}
	if req.QP > 0 {
		args = append(args, "--qp", itoa(req.QP))
	}
	if plan.Mode == "enhance" {
		args = append(args, "--scale", itoa(req.Scale))
		args = append(args, "--kcolor", boolInt(req.KColor))
		args = append(args, "--auto", boolInt(req.Auto))
		args = append(args, "--scene-threshold", ftoa(req.SceneThreshold))
		for _, p := range plan.Model.Params {
			args = append(args, "--"+p.Key, ftoa(req.Params[p.Key]))
		}
		return args
	}
	args = append(args,
		"--factor", itoa(req.Factor),
		"--sensitivity", itoa(req.Sensitivity),
		"--gpu-vram", ftoa(req.DeviceVRAMGB),
		"--instances", itoa(req.Instances),
		"--rdt", ftoa(req.RDT),
	)
	return args
}

func formatCommand(args []string) string {
	quoted := make([]string, len(args))
	for i, arg := range args {
		if strings.ContainsAny(arg, " 	") {
			quoted[i] = "\"" + arg + "\""
		} else {
			quoted[i] = arg
		}
	}
	return strings.Join(quoted, " ")
}

func handleCreateJob(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	req := defaultJobRequest()
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	jobsMu.Lock()
	busy := activeJob != nil && activeJob.isRunning()
	jobsMu.Unlock()
	if busy {
		writeError(w, http.StatusConflict, "a job is already running; wait for it to finish or cancel it")
		return
	}
	var plan *jobPlan
	var err error
	if req.Preview {
		plan, err = planPreview(&req)
	} else {
		plan, err = planJob(&req)
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	jobsMu.Lock()
	if activeJob != nil && activeJob.isRunning() {
		jobsMu.Unlock()
		writeError(w, http.StatusConflict, "a job is already running; wait for it to finish or cancel it")
		return
	}
	label := "Enhance"
	if plan.Mode == "interp" {
		label = "Interpolate"
	}
	j := &job{
		ID: newJobID(), State: "running", Input: normalizePath(req.Input),
		Output: plan.OutputPath, Total: plan.Total, Started: time.Now(),
		Progress: jobProgress{Label: label, Total: plan.Total},
		Preview:  plan.Preview, PreviewStart: plan.PreviewStart,
		PreviewSourceDuration: plan.PreviewSourceDuration, PreviewDuration: plan.PreviewDuration,
	}
	jobs[j.ID] = j
	activeJob = j
	jobsMu.Unlock()

	allowMediaPath(plan.OutputPath)
	args := buildJobArgs(&req, plan)
	ext := strings.TrimPrefix(filepath.Ext(plan.OutputPath), ".")
	if plan.Preview {
		j.addLog("info", fmt.Sprintf("Preview range: %.2fs-%.2fs, output %.2fs", plan.PreviewStart, plan.PreviewStart+plan.PreviewSourceDuration, plan.PreviewDuration))
		j.addLog("info", "Preview file: "+plan.OutputPath)
	} else {
		j.addLog("info", fmt.Sprintf("Settings: mode=%s model=%s encoder=%s container=%s", plan.Mode, plan.Model.Key, plan.Encoder.ID, ext))
		j.addLog("info", "Output file: "+plan.OutputPath)
	}
	j.addLog("info", "Command: gopaz "+formatCommand(args))
	go runJob(j, args)
	writeJSON(w, http.StatusOK, j.view())
}

func handleJobRoute(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/jobs/")
	parts := strings.Split(rest, "/")
	if len(parts) == 0 || parts[0] == "" {
		writeError(w, http.StatusNotFound, "job not found")
		return
	}
	id := parts[0]
	jobsMu.Lock()
	j := jobs[id]
	jobsMu.Unlock()
	if j == nil {
		writeError(w, http.StatusNotFound, "job not found")
		return
	}
	if len(parts) == 1 {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		writeJSON(w, http.StatusOK, j.view())
		return
	}
	if len(parts) == 2 && parts[1] == "cancel" {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if !requestCancel(j) {
			writeError(w, http.StatusConflict, "job has already finished")
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	writeError(w, http.StatusNotFound, "job not found")
}

func runJob(j *job, args []string) {
	exe, err := os.Executable()
	if err != nil {
		finishJob(j, err)
		return
	}
	cmd := exec.Command(exe, args...)
	cmd.Env = append(os.Environ(), "GOPAZ_PROGRESS=json")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		finishJob(j, err)
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		finishJob(j, err)
		return
	}

	j.mu.Lock()
	j.cmd = cmd
	cancelled := j.cancelled
	j.mu.Unlock()
	if cancelled {
		finishJob(j, nil)
		return
	}
	if err := cmd.Start(); err != nil {
		finishJob(j, err)
		return
	}
	j.mu.Lock()
	cancelled = j.cancelled
	j.mu.Unlock()
	if cancelled {
		killProcessTree(cmd)
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		scanEvents(stdout, j, "info")
	}()
	go func() {
		defer wg.Done()
		scanEvents(stderr, j, "error")
	}()
	err = cmd.Wait()
	wg.Wait()
	finishJob(j, err)
}

func scanEvents(r io.Reader, j *job, fallbackLevel string) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var ev childEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			j.addLog(fallbackLevel, line)
			continue
		}
		switch ev.Type {
		case "log":
			level := ev.Level
			if level == "" {
				level = fallbackLevel
			}
			j.addLog(level, ev.Msg)
		case "progress":
			j.setProgress(ev.Label, ev.Cur, ev.Total, ev.Pct)
		case "phase":
			j.setPhase(ev.Phase)
		}
	}
	if err := scanner.Err(); err != nil {
		j.addLog(fallbackLevel, "failed to read child process output: "+err.Error())
	}
}

func finishJob(j *job, err error) {
	cancelled := false
	j.mu.Lock()
	switch {
	case j.cancelled:
		j.State = "cancelled"
		cancelled = true
	case err == nil:
		j.State = "done"
		j.Progress.Pct = 100
		if j.Progress.Total > 0 {
			j.Progress.Cur = j.Progress.Total
		}
	default:
		j.State = "error"
		if j.Error == "" {
			j.Error = err.Error()
		}
	}
	j.Finished = time.Now()
	j.Progress.ETA = 0
	j.mu.Unlock()
	if cancelled {
		j.addLog("warn", "Job cancelled")
	}

	jobsMu.Lock()
	if activeJob == j {
		activeJob = nil
	}
	jobsMu.Unlock()

	if j.State != "done" {
		os.Remove(TempVideoPath(j.Output))
		os.Remove(j.Output)
	}
}

func requestCancel(j *job) bool {
	j.mu.Lock()
	if j.State != "running" {
		j.mu.Unlock()
		return false
	}
	j.cancelled = true
	cmd := j.cmd
	j.mu.Unlock()
	if cmd != nil {
		go killProcessTree(cmd)
	}
	return true
}

func cancelActiveJob() {
	jobsMu.Lock()
	j := activeJob
	jobsMu.Unlock()
	if j != nil {
		requestCancel(j)
	}
}

func (j *job) isRunning() bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.State == "running"
}

func (j *job) addLog(level, msg string) {
	if len(msg) > 4000 {
		msg = msg[:4000] + "..."
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	j.Logs = append(j.Logs, jobLog{Time: time.Now().Format("15:04:05"), Level: level, Msg: msg})
	if len(j.Logs) > 500 {
		j.Logs = j.Logs[len(j.Logs)-500:]
	}
	if level == "error" && msg != "" {
		j.Error = msg
	}
}

// setPhase records the current pipeline phase for progress display.
func (j *job) setPhase(phase string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.Progress.Phase = phase
}

func (j *job) setProgress(label string, cur, total int, pct float64) {
	now := time.Now()
	j.mu.Lock()
	defer j.mu.Unlock()
	if label != "" {
		j.Progress.Label = label
	}
	if total > 0 {
		j.Progress.Total = total
	}
	j.Progress.Cur = cur

	if j.lastProgressAt.IsZero() {
		if elapsed := now.Sub(j.Started).Seconds(); elapsed > 0 && cur > 0 {
			j.Progress.Speed = float64(cur) / elapsed
		}
	} else if cur > j.lastCur {
		if dt := now.Sub(j.lastProgressAt).Seconds(); dt > 0 {
			instant := float64(cur-j.lastCur) / dt
			if j.Progress.Speed <= 0 {
				j.Progress.Speed = instant
			} else {
				j.Progress.Speed = 0.7*j.Progress.Speed + 0.3*instant
			}
		}
	}
	j.lastCur = cur
	j.lastProgressAt = now

	if pct < 0 && j.Progress.Total > 0 {
		pct = float64(cur) / float64(j.Progress.Total) * 100
	}
	if pct >= 0 {
		j.Progress.Pct = pct
	}
	if j.Progress.Speed > 0 && j.Progress.Total > cur {
		j.Progress.ETA = float64(j.Progress.Total-cur) / j.Progress.Speed
	} else {
		j.Progress.ETA = 0
	}
}

func (j *job) view() jobView {
	j.mu.Lock()
	defer j.mu.Unlock()
	logs := make([]jobLog, len(j.Logs))
	copy(logs, j.Logs)
	end := j.Finished
	if end.IsZero() {
		end = time.Now()
	}
	started := ""
	if !j.Started.IsZero() {
		started = j.Started.Format(time.RFC3339)
	}
	finished := ""
	if !j.Finished.IsZero() {
		finished = j.Finished.Format(time.RFC3339)
	}
	return jobView{
		ID: j.ID, State: j.State, Input: j.Input, Output: j.Output,
		Progress: j.Progress, Logs: logs, Error: j.Error,
		StartedAt: started, FinishedAt: finished,
		Elapsed: end.Sub(j.Started).Seconds(),
		Preview: j.Preview, PreviewStart: j.PreviewStart,
		PreviewSourceDuration: j.PreviewSourceDuration, PreviewDuration: j.PreviewDuration,
	}
}

func newJobID() string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(b)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
