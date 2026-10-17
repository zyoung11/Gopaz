package pipeline

import (
	"fmt"
	"io"
	"math"
	"os"

	"tvai/internal/encode"
	"tvai/internal/log"
	"tvai/internal/model"
	"tvai/internal/video"
)

// InterpJob 描述一次插帧/慢放任务。
type InterpJob struct {
	Input, Output string
	Ffmpeg, Ffprobe string
	ModelDir string
	Model    *model.Model
	Factor   int
	GPU      int
	Threads  int
	Overlap  int
	WorkDir  string
	StartFrame, EndFrame int
	Encode   encode.Options
	AudioMode string
	AudioBitrate int
}

// RunInterpolate 执行插帧/慢放主流程。
func RunInterpolate(job InterpJob) error {
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
		if e > info.Frames-1 {
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
		FW: fw, FH: fh, Params: map[string]float64{}, Overlap: job.Overlap,
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

	overlap := job.Overlap
	if overlap <= 0 {
		overlap = 16
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

	if err := writeOrig(frameAt(0)); err != nil {
		enc.Close()
		return err
	}
	for j := 0; j < total-1; j++ {
		window := []*Frame{frameAt(j - 1), frameAt(j), frameAt(j + 1), frameAt(j + 2)}
		for k := 1; k < n; k++ {
			t := float64(k) / float64(n)
			subIdx := int(math.Round(t*float64(subdiv))) - 1
			if subIdx < 0 {
				subIdx = 0
			}
			if subIdx > subdiv-1 {
				subIdx = subdiv - 1
			}
			interp, err := processInterpFrame(r, window, fw, fh, th, tw, subIdx, overlap)
			if err != nil {
				enc.Close()
				return err
			}
			if err := writeOrig(interp); err != nil {
				enc.Close()
				return err
			}
		}
		if err := writeOrig(frameAt(j + 1)); err != nil {
			enc.Close()
			return err
		}
		// 释放远离的缓存。
		for idx := range cache {
			if idx < j-1 {
				delete(cache, idx)
			}
		}
		if frameIdx%20 == 0 {
			log.Progress("插帧", frameIdx, outCount)
		}
	}
	log.ProgressDone()
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
	for y := 0; y < fh; y += stepY {
		for x := 0; x < fw; x += stepX {
			tiles := make([]*Frame, 0, 4)
			for _, f := range window {
				tiles = append(tiles, f.ExtractTile(x, y, tw, th))
			}
			raw, err := r.ProcessTile(tiles, 1, x, y)
			if err != nil {
				return nil, err
			}
			sub := ExtractSubFrame(raw, subIdx)
			ramp := overlap
			if ramp > sub.W {
				ramp = sub.W / 4
			}
			if ramp > sub.H {
				ramp = sub.H / 4
			}
			tileW := NewFrame(sub.W, sub.H)
			FillRampWeight(tileW, ramp)
			MergeTileWeighted(canvas, weight, sub, tileW, x, y)
		}
	}
	NormalizeCanvas(canvas, weight)
	return canvas, nil
}
