// Copyright (c) 2017, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package interp

import (
	"context"
	"fmt"
	"github.com/veypi/vsh/commands"
	"github.com/veypi/vsh/internal/commandutil"
	"io"
	"io/fs"
	"os"
	"path"

	"github.com/veypi/vsh/internal/completionutil"
	"github.com/veypi/vsh/shell/expand"
	"github.com/veypi/vsh/shell/syntax"
)

// LookupHandlerContext returns the [HandlerContext] value stored in ctx,
// which is used when calling handler functions.
func LookupHandlerContext(ctx context.Context) (*HandlerContext, bool) {
	hc, ok := ctx.Value(handlerCtxKey{}).(*HandlerContext)
	return hc, ok
}

func InheritNestedShellState(ctx context.Context, cfg *RunnerConfig) {
	if cfg == nil {
		return
	}
	hc, ok := LookupHandlerContext(ctx)
	if !ok || hc == nil || hc.runner == nil {
		return
	}
	hc.runner.ensureFDTable()
	cfg.inheritedFDs = forkFDTableForExec(hc.runner.fds)
}

func mustHandlerCtx(ctx context.Context) *HandlerContext {
	hc, ok := LookupHandlerContext(ctx)
	if !ok {
		panic("interp: missing HandlerContext in context")
	}
	return hc
}

type handlerCtxKey struct{}
type disableCommandHashKey struct{}

type handlerKind int

const (
	_                    handlerKind = iota
	handlerKindExec                  // [ExecHandlerFunc]
	handlerKindCall                  // [CallHandlerFunc]
	handlerKindOpen                  // [OpenHandlerFunc]
	handlerKindReadDir               // [ReadDirHandlerFunc]
	handlerKindRealpath              // [RealpathHandlerFunc]
	handlerKindProcSubst             // [ProcSubstHandlerFunc]
)

// HandlerContext is the data passed to all the handler functions via [context.WithValue].
// It contains some of the current state of the [Runner].
type HandlerContext struct {
	runner *Runner // for internal use only, e.g. shell state mutation helpers

	// kind records which type of handler this context was built for.
	kind handlerKind

	// Env is a read-only version of the interpreter's environment,
	// including environment variables, global variables, and local function
	// variables.
	Env expand.Environ

	// Dir is the interpreter's current directory.
	Dir string

	// VisibleDir is the shell-visible current directory, preserving logical
	// symlink components after `cd` when applicable.
	VisibleDir string

	// ExecFile is the file currently executing in the runner's active frame stack.
	// It is empty for inline code that is not executing from a file-backed context.
	ExecFile string

	// Internal reports whether this handler invocation is executing trusted
	// interpreter bootstrap code rather than user-supplied shell code.
	Internal bool

	// DisableCommandHash bypasses the runner's cached command lookup table for
	// this handler context without clearing the user-visible hash state. Fresh
	// resolutions still update the table.
	DisableCommandHash bool

	// Pos is the source position which relates to the operation,
	// such as a [syntax.CallExpr] when calling an [ExecHandlerFunc].
	// It may be invalid if the operation has no relevant position information.
	Pos syntax.Pos

	// Stdin is the interpreter's current standard input reader.
	Stdin StdinReader
	// Stdout is the interpreter's current standard output writer.
	Stdout io.Writer
	// Stderr is the interpreter's current standard error writer.
	Stderr io.Writer
}

func (hc *HandlerContext) LookupCommandHash(name string) (string, bool) {
	if hc == nil || hc.runner == nil || hc.DisableCommandHash {
		return "", false
	}
	entry, ok := hc.runner.commandHashLookup(name)
	if !ok {
		return "", false
	}
	return entry.path, true
}

func (hc *HandlerContext) RememberCommandHash(name, path string) {
	if hc == nil || hc.runner == nil {
		return
	}
	hc.runner.commandHashRemember(name, path)
}

func (hc *HandlerContext) IncrementCommandHash(name string) {
	if hc == nil || hc.runner == nil {
		return
	}
	hc.runner.commandHashIncrement(name)
}

func (hc *HandlerContext) ClearCommandHash() {
	if hc == nil || hc.runner == nil {
		return
	}
	hc.runner.commandHashClear()
}

func (hc *HandlerContext) IsBuiltin(name string) bool {
	if hc == nil || hc.runner == nil {
		return IsBuiltin(name)
	}
	return hc.runner.isBuiltinActive(name)
}

func (hc *HandlerContext) IsBuiltinDisabled(name string) bool {
	if hc == nil || hc.runner == nil {
		return false
	}
	return hc.runner.isBuiltinDisabled(name)
}

func (hc *HandlerContext) EnabledBuiltinNames(prefix string) []string {
	if hc == nil || hc.runner == nil {
		return completionutil.BuiltinNames(prefix)
	}
	return hc.runner.enabledBuiltinNames(prefix)
}

func withDisabledCommandHash(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, disableCommandHashKey{}, true)
}

func commandHashDisabled(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	disabled, _ := ctx.Value(disableCommandHashKey{}).(bool)
	return disabled
}

// CallHandlerFunc is a handler which runs on every [syntax.CallExpr].
// It is called once variable assignments and field expansion have occurred.
// The context includes a [HandlerContext] value.
//
// The call's arguments are replaced by what the handler returns,
// and then the call is executed by the Runner as usual.
// The args slice is never empty.
// At this time, returning an empty slice without an error is not supported.
//
// This handler is similar to [ExecHandlerFunc], but has two major differences:
//
// First, it runs for all simple commands, including function calls and builtins.
//
// Second, it is not expected to execute the simple command, but instead to
// allow running custom code which allows replacing the argument list.
// Shell builtins touch on many internals of the Runner, after all.
//
// Returning a non-nil error will halt the [Runner] and will be returned via the API.
type CallHandlerFunc func(ctx context.Context, args []string) ([]string, error)

// TODO: consistently treat handler errors as non-fatal by default,
// but have an interface or API to specify fatal errors which should make
// the shell exit with a particular status code.

// ExecHandlerFunc is a handler which executes simple commands.
// It is called for all [syntax.CallExpr] nodes
// where the first argument is neither a declared function nor a builtin.
// The args slice is never empty.
// The context includes a [HandlerContext] value.
//
// Returning a nil error means a zero exit status.
// Other exit statuses can be set by returning an [ExitStatus] error,
// and such an error is returned via the API if it is the last statement executed.
// Any other error will halt the [Runner] and will be returned via the API.
type ExecHandlerFunc func(ctx context.Context, args []string) error

const defaultExecPath = "/usr/bin:/bin"

func closedExecHandler() ExecHandlerFunc {
	return func(ctx context.Context, args []string) error {
		hc := mustHandlerCtx(ctx)
		path, err := hc.runner.lookPath(ctx, hc.Dir, hc.Env, args[0], true, false)
		if err != nil {
			fmt.Fprintln(hc.Stderr, err)
			if isLookupNotFound(err, args[0]) {
				return ExitStatus(127)
			}
			return ExitStatus(126)
		}
		fmt.Fprintf(hc.Stderr, "%s: command execution unavailable\n", path)
		return ExitStatus(126)
	}
}

func (r *Runner) commandLocations(ctx context.Context, name, cwd string, env expand.Environ, all, filesOnly bool) ([]commands.CommandLocation, error) {
	vars := map[string]string{}
	for name, variable := range env.Each() {
		if variable.IsSet() {
			vars[name] = variable.String()
		}
	}
	if r.lookupCommand != nil {
		return r.lookupCommand(ctx, commands.CommandLookupRequest{Name: name, Env: vars, WorkDir: cwd, All: all, FilesOnly: filesOnly})
	}
	paths, err := commandutil.CommandPaths(ctx, name, cwd, vars, r.platform, all, true, func(ctx context.Context, p string) (fs.FileInfo, error) { return r.statHandler(ctx, p, true) })
	var found []commands.CommandLocation
	for _, p := range paths {
		found = append(found, commands.CommandLocation{Name: path.Base(p), Path: p})
	}
	return found, err
}
func (r *Runner) lookPath(ctx context.Context, cwd string, env expand.Environ, file string, requireExec, useDefaultPath bool) (string, error) {
	if !requireExec {
		vars := map[string]string{}
		for k, v := range env.Each() {
			if v.IsSet() {
				vars[k] = v.String()
			}
		}
		paths, err := commandutil.CommandPaths(ctx, file, cwd, vars, r.platform, false, false, func(ctx context.Context, p string) (fs.FileInfo, error) { return r.statHandler(ctx, p, true) })
		if err != nil {
			return "", err
		}
		if len(paths) > 0 {
			return paths[0], nil
		}
		return "", fmt.Errorf("%q: executable file not found in $PATH", file)
	}
	if useDefaultPath {
		env = expand.ListEnviron("PATH=" + defaultExecPath)
	}
	found, err := r.commandLocations(ctx, file, cwd, env, false, true)
	if err == nil && len(found) > 0 {
		return found[0].Path, nil
	}
	if err != nil {
		return "", err
	}
	return "", fmt.Errorf("%q: executable file not found in $PATH", file)
}
func (r *Runner) lookPathForHash(ctx context.Context, cwd string, env expand.Environ, file string) (string, error) {
	found, err := r.commandLocations(ctx, file, cwd, env, false, false)
	if err == nil && len(found) > 0 {
		return found[0].Path, nil
	}
	if err != nil {
		return "", err
	}
	return "", fmt.Errorf("%q: executable file not found in $PATH", file)
}

func isLookupNotFound(err error, file string) bool {
	return err != nil && err.Error() == fmt.Sprintf("%q: executable file not found in $PATH", file)
}

// OpenHandlerFunc is a handler which opens files.
// It is called for all files that are opened directly by the shell,
// such as in redirects, except for named pipes created by process substitutions.
// The context includes a [HandlerContext] value.
// Files opened by executed programs are not included.
//
// The path parameter may be relative to the current directory,
// which is available via the [HandlerContext] stored in ctx.
//
// Use a return error of type [*os.PathError] to have the error printed to
// stderr and the exit status set to 1.
// Any other error will halt the [Runner] and will be returned via the API.
//
// Note that implementations which do not return [os.File] will cause
// extra files and goroutines for input redirections.
type OpenHandlerFunc func(ctx context.Context, path string, flag int, perm os.FileMode) (io.ReadWriteCloser, error)

// ProcSubstEndpoint describes the shell-visible path and I/O endpoints used to
// execute a process substitution.
//
// Path is the shell-visible path to expand the process substitution to.
// Reader is used as the subprocess stdin for `>(cmd)` substitutions.
// Writer is used as the subprocess stdout for `<(cmd)` substitutions.
// Cleanup is called after the subprocess I/O endpoints are closed.
type ProcSubstEndpoint struct {
	Path    string
	Reader  io.ReadCloser
	Writer  io.WriteCloser
	Cleanup func() error
}

// ProcSubstHandlerFunc provisions the shell-visible path and I/O endpoints used
// to execute a process substitution.
//
// The context includes a [HandlerContext] value.
type ProcSubstHandlerFunc func(ctx context.Context, ps *syntax.ProcSubst) (*ProcSubstEndpoint, error)

// TODO: paths passed to [OpenHandlerFunc] should be cleaned.

func closedOpenHandler() OpenHandlerFunc {
	return func(ctx context.Context, path string, flag int, perm os.FileMode) (io.ReadWriteCloser, error) {
		return nil, &os.PathError{Op: "open", Path: path, Err: fs.ErrNotExist}
	}
}

// ReadDirHandlerFunc is a handler which reads directories. It is called during
// shell globbing, if enabled.
// The context includes a [HandlerContext] value.
type ReadDirHandlerFunc func(ctx context.Context, path string) ([]fs.DirEntry, error)

func closedReadDirHandler() ReadDirHandlerFunc {
	return func(ctx context.Context, path string) ([]fs.DirEntry, error) {
		return nil, &os.PathError{Op: "readdir", Path: path, Err: fs.ErrNotExist}
	}
}

// StatHandlerFunc is a handler which gets a file's information.
// The context includes a [HandlerContext] value.
type StatHandlerFunc func(ctx context.Context, name string, followSymlinks bool) (fs.FileInfo, error)

func closedStatHandler() StatHandlerFunc {
	return func(ctx context.Context, path string, followSymlinks bool) (fs.FileInfo, error) {
		op := "stat"
		if !followSymlinks {
			op = "lstat"
		}
		return nil, &os.PathError{Op: op, Path: path, Err: fs.ErrNotExist}
	}
}

// RealpathHandlerFunc is a handler which resolves a physical path.
// The context includes a [HandlerContext] value.
type RealpathHandlerFunc func(ctx context.Context, name string) (string, error)

func defaultRealpathHandler() RealpathHandlerFunc {
	return func(ctx context.Context, name string) (string, error) {
		hc := mustHandlerCtx(ctx)
		return absPath(hc.Dir, name), nil
	}
}
