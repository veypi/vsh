# FORK.md — vsh 与上游 gbash 的关系

## 基线

- 上游：`github.com/ewhauser/gbash`
- 基线 SHA：`88728c5a0618cf8d8278a6602ae9e1cf05a2159d`（"Update module golang.org/x/text to v0.39.0 [SECURITY] (#843)"）
- LICENSE：上游即 Apache-2.0，本仓沿用（LICENSE 原样保留 + NOTICE 署名来源）

## 剪枝清单（拷入时删除的上游资产）

website/、packages/、docker/、scripts/（上游脚本；本仓 scripts/ 是 fork 自有）、tools/、
flake.nix、flake.lock、go.work、go.work.sum、pnpm-*、package.json、.npmrc、
.goreleaser.yaml、.pre-commit-config.yaml、.renovaterc.json5、.custom-gcl.yml、
.golangci.yml（依赖被剪的 tools/regexponce-gclplugin）、.fuzz-corpus-watermark.json、
.autoenv*、.agents/、.github/、Makefile、CLAUDE.md、CONTRIBUTING.md。

contrib/ 全量保留（jq 挂入需要）；examples/ 保留；上游文档（SPEC.md / THREAT_MODEL.md /
SECURITY.md / AGENTS.md / docs/AST_ROADMAP.md）保留作参考。

## 改名（scripts/rename.sh，可重跑、幂等）

```sh
./scripts/rename.sh            # 在仓库根执行
```

动作：① `github.com/ewhauser/gbash` → `github.com/veypi/vsh`（全部 go.mod / .go / md）；
② 裸标识符 `gbash`→`vsh`、`GBASH`→`VSH`、`Gbash`→`Vsh`；
③ 文件名/目录名含 `gbash` 的同步改名（`cmd/gbash` → `cmd/vsh` 等）。
`./docs` 目录豁免（design.md / todo.md 是平台自有文档），仅上游 `docs/AST_ROADMAP.md` 参与改名。
注意：第三方依赖 `github.com/ewhauser/gomonty`（contrib/python 等使用）**不是**改名对象，保留原样。

## 上游同步策略

不承诺定期 rebase，后续按自有路线分叉开发。安全修复按需 cherry-pick：

```sh
git remote add upstream https://github.com/ewhauser/gbash.git
git fetch upstream
git cherry-pick <sha>   # 冲突点预期集中在改名面，逐文件处理
```

## 分叉点（与上游语义不同之处）

1. **D14 registry 优先**（M1 落地）：`internal/shell/core.go` 的 `lookupCommand` 中，
   名字在 Registry 即整体短路（source = `"registry"`），提到 hash 缓存与 PATH 候选循环
   之前——同名真实文件不再 shadow 内建/平台命令。连带语义变化（测试已同步）：
   - registry 命令不再经过 PATH 解析 → 不写入 hash 表、hits 不增长；
   - PATH 只约束 registry 之外的裸名（真实文件）；空 PATH 不再禁用内建；
   - host 适配器 `RequireExecutableBit` 不再约束 registry 名（仍约束真实文件）；
   - trace 的 `ResolutionSource` 新增 `"registry"`；解析路径为 `builtinCommandDir` 拼接（`/bin/<name>`）。
   上游语义测试（`TestLookupCommandPrefersRealExecutableOverRegistryStub` 等 8 处）已按新语义反转/重写。
2. **D7 --help 全覆盖**（M1.3.2）：130 个内建全部原生应答 `--help`（退出 0 + 非空用法文本）。
   补登 20 个缺口：interp 层（true/false/test/[/pwd/complete/compopt/compgen）与 builtins 层
   （truefalse/test/help/umask/kill/compadjust/nl/rev/rmdir/readlink/sed/not_implemented 系列）。
   Spec 框架命令统一用 `Parse.AutoHelp: true` 接入自动渲染；`internal/runtime/help_coverage_test.go`
   常驻防回归。
3. **fork 适配**：`internal/shell/docs_prune_test.go` 在 website/ 缺失（已剪枝）时跳过遍历。

## 存量 config.yaml 行序迁移提醒（vbox 语义反转）

（3.6.1 核对结果，2026-09-24）**mbp**：`fs_rules` 仅 `rw:/var/run/docker.sock` 单行（无次序问题）；
`net_rules`/`ssh_rules` 空；`exec_allow` 空（无 shell/解释器，3.6.2 通过）；`fs_policy: deny` /
`net_policy: open` / `ssh_policy: deny` 不变——**无需改动**。用户确认「config 没啥要改的」。
win 侧 pending（机器不在线）。

迁移判读规则（供 win/后续机器用）：首命中生效 + cfg 组内反转（文件末行最先生效）；
旧「先宽行后窄行覆盖」写法在新语义下反转，需把窄行挪前；文件尾 permanent grant 追加的
`rw:` 行现在最先生效，注意是否意外压过前面的 `deny:` 行。

pod 侧 fsauth/netauth 从「后命中者胜（last-wins）」反转为「首命中生效（first-wins）」
（见 design.md §11.5）。**开发者机器（mbp / win）上的存量 config.yaml `fs_rules`
行序在两种语义下含义相反**——切换 vbox 前必须人工过一遍（todo M3.6.1）。

## Windows PATH 分隔符（M1.3.1 核实结论）

`internal/runtime/layout.go` 的 `commandDirectoriesForPath` 无条件按 `:` 切分 PATH——
原生 Windows PATH（`C:\...;D:\...`）会被盘符冒号切碎。**结论：fork 内不改切分逻辑**；
约束落在 glue（aic-pod libs/vsh fs_host）——必须向引擎呈现 unix 风格虚拟路径
（PATH 钉 `{session_root}/{sid}/bin` 等不含盘符的虚拟路径，todo 2.3.3），引擎永远看不到
原生 Windows PATH。设备级实证随 M2 glue 的 win 冒烟一并验收（design M1.3.1 的设备实证部分
移至 M2.3.3 对齐）。

## 已知环境性测试失败（非代码问题，2026-09-23 mbp 实录）

- `TestDiffMatchesGNUDiff`：需 GNU diffutils 3.12（上游经 Nix 钉版，脚本已随剪枝删除）；
  设置 `VSH_CONFORMANCE_DIFF` 指向 GNU diff 后运行。ripgrep oracle 用
  `VSH_CONFORMANCE_RIPGREP=$(which rg)`（homebrew rg 15.1.0 实测通过）。
- `TestFieldsGlobIgnoreCharClass`（shell/expand）：测试写 `.env` 文件被 aic-pod 沙箱
  deny 策略拦截——无沙箱环境下通过。
- `contrib/jq TestJQSupportsStreamErrorsMode`：上游原检出在 go1.27 下同样失败
  （gojq 错误消息列号漂移），非 fork 引入。
- `examples/` 模块构建：依赖 grpc 测试数据含 `.key` 文件，沙箱禁止解压写入——
  无沙箱环境或预填模块缓存后可构建。根模块 + 10 个 contrib 模块构建全绿。

## Session.Exec 串行化与单命令死循环防线（2026-09-24 实测定案）

`internal/runtime/session.go`：`Session.Exec` 对整个执行持有 `s.mu`——同会话串行。
**fork 内不改**（并发 shell 共享 env/cwd/fd 语义混乱，上游有意如此）；缺口在 glue
（aic-pod libs/vsh）两层化解：

1. `bg run` 走派生会话（`sid#bg-*`）——不与前台互等（同会话 bg 会死锁）。
2. **管理面快路径**：纯 `bg list/wait/kill/output`（或裸 bg）单命令脚本路由派生会话
   （`sid#mgmt-*`，`isPureBGMgmtScript` 保守判定：单语句/全字面量/无重定向管道）。
   实测漏洞：前台长任务超时转 bg 后仍持基会话锁，救场的 `bg kill` 排在锁后到不了
   执行层，会话活锁至 30min 墙钟。`bg run` 刻意排除（cwd 继承依赖基会话 FS 状态）。

单命令死循环（`yes > /dev/null` 类）防线边界：解释器 limits（MaxCommandCount 等）
只在命令间触发，管不到单个命令内部；兜底 = TaskTable 30min 墙钟（到期 124）+
bounded output。命令级 CPU 硬上限不可行（Go 无 per-goroutine CPU 计量；墙钟会误杀
合法 `sleep`）——维持 30min 有界 + 管理面可杀的设计。
