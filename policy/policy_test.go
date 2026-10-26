package policy

import (
	"context"
	"testing"
)

func TestNewStaticDefaultsToSymlinkDeny(t *testing.T) {
	t.Parallel()
	pol := NewStatic(nil)

	if got, want := pol.SymlinkMode(), SymlinkDeny; got != want {
		t.Fatalf("SymlinkMode() = %q, want %q", got, want)
	}
}

func TestNewStaticDefaultsToExecAllowOpen(t *testing.T) {
	t.Parallel()
	pol := NewStatic(nil)

	if err := pol.AllowExec(context.Background(), ExecRequest{Name: "eval", Source: "builtin", Argv: []string{"eval", "echo ok"}}); err != nil {
		t.Fatalf("AllowExec(eval) error = %v, want nil", err)
	}
}

func TestNewStaticExecAllowlistRejectsMissingCommand(t *testing.T) {
	t.Parallel()
	pol := NewStatic(&Config{AllowedCommands: []string{"cd"}})

	err := pol.AllowExec(context.Background(), ExecRequest{Name: "eval", Source: "builtin", Argv: []string{"eval", "echo ok"}})
	if err == nil {
		t.Fatal("AllowExec(eval) error = nil, want denial")
	}
	if !IsDenied(err) {
		t.Fatalf("AllowExec(eval) error = %v, want denied", err)
	}
}

func TestNewStaticExecAllowlistMatchesResolvedPath(t *testing.T) {
	t.Parallel()
	pol := NewStatic(&Config{AllowedCommands: []string{"/opt/probe"}})

	if err := pol.AllowExec(context.Background(), ExecRequest{Name: "probe", Path: "/opt/probe", Source: "path"}); err != nil {
		t.Fatalf("AllowExec(resolved path) error = %v, want nil", err)
	}
	if err := pol.AllowExec(context.Background(), ExecRequest{Name: "probe", Path: "/other/probe", Source: "path"}); !IsDenied(err) {
		t.Fatalf("AllowExec(other path) error = %v, want denied", err)
	}
}
