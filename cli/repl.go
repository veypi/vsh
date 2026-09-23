package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/veypi/vsh"
	"github.com/veypi/vsh/internal/builtins"
)

func runInteractiveShell(ctx context.Context, rt *vsh.Runtime, parsed *builtins.BashInvocation, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	session, err := rt.NewSession(ctx)
	if err != nil {
		return 1, fmt.Errorf("init session: %w", err)
	}

	if parsed == nil {
		parsed = &builtins.BashInvocation{
			Name:          "vsh",
			ExecutionName: "vsh",
		}
	}
	if parsed.ExecutionName == "" {
		parsed.ExecutionName = parsed.Name
	}

	result, err := session.Interact(ctx, &vsh.InteractiveRequest{
		Name:           parsed.ExecutionName,
		Args:           append([]string(nil), parsed.Args...),
		StartupOptions: append([]string(nil), parsed.StartupOptions...),
		Stdin:          stdin,
		Stdout:         stdout,
		Stderr:         stderr,
	})
	if err != nil {
		return 1, fmt.Errorf("interactive shell error: %w", err)
	}
	if result == nil {
		return 0, nil
	}
	return result.ExitCode, nil
}
