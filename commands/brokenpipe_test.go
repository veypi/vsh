package commands

import (
	"errors"
	"fmt"
	"io"
	"syscall"
	"testing"
)

// 下游提前关闭是可预期的正常终止（SIGPIPE），实现必须能把它与真实故障区分：
// 断管要按 ExitError{141} 收尾，裸 error 会被解释器当作致命中止。
func TestBrokenPipe(t *testing.T) {
	t.Parallel()
	for _, err := range []error{
		io.ErrClosedPipe,
		syscall.EPIPE,
		fmt.Errorf("write stdout: %w", io.ErrClosedPipe),
		errors.New("write |1: broken pipe"),
		errors.New("io: read/write on closed pipe"),
	} {
		if !BrokenPipe(err) {
			t.Fatalf("%v not detected as broken pipe", err)
		}
	}
	for _, err := range []error{
		nil,
		io.EOF,
		errors.New("connection reset by peer"),
		errors.New("ssh: pipe closed by remote"),
	} {
		if BrokenPipe(err) {
			t.Fatalf("%v misdetected as broken pipe", err)
		}
	}
}
