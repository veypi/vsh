# Repository Guidelines

## Project Overview
`vsh` is a Go workspace for a deterministic shell runtime. `contrib/` houses optional heavyweight modules (`sqlite3`, `jq`, `yq`, `extras`). Read `SPEC.md` before changing runtime boundaries or sandbox behavior.

## Build & Test
Use Go 1.26+. The fork removed the upstream Makefile, Nix flake and oracle download scripts. From this module run:

```sh
go build ./...
go test ./...
go vet ./...
```

Before submitting or updating a PR, run `go vet ./...` and fix reported issues. Optional contrib modules have their own go.mod; compile and test the modules used by the consuming application separately.

`TestDiffMatchesGNUDiff` and the ripgrep oracle tests require the exact versions checked by the test. Set `VSH_CONFORMANCE_DIFF` and `VSH_CONFORMANCE_RIPGREP` to those binaries. When an oracle is unavailable, explicitly exclude that test and report the gap; do not claim full conformance. The upstream Nix-based conformance/bats commands are not available in this fork.

## Key Project Rules
- Unknown commands must never fall through to the host OS.
- Match the registry pattern in `commands/` when adding new built-in commands.
- For runtime changes, test exit codes, stdout/stderr, and sandboxed filesystem effects.
- If a change touches shell semantics or policy, add a regression test in `runtime/` or the relevant package.

## SPEC Sync
`SPEC.md` is the product and architecture contract. Update it in the same turn when:
- adding/removing built-in commands
- changing sandbox guarantees, policy defaults, or filesystem abstractions
- changing `mvdan/sh` integration strategy
- expanding scope, roadmap, or introducing new public packages/interfaces

Read the relevant `SPEC.md` sections before editing code, and update them once the design is clear. When in doubt, prefer a small SPEC update over silent drift.

## Commits & PRs
Use short, imperative subjects scoped to one change (e.g., `runtime: normalize command-not-found errors`). PRs should explain user-visible behavior, note any SPEC updates, include trace/CLI output when changing execution behavior, and only be submitted after the build, applicable tests and `go vet ./...` pass.
