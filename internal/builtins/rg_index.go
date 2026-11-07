package builtins

import (
	"context"
	stdfs "io/fs"
	"path"
	"regexp/syntax"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/veypi/vsh/commands"
)

// rgIndexLiterals extracts one required literal from each alternative pattern.
// A literal is only a candidate filter; the original grep matcher remains
// authoritative. Unsupported or non-selective patterns return nil for live walk.
func rgIndexLiterals(opts *grepOptions) []string {
	if opts == nil || opts.invert || opts.filesWithoutMatch || opts.count {
		return nil
	}
	var out []string
	for _, input := range opts.patternInputs {
		if input.kind != grepPatternInputText {
			return nil
		}
		for _, pattern := range splitGrepPatternArg(input.value) {
			literal := pattern
			if opts.matchMode != grepPatternModeFixed {
				re, err := syntax.Parse(pattern, syntax.Perl)
				if err != nil {
					return nil
				}
				literal = requiredRegexpLiteral(re)
			}
			if len([]rune(literal)) < 3 {
				return nil
			}
			out = append(out, literal)
		}
	}
	return out
}

func requiredRegexpLiteral(re *syntax.Regexp) string {
	if re == nil {
		return ""
	}
	switch re.Op {
	case syntax.OpLiteral:
		return string(re.Rune)
	case syntax.OpCapture:
		return requiredRegexpLiteral(re.Sub[0])
	case syntax.OpPlus:
		return requiredRegexpLiteral(re.Sub[0])
	case syntax.OpRepeat:
		if re.Min > 0 {
			return requiredRegexpLiteral(re.Sub[0])
		}
	case syntax.OpConcat:
		best := ""
		for _, sub := range re.Sub {
			if literal := requiredRegexpLiteral(sub); len(literal) > len(best) {
				best = literal
			}
		}
		return best
	}
	return ""
}

type rgCachedRules struct {
	rules []rgIgnoreRule
	err   error
}

type rgIgnoreCacheEntry struct {
	revision     uint64
	abs, logical string
	rules        map[string]rgCachedRules
	ignored      map[string]bool
}

var rgIgnoreCaches sync.Map // map[commands.FileIndex]rgIgnoreCacheEntry

func cloneRGIgnored(src map[string]bool) map[string]bool {
	out := make(map[string]bool, len(src))
	for name, ignored := range src {
		out[name] = ignored
	}
	return out
}

func cloneRGCachedRules(src map[string]rgCachedRules) map[string]rgCachedRules {
	out := make(map[string]rgCachedRules, len(src))
	for name, cached := range src {
		cached.rules = append([]rgIgnoreRule(nil), cached.rules...)
		out[name] = cached
	}
	return out
}

func (c *RG) enumerateIndexedTarget(
	ctx context.Context,
	inv *Invocation,
	opts *rgOptions,
	state *grepRunState,
	abs, logicalAbs, displayBase string,
	initialRules []rgIgnoreRule,
	gitRoot string,
	literals []string,
	records *[]rgFileRecord,
) (bool, error) {
	if inv.FileIndex == nil || opts.followLinks || (!opts.filesMode && len(literals) == 0) {
		return false, nil
	}
	candidates, ok := inv.RGIndexCandidates(ctx, commands.FileIndexQuery{
		Root: abs, Literals: literals, FilesOnly: opts.filesMode, IncludeHidden: opts.hidden,
	})
	if !ok {
		return false, nil
	}
	rootRules := initialRules
	cacheable := !opts.noIgnore && !opts.noIgnoreVCS && len(initialRules) == 0 && len(opts.ignoreFiles) == 0 && len(opts.manualIgnore) == 0
	var cacheIndex commands.FileIndex
	var cacheRevision uint64
	if cacheable {
		cacheIndex, cacheRevision, cacheable = inv.RGIndexCacheKey()
	}
	var cache map[string]rgCachedRules
	ignoredCache := make(map[string]bool)
	cacheHit := false
	if cacheable {
		if value, ok := rgIgnoreCaches.Load(cacheIndex); ok {
			entry := value.(rgIgnoreCacheEntry)
			if entry.revision == cacheRevision && entry.abs == abs && entry.logical == logicalAbs {
				cached, ok := entry.rules[logicalAbs]
				if ok && cached.err == nil {
					cache = cloneRGCachedRules(entry.rules)
					ignoredCache = cloneRGIgnored(entry.ignored)
					rootRules = cached.rules
					cacheHit = true
				}
			}
		}
	}
	if !opts.noIgnore && !cacheHit {
		var err error
		rootRules, err = c.loadAutoIgnoreRules(ctx, inv, opts, abs, logicalAbs, initialRules, gitRoot)
		if err != nil {
			if grepShouldPropagateError(err) {
				return true, err
			}
			grepNoteFileError(inv, &opts.grep, state, displayBase, err)
			return true, nil
		}
	}
	if cache == nil {
		cache = map[string]rgCachedRules{logicalAbs: {rules: rootRules}}
	}
	if cacheable {
		defer func() {
			for _, cached := range cache {
				if cached.err != nil {
					return
				}
			}
			rgIgnoreCaches.Store(cacheIndex, rgIgnoreCacheEntry{
				revision: cacheRevision, abs: abs, logical: logicalAbs, rules: cloneRGCachedRules(cache),
				ignored: cloneRGIgnored(ignoredCache),
			})
		}()
	}
	var cacheMu sync.RWMutex
	cachedRule := func(name string) (rgCachedRules, bool) {
		cacheMu.RLock()
		defer cacheMu.RUnlock()
		cached, ok := cache[name]
		return cached, ok
	}
	storeRule := func(name string, rule rgCachedRules) {
		cacheMu.Lock()
		cache[name] = rule
		cacheMu.Unlock()
	}
	rulesFor := func(parentAbs, parentLogical string) ([]rgIgnoreRule, error) {
		if cached, ok := cachedRule(parentLogical); ok {
			return cached.rules, cached.err
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(parentLogical, logicalAbs), "/")
		currentAbs, currentLogical := abs, logicalAbs
		rules := rootRules
		if rel != "" && rel != "." {
			for _, component := range strings.Split(rel, "/") {
				currentAbs = path.Join(currentAbs, component)
				currentLogical = path.Join(currentLogical, component)
				if cached, ok := cachedRule(currentLogical); ok {
					rules = cached.rules
					if cached.err != nil {
						return nil, cached.err
					}
					continue
				}
				var err error
				rules = append([]rgIgnoreRule(nil), rules...)
				rules, err = c.loadAutoIgnoreRules(ctx, inv, opts, currentAbs, currentLogical, rules, gitRoot)
				storeRule(currentLogical, rgCachedRules{rules: rules, err: err})
				if err != nil {
					return nil, err
				}
			}
		}
		storeRule(parentLogical, rgCachedRules{rules: rules})
		return rules, nil
	}

	type item struct {
		abs, rel, display, logical string
		info                       stdfs.FileInfo
		err                        error
		explicitInclude            bool
	}
	items := make([]item, 0, len(candidates))
	for _, candidate := range candidates {
		rel, ok := rgIndexedRelative(abs, candidate)
		if !ok || rel == "." {
			continue
		}
		display := rgJoinDisplay(displayBase, rel)
		globDecision, err := rgEvaluateGlobs(opts.globs, display)
		if err != nil {
			return true, exitf(inv, 2, "rg: invalid glob: %v", err)
		}
		if globDecision.matchedExclude || !globDecision.allowed {
			continue
		}
		items = append(items, item{
			abs: candidate, rel: rel, display: display, logical: path.Join(logicalAbs, rel),
			explicitInclude: globDecision.explicitInclude,
		})
	}

	// Metadata preflight is independent for each candidate. Parallelize only
	// this step; ordered error reporting and ignore evaluation remain below.
	workers := min(8, len(items))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				items[i].info, _, items[i].err = lstatPath(ctx, inv, items[i].abs)
			}
		}()
	}
	for i := range items {
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	if !opts.noIgnore {
		type parentTarget struct {
			abs, logical string
			depth        int
		}
		unique := make(map[string]parentTarget)
		for i := range items {
			target := parentTarget{
				abs: path.Dir(items[i].abs), logical: path.Dir(items[i].logical),
				depth: strings.Count(path.Dir(items[i].logical), "/"),
			}
			unique[target.logical] = target
		}
		parents := make([]parentTarget, 0, len(unique))
		for _, target := range unique {
			parents = append(parents, target)
		}
		sort.Slice(parents, func(i, j int) bool {
			if parents[i].depth != parents[j].depth {
				return parents[i].depth < parents[j].depth
			}
			return parents[i].logical < parents[j].logical
		})
		for start := 0; start < len(parents); {
			end := start + 1
			for end < len(parents) && parents[end].depth == parents[start].depth {
				end++
			}
			jobs := make(chan parentTarget)
			var rulesWG sync.WaitGroup
			for range min(8, end-start) {
				rulesWG.Add(1)
				go func() {
					defer rulesWG.Done()
					for target := range jobs {
						_, _ = rulesFor(target.abs, target.logical)
					}
				}()
			}
			for _, target := range parents[start:end] {
				jobs <- target
			}
			close(jobs)
			rulesWG.Wait()
			start = end
		}
	}

	for i := range items {
		item := &items[i]
		if item.err != nil {
			if grepShouldPropagateError(item.err) {
				return true, item.err
			}
			grepNoteFileError(inv, &opts.grep, state, item.display, item.err)
			continue
		}
		if item.info.IsDir() || item.info.Mode()&stdfs.ModeSymlink != 0 {
			continue
		}
		hidden := !opts.hidden && rgIndexedHidden(item.rel)
		ignored, known := ignoredCache[item.logical]
		if !known && !opts.noIgnore {
			parentRules, err := rulesFor(path.Dir(item.abs), path.Dir(item.logical))
			if err != nil {
				if grepShouldPropagateError(err) {
					return true, err
				}
				grepNoteFileError(inv, &opts.grep, state, item.display, err)
				continue
			}
			ignoreRules := parentRules
			if len(opts.manualIgnore) > 0 {
				ignoreRules = append(append([]rgIgnoreRule(nil), parentRules...), opts.manualIgnore...)
			}
			if len(ignoreRules) > 0 {
				ignored, err = rgShouldIgnorePath(ignoreRules, item.logical, false)
				if err != nil {
					return true, exitf(inv, 2, "rg: invalid ignore pattern: %v", err)
				}
			}
			ignoredCache[item.logical] = ignored
		}
		if (hidden || ignored) && !item.explicitInclude {
			continue
		}
		*records = append(*records, rgFileRecord{
			abs: item.abs, display: item.display, showName: true,
			explicit: false, skipBinary: !opts.filesMode,
		})
	}
	return true, nil
}

func rgIndexedRelative(root, name string) (string, bool) {
	root = path.Clean(root)
	name = path.Clean(name)
	if name == root || (runtime.GOOS == "windows" && strings.EqualFold(name, root)) {
		return ".", true
	}
	prefix := strings.TrimSuffix(root, "/") + "/"
	if len(name) < len(prefix) {
		return "", false
	}
	if !strings.HasPrefix(name, prefix) && (runtime.GOOS != "windows" || !strings.EqualFold(name[:len(prefix)], prefix)) {
		return "", false
	}
	return name[len(prefix):], true
}

func rgIndexedHidden(rel string) bool {
	for _, component := range strings.Split(rel, "/") {
		if strings.HasPrefix(component, ".") {
			return true
		}
	}
	return false
}
