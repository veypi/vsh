# Changelog

本模块是独立 Go 模块，版本与根模块 `github.com/veypi/vsh` 解耦（tag 前缀
`contrib/jq/`）；当前依赖根模块 v0.2.0。发布与跟版约定见根仓 README「发布」节。

## v0.1.0 — 2026-10-07

- 首个独立版本：提供可选的沙箱内 `jq` 命令（基于 `github.com/itchyny/gojq`）。
- 此前该包随根模块源码树维护、未单独发版；本版行为与拆分前一致。
- 已知问题：`TestJQSupportsStreamErrorsMode` 在 Go 1.27 下失败（错误文本与列号断言
  差异），属继承自最早基线的既有失败；未设 `VSH_CONFORMANCE_*` 的 oracle 用例会自动 skip。
