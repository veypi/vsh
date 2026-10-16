package network

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestStreamResponseBodyToSink：Sink 非 nil 时响应体边收边写，Response.Body 留空；
// OnResponse 先于正文回调（curl -i 的头块依此落盘）。
func TestStreamResponseBodyToSink(t *testing.T) {
	t.Parallel()

	payload := strings.Repeat("stream-payload-", 8192) // ~120 KiB
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = io.WriteString(w, payload)
	}))
	defer server.Close()

	client, err := New(&Config{AllowedURLPrefixes: []string{server.URL}})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	var sink bytes.Buffer
	var (
		gotCode    int
		gotHeaders map[string]string
	)
	resp, err := client.Do(context.Background(), &Request{
		Method: "GET",
		URL:    server.URL + "/blob.bin",
		Sink:   &sink,
		OnResponse: func(statusCode int, status string, headers map[string]string) {
			gotCode, gotHeaders = statusCode, headers
			sink.WriteString("PREFIX:")
		},
	})
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if !resp.Streamed {
		t.Fatal("Streamed = false, want true")
	}
	if got, want := resp.BytesWritten, int64(len(payload)); got != want {
		t.Fatalf("BytesWritten = %d, want %d", got, want)
	}
	if len(resp.Body) != 0 {
		t.Fatalf("Body = %d bytes, want empty for a streamed response", len(resp.Body))
	}
	if got, want := sink.String(), "PREFIX:"+payload; got != want {
		t.Fatalf("sink = %d bytes, want OnResponse prefix followed by %d payload bytes", len(got), len(want))
	}
	if gotCode != http.StatusOK || gotHeaders["Content-Type"] != "application/octet-stream" {
		t.Fatalf("OnResponse meta = %d %v", gotCode, gotHeaders)
	}
}

// TestStreamResponseBodyRespectsCap：流式与内存两条路同口径受 MaxResponseBytes
// 限制——超限即中止，且不把超额部分交给 Sink。
func TestStreamResponseBodyRespectsCap(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(bytes.Repeat([]byte("x"), 4096))
	}))
	defer server.Close()

	client, err := New(&Config{AllowedURLPrefixes: []string{server.URL}, MaxResponseBytes: 1024})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	var sink bytes.Buffer
	_, err = client.Do(context.Background(), &Request{Method: "GET", URL: server.URL + "/big", Sink: &sink})
	var tooLarge *ResponseTooLargeError
	if !errors.As(err, &tooLarge) {
		t.Fatalf("oversize streamed response = %v, want ResponseTooLargeError", err)
	}
	if sink.Len() > 1024 {
		t.Fatalf("sink = %d bytes, want at most the 1024-byte cap", sink.Len())
	}
}

// TestResponseBodyUnlimitedByDefault：未配置 MaxResponseBytes 时没有内建上限
// （原 10 MiB 默认已删除）——内存路径与流式路径都不设限。
func TestResponseBodyUnlimitedByDefault(t *testing.T) {
	t.Parallel()

	const payloadSize = 11 << 20 // 超过已删除的 10 MiB 默认上限
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		block := bytes.Repeat([]byte("y"), 64<<10)
		for sent := 0; sent < payloadSize; sent += len(block) {
			if _, err := w.Write(block); err != nil {
				return
			}
		}
	}))
	defer server.Close()

	client, err := New(&Config{AllowedURLPrefixes: []string{server.URL}})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	resp, err := client.Do(context.Background(), &Request{Method: "GET", URL: server.URL + "/big"})
	if err != nil {
		t.Fatalf("buffered Do() error = %v", err)
	}
	if got, want := len(resp.Body), payloadSize; got != want {
		t.Fatalf("Body = %d bytes, want %d (no implicit response cap)", got, want)
	}

	streamed, err := client.Do(context.Background(), &Request{Method: "GET", URL: server.URL + "/big", Sink: io.Discard})
	if err != nil {
		t.Fatalf("streamed Do() error = %v", err)
	}
	if !streamed.Streamed || streamed.BytesWritten != payloadSize {
		t.Fatalf("streamed = %v, BytesWritten = %d, want %d", streamed.Streamed, streamed.BytesWritten, payloadSize)
	}
}
