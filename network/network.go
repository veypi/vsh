package network

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

type Method string

const (
	MethodGet     Method = "GET"
	MethodHead    Method = "HEAD"
	MethodPost    Method = "POST"
	MethodPut     Method = "PUT"
	MethodDelete  Method = "DELETE"
	MethodPatch   Method = "PATCH"
	MethodOptions Method = "OPTIONS"
)

const (
	defaultMaxRedirects = 20
	defaultTimeout      = 30 * time.Second
)

var defaultAllowedMethods = []Method{MethodGet, MethodHead}

type Config struct {
	AllowedURLPrefixes []string
	AllowedMethods     []Method
	MaxRedirects       int
	Timeout            time.Duration
	// MaxResponseBytes 限制单个响应体的字节数；<= 0（默认）不限制。上限由
	// 引入方按场景给（云端下载限 64 MiB、设备端不限）。
	MaxResponseBytes  int64
	DenyPrivateRanges bool
	// UseProxy 允许请求经代理（Request.ProxyURL，curl 由 -x/会话环境变量
	// 解析填入）。默认 false = 忽略一切代理设置、恒直连（沙箱受控拨号契约）；
	// 多租户云端必须保持 false——代理使 dial 目标变成代理服务器，绕过私网
	// 阻断。仅内置 transport 生效；WithDoer 注入的自定义实现需自行处理
	// Request.ProxyURL。
	UseProxy bool
}

type Request struct {
	Method          string
	URL             string
	Headers         map[string]string
	Body            []byte
	FollowRedirects bool
	Timeout         time.Duration
	// Sink（可选）请求流式投递响应体：客户端边收边写入 Sink，Response.Body
	// 留空、Response.Streamed 置真——大文件下载不再整块驻留内存。忽略 Sink
	// 的客户端照常返回 Body，调用方两条路都必须能收（见 curl -o）。
	Sink io.Writer
	// OnResponse（可选）在响应头到达、正文写出之前回调一次，让调用方先把自己
	// 的前缀写进 Sink（curl -i 的头块）；仅 Sink 非 nil 时有意义。
	OnResponse func(statusCode int, status string, headers map[string]string)
	// ProxyURL 本次请求的显式代理（curl 由 -x 或会话环境变量 HTTPS_PROXY/
	// HTTP_PROXY/ALL_PROXY 解析填入，已规范化为带 scheme 的 URL）。仅在客户
	// 端配置 UseProxy 时生效，否则被忽略。
	ProxyURL string
}

type Response struct {
	StatusCode int
	Status     string
	Headers    map[string]string
	Body       []byte
	URL        string
	// Streamed 表示响应体已流式写入 Request.Sink（此时 Body 为空）。
	Streamed bool
	// BytesWritten 是流式投递的字节数（Streamed 为 false 时恒为 0）。
	BytesWritten int64
}

type Client interface {
	Do(context.Context, *Request) (*Response, error)
}

type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type Resolver interface {
	LookupIPAddr(context.Context, string) ([]net.IPAddr, error)
}

type Option func(*HTTPClient)

type HTTPClient struct {
	cfg      resolvedConfig
	doer     HTTPDoer
	resolver Resolver
}

type resolvedConfig struct {
	allowedURLPrefixes []allowListEntry
	allowedMethods     map[Method]struct{}
	maxRedirects       int
	timeout            time.Duration
	maxResponseBytes   int64
	denyPrivateRanges  bool
	useProxy           bool
}

type allowListEntry struct {
	origin     string
	pathPrefix string
}

type deniedMarker interface {
	networkDenied()
}

type AccessDeniedError struct {
	URL    string
	Reason string
}

func (e *AccessDeniedError) Error() string {
	if e.Reason == "" {
		return fmt.Sprintf("network access denied: %s", e.URL)
	}
	return fmt.Sprintf("network access denied: %s: %s", e.Reason, e.URL)
}

func (*AccessDeniedError) networkDenied() {}

type MethodNotAllowedError struct {
	Method string
}

func (e *MethodNotAllowedError) Error() string {
	return fmt.Sprintf("network access denied: method %q not allowed", e.Method)
}

func (*MethodNotAllowedError) networkDenied() {}

type RedirectNotAllowedError struct {
	URL string
}

func (e *RedirectNotAllowedError) Error() string {
	return fmt.Sprintf("network redirect denied: %s", e.URL)
}

func (*RedirectNotAllowedError) networkDenied() {}

type TooManyRedirectsError struct {
	MaxRedirects int
}

func (e *TooManyRedirectsError) Error() string {
	return fmt.Sprintf("too many redirects (max %d)", e.MaxRedirects)
}

type ResponseTooLargeError struct {
	MaxBytes int64
}

func (e *ResponseTooLargeError) Error() string {
	return fmt.Sprintf("response body too large (max %d bytes)", e.MaxBytes)
}

type InvalidConfigError struct {
	Problems []string
}

func (e *InvalidConfigError) Error() string {
	return fmt.Sprintf("invalid network config:\n%s", strings.Join(e.Problems, "\n"))
}

func IsDenied(err error) bool {
	var denied deniedMarker
	return errors.As(err, &denied)
}

func WithDoer(doer HTTPDoer) Option {
	return func(client *HTTPClient) {
		client.doer = doer
	}
}

func WithResolver(resolver Resolver) Option {
	return func(client *HTTPClient) {
		client.resolver = resolver
	}
}

func New(cfg *Config, opts ...Option) (*HTTPClient, error) {
	resolved, err := resolveConfig(cfg)
	if err != nil {
		return nil, err
	}

	client := &HTTPClient{
		cfg:      resolved,
		resolver: net.DefaultResolver,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(client)
		}
	}
	if client.doer == nil {
		client.doer = client.defaultDoer()
	}
	return client, nil
}

func (c *HTTPClient) defaultDoer() HTTPDoer {
	return &http.Client{
		Transport: &http.Transport{
			// 代理按请求从 ctx 取（Do 在 UseProxy 开启且 req.ProxyURL 非空时
			// 注入）；不读进程环境变量——沙箱受控拨号，环境代理由 curl 从
			// 会话 env 显式解析后经 Request.ProxyURL 传入。
			Proxy: func(httpReq *http.Request) (*url.URL, error) {
				proxyURL, _ := httpReq.Context().Value(requestProxyKey{}).(*url.URL)
				return proxyURL, nil
			},
			DialContext:           c.dialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func (c *HTTPClient) dialContext(ctx context.Context, networkName, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}

	ips, err := c.resolveDialIPs(ctx, host)
	if err != nil {
		return nil, err
	}
	if c.cfg.denyPrivateRanges {
		if slices.ContainsFunc(ips, isPrivateIP) {
			return nil, &AccessDeniedError{URL: host, Reason: "host resolves to private or loopback IP"}
		}
	}

	dialer := net.Dialer{KeepAlive: 30 * time.Second}
	var lastErr error
	for _, ip := range ips {
		conn, err := dialer.DialContext(ctx, networkName, net.JoinHostPort(ip.String(), port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
}

func (c *HTTPClient) resolveDialIPs(ctx context.Context, host string) ([]net.IP, error) {
	if ip := net.ParseIP(host); ip != nil {
		return []net.IP{ip}, nil
	}

	addresses, err := c.lookupIPAddrs(ctx, host)
	if err != nil {
		return nil, err
	}
	ips := make([]net.IP, 0, len(addresses))
	for _, address := range addresses {
		if address.IP != nil {
			ips = append(ips, address.IP)
		}
	}
	if len(ips) == 0 {
		return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
	}
	return ips, nil
}

func (c *HTTPClient) lookupIPAddrs(ctx context.Context, host string) ([]net.IPAddr, error) {
	resolver := c.resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	return resolver.LookupIPAddr(ctx, host)
}

func resolveConfig(cfg *Config) (resolvedConfig, error) {
	if cfg == nil {
		cfg = &Config{}
	}

	entries, problems := normalizeAllowList(cfg.AllowedURLPrefixes)
	if len(problems) > 0 {
		return resolvedConfig{}, &InvalidConfigError{Problems: problems}
	}
	if len(entries) == 0 {
		return resolvedConfig{}, &InvalidConfigError{Problems: []string{"at least one allowed URL prefix is required"}}
	}

	methods := cfg.AllowedMethods
	if len(methods) == 0 {
		methods = defaultAllowedMethods
	}
	allowedMethods := make(map[Method]struct{}, len(methods))
	for _, method := range methods {
		method = Method(strings.ToUpper(strings.TrimSpace(string(method))))
		if method == "" {
			continue
		}
		allowedMethods[method] = struct{}{}
	}
	if len(allowedMethods) == 0 {
		return resolvedConfig{}, &InvalidConfigError{Problems: []string{"at least one allowed HTTP method is required"}}
	}

	maxRedirects := cfg.MaxRedirects
	if maxRedirects <= 0 {
		maxRedirects = defaultMaxRedirects
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	// maxResponseBytes <= 0 = 不限制（默认）：上限是引入方的配置项。
	maxResponseBytes := cfg.MaxResponseBytes

	return resolvedConfig{
		allowedURLPrefixes: entries,
		allowedMethods:     allowedMethods,
		maxRedirects:       maxRedirects,
		timeout:            timeout,
		maxResponseBytes:   maxResponseBytes,
		denyPrivateRanges:  cfg.DenyPrivateRanges,
		useProxy:           cfg.UseProxy,
	}, nil
}

func (c *HTTPClient) Do(ctx context.Context, req *Request) (*Response, error) {
	if req == nil {
		req = &Request{}
	}

	method := Method(strings.ToUpper(strings.TrimSpace(req.Method)))
	if method == "" {
		method = MethodGet
	}
	if _, ok := c.cfg.allowedMethods[method]; !ok {
		return nil, &MethodNotAllowedError{Method: string(method)}
	}

	currentURL := strings.TrimSpace(req.URL)
	if currentURL == "" {
		return nil, &AccessDeniedError{URL: currentURL, Reason: "missing URL"}
	}

	timeout := c.cfg.timeout
	if req.Timeout > 0 && req.Timeout < timeout {
		timeout = req.Timeout
	}
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	redirects := 0
	for {
		if err := c.checkURLAllowed(ctx, currentURL); err != nil {
			return nil, err
		}

		httpReq, err := http.NewRequestWithContext(ctx, string(method), currentURL, bytes.NewReader(req.Body))
		if err != nil {
			return nil, err
		}
		if c.cfg.useProxy && req.ProxyURL != "" {
			proxyURL, perr := ParseProxyURL(req.ProxyURL)
			if perr != nil {
				return nil, perr
			}
			httpReq = httpReq.WithContext(context.WithValue(httpReq.Context(), requestProxyKey{}, proxyURL))
		}
		for name, value := range req.Headers {
			httpReq.Header.Set(name, value)
		}

		resp, err := c.doer.Do(httpReq)
		if err != nil {
			return nil, err
		}
		if isRedirect(resp.StatusCode) && req.FollowRedirects {
			location := strings.TrimSpace(resp.Header.Get("Location"))
			if location == "" {
				return c.readResponse(req, currentURL, resp)
			}
			_ = resp.Body.Close()

			nextURL, err := resolveRedirectURL(currentURL, location)
			if err != nil {
				return nil, err
			}
			if err := c.checkURLAllowed(ctx, nextURL); err != nil {
				return nil, &RedirectNotAllowedError{URL: nextURL}
			}
			redirects++
			if redirects > c.cfg.maxRedirects {
				return nil, &TooManyRedirectsError{MaxRedirects: c.cfg.maxRedirects}
			}
			currentURL = nextURL
			continue
		}

		return c.readResponse(req, currentURL, resp)
	}
}

func (c *HTTPClient) readResponse(req *Request, requestURL string, resp *http.Response) (*Response, error) {
	defer func() { _ = resp.Body.Close() }()

	headers := make(map[string]string, len(resp.Header))
	for name, values := range resp.Header {
		headers[name] = strings.Join(values, ", ")
	}

	out := &Response{
		StatusCode: resp.StatusCode,
		Status:     resp.Status,
		Headers:    headers,
		URL:        requestURL,
	}

	// 流式投递：响应体直写 Sink，不在内存里成型。
	if req != nil && req.Sink != nil {
		if req.OnResponse != nil {
			req.OnResponse(resp.StatusCode, resp.Status, headers)
		}
		written, err := CopyResponseBody(req.Sink, resp.Body, c.cfg.maxResponseBytes, resp.ContentLength)
		if err != nil {
			return nil, err
		}
		out.Streamed = true
		out.BytesWritten = written
		return out, nil
	}

	if c.cfg.maxResponseBytes > 0 && resp.ContentLength > c.cfg.maxResponseBytes {
		return nil, &ResponseTooLargeError{MaxBytes: c.cfg.maxResponseBytes}
	}

	reader := io.Reader(resp.Body)
	if c.cfg.maxResponseBytes > 0 {
		reader = io.LimitReader(resp.Body, c.cfg.maxResponseBytes+1)
	}
	body, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	if c.cfg.maxResponseBytes > 0 && int64(len(body)) > c.cfg.maxResponseBytes {
		return nil, &ResponseTooLargeError{MaxBytes: c.cfg.maxResponseBytes}
	}
	out.Body = body
	return out, nil
}

// CopyResponseBody 把响应体从 src 流式搬运到 dst：预检 Content-Length，再按块
// 读—写，内存占用与响应体大小无关；maxBytes > 0 时跨线立即中止（超限的数据
// 不落盘）。供各 NetClient 实现复用，保证流式语义一致。
func CopyResponseBody(dst io.Writer, src io.Reader, maxBytes, contentLength int64) (int64, error) {
	if maxBytes > 0 && contentLength > maxBytes {
		return 0, &ResponseTooLargeError{MaxBytes: maxBytes}
	}
	buf := make([]byte, 32<<10)
	var total int64
	for {
		n, readErr := src.Read(buf)
		if n > 0 {
			if maxBytes > 0 && total+int64(n) > maxBytes {
				return total, &ResponseTooLargeError{MaxBytes: maxBytes}
			}
			if _, writeErr := dst.Write(buf[:n]); writeErr != nil {
				return total, writeErr
			}
			total += int64(n)
		}
		switch {
		case readErr == io.EOF:
			return total, nil
		case readErr != nil:
			return total, readErr
		}
	}
}

func (c *HTTPClient) checkURLAllowed(ctx context.Context, rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return &AccessDeniedError{URL: rawURL, Reason: "invalid URL"}
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return &AccessDeniedError{URL: rawURL, Reason: "only http and https are allowed"}
	}
	if parsed.Host == "" {
		return &AccessDeniedError{URL: rawURL, Reason: "missing host"}
	}
	if !urlAllowed(parsed, c.cfg.allowedURLPrefixes) {
		return &AccessDeniedError{URL: rawURL, Reason: "URL not in allowlist"}
	}
	if c.cfg.denyPrivateRanges {
		if err := c.checkPrivateHost(ctx, parsed); err != nil {
			return err
		}
	}
	return nil
}

func (c *HTTPClient) checkPrivateHost(ctx context.Context, parsed *url.URL) error {
	host := parsed.Hostname()
	if isPrivateHostname(host) {
		return &AccessDeniedError{URL: parsed.String(), Reason: "private or loopback host blocked"}
	}

	if ip := net.ParseIP(host); ip != nil {
		return nil
	}

	addresses, err := c.lookupIPAddrs(ctx, host)
	if err != nil {
		var dnsErr *net.DNSError
		if errors.As(err, &dnsErr) && (dnsErr.IsNotFound || dnsErr.Err == "no such host") {
			return nil
		}
		return &AccessDeniedError{URL: parsed.String(), Reason: "DNS resolution failed for private range check"}
	}
	for _, address := range addresses {
		if isPrivateIP(address.IP) {
			return &AccessDeniedError{URL: parsed.String(), Reason: "host resolves to private or loopback IP"}
		}
	}
	return nil
}

func normalizeAllowList(entries []string) (normalized []allowListEntry, problems []string) {
	if len(entries) == 0 {
		return nil, nil
	}

	out := make([]allowListEntry, 0, len(entries))
	problems = make([]string, 0)
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		parsed, err := url.Parse(entry)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			problems = append(problems, fmt.Sprintf("invalid allowlist URL %q", entry))
			continue
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			problems = append(problems, fmt.Sprintf("allowlist URL must use http or https: %q", entry))
			continue
		}
		if parsed.User != nil {
			problems = append(problems, fmt.Sprintf("allowlist URL must not include userinfo: %q", entry))
			continue
		}
		normalized := allowListEntry{
			origin:     parsed.Scheme + "://" + parsed.Host,
			pathPrefix: parsed.EscapedPath(),
		}
		if normalized.pathPrefix == "" {
			normalized.pathPrefix = "/"
		}
		key := normalized.origin + normalized.pathPrefix
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, normalized)
	}
	return out, problems
}

func urlAllowed(target *url.URL, entries []allowListEntry) bool {
	origin := target.Scheme + "://" + target.Host
	pathname := target.EscapedPath()
	if pathname == "" {
		pathname = "/"
	}

	for _, entry := range entries {
		if origin != entry.origin {
			continue
		}
		if allowListPathMatches(pathname, entry.pathPrefix) {
			return true
		}
	}
	return false
}

func allowListPathMatches(pathname, prefix string) bool {
	if prefix == "" || prefix == "/" {
		return true
	}
	if pathname == prefix {
		return true
	}
	if strings.HasSuffix(prefix, "/") {
		return strings.HasPrefix(pathname, prefix)
	}
	return strings.HasPrefix(pathname, prefix+"/")
}

func resolveRedirectURL(currentURL, location string) (string, error) {
	base, err := url.Parse(currentURL)
	if err != nil {
		return "", err
	}
	next, err := url.Parse(location)
	if err != nil {
		return "", err
	}
	return base.ResolveReference(next).String(), nil
}

// requestProxyKey 把本次请求的代理 URL 经 ctx 传给 transport 的 Proxy 挂钩
// （transport 是共享的，代理是按请求的）。
type requestProxyKey struct{}

// ParseProxyURL 规范化并校验代理 URL：缺 scheme 补 http://；只允许
// http/https/socks5/socks5h（net/http transport 原生支持的集合）。
func ParseProxyURL(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("empty proxy URL")
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid proxy URL %q: %w", raw, err)
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https", "socks5", "socks5h":
	default:
		return nil, fmt.Errorf("unsupported proxy scheme %q (want http/https/socks5/socks5h)", u.Scheme)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("invalid proxy URL %q: missing host", raw)
	}
	return u, nil
}

func isRedirect(statusCode int) bool {	switch statusCode {
	case http.StatusMovedPermanently,
		http.StatusFound,
		http.StatusSeeOther,
		http.StatusTemporaryRedirect,
		http.StatusPermanentRedirect:
		return true
	default:
		return false
	}
}

func isPrivateHostname(host string) bool {
	host = strings.TrimSpace(strings.ToLower(host))
	host = strings.TrimPrefix(strings.TrimSuffix(host, "]"), "[")
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return isPrivateIP(ip)
}

func isPrivateIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalMulticast() || ip.IsLinkLocalUnicast()
}
