# Changelog

本仓版本从 0.1.0 开始记录（0.1.0 之前的代码基础与归属见 README「来源与署名」）。
版本号见 [VERSION](VERSION)，遵循 SemVer：破坏性变更进位 minor。

## 0.2.0 — 2026-10-07

### 变更（破坏性）

- **命令查找与注册**：外部裸名改为「先查 Registry、再查真实 PATH」，显式路径只访问
  真实文件；不再生成、识别、修复或清理命令 stub 文件。Registry 不再写文件 hash，
  也不产生虚假的 `/bin` 路径。
- **`commands` 包**：删除 `Unregister`——注册表由调用方持有，注册后不再支持反注册。
- **包结构**：`internal/runtime` 并入根包，删除镜像 Runtime/Session 层与两份
  Config/DTO。此前 `WithConfig` 未透传 `NativeExec` 会造成静默漂移，现在只有一条
  构造路径。
- **移除 `contrib/python`（`python` / `python3` 命令）**：vsh 全树不再依赖任何上游
  `github.com/ewhauser/*` 代码——此前唯一残留是 `contrib/python` → `ewhauser/gomonty`
  （绑定层背后是 MIT 的 `pydantic/monty` 解释器；内置原生库 6 个 target 约 250MB）。
  `extras` 的命令清单/测试同步收敛：`extras` 现在聚合
  awk / html-to-markdown / jq / sqlite3 / yq 五个命令。需要沙箱内 Python 时，按嵌入
  面接入官方运行时（或 CLI/MCP 服务），不在库内自带解释器。
- **移除 LLM 工具契约模块与对应示例**：删除 `contrib/bashtool`（面向 LLM 的 bash
  工具契约）与 `contrib/codingtools`（`read`/`edit`/`write` 工具契约），以及仅有的
  两个使用者 `examples/adk-bash-chat`、`examples/vsh-eval`。理由：vsh 的定位是进程内
  shell 引擎（+ 沙箱原语），不携带 agent 侧的工具/schema/prompt 契约；`codingtools`
  在仓内零引用，`bashtool` 只被这两个示例使用（并把 extras 及 5 个命令模块的依赖拖进
  使用者的构建图）。需要同类契约时由集成方在自侧实现。

### 新增

- 导出 `commands.BrokenPipe`：断管（EPIPE）判定收敛为单点，内建命令与自定义命令
  共用同一实现。
- **CI（`.github/workflows/ci.yml`）**：gofmt 必须为空、`go vet` + `go test ./...`、
  darwin/linux/windows × amd64/arm64 构建、`contrib/*` 与 `examples` 模块构建。
- 平台支持面写入 [README](README.md) 「平台支持」：darwin / linux / windows 为支持面；
  其余平台不承诺行为（当前普遍可编译，含 unsupported 桩）。

### 修复

- **重定向落盘错误不再静默**：虚拟文件系统在 Close 时落盘，此前 `Close()` 的错误被
  丢弃——配额拒绝或写回失败会被报成 exit 0（例如 `>>` 追加超限静默丢数据）。现在
  Close 失败终结语句并返回非零退出。
- **PATH 搜索**：修复 Registry 与真实 PATH 混排时的解析与回退顺序（含显式路径场景）。
- **`access` 内建**：root 身份按 POSIX 真实行为绕过权限位检查。
- **跨平台编译修复（此前只在 darwin/linux/windows 上编过）**：18 个 GOOS/GOARCH 目标
  现有 17 个可编译。修掉：`ulimit` 的 `Rlimit` 值类型（FreeBSD/DragonFly 是 int64）；
  `unix.Getpgrp()` 在 solaris/illumos 返回 `(int, error)`（根与 `host/` 各一对
  `*_solaris.go`）；`cmd/vsh-gnu` 的 `syscall.ETXTBSY`（改为 unix/非 unix 一对
  `errTextBusy`）；plan9 缺 `EFBIG`/`EXDEV`/`syscall.Stat_t`/`mkfifoAt`（errno 与桩
  补齐）；wasip1 缺 `unix.Timespec`/`syscall.Getrusage`（`Lchtimes` 抽成 `lchtimesAt`
  平台对，`times_*` 覆盖 wasip1）。**陷阱**：`*_js.go` 这类文件名后缀会隐含限定
  GOOS，build tag 里加 `wasip1` 不生效——已改为 `*_wasm.go`。plan9 作为明确不支持
  平台，在包边界给出自解释失败（缺 POSIX errno 族与 fork/pipe 语义）。
- **oracle 用例改为可跳过**：`VSH_CONFORMANCE_RIPGREP` / `VSH_CONFORMANCE_DIFF` 未设时
  grep/diff/rg 一致性用例从「失败」改为「跳过」（新增 `RequireNixRipgrepOrSkip` /
  `RequireNixDiffOrSkip`，设了但指错仍失败），提示文案去掉已剪枝的
  `./scripts/ensure-*.sh`——否则 CI 必红。

### 文档

- README 重写为独立库用法（嵌入 API、命令行、扩展点、contrib、examples、发布、已知边界）。
- 新增本变更记录（`docs/` 下的平台集成进度表已移除——那是集成方内部记录，不属库文档）。
- **文档全量清理**：examples 的 `make run-*`（上游 Makefile 已剪枝）改为可跑的
  `cd examples && go run ./<name> …`；`internal/conformance/README` 的 `make` 目标改为
  `VSH_RUN_CONFORMANCE=1 go test …`；AGENTS.md 同步 contrib 清单/独立发版/`GOWORK=off`
  并删去过时的 `mvdan/sh` 一条；SPEC.md 的包布局段改成本仓现状（去 `packages/`、
  `tests/`）；THREAT_MODEL 加状态行并去掉已剪枝面；SECURITY 加支持面声明；`.gitignore`
  清掉上游 `website/` 等条目；全仓 25 个未格式化文件统一 `gofmt`。
- **对外定位改为独立维护**：删除 `FORK.md`（改名/剪枝/同步策略等 fork 叙事），
  README 开头不再提出来源；来源与署名收拢到 README 末尾「来源与署名」节（含基线与
  “不再跟踪上游”的声明），各处引用同步（SPEC/THREAT_MODEL/AGENTS/SECURITY/CHANGELOG/
  contrib）——上游归属署名仍保留在 [NOTICE](NOTICE)（Apache-2.0 要求）。
- **README 拆语言**：`README.md` 保持中文（面向使用者、仓库主入口），英文版移入
  `README.en.md`；两份顶部互相引用、内容同步。`AGENTS.md` 保持 AI/贡献者向，并补文档
  语言约定、平台矩阵与发版约定。

### 发布

- **`contrib/*` 改为各自独立发版**（均自 `v0.1.0` 起，tag 形如 `contrib/jq/v0.1.0`）：
  只引入需要的那一个模块，其余工具的依赖不进构建图。每个模块新增 `CHANGELOG.md`，
  并把 `require github.com/veypi/vsh` 从占位 `v0.0.38`（仓外无法解析）改为真实版本
  `v0.2.0`；`extras` 对其它 contrib 的依赖同步指向 `v0.1.0`。
- 新增 [`scripts/release-tags.sh`](scripts/release-tags.sh)：根 tag / contrib 批量 tag /
  单模块 tag，默认 dry-run，只建本地 tag 不推送。
- README 新增 「发布」节：版本面、发布顺序与**跟版约定**（根模块升 minor → 所有 contrib
  在同一窗口内各发一版并把 require 指向新根版本）。
- `examples/go.mod` 的根与 contrib require 同步改为真实版本。

## 0.1.0 — 2026-09-29

- 首个版本：以 Apache-2.0 的开源代码库 gbash（基线 `88728c5`）为起点独立维护——
  module 与包名改为 `vsh`，剪掉不需要的 website / 打包 / CI 资产，保留 `contrib/`、
  `examples/` 与规格文档（SPEC.md、THREAT_MODEL.md、SECURITY.md、AGENTS.md）。
  来源与署名见 README「来源与署名」与 [NOTICE](NOTICE)。
