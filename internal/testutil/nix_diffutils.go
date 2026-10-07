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
	nixDiffEnv             = "VSH_CONFORMANCE_DIFF"
	pinnedNixDiffVersion   = "3.12"
	pinnedNixDiffSubstring = "GNU diffutils) " + pinnedNixDiffVersion
)

var errNixDiffUnset = errors.New(nixDiffEnv + " is not set")

// RequireNixDiff returns the pinned GNU diff oracle configured for the test
// suite, failing the test when it is unavailable or misconfigured.
func RequireNixDiff(tb testing.TB) string {
	tb.Helper()

	path, firstLine, err := resolveNixDiff(tb.Context())
	if err != nil {
		tb.Fatalf("%v\n\n%s", err, nixDiffInstructions())
	}
	tb.Logf("diff oracle: %s (%s)", firstLine, path)
	return path
}

// RequireNixDiffOrSkip returns the pinned GNU diff oracle configured for the
// test suite, skipping the test when it is unset. If the env var is set but
// points at the wrong diff, the test fails so misconfiguration is surfaced
// immediately.
func RequireNixDiffOrSkip(tb testing.TB) string {
	tb.Helper()

	path, firstLine, err := resolveNixDiff(tb.Context())
	if err != nil {
		if errors.Is(err, errNixDiffUnset) {
			tb.Skipf("%v\n\n%s", err, nixDiffInstructions())
		}
		tb.Fatalf("%v\n\n%s", err, nixDiffInstructions())
	}
	tb.Logf("diff oracle: %s (%s)", firstLine, path)
	return path
}

func resolveNixDiff(ctx context.Context) (path, firstLine string, err error) {
	path = strings.TrimSpace(os.Getenv(nixDiffEnv)) //nolint:forbidigo // Tests explicitly read the oracle diff path from the host env.
	if path == "" {
		return "", "", errNixDiffUnset
	}

	out, err := exec.CommandContext(ctx, path, "--version").Output() //nolint:forbidigo // Tests validate the configured external diff oracle before use.
	if err != nil {
		return "", "", fmt.Errorf("failed to get diff version from %s: %w", path, err)
	}

	firstLine, _, _ = strings.Cut(string(out), "\n")
	if !strings.Contains(firstLine, pinnedNixDiffSubstring) {
		return "", "", fmt.Errorf(
			"tests require diffutils %s (pinned via Nix), got: %s",
			pinnedNixDiffVersion,
			firstLine,
		)
	}

	return path, firstLine, nil
}

func nixDiffInstructions() string {
	return "set " + nixDiffEnv + " to a GNU diffutils " + pinnedNixDiffVersion + " binary to run the oracle test:\n" +
		"  export " + nixDiffEnv + "=/path/to/diff\n" +
		"(本 fork 不含上游的 Nix/oracle 下载脚本；未设置时该用例会被跳过。)"
}
