package builtins_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/veypi/vsh/network"
)

// streamingStubClient 是实现了流式投递的 network.Client（req.Sink 非 nil 就边写
// 边返回，否则退回内存路径）——覆盖 curl -o 的两条收口。
type streamingStubClient struct {
	payload   string
	headers   map[string]string
	sawSink   bool
	lastReq   *network.Request
	streamErr error
}

func (c *streamingStubClient) Do(_ context.Context, req *network.Request) (*network.Response, error) {
	c.lastReq = req
	headers := c.headers
	if headers == nil {
		headers = map[string]string{"Content-Type": "application/octet-stream"}
	}
	resp := &network.Response{StatusCode: 200, Status: "200 OK", Headers: headers, URL: req.URL}
	if req.Sink != nil {
		c.sawSink = true
		if req.OnResponse != nil {
			req.OnResponse(200, "200 OK", headers)
		}
		n, err := io.WriteString(req.Sink, c.payload)
		if err != nil {
			return nil, err
		}
		if c.streamErr != nil {
			return nil, c.streamErr
		}
		resp.Streamed = true
		resp.BytesWritten = int64(n)
		return resp, nil
	}
	resp.Body = []byte(c.payload)
	return resp, nil
}

// TestCurlOutputStreamsBody：-o 走流式投递（客户端收到 Sink），文件拿到完整正文，
// stdout 不重复输出正文。
func TestCurlOutputStreamsBody(t *testing.T) {
	t.Parallel()

	client := &streamingStubClient{payload: strings.Repeat("blob", 40000)}
	session := newSession(t, &Config{NetworkClient: client})

	result := mustExecSession(t, session, "curl -s -o /tmp/out.bin https://api.example.com/blob.bin\n")
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if !client.sawSink {
		t.Fatal("curl -o did not use streaming delivery (req.Sink was nil)")
	}
	if got := string(readSessionFile(t, session, "/tmp/out.bin")); got != client.payload {
		t.Fatalf("out.bin = %d bytes, want %d payload bytes", len(got), len(client.payload))
	}
	if result.Stdout != "" {
		t.Fatalf("Stdout = %q, want empty for -o", result.Stdout)
	}
}

// TestCurlOutputWritesHeadersFirstWhenIncluding：-i 与 -o 同用时头块先于正文落盘
// （与真实 curl 一致），且仍走流式。
func TestCurlOutputWritesHeadersFirstWhenIncluding(t *testing.T) {
	t.Parallel()

	client := &streamingStubClient{payload: "body-bytes"}
	session := newSession(t, &Config{NetworkClient: client})

	result := mustExecSession(t, session, "curl -s -i -o /tmp/out.txt https://api.example.com/data\n")
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if !client.sawSink {
		t.Fatal("curl -i -o did not use streaming delivery (req.Sink was nil)")
	}
	want := "HTTP/1.1 200 OK\r\nContent-Type: application/octet-stream\r\n\r\nbody-bytes"
	if got := string(readSessionFile(t, session, "/tmp/out.txt")); got != want {
		t.Fatalf("out.txt = %q, want %q", got, want)
	}
}

// TestCurlOutputFallsBackToBufferedClient：客户端未实现流式（返回 Body、不写 Sink）
// 时，-o 仍把正文落盘。
func TestCurlOutputFallsBackToBufferedClient(t *testing.T) {
	t.Parallel()

	client := &curlStubClient{do: func(_ context.Context, req *network.Request) (*network.Response, error) {
		return &network.Response{StatusCode: 200, Status: "200 OK", Body: []byte("buffered"), URL: req.URL}, nil
	}}
	session := newSession(t, &Config{NetworkClient: client})

	result := mustExecSession(t, session, "curl -s -o /tmp/out.bin https://api.example.com/blob.bin\n")
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := string(readSessionFile(t, session, "/tmp/out.bin")), "buffered"; got != want {
		t.Fatalf("out.bin = %q, want %q", got, want)
	}
}

// TestCurlWriteOutReportsStreamedSize：-w %{size_download} 对流式投递报告已写字节数
// （此时 Body 为空，不能再按 Body 长度算）。
func TestCurlWriteOutReportsStreamedSize(t *testing.T) {
	t.Parallel()

	client := &streamingStubClient{payload: strings.Repeat("z", 1234)}
	session := newSession(t, &Config{NetworkClient: client})

	result := mustExecSession(t, session, "curl -s -o /tmp/out.bin -w '%{size_download}' https://api.example.com/blob.bin\n")
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, strconv.Itoa(len(client.payload)); got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}

// notifyWriter 每次写入时发一次信号（不阻塞），用于断言字节在响应结束前
// 就已到达 stdout。
type notifyWriter struct {
	mu  sync.Mutex
	buf bytes.Buffer
	ch  chan struct{}
}

func (w *notifyWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	n, err := w.buf.Write(p)
	w.mu.Unlock()
	select {
	case w.ch <- struct{}{}:
	default:
	}
	return n, err
}

func (w *notifyWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

// TestCurlStreamsStdoutBeforeResponseEnds：无 -o 时正文边收边出——响应体还没
// 结束，stdout 就已经拿到首块（SSE 场景的回归点）。
func TestCurlStreamsStdoutBeforeResponseEnds(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseAll := func() { releaseOnce.Do(func() { close(release) }) }
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "data: first\n\n")
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		<-release
		_, _ = io.WriteString(w, "data: second\n\n")
	}))
	defer server.Close()
	defer releaseAll()

	session := newSession(t, &Config{Network: &NetworkConfig{AllowedURLPrefixes: []string{server.URL}}})
	stdout := &notifyWriter{ch: make(chan struct{}, 16)}
	done := make(chan *ExecutionResult, 1)
	go func() {
		result, err := session.Exec(context.Background(), &ExecutionRequest{
			Script: "curl -sN " + server.URL + "/events\n",
			Stdout: stdout,
		})
		if err != nil {
			done <- nil
			return
		}
		done <- result
	}()

	select {
	case <-stdout.ch:
	case <-time.After(5 * time.Second):
		t.Fatal("no stdout bytes before the response finished (body still buffered)")
	}
	if got := stdout.String(); !strings.Contains(got, "data: first") {
		t.Fatalf("stdout = %q, want first event before the stream ends", got)
	}

	releaseAll()
	result := <-done
	if result == nil {
		t.Fatal("Session.Exec() returned an error")
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got := stdout.String(); !strings.Contains(got, "data: second") {
		t.Fatalf("stdout = %q, want the closing event", got)
	}
}

// TestCurlOutputStreamsLargeFileEndToEnd：真 HTTP 服务器 + 内建客户端——70 MiB
// 下载走流式落盘（超过旧 64 MiB 上限与内存预算都不该有限制），落盘尺寸正确。
func TestCurlOutputStreamsLargeFileEndToEnd(t *testing.T) {
	t.Parallel()

	const payloadSize = 70 << 20
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		block := bytes.Repeat([]byte("L"), 64<<10)
		for sent := 0; sent < payloadSize; sent += len(block) {
			if _, err := w.Write(block); err != nil {
				return
			}
		}
	}))
	defer server.Close()

	session := newSession(t, &Config{Network: &NetworkConfig{AllowedURLPrefixes: []string{server.URL}}})
	result := mustExecSession(t, session, "curl -s -o /tmp/big.bin "+server.URL+"/big.bin\n")
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	info, err := session.FileSystem().Stat(context.Background(), "/tmp/big.bin")
	if err != nil {
		t.Fatalf("Stat(/tmp/big.bin) error = %v", err)
	}
	if got := info.Size(); got != payloadSize {
		t.Fatalf("big.bin = %d bytes, want %d", got, payloadSize)
	}
}

// TestCurlOutputRemovesPartialFileOnFailure：流式下载中途失败不留半截文件。
func TestCurlOutputRemovesPartialFileOnFailure(t *testing.T) {
	t.Parallel()

	client := &streamingStubClient{
		payload:   strings.Repeat("partial", 100),
		streamErr: context.DeadlineExceeded,
	}
	session := newSession(t, &Config{NetworkClient: client})

	result := mustExecSession(t, session, "curl -s -o /tmp/out.bin https://api.example.com/blob.bin\n")
	if result.ExitCode == 0 {
		t.Fatalf("ExitCode = 0, want failure; stdout=%q", result.Stdout)
	}
	if _, err := session.FileSystem().Open(context.Background(), "/tmp/out.bin"); err == nil {
		t.Fatal("partial download file was kept, want it removed")
	}
}
