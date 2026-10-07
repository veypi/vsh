# Changelog

本模块是独立 Go 模块，版本与根模块 `github.com/veypi/vsh` 解耦（tag 前缀
`contrib/extras/`）；当前依赖根模块 v0.2.0 与 awk/htmltomarkdown/jq/sqlite3/yq
各自的 v0.1.0。发布与跟版约定见根仓 README「发布」节。

## v0.1.0 — 2026-10-07

- 首个独立版本：聚合注册稳定的可选命令（`awk`、`htmltomarkdown`、`jq`、`sqlite3`、`yq`），
  并提供 `FullRegistry()`。
- 依赖上述 5 个 contrib 模块的 v0.1.0；`nodejs` 等实验性命令不在其中。
- 原 `contrib/python`（`python` / `python3`，底层是上游 `gomonty`）不再存在——
  vsh 全树不再依赖任何 `github.com/ewhauser/*` 代码。
- 此前该包随根模块源码树维护、未单独发版；本版行为与拆分前一致。
