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
SECURITY.md / AGENTS.md）保留作参考。

## 改名（scripts/rename.sh，可重跑、幂等）

```sh
./scripts/rename.sh            # 在仓库根执行
```

动作：① `github.com/ewhauser/gbash` → `github.com/veypi/vsh`（全部 go.mod / .go / md）；
② 裸标识符 `gbash`→`vsh`、`GBASH`→`VSH`、`Gbash`→`Vsh`；
③ 文件名/目录名含 `gbash` 的同步改名（`cmd/gbash` → `cmd/vsh` 等）。
`./docs` 目录豁免（todo.md 是平台自有文档）。
注意：第三方依赖 `github.com/ewhauser/gomonty`（contrib/python 等使用）**不是**改名对象，保留原样。

## 上游同步策略

不承诺定期 rebase，后续按自有路线分叉开发。安全修复按需 cherry-pick：

```sh
git remote add upstream https://github.com/ewhauser/gbash.git
git fetch upstream
git cherry-pick <sha>   # 冲突点预期集中在改名面，逐文件处理
```

## 当前架构差异（2026-10-03）

- 解释器 builtin 与 Registry 分开。外部裸名先查 Registry，再查真实 PATH；显式路径只查文件。不再生成、识别、修复或清理 stub。
- 新的 NativeExec 回调只接受已经解析的真实二进制路径，默认不提供。vsh 不依赖 vbox，也不直接启动宿主程序。
- type/which/command/hash/补全与嵌套执行共用解析。Registry 不写文件 hash，不产生虚假 /bin 路径。状态性 complete/compopt/compgen 只留解释器实现。
- 默认 memory FS factory 准备 home/tmp；自定义 factory 自己准备 cwd。Runtime 和 Session 不根据 HOME/PATH/命令变化维护布局。
- 移除 VSH_COMPAT_ROOT 和 VSH_COMPAT_PHYSICAL_PWD 的路径投影；测试与集成使用真正的 FS namespace。
- 原生 --help 应答与已有 shell variant 能力保留；具体边界见 SPEC.md。

## 集成边界

pod/cloud 的共享集成是 aic-pod/libs/execution：管理执行句柄、等待和 bg，并仅注册本端可用的平台命令。host 在已解析文件出口做 exec_rules 判定后交 vbox；cloud 不注入 NativeExec。service 生命周期属于 skillrun，不占 bg 配额。

Windows 的 PATH 在 host 环境入口转换为冒号分隔的 /c/... 形态；程序路径、cwd 和路径环境变量在 OS 出口统一还原。vsh 不解析原生分号 PATH。

Session.Exec 仍串行化同一会话的 shell 状态。前台等待结束后可以由 execution 托管同一执行；bg list/wait/kill 管理已有句柄，不另起一条执行。进程级硬资源限制由 vbox/部署环境负责。

## 本仓验证入口

上游 Makefile、Nix flake 和 oracle 下载脚本已随 fork 剪枝。根模块用 go test ./... 与 go vet ./...；contrib 是独立模块，按 go.work 分别验证。ripgrep/diff oracle 需要分别指定 VSH_CONFORMANCE_RIPGREP、VSH_CONFORMANCE_DIFF，缺少固定版本时应报告未验证，不能把跳过当通过。

已知基线失败：`contrib/jq TestJQSupportsStreamErrorsMode` 在 Go 1.27 下的错误文本及列号断言不一致，上游原检出也会失败。本次架构调整未修改 jq 实现或该断言；2026-10-03 复验仍能重现。这个问题与命令解析分派无关，不计作本次回归通过项。
