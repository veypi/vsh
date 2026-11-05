# Changelog

本模块是独立 Go 模块，版本与根模块 `github.com/veypi/vsh` 解耦（tag 前缀
`contrib/awk/`）；当前依赖根模块 v0.2.0。发布与跟版约定见根仓 README「发布」节。

## 未发布

- **awk 可被取消，无进展时自限（堵住「一个脚本烧满一个核 5 天」）**：解释器此前走 `interp.ExecProgram`（不带 ctx），`bg kill` / exec 超时 / 断连的取消信号到不了 VM。2026-10-31 生产事故：用户脚本的 while 循环索引写错（`n=n+n0`，`n0` 未定义=0），对着 3.8MB 单行 XML 无限循环；该 exec 以 background 方式运行，`bg kill` 回报 "killed"，解释器 goroutine 却仍占满一个核（累计 119 CPU 小时、持续 5 天）。现在：①解释器改走 `interp.ExecuteContext`（VM 每 1000 条指令查一次 ctx），取消真正生效；②新增停滞检测——输入无新字节、输出无新字节持续超过阈值（默认 120s；`VSH_AWK_MAX_STALL` 以秒覆盖，0=关闭）即判定「无进展」并中止，报 `awk: stalled (no input/output progress for 2m0s — possible infinite loop)`。进展信号 = 读入字节 + 写出字节（含 `print > "file"`）。回归：`stall_test.go`（ctx 取消 0.3s 内中止、死循环 1s 内自限、正常脚本不受影响、阈值可关闭）。

## v0.1.0 — 2026-10-07

- 首个独立版本：提供可选的沙箱内 `awk` 命令（纯 Go 实现）。
- 此前该包随根模块源码树维护、未单独发版；本版行为与拆分前一致。
