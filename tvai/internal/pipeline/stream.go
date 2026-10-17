package pipeline

import (
	"io"
	"sync"

	"tvai/internal/video"
)

type frameReader struct {
	ch   chan *Frame
	done chan struct{}
	once sync.Once
	wg   sync.WaitGroup
	mu   sync.Mutex
	err  error
}

func startFrameReader(dec *video.Decoder, w, h, total int) *frameReader {
	r := &frameReader{ch: make(chan *Frame, 2), done: make(chan struct{})}
	r.wg.Add(1)
	go r.run(dec, w, h, total)
	return r
}

func (r *frameReader) run(dec *video.Decoder, w, h, total int) {
	defer r.wg.Done()
	defer close(r.ch)
	buf := make([]byte, w*h*3)
	for read := 0; total <= 0 || read < total; read++ {
		if err := dec.ReadFrame(buf); err != nil {
			if err != io.EOF {
				select {
				case <-r.done:
				default:
					r.mu.Lock()
					r.err = err
					r.mu.Unlock()
				}
			}
			return
		}
		f := FromRGB24(buf, w, h)
		select {
		case r.ch <- f:
		case <-r.done:
			return
		}
	}
}

func (r *frameReader) Next() (*Frame, bool) {
	f, ok := <-r.ch
	return f, ok
}

func (r *frameReader) Stop() {
	r.once.Do(func() { close(r.done) })
}

func (r *frameReader) Err() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.err
}

type frameWriter struct {
	ch  chan *Frame
	enc *video.Encoder
	wg  sync.WaitGroup
	err error
}

func startFrameWriter(enc *video.Encoder) *frameWriter {
	w := &frameWriter{ch: make(chan *Frame, 2), enc: enc}
	w.wg.Add(1)
	go w.run()
	return w
}

func (w *frameWriter) run() {
	defer w.wg.Done()
	for f := range w.ch {
		if w.err != nil {
			continue
		}
		if err := w.enc.WriteFrame(f.ToRGB24()); err != nil {
			w.err = err
		}
	}
}

func (w *frameWriter) Write(f *Frame) {
	w.ch <- f
}

func (w *frameWriter) Close() error {
	close(w.ch)
	w.wg.Wait()
	return w.err
}
