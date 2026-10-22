package interp

// brokenpipe_test.go 钉住「断管不是致命错误」这条契约。
//
// 场景（2026-10-22 实测）：命令往 stdout 写数据时下游已经没了（管道右端命令
// 不存在 → 读端关闭），写端拿到 EPIPE。命令若把裸 error 返回上来，解释器会
// 当成致命错误把整段脚本打断，还把真正的失败原因（右端命令不存在）盖住。
// 正确收尾：按 shell 惯例以 141 结束该命令，脚本继续跑。

import (
	"errors"
	"io"
	"testing"
)

func TestFromHandlerErrorBrokenPipeIsNotFatal(t *testing.T) {
	t.Parallel()
	for _, err := range []error{
		io.ErrClosedPipe,
		errors.New("io: read/write on closed pipe"),
		errors.New("write stdout: broken pipe"),
	} {
		var e exitStatus
		e.fromHandlerError(err)
		if e.fatalExit {
			t.Fatalf("%v 被当成致命错误（会把整段脚本打断）", err)
		}
		if e.code != 141 {
			t.Fatalf("%v 的退出码 = %d, want 141", err, e.code)
		}
		if e.err != nil {
			t.Fatalf("%v 不该留下错误（它会盖住真正的失败原因）：%v", err, e.err)
		}
	}
}

// 真实故障仍然是致命错误——不能因为上面那条兜底就把所有错误都放过去。
func TestFromHandlerErrorRealFailureStaysFatal(t *testing.T) {
	t.Parallel()
	var e exitStatus
	e.fromHandlerError(errors.New("boom"))
	if !e.fatalExit {
		t.Fatal("真实故障应当仍是致命错误")
	}
	if e.err == nil {
		t.Fatal("致命错误必须带着 err")
	}
}
