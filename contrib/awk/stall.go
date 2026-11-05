package awk

// 取消与停滞检测（2026-11-05 增补）。
//
// 事故背景：生产上一个用户 awk 脚本的 while 循环索引写错（n=n+n0，n0 未
// 定义=0），对着 3.8MB 单行 XML 无限循环；该 exec 以 background 方式运行，
// `bg kill` 回报 "killed"，但解释器 goroutine 一直占满一个核（累计 119
// CPU 小时、持续 5 天）。两个独立缺陷：
//
//  1. 解释器不接 ctx：contrib 之前调 interp.ExecProgram（无 ctx），bg kill
//     / exec 超时 / 断连的取消信号到不了 VM，取消形同虚设。
//  2. 无人取消时无自限：脚本可以无限跑下去，平台没有任何兜底。
//
// 这里补第 2 道闸（停滞检测）：输入无新字节、输出无新字节持续超过阈值
// （默认 120s，可用 VSH_AWK_MAX_STALL 覆盖，0=关闭）即判定"无进展"——
// 无限循环的典型特征——主动取消并报明确错误。第 1 道闸在 awk.go（改走
// interp.ExecuteContext）。

import (
	"context"
	"errors"
	"io"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

const (
	// defaultStallLimit 是默认的"无进展"上限：读输入与写输出都没有进展
	// 超过此时长即判定无限循环并中止。
	defaultStallLimit = 120 * time.Second

	// maxStallCheckInterval 是看门狗检查间隔的上限（进展比对频率）。
	maxStallCheckInterval = 5 * time.Second

	// stallLimitEnv 覆盖无进展上限（秒）；0 = 关闭停滞检测。
	stallLimitEnv = "VSH_AWK_MAX_STALL"
)

// errAWKStall 是停滞检测触发的哨兵错误（经 context cause 传出）。
var errAWKStall = errors.New("stalled")

// progressCounter 统计输入读取与输出写入的字节数——停滞检测的进展信号。
type progressCounter struct {
	in  atomic.Int64
	out atomic.Int64
}

func (p *progressCounter) addIn(n int) {
	if p != nil && n > 0 {
		p.in.Add(int64(n))
	}
}

func (p *progressCounter) addOut(n int) {
	if p != nil && n > 0 {
		p.out.Add(int64(n))
	}
}

func (p *progressCounter) total() int64 {
	if p == nil {
		return 0
	}
	return p.in.Load() + p.out.Load()
}

// countingReader 统计从底层读取的字节数。
type countingReader struct {
	r io.Reader
	c *progressCounter
}

func (r *countingReader) Read(p []byte) (int, error) {
	n, err := r.r.Read(p)
	r.c.addIn(n)
	return n, err
}

// countingReadCloser 统计从底层文件读取的字节数。
type countingReadCloser struct {
	r io.ReadCloser
	c *progressCounter
}

func (r *countingReadCloser) Read(p []byte) (int, error) {
	n, err := r.r.Read(p)
	r.c.addIn(n)
	return n, err
}

func (r *countingReadCloser) Close() error { return r.r.Close() }

// countingWriter 统计写出的字节数，并透传 Flush（解释器收尾时会断言 flusher）。
type countingWriter struct {
	w io.Writer
	c *progressCounter
}

func (w *countingWriter) Write(p []byte) (int, error) {
	n, err := w.w.Write(p)
	w.c.addOut(n)
	return n, err
}

func (w *countingWriter) Flush() {
	if f, ok := w.w.(interface{ Flush() error }); ok {
		_ = f.Flush()
	}
}

// countingWriteCloser 统计写入文件的字节数（print > "file" 同样是进展）。
type countingWriteCloser struct {
	w io.WriteCloser
	c *progressCounter
}

func (w *countingWriteCloser) Write(p []byte) (int, error) {
	n, err := w.w.Write(p)
	w.c.addOut(n)
	return n, err
}

func (w *countingWriteCloser) Close() error { return w.w.Close() }

// stallLimit 读取"无进展"上限：VSH_AWK_MAX_STALL（秒）覆盖默认值；0 = 关闭。
func stallLimit(env map[string]string) time.Duration {
	if env != nil {
		if raw, ok := env[stallLimitEnv]; ok {
			if n, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil {
				if n <= 0 {
					return 0
				}
				return time.Duration(n) * time.Second
			}
		}
	}
	return defaultStallLimit
}

// watchStall 是停滞看门狗：无进展超过 limit 即以 errAWKStall 取消执行 ctx
// （解释器每 1000 条指令查一次 ctx → 及时中止）。stop 关闭即退出。
func watchStall(ctx context.Context, cancel context.CancelCauseFunc, prog *progressCounter, limit time.Duration, stop <-chan struct{}) {
	if limit <= 0 {
		return
	}
	ticker := time.NewTicker(stallCheckInterval(limit))
	defer ticker.Stop()
	last := prog.total()
	lastChange := time.Now()
	for {
		select {
		case <-stop:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			if cur := prog.total(); cur != last {
				last = cur
				lastChange = time.Now()
				continue
			}
			if time.Since(lastChange) >= limit {
				cancel(errAWKStall)
				return
			}
		}
	}
}

// stallCheckInterval 按上限推导检查间隔：上限越小检查越勤（保证小阈值也能
// 及时触发），最大不超过 maxStallCheckInterval。
func stallCheckInterval(limit time.Duration) time.Duration {
	interval := limit / 4
	if interval > maxStallCheckInterval {
		interval = maxStallCheckInterval
	}
	if interval < 100*time.Millisecond {
		interval = 100 * time.Millisecond
	}
	return interval
}
