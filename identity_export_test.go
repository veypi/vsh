package vsh

import (
	"context"
	"strings"
	"testing"
)

// 身份键必须出现在「命令可见环境」里：内建命令读的是命令可见环境
//（commands.NewInvocation 的 Env ← invokeResolvedCommand 的 currentEnv）。
// UID/EUID 缺失时 vsh 回落默认虚拟身份（1000）——在真实属主不是 1000 的设备上
//（macOS 桌面用户 uid=501），按 POSIX 权限档判定的内建会落 group 档而不是属主档，
// 把属主可写的目录误判为不可写（0755 目录里内建 mv 报 Permission denied，原生
// /bin/mv 正常）。
func TestIdentityKeysReachCommandEnv(t *testing.T) {
	rt, err := New()
	if err != nil {
		t.Fatalf("New error = %v", err)
	}
	res, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "id\n",
		Env:    map[string]string{"UID": "501", "EUID": "501", "GID": "20", "EGID": "20"},
	})
	if err != nil {
		t.Fatalf("Run error = %v", err)
	}
	if !strings.Contains(res.Stdout, "uid=501") {
		t.Fatalf("id stdout = %q, want uid=501（可信身份必须对内建可见）", res.Stdout)
	}
	if !strings.Contains(res.Stdout, "euid=501") && !strings.Contains(res.Stdout, "uid=501") {
		t.Fatalf("id stdout = %q, want euid=501", res.Stdout)
	}
}

// 可信身份不可被脚本改写：UID/EUID 是只读变量，导出也拒绝。
func TestIdentityKeysNotSpoofable(t *testing.T) {
	rt, err := New()
	if err != nil {
		t.Fatalf("New error = %v", err)
	}
	res, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "export UID=999; id\n",
		Env:    map[string]string{"UID": "501", "EUID": "501"},
	})
	if err != nil {
		// 拒绝改写时报错退出同样算通过（身份没被改）。
		if !strings.Contains(err.Error(), "readonly") && !strings.Contains(res.Stderr, "readonly") {
			t.Fatalf("Run error = %v (stderr=%q), want readonly rejection", err, res.Stderr)
		}
		return
	}
	if strings.Contains(res.Stdout, "uid=999") {
		t.Fatalf("id stdout = %q, want uid=501（脚本不得改写可信身份）", res.Stdout)
	}
	if !strings.Contains(res.Stdout, "uid=501") {
		t.Fatalf("id stdout = %q, want uid=501", res.Stdout)
	}
}
