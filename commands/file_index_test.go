package commands

import (
	"context"
	"errors"
	"os"
	"testing"

	gbfs "github.com/veypi/vsh/fs"
	"github.com/veypi/vsh/policy"
)

type testIndex struct{ paths []string }

func (i testIndex) Candidates(context.Context, FileIndexQuery) ([]string, bool) {
	return append([]string(nil), i.paths...), true
}

type denyIndexPath struct{ *policy.Static }

func (p denyIndexPath) AllowPath(ctx context.Context, action policy.FileAction, target string) error {
	if target == "/denied.txt" {
		return errors.New("denied")
	}
	return p.Static.AllowPath(ctx, action, target)
}

func TestInvocationFileIndexFiltersDeniedCandidates(t *testing.T) {
	ctx := context.Background()
	fsys := gbfs.NewMemory()
	for _, name := range []string{"/allowed.txt", "/denied.txt"} {
		f, err := fsys.OpenFile(ctx, name, os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			t.Fatal(err)
		}
		_ = f.Close()
	}
	base := policy.NewStatic(nil)
	inv := NewInvocation(&InvocationOptions{
		FileSystem: fsys,
		Policy:     denyIndexPath{base},
		FileIndex:  testIndex{paths: []string{"/allowed.txt", "/denied.txt"}},
	})
	paths, ok := inv.FileIndex.Candidates(ctx, FileIndexQuery{Root: "/", FilesOnly: true})
	if !ok || len(paths) != 1 || paths[0] != "/allowed.txt" {
		t.Fatalf("candidates = %v, %v", paths, ok)
	}
}
