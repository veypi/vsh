package builtins

import (
	"bytes"
	"testing"
)

// flushRecorder 记录写入与 flush 次数——curlSink 的逐块 flush 契约靠它观测。
type flushRecorder struct {
	writes  int
	flushes int
	buf     bytes.Buffer
}

func (w *flushRecorder) Write(p []byte) (int, error) {
	w.writes++
	return w.buf.Write(p)
}

func (w *flushRecorder) Flush() error {
	w.flushes++
	return nil
}

// TestCurlSinkFlushesEveryChunk：每个正文块写后立即 flush（-N 的默认语义）。
func TestCurlSinkFlushesEveryChunk(t *testing.T) {
	t.Parallel()

	recorder := &flushRecorder{}
	sink := newCurlSink(recorder)
	if _, err := sink.Write([]byte("data: one\n\n")); err != nil {
		t.Fatalf("Write error = %v", err)
	}
	if _, err := sink.Write([]byte("data: two\n\n")); err != nil {
		t.Fatalf("Write error = %v", err)
	}
	if got, want := recorder.flushes, 2; got != want {
		t.Fatalf("flushes = %d, want %d", got, want)
	}
	if got, want := sink.body, int64(len("data: one\n\ndata: two\n\n")); got != want {
		t.Fatalf("body = %d, want %d", got, want)
	}
}

// TestCurlSinkDropDiscardsBodyButKeepsCountZero：丢弃模式不写目标、不涨 body
// （重试与 -f 的判定依赖 body == 0）。
func TestCurlSinkDropDiscardsBodyButKeepsCountZero(t *testing.T) {
	t.Parallel()

	recorder := &flushRecorder{}
	sink := newCurlSink(recorder)
	sink.drop = true
	n, err := sink.Write([]byte("discarded"))
	if err != nil {
		t.Fatalf("Write error = %v", err)
	}
	if n != len("discarded") {
		t.Fatalf("Write returned %d, want %d (must not shorten the body)", n, len("discarded"))
	}
	if sink.body != 0 {
		t.Fatalf("body = %d, want 0 in drop mode", sink.body)
	}
	if recorder.buf.Len() != 0 {
		t.Fatalf("writer got %q, want nothing in drop mode", recorder.buf.String())
	}
}

// TestCurlSinkHeadIsNotBody：-i/-I 的头块不计入 body（否则重试判定永远失效）。
func TestCurlSinkHeadIsNotBody(t *testing.T) {
	t.Parallel()

	recorder := &flushRecorder{}
	sink := newCurlSink(recorder)
	if err := sink.writeHead("HTTP/1.1 200 OK\r\n\r\n"); err != nil {
		t.Fatalf("writeHead error = %v", err)
	}
	if sink.body != 0 {
		t.Fatalf("body = %d, want 0 after header block", sink.body)
	}
	if recorder.flushes == 0 {
		t.Fatal("header block was not flushed")
	}
}
