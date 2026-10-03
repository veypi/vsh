package builtins

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"
)

func executeCommand(ctx context.Context, inv *Invocation, opts *executeCommandOptions) (*ExecutionResult, error) {
	if opts == nil {
		opts = &executeCommandOptions{}
	}
	if inv.Exec == nil {
		return nil, fmt.Errorf("subexec callback missing")
	}
	if len(opts.Argv) == 0 {
		return nil, fmt.Errorf("missing command")
	}
	searchEnv := opts.SearchEnv
	if searchEnv == nil {
		searchEnv = inv.Env
	}
	env := opts.Env
	if env == nil {
		env = inv.Env
	}
	workDir := opts.WorkDir
	if workDir == "" {
		workDir = inv.Cwd
	}

	return inv.Exec(ctx, &ExecutionRequest{
		Command:    append([]string(nil), opts.Argv...),
		SearchEnv:  searchEnv,
		Argv0:      opts.Argv0,
		Env:        env,
		WorkDir:    workDir,
		ReplaceEnv: opts.ReplaceEnv,
		Stdin:      opts.Stdin,
		Stdout:     opts.Stdout,
		Stderr:     opts.Stderr,
		Timeout:    opts.Timeout,
	})
}

type executeCommandOptions struct {
	Argv0      *string
	Argv       []string
	Env        map[string]string
	SearchEnv  map[string]string
	WorkDir    string
	ReplaceEnv bool
	Stdin      io.Reader
	Stdout     io.Writer
	Stderr     io.Writer
	Timeout    time.Duration
}

func shellJoinArgs(args []string) string {
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		quoted = append(quoted, shellQuoteForDisplay(arg))
	}
	return strings.Join(quoted, " ")
}

func shellQuoteForDisplay(value string) string {
	if value == "" {
		return "''"
	}
	if !shellNeedsCompositeQuote(value) {
		return shellQuoteWholeArg(value)
	}

	parts := make([]string, 0, len(value))
	start := 0
	for i := 0; i < len(value); i++ {
		if !shellNeedsANSICQuote(value[i]) {
			continue
		}
		if start < i {
			parts = append(parts, shellQuoteCompositeChunk(value[start:i]))
		}
		parts = append(parts, shellANSICQuote(value[i]))
		start = i + 1
	}
	if start < len(value) {
		parts = append(parts, shellQuoteCompositeChunk(value[start:]))
	}
	return strings.Join(parts, "")
}

func shellQuoteWholeArg(value string) string {
	if shellIsBareword(value) {
		return value
	}
	return shellQuoteCompositeChunk(value)
}

func shellQuoteCompositeChunk(value string) string {
	if value == "" {
		return ""
	}
	if !strings.Contains(value, "'") {
		return "'" + value + "'"
	}
	if !strings.ContainsAny(value, "\"\\$`") {
		return `"` + value + `"`
	}
	return shellSingleQuote(value)
}

func shellNeedsCompositeQuote(value string) bool {
	for i := 0; i < len(value); i++ {
		if shellNeedsANSICQuote(value[i]) {
			return true
		}
	}
	return false
}

func shellNeedsANSICQuote(ch byte) bool {
	switch ch {
	case '\a', '\b', '\f', '\n', '\r', '\t', '\v':
		return true
	default:
		return ch < 0x20 || ch == 0x7f
	}
}

func shellANSICQuote(ch byte) string {
	switch ch {
	case '\a':
		return `$'\a'`
	case '\b':
		return `$'\b'`
	case '\f':
		return `$'\f'`
	case '\n':
		return `$'\n'`
	case '\r':
		return `$'\r'`
	case '\t':
		return `$'\t'`
	case '\v':
		return `$'\v'`
	default:
		return fmt.Sprintf("$'\\x%02x'", ch)
	}
}

func shellIsBareword(value string) bool {
	for i := 0; i < len(value); i++ {
		ch := value[i]
		switch {
		case ch >= 'a' && ch <= 'z':
		case ch >= 'A' && ch <= 'Z':
		case ch >= '0' && ch <= '9':
		case strings.ContainsRune("%+,-./:=@_^", rune(ch)):
		default:
			return false
		}
	}
	return true
}

func shellSingleQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

func writeExecutionOutputs(inv *Invocation, result *ExecutionResult) error {
	if result == nil {
		return nil
	}
	// Write stderr before stdout so that trace output (xtrace) precedes
	// command output when both streams share the same writer (e.g. 2>&1).
	if result.Stderr != "" {
		if _, err := fmt.Fprint(inv.Stderr, result.Stderr); err != nil {
			return &ExitError{Code: 1, Err: err}
		}
	}
	if result.Stdout != "" {
		if _, err := fmt.Fprint(inv.Stdout, result.Stdout); err != nil {
			return &ExitError{Code: 1, Err: err}
		}
	}
	return nil
}

func exitForExecutionResult(result *ExecutionResult) error {
	if result == nil || result.ExitCode == 0 {
		return nil
	}
	return &ExitError{Code: result.ExitCode}
}

func sortedEnvPairs(env map[string]string) []string {
	pairs := make([]string, 0, len(env))
	for key, value := range env {
		pairs = append(pairs, key+"="+value)
	}
	slices.Sort(pairs)
	return pairs
}
