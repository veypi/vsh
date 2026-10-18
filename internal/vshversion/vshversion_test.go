package vshversion

import (
	"os"
	"strings"
	"testing"
)

// TestVersionMatchesVersionFile 保证常量与仓库根 VERSION 文件同步。
func TestVersionMatchesVersionFile(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../VERSION")
	if err != nil {
		t.Fatalf("read VERSION: %v", err)
	}
	if got, want := Version, strings.TrimSpace(string(data)); got != want {
		t.Fatalf("Version = %q, want %q (update VERSION and vshversion.Version together)", got, want)
	}
}
