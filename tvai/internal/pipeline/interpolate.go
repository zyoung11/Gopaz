package pipeline

import (
	"fmt"
	"os"
	"sync"
	"time"

	"tvai/internal/encode"
	"tvai/internal/log"
	"tvai/internal/model"
	"tvai/internal/ort"
	"tvai/internal/video"
)

// InterpJob 描述一次插帧/慢放任务。
type InterpJob struct {
	Input, Output        string
	Ffmpeg, Ffprobe      string
	ModelDir             string
	Model                *model.Model
	Factor               int
	RDT                  float64 // 原始去重阈值；<0 时使用 Sensitivity
	Sensitivity          int     // Topaz Sensitivity 0-100，rdt=Sensitivity/1000
	Instances            int     // 并发实例数，0=自动
	Workers              int     // 并行 worker 数，0=自动
	VramGB               float64 // 每会话显存上限(GB)
	DeviceVRAMGB         float64 // Aion tile 预算(GB)，0=自动
	GPU                  int
	Threads              int
	Overlap              int
	WorkDir              string
	StartFrame, EndFrame int
	Encode               encode.Options
	AudioMode            string
	AudioBitrate         int
}

// RunInterpolate 执行插帧/慢放主流程。
func RunInterpolate(job InterpJob) error {
	startAll := time.Now()
	info, err := video.Probe(job.Ffprobe, job.Input)
	if err != nil {
		return err
	}
	fw, fh := info.Width, info.Height
	if fw <= 0 || fh <= 0 {
		return fmt.Errorf("无法确定输入分辨率")
	}
	fr := ResolveFrameRange(job.StartFrame, job.EndFrame, info.Frames)
	rangeSpecified := job.StartFrame >= 0 || job.EndFrame >= 0
	total := max(info.Frames, 0)
	if rangeSpecified {
		if fr.Count > 0 {
			total = fr.Count
		} else if info.Frames > 0 {
			return fmt.Errorf("帧范围无效或超出视频长度")
		} else if job.StartFrame >= 0 && job.EndFrame >= 0 {
			return fmt.Errorf("帧范围无效")
		}
	}
	if total <= 0 {
		return fmt.Errorf("无法确定输入帧数（容器未提供 nb_frames），请用 --end 指定范围")
	}
	n := job.Factor
	if n < 2 {
		return fmt.Errorf("慢放倍数需 >= 2")
	}
	log.Infof("输入 %dx%d, %.3f fps, %d 帧, 慢放 %dx", fw, fh, info.FPS, total, n)
	if rangeSpecified {
		log.Infof("帧范围 %d-%d, 共 %d 帧", fr.Start, fr.End, total)
	}

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
		return fmt.Errorf("插帧计划创建失败")
	}
	subdiv := r.cfg.Model.OutFrames
	log.Infof("使用 tile %dx%d, %d 块/批 %d, 模型 %s, 每次推理产出 %d 子帧, 会话 %d, worker %d",
		plan.tw, plan.th, len(plan.pos), plan.batch, job.Model.Display, subdiv, len(r.pool), r.workers)

	if job.GPU != -1 {
		if err := probeInterp(r); err != nil {
			log.Warnf("CUDA 推理探测失败（%v），回退到 CPU", err)
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
			log.Infof("CPU 模式使用 tile %dx%d, %d 块/批 %d", plan.tw, plan.th, len(plan.pos), plan.batch)
		}
	}

	dec, err := video.NewDecoder(job.Ffmpeg, job.Input, fw, fh, fr.Start, fr.End)
	if err != nil {
		return err
	}
	defer dec.Close()
	reader := startFrameReader(dec, fw, fh, total)
	defer reader.Stop()

	outFPS := info.FPS
	encArgs, err := encode.BuildArgs(job.Encode)
	if err != nil {
		return err
	}
	tmpVideo := TempVideoPath(job.Output)
	enc, err := video.NewEncoder(job.Ffmpeg, tmpVideo, fw, fh, outFPS, encArgs)
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
		log.Infof("重复帧替换阈值 rdt=%.4f", rdt)
	}
	workers := make([]*interpWorker, r.workers)
	for i := range workers {
		si := i % len(r.pool)
		var bot *ort.Session
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
			for u1 < total-1 && SceneDiff(frameAt(u1-1), frameAt(u1)) < rdt {
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
				log.Progress("插帧", frameIdx, outCount)
			}
		}
		writeOrig(frameAt(u1))
		for idx := range cache {
			if idx < u0 {
				delete(cache, idx)
			}
		}
		if frameIdx%20 == 0 {
			log.Progress("插帧", frameIdx, outCount)
		}
		u0 = u1
	}
	log.ProgressDone()
	if err := writer.Close(); err != nil {
		enc.Close()
		if se := enc.Stderr(); se != "" {
			return fmt.Errorf("写入编码器失败: %w; ffmpeg: %s", err, se)
		}
		return err
	}
	if err := reader.Err(); err != nil {
		enc.Close()
		os.Remove(tmpVideo)
		return fmt.Errorf("解码失败: %w", err)
	}
	wall := time.Since(startAll).Seconds()
	log.Infof("耗时分解: 总 %.1fs, 模型推理 %.1fs (%.0f%%), 其余 %.1fs", wall, float64(r.inferNanos)/1e9, float64(r.inferNanos)/1e9/wall*100, wall-float64(r.inferNanos)/1e9)
	if dupFrames > 0 {
		log.Infof("替换重复帧 %d 个 (rdt=%.4f)", dupFrames, rdt)
	}
	if err := enc.Close(); err != nil {
		os.Remove(tmpVideo)
		return fmt.Errorf("编码收尾失败: %w", err)
	}
	if err := MuxAudio(job.Ffmpeg, tmpVideo, job.Input, job.Output, job.AudioMode, job.AudioBitrate, 1.0/float64(n)); err != nil {
		os.Remove(tmpVideo)
		return err
	}
	os.Remove(tmpVideo)
	log.Successf("完成: %s (%d 帧, %.3f fps)", job.Output, frameIdx, outFPS)
	return nil
}

// probeInterp 用一小块输入确认 CUDA 推理可用且不产生 NaN。
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
	var bot *ort.Session
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
			return fmt.Errorf("推理输出包含 NaN")
		}
		return nil
	}
	if hasNaNF16(view.f16) {
		return fmt.Errorf("推理输出包含 NaN")
	}
	return nil
}

// processInterpInterval 对一个慢放区间只做一轮推理，再把所有需要的子帧融合出来。
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
