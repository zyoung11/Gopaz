package main

import (
	"fmt"
	"os"
	"sync"
	"time"
)

// InterpJob describes one interpolation or slowdown task.
type InterpJob struct {
	Input, Output   string
	Ffmpeg, Ffprobe string
	ModelDir        string
	Model           *Model
	Factor          int
	// RDT is the raw duplicate threshold; a negative value uses Sensitivity.
	RDT float64
	// Sensitivity is the Topaz 0-100 control, mapped to rdt as Sensitivity/1000.
	Sensitivity int
	// Instances is the number of concurrent sessions; 0 means auto.
	Instances int
	// Workers is the number of parallel workers; 0 means auto.
	Workers int
	// VramGB caps GPU memory per session in GB.
	VramGB float64
	// DeviceVRAMGB is the Aion tile budget in GB; 0 means auto.
	DeviceVRAMGB         float64
	GPU                  int
	Threads              int
	Overlap              int
	WorkDir              string
	StartFrame, EndFrame int
	Encode               EncodeOptions
	AudioMode            string
	AudioBitrate         int
}

// RunInterpolate runs the interpolation pipeline.
func RunInterpolate(job InterpJob) error {
	startAll := time.Now()
	info, err := ProbeVideo(job.Ffprobe, job.Input)
	if err != nil {
		return err
	}
	fw, fh := info.Width, info.Height
	if fw <= 0 || fh <= 0 {
		return fmt.Errorf("cannot determine input resolution")
	}
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
		}
	}
	if total <= 0 {
		return fmt.Errorf("cannot determine input frames because the container has no nb_frames; use --end")
	}
	n := job.Factor
	if n < 2 {
		return fmt.Errorf("slow motion factor must be at least 2")
	}
	Infof("Input %dx%d, %.3f fps, %d frames, %dx slow motion", fw, fh, info.FPS, total, n)
	if rangeSpecified {
		Infof("Frame range %d-%d, %d frames", fr.Start, fr.End, total)
	}
	audioStart, audioDuration := sourceAudioTrim(info, fr, total, rangeSpecified)

	cfg := UpscaleConfig{
		Model: job.Model, ModelDir: job.ModelDir, Scale: 1,
		FW: fw, FH: fh, Params: map[string]float64{}, Overlap: job.Overlap, Instances: job.Instances, VramGB: job.VramGB,
		DeviceVRAMGB: job.DeviceVRAMGB, GPU: job.GPU, Threads: job.Threads, WorkDir: job.WorkDir, Workers: job.Workers,
	}
	r, err := LoadRunner(cfg)
	if err != nil {
		return err
	}
	defer func() {
		if r != nil {
			r.Close()
		}
	}()
	plan := r.plan
	if plan == nil {
		return fmt.Errorf("failed to create interpolation plan")
	}
	subdiv := r.cfg.Model.OutFrames
	Infof("Using tile %dx%d, %d tiles/batch %d, model %s, %d subframes per inference, sessions %d, workers %d",
		plan.tw, plan.th, len(plan.pos), plan.batch, job.Model.Display, subdiv, len(r.pool), r.workers)

	if job.GPU != -1 {
		if err := probeInterp(r); err != nil {
			Warnf("CUDA probe failed (%v), falling back to CPU", err)
			r.Close()
			r = nil
			cfg.GPU = -1
			cfg.Instances = 1
			cfg.Workers = 1
			r, err = LoadRunner(cfg)
			if err != nil {
				return err
			}
			plan = r.plan
			subdiv = r.cfg.Model.OutFrames
			Infof("CPU mode using tile %dx%d, %d tiles/batch %d", plan.tw, plan.th, len(plan.pos), plan.batch)
		}
	}

	dec, err := NewVideoDecoder(job.Ffmpeg, job.Input, fw, fh, fr.Start, fr.End)
	if err != nil {
		return err
	}
	defer dec.Close()
	reader := startFrameReader(dec, fw, fh, total)
	defer reader.Stop()

	outFPS := info.FPS
	encArgs, err := BuildArgs(job.Encode)
	if err != nil {
		return err
	}
	tmpVideo := TempVideoPath(job.Output)
	enc, err := NewVideoEncoder(job.Ffmpeg, tmpVideo, fw, fh, outFPS, encArgs)
	if err != nil {
		return err
	}

	cache := make(map[int]*Frame)
	nextRead, eofAt := 0, -1
	var lastF *Frame
	ensure := func(idx int) {
		if idx > total-1 {
			idx = total - 1
		}
		for nextRead <= idx {
			if eofAt >= 0 && nextRead >= eofAt {
				cache[nextRead] = lastF
				nextRead++
				continue
			}
			f, ok := reader.Next()
			if !ok {
				eofAt = nextRead
				cache[nextRead] = lastF
				nextRead++
				continue
			}
			cache[nextRead] = f
			lastF = f
			nextRead++
		}
	}
	frameAt := func(idx int) *Frame {
		if idx < 0 {
			idx = 0
		}
		ensure(idx)
		if f, ok := cache[idx]; ok && f != nil {
			return f
		}
		if lastF != nil {
			return lastF
		}
		return NewFrame(fw, fh)
	}

	rdt := job.RDT
	if rdt < 0 {
		rdt = float64(job.Sensitivity) / 1000
	}
	if rdt < 0 {
		rdt = 0
	}
	if rdt > 0 {
		Infof("Duplicate frame threshold rdt=%.4f", rdt)
	}
	workers := make([]*interpWorker, r.workers)
	for i := range workers {
		si := i % len(r.pool)
		var bot *Session
		if si < len(r.bottomPool) {
			bot = r.bottomPool[si]
		}
		workers[i] = newInterpWorker(r, r.pool[si], bot, plan)
	}
	outCount := (total-1)*n + 1
	frameIdx := 0
	writer := startFrameWriter(enc)
	writeOrig := func(f *Frame) {
		writer.Write(f)
		frameIdx++
	}
	dupFrames := 0
	writeOrig(frameAt(0))
	for u0 := 0; u0 < total-1; {
		u1 := u0 + 1
		if rdt > 0 {
			for u1 < total-1 && DuplicateRatio(frameAt(u1-1), frameAt(u1)) < rdt {
				u1++
			}
		}
		gap := u1 - u0
		if gap > 1 {
			dupFrames += gap - 1
		}
		window := []*Frame{frameAt(u0 - 1), frameAt(u0), frameAt(u1), frameAt(u1 + 1)}
		span := gap * n
		unique, order := plan.subFrames(span, subdiv)
		canvases, err := processInterpInterval(r, workers, window, unique, plan)
		if err != nil {
			writer.Close()
			enc.Close()
			os.Remove(tmpVideo)
			return err
		}
		for _, oi := range order {
			writer.WriteNorm(canvases[oi], plan.inv)
			frameIdx++
			if frameIdx%20 == 0 {
				Progress("Interpolating", frameIdx, outCount)
			}
		}
		writeOrig(frameAt(u1))
		for idx := range cache {
			if idx < u0 {
				delete(cache, idx)
			}
		}
		if frameIdx%20 == 0 {
			Progress("Interpolating", frameIdx, outCount)
		}
		u0 = u1
	}
	ProgressDone()
	if err := writer.Close(); err != nil {
		enc.Close()
		if se := enc.Stderr(); se != "" {
			return fmt.Errorf("failed to write encoder: %w; ffmpeg: %s", err, se)
		}
		return err
	}
	if err := reader.Err(); err != nil {
		enc.Close()
		os.Remove(tmpVideo)
		return fmt.Errorf("decode failed: %w", err)
	}
	wall := time.Since(startAll).Seconds()
	Infof("Timing: total %.1fs, inference %.1fs (%.0f%%), other %.1fs", wall, float64(r.inferNanos)/1e9, float64(r.inferNanos)/1e9/wall*100, wall-float64(r.inferNanos)/1e9)
	if dupFrames > 0 {
		Infof("Replaced %d duplicate frames (rdt=%.4f)", dupFrames, rdt)
	}
	if err := enc.Close(); err != nil {
		os.Remove(tmpVideo)
		return fmt.Errorf("encoder finalization failed: %w", err)
	}
	if err := MuxAudio(job.Ffmpeg, tmpVideo, job.Input, job.Output, job.AudioMode, job.AudioBitrate, 1.0/float64(n), audioStart, audioDuration); err != nil {
		os.Remove(tmpVideo)
		return err
	}
	os.Remove(tmpVideo)
	Successf("Done: %s (%d frames, %.3f fps)", job.Output, frameIdx, outFPS)
	return nil
}

// probeInterp checks CUDA inference on a small input and rejects NaN output.
func probeInterp(r *Runner) error {
	plan := r.plan
	if plan == nil {
		return nil
	}
	tiles := plan.pos
	if r.cfg.Model.SingleFile != "" {
		plan = newInterpPlan(128, 128, 128, 128, plan.ramp, 1)
		tiles = plan.pos
	} else if len(tiles) > 1 {
		tiles = tiles[:1]
	}
	win := []*Frame{NewFrame(1, 1), NewFrame(1, 1), NewFrame(1, 1), NewFrame(1, 1)}
	var bot *Session
	if len(r.bottomPool) > 0 {
		bot = r.bottomPool[0]
	}
	w := newInterpWorker(r, r.pool[0], bot, plan)
	out, err := w.run(win, tiles)
	if err != nil {
		return err
	}
	defer out.Destroy()
	n := len(tiles) * plan.th * plan.tw * r.cfg.Model.OutFrames * 3
	view, err := r.outputView(out, n)
	if err != nil {
		return err
	}
	if view.f32 != nil {
		if hasNaN(view.f32) {
			return fmt.Errorf("inference output contains NaN")
		}
		return nil
	}
	if hasNaNF16(view.f16) {
		return fmt.Errorf("inference output contains NaN")
	}
	return nil
}

// processInterpInterval runs inference once per slowdown interval and blends
// every required subframe from that single result.
func processInterpInterval(r *Runner, workers []*interpWorker, window []*Frame, unique []int, plan *interpPlan) ([]*Frame, error) {
	canvases := make([]*Frame, len(unique))
	for i := range canvases {
		canvases[i] = NewFrame(plan.fw, plan.fh)
	}
	if len(unique) == 0 || len(plan.pos) == 0 || len(workers) == 0 {
		return canvases, nil
	}
	batches := make([][]tilePos, 0, (len(plan.pos)+plan.batch-1)/plan.batch)
	for s := 0; s < len(plan.pos); s += plan.batch {
		e := min(s+plan.batch, len(plan.pos))
		batches = append(batches, plan.pos[s:e])
	}
	nw := min(len(workers), len(batches))
	batchCh := make(chan []tilePos)
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
	stride := r.cfg.Model.OutFrames * 3
	for i := range nw {
		w := workers[i]
		wg.Go(func() {
			for b := range batchCh {
				out, err := w.run(window, b)
				if err != nil {
					setErr(err)
					continue
				}
				view, err := r.outputView(out, len(b)*plan.th*plan.tw*stride)
				if err != nil {
					out.Destroy()
					setErr(err)
					continue
				}
				mergeMu.Lock()
				var mg sync.WaitGroup
				for ui := range canvases {
					mg.Go(func() {
						mergeSubframe(canvases[ui], view, b, unique[ui], stride, plan)
					})
				}
				mg.Wait()
				mergeMu.Unlock()
				out.Destroy()
			}
		})
	}
	for _, b := range batches {
		batchCh <- b
	}
	close(batchCh)
	wg.Wait()
	return canvases, firstErr
}
