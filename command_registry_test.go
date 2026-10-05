package vsh

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"strings"
	"testing"

	"github.com/veypi/vsh/commands"
	gbfs "github.com/veypi/vsh/fs"
	"github.com/veypi/vsh/policy"
)

func TestRegistryDiscoveryWithoutFiles(t *testing.T) {
	ctx := context.Background()
	registry := registryWithCommands(t, commands.DefineCommand("probe", func(_ context.Context, inv *commands.Invocation) error {
		_, err := io.WriteString(inv.Stdout, "ok\n")
		return err
	}))
	session := newSession(t, &Config{Registry: registry})
	result := mustExecSession(t, session, `PATH=/missing
probe
command -v probe
type -t probe
which probe
hash probe
type -p probe
type -P probe || true`)
	if result.ExitCode != 0 || result.Stdout != "ok\nprobe\nregistered\nprobe\n" {
		t.Fatalf("result=%+v", result)
	}
	for _, p := range []string{"/bin", "/usr/bin", "/missing"} {
		if _, err := session.FileSystem().Stat(ctx, p); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("unexpected generated path %s: %v", p, err)
		}
	}
	registry.Unregister("probe")
	if result := mustExecSession(t, session, "probe"); result.ExitCode != 127 {
		t.Fatalf("removed=%+v", result)
	}
}

func TestCustomFactoryHasNoLayoutSideEffects(t *testing.T) {
	mem := gbfs.NewMemory()
	ctx := context.Background()
	if err := mem.MkdirAll(ctx, "/work", 0755); err != nil {
		t.Fatal(err)
	}
	session := newSession(t, &Config{BaseEnv: map[string]string{"HOME": "/absent/home", "PATH": "/absent/bin"}, FileSystem: CustomFileSystem(gbfs.FactoryFunc(func(context.Context) (gbfs.FileSystem, error) { return mem, nil }), "/work")})
	result := mustExecSession(t, session, "echo hi; exit 3")
	if result.Stdout != "hi\n" || result.ExitCode != 3 {
		t.Fatalf("result=%+v", result)
	}
	if _, err := mem.Stat(ctx, "/absent"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("layout write: %v", err)
	}
	if _, err := session.Exec(ctx, &ExecutionRequest{WorkDir: "/missing", Script: "true"}); err == nil {
		t.Fatal("missing cwd accepted")
	}
}

func TestExplicitPathAndNativeBoundary(t *testing.T) {
	calls := 0
	session := newSession(t, &Config{Policy: policy.NewStatic(&policy.Config{ReadRoots: []string{"/"}, WriteRoots: []string{"/"}}), NativeExec: func(_ context.Context, p string, inv *commands.Invocation) error {
		calls++
		if p != "/opt/probe" {
			t.Fatalf("path=%q", p)
		}
		_, err := io.WriteString(inv.Stdout, strings.Join(inv.Args, "|"))
		return err
	}})
	if err := session.FileSystem().MkdirAll(context.Background(), "/opt", 0755); err != nil {
		t.Fatal(err)
	}
	writeSessionFile(t, session, "/opt/probe", []byte("\x7fELF\x00binary"))
	if err := session.FileSystem().Chmod(context.Background(), "/opt/probe", 0755); err != nil {
		t.Fatal(err)
	}
	r := mustExecSession(t, session, "/opt/probe 'a b' ''")
	if r.ExitCode != 0 || r.Stdout != "a b|" || calls != 1 {
		t.Fatalf("result=%+v calls=%d", r, calls)
	}
	for _, script := range []string{"/bin/echo hi", "unknown-command"} {
		r = mustExecSession(t, session, script)
		if r.ExitCode != 127 || calls != 1 {
			t.Fatalf("result=%+v calls=%d", r, calls)
		}
	}
	r = mustExecSession(t, session, "/opt")
	if r.ExitCode != 126 {
		t.Fatalf("directory=%+v", r)
	}
}

func TestArgv0DoesNotChangeCommandIdentity(t *testing.T) {
	var got string
	registry := registryWithCommands(t, commands.DefineCommand("probe", func(_ context.Context, inv *commands.Invocation) error {
		if inv.Argv0 != nil {
			got = *inv.Argv0
		}
		return nil
	}))
	s := newSession(t, &Config{Registry: registry})
	result := mustExecSession(t, s, "env -a imaginary probe\nenv -a named bash -c 'printf \"<%s>\\n\" \"$0\"'\nenv -a '' bash -c 'printf \"<%s>\\n\" \"$0\"'\nenv -a other bash -c 'printf \"<%s>\\n\" \"$0\"' bash\n")
	if result.ExitCode != 0 || result.Stdout != "<named>\n<>\n<bash>\n" || got != "imaginary" {
		t.Fatalf("result=%+v argv0=%q", result, got)
	}
}

func TestExplicitShebangCannotSelectRegisteredBasename(t *testing.T) {
	calls := 0
	registry := registryWithCommands(t, commands.DefineCommand("probe", func(context.Context, *commands.Invocation) error { calls++; return nil }))
	s := newSession(t, &Config{Registry: registry})
	writeSessionFile(t, s, "/home/agent/script", []byte("#!/absent/probe\n"))
	if err := s.FileSystem().Chmod(context.Background(), "/home/agent/script", 0755); err != nil {
		t.Fatal(err)
	}
	result := mustExecSession(t, s, "./script")
	if result.ExitCode != 126 || calls != 0 {
		t.Fatalf("result=%+v calls=%d", result, calls)
	}
}
