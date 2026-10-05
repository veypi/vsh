package vsh

import (
	"context"
	"fmt"
	"io"
	"maps"
	"strings"
	"time"

	gbfs "github.com/veypi/vsh/fs"
	"github.com/veypi/vsh/policy"
)

// Runtime executes bash-like scripts inside the configured sandbox.
//
// Use [New] to construct a runtime, [Runtime.Run] for one-shot execution, and
// [Runtime.NewSession] when you want multiple executions to share the same
// sandbox filesystem state.
type Runtime struct {
	cfg            Config
	sessionFactory gbfs.Factory
}

// NewSession creates a new persistent session backed by the runtime's
// configured filesystem factory and sandbox policy.
//
// Each session gets its own filesystem state. Repeated calls create isolated
// sessions, while repeated calls to [Session.Exec] on the same session share the
// same sandbox filesystem.
func (r *Runtime) NewSession(ctx context.Context) (*Session, error) {
	if r == nil {
		return nil, fmt.Errorf("vsh: runtime is nil")
	}
	ctx = withHostProcessGroup(ctx)
	return r.newSession(ctx)
}

// Run executes a script in a fresh session and returns the result.
//
// Use [Runtime.NewSession] when you want filesystem state to persist across
// multiple executions.
func (r *Runtime) Run(ctx context.Context, req *ExecutionRequest) (*ExecutionResult, error) {
	if r == nil {
		return nil, fmt.Errorf("vsh: runtime is nil")
	}
	ctx = withHostProcessGroup(ctx)
	session, err := r.newSession(ctx)
	if err != nil {
		return nil, err
	}
	return session.Exec(ctx, req)
}

func (r *Runtime) newSession(ctx context.Context) (*Session, error) {
	fsys, err := r.sessionFactory.New(ctx)
	if err != nil {
		return nil, err
	}
	fsys = wrapSandboxFileSystem(fsys)

	now := time.Now()
	return &Session{
		cfg:         r.cfg,
		id:          nextTraceID("sess"),
		fs:          fsys,
		bootAt:      now.UTC(),
		currentTime: now.UTC(),
		clockRealAt: now,
	}, nil
}

type limitOverridePolicy struct {
	base   policy.Policy
	limits policy.Limits
}

func (p limitOverridePolicy) AllowCommand(ctx context.Context, name string, argv []string) error {
	return p.base.AllowCommand(ctx, name, argv)
}

func (p limitOverridePolicy) AllowBuiltin(ctx context.Context, name string, argv []string) error {
	return p.base.AllowBuiltin(ctx, name, argv)
}

func (p limitOverridePolicy) AllowPath(ctx context.Context, action policy.FileAction, target string) error {
	return p.base.AllowPath(ctx, action, target)
}

func (p limitOverridePolicy) Limits() policy.Limits {
	return p.limits
}

func (p limitOverridePolicy) SymlinkMode() policy.SymlinkMode {
	return p.base.SymlinkMode()
}

func mergeLimits(base, overrides policy.Limits) policy.Limits {
	if overrides.MaxCommandCount != 0 {
		base.MaxCommandCount = overrides.MaxCommandCount
	}
	if overrides.MaxGlobOperations != 0 {
		base.MaxGlobOperations = overrides.MaxGlobOperations
	}
	if overrides.MaxLoopIterations != 0 {
		base.MaxLoopIterations = overrides.MaxLoopIterations
	}
	if overrides.MaxSubstitutionDepth != 0 {
		base.MaxSubstitutionDepth = overrides.MaxSubstitutionDepth
	}
	if overrides.MaxStdoutBytes != 0 {
		base.MaxStdoutBytes = overrides.MaxStdoutBytes
	}
	if overrides.MaxStderrBytes != 0 {
		base.MaxStderrBytes = overrides.MaxStderrBytes
	}
	if overrides.MaxFileBytes != 0 {
		base.MaxFileBytes = overrides.MaxFileBytes
	}
	return base
}

func defaultName(name string) string {
	if name == "" {
		return "stdin"
	}
	return name
}

func mergeEnv(base, override map[string]string) map[string]string {
	out := make(map[string]string, len(base)+len(override))
	maps.Copy(out, base)
	maps.Copy(out, override)
	return out
}

func stdinOrEmpty(reader io.Reader) io.Reader {
	if reader == nil {
		return strings.NewReader("")
	}
	return reader
}
