package commandutil

import (
	"context"
	"errors"
	"io/fs"
	"testing"
	"time"

	"github.com/veypi/vsh/host"
)

func TestWindowsCommandOperands(t *testing.T) {
	for in, want := range map[string]string{`C:\Tools\run.exe`: "/c/Tools/run.exe", "C:/Tools/run.exe": "/c/Tools/run.exe", `.\run`: "./run", "run": "run"} {
		if got := CommandPath(in, host.Platform{OS: host.OSWindows}); got != want {
			t.Fatalf("%q -> %q, want %q", in, got, want)
		}
		if got := CommandPath(in, host.Platform{OS: host.OSLinux}); got != in {
			t.Fatalf("POSIX operand changed: %q", got)
		}
	}
}

// PATH 搜索形态：探测被规则拒绝的目录按 miss 继续搜——后续目录命中正常；
// 全程无命中时返回 not found（不允许拒绝错误顶掉 command not found）。
func TestCommandPathsSearchDeniedDir(t *testing.T) {
	deniedErr := fs.ErrPermission
	existing := map[string]bool{"/later/bin/tool": true}
	stat := func(_ context.Context, p string) (fs.FileInfo, error) {
		if p == "/denied/bin/tool" || p == "/denied/bin/nope" {
			return nil, deniedErr
		}
		if existing[p] {
			return fakeFileInfo{}, nil
		}
		return nil, fs.ErrNotExist
	}
	env := map[string]string{"PATH": "/denied/bin:/later/bin"}
	platform := host.Platform{OS: host.OSLinux}

	// 被拒目录之后的目录命中：搜索不被中断。
	paths, err := CommandPaths(context.Background(), "tool", "/", env, platform, false, true, stat)
	if err != nil || len(paths) != 1 || paths[0] != "/later/bin/tool" {
		t.Fatalf("tool: paths=%v err=%v, want [/later/bin/tool] <nil>", paths, err)
	}

	// 全程无命中：not found，不报权限错误。
	paths, err = CommandPaths(context.Background(), "nope", "/", env, platform, false, true, stat)
	if err != nil || len(paths) != 0 {
		t.Fatalf("nope: paths=%v err=%v, want [] <nil>", paths, err)
	}
}

// 显式路径形态：探测被规则拒绝时错误上抛（调用方报权限错误）。
func TestCommandPathsExplicitDenied(t *testing.T) {
	stat := func(_ context.Context, p string) (fs.FileInfo, error) {
		return nil, fs.ErrPermission
	}
	_, err := CommandPaths(context.Background(), "/denied/bin/tool", "/", map[string]string{}, host.Platform{OS: host.OSLinux}, false, true, stat)
	if !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("err=%v, want ErrPermission", err)
	}
}

type fakeFileInfo struct{}

func (fakeFileInfo) Name() string      { return "x" }
func (fakeFileInfo) Size() int64       { return 0 }
func (fakeFileInfo) Mode() fs.FileMode { return 0o755 }
func (fakeFileInfo) ModTime() time.Time {
	return time.Time{}
}
func (fakeFileInfo) IsDir() bool      { return false }
func (fakeFileInfo) Sys() interface{} { return nil }
