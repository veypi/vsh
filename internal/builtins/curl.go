package builtins

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	stdfs "io/fs"
	"maps"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	gbfs "github.com/veypi/vsh/fs"
	"github.com/veypi/vsh/internal/vshversion"
	"github.com/veypi/vsh/network"
)

type Curl struct{}

type curlOptions struct {
	method          string
	headers         map[string]string
	data            string
	hasData         bool
	dataBinary      bool
	formFields      []curlFormField
	user            string
	uploadFile      string
	cookieJar       string
	outputFile      string
	useRemoteName   bool
	headOnly        bool
	includeHeaders  bool
	silent          bool
	showError       bool
	failSilently    bool
	followRedirects bool
	writeOut        string
	verbose         bool
	timeout         time.Duration
	url             string
	proxy           string
	proxySet        bool // 区分「未给 -x」与「-x "" 显式禁用代理」
	insecure        bool
	dumpHeader      string
	retry           int
	retryDelay      time.Duration
}

type curlFormField struct {
	name        string
	value       string
	filename    string
	contentType string
}

const (
	curlFormBoundary = "----CurlFormBoundaryjbgo"
)

// curlVersionText 与真实 curl 同构（首列后第一个词是版本号），版本号跟随 vsh。
const curlVersionText = "curl " + vshversion.Version + " (vsh builtin)\n" +
	"Protocols: http https\n" +
	"Features: HTTP2\n"

const curlHelpText = `usage: curl [OPTIONS] URL

  -X, --request METHOD       HTTP method (GET, POST, PUT, DELETE, etc.)
  -H, --header HEADER        add header (can be used multiple times)
  -d, --data DATA            HTTP POST data
      --data-raw DATA        HTTP POST data (no @ interpretation)
      --data-binary DATA     HTTP POST binary data
      --data-urlencode DATA  URL-encode and POST data
  -F, --form NAME=VALUE      multipart form data
  -u, --user USER:PASS       HTTP authentication
  -A, --user-agent STR       set User-Agent header
  -e, --referer URL          set Referer header
  -b, --cookie DATA          send cookies
  -c, --cookie-jar FILE      save cookies to file
  -T, --upload-file FILE     upload file (defaults method to PUT)
  -o, --output FILE          write output to file
  -O, --remote-name          write to a file named from the URL
  -I, --head                 show headers only (HEAD request)
  -i, --include              include response headers in output
  -s, --silent               silent mode (no progress)
  -S, --show-error           show errors even when silent
  -f, --fail                 fail on HTTP status >= 400
  -L, --location             follow redirects (default)
      --max-redirs NUM       accepted for compatibility
  -m, --max-time SECS        maximum request time
      --connect-timeout SECS accepted as an overall timeout fallback
  -x, --proxy URL            use proxy ([scheme://]host[:port]; empty disables; overrides env)
  -w, --write-out FMT        output format after completion
  -v, --verbose              verbose output (stderr)
  -N, --no-buffer            disable output buffering (always on: writes flush per chunk)
  -D, --dump-header FILE     write response headers to file
  -k, --insecure             skip TLS certificate verification
      --compressed           request a compressed response (gzip is transparent; br/zstd unsupported)
      --retry NUM            retry transient failures (network error/408/429/5xx) up to NUM times
      --retry-delay SECS     wait between retries (default 1)
  -g, --globoff              accepted for compatibility (URLs are never globbed)
      --http1.1/--http2/--http3 accepted for compatibility
  -#, --progress-bar         accepted for compatibility (no progress meter)
  -V, --version              show version and exit
      --help                 show this help
`

func NewCurl() *Curl {
	return &Curl{}
}

func (c *Curl) Name() string {
	return "curl"
}

func (c *Curl) Run(ctx context.Context, inv *Invocation) error {
	return RunCommand(ctx, c, inv)
}

func (c *Curl) Spec() CommandSpec {
	return CommandSpec{
		Name:  "curl",
		Usage: "curl [OPTIONS] URL",
		Options: []OptionSpec{
			{Name: "request", Short: 'X', Long: "request", Arity: OptionRequiredValue, ValueName: "METHOD", Help: "HTTP method (GET, POST, PUT, DELETE, etc.)"},
			{Name: "header", Short: 'H', Long: "header", Arity: OptionRequiredValue, ValueName: "HEADER", Repeatable: true, Help: "add header (can be used multiple times)"},
			{Name: "data", Short: 'd', Long: "data", Arity: OptionRequiredValue, ValueName: "DATA", Help: "HTTP POST data"},
			{Name: "data-raw", Long: "data-raw", Arity: OptionRequiredValue, ValueName: "DATA", Help: "HTTP POST data (no @ interpretation)"},
			{Name: "data-binary", Long: "data-binary", Arity: OptionRequiredValue, ValueName: "DATA", Help: "HTTP POST binary data"},
			{Name: "data-urlencode", Long: "data-urlencode", Arity: OptionRequiredValue, ValueName: "DATA", Repeatable: true, Help: "URL-encode and POST data"},
			{Name: "form", Short: 'F', Long: "form", Arity: OptionRequiredValue, ValueName: "NAME=VALUE", Repeatable: true, Help: "multipart form data"},
			{Name: "user", Short: 'u', Long: "user", Arity: OptionRequiredValue, ValueName: "USER:PASS", Help: "HTTP authentication"},
			{Name: "user-agent", Short: 'A', Long: "user-agent", Arity: OptionRequiredValue, ValueName: "STR", Help: "set User-Agent header"},
			{Name: "referer", Short: 'e', Long: "referer", Arity: OptionRequiredValue, ValueName: "URL", Help: "set Referer header"},
			{Name: "cookie", Short: 'b', Long: "cookie", Arity: OptionRequiredValue, ValueName: "DATA", Help: "send cookies"},
			{Name: "cookie-jar", Short: 'c', Long: "cookie-jar", Arity: OptionRequiredValue, ValueName: "FILE", Help: "save cookies to file"},
			{Name: "upload-file", Short: 'T', Long: "upload-file", Arity: OptionRequiredValue, ValueName: "FILE", Help: "upload file (defaults method to PUT)"},
			{Name: "output", Short: 'o', Long: "output", Arity: OptionRequiredValue, ValueName: "FILE", Help: "write output to file"},
			{Name: "remote-name", Short: 'O', Long: "remote-name", Help: "write to a file named from the URL"},
			{Name: "head", Short: 'I', Long: "head", Help: "show headers only (HEAD request)"},
			{Name: "include", Short: 'i', Long: "include", Help: "include response headers in output"},
			{Name: "silent", Short: 's', Long: "silent", Help: "silent mode (no progress)"},
			{Name: "show-error", Short: 'S', Long: "show-error", Help: "show errors even when silent"},
			{Name: "fail", Short: 'f', Long: "fail", Help: "fail on HTTP status >= 400"},
			{Name: "location", Short: 'L', Long: "location", Help: "follow redirects (default)"},
			{Name: "max-redirs", Long: "max-redirs", Arity: OptionOptionalValue, ValueName: "NUM", Help: "accepted for compatibility"},
			{Name: "max-time", Short: 'm', Long: "max-time", Arity: OptionRequiredValue, ValueName: "SECS", Help: "maximum request time"},
			{Name: "connect-timeout", Long: "connect-timeout", Arity: OptionRequiredValue, ValueName: "SECS", Help: "accepted as an overall timeout fallback"},
			{Name: "proxy", Short: 'x', Long: "proxy", Arity: OptionRequiredValue, ValueName: "URL", Help: "use proxy ([scheme://]host[:port]; empty disables; overrides env)"},
			{Name: "write-out", Short: 'w', Long: "write-out", Arity: OptionRequiredValue, ValueName: "FMT", Help: "output format after completion"},
			{Name: "verbose", Short: 'v', Long: "verbose", Help: "verbose output (stderr)"},
			{Name: "no-buffer", Short: 'N', Long: "no-buffer", Help: "disable output buffering (always on)"},
			{Name: "dump-header", Short: 'D', Long: "dump-header", Arity: OptionRequiredValue, ValueName: "FILE", Help: "write response headers to file"},
			{Name: "insecure", Short: 'k', Long: "insecure", Help: "skip TLS certificate verification"},
			{Name: "compressed", Long: "compressed", Help: "request a compressed response (gzip is transparent)"},
			{Name: "retry", Long: "retry", Arity: OptionRequiredValue, ValueName: "NUM", Help: "retry transient failures up to NUM times"},
			{Name: "retry-delay", Long: "retry-delay", Arity: OptionRequiredValue, ValueName: "SECS", Help: "wait between retries (default 1)"},
			{Name: "globoff", Short: 'g', Long: "globoff", Help: "accepted for compatibility (URLs are never globbed)"},
			{Name: "http1.1", Long: "http1.1", Help: "accepted for compatibility"},
			{Name: "http2", Long: "http2", Help: "accepted for compatibility"},
			{Name: "http3", Long: "http3", Help: "accepted for compatibility"},
			{Name: "progress-bar", Short: '#', Long: "progress-bar", Help: "accepted for compatibility (no progress meter)"},
			{Name: "version", Short: 'V', Long: "version", Help: "show version and exit"},
			{Name: "help", Long: "help", Help: "show this help"},
		},
		Args: []ArgSpec{
			{Name: "args", ValueName: "ARG", Repeatable: true},
		},
		Parse: ParseConfig{
			GroupShortOptions:     true,
			LongOptionValueEquals: true,
		},
		HelpRenderer: renderStaticHelp(curlHelpText),
	}
}

func (c *Curl) NormalizeInvocation(inv *Invocation) *Invocation {
	if inv == nil {
		return nil
	}
	normalized := make([]string, 0, len(inv.Args)+4)
	args := inv.Args
	for i, arg := range args {
		if arg == "--" {
			normalized = append(normalized, arg)
			normalized = append(normalized, args[i+1:]...)
			break
		}
		if option, value, ok := splitCurlAttachedShort(arg); ok {
			normalized = append(normalized, option, value)
			continue
		}
		normalized = append(normalized, arg)
		if fallback, ok := curlMissingValueFallback(arg); ok && i == len(args)-1 {
			normalized = append(normalized, fallback)
		}
	}
	clone := *inv
	clone.Args = normalized
	return &clone
}

func (c *Curl) RunParsed(ctx context.Context, inv *Invocation, matches *ParsedCommand) error {
	if matches.Has("help") {
		return renderStaticHelp(curlHelpText)(inv.Stdout, c.Spec())
	}
	if matches.Has("version") {
		if _, err := io.WriteString(inv.Stdout, curlVersionText); err != nil {
			return &ExitError{Code: 1, Err: err}
		}
		return nil
	}

	opts, err := parseCurlMatches(matches)
	if err != nil {
		return err
	}
	if inv.Fetch == nil {
		return exitf(inv, 1, "curl: internal error: network client not available")
	}

	requestURL := opts.url
	if !strings.HasPrefix(requestURL, "http://") && !strings.HasPrefix(requestURL, "https://") {
		requestURL = "https://" + requestURL
	}

	body, contentType, err := prepareCurlRequestBody(ctx, inv, opts)
	if err != nil {
		return err
	}
	headers := prepareCurlHeaders(opts, contentType)

	// 输出目标：-o/-O 落盘，否则 stdout。两条路都走流式投递（-I 时正文写进
	// sink 直接被丢弃）——SSE/长响应边收边出，不再攒到结束才吐字节。
	var (
		outFile gbfs.File
		outAbs  string
		out     io.Writer = inv.Stdout
	)
	if opts.outputFile != "" || opts.useRemoteName {
		filename := opts.outputFile
		if filename == "" {
			filename = extractCurlFilename(requestURL)
		}
		outAbs = gbfs.Resolve(inv.Cwd, filename)
		outFile, err = createFileForWrite(ctx, inv, outAbs, 0o644)
		if err != nil {
			return err
		}
		out = outFile
	}
	sink := newCurlSink(out)
	sink.drop = opts.headOnly

	// -D：头块按跳累积，结束时一次落盘（启动即截断，语义同真实 curl）。
	var dumpBuf bytes.Buffer
	dumpAbs := ""
	if opts.dumpHeader != "" {
		dumpAbs = gbfs.Resolve(inv.Cwd, opts.dumpHeader)
	}

	// -v 的调试流走 stderr（真实 curl 行为），管道里不会污染正文。
	if opts.verbose {
		var b strings.Builder
		fmt.Fprintf(&b, "> %s %s\n", opts.method, requestURL)
		curlWriteVerboseHeaders(&b, '>', headers)
		b.WriteString(">\n")
		_, _ = io.WriteString(inv.Stderr, b.String())
	}

	attempt := 0
	retryPending := false
	var headerErr error
	// OnResponse 在响应头到达、正文写出之前回调：-D/-v/-i 的前缀都从这里按
	// 到达顺序播出，与正文的流式写入保持先后关系。
	onResponse := func(statusCode int, status string, respHeaders map[string]string) {
		statusText := curlStatusTextOf(status, statusCode)
		if opts.dumpHeader != "" {
			fmt.Fprintf(&dumpBuf, "HTTP/1.1 %d %s\r\n", statusCode, statusText)
			dumpBuf.WriteString(formatCurlHeaders(respHeaders))
			dumpBuf.WriteString("\r\n\r\n")
		}
		if opts.verbose {
			var b strings.Builder
			fmt.Fprintf(&b, "< HTTP/1.1 %d %s\n", statusCode, statusText)
			curlWriteVerboseHeaders(&b, '<', respHeaders)
			b.WriteString("<\n")
			_, _ = io.WriteString(inv.Stderr, b.String())
		}
		// 本次响应整块丢弃（要重试 / -f 失败）：连 -i 头块也不写，避免前缀重复。
		if sink.body == 0 && attempt < opts.retry && curlRetryableStatus(statusCode) {
			sink.drop = true
			retryPending = true
			return
		}
		if opts.failSilently && statusCode >= 400 {
			sink.drop = true
			return
		}
		if opts.includeHeaders || opts.headOnly {
			if werr := sink.writeHead(fmt.Sprintf("HTTP/1.1 %d %s\r\n%s\r\n\r\n", statusCode, statusText, formatCurlHeaders(respHeaders))); werr != nil && headerErr == nil {
				headerErr = werr
			}
		}
	}

	req := &network.Request{
		Method:          opts.method,
		URL:             requestURL,
		Headers:         headers,
		Body:            body,
		FollowRedirects: opts.followRedirects,
		Timeout:         opts.timeout,
		ProxyURL:        resolveCurlProxy(inv, requestURL, opts),
		Insecure:        opts.insecure,
		Sink:            sink,
		OnResponse:      onResponse,
	}

	var resp *network.Response
	for attempt = 0; ; attempt++ {
		sink.drop = opts.headOnly
		retryPending = false
		resp, err = inv.Fetch(ctx, req)
		if err == nil && headerErr != nil {
			err = headerErr
		}
		if err != nil {
			// 已经写出正文字节就不再重试（重试无法撤回已交付的数据）。
			if sink.body > 0 || attempt >= opts.retry {
				break
			}
			if !curlRetrySleep(ctx, opts) {
				err = ctx.Err()
				if err == nil {
					err = context.Canceled
				}
				break
			}
			continue
		}
		if retryPending {
			if !curlRetrySleep(ctx, opts) {
				err = ctx.Err()
				if err == nil {
					err = context.Canceled
				}
				break
			}
			continue
		}
		// 非流式客户端不会回调 onResponse，重试判定得在收齐后补一次。
		if !resp.Streamed && attempt < opts.retry && curlRetryableStatus(resp.StatusCode) {
			if !curlRetrySleep(ctx, opts) {
				err = ctx.Err()
				if err == nil {
					err = context.Canceled
				}
				break
			}
			continue
		}
		break
	}
	if err != nil {
		writeCurlDump(ctx, inv, dumpAbs, dumpBuf.Bytes())
		curlDiscardOutput(ctx, inv, outFile, outAbs)
		return curlRequestError(inv, opts, err)
	}

	// 客户端未实现流式投递：头块与正文在响应收齐后一次补齐（顺序与流式一致）。
	if !resp.Streamed {
		statusText := curlStatusText(resp)
		if opts.dumpHeader != "" {
			fmt.Fprintf(&dumpBuf, "HTTP/1.1 %d %s\r\n", resp.StatusCode, statusText)
			dumpBuf.WriteString(formatCurlHeaders(resp.Headers))
			dumpBuf.WriteString("\r\n\r\n")
		}
		if opts.verbose {
			var b strings.Builder
			fmt.Fprintf(&b, "< HTTP/1.1 %d %s\n", resp.StatusCode, statusText)
			curlWriteVerboseHeaders(&b, '<', resp.Headers)
			b.WriteString("<\n")
			_, _ = io.WriteString(inv.Stderr, b.String())
		}
		if !(opts.failSilently && resp.StatusCode >= 400) && (opts.includeHeaders || opts.headOnly) {
			if werr := sink.writeHead(fmt.Sprintf("HTTP/1.1 %d %s\r\n%s\r\n\r\n", resp.StatusCode, statusText, formatCurlHeaders(resp.Headers))); werr != nil {
				curlDiscardOutput(ctx, inv, outFile, outAbs)
				return &ExitError{Code: 1, Err: werr}
			}
		}
	}

	if err := saveCurlCookies(ctx, inv, opts, resp); err != nil {
		curlDiscardOutput(ctx, inv, outFile, outAbs)
		return err
	}

	if opts.failSilently && resp.StatusCode >= 400 {
		// -f：失败不留输出文件（与 curl -f「不产生输出」一致）；stdout 路径的
		// 错误正文已在 onResponse 里被丢弃。
		curlDiscardOutput(ctx, inv, outFile, outAbs)
		if !opts.silent || opts.showError {
			return exitf(inv, 22, "curl: (22) The requested URL returned error: %d", resp.StatusCode)
		}
		return &ExitError{Code: 22}
	}

	if !resp.Streamed && !opts.headOnly {
		if _, werr := sink.Write(resp.Body); werr != nil {
			curlDiscardOutput(ctx, inv, outFile, outAbs)
			return &ExitError{Code: 1, Err: werr}
		}
	}

	if outFile != nil {
		if cerr := outFile.Close(); cerr != nil {
			_ = inv.FS.Remove(ctx, outAbs, false)
			return &ExitError{Code: 1, Err: cerr}
		}
		recordFileMutation(inv.TraceRecorder(), "write", outAbs, outAbs, outAbs)
	}

	if err := writeCurlDump(ctx, inv, dumpAbs, dumpBuf.Bytes()); err != nil {
		return err
	}

	if opts.writeOut != "" {
		if _, werr := io.WriteString(inv.Stdout, applyCurlWriteOut(opts.writeOut, resp)); werr != nil {
			return &ExitError{Code: 1, Err: werr}
		}
	}
	return nil
}

func parseCurlMatches(matches *ParsedCommand) (*curlOptions, error) {
	opts := &curlOptions{
		method:          "GET",
		headers:         make(map[string]string),
		followRedirects: true,
	}
	impliesPost := false
	values := map[string][]string{
		"request":         matches.Values("request"),
		"header":          matches.Values("header"),
		"data":            matches.Values("data"),
		"data-raw":        matches.Values("data-raw"),
		"data-binary":     matches.Values("data-binary"),
		"data-urlencode":  matches.Values("data-urlencode"),
		"form":            matches.Values("form"),
		"user":            matches.Values("user"),
		"user-agent":      matches.Values("user-agent"),
		"referer":         matches.Values("referer"),
		"cookie":          matches.Values("cookie"),
		"cookie-jar":      matches.Values("cookie-jar"),
		"upload-file":     matches.Values("upload-file"),
		"max-time":        matches.Values("max-time"),
		"connect-timeout": matches.Values("connect-timeout"),
		"proxy":           matches.Values("proxy"),
		"output":          matches.Values("output"),
		"write-out":       matches.Values("write-out"),
		"dump-header":     matches.Values("dump-header"),
		"retry":           matches.Values("retry"),
		"retry-delay":     matches.Values("retry-delay"),
	}
	indexes := make(map[string]int, len(values))

	for _, name := range matches.OptionOrder() {
		value := curlNextMatchValue(values, indexes, name)
		switch name {
		case "request":
			opts.method = value
		case "header":
			parseCurlHeader(opts.headers, value)
		case "data", "data-raw":
			opts.data = value
			opts.hasData = true
			impliesPost = true
		case "data-binary":
			opts.data = value
			opts.hasData = true
			opts.dataBinary = true
			impliesPost = true
		case "data-urlencode":
			curlAppendURLEncodedData(opts, value)
			impliesPost = true
		case "form":
			if field, ok := parseCurlFormField(value); ok {
				opts.formFields = append(opts.formFields, field)
			}
			impliesPost = true
		case "user":
			opts.user = value
		case "user-agent":
			opts.headers["User-Agent"] = value
		case "referer":
			opts.headers["Referer"] = value
		case "cookie":
			opts.headers["Cookie"] = value
		case "cookie-jar":
			opts.cookieJar = value
		case "upload-file":
			opts.uploadFile = value
			if opts.method == "GET" {
				opts.method = "PUT"
			}
		case "max-time":
			opts.timeout = parseCurlTimeout(value)
		case "connect-timeout":
			if opts.timeout <= 0 {
				opts.timeout = parseCurlTimeout(value)
			}
		case "proxy":
			opts.proxy = value
			opts.proxySet = true
		case "output":
			opts.outputFile = value
		case "remote-name":
			opts.useRemoteName = true
		case "head":
			opts.headOnly = true
			opts.method = "HEAD"
		case "include":
			opts.includeHeaders = true
		case "silent":
			opts.silent = true
		case "show-error":
			opts.showError = true
		case "fail":
			opts.failSilently = true
		case "location":
			opts.followRedirects = true
		case "max-redirs":
			// Accepted for compatibility. Redirect depth is enforced by network.Config.
		case "write-out":
			opts.writeOut = value
		case "verbose":
			opts.verbose = true
		case "no-buffer":
			// 默认就是逐块写 + flush，-N 只是兼容旗标。
		case "dump-header":
			opts.dumpHeader = value
		case "insecure":
			opts.insecure = true
		case "compressed":
			// 不显式设 Accept-Encoding：Go transport 默认请求 gzip 并透明解压。
		case "retry":
			opts.retry = parseCurlCount(value)
		case "retry-delay":
			opts.retryDelay = parseCurlTimeout(value)
		case "globoff", "http1.1", "http2", "http3", "progress-bar":
			// 兼容旗标：URL 从不展开通配，协议版本由 transport 协同（h2 默认），
			// 也没有进度条可关。
		}
	}

	if impliesPost && opts.method == "GET" {
		opts.method = "POST"
	}
	args := matches.Positionals()
	if len(args) > 0 {
		opts.url = args[len(args)-1]
	}
	if opts.url == "" {
		return nil, &ExitError{Code: 2, Err: errors.New("curl: no URL specified")}
	}
	return opts, nil
}

func curlNextMatchValue(values map[string][]string, indexes map[string]int, name string) string {
	valueIndex := indexes[name]
	indexes[name] = valueIndex + 1
	if valueIndex >= len(values[name]) {
		return ""
	}
	return values[name][valueIndex]
}

func splitCurlAttachedShort(arg string) (option, value string, ok bool) {
	if len(arg) <= 2 || !strings.HasPrefix(arg, "-") || strings.HasPrefix(arg, "--") {
		return "", "", false
	}
	switch arg[:2] {
	case "-X", "-d", "-u", "-A", "-e", "-b", "-x", "-o", "-D":
		return arg[:2], arg[2:], true
	default:
		return "", "", false
	}
}

func curlMissingValueFallback(arg string) (fallback string, ok bool) {
	switch arg {
	case "-X", "--request":
		return "GET", true
	case "-H", "--header", "-d", "--data", "--data-raw", "--data-binary", "--data-urlencode", "-F", "--form",
		"-u", "--user", "-A", "--user-agent", "-e", "--referer", "-b", "--cookie", "-c", "--cookie-jar",
		"-T", "--upload-file", "-m", "--max-time", "--connect-timeout", "-x", "--proxy", "-o", "--output", "-w", "--write-out",
		"-D", "--dump-header", "--retry", "--retry-delay":
		return "", true
	default:
		return "", false
	}
}

func curlAppendURLEncodedData(opts *curlOptions, value string) {
	if opts.hasData && opts.data != "" {
		opts.data += "&"
	}
	opts.data += encodeCurlFormData(value)
	opts.hasData = true
}

func parseCurlTimeout(value string) time.Duration {
	secs, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || secs <= 0 {
		return 0
	}
	return time.Duration(secs * float64(time.Second))
}

// resolveCurlProxy 解析本次请求的代理：-x/--proxy 优先于会话环境变量；
// -x "" 显式禁用。环境变量按目标 scheme 取 https_proxy/http_proxy（小写
// 优先、大写兼容），兜底 all_proxy；no_proxy 命中则直连。返回值已规范化
// （缺 scheme 补 http://）。是否真正使用由网络客户端的 UseProxy 配置决定
// （未启用时客户端忽略 Request.ProxyURL）。不读进程环境——沙箱内 curl 只
// 看会话 env。
func resolveCurlProxy(inv *Invocation, requestURL string, opts *curlOptions) string {
	if opts.proxySet {
		return normalizeCurlProxyURL(opts.proxy)
	}
	if inv == nil || len(inv.Env) == 0 {
		return ""
	}
	if curlNoProxyMatch(envValue(inv.Env, "no_proxy", "NO_PROXY"), requestURL) {
		return ""
	}
	scheme := "https"
	if strings.HasPrefix(requestURL, "http://") {
		scheme = "http"
	}
	raw := envValue(inv.Env, scheme+"_proxy", strings.ToUpper(scheme)+"_PROXY")
	if raw == "" {
		raw = envValue(inv.Env, "all_proxy", "ALL_PROXY")
	}
	return normalizeCurlProxyURL(raw)
}

func envValue(env map[string]string, names ...string) string {
	for _, name := range names {
		if v := strings.TrimSpace(env[name]); v != "" {
			return v
		}
	}
	return ""
}

func normalizeCurlProxyURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	return raw
}

// curlNoProxyMatch 判断 no_proxy 列表是否命中目标：逗号分隔；"*" 全命中；
// 条目按主机后缀匹配（前导 . 忽略），可带 :port 限定；大小写不敏感。
func curlNoProxyMatch(noProxy, requestURL string) bool {
	if strings.TrimSpace(noProxy) == "" {
		return false
	}
	parsed, err := url.Parse(requestURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "" {
		return false
	}
	port := parsed.Port()
	if port == "" {
		if parsed.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	for _, entry := range strings.Split(noProxy, ",") {
		entry = strings.ToLower(strings.TrimSpace(entry))
		if entry == "" {
			continue
		}
		if entry == "*" {
			return true
		}
		entryHost, entryPort, hasPort := strings.Cut(entry, ":")
		if hasPort && entryPort != port {
			continue
		}
		entryHost = strings.TrimPrefix(entryHost, ".")
		if host == entryHost || strings.HasSuffix(host, "."+entryHost) {
			return true
		}
	}
	return false
}

func parseCurlHeader(headers map[string]string, header string) {
	colonIndex := strings.IndexByte(header, ':')
	if colonIndex <= 0 {
		return
	}
	name := strings.TrimSpace(header[:colonIndex])
	if name == "" {
		return
	}
	headers[name] = strings.TrimSpace(header[colonIndex+1:])
}

func parseCurlFormField(spec string) (curlFormField, bool) {
	before, after, ok := strings.Cut(spec, "=")
	if !ok {
		return curlFormField{}, false
	}

	field := curlFormField{
		name:  before,
		value: after,
	}

	if idx := strings.LastIndex(field.value, ";type="); idx >= 0 && !strings.Contains(field.value[idx+len(";type="):], ";") {
		field.contentType = field.value[idx+len(";type="):]
		field.value = field.value[:idx]
	}
	if idx := strings.Index(field.value, ";filename="); idx >= 0 {
		filename := field.value[idx+len(";filename="):]
		if cut := strings.IndexByte(filename, ';'); cut >= 0 {
			filename = filename[:cut]
		}
		field.filename = filename
		field.value = strings.Replace(field.value, ";filename="+filename, "", 1)
	}
	if strings.HasPrefix(field.value, "@") || strings.HasPrefix(field.value, "<") {
		if field.filename == "" {
			field.filename = path.Base(field.value[1:])
		}
	}
	return field, true
}

func encodeCurlFormData(input string) string {
	before, after, ok := strings.Cut(input, "=")
	if ok {
		name := before
		value := after
		if name != "" {
			return curlURIComponent(name) + "=" + curlURIComponent(value)
		}
		return curlURIComponent(value)
	}
	return curlURIComponent(input)
}

func curlURIComponent(value string) string {
	return url.QueryEscape(value)
}

func prepareCurlRequestBody(ctx context.Context, inv *Invocation, opts *curlOptions) (body []byte, contentType string, err error) {
	if opts.uploadFile != "" {
		data, _, err := readAllFile(ctx, inv, opts.uploadFile)
		if err != nil {
			return nil, "", err
		}
		return data, "", nil
	}

	if len(opts.formFields) > 0 {
		fileContents := make(map[string][]byte, len(opts.formFields))
		for _, field := range opts.formFields {
			if !strings.HasPrefix(field.value, "@") && !strings.HasPrefix(field.value, "<") {
				continue
			}
			target := field.value[1:]
			data, _, err := readAllFile(ctx, inv, target)
			if err != nil {
				if errors.Is(err, stdfs.ErrNotExist) {
					fileContents[target] = nil
					continue
				}
				return nil, "", err
			}
			fileContents[target] = data
		}
		return generateCurlMultipartBody(opts.formFields, fileContents), "multipart/form-data; boundary=" + curlFormBoundary, nil
	}

	if opts.hasData {
		return []byte(opts.data), "", nil
	}

	return nil, "", nil
}

func generateCurlMultipartBody(fields []curlFormField, fileContents map[string][]byte) []byte {
	var buf bytes.Buffer

	for _, field := range fields {
		value := []byte(field.value)
		if strings.HasPrefix(field.value, "@") || strings.HasPrefix(field.value, "<") {
			value = fileContents[field.value[1:]]
		}

		fmt.Fprintf(&buf, "--%s\r\n", curlFormBoundary)
		if field.filename != "" {
			fmt.Fprintf(&buf, "Content-Disposition: form-data; name=%q; filename=%q\r\n", field.name, field.filename)
			if field.contentType != "" {
				fmt.Fprintf(&buf, "Content-Type: %s\r\n", field.contentType)
			}
		} else {
			fmt.Fprintf(&buf, "Content-Disposition: form-data; name=%q\r\n", field.name)
		}
		buf.WriteString("\r\n")
		buf.Write(value)
		buf.WriteString("\r\n")
	}

	fmt.Fprintf(&buf, "--%s--\r\n", curlFormBoundary)
	return buf.Bytes()
}

func prepareCurlHeaders(opts *curlOptions, contentType string) map[string]string {
	headers := make(map[string]string, len(opts.headers)+2)
	maps.Copy(headers, opts.headers)
	if opts.user != "" {
		headers["Authorization"] = "Basic " + base64.StdEncoding.EncodeToString([]byte(opts.user))
	}
	if contentType != "" && !curlHasHeader(headers, "Content-Type") {
		headers["Content-Type"] = contentType
	}
	return headers
}

func curlHasHeader(headers map[string]string, name string) bool {
	for existing := range headers {
		if strings.EqualFold(existing, name) {
			return true
		}
	}
	return false
}

func saveCurlCookies(ctx context.Context, inv *Invocation, opts *curlOptions, resp *network.Response) error {
	if opts.cookieJar == "" {
		return nil
	}
	setCookie := curlHeaderValue(resp.Headers, "Set-Cookie")
	if setCookie == "" {
		return nil
	}
	return writeFileContents(ctx, inv, gbfs.Resolve(inv.Cwd, opts.cookieJar), []byte(setCookie), 0o644)
}

// curlSink 是 curl 的流式输出目标：正文逐块写入并立即 flush（-N 的语义就是
// 默认行为，不再额外缓冲）；drop 用于丢弃整块响应（-I 的正文、-f 的错误正文、
// 待重试的那一次响应），body 记录已交付的正文字节（重试判定用，头块不计入）。
type curlSink struct {
	w       io.Writer
	flusher interface{ Flush() error }
	drop    bool
	body    int64
}

func newCurlSink(w io.Writer) *curlSink {
	sink := &curlSink{w: w}
	if flusher, ok := w.(interface{ Flush() error }); ok {
		sink.flusher = flusher
	}
	return sink
}

func (s *curlSink) Write(p []byte) (int, error) {
	if s.drop {
		return len(p), nil
	}
	n, err := s.w.Write(p)
	s.body += int64(n)
	if err == nil {
		s.flush()
	}
	return n, err
}

// writeHead 写 -i/-I 的头块（不计入 body，不影响重试判定）。
func (s *curlSink) writeHead(block string) error {
	if _, err := io.WriteString(s.w, block); err != nil {
		return err
	}
	s.flush()
	return nil
}

func (s *curlSink) flush() {
	if s.flusher != nil {
		_ = s.flusher.Flush()
	}
}

func curlWriteVerboseHeaders(w io.Writer, prefix rune, headers map[string]string) {
	keys := make([]string, 0, len(headers))
	for name := range headers {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	for _, name := range keys {
		fmt.Fprintf(w, "%c %s: %s\n", prefix, name, headers[name])
	}
}

// curlRetryableStatus 与真实 curl --retry 的默认可重试状态一致。
func curlRetryableStatus(statusCode int) bool {
	switch statusCode {
	case 408, 429, 500, 502, 503, 504:
		return true
	default:
		return false
	}
}

// curlRetrySleep 等待下一次重试；ctx 结束时返回 false（调用方停止重试）。
func curlRetrySleep(ctx context.Context, opts *curlOptions) bool {
	delay := opts.retryDelay
	if delay <= 0 {
		delay = time.Second
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func parseCurlCount(value string) int {
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// writeCurlDump 落盘 -D 累积的头块；未指定 -D 时什么也不做。
func writeCurlDump(ctx context.Context, inv *Invocation, abs string, data []byte) error {
	if abs == "" {
		return nil
	}
	return writeFileContents(ctx, inv, abs, data, 0o644)
}

func curlStatusText(resp *network.Response) string {
	if resp == nil {
		return ""
	}
	return curlStatusTextOf(resp.Status, resp.StatusCode)
}

func curlStatusTextOf(status string, statusCode int) string {
	status = strings.TrimSpace(status)
	if status == "" {
		return http.StatusText(statusCode)
	}
	if after, ok := strings.CutPrefix(status, strconv.Itoa(statusCode)); ok {
		return strings.TrimSpace(after)
	}
	return status
}

// curlDiscardOutput 关闭并删除未完成的下载文件：fetch/落盘中途失败时不留半截
// 文件（否则调用方会把残缺体当成完整结果）。
func curlDiscardOutput(ctx context.Context, inv *Invocation, file gbfs.File, abs string) {
	if file == nil {
		return
	}
	_ = file.Close()
	_ = inv.FS.Remove(ctx, abs, false)
}

func formatCurlHeaders(headers map[string]string) string {
	keys := make([]string, 0, len(headers))
	for name := range headers {
		keys = append(keys, name)
	}
	sort.Strings(keys)

	lines := make([]string, 0, len(keys))
	for _, name := range keys {
		lines = append(lines, fmt.Sprintf("%s: %s", name, headers[name]))
	}
	return strings.Join(lines, "\r\n")
}

func extractCurlFilename(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "index.html"
	}
	name := path.Base(parsed.Path)
	if name == "." || name == "/" || name == "" {
		return "index.html"
	}
	return name
}

func applyCurlWriteOut(format string, resp *network.Response) string {
	output := format
	output = strings.ReplaceAll(output, "%{http_code}", strconv.Itoa(resp.StatusCode))
	output = strings.ReplaceAll(output, "%{content_type}", curlHeaderValue(resp.Headers, "Content-Type"))
	output = strings.ReplaceAll(output, "%{url_effective}", resp.URL)
	output = strings.ReplaceAll(output, "%{size_download}", strconv.FormatInt(curlDownloadSize(resp), 10))
	output = strings.ReplaceAll(output, "\\n", "\n")
	return output
}

func curlHeaderValue(headers map[string]string, name string) string {
	for key, value := range headers {
		if strings.EqualFold(key, name) {
			return value
		}
	}
	return ""
}

// curlDownloadSize 已下载字节数：流式投递时为 BytesWritten（Body 为空），
// 内存路径为 Body 长度。
func curlDownloadSize(resp *network.Response) int64 {
	if resp == nil {
		return 0
	}
	if resp.Streamed {
		return resp.BytesWritten
	}
	return int64(len(resp.Body))
}

func curlRequestError(inv *Invocation, opts *curlOptions, err error) error {
	exitCode := 1

	var methodErr *network.MethodNotAllowedError
	var redirectErr *network.RedirectNotAllowedError
	var redirectLimitErr *network.TooManyRedirectsError
	switch {
	case errors.As(err, &methodErr):
		exitCode = 3
	case errors.As(err, &redirectErr):
		exitCode = 47
	case errors.As(err, &redirectLimitErr):
		exitCode = 47
	case errors.Is(err, context.DeadlineExceeded):
		exitCode = 28
	case strings.Contains(strings.ToLower(err.Error()), "aborted"):
		exitCode = 28
	case network.IsDenied(err):
		exitCode = 7
	}

	if !opts.silent || opts.showError {
		return exitf(inv, exitCode, "curl: (%d) %s", exitCode, curlErrorMessage(err))
	}
	return &ExitError{Code: exitCode}
}

func curlErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	var redirectErr *network.RedirectNotAllowedError
	switch {
	case errors.As(err, &redirectErr):
		return "Redirect target not in allow-list: " + redirectErr.URL
	case errors.Is(err, context.DeadlineExceeded):
		return "The operation was aborted"
	case strings.Contains(strings.ToLower(err.Error()), "aborted"):
		return err.Error()
	}
	message := err.Error()
	if message == "" {
		return ""
	}
	return strings.ToUpper(message[:1]) + message[1:]
}

var _ Command = (*Curl)(nil)
var _ SpecProvider = (*Curl)(nil)
var _ ParsedRunner = (*Curl)(nil)
var _ ParseInvocationNormalizer = (*Curl)(nil)
