package runtime

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/veypi/vsh/internal/builtins"
)

// TestBuiltinHelpCoverage (vsh fork, M1.3.2): every user-facing builtin must
// natively answer `<name> --help` through the engine with exit 0 and non-empty
// usage text (D7: the tool layer never intercepts --help). Internal helpers
// (__-prefixed) are exempt. Failures are reported as a complete list.
func TestBuiltinHelpCoverage(t *testing.T) {
	t.Parallel()

	names := builtins.DefaultRegistry().Names()
	sort.Strings(names)

	rt := newRuntime(t, &Config{})

	var missing []string
	for _, name := range names {
		if strings.HasPrefix(name, "__") {
			continue
		}
		result, err := rt.Run(context.Background(), &ExecutionRequest{
			Script: name + " --help\n",
		})
		if err != nil {
			missing = append(missing, fmt.Sprintf("%s (run err=%v)", name, err))
			continue
		}
		out := strings.TrimSpace(result.Stdout + result.Stderr)
		if result.ExitCode != 0 || out == "" {
			missing = append(missing, fmt.Sprintf("%s (exit=%d out=%q)", name, result.ExitCode, truncateHelp(out, 80)))
		}
	}

	if len(missing) > 0 {
		t.Fatalf("%d builtins lack native --help:\n%s", len(missing), strings.Join(missing, "\n"))
	}
}

func truncateHelp(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
