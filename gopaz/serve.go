package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed web
var webFiles embed.FS

var videoExtensions = []string{
	"mp4", "mkv", "mov", "avi", "webm", "m4v", "ts", "m2ts", "mts", "mxf", "wmv", "flv", "mpg", "mpeg", "3gp", "ogv",
}

var (
	mediaMu      sync.Mutex
	mediaAllowed = map[string]bool{}
)

type modelInfo struct {
	Key            string      `json:"key"`
	Display        string      `json:"display"`
	Kind           string      `json:"kind"`
	Scales         []int       `json:"scales"`
	Params         []paramInfo `json:"params"`
	NPUUnsupported bool        `json:"npuUnsupported"`
}

type paramInfo struct {
	Key     string `json:"key"`
	GUIName string `json:"guiName"`
}

type encoderInfo struct {
	ID          string   `json:"id"`
	Desc        string   `json:"desc"`
	Extensions  []string `json:"extensions"`
	HasCQP      bool     `json:"hasCqp"`
	RateControl bool     `json:"rateControl"`
}

type providerInfo struct {
	ID      string       `json:"id"`
	Display string       `json:"display"`
	Devices []deviceInfo `json:"devices"`
}

type defaultsInfo struct {
	Encoder      string `json:"encoder"`
	Quality      string `json:"quality"`
	RateControl  string `json:"rateControl"`
	AudioCodec   string `json:"audioCodec"`
	AudioBitrate int    `json:"audioBitrate"`
}

type configResponse struct {
	Models    []modelInfo    `json:"models"`
	Encoders  []encoderInfo  `json:"encoders"`
	Providers []providerInfo `json:"providers"`
	GPUs      []deviceInfo   `json:"gpus"`
	Defaults  defaultsInfo   `json:"defaults"`
}

type pathRequest struct {
	Path string `json:"path"`
}

type pathResponse struct {
	Path      string `json:"path,omitempty"`
	Cancelled bool   `json:"cancelled,omitempty"`
	Error     string `json:"error,omitempty"`
}

type probeResponse struct {
	Path     string  `json:"path"`
	Name     string  `json:"name"`
	Dir      string  `json:"dir"`
	Size     int64   `json:"size"`
	Width    int     `json:"width"`
	Height   int     `json:"height"`
	FPS      float64 `json:"fps"`
	Frames   int     `json:"frames"`
	Duration float64 `json:"duration"`
	HasAudio bool    `json:"hasAudio"`
	Rotation int     `json:"rotation"`
}

type outputPreview struct {
	Path string `json:"path"`
	Name string `json:"name"`
	Dir  string `json:"dir"`
}

func cmdServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	port := fs.Int("port", 8765, "listen port")
	noOpen := fs.Bool("no-open", false, "do not open the browser")
	fs.Parse(args)

	ln, err := listenLocal(*port)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("http://127.0.0.1:%d", ln.Addr().(*net.TCPAddr).Port)
	Infof("Web UI: %s", url)
	if !*noOpen {
		if err := openBrowser(url); err != nil {
			Warnf("could not open the browser: %v", err)
		}
	}

	srv := &http.Server{Handler: newMux(), ReadHeaderTimeout: 10 * time.Second}
	defer cleanupPreviewDir()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	go func() {
		<-ctx.Done()
		cancelActiveJob()
		shutdown, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		srv.Shutdown(shutdown)
	}()

	err = srv.Serve(ln)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func listenLocal(preferred int) (net.Listener, error) {
	if preferred <= 0 {
		return net.Listen("tcp", "127.0.0.1:0")
	}
	var lastErr error
	for i := range 10 {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", preferred+i))
		if err == nil {
			return ln, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("ports %d-%d are all unavailable: %w", preferred, preferred+9, lastErr)
}

func openBrowser(url string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("cmd", "/c", "start", "", url).Start()
	case "darwin":
		return exec.Command("open", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
}

func newMux() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/config", handleConfig)
	mux.HandleFunc("/api/probe", handleProbe)
	mux.HandleFunc("/api/pick/file", handlePickFile)
	mux.HandleFunc("/api/pick/folder", handlePickFolder)
	mux.HandleFunc("/api/preview-output", handlePreviewOutput)
	mux.HandleFunc("/api/media", handleMedia)
	mux.HandleFunc("/api/jobs", handleCreateJob)
	mux.HandleFunc("/api/jobs/", handleJobRoute)
	mux.HandleFunc("/api/reveal", handleReveal)
	mux.Handle("/", webHandler())
	return mux
}

func webHandler() http.Handler {
	sub, err := fs.Sub(webFiles, "web")
	if err != nil {
		panic(err)
	}
	fileServer := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		fileServer.ServeHTTP(w, r)
	})
}

func handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var out configResponse
	for _, key := range ModelKeys() {
		m := ModelRegistry()[key]
		kind := "enhance"
		if m.Kind == KindInterp {
			kind = "interp"
		}
		item := modelInfo{Key: m.Key, Display: m.Display, Kind: kind, Scales: m.Scales, NPUUnsupported: m.NPUUnsupported}
		for _, p := range m.Params {
			item.Params = append(item.Params, paramInfo{Key: p.Key, GUIName: p.GUIName})
		}
		out.Models = append(out.Models, item)
	}
	for _, id := range EncoderIDs() {
		e := EncoderRegistry()[id]
		out.Encoders = append(out.Encoders, encoderInfo{
			ID: e.ID, Desc: e.Desc, Extensions: e.Extensions,
			HasCQP: e.HasCQP, RateControl: e.RateControl,
		})
	}
	out.Defaults = defaultsInfo{
		Encoder: defaultEncoder(localTool("ffmpeg")), Quality: "High",
		RateControl: "cq", AudioCodec: "copy", AudioBitrate: 320,
	}
	gpus := cudaDevices()
	dml := dmlDevices()
	out.GPUs = gpus
	if len(out.GPUs) == 0 {
		out.GPUs = dml
	}
	out.Providers = []providerInfo{
		{ID: "cuda", Display: "CUDA", Devices: gpus},
		{ID: "tensorrt", Display: "TensorRT", Devices: gpus},
		{ID: "dml", Display: "DirectML", Devices: dml},
		{ID: "openvino", Display: "OpenVINO", Devices: openVINODevices()},
	}
	writeJSON(w, http.StatusOK, out)
}

func handleProbe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req pathRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	path := normalizePath(req.Path)
	if path == "" {
		writeError(w, http.StatusBadRequest, "provide a video path")
		return
	}
	st, err := os.Stat(path)
	if err != nil {
		writeError(w, http.StatusBadRequest, "file does not exist or is not accessible")
		return
	}
	if st.IsDir() {
		writeError(w, http.StatusBadRequest, "path is a folder; select a video file")
		return
	}
	info, err := ProbeVideo(localTool("ffprobe"), path)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read video info: "+err.Error())
		return
	}
	allowMediaPath(path)
	writeJSON(w, http.StatusOK, probeResponse{
		Path: path, Name: filepath.Base(path), Dir: filepath.Dir(path), Size: st.Size(),
		Width: info.Width, Height: info.Height, FPS: info.FPS, Frames: info.Frames,
		Duration: info.Duration, HasAudio: info.HasAudio, Rotation: info.Rotation,
	})
}

func handlePickFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	path, err := pickVideoFile()
	if err != nil {
		if isPickerCancel(err) {
			writeJSON(w, http.StatusOK, pathResponse{Cancelled: true})
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, pathResponse{Path: path})
}

func handlePickFolder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	path, err := pickOutputFolder()
	if err != nil {
		if isPickerCancel(err) {
			writeJSON(w, http.StatusOK, pathResponse{Cancelled: true})
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, pathResponse{Path: path})
}

func isPickerCancel(err error) bool {
	return strings.Contains(strings.ToLower(err.Error()), "cancel")
}

func handlePreviewOutput(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	req := defaultJobRequest()
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	plan, err := resolveOutput(&req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, outputPreview{
		Path: plan.OutputPath, Name: filepath.Base(plan.OutputPath), Dir: filepath.Dir(plan.OutputPath),
	})
}

func handleReveal(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req pathRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	path := normalizePath(req.Path)
	st, err := os.Stat(path)
	if err != nil {
		writeError(w, http.StatusBadRequest, "path does not exist")
		return
	}
	if err := revealPath(path, st.IsDir()); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func revealPath(path string, isDir bool) error {
	switch runtime.GOOS {
	case "windows":
		if isDir {
			return exec.Command("explorer", path).Start()
		}
		return exec.Command("explorer", "/select,", path).Start()
	case "darwin":
		if isDir {
			return exec.Command("open", path).Start()
		}
		return exec.Command("open", "-R", path).Start()
	default:
		if isDir {
			return exec.Command("xdg-open", path).Start()
		}
		return exec.Command("xdg-open", filepath.Dir(path)).Start()
	}
}

func normalizePath(path string) string {
	path = strings.TrimSpace(path)
	path = strings.Trim(path, "\"")
	if path == "" {
		return ""
	}
	return filepath.Clean(path)
}

func decodeJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	return json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(v)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func boolInt(v bool) string {
	if v {
		return "1"
	}
	return "0"
}

func ftoa(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func itoa(v int) string {
	return strconv.Itoa(v)
}

func allowMediaPath(path string) string {
	path = normalizePath(path)
	if path == "" {
		return path
	}
	absolute, err := filepath.Abs(path)
	if err == nil {
		path = absolute
	}
	mediaMu.Lock()
	mediaAllowed[path] = true
	mediaMu.Unlock()
	return path
}

func mediaPathAllowed(path string) bool {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	mediaMu.Lock()
	allowed := mediaAllowed[absolute]
	mediaMu.Unlock()
	return allowed
}

func handleMedia(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	path := normalizePath(r.URL.Query().Get("path"))
	if path == "" || !mediaPathAllowed(path) {
		writeError(w, http.StatusForbidden, "media path is not allowed")
		return
	}
	file, err := os.Open(path)
	if err != nil {
		writeError(w, http.StatusNotFound, "media file not found")
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info.IsDir() {
		writeError(w, http.StatusNotFound, "media file not found")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	http.ServeContent(w, r, filepath.Base(path), info.ModTime(), file)
}
