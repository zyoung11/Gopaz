package pipeline

import (
	"fmt"
	"io"
	"os"

	"tvai/internal/encode"
	"tvai/internal/log"
	"tvai/internal/model"
	"tvai/internal/video"
)

// UpscaleJob 描述一次超分任务。
type UpscaleJob struct {
	Input, Output   string
	Ffmpeg, Ffprobe string
	ModelDir        string
	Model           *model.Model
	Scale           int
	Params          map[string]float64
	GPU             int
	Threads         int
	Overlap         int
	// KColor 为 true 时按模型 keepColor 做 Lab 色彩校正。
	KColor bool
	// SceneThreshold 为场景切换阈值，0=默认 6.5。
	SceneThreshold       float64
	WorkDir              string
	StartFrame, EndFrame int
	Encode               encode.Options
	AudioMode            string
	AudioBitrate         int
}

// RunUpscale 执行超分/修复主流程。
func RunUpscale(job UpscaleJob) error {
	info, err := video.Probe(job.Ffprobe, job.Input)
	if err != nil {
		return err
	}
	fw, fh := info.Width, info.Height
	if fw <= 0 || fh <= 0 {
		return fmt.Errorf("无法确定输入分辨率")
	}
	log.Infof("输入 %dx%d, %.3f fps, %d 帧", fw, fh, info.FPS, info.Frames)

	so := job.Model.Temporal
	if so <= 0 {
		so = 1
	}

	cfg := UpscaleConfig{
		Model: job.Model, ModelDir: job.ModelDir, Scale: job.Scale,
		FW: fw, FH: fh, Params: job.Params, Overlap: job.Overlap,
		SceneThreshold: job.SceneThreshold,
		GPU:            job.GPU, Threads: job.Threads, WorkDir: job.WorkDir,
	}
	r, err := LoadRunner(cfg)
	if err != nil {
		return err
	}
	defer r.Close()
	th, tw := r.TileSize()
	log.Infof("使用 tile %dx%d, 模型 %s", th, tw, job.Model.Display)

	outW, outH := fw*job.Scale, fh*job.Scale
	encArgs, err := encode.BuildArgs(job.Encode)
	if err != nil {
		return err
	}
	// 先编码到临时文件，再并入音频得到最终输出。
	tmpVideo := TempVideoPath(job.Output)
	dec, err := video.NewDecoder(job.Ffmpeg, job.Input, fw, fh, job.StartFrame, job.EndFrame)
	if err != nil {
		return err
	}
	defer dec.Close()

	enc, err := video.NewEncoder(job.Ffmpeg, tmpVideo, outW, outH, info.FPS, encArgs)
	if err != nil {
		return err
	}

	// 带前瞻的环形缓冲时序窗口。
	center := job.Model.CenterIdx
	if center >= so {
		center = so - 1
	}
	lookAhead := so - center - 1
	ring := make([]*Frame, so)
	var lastFrame *Frame
	readPos, emitPos := 0, 0
	total := max(info.Frames, 0)

	frameIdx := 0
	buf := make([]byte, fw*fh*3)
	overlap := r.Overlap()
	needTemporal := r.HasInput("hr_prev")

	readMore := func() bool {
		if total > 0 && readPos >= total {
			return false
		}
		if err := dec.ReadFrame(buf); err == io.EOF {
			return false
		} else if err != nil {
			return false
		}
		f := FromRGB24(buf, fw, fh)
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
		if needTemporal && r.prevHR == nil {
			r.ResetTemporal(centerFrame.UpscaleBilinear(outW, outH))
		}
		rawFrame, err := processFrame(r, window, fw, fh, th, tw, job.Scale, overlap)
		if err != nil {
			enc.Close()
			return err
		}
		outFrame := rawFrame
		if job.Model.KeepColor && job.KColor {
			outFrame = rawFrame.Clone()
			ColorTransferLab(centerFrame, outFrame)
		}
		if err := enc.WriteFrame(outFrame.ToRGB24()); err != nil {
			enc.Close()
			if se := enc.Stderr(); se != "" {
				return fmt.Errorf("写入编码器失败: %w; ffmpeg: %s", err, se)
			}
			return err
		}
		if needTemporal {
			r.SetPrevOutput(rawFrame)
		}
		frameIdx++
		emitPos++
		if frameIdx%10 == 0 {
			if total > 0 {
				log.Progress("超分", frameIdx, total)
			} else {
				log.Progress("超分", frameIdx, -1)
			}
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
	log.Successf("完成: %s (%d 帧, %dx%d)", job.Output, frameIdx, outW, outH)
	return nil
}

// processFrame 对一帧完整处理：分块推理 + 重叠融合。
func processFrame(r *Runner, window []*Frame, fw, fh, th, tw, scale, overlap int) (*Frame, error) {
	center := r.cfg.Model.CenterIdx
	if center >= len(window) {
		center = len(window) - 1
	}
	// 输出画布与权重。
	canvas := NewFrame(fw*scale, fh*scale)
	weight := NewFrame(fw*scale, fh*scale)

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
			// 从窗口每帧提取同一位置 tile。
			tiles := make([]*Frame, 0, len(window))
			for _, f := range window {
				tiles = append(tiles, f.ExtractTile(x, y, tw, th))
			}
			tileOut, err := r.ProcessTile(tiles, scale, x*scale, y*scale)
			if err != nil {
				return nil, err
			}
			ramp := overlap * scale
			if ramp > tileOut.W {
				ramp = tileOut.W / 4
			}
			if ramp > tileOut.H {
				ramp = tileOut.H / 4
			}
			tileW := NewFrame(tileOut.W, tileOut.H)
			FillRampWeight(tileW, ramp)
			MergeTileWeighted(canvas, weight, tileOut, tileW, x*scale, y*scale)
		}
	}
	NormalizeCanvas(canvas, weight)
	return canvas, nil
}
