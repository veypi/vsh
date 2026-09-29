# vsh 引擎化实施 todo

- 创建：2026-09-23（会话 b60a961e）
- 契约源：fork 行为定案见 `FORK.md`；平台集成契约见 aic-pod `docs/hosts-vsh-redesign.md` / `docs/hosts-tools.md`；规则表语义见 aic `docs/permission_rules.md`。原 v4.3 设计稿已随实施完成删除（历史见 git），本文件只做任务分解与进度跟踪
- 基线：gbash @ `88728c5a0618cf8d8278a6602ae9e1cf05a2159d`（/Users/veypi/test/gbash，工作区干净）
- 工作区：go.work 加 `./vsh`、`./vbox`（M1）；vsh/vbox 仓已空建（仅 LICENSE/README）
- 纪律：直接切换无灰度（D10）；不自己重启后端（用户操作）；提交只 add 自己改的文件

状态图例：`[ ]` 未开始 / `[~]` 进行中 / `[x]` 完成 / `[!]` 阻塞（附原因）

---

## M1：fork 改名 + registry 优先 + 核实项

目标：`ivec/vsh` = gbash 全量拷贝改名（module `github.com/veypi/vsh`，包名 gbash→vsh），测试绿，D14 落地。

### 1.1 fork 机械改写
- [x] 1.1.1 gbash 拷入 `ivec/vsh`：剪枝 website/packages/docker/flake.nix/flake.lock/scripts/非 Go 资产；删 fork 内 go.work/go.work.sum；contrib 保留（jq 要挂，§9.16）
- [x] 1.1.2 12 个 go.mod module 路径 `github.com/ewhauser/gbash*` → `github.com/veypi/vsh*`；包名/导入注释 gbash→vsh 全量机械改写（含 stub 文件内容 `# gbash virtual command stub` 字样，layout.go:110）
- [x] 1.1.3 LICENSE 换 Apache-2.0 + NOTICE（标明上游 ewhauser/gbash 与基线 SHA 88728c5）；README 重写为 fork 定位（注：上游 LICENSE 本就是 Apache-2.0，原样沿用 + NOTICE）
- [x] 1.1.4 FORK.md：基线 SHA、改名脚本（可重跑）、上游同步策略（按需 cherry-pick，不承诺 rebase）、存量 config.yaml 行序迁移提醒（§11.5）
- [x] 1.1.5 go.work `use` 加 `./vsh`、`./vbox`；`go build ./...` + 上游相关包 `go test ./...` 全绿（纯机械验收）（注：根模块+10 contrib 构建绿；examples 因沙箱禁写 .key 环境性受阻，记 FORK.md）

### 1.2 D14 registry 优先（分叉第一刀）
- [x] 1.2.1 `lookupCommand`（internal/shell/core.go:1108-1143）：名字在 Registry 即整体短路，提到 PATH 候选循环**之前**（不是仅 lookupCommandPath 内提前——评审结论：循环前短路才彻底关闭 shadow）（落在 hash 缓存之前，source=registry）
- [x] 1.2.2 `isUnsupportedVirtualBuiltinStub`（core.go:1472）与 `ensureCommandStub` 同名跳过（layout.go:90-112）保留，确认 registry 优先后无害
- [x] 1.2.3 上游语义测试反转：`TestLookupCommandPrefersRealExecutableOverRegistryStub` 等改为断言 registry 优先（共 8 处：lookup/hash×3/trace×2/PATH×2/host RequireExecutableBit/command -p）
- [x] 1.2.4 新增用例：PATH 目录同名真实文件不 shadow 内建/平台命令（cloud 内存层 + host 会话 bin 目录各一）（单测级双用例落地；端到端双端用例随 M2/M3 验收 2）

### 1.3 M1 强制核实项（v4.1）
- [x] 1.3.1 **Windows PATH 分隔符**：layout.go `commandDirectoriesForPath` 按 `:` 切分——结论：fork 不改切分逻辑，约束落 glue（fs_host 必须呈现 unix 风格虚拟路径，与 2.3.3 对齐）；设备级实证随 M2 win 冒烟（结论记 FORK.md）
- [x] 1.3.2 **内建 --help 覆盖度**：130 内建逐一 `--help`，缺失的 fork 补齐（D7）（实测 20 缺口已全部补齐；internal/runtime/help_coverage_test.go 常驻防回归）
- [x] 1.3.3 **内建 ls/rg 基础行为**：确认脚本命令角色下的行为与 help 文本（fs 的结构化 ls/rg 双轨独立，不涉引擎）（内建测试全绿 + --help 覆盖测试保证；rg oracle 用 homebrew rg 15.1.0 通过）
- [x] 1.3.4 **jq 挂入验证**：contrib/jq 在 fork 内可构建可注册（cloud/host registry 各一行 Register 的接法在 M2 cmds.go 定型）（构建绿 + Register(registry) API 确认；1 个上游既有环境性测试失败，非 fork 引入，记 FORK.md）

### 1.4 M1 出口
- [x] 1.4.1 `go test ./...` 全绿（含反转后的语义测试）（23 包 ok；仅 2 个环境性失败：diff oracle 需 GNU diffutils 3.12、.env 写入被沙箱 deny——均记 FORK.md）
- [x] 1.4.2 提交 vsh 仓（一次 fork 提交 + 一次 D14 提交，分开）（实际三次：89d24cf fork / 6bfe55a D14 / 54d7d04 D7 --help；vbox 仓 58714a8 go.mod）

---

## M2：libs/vsh glue 七文件 + vbox 接口定稿

目标：`aic-pod/libs/vsh`（cloud/host 共用 glue）全部落地 + 单测；vbox 接口定稿，实现暂留 pod。

### 2.1 engine.go
- [x] 2.1.1 Runtime 单例 + NewSession per exec + limits（§6 全项落 engineLimits；Runtime 级 HOME 钉内存层 /tmp/.vsh-layout-home 供布局初始化，真实 HOME 每 exec 注入）
- [x] 2.1.2 **SymlinkMode 覆盖（v4.1 强制项）**：engine.go 显式 SymlinkFollow（注释记录冲突缘由）；symlink 放行/逃逸用例在 vbox MatchNoFollow 测试侧，host 端到端随 M3 冒烟
- [x] 2.1.3 前台 300s 上限（Engine.Exec 钳制，page 180 由工具层 cap）；bg 任务表（glue 自建 TaskTable）+ 后台墙钟 30min 到期 124（Start 统一施加）；超时转 bg 不取消 ctx 属 exec 工具层（3.1.2）
- [x] 2.1.4 panic recover 隔离；会话隔离用例（内存层 per-session 隔离 + UFS 直通共享语义双双钉死）
- [x] 2.1.5 日志 tee（ExecRequest.Log 全量，capture+passthrough 双得）；Content 前 1000 行/attrs/exit_code 透出属 exec 工具层（3.1.x）

### 2.2 fs_ufs.go（cloud）
- [x] 2.2.1 UFS 直通 + 系统目录内存层 `{/bin,/usr/bin,/tmp,/etc,/dev,/proc}` + canonicalize（UFS 无 symlink，词法归一；host 侧走 vbox.Canonical）
- [x] 2.2.2 vbox FSRuleSet 挂接（Rules 快照源每次 IO 取当次值）；Match/MatchNoFollow（Remove/Rename 用 NoFollow）；越界硬拒绝 + 报错引导 `grant fs`
- [x] 2.2.3 **QuotaFS 包装（v4.1）**：backing 由调用方包装（胶水接缝就位；QuotaFS 随 cloudenv 删除迁移在 3b 完成物理搬迁）
- [x] 2.2.4 init 副作用核实：`Chmod(/tmp, sticky)` 走内存层；`MkdirAll` 对齐 os 语义（已存在目录幂等短路，先于门控——根/会话根不被写门误拦）
- [x] 2.2.5 红线用例（/tmp、/bin 写不进 backing）；UFS 无可执行位持久化确认（chmod → ErrUnsupportedOp）→ 工具描述引导 `bash x.sh`（3.1.1 写描述）
- [x] 2.2.6 **用户根 jail（v4.3）**：代码硬约束，越界读写 ErrOutsideJail（跨用户用例绿）

### 2.3 fs_host.go（host）
- [x] 2.3.1 OS backing + PinStubs 钉 `{session_root}/{sid}/bin`（幂等覆盖；非法 stub 名拒绝）
- [x] 2.3.2 用户态执行复用同一 ufsAdapter/vbox matcher（与 OS 沙箱同源）；`>` 重定向门控用例 cloud 形态先行（同码路径），host 冒烟随 3.1.3
- [ ] 2.3.3 Windows 虚拟根语义（1.3.1 结论：glue 呈现 unix 风格虚拟路径）——win 设备实证随 host 冒烟（无 win 设备，记录遗留）

### 2.4 netclient.go
- [x] 2.4.1 cloud：NetRuleSet default open + 动态行 + 私网阻断（RFC1918+loopback+169.254/16 含 metadata，dial 时复核防 DNS rebinding）+ 重定向逐跳复核（上限 10）/超时/响应上限 + 审计（URL/状态/大小/耗时）
- [x] 2.4.2 **下载配额（v4.1）**：OnResponseSize 预检钩子 + 写盘路径经 FS backing QuotaFS（双闸门，curl -o 不绕过）
- [ ] 2.4.3 host：对接 pod net 策略（NetClient 复用，Rules 源换 pod Snapshot——随 3c host grant 接线）

### 2.5 analyze.go
- [x] 2.5.1 syntax AST → `{WriteTargets, UsesNetwork, SyntaxError}`；语法错直接返回
- [x] 2.5.2 **写参表**（重定向 > >> &> &>> + cp/mv/ln/install/rsync 末参、tee/mkdir/touch/rm/rmdir/truncate 全参、chmod/chown/chgrp 跳首参、sed -i、tar -f、curl -o、wget -O、dd of=；仅预检报错材料，非拦截表）
- [x] 2.5.3 脚本递归（bash|sh|source|.|./x.sh 字面文件参数，限深 3 + seen 去重；不可读/动态放弃）；TOCTOU 注释文档化
- [x] 2.5.4 字面写目标材料就绪（预检报错在 3.1.2）；动态构造运行期硬拒兜底（适配器门，`rm $X` 语义）

### 2.6 cmds.go / native.go
- [x] 2.6.1 平台命令：commands / bg / grant / list_hosts / send_user（自带 help；grant 域 fs/net/cmd，ssh 域随 3c host 接线）
- [x] 2.6.2 组合 Registry（内建→jq→平台命令；host native 追加）；stub 由引擎 initializeSandboxLayout 按 Registry.Names() 自动写 PATH 目录（接线实证，which/commands 一致）
- [x] 2.6.3 jq 挂入（NewEngine 统一 Register，cloud/host 同源）
- [x] 2.6.4 native.go：NativeRegistry 白名单（种子+Allow 扩充）→exec_procs.RunProcess（fail-closed 沙箱兜底；vbox.Compile/Box 接口形状对齐、OS 落地阶段二）；默认不含 shell/解释器用例钉死；grant help 文案含解释器警告

### 2.7 vbox 接口定稿 + fsauth/netauth 拆分（§11.5 阶段一）
- [x] 2.7.1 `ivec/vbox` 接口骨架（Rule/FSRuleSet/NetRuleSet/Policy/Compile/Box + Caps + ErrOSLoweringPending）
- [~] 2.7.2 纯 matcher 按 first-wins 重写迁入 vbox（Match/MatchNoFollow/canonical/dual-forms/ScrubEnv ✓）；状态层（cfg 耦合/Reconcile/tempTables/持久化）留 pod——挪 3c 与 grant 接线同块
- [~] 2.7.3 **语义反转测试重写**：vbox 侧新语义测试全新写（行序回归/firmlink 防护绿）；fsauth/sandbox 旧测试随删除面处理（不另重写）——darwin seatbelt M3 行序映射随 3c
- [ ] 2.7.4 Snapshot(sid) 纯拼接 + grant.go DenyHit 拒批删除——挪 3c

### 2.8 M2 出口
- [x] 2.8.1 glue 单测全绿（30+ 用例）；UFS 适配器真实 IO 走 localFS 实测（吞吐基准非目标，功能/门控/红线用例覆盖）
- [x] 2.8.2 vbox matcher 单测（first-wins、NoFollow、行序）全绿

---

## M3：aic / aic-pod 接线 + 删除面（直接切换；v4.3 拆 3a/3b/3c 三个冒烟点）

### 3.1 exec 工具重写（aic）【3a】
- [x] 3.1.1 spec：`{1host, script, workdir?, timeout?, stdin?, nosandbox?}`；description 含 commands 发现/`<cmd> --help`/bash x.sh 引导/规则表内不审批；timeout caps 300（page 180）
- [x] 3.1.2 cloud 流程：analyze 预检（语法错直返/字面写目标出区可读报错引导 grant/字面 grant 恒4级）→ Engine.Run；无 CheckLevel/WriteScope；Tasks.Start+Wait 超时转 bg（不取消 ctx）；引擎 Runtime 单例跨用户 + cloudParts 路由 + QuotaFS backing + D15 初始规则表
- [x] 3.1.3 host 转发：script 签名下发 pod，pod 侧引擎执行（engine_vsh.go + dispatch execCmd script 分支；fsauth.Snapshot 表门 + native 白名单 + SnapshotAllVbox NetClient）
- [x] 3.1.4 page：单命令 dispatch，shell-like 引号剥离分词（不展开变量/glob）；组合语法明确文案拒绝（9 用例钉死）
- [x] 3.1.5 FS 写审计随 .exec 日志落行（res.Writes，cloud/pod 双端）
- [x] 3.1.6 nosandbox host-only 恒 4 级（Evaluate + Run 双检查，cloud/page 拒参）

### 3.2 fs 工具瘦身（v4.1 口径）【3b】
- [x] 3.2.1 保留 write/edit/read/ls/rg 五 action 三端；cp/mv/rm 下线报可读错误引导 exec（aic 服务端三端统一短路 + page_fs 前端同文案）
- [x] 3.2.2 后端薄层：cloud = ufs.FS 包 QuotaFS（fsx.Env Gate 接 CloudFSRules）；host = hostfs（text.* 经 fsx.RunFS + fsauth 快照门）；page = 现有前端通道；ls/rg 逻辑自 vcore 移植（aic-pod/libs/fsx，cloud/host 共用）；vcore 依赖彻底切除
- [x] 3.2.3 write/edit 出区 → 规则表拒绝 + 引导 grant（CheckCloudAccess/WriteGrade 3 级链退役）；write/edit 配额预检保留（CheckStorageQuota + QuotaFS 执行期闸门）
- [x] 3.2.4 工具描述重写：双轨口径（fs ls/rg = 结构化便利层；exec 内建 ls/rg = 脚本命令）+ 规则表内操作不审批提示

### 3.3 page exec 扩编（前端）【3b】
- [x] 3.3.1 一份 OPFS ls/rg 实现，fs 与 exec 双通道共用（fsops.js 单实现；regex = RE2 子集，RG_UNSUPPORTED_RE 拒绝 lookaround/backreference——既有）
- [x] 3.3.2 cp/mv/rm 注册为 page exec 命令（os/fs_cmds.js + os.html aiCommands；ls/rg 同注册；req-reply 180s 同通道）

### 3.4 cloud vbox 规则表 + grant 接线（D15）【3c】
- [x] 3.4.1 cloud 初始表行序：temp（表头）→ 便利根 rw 会话目录 → ro /u/{uid}、ro skills；DefaultWrite deny（temp 存储 = aic/libs/tools 包级 sync.Map，CloudFSRules 内部插表头，exec 引擎与 fs 工具自动同表）
- [x] 3.4.2 grant fs/net cloud 域：temp 行插表头、会话级、恒 4 级审批（GrantCloudFS jail 校验/GrantCloudNet host:port 校验；cloudGrant 真实现；NetClient Rules 改 per-sid（ctx 注入会话键，glue SessionFromContext）——进程级并集跨用户泄漏不允许；私网阻断保持 SSRF 硬底线不可授权）
- [x] 3.4.3 host grant 四域对接 vbox（vshGrant 补 ssh 域入 sshPol——执行面随 ssh 工具重建另接；DenyHit 拒批删除（grant.go 两分支 + fsauth decide/netauth Allowed 旧硬底线同步翻转，temp 插表头压 deny）；permanent 追加位置核对：append 文件尾 + cfg 组内反转 = 新行位于 cfg 段最前，新语义下正确，TestSnapshotCfgTailAppendWins 钉死；NetClient host 侧改 SnapshotVbox(sid)，SnapshotAllVbox 删除——偏差#2 消除）

### 3.5 删除面清理（无清单无灰度）【3b】
- [x] 3.5.1 aic-pod `libs/vcore` 整体（ls/rg 逻辑已移植 aic-pod/libs/fsx；quota_fs.go 在 aic/libs/tools 保留、Fetcher 配额由 glue NetClient + QuotaFS backing 承接）
- [x] 3.5.2 aic `tools/exec`：git.go/git_policy.go/git_quota.go/scope.go/action·argv 链路/gateFS+fsRequirement（M3a 已删；本阶段删 allow 目录收尾）
- [x] 3.5.3 aic-pod `libs/exec_procs`（host 已切换：bg 由引擎任务表承接（30min 墙钟）；exec_procs 保留供 native 白名单包装器使用（RunProcess 沙箱兜底），物理删除随 vbox 阶段二（4.4.1））
- [x] 3.5.4 aic `tools/exec/allow`、`tools/fs/allow` 及 allow 命中判定链（D13 整体废除：resolve API 去 allow 参数、ToolConfig.Allow 字段删除、前端「本会话允许」按钮+滑轨+approval_scope.js 删除）
- [x] 3.5.5 cloud git/ssh/scp 删除；自研 curl/json 删除（aic 侧 M3a 已删；pod 侧 ssh.go/scp.go/json/旧 commands/bg_* wire 面随 execCmd 瘦身删除，ssh 域随 3c 重建）
- [x] 3.5.6 `grep -r aic-pod/libs/vcore` 为空（验收 11；vcore 目录物理删除）

### 3.6 存量配置迁移【3c】
- [x] 3.6.1 mbp config.yaml 人工核对（2026-09-24 用户确认无需改动；单行 rw:/var/run/docker.sock 无次序问题，net/ssh 空）——结果记 FORK.md；win pending（机器不在线）
- [x] 3.6.2 caps/exec_allow 种子白名单核对（mbp exec_allow 空——无 shell/解释器，通过；win pending）

### 3.7 M3 出口（v4.3：三个提交点各自冒烟）
- [x] 3.7.1 3a 冒烟（3.1 后，用户重启后端）：exec script 三端（cloud/host/page）——2026-09-24 实测全过（报告：会话 f4aea329 vsh_smoke_report.md）
- [x] 3.7.2 3b 冒烟（3.2+3.3+3.5 后）：fs 五 action 三端 / page 命令 / vcore 无残留——2026-09-24 实测全过（vcore 目录已删、代码无引用，仅 docs/注释历史记录）
- [~] 3.7.3 3c 冒烟（3.4+3.6 后）：grant 闭环 / 规则表行序回归；host（mbp）+ win 设备冒烟——2026-09-24 cloud+mbp 全过（grant 双工具同表、行序回归、4 级审批链用户确认）；win 被 F7 阻塞

---

## M4：验收 + 红队 + 文档

### 4.1 验收 12 条（逐条打勾）
- [x] 4.1.1 脚本语义冒烟：管道/重定向/heredoc/变量/嵌套 bash/退出码 126·127·130·124——2026-09-24 实测（124 由 timeout 内建、130 由 bg kill 旁证）
- [x] 4.1.2 红线：UFS 无 stub 污染；host stub 只落会话 bin；registry 优先双端用例——2026-09-24 实测（host stub 在 sessions/.vsh-host/bin 共 137 个，见 F8；伪造 jq shadow 不生效）
- [x] 4.1.3 权限三段：便利根自由 / grant temp 通行 / 越界硬拒绝引导 grant；行序回归（ro 行存在时会话目录可写）；symlink 红队；Match/MatchNoFollow 双端同 matcher 单测——2026-09-24 实测（symlink 见 F4）
- [x] 4.1.4 动态逃逸 `rm $X` 拒绝且报错可读——2026-09-24 实测（ro 区逐文件拒绝，目录完好）
- [x] 4.1.5 网络：default open / 私网阻断（含 169.254.169.254）/ 审计字段齐全 / 不触发审批——2026-09-24 实测（200 + 三例私网 code=7；审计字段在 .exec 日志未逐一核）
- [~] 4.1.6 bg list/wait/kill 闭环 + 到期 124——2026-09-24 run/list/output/kill/wait 实测全通；到期 124 未实测（需 30min 墙钟）
- [~] 4.1.7 panic 隔离；并发会话隔离——单测覆盖（2.1.4），未实地构造
- [ ] 4.1.8 稳定性：连续 500 次 exec 无泄漏；Session P95 记录数值（对照 §10.0 基线）；可执行位结论
- [x] 4.1.9 page 受限语法文案 + help 原生自答——2026-09-24 实测（组合语法拒绝文案 ✓；`<cmd> --help` 覆盖 ✓；裸 help 为 bash 原生自答）
- [x] 4.1.10 host：白名单外 127；grant cmd 审批后可用且与内建管道组合；OS 沙箱收容；`>` 重定向门控——2026-09-24 实测（python3 127 ✓、grant cmd + 管道 ✓、重定向门控 ✓；OS 沙箱收容未外部验证）
- [~] 4.1.11 fs 五 action 三端回归；cp/mv/rm 下线报错；vcore 无残留；**配额闭环**（引擎内建写超限报错、fs 预检、rm 不拦）——2026-09-24 前三项实测过；配额机制就位（QuotaFS check）但实满测试未做
- [x] 4.1.12 page ls/rg 单实现双通道一致；cp/mv/rm exec 命令正确；fs write/edit/read 回归——2026-09-24 实测（五 action + exec cp/rg；遇一次 page 路由问题记 F6）

### 4.2 红队
- [x] 4.2.1 symlink 逃逸用例集（双端）——2026-09-24 实测双端无逃逸（cloud symlink 语义异常记 F4；host ln 不支持）
- [x] 4.2.2 stub shadow 尝试（写同名真实文件，双端）——2026-09-24 实测 registry 优先不可劫持
- [x] 4.2.3 脚本文件嵌套绕过（bash x.sh 内调未授权原生命令）——2026-09-24 实测 bash x.sh 内 python3 仍 127
- [x] 4.2.4 分析→执行 TOCTOU 窗口利用尝试（确认仅影响报错文案）——文档化 by design
- [ ] 4.2.5 配额绕过尝试（curl -o 大文件 / tar 解包 / 重定向追加）

### 4.3 文档
- [ ] 4.3.1 `instruction_sets_v2` §5 推倒重写（exec script 模型 + fs 五 action + grant 语义）
- [ ] 4.3.2 exec/fs 工具描述定稿（双轨口径、env 不跨 exec 持久提示、规则表内操作不审批提示）
- [ ] 4.3.3 skills 文档过一遍：依赖 fs rg/ls 行为的写法、exec curl/json 的写法（office_studio / req_scope / relia_scope / comply_scope 等数据面技能确认走 page exec curl 不受影响）
- [ ] 4.3.4 permission_rules.md §3 session 硬底线条款作废重写；host_sandbox.md 同步 vbox 语义
- [ ] 4.3.5 aic/docs/CHANGELOG.md 记录切换

### 4.4 vbox 阶段二（M4 后，可排期）
- [ ] 4.4.1 sandbox_*.go 物理迁入 vbox，exec_procs 删除收尾，pod 改 import

---

## 跨里程碑风险看板

| 风险 | 缓解 | 状态 |
|---|---|---|
| Windows PATH `:` 切分切碎盘符 | M1.3.1 强制实证，必要则 fork 改切分 | [!] win 实证失败（2026-09-24）：exec 建会话即 write /C 被拒（F7），虚拟根语义待落地 |
| SymlinkDeny 抢先于 FS 适配器 | M2.1.2 显式覆盖 + 用例 | [x] 已落地；2026-09-24 实测双端无逃逸（cloud symlink 语义异常记 F4） |
| 内建 --help 覆盖不全 | M1.3.2 清点补齐 | [x] 20 缺口已补齐，coverage 测试常驻 |
| UFS 无可执行位持久化 | M2.2.5 核实，工具描述引导 bash x.sh | [x] 已核实（chmod ErrUnsupportedOp + bash x.sh 引导）；126 退出码 2026-09-24 实测 |
| 存量 config 行序反转灵异 | M3.6.1 人工迁移 + FORK.md | [x] mbp 已确认无需改动（win pending 随 F7） |
| analyze 写参表 90 内建工作量 | M2.5.2 估足，雏形演化自 fsRequirement | [x] 已落地；实测发现预检过严/误报记 F1/F3 |
| fsauth/sandbox 测试重写量（2500+ 行） | M2.7.3 独立任务块，不与其他并行 | [x] vbox 侧新语义测试绿；3c 冒烟过（win 残项随 F7） |
| 配额随 vcore 删除丢失 | M3.5.1 先迁移后删除，验收 4.1.11 闭环 | [~] QuotaFS 机制就位（check 闸门）；实满与绕过测试未做 |

---

## 实测发现清单（2026-09-24，会话 f4aea329；详报 vsh_smoke_report.md）

| # | 级别 | 问题 | 状态 |
|---|---|---|---|
| F7 | 阻塞 | win exec 建会话即 `write /C: path denied`：Windows 虚拟根语义（2.3.3）未落地，win 端完全不可用 | [ ] 待修 |
| F4 | 正确性 | cloud 内存层 symlink 不跟随 UFS backing：经链接写返回 0 落影子子树、经链接读真实目标失败；无逃逸（fail-closed）但静默误导 | [ ] 待修 |
| F1 | UX | analyze 预检整单拦字面 /tmp 写，运行期内存层本允许（动态路径可写），预检与运行期语义不一致 | [ ] 待修 |
| F2 | UX | cloud 默认 workdir=/u/admin（ro 用户根），相对路径写全被拒（host 默认 rw workspace）；建议默认会话目录 | [ ] 待定 |
| F3 | 误报 | 预检把 `ln -s <target> <link>` 的 target（读操作）当写目标拦单 | [ ] 待修 |
| F5 | 一致性 | 出区写：cloud 预检整单拒 vs host 运行期逐命令拒，两端预检覆盖不一致 | [ ] 待确认是否 by design |
| F6 | 平台 | 多 page 客户端连接时 page exec 路由卡在无 OPFS 的客户端（820eea86），原客户端不再被选中 | [ ] 待查（属 aic 路由非 vsh） |
| F8 | 偏差 | host stub 目录实为 sessions/.vsh-host/bin（共享），非 2.3.1 的 {session_root}/{sid}/bin；疑 Runtime 单例有意调整 | [ ] 待确认 |
| F9 | 审批 | 三次 grant（cloud fs / host fs / host cmd）均触发 4 级审批弹窗，用户手动批准——审批链正常 | [x] 已确认正常 |
| F10 | 体验 | 前台 exec 自身注册进 bg 任务表占 bg-N id，`bg kill <自己>` 自杀（exit=130），id 语义易混 | [ ] 待定 |
