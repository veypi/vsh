package vsh

// pipeline_brokenpipe_test.go 端到端钉住「断管不炸脚本」这条契约。
//
// 场景（2026-10-22 实测）：注册命令往管道写大量数据，下游提前退出
// （head -1）或右端命令不存在，写端拿到 EPIPE。命令若把裸 error 返回上来
// （漏了 commands.BrokenPipe → ExitError{141} 的映射），解释器必须兜底
// 按 141 收尾并让脚本继续跑，而不是当致命错误整段打断。

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/veypi/vsh/commands"
)

// definePipewriter 注册一个往 stdout 写 256KB 行数据（远超 64KB 管道缓冲）
// 的命令；写失败时故意返回裸 error（不做 BrokenPipe 映射），模拟
// 「作者漏映射」的真实命令，逼解释器走兜底路径。
func definePipewriter() commands.Command {
	return commands.DefineCommand("pipewriter", func(_ context.Context, inv *commands.Invocation) error {
		line := append(bytes.Repeat([]byte("x"), 63), '\n')
		for i := 0; i < 4096; i++ { // 4096 行 × 64B = 256KB
			if _, err := inv.Stdout.Write(line); err != nil {
				return err
			}
		}
		return nil
	})
}

// 下游提前退出：写端断管收成 141，脚本继续跑。
func TestPipelineBrokenPipeNotFatal(t *testing.T) {
	t.Parallel()
	session := newSession(t, &Config{Registry: registryWithCommands(t, definePipewriter())})

	result := mustExecSession(t, session, "pipewriter | head -1\necho alive:$?\n")
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	lines := strings.Split(strings.TrimRight(result.Stdout, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("stdout 行数 = %d, want 2; stdout=%q", len(lines), result.Stdout)
	}
	if got, want := lines[0], strings.Repeat("x", 63); got != want {
		t.Fatalf("head 行 = %q, want 63 个 x", got)
	}
	// 管道退出码取最右端（head 正常退出 0），左端的 141 不进 $?；
	// 第二行存在本身就证明脚本没被断管打断。
	if got := lines[1]; got != "alive:0" {
		t.Fatalf("末行 = %q, want alive:0（脚本必须活到管道之后）", got)
	}
	if strings.Contains(result.Stderr, "pipe") {
		t.Fatalf("stderr 不该有断管噪音：%q", result.Stderr)
	}
}

// 右端命令不存在：左端断管不能把「command not found（127）」这个真错盖住，
// 脚本同样继续跑。
func TestPipelineBrokenPipeMissingRightSide(t *testing.T) {
	t.Parallel()
	session := newSession(t, &Config{Registry: registryWithCommands(t, definePipewriter())})

	result := mustExecSession(t, session, "pipewriter | definitely_missing_cmd_xyz\necho after:$?\n")
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	lines := strings.Split(strings.TrimRight(result.Stdout, "\n"), "\n")
	if got := lines[len(lines)-1]; got != "after:127" {
		t.Fatalf("末行 = %q, want after:127（右端 127 才是真错，不能被断管盖住）; stdout=%q", got, result.Stdout)
	}
}
