package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// UpscaleJob describes one upscale or restoration task.
type UpscaleJob struct {
	Input, Output   string
	Ffmpeg, Ffprobe string
	ModelDir        string
	Model           *Model
	Scale           int
	Params          map[string]float64
	Auto            bool
	GPU             int
	Threads         int
	Workers         int
	VramGB          float64
	Overlap         int
	Provider        Provider
	VideoDevice     string
	Device          string
	// KColor enables Lab color correction when the model uses keepColor.
	KColor bool
	// SceneThreshold is the scene change threshold; 0 uses the default 6.5.
	SceneThreshold       float64
	StartFrame, EndFrame int
	Encode               EncodeOptions
	AudioMode            string
	AudioBitrate         int
}

// runnerConfig builds the runner configuration for this job.
func (job UpscaleJob) runnerConfig(fw, fh int) UpscaleConfig {
	return UpscaleConfig{
		Model: job.Model, ModelDir: job.ModelDir, Scale: job.Scale,
		FW: fw, FH: fh, Params: job.Params, Overlap: job.Overlap,
		SceneThreshold: job.SceneThreshold,
		Auto:           job.Auto,
		GPU:            job.GPU, Threads: job.Threads, Workers: job.Workers, VramGB: job.VramGB,
		Provider: job.Provider, Device: job.Device,
	}
}

// RunUpscale runs the upscale and restoration pipeline.
func RunUpscale(job UpscaleJob) error {
	info, err := ProbeVideo(job.Ffprobe, job.Input)
	if err != nil {
		return err
	}
	fw, fh := info.Width, info.Height
	if fw <= 0 || fh <= 0 {
		return fmt.Errorf("cannot determine input resolution")
	}
	Infof("Input %dx%d, %.3f fps, %d frames", fw, fh, info.FPS, info.Frames)

	fr := ResolveFrameRange(job.StartFrame, job.EndFrame, info.Frames)
	rangeSpecified := job.StartFrame >= 0 || job.EndFrame >= 0
	total := max(info.Frames, 0)
	if rangeSpecified {
		if fr.Count > 0 {
			total = fr.Count
		} else if info.Frames > 0 {
			return fmt.Errorf("frame range is invalid or exceeds video length")
		} else if job.StartFrame >= 0 && job.EndFrame >= 0 {
			return fmt.Errorf("invalid frame range")
		} else {
			total = 0
		}
	}
	if rangeSpecified && total > 0 {
		Infof("Frame range %d-%d, %d frames", fr.Start, fr.End, total)
	}
	audioStart, audioDuration := sourceAudioTrim(info, fr, total, rangeSpecified)
	if err := os.MkdirAll(filepath.Dir(job.Output), 0o755); err != nil {
		return fmt.Errorf("failed to create output folder: %w", err)
	}

	so := job.Model.Temporal
	if so <= 0 {
		so = 1
	}

	cfg := job.runnerConfig(fw, fh)
	r, err := LoadRunner(cfg)
	if err != nil {
		return err
	}
	defer func() {
		if r != nil {
			r.Close()
		}
	}()
	th, tw := r.TileSize()
	Infof("Using tile %dx%d, model %s, workers %d", tw, th, job.Model.Display, r.workers)
	if job.GPU != -1 {
		switch r.cfg.Provider {
		case ProviderCUDA:
			if err := probeEnhance(r); err != nil {
				Warnf("CUDA probe failed (%v), falling back to CPU", err)
				r.Close()
				r = nil
				cfg.GPU = -1
				cfg.Workers = 1
				r, err = LoadRunner(cfg)
				if err != nil {
					return err
				}
				th, tw = r.TileSize()
				Infof("CPU mode using tile %dx%d, model %s, workers %d", tw, th, job.Model.Display, r.workers)
			}
		case ProviderTensorRT:
			if err := warmupEnhance(r); err != nil {
				return err
			}
		}
	}

	outW, outH := fw*job.Scale, fh*job.Scale
	if err := ValidateEncoderSize(job.Encode.EncoderID, outW, outH); err != nil {
		return err
	}
	encArgs, err := BuildArgs(job.Encode)
	if err != nil {
		return err
	}
	tmpVideo := TempVideoPath(job.Output)
	dec, err := NewVideoDecoder(job.Ffmpeg, job.Input, fw, fh, fr.Start, fr.End, job.VideoDevice)
	if err != nil {
		return err
	}
	defer dec.Close()

	enc, err := NewVideoEncoder(job.Ffmpeg, tmpVideo, outW, outH, info.FPS, encArgs)
	if err != nil {
		return err
	}
	reader := startFrameReader(dec, fw, fh, total)
	defer reader.Stop()
	writer := startFrameWriter(enc)

	center := job.Model.CenterIdx
	if center >= so {
		center = so - 1
	}
	lookAhead := so - center - 1
	ring := make([]*Frame, so)
	var lastFrame *Frame
	readPos, emitPos := 0, 0

	frameIdx := 0
	needTemporal := r.HasInput("hr_prev")

	readMore := func() bool {
		if total > 0 && readPos >= total {
			return false
		}
		f, ok := reader.Next()
		if !ok {
			return false
		}
		if so > 1 {
			ring[readPos%so] = f
		} else if so == 1 {
			ring[0] = f
		}
		lastFrame = f
		readPos++
		return true
	}

	frameAt := func(idx int) *Frame {
		if idx < 0 {
			idx = 0
		}
		if total > 0 && idx >= total {
			idx = total - 1
		}
		if idx >= readPos {
			idx = readPos - 1
		}
		if idx < 0 || lastFrame == nil {
			return NewFrame(fw, fh)
		}
		if so <= 1 {
			if ring[0] == nil {
				return lastFrame
			}
			return ring[0]
		}
		f := ring[idx%so]
		if f == nil {
			return lastFrame
		}
		return f
	}

	for {
		for readPos <= emitPos+lookAhead {
			if !readMore() {
				break
			}
		}
		if total > 0 && emitPos >= total {
			break
		}
		if total == 0 && emitPos >= readPos {
			break
		}
		window := make([]*Frame, so)
		for j := 0; j < so; j++ {
			window[j] = frameAt(emitPos - center + j)
		}
		centerFrame := frameAt(emitPos)
		if r.autoSess != nil && center > 0 && center < len(window) {
			if aerr := r.autoParams(window[center-1], window[center]); aerr != nil {
				Warnf("auto parameters failed: %v", aerr)
			}
		}
		if needTemporal && r.prevHR == nil {
			r.ResetTemporal(centerFrame.UpscaleBilinear(outW, outH))
		}
		rawFrame, err := processFrame(r, window)
		if err != nil {
			writer.Close()
			enc.Close()
			os.Remove(tmpVideo)
			return err
		}
		outFrame := rawFrame
		if job.Model.KeepColor && job.KColor {
			outFrame = rawFrame.Clone()
			ColorTransferLab(centerFrame, outFrame)
		}
		writer.Write(outFrame)
		if needTemporal {
			r.SetPrevOutput(rawFrame)
		}
		frameIdx++
		emitPos++
		if frameIdx%10 == 0 {
			if total > 0 {
				Progress("Enhancing", frameIdx, total)
			} else {
				Progress("Enhancing", frameIdx, -1)
			}
		}
	}
	ProgressDone()
	if dec.Frames() == 0 {
		writer.Close()
		enc.Close()
		os.Remove(tmpVideo)
		return fmt.Errorf("decoder produced no frames; try another video engine or software decoding; ffmpeg: %s", dec.Stderr())
	}
	if err := writer.Close(); err != nil {
		enc.Close()
		if se := enc.Stderr(); se != "" {
			return fmt.Errorf("failed to write encoder: %w; ffmpeg: %s%s", err, se, encoderHint(se))
		}
		return err
	}
	if err := reader.Err(); err != nil {
		dec.Close()
		enc.Close()
		os.Remove(tmpVideo)
		if se := dec.Stderr(); se != "" {
			return fmt.Errorf("decode failed: %w; ffmpeg: %s", err, se)
		}
		return fmt.Errorf("decode failed: %w", err)
	}
	if err := enc.Close(); err != nil {
		os.Remove(tmpVideo)
		return fmt.Errorf("encoder finalization failed: %w", err)
	}
	if err := MuxAudio(job.Ffmpeg, tmpVideo, job.Input, job.Output, job.AudioMode, job.AudioBitrate, 1, audioStart, audioDuration); err != nil {
		os.Remove(tmpVideo)
		return err
	}
	os.Remove(tmpVideo)
	Successf("Done: %s (%d frames, %dx%d)", job.Output, frameIdx, outW, outH)
	return nil
}

// probeEnhance runs one full-tile inference and rejects NaN output.
func probeEnhance(r *Runner) error {
	th, tw := r.TileSize()
	n := max(r.cfg.Model.Temporal, 1)
	tiles := make([]*Frame, n)
	for i := range tiles {
		tiles[i] = NewFrame(tw, th)
	}
	out, err := r.ProcessTile(tiles, r.cfg.Scale, 0, 0)
	if err != nil {
		return err
	}
	if hasNaN(out.Data) {
		return fmt.Errorf("inference output contains NaN")
	}
	return nil
}

// warmupEnhance runs one full tile through TensorRT to build its engine.
func warmupEnhance(r *Runner) error {
	th, tw := r.TileSize()
	Infof("Warming up TensorRT engine for tile %dx%d; the first run may take several minutes", tw, th)
	start := time.Now()
	if err := probeEnhance(r); err != nil {
		return fmt.Errorf("TensorRT warmup failed: %w", err)
	}
	Infof("TensorRT engine ready in %.1fs", time.Since(start).Seconds())
	return nil
}

// processFrame fully processes one frame with parallel tile inference and
// precomputed weight blending.
func processFrame(r *Runner, window []*Frame) (*Frame, error) {
	p := r.enhPlan
	if p == nil {
		return nil, fmt.Errorf("enhance plan is not initialized")
	}
	canvas := NewFrame(p.outW, p.outH)
	nw := min(r.workers, len(p.pos))
	if nw <= 0 {
		return canvas, nil
	}
	tasks := make(chan tilePos)
	var wg sync.WaitGroup
	var mergeMu sync.Mutex
	var errMu sync.Mutex
	var firstErr error
	setErr := func(err error) {
		errMu.Lock()
		if firstErr == nil {
			firstErr = err
		}
		errMu.Unlock()
	}
	for range nw {
		wg.Go(func() {
			for tp := range tasks {
				tiles := make([]*Frame, len(window))
				for j, f := range window {
					tiles[j] = f.ExtractTile(tp.x, tp.y, p.tw, p.th)
				}
				out, err := r.ProcessTile(tiles, p.scale, tp.x*p.scale, tp.y*p.scale)
				if err != nil {
					setErr(err)
					continue
				}
				mergeMu.Lock()
				addWeightedTile(canvas, out, p.weight, tp.x*p.scale, tp.y*p.scale)
				mergeMu.Unlock()
			}
		})
	}
	for _, tp := range p.pos {
		tasks <- tp
	}
	close(tasks)
	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	normalizeCanvasInv(canvas, p.inv)
	return canvas, nil
}
