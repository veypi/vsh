package builtins_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/veypi/vsh/internal/vshversion"
	"github.com/veypi/vsh/network"
)

// retryStubResponse 描述第 N 次请求的行为：状态码、正文与（可选）中途错误。
type retryStubResponse struct {
	status  int
	payload string
	// beforeErrPayload 在返回错误前先写出的正文字节（模拟流中断）。
	beforeErrPayload string
	err              error
}

// retryStubClient 顺序返回预置响应，并记录调用次数——--retry 是重试次数契约的
// 唯一可观测点。
type retryStubClient struct {
	mu        sync.Mutex
	calls     int
	responses []retryStubResponse
}

func (c *retryStubClient) Do(_ context.Context, req *network.Request) (*network.Response, error) {
	c.mu.Lock()
	index := c.calls
	c.calls++
	c.mu.Unlock()
	if index >= len(c.responses) {
		index = len(c.responses) - 1
	}
	item := c.responses[index]
	status := item.status
	if status == 0 {
		status = http.StatusOK
	}
	headers := map[string]string{"Content-Type": "text/plain"}
	resp := &network.Response{
		StatusCode: status,
		Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Headers:    headers,
		URL:        req.URL,
	}
	if req.Sink != nil {
		if req.OnResponse != nil {
			req.OnResponse(status, resp.Status, headers)
		}
		if item.beforeErrPayload != "" {
			_, _ = io.WriteString(req.Sink, item.beforeErrPayload)
		}
		if item.err != nil {
			return nil, item.err
		}
		n, werr := io.WriteString(req.Sink, item.payload)
		if werr != nil {
			return nil, werr
		}
		resp.Streamed = true
		resp.BytesWritten = int64(n)
		return resp, nil
	}
	if item.err != nil {
		return nil, item.err
	}
	resp.Body = []byte(item.payload)
	return resp, nil
}

func (c *retryStubClient) CallCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

// TestCurlStreamsStdoutBody：不带 -o 时同样走流式投递（req.Sink 非 nil），
// 正文完整写到 stdout——SSE/长响应不再攒到连接结束才吐字节。
func TestCurlStreamsStdoutBody(t *testing.T) {
	t.Parallel()

	payload := strings.Repeat("data: event\n\n", 500)
	client := &streamingStubClient{payload: payload}
	session := newSession(t, &Config{NetworkClient: client})

	result := mustExecSession(t, session, "curl -sN https://api.example.com/events\n")
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if !client.sawSink {
		t.Fatal("curl without -o did not stream (req.Sink was nil)")
	}
	if result.Stdout != payload {
		t.Fatalf("Stdout = %d bytes, want %d payload bytes", len(result.Stdout), len(payload))
	}
}

// TestCurlRetryOnRetryableStatus：503 后按 --retry 重试，丢弃被重试那次的正文。
func TestCurlRetryOnRetryableStatus(t *testing.T) {
	t.Parallel()

	client := &retryStubClient{responses: []retryStubResponse{
		{status: http.StatusServiceUnavailable, payload: "busy"},
		{status: http.StatusOK, payload: "ok"},
	}}
	session := newSession(t, &Config{NetworkClient: client})

	result := mustExecSession(t, session, "curl -s --retry 2 --retry-delay 1 https://api.example.com/data\n")
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := client.CallCount(), 2; got != want {
		t.Fatalf("CallCount = %d, want %d", got, want)
	}
	if got, want := result.Stdout, "ok"; got != want {
		t.Fatalf("Stdout = %q, want %q (retried attempt body must be dropped)", got, want)
	}
}

// TestCurlRetryStopsAfterBodyBytes：已经写出正文字节后出错就不再重试（无法撤回）。
func TestCurlRetryStopsAfterBodyBytes(t *testing.T) {
	t.Parallel()

	client := &retryStubClient{responses: []retryStubResponse{{
		status:           http.StatusOK,
		payload:          "ignored",
		beforeErrPayload: "partial",
		err:              context.DeadlineExceeded,
	}}}
	session := newSession(t, &Config{NetworkClient: client})

	result := mustExecSession(t, session, "curl -s --retry 3 --retry-delay 1 https://api.example.com/data\n")
	if result.ExitCode == 0 {
		t.Fatalf("ExitCode = 0, want failure; stdout=%q", result.Stdout)
	}
	if got, want := client.CallCount(), 1; got != want {
		t.Fatalf("CallCount = %d, want %d (no retry after partial body)", got, want)
	}
}

// TestCurlVersionFlag：-V/--version 输出 vsh 版本与协议行，且不需要 URL。
func TestCurlVersionFlag(t *testing.T) {
	t.Parallel()

	session := newSession(t, &Config{NetworkClient: &curlStubClient{}})
	result := mustExecSession(t, session, "curl -V\n")
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	wantPrefix := "curl " + vshversion.Version + " (vsh builtin)\n"
	if !strings.HasPrefix(result.Stdout, wantPrefix) {
		t.Fatalf("Stdout = %q, want prefix %q", result.Stdout, wantPrefix)
	}
	for _, want := range []string{"Protocols: http https\n", "Features: HTTP2\n"} {
		if !strings.Contains(result.Stdout, want) {
			t.Fatalf("Stdout = %q, want %q", result.Stdout, want)
		}
	}
}

// TestCurlCompatibilityFlagsAccepted：日常脚本里的兼容旗标不再报 unknown option。
func TestCurlCompatibilityFlagsAccepted(t *testing.T) {
	t.Parallel()

	client := &streamingStubClient{payload: "ok"}
	session := newSession(t, &Config{NetworkClient: client})

	result := mustExecSession(t, session,
		"curl -sN -g --compressed --http1.1 --http2 --http3 -# --no-buffer https://api.example.com/data\n",
	)
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, "ok"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}

// TestCurlOutputToDevNull：`-o /dev/null -w '%{http_code}'` 这类脚本写法可用
// （/dev/null 由 fs 层的虚拟设备吞掉正文）。
func TestCurlOutputToDevNull(t *testing.T) {
	t.Parallel()

	client := &streamingStubClient{payload: "ignored"}
	session := newSession(t, &Config{NetworkClient: client})

	result := mustExecSession(t, session, "curl -s -o /dev/null -w '%{http_code}' https://api.example.com/data\n")
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, "200"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}

// TestCurlDumpHeaderWritesFile：-D 把响应头写进文件（流式路径由 OnResponse 供给）。
func TestCurlDumpHeaderWritesFile(t *testing.T) {
	t.Parallel()

	client := &streamingStubClient{payload: "body", headers: map[string]string{"Content-Type": "text/plain"}}
	session := newSession(t, &Config{NetworkClient: client})

	result := mustExecSession(t, session, "curl -s -D /tmp/headers.txt https://api.example.com/data\n")
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	want := "HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\n\r\n"
	if got := string(readSessionFile(t, session, "/tmp/headers.txt")); got != want {
		t.Fatalf("headers.txt = %q, want %q", got, want)
	}
	if got, want := result.Stdout, "body"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}

// TestCurlInsecureSetsRequestFlag：-k 把跳过校验的意图传给网络客户端。
func TestCurlInsecureSetsRequestFlag(t *testing.T) {
	t.Parallel()

	client := &curlStubClient{}
	session := newSession(t, &Config{NetworkClient: client})

	result := mustExecSession(t, session, "curl -sk https://api.example.com/data\n")
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	req := client.LastRequest()
	if req == nil {
		t.Fatal("LastRequest = nil, want captured request")
	}
	if !req.Insecure {
		t.Fatal("Insecure = false, want true for -k")
	}
}
