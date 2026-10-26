# vsh

**A pure-Go in-process shell engine** you can embed in any Go program: parsing, execution
and 90+ built-in commands, with the command table, filesystem and network egress all
injected by the caller. It starts no host program by default and does not require `bash` to
exist on the machine.

[中文](README.md) | **English**

```go
rt, _ := vsh.New()                       // default: in-memory FS, built-ins, no network
sess, _ := rt.NewSession(ctx)
res, _ := sess.Exec(ctx, &vsh.ExecutionRequest{
	Script: `for i in 1 2 3; do echo "line $i"; done | wc -l`,
})
fmt.Print(res.Stdout)   // 3
```

The execution contract lives in [docs/reference/spec.md](docs/reference/spec.md);
future work lives in [docs/development/todo.md](docs/development/todo.md); the documentation
map in [docs/index.md](docs/index.md); the security model in
[docs/reference/threat-model.md](docs/reference/threat-model.md) and
[SECURITY.md](SECURITY.md); origin and attribution
at the end of this file.

## Highlights

- **In-process execution**: `cat`, `ls`, `grep`, `sed`, `awk`… are Go implementations; the
  host shell is never forked. External-program capability is explicitly supplied by the caller
  and absent by default. `WithFileCommandResolver` turns a resolved file into an ordinary
  command implementation, which then passes through the same `AllowExec` gate as every other command.
- **Every capability is injected**: command table (`WithRegistry`), filesystem
  (`WithFileSystem` / `WithWorkspace`), network (`WithNetworkClient` / `WithHTTPAccess`).
  The engine implements virtual-shell semantics and uniform command admission. It knows
  nothing about approvals, users, credentials or auditing, and provides no OS/native-process
  sandbox.
- **Virtualized by default**: with no options you get an in-memory filesystem
  (`/home/agent`), built-in commands, a static policy and no network. Each `Session` owns
  its state; execution is serial within a session and isolated between sessions. These are
  in-process virtual runtime boundaries, not a native-process isolation guarantee.
- **Observable**: `WithTracing`, `WithLogger` and `WithAnalysisObserver` emit structured
  events.
- **Quotaed**: command count, loop iterations, glob count, output size and more can be
  tightened via `WithPolicy` / `WithLimitOverrides`, so untrusted scripts cannot drag the
  host down.

## Install

```sh
go get github.com/veypi/vsh
```

Go 1.26+ (see [go.mod](go.mod)).

## Quick start (embedding)

```go
package main

import (
	"context"
	"fmt"

	"github.com/veypi/vsh"
)

func main() {
	ctx := context.Background()

	rt, err := vsh.New() // default: in-memory FS + built-ins + no network
	if err != nil {
		panic(err)
	}
	sess, err := rt.NewSession(ctx)
	if err != nil {
		panic(err)
	}

	res, err := sess.Exec(ctx, &vsh.ExecutionRequest{
		Script: `for i in 1 2 3; do echo "line $i"; done | wc -l`,
	})
	if err != nil {
		panic(err)
	}
	fmt.Print(res.Stdout)     // 3
	fmt.Println(res.ExitCode) // 0
}
```

Mount a real directory, open controlled network access, add your own commands:

```go
rt, err := vsh.New(
	vsh.WithWorkspace("/srv/app"),                  // host directory mounted into the sandbox
	vsh.WithHTTPAccess("https://api.example.com"),  // curl may only reach this prefix
	vsh.WithRegistry(reg),                          // custom commands; may override built-ins
	vsh.WithLimitOverrides(policy.Limits{MaxCommandCount: 2000}),
)
res, err := rt.Run(ctx, &vsh.ExecutionRequest{Script: `cat data/*.json | jq -r .id`})
```

`Runtime.Run` creates a throwaway session for a single call; use `Runtime.NewSession` plus
repeated `Session.Exec` when several scripts must share state.

## Command line

```sh
go install github.com/veypi/vsh/cmd/vsh@latest

vsh -c 'echo hello | tr a-z A-Z'                      # HELLO
vsh --root ./project --cwd /home/agent/project -c ls   # read-only mount + in-memory overlay
vsh -i                                                 # interactive
```

`vsh --help` lists the shell options and the virtual filesystem options (`--root` mounts a
host directory read-only with an in-memory overlay, `--readwrite-root` writes through to the
host, `--inherit-env` passes selected host environment variables through). `cmd/vsh-gnu` is
the harness used to run the GNU coreutils test suite against the built-ins.

## Extension points

| Option | Purpose |
| --- | --- |
| `WithRegistry` | Command table; bare-name lookup order = registry → real `PATH`; an explicit path checks the identical registry key before files |
| `WithFileSystem` / `WithWorkspace` / `WithWorkingDir` | Filesystem backend and the session's initial cwd |
| `WithNetworkClient` / `WithNetwork` / `WithHTTPAccess` | Network egress and the allowed URL prefixes |
| `WithPolicy` / `WithLimitOverrides` | Unified `AllowExec` admission, virtual-resource policy and quotas |
| `WithFileCommandResolver` | Optional file-command resolver; constructs regular `commands.Command` values, while process isolation remains the caller implementation's responsibility |
| `WithTracing` / `WithLogger` / `WithAnalysisObserver` | Structured events, logs, read-only semantic observation |
| `WithBaseEnv` | Base environment for child processes |

Public sub-packages: `commands` (authoring and registering commands), `fs` (filesystem
backends), `policy` (limits and policy), `network` (HTTP client), `shell/syntax` (syntax
tree), `trace` (event model). `internal/*` and anything not listed here is not public API.

## contrib

Every tool under [`contrib/`](contrib) is an **independent Go module** (its own `go.mod`,
`CHANGELOG.md` and version), and the root module does not depend on any of them — pull in
only what you need and the rest stays out of your build graph:

```sh
go get github.com/veypi/vsh/contrib/jq@v0.1.0
```

| Module | Provides | How to include |
| --- | --- | --- |
| `contrib/awk` | `awk` command | Explicitly, or via `extras` |
| `contrib/htmltomarkdown` | `htmltomarkdown` command | Explicitly, or via `extras` |
| `contrib/jq` | `jq` command (gojq) | Explicitly, or via `extras` |
| `contrib/sqlite3` | `sqlite3` command | Explicitly, or via `extras` |
| `contrib/yq` | `yq` command | Explicitly, or via `extras` |
| `contrib/extras` | Aggregate registration of the five above (incl. `FullRegistry()`) | Aggregate |
| `contrib/nodejs` | Experimental `nodejs` command (not hardened, not in `extras`) | Explicitly |

`extras` depends on those five modules, so contrib has a version-following obligation — see
"Release" below.

## examples

[`examples/`](examples) is an independent module covering common embedding patterns:
OpenTelemetry tracing, sqlite and agentfs as filesystem backends, transactional workspaces,
custom commands (zstd), a harness overlay, OAuth network extensions, and more.

## Known limits

- The target is a **bash-compatible subset**, not a full bash implementation; [docs/reference/spec.md](docs/reference/spec.md)
  is the contract.
- vsh is a virtual shell, not an OS/native-process sandbox. Any isolation required by an
  external-program command must be supplied by its caller-provided implementation.
- No external-program capability exists by default: `ls` and friends run as built-ins. A real
  program requires both an explicitly configured file-command resolver and a policy that admits the command.
- Registry hits take priority over `PATH`; an explicit path (`/bin/ls`) checks the identical
  registry key before touching real files and never creates or repairs stub directories.
- Execution within one session is serial; use several sessions for concurrency.
- On Windows, paths and environment are normalized to the `/c/...` canonical form at the
  entry points and restored at the OS boundary; native semicolon `PATH` is not parsed.
- In `contrib/jq`, the stream-error cases fail under Go 1.27 (error text / column-number
  assertion differences). This is an inherited failure from the earliest baseline and is
  unrelated to command parsing.

## Platform support

| Platform | Status |
| --- | --- |
| darwin (arm64 / amd64) | **Supported**: CI build + full test suite |
| linux (amd64 / arm64 / 386) | **Supported**: CI build + full test suite |
| windows (amd64 / arm64) | **Supported**: CI build + full test suite |
| freebsd / netbsd / openbsd / dragonfly / solaris / illumos / aix / android / js / wasip1 | Not promised: compiles today (`GOOS=… go build ./...`, including the unsupported stubs) but no behavioral verification |
| plan9 | **Unsupported**: missing POSIX errno family plus fork/pipe semantics; the build fails explicitly |

Anything not listed counts as "not promised".
[`.github/workflows/ci.yml`](.github/workflows/ci.yml) only builds and fully tests the three
supported platforms.

## Testing

```sh
go test ./...
go vet ./...

cd contrib/jq && go test ./...    # contrib modules are independent: verify them separately
```

Some conformance cases need external oracles (`VSH_CONFORMANCE_RIPGREP`,
`VSH_CONFORMANCE_DIFF` pointing at pinned ripgrep / diff builds); when they are missing the
case reports itself as skipped — a skip is not a pass.

contrib modules are separate modules: when building or testing inside one while the checkout
sits in a larger Go workspace that does not `use` it, set `GOWORK=off` (the module's own
`replace ../..` still points at the local root module).

## Release

Three version surfaces:

| Object | Tag form | Notes |
| --- | --- | --- |
| Root module | `vX.Y.Z` | The shell engine itself; version in [VERSION](VERSION), changes in [CHANGELOG.md](CHANGELOG.md) |
| contrib modules | `contrib/<name>/vX.Y.Z` | Independent versions, all starting at **v0.1.0**, each with its own CHANGELOG |
| `examples/` | Not released | In-repo usage examples (own `go.mod`) |

Create tags in bulk with [`scripts/release-tags.sh`](scripts/release-tags.sh) (dry-run by
default; `--yes` creates local tags only — it never pushes):

```sh
scripts/release-tags.sh --list                  # current state: released version per module
scripts/release-tags.sh --contrib 0.1.0         # one version for all contrib modules (root untouched)
scripts/release-tags.sh --all 0.2.1             # root + all contrib modules at the same version
scripts/release-tags.sh --only jq=0.1.1         # release a single module
```

**Version-following agreement** (especially important while v0.x): Go makes no compatibility
promise for `v0`, and a minor bump of the root module drags downstream `require`s up through
MVS. Therefore:

- root **minor** bump: every contrib module must be released in the same window with its
  `require github.com/veypi/vsh` pointing at the new root version;
- root **patch** bump: contrib modules may stay untouched if no reachable API changed;
- contrib modules follow their own SemVer (behavior breaks bump minor; while v0, compatibility
  is only promised within a minor).

Release order:

1. finalize `VERSION` and `CHANGELOG.md` for the root module, commit;
2. tag the root module;
3. point each contrib `go.mod` at the new root version and add its CHANGELOG entry;
4. `scripts/release-tags.sh --contrib <version>` (or `--only <name>=<version>`) to tag contrib;
5. push the tags; the Go module proxy picks them up on demand.

In-tree development relies on each contrib module's `replace github.com/veypi/vsh => ../..`;
`replace` is not part of a published module, so consumers get the released version named in
`require`.

## Origin and attribution

This project began from an Apache-2.0 open-source shell runtime
([gbash](https://github.com/ewhauser/gbash), baseline commit
`88728c5a0618cf8d8278a6602ae9e1cf05a2159d`) and has since been **independently maintained and
evolved** in this repository: it went through large-scale rewrites, pruning and original
design (package and module layout, command registration and parsing, FS/network
abstractions, virtual runtime boundaries, contrib modularization, an independent release process).
It no longer tracks upstream and promises neither compatibility nor regular rebases; security
fixes are handled here as needed.

Upstream attribution and the license text live in [NOTICE](NOTICE) and [LICENSE](LICENSE).

## License

Apache-2.0 — see [LICENSE](LICENSE).
