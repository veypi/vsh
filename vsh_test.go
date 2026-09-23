package vsh_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/veypi/vsh"
)

func TestNewRunsSimpleScript(t *testing.T) {
	t.Parallel()

	rt, err := vsh.New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	result, err := rt.Run(context.Background(), &vsh.ExecutionRequest{
		Script: "echo hi\npwd\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0", result.ExitCode)
	}
	if got, want := result.Stdout, "hi\n/home/agent\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}

func TestWithWorkspaceMountsHostDirectory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "note.txt"), []byte("workspace\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	rt, err := vsh.New(vsh.WithWorkspace(root))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	result, err := rt.Run(context.Background(), &vsh.ExecutionRequest{
		Script: "pwd\ncat note.txt\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0", result.ExitCode)
	}
	if got, want := result.Stdout, vsh.DefaultWorkspaceMountPoint+"\nworkspace\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}

func TestWithHTTPAccessEnablesCurl(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/status" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte("ok\n"))
	}))
	defer server.Close()

	rt, err := vsh.New(vsh.WithHTTPAccess(server.URL + "/"))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	result, err := rt.Run(context.Background(), &vsh.ExecutionRequest{
		Script: "curl -s " + shellQuote(server.URL+"/status") + "\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0, stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, "ok\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
