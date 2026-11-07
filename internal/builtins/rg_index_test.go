package builtins_test

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/veypi/vsh/commands"
)

type stubRGIndex struct {
	paths []string
	calls int
}

func (s *stubRGIndex) Candidates(context.Context, commands.FileIndexQuery) ([]string, bool) {
	s.calls++
	return append([]string(nil), s.paths...), true
}

type versionedRGIndex struct {
	*stubRGIndex
	revision atomic.Uint64
}

func (s *versionedRGIndex) IndexRevision() uint64 { return s.revision.Load() }

func TestRGIgnoreCacheInvalidatesOnIndexRevision(t *testing.T) {
	index := &versionedRGIndex{stubRGIndex: &stubRGIndex{paths: []string{
		"/tmp/work/ignored.txt", "/tmp/work/visible.txt",
	}}}
	index.revision.Store(1)
	rt := newRuntime(t, &Config{FileIndex: index})
	first, err := rt.Run(context.Background(), &ExecutionRequest{Script: "mkdir -p /tmp/work/.git\nprintf 'ignored.txt\\n' > /tmp/work/.gitignore\nprintf 'x\\n' > /tmp/work/ignored.txt\nprintf 'x\\n' > /tmp/work/visible.txt\ncd /tmp/work\nrg --files . | sort\n"})
	if err != nil {
		t.Fatal(err)
	}
	if first.Stdout != "./visible.txt\n" {
		t.Fatalf("first stdout=%q stderr=%q", first.Stdout, first.Stderr)
	}

	index.revision.Add(1)
	second, err := rt.Run(context.Background(), &ExecutionRequest{Script: "mkdir -p /tmp/work/.git\nprintf '\\n' > /tmp/work/.gitignore\nprintf 'x\\n' > /tmp/work/ignored.txt\nprintf 'x\\n' > /tmp/work/visible.txt\ncd /tmp/work\nrg --files . | sort\n"})
	if err != nil {
		t.Fatal(err)
	}
	if second.Stdout != "./ignored.txt\n./visible.txt\n" {
		t.Fatalf("revision did not invalidate ignore cache: stdout=%q stderr=%q exit=%d", second.Stdout, second.Stderr, second.ExitCode)
	}
}

func TestRGCountUsesLiveFileSet(t *testing.T) {
	index := &stubRGIndex{paths: []string{"/tmp/work/hit.txt"}}
	rt := newRuntime(t, &Config{FileIndex: index})
	result, err := rt.Run(context.Background(), &ExecutionRequest{Script: "mkdir -p /tmp/work\nprintf 'hit\\n' > /tmp/work/hit.txt\nprintf 'miss\\n' > /tmp/work/miss.txt\ncd /tmp/work\nrg -c hit . | sort\n"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Stdout != "./hit.txt:1\n./miss.txt:0\n" || index.calls != 0 {
		t.Fatalf("count result=%q index calls=%d", result.Stdout, index.calls)
	}
}

func TestRGIndexedCandidatesKeepIgnoreAndHiddenSemantics(t *testing.T) {
	index := &stubRGIndex{paths: []string{
		"/tmp/work/.git/HEAD",
		"/tmp/work/.gitignore",
		"/tmp/work/.hidden.txt",
		"/tmp/work/ignored.log",
		"/tmp/work/sub/local.txt",
		"/tmp/work/visible.txt",
	}}
	rt := newRuntime(t, &Config{FileIndex: index})
	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "mkdir -p /tmp/work/.git /tmp/work/sub\n" +
			"printf 'ref: refs/heads/main\\n' > /tmp/work/.git/HEAD\n" +
			"printf '*.log\\n' > /tmp/work/.gitignore\n" +
			"printf 'hit\\n' > /tmp/work/.hidden.txt\n" +
			"printf 'hit\\n' > /tmp/work/ignored.log\n" +
			"printf 'hit\\n' > /tmp/work/sub/local.txt\n" +
			"printf 'hit\\n' > /tmp/work/visible.txt\n" +
			"cd /tmp/work\n" +
			"rg hit . | sort\n" +
			"printf '%s\\n' '---'\n" +
			"rg --hidden --no-ignore hit . | sort\n" +
			"printf '%s\\n' '---'\n" +
			"rg --files . | sort\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("exit=%d stderr=%q", result.ExitCode, result.Stderr)
	}
	want := strings.Join([]string{
		"./sub/local.txt:hit",
		"./visible.txt:hit",
		"---",
		"./.hidden.txt:hit",
		"./ignored.log:hit",
		"./sub/local.txt:hit",
		"./visible.txt:hit",
		"---",
		"./sub/local.txt",
		"./visible.txt",
		"",
	}, "\n")
	if result.Stdout != want {
		t.Fatalf("stdout=%q want=%q", result.Stdout, want)
	}
	if index.calls != 3 {
		t.Fatalf("index calls=%d, want 3", index.calls)
	}
}
