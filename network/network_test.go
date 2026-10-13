package network

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

type resolverFunc func(context.Context, string) ([]net.IPAddr, error)

func (fn resolverFunc) LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	return fn(ctx, host)
}

type sequenceResolver struct {
	mu        sync.Mutex
	responses [][]net.IPAddr
}

func (r *sequenceResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.responses) == 0 {
		return nil, &net.DNSError{Err: "no such host", IsNotFound: true}
	}
	response := r.responses[0]
	r.responses = r.responses[1:]
	return response, nil
}

type staticHTTPDoer struct{}

func (staticHTTPDoer) Do(*http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader("ok")),
	}, nil
}

func TestNewRejectsInvalidAllowList(t *testing.T) {
	t.Parallel()
	_, err := New(&Config{
		AllowedURLPrefixes: []string{"example.com"},
	})
	if err == nil {
		t.Fatal("New() error = nil, want invalid config error")
	}
}

func TestClientAllowsMatchingOriginAndPathPrefix(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	client, err := New(&Config{
		AllowedURLPrefixes: []string{server.URL + "/v1/"},
		DenyPrivateRanges:  false,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	resp, err := client.Do(context.Background(), &Request{
		URL: server.URL + "/v1/users",
	})
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if got, want := string(resp.Body), "ok"; got != want {
		t.Fatalf("Body = %q, want %q", got, want)
	}
}

func TestClientBlocksPathOutsideAllowListPrefix(t *testing.T) {
	t.Parallel()
	client, err := New(&Config{
		AllowedURLPrefixes: []string{"https://api.example.com/v1/"},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = client.Do(context.Background(), &Request{
		URL: "https://api.example.com/v2/users",
	})
	if !IsDenied(err) {
		t.Fatalf("Do() error = %v, want denied error", err)
	}
}

func TestClientTreatsAllowListPathsAsSegmentBoundaries(t *testing.T) {
	t.Parallel()
	client, err := New(&Config{
		AllowedURLPrefixes: []string{"https://api.example.com/private"},
	}, WithDoer(staticHTTPDoer{}))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	for _, target := range []string{
		"https://api.example.com/private",
		"https://api.example.com/private/token",
	} {
		_, err := client.Do(context.Background(), &Request{URL: target})
		if err != nil {
			t.Fatalf("Do(%q) error = %v, want allowed request", target, err)
		}
	}

	_, err = client.Do(context.Background(), &Request{
		URL: "https://api.example.com/private-token",
	})
	if !IsDenied(err) {
		t.Fatalf("Do() error = %v, want denied sibling-path request", err)
	}
}

func TestClientBlocksDisallowedMethod(t *testing.T) {
	t.Parallel()
	client, err := New(&Config{
		AllowedURLPrefixes: []string{"https://api.example.com"},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = client.Do(context.Background(), &Request{
		Method: "POST",
		URL:    "https://api.example.com/items",
	})
	var methodErr *MethodNotAllowedError
	if !errors.As(err, &methodErr) {
		t.Fatalf("Do() error = %v, want method denied error", err)
	}
}

func TestClientRevalidatesRedirectTargets(t *testing.T) {
	t.Parallel()
	deniedURL := "https://other.example.com/blocked"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, deniedURL, http.StatusFound)
	}))
	defer server.Close()

	client, err := New(&Config{
		AllowedURLPrefixes: []string{server.URL},
		DenyPrivateRanges:  false,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = client.Do(context.Background(), &Request{
		URL:             server.URL,
		FollowRedirects: true,
	})
	var redirectErr *RedirectNotAllowedError
	if !errors.As(err, &redirectErr) {
		t.Fatalf("Do() error = %v, want redirect denied error", err)
	}
}

func TestClientRevalidatesRedirectTargetsAcrossPathBoundary(t *testing.T) {
	t.Parallel()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, server.URL+"/private-token", http.StatusFound)
	}))
	defer server.Close()

	client, err := New(&Config{
		AllowedURLPrefixes: []string{server.URL + "/private"},
		DenyPrivateRanges:  false,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = client.Do(context.Background(), &Request{
		URL:             server.URL + "/private",
		FollowRedirects: true,
	})
	var redirectErr *RedirectNotAllowedError
	if !errors.As(err, &redirectErr) {
		t.Fatalf("Do() error = %v, want redirect denied error", err)
	}
}

func TestClientEnforcesResponseSizeLimit(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("a", 32)))
	}))
	defer server.Close()

	client, err := New(&Config{
		AllowedURLPrefixes: []string{server.URL},
		MaxResponseBytes:   8,
		DenyPrivateRanges:  false,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = client.Do(context.Background(), &Request{URL: server.URL})
	var sizeErr *ResponseTooLargeError
	if !errors.As(err, &sizeErr) {
		t.Fatalf("Do() error = %v, want response-too-large error", err)
	}
}

func TestClientBlocksPrivateRangesLexically(t *testing.T) {
	t.Parallel()
	client, err := New(&Config{
		AllowedURLPrefixes: []string{"http://127.0.0.1"},
		DenyPrivateRanges:  true,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = client.Do(context.Background(), &Request{URL: "http://127.0.0.1/"})
	if !IsDenied(err) {
		t.Fatalf("Do() error = %v, want denied error", err)
	}
}

func TestClientBlocksPrivateRangesAfterDNSResolution(t *testing.T) {
	t.Parallel()
	client, err := New(&Config{
		AllowedURLPrefixes: []string{"https://api.example.com"},
		DenyPrivateRanges:  true,
	}, WithResolver(resolverFunc(func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
	})))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = client.Do(context.Background(), &Request{URL: "https://api.example.com/path"})
	if !IsDenied(err) {
		t.Fatalf("Do() error = %v, want denied error", err)
	}
}

func TestClientBlocksPrivateRangesDuringDial(t *testing.T) {
	t.Parallel()
	client, err := New(&Config{
		AllowedURLPrefixes: []string{"http://api.example.test:80"},
		DenyPrivateRanges:  true,
	}, WithResolver(&sequenceResolver{
		responses: [][]net.IPAddr{
			{{IP: net.ParseIP("93.184.216.34")}},
			{{IP: net.ParseIP("127.0.0.1")}},
		},
	}))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = client.Do(context.Background(), &Request{URL: "http://api.example.test:80/path"})
	if !IsDenied(err) {
		t.Fatalf("Do() error = %v, want denied error", err)
	}
}

func TestClientIgnoresHostProxyEnvironment(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("backend"))
	}))
	defer backend.Close()

	var mu sync.Mutex
	proxyHit := false
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		proxyHit = true
		mu.Unlock()
		http.Error(w, "proxy", http.StatusBadGateway)
	}))
	defer proxy.Close()

	t.Setenv("HTTP_PROXY", proxy.URL)
	t.Setenv("http_proxy", proxy.URL)
	t.Setenv("HTTPS_PROXY", proxy.URL)
	t.Setenv("NO_PROXY", "")

	backendURL, err := url.Parse(backend.URL)
	if err != nil {
		t.Fatalf("Parse(%q) error = %v", backend.URL, err)
	}
	_, port, err := net.SplitHostPort(backendURL.Host)
	if err != nil {
		t.Fatalf("SplitHostPort(%q) error = %v", backendURL.Host, err)
	}
	allowedHost := net.JoinHostPort("allowed.example.test", port)

	client, err := New(&Config{
		AllowedURLPrefixes: []string{"http://" + allowedHost},
		DenyPrivateRanges:  false,
	}, WithResolver(resolverFunc(func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
	})))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	resp, err := client.Do(context.Background(), &Request{URL: "http://" + allowedHost + "/via-default-transport"})
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if got, want := string(resp.Body), "backend"; got != want {
		t.Fatalf("Body = %q, want %q", got, want)
	}

	mu.Lock()
	hit := proxyHit
	mu.Unlock()
	if hit {
		t.Fatal("proxy server was used, want direct sandbox-controlled dial")
	}
}

// TestClientHonorsRequestProxyWhenEnabled：UseProxy 开启时 Request.ProxyURL
// 生效——请求拨向代理而非目标（代理返回 502 标记体，区别于后端 200）。
func TestClientHonorsRequestProxyWhenEnabled(t *testing.T) {
	var mu sync.Mutex
	proxyHit := false
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		proxyHit = true
		mu.Unlock()
		http.Error(w, "via-proxy", http.StatusBadGateway)
	}))
	defer proxy.Close()

	client, err := New(&Config{
		AllowedURLPrefixes: []string{"http://allowed.example.test"},
		UseProxy:           true,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	resp, err := client.Do(context.Background(), &Request{
		URL:      "http://allowed.example.test/path",
		ProxyURL: proxy.URL,
	})
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if got, want := resp.StatusCode, http.StatusBadGateway; got != want {
		t.Fatalf("StatusCode = %d, want %d（应拿到代理的响应）", got, want)
	}
	mu.Lock()
	hit := proxyHit
	mu.Unlock()
	if !hit {
		t.Fatal("proxy server was not used, want Request.ProxyURL honored when UseProxy is on")
	}
}

// TestClientIgnoresRequestProxyWhenDisabled：默认（UseProxy=false）忽略
// Request.ProxyURL，直连目标——代理不被触碰。
func TestClientIgnoresRequestProxyWhenDisabled(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("backend"))
	}))
	defer backend.Close()

	var mu sync.Mutex
	proxyHit := false
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		proxyHit = true
		mu.Unlock()
		http.Error(w, "proxy", http.StatusBadGateway)
	}))
	defer proxy.Close()

	client, err := New(&Config{
		AllowedURLPrefixes: []string{backend.URL},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	resp, err := client.Do(context.Background(), &Request{
		URL:      backend.URL + "/direct",
		ProxyURL: proxy.URL,
	})
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if got, want := string(resp.Body), "backend"; got != want {
		t.Fatalf("Body = %q, want %q", got, want)
	}
	mu.Lock()
	hit := proxyHit
	mu.Unlock()
	if hit {
		t.Fatal("proxy server was used with UseProxy off, want Request.ProxyURL ignored")
	}
}

func TestParseProxyURL(t *testing.T) {
	t.Parallel()

	cases := []struct {
		raw     string
		want    string
		wantErr bool
	}{
		{"http://127.0.0.1:7897", "http://127.0.0.1:7897", false},
		{"127.0.0.1:7897", "http://127.0.0.1:7897", false},
		{"socks5://127.0.0.1:7897", "socks5://127.0.0.1:7897", false},
		{"socks5h://127.0.0.1:7897", "socks5h://127.0.0.1:7897", false},
		{"https://user:pass@proxy.example:8443", "https://user:pass@proxy.example:8443", false},
		{"", "", true},
		{"ftp://proxy.example", "", true},
		{"http://", "", true},
	}
	for _, tc := range cases {
		got, err := ParseProxyURL(tc.raw)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("ParseProxyURL(%q) error = nil, want error", tc.raw)
			}
			continue
		}
		if err != nil {
			t.Fatalf("ParseProxyURL(%q) error = %v", tc.raw, err)
		}
		if got.String() != tc.want {
			t.Fatalf("ParseProxyURL(%q) = %q, want %q", tc.raw, got.String(), tc.want)
		}
	}
}
