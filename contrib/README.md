# Contrib Commands

`contrib/` holds optional `vsh` extensions that should not inflate the core runtime module.

Rules for this directory:

- each direct child under `contrib/` is its own Go module
- contrib commands are opt-in and are not registered by `vsh.DefaultRegistry()`
- contrib is where heavyweight or niche commands live so the main `github.com/veypi/vsh` module stays small

Today that includes:

- `contrib/awk` for the optional sandboxed `awk` command
- `contrib/extras` for a convenience helper that builds a registry with the stable contrib commands enabled
- `contrib/htmltomarkdown` for the optional sandboxed `html-to-markdown` command
- `contrib/jq` for the optional sandboxed `jq` command and its JSON/query stack
- `contrib/nodejs` for the optional experimental sandboxed `nodejs` command backed by `goja` and a curated `goja_nodejs` allowlist; it is intentionally not included in `contrib/extras` yet as it is absolutely not secure in its current implementation, and its module-level design notes live in `contrib/nodejs/README.md`
- `contrib/sqlite3` for the optional sandboxed `sqlite3` command
- `contrib/yq` for the optional sandboxed `yq` command and its YAML/query stack

The former `contrib/python` module was removed in 0.2.0: `vsh` no longer depends on any upstream `github.com/ewhauser/*` code. If you need Python inside the sandbox, wire an official runtime in through the embedding surface (or an MCP/CLI service) instead of bundling an interpreter here.

Versioning rules:

- the root module is tagged `vX.Y.Z`
- each contrib module has its own version, tagged with its module path prefix, for example `contrib/jq/vX.Y.Z`; every contrib module starts at `v0.1.0`
- contrib modules must require real tagged `github.com/veypi/vsh` versions, never placeholders such as `v0.0.0` or `v0.0.38`
- contrib modules keep committed local `replace` directives so each module builds against the local checkout during development (those directives do not affect consumers)
- 跟版：when the root module bumps a minor version, every contrib module must release in the same window — point `require github.com/veypi/vsh` at the new root version, add a CHANGELOG entry, and tag it
- `scripts/set-root-version.sh <X.Y.Z>` rewrites the root requirement in every contrib module (and `examples/`)
- `scripts/release-tags.sh` creates the tags: `--root`, `--contrib`, `--all`, `--only <name>=<version>`, `--list` (dry-run by default, local tags only)
- publishing is a plain `git push` of those tags; 本仓不依赖任何发布自动化
