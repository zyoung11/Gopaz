package pipeline

import (
	"io"
	"sync"

	"gopaz/internal/video"
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

type outFrame struct {
	f   *Frame
	inv []float32
}

type frameWriter struct {
	ch  chan outFrame
	enc *video.Encoder
	wg  sync.WaitGroup
	err error
}

func startFrameWriter(enc *video.Encoder) *frameWriter {
	w := &frameWriter{ch: make(chan outFrame, 4), enc: enc}
	w.wg.Add(1)
	go w.run()
	return w
}

func (w *frameWriter) run() {
	defer w.wg.Done()
	for of := range w.ch {
		if w.err != nil {
			continue
		}
		var buf []byte
		if of.inv != nil {
			buf = of.f.ToRGB24Norm(of.inv)
		} else {
			buf = of.f.ToRGB24()
		}
		if err := w.enc.WriteFrame(buf); err != nil {
			w.err = err
		}
	}
}

func (w *frameWriter) Write(f *Frame) {
	w.ch <- outFrame{f: f}
}

func (w *frameWriter) WriteNorm(f *Frame, inv []float32) {
	w.ch <- outFrame{f: f, inv: inv}
}

func (w *frameWriter) Close() error {
	close(w.ch)
	w.wg.Wait()
	return w.err
}
