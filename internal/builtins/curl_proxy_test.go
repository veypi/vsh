package builtins_test

import (
	"testing"
)

// curl 的代理解析契约：-x/--proxy 优先于会话环境变量；-x "" 显式禁用；
// 环境变量按目标 scheme 取 https_proxy/http_proxy（小写优先、大写兼容），
// 兜底 all_proxy；no_proxy 命中直连。curl 只负责把结果填进
// Request.ProxyURL——是否真正使用由网络客户端的 UseProxy 配置决定（见
// network 包测试）。

func curlProxyOf(t *testing.T, script string) string {
	t.Helper()
	client := &curlStubClient{}
	session := newSession(t, &Config{NetworkClient: client})
	result := mustExecSession(t, session, script)
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	req := client.LastRequest()
	if req == nil {
		t.Fatal("LastRequest = nil, want captured request")
	}
	return req.ProxyURL
}

func TestCurlProxyFlagSetsRequestProxy(t *testing.T) {
	t.Parallel()
	if got, want := curlProxyOf(t, "curl -s -x http://127.0.0.1:7897 https://example.com\n"), "http://127.0.0.1:7897"; got != want {
		t.Fatalf("ProxyURL = %q, want %q", got, want)
	}
}

func TestCurlProxyFlagNormalizesMissingScheme(t *testing.T) {
	t.Parallel()
	if got, want := curlProxyOf(t, "curl -s -x 127.0.0.1:7897 https://example.com\n"), "http://127.0.0.1:7897"; got != want {
		t.Fatalf("ProxyURL = %q, want %q", got, want)
	}
	if got, want := curlProxyOf(t, "curl -s --proxy=socks5://127.0.0.1:7897 https://example.com\n"), "socks5://127.0.0.1:7897"; got != want {
		t.Fatalf("ProxyURL = %q, want %q", got, want)
	}
}

func TestCurlProxyFromSessionEnv(t *testing.T) {
	t.Parallel()
	if got, want := curlProxyOf(t, "export HTTPS_PROXY=http://127.0.0.1:7897\ncurl -s https://example.com\n"), "http://127.0.0.1:7897"; got != want {
		t.Fatalf("ProxyURL = %q, want %q", got, want)
	}
	// 小写优先、大写兼容；http 目标读 http_proxy。
	if got, want := curlProxyOf(t, "export https_proxy=http://lower:1\ncurl -s https://example.com\n"), "http://lower:1"; got != want {
		t.Fatalf("ProxyURL = %q, want %q", got, want)
	}
	if got, want := curlProxyOf(t, "export http_proxy=http://web:8080\ncurl -s http://example.com\n"), "http://web:8080"; got != want {
		t.Fatalf("ProxyURL = %q, want %q", got, want)
	}
	// scheme 专属变量缺省时兜底 all_proxy。
	if got, want := curlProxyOf(t, "export ALL_PROXY=socks5://127.0.0.1:7897\ncurl -s https://example.com\n"), "socks5://127.0.0.1:7897"; got != want {
		t.Fatalf("ProxyURL = %q, want %q", got, want)
	}
	// 未设置任何代理变量 = 直连。
	if got := curlProxyOf(t, "curl -s https://example.com\n"); got != "" {
		t.Fatalf("ProxyURL = %q, want empty (direct)", got)
	}
}

func TestCurlProxyFlagOverridesEnv(t *testing.T) {
	t.Parallel()
	if got, want := curlProxyOf(t, "export HTTPS_PROXY=http://env:1\ncurl -s -x http://flag:2 https://example.com\n"), "http://flag:2"; got != want {
		t.Fatalf("ProxyURL = %q, want %q", got, want)
	}
	// -x "" 显式禁用（即使有环境变量）。
	if got := curlProxyOf(t, "export HTTPS_PROXY=http://env:1\ncurl -s -x '' https://example.com\n"); got != "" {
		t.Fatalf("ProxyURL = %q, want empty (-x '' disables)", got)
	}
}

func TestCurlNoProxySkipsEnvProxy(t *testing.T) {
	t.Parallel()
	// 后缀命中。
	if got := curlProxyOf(t, "export HTTPS_PROXY=http://env:1\nexport NO_PROXY=example.com\ncurl -s https://example.com\n"); got != "" {
		t.Fatalf("ProxyURL = %q, want empty (no_proxy suffix match)", got)
	}
	if got := curlProxyOf(t, "export HTTPS_PROXY=http://env:1\nexport no_proxy=.example.com\ncurl -s https://api.example.com\n"); got != "" {
		t.Fatalf("ProxyURL = %q, want empty (no_proxy leading-dot match)", got)
	}
	// "*" 全命中。
	if got := curlProxyOf(t, "export HTTPS_PROXY=http://env:1\nexport NO_PROXY=*\ncurl -s https://example.com\n"); got != "" {
		t.Fatalf("ProxyURL = %q, want empty (no_proxy *)", got)
	}
	// 端口限定不匹配则照常走代理。
	if got, want := curlProxyOf(t, "export HTTPS_PROXY=http://env:1\nexport NO_PROXY=example.com:8080\ncurl -s https://example.com\n"), "http://env:1"; got != want {
		t.Fatalf("ProxyURL = %q, want %q (port mismatch keeps proxy)", got, want)
	}
	// 非后缀不命中。
	if got, want := curlProxyOf(t, "export HTTPS_PROXY=http://env:1\nexport NO_PROXY=ample.com\ncurl -s https://example.com\n"), "http://env:1"; got != want {
		t.Fatalf("ProxyURL = %q, want %q (non-suffix keeps proxy)", got, want)
	}
}
