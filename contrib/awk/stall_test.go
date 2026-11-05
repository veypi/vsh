package awk

import (
	"context"
	"strings"
	"testing"
	"time"
)

// 回归（生产事故 2026-10-31）：ctx 取消必须能打断 awk 解释器。此前 contrib
// 调 interp.ExecProgram（不带 ctx），bg kill 回报 "killed"，解释器 goroutine
// 却一直占满一个核（累计 119 CPU 小时、持续 5 天）。
func TestAWKStopsOnContextCancel(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		time.Sleep(300 * time.Millisecond)
		cancel()
	}()

	done := make(chan awkCommandResult, 1)
	go func() {
		done <- runAWKCommand(t, &awkCommandOptions{
			Ctx:  ctx,
			Args: []string{"BEGIN{while(1){}}"},
		})
	}()

	select {
	case result := <-done:
		if result.ExitCode == 0 {
			t.Fatalf("ExitCode = 0, want non-zero; stderr=%q", result.Stderr)
		}
		if !strings.Contains(result.Stderr, "context canceled") {
			t.Fatalf("stderr = %q, want %q", result.Stderr, "context canceled")
		}
	case <-time.After(15 * time.Second):
		t.Fatal("awk did not stop after context cancel")
	}
}

// 回归：无人取消时，无进展（典型无限循环特征）必须自限并报明确错误——
// 不能出现「脚本无限跑、平台无任何兜底」。
func TestAWKStallDetectionAbortsInfiniteLoop(t *testing.T) {
	t.Parallel()

	result := runAWKCommand(t, &awkCommandOptions{
		Args: []string{"BEGIN{while(1){}}"},
		Env:  map[string]string{stallLimitEnv: "1"},
	})
	if result.ExitCode == 0 {
		t.Fatalf("ExitCode = 0, want non-zero; stderr=%q", result.Stderr)
	}
	if !strings.Contains(result.Stderr, "stalled") {
		t.Fatalf("stderr = %q, want %q", result.Stderr, "stalled")
	}
	if !strings.Contains(result.Stderr, "possible infinite loop") {
		t.Fatalf("stderr = %q, want %q", result.Stderr, "possible infinite loop")
	}
}

// 正常脚本不受影响：持续读入/输出即有进展，不触发停滞。
func TestAWKStallDetectionKeepsRunningPrograms(t *testing.T) {
	t.Parallel()

	result := runAWKCommand(t, &awkCommandOptions{
		Args:  []string{"{print $1}"},
		Stdin: "a\nb\nc\n",
		Env:   map[string]string{stallLimitEnv: "1"},
	})
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, "a\nb\nc\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}

// 停滞检测可关闭（VSH_AWK_MAX_STALL=0）——需要长跑的用户自行承担。
func TestAWKStallDetectionCanBeDisabled(t *testing.T) {
	t.Parallel()

	if got := stallLimit(map[string]string{stallLimitEnv: "0"}); got != 0 {
		t.Fatalf("stallLimit(0) = %s, want 0", got)
	}
	if got := stallLimit(map[string]string{stallLimitEnv: "30"}); got != 30*time.Second {
		t.Fatalf("stallLimit(30) = %s, want 30s", got)
	}
	if got := stallLimit(nil); got != defaultStallLimit {
		t.Fatalf("stallLimit(nil) = %s, want %s", got, defaultStallLimit)
	}
}
