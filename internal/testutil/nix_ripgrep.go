package testutil

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

const (
	nixRipgrepEnv             = "VSH_CONFORMANCE_RIPGREP"
	pinnedNixRipgrepVersion   = "15.1.0"
	pinnedNixRipgrepSubstring = "ripgrep " + pinnedNixRipgrepVersion
)

var errNixRipgrepUnset = errors.New(nixRipgrepEnv + " is not set")

// RequireNixRipgrep returns the pinned ripgrep oracle configured for the test
// suite, failing the test when it is unavailable or misconfigured.
func RequireNixRipgrep(tb testing.TB) string {
	tb.Helper()

	path, firstLine, err := resolveNixRipgrep(tb.Context())
	if err != nil {
		tb.Fatalf("%v\n\n%s", err, nixRipgrepInstructions())
	}
	tb.Logf("ripgrep oracle: %s (%s)", firstLine, path)
	return path
}

// RequireNixRipgrepOrSkip returns the pinned ripgrep oracle configured for the
// test suite, skipping the test when it is unset. If the env var is set but
// points at the wrong ripgrep, the test fails so misconfiguration is surfaced
// immediately.
func RequireNixRipgrepOrSkip(tb testing.TB) string {
	tb.Helper()

	path, firstLine, err := resolveNixRipgrep(tb.Context())
	if err != nil {
		if errors.Is(err, errNixRipgrepUnset) {
			tb.Skipf("%v\n\n%s", err, nixRipgrepInstructions())
		}
		tb.Fatalf("%v\n\n%s", err, nixRipgrepInstructions())
	}
	tb.Logf("ripgrep oracle: %s (%s)", firstLine, path)
	return path
}

func resolveNixRipgrep(ctx context.Context) (path, firstLine string, err error) {
	path = strings.TrimSpace(os.Getenv(nixRipgrepEnv)) //nolint:forbidigo // Tests explicitly read the oracle ripgrep path from the host env.
	if path == "" {
		return "", "", errNixRipgrepUnset
	}

	out, err := exec.CommandContext(ctx, path, "--version").Output() //nolint:forbidigo // Tests validate the configured external ripgrep oracle before use.
	if err != nil {
		return "", "", fmt.Errorf("failed to get ripgrep version from %s: %w", path, err)
	}

	firstLine, _, _ = strings.Cut(string(out), "\n")
	if !strings.Contains(firstLine, pinnedNixRipgrepSubstring) {
		return "", "", fmt.Errorf(
			"tests require ripgrep %s (pinned via Nix), got: %s",
			pinnedNixRipgrepVersion,
			firstLine,
		)
	}

	return path, firstLine, nil
}

func nixRipgrepInstructions() string {
	return "set " + nixRipgrepEnv + " to a ripgrep " + pinnedNixRipgrepVersion + " binary to run the oracle test:\n" +
		"  export " + nixRipgrepEnv + "=/path/to/rg\n" +
		"(本 fork 不含上游的 Nix/oracle 下载脚本；未设置时该用例会被跳过。)"
}
