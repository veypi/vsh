# Repository Guidelines

Notes for AI agents and contributors working **in this repository**. The user-facing
overview is [README.md](README.md) (Chinese) / [README.en.md](README.en.md) (English).

## Project Overview
`vsh` is a Go module for a deterministic, embeddable virtual-shell runtime. It owns script parsing, command resolution, uniform command admission and virtual runtime resources; it does not provide an OS/process sandbox. Optional heavyweight commands live in separate modules under `contrib/` (`awk`, `htmltomarkdown`, `jq`, `sqlite3`, `yq`, plus the experimental `nodejs` and the `extras` umbrella). Read `docs/reference/spec.md` before changing runtime or command-policy boundaries.

## Build & Test
Use Go 1.26+. 本仓不含上游的 Makefile、Nix flake 与 oracle 下载脚本。From this module run:

```sh
go build ./...
go test ./...
go vet ./...
```

Before submitting or updating a PR, run `go vet ./...` and fix reported issues. Optional contrib modules have their own go.mod and their own version/tag (`contrib/<name>/vX.Y.Z`); from inside such a module use `GOWORK=off` when the checkout sits inside a larger Go workspace, and build/test the modules used by the consuming application separately. CI (`.github/workflows/ci.yml`) runs gofmt, `go vet`/`go test ./...`, the three supported platform builds, and the contrib/examples module builds.

`TestDiffMatchesGNUDiff` and the ripgrep oracle tests require the exact versions checked by the test. Set `VSH_CONFORMANCE_DIFF` and `VSH_CONFORMANCE_RIPGREP` to those binaries; when they are unset the case skips itself (`testutil.RequireNixDiffOrSkip` / `RequireNixRipgrepOrSkip`). When an oracle is unavailable, explicitly exclude that test and report the gap; a skip is not a pass, so do not claim full conformance.

## Key Project Rules
- Unknown commands must never fall through to the host OS.
- Every resolved command uses the same `exec_policy` admission point before its implementation runs. Do not add separate admission paths for builtins, registered commands or external-program adapters.
- Command implementations hide their own execution details and implementation-specific policies; the shell runtime does not branch on those details.
- vsh provides a virtual shell, not an OS sandbox. Do not claim that its in-process filesystem, network or resource controls isolate a native child process.
- Match the registry pattern in `commands/` when adding new built-in commands.
- For runtime changes, test exit codes, stdout/stderr, and sandboxed filesystem effects.
- If a change touches shell semantics or policy, add a regression test in `runtime/` or the relevant package.

## Platform Matrix
- Supported surface: darwin / linux / windows (CI builds and runs the full suite). Anything else is unverified: it may compile today, but do not claim behavior for it.
- `plan9` must keep failing explicitly at the package boundary (missing POSIX errno family and fork/pipe semantics) — the sentinel files are `unsupported_plan9.go` / `unsupported_wasm.go`.
- A `*_js.go` filename implies `GOOS=js`; for wasip1 use `*_wasm.go` plus an explicit build tag, otherwise the build tag alone changes nothing.

## Docs & Language
- `README.md` is **Chinese** and is the repo's main, human-facing entry point; `README.en.md` is the English mirror. Both open with a language switcher line and must be updated together when user-visible behavior changes.
- Unified layout (shared across the ivec repos): READMEs, `CHANGELOG.md`, `AGENTS.md`,
  `SECURITY.md` and `VERSION` at the root; everything else under `docs/` with
  `docs/index.md` as the map and `architecture/` / `reference/` / `guides/` /
  `development/` as the only categories.
- `docs/reference/spec.md` is the product/architecture contract and stays Chinese; README
  links to it and to `docs/reference/threat-model.md` / `SECURITY.md` as the normative docs.
  Completed changes go to `CHANGELOG.md`; future plans only to `docs/development/todo.md`.
- Keep documentation limited to the generic virtual-shell library. Do not document particular embedding products, process-sandbox libraries or their configuration domains here.
- Keep the READMEs for users: what it is, highlights, install, quickstart, CLI, extension points, contrib table, known limits, platform matrix, testing, release, attribution, license. Editorial/internal detail (verification workflow, module traps, invariants) belongs in this file.
- This library is **independently maintained**: do not reintroduce upstream/fork framing or a `FORK.md`. Attribution lives in the README's final "Origin and attribution" section plus `NOTICE` / `LICENSE`.

## SPEC Sync
`docs/reference/spec.md` is the product and architecture contract. Update it in the same turn when:
- adding/removing built-in commands
- changing sandbox guarantees, policy defaults, or filesystem abstractions
- changing the in-tree shell core integration strategy (`shell/`, `internal/shell/`)
- expanding scope, roadmap, or introducing new public packages/interfaces

Read the relevant `docs/reference/spec.md` sections before editing code, and update them once the design is clear. When in doubt, prefer a small SPEC update over silent drift.

## Release & Versioning
- Root module: tag `vX.Y.Z`, version in `VERSION`, changes in `CHANGELOG.md`.
- contrib modules: tag `contrib/<name>/vX.Y.Z`, each starting at **v0.1.0** with its own `CHANGELOG.md`; `examples/` is not released.
- `scripts/release-tags.sh` lists/creates tags (dry-run unless `--yes`; **local tags only, never push**); `scripts/set-root-version.sh` updates the root version in the contrib `go.mod` files.
- Version-following: a root **minor** bump requires every contrib module to be released in the same window with its `require github.com/veypi/vsh` pointed at the new root; a root **patch** bump may leave contrib untouched. contrib modules follow their own SemVer.
- In-tree development relies on `replace github.com/veypi/vsh => ../..` inside each contrib module; `replace` never reaches consumers.

## Commits & PRs
Use short, imperative subjects scoped to one change (e.g., `runtime: normalize command-not-found errors`). PRs should explain user-visible behavior, note any SPEC updates, include trace/CLI output when changing execution behavior, and only be submitted after the build, applicable tests and `go vet ./...` pass.
