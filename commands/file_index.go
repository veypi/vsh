package commands

import (
	"context"

	"github.com/veypi/vsh/policy"
)

// FileIndexQuery describes an optional indexed-file candidate lookup.
//
// Root must be an absolute path in the command filesystem's canonical form.
// FilesOnly requests the file snapshot; otherwise Literals contains alternative
// literal strings and the returned paths are a superset of files that may contain
// at least one of them. Callers must always read and match the original files.
type FileIndexQuery struct {
	Root          string
	Literals      []string
	FilesOnly     bool
	IncludeHidden bool
}

// FileIndex is an optional resident candidate index supplied by the embedder.
// ok=false means the query cannot be accelerated and the caller must walk the
// filesystem. The index never grants access or replaces the real matcher.
type FileIndex interface {
	Candidates(ctx context.Context, query FileIndexQuery) (paths []string, ok bool)
}

// policyFileIndex prevents the optional capability from becoming a path-listing
// bypass for custom commands. Every candidate is checked against the current
// invocation filesystem policy before it becomes visible to the command.
// RGIndexCacheKey identifies the underlying resident index and its current
// revision. rg uses it to invalidate derived ignore-rule caches on any change.
func (i *Invocation) RGIndexCacheKey() (FileIndex, uint64, bool) {
	if i == nil || i.FileIndex == nil {
		return nil, 0, false
	}
	index := i.FileIndex
	if wrapped, ok := index.(*policyFileIndex); ok {
		index = wrapped.index
	}
	versioned, ok := index.(interface{ IndexRevision() uint64 })
	if !ok {
		return nil, 0, false
	}
	return index, versioned.IndexRevision(), true
}

// RGIndexCandidates avoids the policy wrapper's per-file precheck for rg only.
// rg subsequently performs its own policy-checked Lstat and Read for every
// candidate. Other custom commands must keep using Invocation.FileIndex.
func (i *Invocation) RGIndexCandidates(ctx context.Context, query FileIndexQuery) ([]string, bool) {
	if i == nil || i.FileIndex == nil {
		return nil, false
	}
	if wrapped, ok := i.FileIndex.(*policyFileIndex); ok {
		return wrapped.index.Candidates(ctx, query)
	}
	return i.FileIndex.Candidates(ctx, query)
}

type policyFileIndex struct {
	index FileIndex
	fs    *CommandFS
}

func (p *policyFileIndex) Candidates(ctx context.Context, query FileIndexQuery) ([]string, bool) {
	if p == nil || p.index == nil || p.fs == nil {
		return nil, false
	}
	if _, err := p.fs.Stat(ctx, query.Root); err != nil {
		return nil, false
	}
	paths, ok := p.index.Candidates(ctx, query)
	if !ok {
		return nil, false
	}
	allowed := make([]string, 0, len(paths))
	for _, name := range paths {
		// Authorization is the gate here; the rg caller still stats and reads the
		// file. Avoid a duplicate filesystem Stat for every indexed candidate.
		if err := p.fs.checkQuiet(ctx, policy.FileActionStat, p.fs.Resolve(name)); err == nil {
			allowed = append(allowed, name)
		}
	}
	return allowed, true
}
