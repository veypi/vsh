package builtins_test

import (
	"context"
	"strings"
	"testing"

	vsh "github.com/veypi/vsh"
)

func TestCompoptPersistsAcrossCommandsInScript(t *testing.T) {
	t.Parallel()
	session := newSession(t, &Config{})

	result := mustExecSession(t, session, "complete -W 'foo bar' mycommand\ncompopt -o nospace mycommand\ncomplete -p mycommand\n")
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0 (stderr=%q)", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, "complete -o nospace -W 'foo bar' mycommand\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
	if got := result.Stderr; got != "" {
		t.Fatalf("Stderr = %q, want empty", got)
	}
}

func TestCompoptErrorsWithoutActiveCompletionContext(t *testing.T) {
	t.Parallel()
	session := newSession(t, &Config{})

	result := mustExecSession(t, session, "compopt -o filenames +o nospace\n")
	if result.ExitCode != 1 {
		t.Fatalf("ExitCode = %d, want 1", result.ExitCode)
	}
	if got, want := result.Stderr, "compopt: not currently executing completion function\n"; got != want {
		t.Fatalf("Stderr = %q, want %q", got, want)
	}
}

func TestCompoptInvalidOptionReturnsExitCodeTwo(t *testing.T) {
	t.Parallel()
	session := newSession(t, &Config{})

	result := mustExecSession(t, session, "compopt -o invalid cmd\n")
	if result.ExitCode != 2 {
		t.Fatalf("ExitCode = %d, want 2", result.ExitCode)
	}
	if got, want := result.Stderr, "compopt: invalid: invalid option name\n"; got != want {
		t.Fatalf("Stderr = %q, want %q", got, want)
	}
}

func TestCompoptModifiesDefaultAndEmptyCompletionScopes(t *testing.T) {
	t.Parallel()
	session := newSession(t, &Config{})
	result := mustExecSession(t, session, "complete -F myfunc -D\ncompopt -D -o nospace -o filenames\ncomplete -W '' -E\ncompopt -E -o default\ncomplete -p -D\ncomplete -p -E\n")
	if result.ExitCode != 0 || result.Stderr != "" {
		t.Fatalf("result = %+v", result)
	}
	for _, part := range []string{"-F myfunc", "-o nospace", "-o filenames", "-D", "-o default", "-E"} {
		if !strings.Contains(result.Stdout, part) {
			t.Fatalf("missing %q in %q", part, result.Stdout)
		}
	}
}

func TestCompoptPreservesExistingSpecWhileDisablingOptions(t *testing.T) {
	t.Parallel()
	session := newSession(t, &Config{})
	result := mustExecSession(t, session, "complete -o nospace -o filenames -F myfunc cmd\ncompopt +o nospace cmd\ncomplete -p cmd\n")
	if result.ExitCode != 0 || result.Stdout != "complete -o filenames -F myfunc cmd\n" {
		t.Fatalf("result = %+v", result)
	}
}

func TestCompoptPersistsAcrossInteractiveEntries(t *testing.T) {
	t.Parallel()
	session := newSession(t, &Config{})

	var stdout strings.Builder
	var stderr strings.Builder
	result, err := session.Interact(context.Background(), &vsh.InteractiveRequest{
		Stdin:  strings.NewReader("complete -F myfunc cmd\ncompopt -o nospace cmd\ncomplete -p cmd\nexit\n"),
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if err != nil {
		t.Fatalf("Interact() error = %v", err)
	}
	if result == nil {
		t.Fatalf("Interact() result = nil")
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stdout=%q stderr=%q", result.ExitCode, stdout.String(), stderr.String())
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("stderr = %q, want empty", got)
	}
	if !strings.Contains(stdout.String(), "complete -o nospace -F myfunc cmd\n") {
		t.Fatalf("stdout = %q, want completion output", stdout.String())
	}
}

func TestCompletionBuiltinsShareState(t *testing.T) {
	t.Parallel()
	session := newSession(t, &Config{})

	result := mustExecSession(t, session, ""+
		"complete -W 'foo bar' cmd\n"+
		"builtin complete -p cmd\n"+
		"compopt -o nospace cmd\n"+
		"builtin complete -p cmd\n"+
		"builtin compopt +o nospace cmd\n"+
		"complete -p cmd\n")
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0 (stderr=%q)", result.ExitCode, result.Stderr)
	}
	const want = "" +
		"complete -W 'foo bar' cmd\n" +
		"complete -o nospace -W 'foo bar' cmd\n" +
		"complete -W 'foo bar' cmd\n"
	if got := result.Stdout; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
	if got := result.Stderr; got != "" {
		t.Fatalf("Stderr = %q, want empty", got)
	}
}

func TestCompgenBuiltinEntryPoints(t *testing.T) {
	t.Parallel()
	session := newSession(t, &Config{})

	result := mustExecSession(t, session, ""+
		"compgen -A builtin g\n"+
		"echo ---\n"+
		"builtin compgen -A builtin g\n"+
		"echo ---\n"+
		"command compgen -A builtin g\n"+
		"echo ---\n"+
		"builtin compgen -A builtin g\n")
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0 (stderr=%q)", result.ExitCode, result.Stderr)
	}
	const want = "" +
		"getopts\n" +
		"---\n" +
		"getopts\n" +
		"---\n" +
		"getopts\n" +
		"---\n" +
		"getopts\n"
	if got := result.Stdout; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}

func TestCompgenAcceptsKeywordAndExportActions(t *testing.T) {
	t.Parallel()
	session := newSession(t, &Config{})

	result := mustExecSession(t, session, ""+
		"export MATCH_ME=1\n"+
		"compgen -A export MATCH_\n"+
		"echo ---\n"+
		"compgen -A keyword wh\n")
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0 (stderr=%q)", result.ExitCode, result.Stderr)
	}
	const want = "" +
		"MATCH_ME\n" +
		"---\n" +
		"while\n"
	if got := result.Stdout; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
	if got := result.Stderr; got != "" {
		t.Fatalf("Stderr = %q, want empty", got)
	}
}

func TestCompgenCommandActionRespectsDisabledBuiltins(t *testing.T) {
	t.Parallel()
	session := newSession(t, &Config{})

	result := mustExecSession(t, session, ""+
		"enable -n eval\n"+
		"compgen -A builtin ev\n"+
		"echo ---\n"+
		"compgen -A helptopic ev\n"+
		"echo ---\n"+
		"compgen -A command ev\n"+
		"echo ---\n"+
		"enable -n printf\n"+
		"PATH=/bin\n"+
		"compgen -A command printf\n")
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0 (stderr=%q)", result.ExitCode, result.Stderr)
	}
	const want = "" +
		"eval\n" +
		"---\n" +
		"eval\n" +
		"---\n" +
		"---\n" +
		"printf\n"
	if got := result.Stdout; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
	if got := result.Stderr; got != "" {
		t.Fatalf("Stderr = %q, want empty", got)
	}
}
