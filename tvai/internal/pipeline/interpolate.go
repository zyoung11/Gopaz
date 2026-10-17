package pipeline

import (
	"fmt"
	"io"
	"math"
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
	Instances            int     // 并发实例数
	VramGB               float64 // 每会话显存上限(GB)
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
	total := info.Frames
	// 若指定帧范围，解码器会只选出范围内帧，需相应调整总帧数。
	if job.StartFrame >= 0 || job.EndFrame >= 0 {
		s, e := job.StartFrame, job.EndFrame
		if s < 0 {
			s = 0
		}
		if e < 0 {
			e = info.Frames - 1
		}
		if info.Frames > 0 && e > info.Frames-1 {
			e = info.Frames - 1
		}
		if s <= e {
			total = e - s + 1
		}
	}
	if total <= 0 {
		return fmt.Errorf("无法确定输入帧数（容器未提供 nb_frames）")
	}
	n := job.Factor
	if n < 2 {
		return fmt.Errorf("慢放倍数需 >= 2")
	}
	log.Infof("输入 %dx%d, %.3f fps, %d 帧, 慢放 %dx", fw, fh, info.FPS, total, n)

	// 插帧不改分辨率：scale 空间为 1，仅时间轴加密。
	cfg := UpscaleConfig{
		Model: job.Model, ModelDir: job.ModelDir, Scale: 1,
		FW: fw, FH: fh, Params: map[string]float64{}, Overlap: job.Overlap, Instances: job.Instances, VramGB: job.VramGB,
		GPU: job.GPU, Threads: job.Threads, WorkDir: job.WorkDir,
	}
	r, err := LoadRunner(cfg)
	if err != nil {
		return err
	}
	defer func() { r.Close() }()
	th, tw := r.TileSize()
	subdiv := r.cfg.Model.OutFrames // Apollo=8, Aion=16
	log.Infof("使用 tile %dx%d, 模型 %s, 每次推理产出 %d 子帧", th, tw, job.Model.Display, subdiv)

	dec, err := video.NewDecoder(job.Ffmpeg, job.Input, fw, fh, job.StartFrame, job.EndFrame)
	if err != nil {
		return err
	}
	defer dec.Close()

	outFPS := info.FPS * float64(n)
	encArgs, err := encode.BuildArgs(job.Encode)
	if err != nil {
		return err
	}
	tmpVideo := TempVideoPath(job.Output)
	enc, err := video.NewEncoder(job.Ffmpeg, tmpVideo, fw, fh, outFPS, encArgs)
	if err != nil {
		return err
	}

	// 帧缓存（带前瞻），支持 [j-1, j+2] 窗口。
	cache := make(map[int]*Frame)
	nextRead, eofAt := 0, -1
	var lastF *Frame
	buf := make([]byte, fw*fh*3)
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
			if err := dec.ReadFrame(buf); err == io.EOF {
				eofAt = nextRead
				cache[nextRead] = lastF
				nextRead++
				continue
			} else if err != nil {
				cache[nextRead] = lastF
				nextRead++
				continue
			}
			f := FromRGB24(buf, fw, fh)
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
		return lastF
	}

	overlap := r.Overlap()
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
	// 验证 CUDA 推理是否产出有效值：部分模型（如 Apollo）在个别 GPU 上会
	// 产出 NaN。若检测到则回退到 CPU（较慢但正确）。
	if job.GPU != -1 {
		probeWin := []*Frame{frameAt(-1), frameAt(0), frameAt(1), frameAt(2)}
		pf, perr := processInterpFrame(r, probeWin, fw, fh, th, tw, 0, overlap)
		if perr != nil || (pf != nil && hasNaN(pf.Data)) {
			log.Warnf("CUDA 推理不可用（%v），回退到 CPU", perr)
			r.Close()
			cfg.GPU = -1
			r, err = LoadRunner(cfg)
			if err != nil {
				return err
			}
		}
	}
	outCount := (total-1)*n + 1
	frameIdx := 0
	writeOrig := func(f *Frame) error {
		if err := enc.WriteFrame(f.ToRGB24()); err != nil {
			return err
		}
		frameIdx++
		return nil
	}

	dupFrames := 0
	if err := writeOrig(frameAt(0)); err != nil {
		enc.Close()
		return err
	}
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
		for i := 1; i < span; i++ {
			t := float64(i) / float64(span)
			subIdx := min(max(int(math.Round(t*float64(subdiv)))-1, 0), subdiv-1)
			interp, err := processInterpFrame(r, window, fw, fh, th, tw, subIdx, overlap)
			if err != nil {
				enc.Close()
				return err
			}
			if err := writeOrig(interp); err != nil {
				enc.Close()
				return err
			}
			if frameIdx%20 == 0 {
				log.Progress("插帧", frameIdx, outCount)
			}
		}
		if err := writeOrig(frameAt(u1)); err != nil {
			enc.Close()
			return err
		}
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
	wall := time.Since(startAll).Seconds()
	log.Infof("耗时分解: 总 %.1fs, 模型推理 %.1fs (%.0f%%), 其余 %.1fs", wall, float64(r.inferNanos)/1e9, float64(r.inferNanos)/1e9/wall*100, wall-float64(r.inferNanos)/1e9)
	if dupFrames > 0 {
		log.Infof("替换重复帧 %d 个 (rdt=%.4f)", dupFrames, rdt)
	}
	if err := enc.Close(); err != nil {
		os.Remove(tmpVideo)
		return fmt.Errorf("编码收尾失败: %w", err)
	}
	if err := MuxAudio(job.Ffmpeg, tmpVideo, job.Input, job.Output, job.AudioMode, job.AudioBitrate); err != nil {
		os.Remove(tmpVideo)
		return err
	}
	os.Remove(tmpVideo)
	log.Successf("完成: %s (%d 帧, %.3f fps)", job.Output, frameIdx, outFPS)
	return nil
}

// processInterpFrame 对一帧插值结果做分块推理 + 重叠融合。
func processInterpFrame(r *Runner, window []*Frame, fw, fh, th, tw, subIdx, overlap int) (*Frame, error) {
	canvas := NewFrame(fw, fh)
	weight := NewFrame(fw, fh)
	stepX := tw - overlap
	stepY := th - overlap
	if stepX <= 0 {
		stepX = tw
	}
	if stepY <= 0 {
		stepY = th
	}
	// 1. 收集 tile 位置。
	var xs, ys []int
	for y := 0; y < fh; y += stepY {
		for x := 0; x < fw; x += stepX {
			xs = append(xs, x)
			ys = append(ys, y)
		}
	}
	np := len(xs)
	if np == 0 {
		return canvas, nil
	}
	// 2. 提取所有 tile（每 tile 内部并行）。
	tileWins := make([][]*Frame, np)
	for i := range np {
		win := make([]*Frame, len(window))
		for j, f := range window {
			win[j] = f.ExtractTile(xs[i], ys[i], tw, th)
		}
		tileWins[i] = win
	}
	// 3. 分批 + 跨会话并发推理（保持 GPU 喂饱）。
	nInst := max(len(r.pool), 1)
	batchSize := r.BatchLimit()
	type batch struct{ idx []int }
	var batches []batch
	for s := 0; s < np; s += batchSize {
		e := min(s+batchSize, np)
		idx := make([]int, 0, e-s)
		for i := s; i < e; i++ {
			idx = append(idx, i)
		}
		batches = append(batches, batch{idx})
	}
	results := make([]*Frame, np)
	sessCh := make(chan int, nInst)
	for i := range nInst {
		sessCh <- i
	}
	batchCh := make(chan batch)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	workers := min(nInst, len(batches))
	for range workers {
		wg.Go(func() {
			for b := range batchCh {
				si := <-sessCh
				main := r.pool[si]
				var bot *ort.Session
				if si < len(r.bottomPool) {
					bot = r.bottomPool[si]
				}
				tilesBatch := make([][]*Frame, len(b.idx))
				for k, ti := range b.idx {
					tilesBatch[k] = tileWins[ti]
				}
				raws, err := r.runInterpBatch(main, bot, tilesBatch)
				sessCh <- si
				if err != nil {
					mu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					mu.Unlock()
					continue
				}
				for k, ti := range b.idx {
					sub := ExtractSubFrame(raws[k], subIdx)
					mu.Lock()
					results[ti] = sub
					mu.Unlock()
				}
			}
		})
	}
	for _, b := range batches {
		batchCh <- b
	}
	close(batchCh)
	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	// 4. 融合（每 tile 内部并行）。
	ramp := overlap
	for i := range np {
		sub := results[i]
		if sub == nil {
			continue
		}
		r := ramp
		if r > sub.W {
			r = sub.W / 4
		}
		if r > sub.H {
			r = sub.H / 4
		}
		tileW := NewFrame(sub.W, sub.H)
		FillRampWeight(tileW, r)
		MergeTileWeighted(canvas, weight, sub, tileW, xs[i], ys[i])
	}
	NormalizeCanvas(canvas, weight)
	return canvas, nil
}
