package vsh

import (
	"context"
	"testing"

	"github.com/veypi/vsh/policy"
)

func TestWorkDirUpdatesPWD(t *testing.T) {
	t.Parallel()
	rt := newRuntime(t, &Config{})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		WorkDir: "/tmp",
		Script:  "echo \"$PWD\"\npwd\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0", result.ExitCode)
	}
	if got, want := result.Stdout, "/tmp\n/tmp\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}

func TestInvalidVisiblePWDDoesNotOverrideSandboxWorkDir(t *testing.T) {
	t.Parallel()
	rt := newRuntime(t, &Config{})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		WorkDir: "/tmp",
		Env: map[string]string{
			"PWD": "/private/tmp/host-only-path",
		},
		Script: "echo \"$PWD\"\npwd -L\npwd -P\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, "/tmp\n/tmp\n/tmp\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}

func TestCommandsResolveAgainstInternalPWDWhenVisiblePWDCorrupted(t *testing.T) {
	t.Parallel()
	rt := newRuntime(t, &Config{})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "" +
			"mkdir dir\n" +
			"cd dir\n" +
			"mkdir target\n" +
			"PWD=/tmp/host-physical\n" +
			"chmod -R 700 target\n" +
			"stat -c '%a' /home/agent/dir/target\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stdout=%q stderr=%q", result.ExitCode, result.Stdout, result.Stderr)
	}
	if got, want := result.Stdout, "700\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}

func TestRelativePathsUseVirtualWorkDir(t *testing.T) {
	t.Parallel()
	rt := newRuntime(t, &Config{})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "echo hi > note.txt\ncat note.txt\npwd\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0", result.ExitCode)
	}
	if got, want := result.Stdout, "hi\n/home/agent\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}

func TestVirtualCDUpdatesPWD(t *testing.T) {
	t.Parallel()
	rt := newRuntime(t, &Config{})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "pwd\ncd /tmp\npwd\ncd \"$HOME\"\npwd\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0", result.ExitCode)
	}
	if got, want := result.Stdout, "/home/agent\n/tmp\n/home/agent\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}

func TestVirtualCDAcceptsEndOfOptionsMarker(t *testing.T) {
	t.Parallel()
	rt := newRuntime(t, &Config{})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "" +
			"mkdir -p -- /tmp/-dir\n" +
			"cd -- /tmp\n" +
			"pwd\n" +
			"cd -- -dir\n" +
			"pwd\n" +
			"cd /\n" +
			"cd --\n" +
			"pwd\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, "/tmp\n/tmp/-dir\n/home/agent\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
	if got := result.Stderr; got != "" {
		t.Fatalf("Stderr = %q, want empty", got)
	}
}

func TestDirectoryStackBuiltinsManageVirtualPWD(t *testing.T) {
	t.Parallel()
	rt := newRuntime(t, &Config{})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "" +
			"mkdir -p a b\n" +
			"pushd a >/dev/null\n" +
			"pushd ../b >/dev/null\n" +
			"dirs -v -l\n" +
			"pushd +1 >/dev/null\n" +
			"dirs -v -l\n" +
			"popd >/dev/null\n" +
			"dirs -v -l\n" +
			"cd /tmp\n" +
			"dirs -v -l\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}

	want := "" +
		" 0  /home/agent/b\n" +
		" 1  /home/agent/a\n" +
		" 2  /home/agent\n" +
		" 0  /home/agent/a\n" +
		" 1  /home/agent\n" +
		" 2  /home/agent/b\n" +
		" 0  /home/agent\n" +
		" 1  /home/agent/b\n" +
		" 0  /tmp\n" +
		" 1  /home/agent/b\n"
	if got := result.Stdout; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
	if got := result.Stderr; got != "" {
		t.Fatalf("Stderr = %q, want empty", got)
	}
}

func TestDirectoryStackBuiltinsResolveDeferredEntries(t *testing.T) {
	t.Parallel()
	rt := newRuntime(t, &Config{})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "" +
			"mkdir -p rel\n" +
			"pushd -n rel >/dev/null\n" +
			"dirs -v -l\n" +
			"dirs +1\n" +
			"pushd +1 >/dev/null\n" +
			"pwd\n" +
			"dirs -v -l\n" +
			"popd -n +0 >/dev/null\n" +
			"dirs -v -l\n" +
			"pwd\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}

	want := "" +
		" 0  /home/agent\n" +
		" 1  rel\n" +
		"rel\n" +
		"/home/agent/rel\n" +
		" 0  /home/agent/rel\n" +
		" 1  /home/agent\n" +
		" 0  /home/agent/rel\n" +
		"/home/agent/rel\n"
	if got := result.Stdout; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
	if got := result.Stderr; got != "" {
		t.Fatalf("Stderr = %q, want empty", got)
	}
}

func TestDirectoryStackBuiltinsReportErrors(t *testing.T) {
	t.Parallel()
	rt := newRuntime(t, &Config{})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "" +
			"pushd\n" +
			"popd\n" +
			"dirs +9\n" +
			"mkdir -p a\n" +
			"pushd a >/dev/null\n" +
			"pushd +9\n" +
			"popd +9\n" +
			"dirs +9\n" +
			"pushd /no/such/dir\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 1 {
		t.Fatalf("ExitCode = %d, want 1; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got := result.Stdout; got != "" {
		t.Fatalf("Stdout = %q, want empty", got)
	}

	wantStderr := "" +
		"pushd: no other directory\n" +
		"popd: directory stack empty\n" +
		"dirs: directory stack empty\n" +
		"pushd: +9: directory stack index out of range\n" +
		"popd: +9: directory stack index out of range\n" +
		"dirs: 9: directory stack index out of range\n" +
		"pushd: /no/such/dir: No such file or directory\n"
	if got := result.Stderr; got != wantStderr {
		t.Fatalf("Stderr = %q, want %q", got, wantStderr)
	}
}

func TestPwdHonorsLogicalAndPhysicalModes(t *testing.T) {
	t.Parallel()
	rt := newRuntime(t, &Config{
		Policy: policy.NewStatic(&policy.Config{
			ReadRoots:   []string{"/"},
			WriteRoots:  []string{"/"},
			SymlinkMode: policy.SymlinkFollow,
		}),
	})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "" +
			"mkdir -p a/b\n" +
			"ln -s a/b c\n" +
			"cd c\n" +
			"pwd -L\n" +
			"pwd -P\n" +
			"pwd\n" +
			"POSIXLY_CORRECT=1 pwd\n" +
			"PWD=\"$PWD/.\" pwd -L\n" +
			"PWD=bogus pwd -L\n" +
			"PWD=\"/home/agent\" pwd -L\n" +
			"PWD=\"/home/agent/a/../c\" pwd -L\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}

	want := "" +
		"/home/agent/c\n" +
		"/home/agent/a/b\n" +
		"/home/agent/c\n" +
		"/home/agent/c\n" +
		"/home/agent/c\n" +
		"/home/agent/c\n" +
		"/home/agent/c\n" +
		"/home/agent/c\n"
	if got := result.Stdout; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}
