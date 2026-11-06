# Changelog

本模块是独立 Go 模块，版本与根模块 `github.com/veypi/vsh` 解耦（tag 前缀
`contrib/sqlite3/`）；当前依赖根模块 v0.2.0。发布与跟版约定见根仓 README「发布」节。

## 未发布

- **长查询可被取消**：连接打开后接上 `conn.SetInterrupt(ctx)`（`ncruces/go-sqlite3` 原生支持），ctx 取消（`bg kill` / exec 超时 / 断连）时打断正在执行的 SQL——此前长查询不理会取消，只能等它自己跑完（与 awk 同类缺陷：进程内命令对取消无感）。回归：无限递归 CTE 在 ctx 取消后实际中断。

## v0.1.0 — 2026-10-07

- 首个独立版本：提供可选的沙箱内 `sqlite3` 命令（基于
  `github.com/ncruces/go-sqlite3`）。
- 此前该包随根模块源码树维护、未单独发版；本版行为与拆分前一致。
