package vsh

import (
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/veypi/vsh/commands"
	"github.com/veypi/vsh/policy"
)

type recordingExecPolicy struct {
	*policy.Static
	requests []policy.ExecRequest
}

func newRecordingExecPolicy() *recordingExecPolicy {
	return &recordingExecPolicy{Static: policy.NewStatic(nil)}
}

func (p *recordingExecPolicy) AllowExec(_ context.Context, req policy.ExecRequest) error {
	req.Argv = append([]string(nil), req.Argv...)
	p.requests = append(p.requests, req)
	return nil
}

func (p *recordingExecPolicy) requestNamed(name string) *policy.ExecRequest {
	for i := range p.requests {
		if p.requests[i].Name == name {
			return &p.requests[i]
		}
	}
	return nil
}

func TestExecPolicyUsesResolvedCommandIdentity(t *testing.T) {
	registry := registryWithCommands(t, commands.DefineCommand("probe", func(_ context.Context, inv *commands.Invocation) error {
		_, err := io.WriteString(inv.Stdout, "registered\n")
		return err
	}))
	resolver := commands.FileCommandResolverFunc(func(_ context.Context, req commands.FileCommandRequest) (commands.Command, error) {
		return commands.DefineCommand(req.Path, func(_ context.Context, inv *commands.Invocation) error {
			_, err := fmt.Fprintf(inv.Stdout, "file:%s\n", inv.Args[0])
			return err
		}), nil
	})
	pol := newRecordingExecPolicy()
	session := newSession(t, &Config{Registry: registry, Policy: pol, FileCommands: resolver})
	writeSessionFile(t, session, "/opt/probe", []byte("\x7fELF\x00binary"))
	if err := session.FileSystem().Chmod(context.Background(), "/opt/probe", 0o755); err != nil {
		t.Fatal(err)
	}

	result := mustExecSession(t, session, "probe\n/opt/probe arg\n")
	if result.ExitCode != 0 || result.Stdout != "registered\nfile:arg\n" {
		t.Fatalf("result=%+v", result)
	}
	bare := pol.requestNamed("probe")
	if bare == nil || bare.Source != "registry" || bare.Path != "" {
		t.Fatalf("bare request=%+v", bare)
	}
	explicit := pol.requestNamed("/opt/probe")
	if explicit == nil || explicit.Source != "path" || explicit.Path != "/opt/probe" {
		t.Fatalf("explicit request=%+v", explicit)
	}
	if len(explicit.Argv) != 2 || explicit.Argv[0] != "/opt/probe" || explicit.Argv[1] != "arg" {
		t.Fatalf("explicit argv=%q", explicit.Argv)
	}
	if explicit.WorkDir != defaultHomeDir {
		t.Fatalf("explicit workdir=%q, want %q", explicit.WorkDir, defaultHomeDir)
	}
}

func TestExecPolicyDenialSkipsCommandImplementations(t *testing.T) {
	denyAll := func() policy.Policy {
		return policy.NewStatic(&policy.Config{AllowedCommands: []string{"__deny_all__"}})
	}

	t.Run("registered", func(t *testing.T) {
		runs := 0
		registry := registryWithCommands(t, commands.DefineCommand("probe", func(context.Context, *commands.Invocation) error {
			runs++
			return nil
		}))
		session := newSession(t, &Config{Registry: registry, Policy: denyAll()})
		result := mustExecSession(t, session, "probe\n")
		if result.ExitCode != 126 || runs != 0 {
			t.Fatalf("result=%+v runs=%d", result, runs)
		}
	})

	t.Run("lazy", func(t *testing.T) {
		loads := 0
		runs := 0
		registry := registryWithCommands(t)
		if err := registry.RegisterLazy("lazy-probe", func() (commands.Command, error) {
			loads++
			return commands.DefineCommand("lazy-probe", func(context.Context, *commands.Invocation) error {
				runs++
				return nil
			}), nil
		}); err != nil {
			t.Fatal(err)
		}
		session := newSession(t, &Config{Registry: registry, Policy: denyAll()})
		result := mustExecSession(t, session, "lazy-probe\n")
		if result.ExitCode != 126 || loads != 0 || runs != 0 {
			t.Fatalf("result=%+v loads=%d runs=%d", result, loads, runs)
		}
	})

	t.Run("file command", func(t *testing.T) {
		resolves := 0
		runs := 0
		resolver := commands.FileCommandResolverFunc(func(_ context.Context, req commands.FileCommandRequest) (commands.Command, error) {
			resolves++
			return commands.DefineCommand("file-probe", func(context.Context, *commands.Invocation) error {
				runs++
				return nil
			}), nil
		})
		session := newSession(t, &Config{Policy: denyAll(), FileCommands: resolver})
		writeSessionFile(t, session, "/opt/probe", []byte("\x7fELF\x00binary"))
		if err := session.FileSystem().Chmod(context.Background(), "/opt/probe", 0o755); err != nil {
			t.Fatal(err)
		}
		result := mustExecSession(t, session, "/opt/probe\n")
		if result.ExitCode != 126 || resolves != 1 || runs != 0 {
			t.Fatalf("result=%+v resolves=%d runs=%d", result, resolves, runs)
		}
	})

	t.Run("shell builtin", func(t *testing.T) {
		runs := 0
		registry := registryWithCommands(t, commands.DefineCommand("probe", func(context.Context, *commands.Invocation) error {
			runs++
			return nil
		}))
		session := newSession(t, &Config{Registry: registry, Policy: denyAll()})
		result := mustExecSession(t, session, "eval probe\n")
		if result.ExitCode != 126 || runs != 0 {
			t.Fatalf("result=%+v runs=%d", result, runs)
		}
	})
}
