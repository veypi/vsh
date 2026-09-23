# exec script 化 × vsh 引擎设计方案 v4.3（终版）

- 状态：终版，可直接实施
- 日期：2026-09-23（v4.2/v4.3 同日评审修订）
- 作者：coder（会话 8b121ba8 起，v3 由会话 d5815216 按用户拍板重做，v4 由会话 b60a961e 按用户拍板重做审批模型，v4.1 同会话按评审结论修订，v4.2/v4.3 由会话 6b188bd1 按评审拍板修订）
- 依据：上游 gbash 代码核对（`/Users/veypi/test/gbash` @ `88728c5a0618cf8d8278a6602ae9e1cf05a2159d`，工作区干净）、aic/aic-pod 现状全量代码核对（v3 新增：exec/vcore/fsauth/netauth/grant/dispatch 通读）、v1/v2/v3 评审结论、v4 新增：stub/shadow 与 lookup 顺序代码复核（core.go:1222、layout.go:90-112）
- 相对 v2 的核心变化：**fs 工具瘦身（ls/rg 随 v4.1 保留、cp/mv/rm 下沉为引擎内建，fs 保留 write/edit/read/ls/rg）、vcore 真正整体删除、page 文件操作挪 page exec（write/edit/read/ls/rg 保留 fs）**、vbox 规则语义反转（首命中）的行序定稿、host stub 布局补齐、analyze 写参表定稿
- 相对 v4 的修订（v4.1，用户拍板 2026-09-23）：**配额接线补齐**（§4.1/§4.3：cloud 进程内写全路过 QuotaFS，随 vcore/cloudenv 删除迁移不丢失）、**fs 保留 ls/rg 双轨**（D11 修订：fs 保留 write/edit/read/ls/rg 五 action；cloud/host 引擎内建 ls/rg 照常存在，两者互不牵扯；page 端单实现双通道；cp/mv/rm 下沉不变）
- 相对 v4.1 的修订（v4.2，用户拍板 2026-09-23）：**行序即语义**——cloud 初始表行序定稿 `temp → 便利根 rw 会话目录 → ro 行`；去除"行类排序/特殊覆盖"归纳措辞；验收 3 补行序回归用例（ro 行存在时会话目录可写）
- 相对 v4.2 的修订（v4.3，用户拍板 2026-09-23，评审结论）：§4.1 明确 **cloud 用户根 jail 为代码硬约束**（规则表只管辖 `/u/{uid}` 内，越界路径配置写了也无效，不构成跨用户访问通道）；§8 M3 拆 **3a/3b/3c 三个可独立冒烟的提交点**（直接切换不变、不留灰度开关）；§5.1 page exec **分词规则写明**（shell-like 引号剥离，不展开变量/glob）
- 相对 v3 的核心变化（v4，用户拍板 2026-09-23）：**widen 机制废除、审批与权限正交**（cloud 挂 vbox 规则表：ro 用户盘 / rw 会话目录；exec/fs 写出区不审批、硬拒并引导 grant；grant temp = 唯一扩权通道，即 D13 后置的会话级允许机制；网络同正交化，UsesNetwork 不再升档）、**命令解析 registry 优先**（关闭 stub shadow）

## 0. 设计原则

**每个决策只有一个决策点。** 权责表即架构：

| 组件 | 唯一职责 | 明确不做 |
|---|---|---|
| vsh fork | shell 语义（解析/执行/内建/沙箱原语） | 不知审批、用户、审计的存在 |
| FS 适配器 | 一切进程内路径读写的放行/拒绝（规则表判定、挂载路由、canonicalize） | 不懂"等级"，只执行规则表 |
| NetClient | 一切进程内网络请求的放行/拒绝 + 审计 | 同上 |
| analyze | 脚本 → 预检材料（语法错、字面写目标、是否用网） | 不做安全判断，不拦截执行，不进审批链 |
| exec 工具（aic） | 调用装配：analyze 预检 → 调引擎（审批只在 grant/nosandbox，见 §2） | 不解析路径、不维护命令表、不做权限判定 |
| fs 工具（aic） | 结构化文件能力：write/edit/read + ls/rg（三端保留；ls/rg = LLM 友好的结构化搜索便利层） | 不再有 cp/mv/rm |
| Engine | 执行生命周期：Session、limits、墙钟、panic 隔离、bg 任务表、日志 | 不知审批存在 |
| pod OS 沙箱 | 原生子进程兜底（Seatbelt/bwrap/受限令牌，fail-closed） | 不管进程内操作 |

**fs 与 exec 的分工**：fs = LLM 友好的结构化能力（oldText/newText、行号读取、128KB 预算、二进制/vision、rg 结构化 JSON/行上限/minified 跳过、ls JSON tree/is_repo 检测——引擎内建替代不了的便利层）；cp/mv/rm 是纯壳层动作，下沉为 vsh 引擎内建（cloud/host）或 page exec 注册命令（page）。ls/rg 双轨：fs 的结构化实现与引擎内建的脚本命令各自独立、互不牵扯；page 端前端只实现一份 OPFS ls/rg，fs 与 exec 双通道共用。exec 的"双门模型"（gateFS/fsRequirement）随之消失——FS 适配器是唯一文件门。

## 1. 已定决策（终版）

| # | 决策 |
|---|---|
| D1 | exec 入参统一 `script`：`{1host, script, workdir?, timeout?, stdin?}`；nosandbox 保留 host-only（请求级 Critical(4) 审批，现状语义不变） |
| D2 | 后台管理 `bg list` / `bg wait <id> [--wait N]` / `bg kill <id>`；cloud/host 同用引擎任务表 |
| D3 | cloud 删除 git/ssh/scp（git 走 host 原生命令）；自研 curl/json **直接删除，不做迁移清单、不留兼容层**（json 是垃圾实现；技能数据通道本就走 page exec curl 同源调 api，不依赖 cloud curl） |
| D4 | fork `ivec/vsh`（module `github.com/veypi/vsh`，**包名 gbash→vsh 全量改**）改名一次到位，**永久零补丁**；后续自由分叉开发，LICENSE 改 Apache-2.0 + NOTICE 标明来源（见 §3、§11） |
| D5 | **权限与审批正交**（v4）：权限 = vbox 规则表（cloud/host 同一 matcher）；审批只发生在改表（grant 恒 4）与 nosandbox（恒 4）；**命令不定档、执行不审批、不设命令分级表**（见 §2） |
| D6 | grant 命令：cloud 域 fs/net（会话级）；host 域 cmd/fs/net/ssh（对接 pod 现有 grant 语义：temp 会话级内存 / permanent 本地配置；**规则表 = 纯有序表，从前向后扫、首命中生效，新规则插表头**；行序见 §11.3；无层级特权、无打洞特判；审批策略属产品层，不进底层） |
| D7 | `--help` 引擎原生自答，工具层不拦截 |
| D8 | host 同期引擎化：不暴露任何 shell/解释器，原生命令经白名单 + `grant cmd` 逐个接入；管道/重定向往 vsh 引擎内走，不经 bash |
| D9 | 原生子进程安全兜底 = pod 现有 OS 沙箱；进程内操作（内建/重定向）由引擎 FS 适配器在用户态执行同一套策略 |
| D10 | 直接切换：无灰度开关、无 legacy 回退、无历史数据兼容 |
| D11 | **fs 工具瘦身**（v4.1 修订）：保留 write/edit/read/ls/rg 五 action（cloud/page/host 三端不变；ls/rg = 结构化搜索便利层：rg JSON 输出/行上限/minified 跳过、ls JSON tree/is_repo 检测）；cp/mv/rm 从 fs 下线——cloud/host 由 vsh 引擎内建承接（经 FS 适配器门控），page 由 page exec 注册命令承接。**cloud/host 引擎内建 ls/rg 照常存在（脚本命令），与 fs 的 ls/rg 双轨互不牵扯；page 端前端只实现一份 OPFS ls/rg，fs 与 exec 两个通道共用** |
| D12 | **vcore 整体删除**（v3 修正 v2 错误）：v2 删除面写"整体删除"但 fs 工具的 ls/rg/read/write 后端就在 vcore；D11（v4.1）后 cp/mv/rm 后端随动作下线，fs read/write/edit 后端基于 ufs.FS/hostfs 重写薄层、ls/rg 逻辑自 vcore 移植进该薄层（不经 vcore），vcore 一行不留 |
| D13 | **allow 机制（"本会话允许"规则匹配）整体废除**（v3，用户拍板）：`aic/tools/exec/allow`、`aic/tools/fs/allow` 及调用链中的 allow 命中判定随切换删除，**不兼容旧规则、重构期不做任何匹配适配**；过渡期重复操作重复审批（可接受）；其角色由 **grant temp 直接承接**（会话级、插表头、恒 4 级审批）——v4 起不再另行立项 |
| D14 | **registry 优先**（v4，用户拍板）：命令查找 Registry 命中优先于 PATH 真实文件，关闭 stub shadow 通道（gbash 现状：真实文件优先 + ensureCommandStub 同名跳过）；fork M1 顺手改，上游语义测试同步反转 |
| D15 | **cloud 挂 vbox 规则表**（v4，用户拍板）：初始行序 = temp（grant 来，表头）→ 便利根 rw 会话目录 → ro `/u/{uid}`、ro `/u/{uid}/skills`；DefaultWrite deny；grant fs/net cloud 域 temp 行插表头；exec/fs 写出区不审批、硬拒并引导 grant |

## 2. 权限与审批（正交模型，v4 重写）

**权限归规则表，审批归 grant——两者正交。** v3 的 widen（审批通过扩写域）废除：审批不再改变任何权限，执行不再触发审批。90 个内建命令全部平权——`rm` 危险只是因为能删文件，而所有删除都经过 FS 适配器这唯一的门（规则表判定）。

**权限模型（cloud/host 同一 vbox matcher，§11.3）**：

- cloud 初始表（从上到下、首命中生效）：temp 行（grant 来，表头）→ 便利根（rw 会话目录）→ ro `/u/{uid}`、ro `/u/{uid}/skills`；DefaultWrite = deny，读默认开放；
- host：pod 现状规则表语义不变（workspace 等便利根 rw）；
- 进程内一切读写经 FS 适配器 Match/MatchNoFollow 判定；越界**硬拒绝，报错可读并引导 `grant fs rw <dir>`**。

**审批面（全部就两条）**：

- `grant` 命令 → 恒 4 级（唯一扩权入口；cloud 域 fs/net、host 域 cmd/fs/net/ssh；temp 行会话级、插表头）；
- nosandbox（host-only）→ 恒 4 级（现状语义不变）。

**不审批的部分**：exec 常规执行（写便利根、读开放区）免审批；写出区不升档、不弹窗，直接可读报错引导 grant——**动态构造同口径**（`rm $X` 分析不出目标照样跑，出区运行时硬拒）；网络 default open（cloud 私网阻断 + 审计，host 对接 pod net 策略），UsesNetwork 不再升档，收紧走规则表行。

**效果**：稳态零审批；扩权一次批、会话内持续（grant temp）；审批疲劳机制性消除——只有「改规则」才审批；D13 后置的「新会话级允许机制」立项取消，**grant temp 即该机制**。

便利根：cloud = 会话目录（`/u/{uid}/.sessions/{sid}`）；host = workspace 等 pod 内置便利根（现状语义）。

analyze 输出只有三样：`{WriteTargets[], UsesNetwork bool, SyntaxError}`。定位：**预检与报错材料**——语法错直接返回；字面写目标出区时预检生成可读报错（引导 grant）；不做安全判断，不进审批链。

**analyze 写参表（v3 明确）**：WriteTargets 提取需要"每个内建哪个参数是写目标"的表——`>`/`>>` 重定向目标、`cp`/`mv` 末参、`tee` 全参、`sed -i` 的 file、`mkdir`/`touch` 位置参数等，约 90 内建的一张小表。这是预检报错材料表——非拦截表、非审批表（与 v2 删掉的命令分级表定位不同）；雏形可直接演化自现状 `fsRequirement`/`cloudWriteTargets`（aic/tools/exec/exec.go:210、cloud.go:160）。隐式写（cwd 下展开、tar 解包）天然漏出，靠运行时硬拒绝兜底，报错文案必须可读（验收 4）。

**analyze 补强（脚本文件递归）**：`bash|sh|source|./x.sh` 带字面文件参数时，读取该文件递归分析（限深 2–3 层），文件内的写目标/用网合并进预检材料；文件不可读或内容含动态构造 → 放弃递归（出区目标由运行时硬拒兜底）。已知 TOCTOU（分析到执行之间文件可变）：v4 后材料仅用于报错文案，影响更小；不引快照机制。

**fs 工具同口径**：`write/edit` 出区 → 规则表拒绝 + 引导 `grant fs`（现状 CheckCloudAccess/WriteGrade 3 级审批链随切换退役）；`read/ls/rg` 开放。**配额（v4.1）**：fs write/edit 的 `CheckStorageQuota` 预检保留；引擎进程内写（重定向/cp/curl -o 等）由 fs_ufs backing 的 QuotaFS 执行期闸门兜底（§4.1）。

**已知降档（有意为之，文档化）**：现状 git `reset --hard`/`checkout -- <path>`/`branch -D` 是 Danger(3) 必审批（levels.go 的 pathspec 分析）；正交化后 host 上 git 作为白名单原生命令，规则表允许的操作一律免审批（`git push` 用网也不再升档——网络走 net 规则 + 审计）。写域内 `rm -rf` 同理。机制自洽（规则表限制爆炸半径），工具描述明示"规则表内操作不审批"。

## 3. fork：永久零补丁（已核实）

关键事实链（2026-09-23 两轮核实）：

1. stub 执行路径：`lookupCommandPath`（`internal/shell/core.go`）命中 stub 文件后走 `lookupRegistryCommand` → **`exec.Registry.Lookup(name)`**——Registry 是公开接口；
2. stub 写入：`initializeSandboxLayout`（`internal/runtime/layout.go`）按 **env PATH 里的每个目录**写 stub（非写死 /bin、/usr/bin）；对 HOME 只 `MkdirAll` 不写文件；
3. NetworkClient 注入：`runtime.go:68-72`，`NetworkClient != nil` 时自动注册 curl，引擎内置 allowlist 不生效，策略单点在注入客户端；
4. 默认 limits（`runtime.go:73-81`）：stdout/stderr 1MiB、file 8MiB、cmd 10000、loop 10000、glob 100000、subst 50；
5. `shell/syntax` 为公共包，analyze 直接用；jq/yq/awk 在 `contrib/` 子模块（非内建），rg/find/grep 是内建。

**结论：libs/vsh 自实现组合 Registry 即可注入一切平台/原生命令，P1a–d 补丁集永久删除，不向上游提 PR。**

```go
type Registry struct {
    base     commands.CommandRegistry // 引擎内建（含 ls/rg/cp/mv/rm/grep/find，D11 的壳层动作由它们承接；ls/rg 与 fs 的结构化实现双轨共存，v4.1）
    platform map[string]Command       // cloud: commands/bg/grant/list_hosts/send_user
    native   *NativeTable             // host: 白名单 + grant cmd 来的原生命令
}
// Lookup = base → platform → native；Names() = 并集（驱动 stub 写入 → which/commands 自动一致）
```

fork 工作内容 = 12 个 go.work 模块的 module 路径 + 包名（gbash→vsh）机械改写 + LICENSE 换 Apache-2.0（附 NOTICE 标明上游来源 ewhauser/gbash 与基线 SHA）+ 一次提交 + FORK.md（基线 SHA、改名脚本、上游同步策略）。**上游同步降级为按需拣选**（安全修复时 cherry-pick），不承诺定期 rebase——后续按自有路线分叉开发。

**M1 分叉改动（v4 新增，D14）**：命令解析改 **registry 优先**——`lookupCommandPath`（internal/shell/core.go:1222）把「名字在 Registry 即短路」从 stat 之后提到 stat 之前，同名真实文件不再 shadow 平台/内建命令；`ensureCommandStub`（layout.go:90-112）的同名跳过逻辑保留（registry 优先后无害）；`TestLookupCommandPrefersRealExecutableOverRegistryStub` 等上游语义测试同步反转。

**M1 附带核对项（v3 新增，v4.1 修订）**：D7 要求 `--help` 引擎原生自答——gbash 90 个内建的 `--help` 文本覆盖度未核实，缺失的由 fork 补齐（分叉自由）；顺带核对内建 ls/rg 的基础行为与 --help 文本（v4.1 后它们只承担脚本命令角色；fs 的 ls/rg 保留结构化语义，工具描述与 skills 文档按双轨口径写——结构化消费引导 fs，脚本管道用 exec 内建）。**jq 挂入（v4.1，用户拍板）**：json 删除后 JSON 查询/改写由 contrib/jq 承接，M1 挂入 cloud registry（一行 Register；host 侧同挂，与内建管道组合）。

## 4. libs/vsh 组件（aic-pod/libs/vsh，cloud/host 共用）

```
engine.go     Runtime 单例 + Session + limits/墙钟 + panic recover + 任务表 + 日志 tee
fs_ufs.go     cloud：UFS 直通 + 系统目录内存层 + canonicalize + 写域判定（唯一进程内路径权威）
fs_host.go    host：OS 文件系统 + stub 目录内存覆盖层，用户态执行 pod fs 策略（复用 pod 判定函数）
netclient.go  cloud：URL 前缀白名单 + 私网阻断 + 审计；host：对接 pod net 策略
analyze.go    syntax 解析 → {WriteTargets, UsesNetwork, SyntaxError}（两端共用；含写参表与脚本递归）
cmds.go       平台命令：commands / bg / grant / list_hosts / send_user（自带 help 文本）
native.go     host：原生命令包装器（os/exec 起真实二进制，stdio 接引擎管道）
```

引擎 Policy 用默认静态策略 + LimitOverrides，不自写实现。

### 4.1 cloud 命名空间与挂载

- HOME=`/u/{uid}`，PATH 钉死 `/usr/bin:/bin`（env 由平台每次注入，不跨 exec 持久；stub 只落这两个目录）；
- 内存层前缀集 `{/bin, /usr/bin, /tmp, /etc, /dev, /proc}` → per-exec 内存层，用完即弃；其余路径直通 UFS；
- **用户根 jail = 代码硬约束（v4.3 明确）**：cloud 一切文件访问（引擎进程内 + fs 工具）被写死限制在 `/u/{uid}` 之下，规则表只管辖其内的放行/拒绝；`/u/{uid}` 之外的路径即使因 bug 写入规则行也**不生效**——跨用户访问不存在配置通道；
- 权限 = vbox FSRuleSet（D15）：行序 temp（`grant fs` 来，表头）→ 便利根 rw 会话目录 → ro `/u/{uid}`、ro `/u/{uid}/skills`；DefaultWrite deny；越界硬拒绝且报错引导 grant；
- symlink：canonicalize-then-check，realpath 必须落允许根；
- **存储配额（v4.1 补齐）**：fs_ufs backing = QuotaFS(ufs.FS)——现状 `aic/libs/tools/quota_fs.go` 的执行期闸门随 vcore/cloudenv 删除迁移保留，引擎进程内写（重定向/cp/mv/tee/tar 解包等一切落 UFS 的写）与 fs 工具共用同一闸门；超配额报错可读（ErrStorageQuotaExceeded），rm/释放类不拦；fs write/edit 的写前预检（CheckStorageQuota）保留不变；
- **红线**：任意脚本执行后 UFS 不得出现 /bin、/usr/bin、/tmp stub 污染；
- 脚本文件处理：`bash x.sh` / `sh x.sh` / `source x.sh` / `./x.sh` 全部在引擎进程内解释（bash 为引擎内建），脚本内每条命令递归命中同一 Registry/FS 门——脚本文件不能成为绕过通道；host 端同理（脚本内调原生二进制逐条命中 native 白名单，无法走私未授权命令）；
- 边界语义：init 会 `MkdirAll(workDir)`——走同一规则表门，会话外 workdir 拒绝并报错；`~` = 用户盘根，写 `~/x` 规则表拒绝并引导 `grant fs`（工具描述引导临时文件放会话目录）。

### 4.2 host 集成

- Registry = 内建 + native 白名单（种子 = caps/exec_allow 声明）；
- **stub/PATH 布局（v3 补齐）**：host backing 是 OS 文件系统，`/usr/bin` 不可写——PATH 钉到会话级真实目录 `{session_root}/{sid}/bin`（`~/.aic/sessions/{sid}/bin`，fsauth 基础白名单内、天然可写），stub 每次 exec 写入该目录，随会话清理；不引内存覆盖层（host 侧会话目录本就是 scratch，无 cloud 的污染红线问题）；stub 目录虽在写域内可写，D14 registry 优先后同名真实文件无法 shadow 平台命令；
- 默认白名单**不含任何 shell/解释器**；`grant cmd python` 恒 4 级审批，审批文案明示"授予解释器 = 授予该进程一切能力"；
- 原生命令 = 包装器起子进程，**子进程由 pod 现有 OS 沙箱兜底**（Seatbelt/bwrap/受限令牌，per-call 按当次策略生成，fail-closed）；
- 进程内操作（内建命令、`>` 重定向、管道）走 fs_host 适配器，用户态执行与沙箱同源的 fs 策略——一套策略源，两种执行机制；
- 云端审批不覆盖本地策略（granted_level 语义不变）；nosandbox 维持 Critical(4)；
- `exec_procs` 随切换退役，bg 由引擎任务表统一承接（后台墙钟 10m → 30min，见 §6）。

### 4.3 网络

- cloud：注入 NetClient = 唯一网络权威（NetRuleSet default open + grant net 动态行 + 私网阻断 + 重定向上限/超时/响应上限 + 审计 URL/状态/大小/耗时；**网络不触发审批**）；**下载写盘路径同挂 QuotaFS（v4.1：现 cloudenv.go:116 Fetcher 配额检查迁移，curl -o 不得绕过配额）**；私网阻断清单显式化：RFC1918 + loopback + link-local 169.254.0.0/16（含云 metadata 169.254.169.254，SSRF 首选目标）；
- host：内建 curl 走 NetClient 对接 pod net 策略；原生子进程网络由 OS 沙箱按可表达性处理（沿用现状：macOS 仅 loopback 精确、不可表达条目启动前拒绝）。

## 5. aic / aic-pod 接线

### 5.1 exec 工具

- spec 重写：`{1host, script, workdir?, timeout?, stdin?}`，description 含"用 `commands` 发现、`<cmd> --help` 查用法"；timeout caps 统一 300（page 180）；
- cloud 流程：`analyze（语法错直接返回；字面写目标出区 → 可读报错引导 grant）→ Engine.Run(script)`——无 CheckLevel、无 WriteScope（v4 正交化）；超时→转 bg（不取消 ctx，任务有独立墙钟上限 30min，到期 124 杀掉）；
- page：单命令 dispatch，**分词 = shell-like 引号剥离**（单/双引号成对去引、反斜杠转义；不展开变量/glob——保证 `curl -d '{"a": "b c"}'` 类参数完整传递，v4.3）；`|`/`&&`/`>` 等 → 明确文案"page exec 暂不支持组合语法，请拆为单命令"；
- 输出约定：Content = stdout 前 1000 行；attrs + exit_code（126/127/130/124 逐码透出）；`.exec/{msg_id}.log` 全量 tee，后台化后继续写同一日志；
- **FS 写审计进 trace（v3 新增）**：网络有审计，进程内文件写补写路径记录（随 .exec 日志 tee，顺手做）。

### 5.2 fs 工具瘦身（D11，v4.1 修订）

- 保留 `write/edit/read/ls/rg` 五个 action，cloud/page/host 三端行为不变（结构化编辑：oldText/newText、行号读取、128KB 预算、二进制/vision；结构化搜索：ls JSON tree/is_repo 检测、rg JSON 输出/行上限/minified 跳过）；
- read/write/edit 后端重写薄层、ls/rg 逻辑自 vcore 移植进薄层：cloud 基于 ufs.FS（包 QuotaFS，§4.1）、host 基于 hostfs、page 走现有前端通道——vcore 依赖彻底切除；
- 权限：`write/edit` 出区 → 规则表拒绝 + 引导 `grant fs`（v4 正交化，CheckCloudAccess/WriteGrade 3 级链随切换退役）；`read/ls/rg` 开放；
- 删除：`cp/mv/rm` 四端 action、`FSRequiredIn`（rm recursive 升 Danger）、`fs/allow` 中随动规则；工具描述反转重写（不再是"文件操作全在 fs"，而是"结构化编辑与搜索在 fs，壳层动作 cp/mv/rm 在 exec"；双轨口径写清：fs ls/rg = 结构化便利层，exec 内建 ls/rg = 脚本命令）。

### 5.3 page exec 扩编（D11 page 侧，v4.1 修订）

- 前端实现**一份** OPFS `ls/rg`，fs 工具与 exec 命令两个通道共用同一实现（用户拍板 2026-09-23；输出契约以 fs 结构化语义为准；regex 用 RE2 语义子集——JS RegExp 须主动拒绝 lookaround/backreference，保持三端口径一致）；
- `cp/mv/rm` 注册为 page exec 命令（read/write/edit 不挪，page 继续走 fs 工具——前端结构化编辑能力现成，不重复造）；
- 与现有 page curl 同通道（req-reply，180s）；page exec 统一 Write(2) 基线不变。

### 5.4 删除面（直接删，无清单无灰度）

- aic-pod `libs/vcore` **整体**（D12：curl/json/git/commands/bg_* 随 exec 切换死；cp/mv/rm 后端随 fs 动作死、ls/rg 逻辑移植 §5.2 薄层后删；read/write/memvfs 由 §5.2 薄层替代；`aic/libs/tools/cloudenv.go` 的 vcore.Env 装配随之删除——**注意 cloudenv.go:116 的 Fetcher 配额检查与 quota_fs.go 的 QuotaFS 必须迁移保留，见 §4.1/§4.3**）；
- aic `tools/exec`：`git.go` / `git_policy.go` / `git_quota.go` / `scope.go` 静态表 / action·argv 链路 / `gateFS`+`fsRequirement` 双门（FS 适配器是唯一门）；
- aic-pod `libs/exec_procs`（host 切换后）；
- aic `tools/exec/allow`、`aic/tools/fs/allow` 及 allow 命中判定链（D13：整体废除，不做匹配适配，新机制后置另行设计）；
- vcore/levels.go 的 exec 半（execCoreLevels/gitSubLevels/jsonSubLevels/pathspec 分析）；fs 半（FSRequired）随瘦身收敛到五 action（write/edit/read/ls/rg）；
- aic fs 审批链的 WriteGrade 出区升 3 级分支（CheckCloudAccess；v4 正交化后由规则表拒绝替代）；
- `instruction_sets_v2` §5 推倒重写（不打补丁），exec/fs 工具描述同步重写，skills 文档中依赖 fs rg/ls 行为的写法过一遍。

## 6. limits 定稿（已拍板）

| 项 | 值 |
|---|---|
| MaxStdoutBytes | 8 MiB |
| MaxStderrBytes | 1 MiB |
| MaxFileBytes | 64 MiB |
| MaxCommandCount / MaxLoopIterations | 10000 |
| MaxGlobOperations / MaxSubstitutionDepth | 100000 / 50 |
| 前台 timeout 上限 | 300s（page 180s） |
| 后台任务墙钟 | 30min，到期 124 终止（现状 10m，随切换调整） |

## 7. 验收标准（12 条）

1. 脚本语义冒烟：管道/重定向/heredoc/变量/嵌套 bash（cloud 内）/退出码 126·127·130·124；
2. **红线**：任意脚本执行后 UFS 无 /bin、/usr/bin、/tmp stub 污染；host 侧 stub 只落 `{session_root}/{sid}/bin`；registry 优先：stub/PATH 目录同名真实文件不 shadow 内建与平台命令（cloud/host 各一用例）；
3. 权限三段：便利根自由 / `grant fs` temp 后通行（会话内免重批）/ 未授权越界硬拒绝且报错引导 grant；行序回归：ro 行存在时 cloud 会话目录可写（首命中生效）；symlink 逃逸红队用例全过；Match/MatchNoFollow 与 host 同 matcher 单测；
4. 动态逃逸用例：`rm $X`（X 指向便利根外）→ 拒绝且报错可读、引导 grant；
5. 网络：default open 放行 / 私网阻断 / 审计字段齐全；网络不触发审批；
6. bg list/wait/kill 闭环 + 后台墙钟到期 124；
7. panic 隔离不带崩进程；同 Runtime 并发会话隔离；
8. 稳定性基线：连续 500 次 exec 后 goroutine/内存无泄漏趋势；Session 创建 P95 延迟有记录数值；UFS 适配器支持可执行位持久化（`./x.sh` 依赖 chmod +x，不支持则工具描述引导 `bash x.sh` 形式）；
9. page 受限语法文案 + help 原生自答；
10. host：白名单外命令 127；`grant cmd` 审批后可用且与内建管道组合；原生子进程被 OS 沙箱收容（写白名单外拒绝）；`>` 重定向经 fs_host 门控；
11. **fs 瘦身（v4.1 修订）**：fs 工具 write/edit/read/ls/rg 三端回归全绿；`fs cp/mv/rm` 已下线 action 报可读错误并引导 exec；vcore 无残留引用（`grep -r aic-pod/libs/vcore` 为空）；**配额闭环**：引擎内建写（重定向/cp/curl -o）超配额可读报错，fs write/edit 预检保留，rm 不拦；
12. **page exec 文件命令（v4.1 修订）**：page `ls/rg` 单实现双通道（fs 与 exec 结果一致）；page `cp/mv/rm` exec 命令对 OPFS 行为正确；page fs write/edit/read 回归不变。

## 8. 里程碑

- **M1**：fork 改名 + go.work 接线 + 上游相关包测试绿（纯机械）+ 内建 --help/ls/rg 行为核对（§3 末尾）+ registry 优先分叉改动（D14，含上游语义测试反转）+ **Windows PATH 分隔符核实（v4.1 强制项：layout.go `commandDirectoriesForPath` 按 `:` 切分——host FS 适配器必须向引擎呈现 unix 风格虚拟路径使 `{session_root}/{sid}/bin` 不含盘符冒号，否则 fork 内改切分逻辑；win 设备实证）** + jq 挂入 cloud/host registry（v4.1 已拍板，§3）；
- **M2**：libs/vsh 七文件 + 单测（engine / fs_ufs / fs_host / netclient / analyze / cmds / native）；analyze 写参表在此估足工作量；**引擎 Policy SymlinkMode 核实（v4.1 强制项：默认 SymlinkDeny 会在 allowPath 先于 FS 适配器拦截 symlink 穿越，与适配器 canonicalize-then-check 语义冲突——必须显式覆盖为放行型，保证 FS 适配器是唯一进程内路径权威）**；fs_ufs 的 QuotaFS 包装与 NetClient 下载配额挂检在此落地（§4.1/§4.3）；
- **M3**（v4.3 拆三个可独立冒烟的提交点；直接切换不变、不留灰度开关）：
  - **M3a**：exec 工具切换（aic spec 重写 + cloud/host/page 三端接引擎）→ 冒烟；
  - **M3b**：fs 瘦身 + page exec 扩编 + 删除面清理（直接切换）→ 冒烟；
  - **M3c**：cloud vbox 规则表与 grant fs/net cloud 域接线（D15）+ host grant 四域对接 → 冒烟；
- **M4**：验收 12 条 + 红队 + instruction_sets_v2 §5 重写 + skills 文档过一遍。

## 9. 已敲定事项记录

1. limits 数值 → §6（用户确认）；
2. 系统目录前缀集 → 去掉 /home/agent，HOME=/u/{uid}，PATH 钉死（用户提出并确认；核实 init 对 HOME 仅 MkdirAll）；
3. host 白名单 → 种子 = caps/exec_allow 声明，运行时 `grant cmd` 扩充；不暴露 shell/解释器（用户提出"原生命令逐个申请"模型）；
4. 原生子进程兜底 → pod 现有 OS 沙箱（用户确认）；
5. curl/json → 直接删除，零迁移（用户确认；v3 补：技能数据通道走 page exec curl 同源调 api，删 cloud 自研 curl 对技能生态无影响）；
6. 规则表 = 纯有序表：**从前向后扫、首命中生效，新规则插表头**；session temp 与全局表纯拼接（temp 在前），temp 可覆盖 deny（"用户点就点了"，审批疲劳属产品层）；permission_rules.md §3 的 session 硬底线作废，grant.go DenyHit 拒批随 vbox 迁移删除（用户拍板 2026-09-23，推翻 D6 初稿"deny 内目标不可申请"）；
7. **fs 工具瘦身（v3 用户拍板，v4.1 修订）**：cp/mv/rm 挪 exec（cloud/host 引擎内建、page 挪 page exec）；ls/rg 双轨——fs 保留（三端含 page），引擎内建照常存在，page 端单实现双通道；fs 共留 write/edit/read/ls/rg 五 action；vcore 随之真正整体删除；
8. ~~widen 粒度（v3）~~ **已作废（v4，见 12）**：widen 机制整体废除——审批不再扩写域，扩权唯一通道是 grant temp；
9. **host stub 布局（v3）**：PATH 钉会话级 `{session_root}/{sid}/bin`，不引内存覆盖层；
10. **行序（v4.2 修订）**：见 §11.3——单一有序表、首命中生效，无任何特殊覆盖规则；host 行序 `temp → cfg/permanent → builtin deny → 便利根`，cloud 行序 `temp → 便利根 rw 会话目录 → ro 行`。
11. **allow 机制废除（v3，用户拍板 2026-09-23）**：fs/exec 的"本会话允许"规则匹配整体删除（D13），旧规则不兼容、重构期不适配；过渡期审批仅当次生效；v4 起由 grant temp 承接，不再独立立项（见 12）。
12. **审批与权限正交（v4，用户拍板 2026-09-23，推翻 8）**：widen 机制废除；cloud 挂 vbox 规则表（行序：temp → 便利根 rw 会话目录 → ro `/u/{uid}`、ro skills；DefaultWrite deny）；exec/fs 写出区不审批、硬拒并引导 grant；**grant temp = 唯一扩权通道**（恒 4 级、会话级、插表头），即 D13 后置的会话级允许机制（立项取消）；网络同正交化（default open + 私网阻断 + 审计，UsesNetwork 不升档）；
13. **registry 优先（v4，用户拍板 2026-09-23）**：命令解析 Registry 命中优先于 PATH 真实文件，关闭 stub shadow 通道；fork M1 顺手实施（D14）。
14. **配额接线（v4.1，用户拍板 2026-09-23）**：cloud 进程内写全路过 QuotaFS——fs_ufs backing 包装（§4.1）+ NetClient 下载挂检（§4.3）+ fs write/edit 预检保留；随 vcore/cloudenv 删除迁移不丢失；验收 11 闭环；
15. **fs 保留 ls/rg 双轨（v4.1，用户拍板 2026-09-23）**：fs 工具保留 write/edit/read/ls/rg 五 action（ls/rg = 结构化搜索便利层）；cloud/host 引擎内建 ls/rg 照常存在（脚本命令），两者互不牵扯；page 端前端单实现、fs 与 exec 双通道共用；cp/mv/rm 下沉不变（D11 修订、D12 删除面同步调整：ls/rg 逻辑移植 fs 薄层而非随动作下线）。
16. **jq 挂入（v4.1，用户拍板 2026-09-23）**：json 删除后由 contrib/jq 承接 JSON 查询/改写，M1 挂入 cloud 与 host registry。

## 10. 附录：关键代码位置（2026-09-23 核实）

### 10.0 性能实测（2026-09-23，mbp M 系，in-process，内存 FS；harness = /Users/veypi/ivec/temps/gbash-perf/cmd/perf）

- Runtime 创建 655µs（一次性）；**NewSession P50=22µs / P95=130µs**（含内存层初始化）；
- Exec 简单管道（echo|grep|wc）P50=88µs / P95=307µs；混合脚本（循环+重定向+算术）P50=276µs / P95=486µs；
- 串行 500 轮 + 并发 8×50（122.8ms 总耗时）**零失败**；结束后 goroutines +0、heap +0.3MB，无泄漏趋势；
- 结论：引擎开销相对 LLM/网络延迟可忽略；验收第 8 条基线以此为准。注意：此处 FS 为内存层，UFS 适配器的真实 IO 开销在 M2 单测中补测。

```
# gbash（fork 基线 88728c5）
internal/shell/command_dispatch.go:~66      # 127 出口（executeCommand 内 if !ok）
internal/shell/core.go:1108-1143            # lookupCommand（PATH 解析）
internal/shell/core.go                      # lookupCommandPath：stub → exec.Registry.Lookup
internal/shell/core.go:1222-1230            # stub→registry 短路位置（M1 改 registry 优先：stat 前先查 Registry，D14）
internal/runtime/layout.go:90-112           # ensureCommandStub 同名跳过（shadow 成因；registry 优先后无害）
internal/runtime/layout.go                  # initializeSandboxLayout：stub 按 PATH 目录写；HOME 仅 MkdirAll
internal/runtime/runtime.go:68-81           # NetworkClient 注入自动注册 curl；默认 limits
internal/builtins/subexec_helpers.go:221    # resolveAllCommands（which/type 解析路径）
contrib/{jq,yq,awk}                         # 非内建，按需挂入
# aic / aic-pod（v3 全量通读核对）
aic/tools/exec/{exec.go,cloud.go,scope.go,git*.go,approval.go,bg.go}   # 切换/删除面
aic/tools/exec/exec.go:186-233              # gateFS/fsRequirement 双门（删除；写参表雏形）
aic/tools/exec/cloud.go:98,160              # cloudWriteTargets → checkCloudWriteLevel（写分级现状；v4 后 analyze 写参表演化自它，仅作预检材料）
aic/tools/fs/{fs.go,approval.go}            # 瘦身面：RunFS/FSActions/FSRequired 依赖 vcore
aic/libs/tools/cloudenv.go                  # vcore.Env 装配（随 vcore 删除）
aic-pod/libs/vcore                          # 整体删除（含 fs 后端 memvfs/ls/rg/read/write）
aic-pod/libs/vcore/levels.go                # exec 半删；fs 半收敛到五 action（v4.1）
aic-pod/libs/exec_procs                     # host 现状执行（切换后退役；sandbox_*.go 迁 vbox）
aic-pod/libs/fsauth,netauth                 # 规则判定（last-wins → vbox first-wins 反转，见 §11.5）
aic-pod/libs/host/grant.go                  # grant 四域现状（exec/fs/net/ssh；DenyHit 拒批随迁移删）
aic-pod/libs/host/dispatch.go               # execCmd：声明匹配 → execAllowed → vcore.Run/runLocal 三层
aic/tools/exec/allow/, aic/tools/fs/allow/    # "本会话允许"规则匹配（D13 废除，新机制后置）
aic-pod/docs/host_sandbox.md                # pod OS 沙箱与 grant 现状语义
```

## 11. 库接口规划（vsh / vbox）

### 11.1 定位与边界

| 库 | module | 职责 | 不知道 |
|---|---|---|---|
| vsh | `github.com/veypi/vsh` | shell 引擎：解析/执行/内建/进程内沙箱原语（gbash fork，改名零补丁，后续自由分叉） | 审批、用户、OS 沙箱 |
| vbox | `github.com/veypi/vbox` | 原生子进程沙箱：规则模型 + 纯判定 + OS 落地（Seatbelt/bwrap/Win 令牌）+ 进程生命周期 | shell、审批、grant 持久化 |
| glue | `aic-pod/libs/vsh` | 平台集成：engine 生命周期 / FS 适配器 / NetClient / analyze / 平台命令 / native 包装器 | — |

vbox 抽离来源：`aic-pod/libs/exec_procs/sandbox_*.go`（OS 机制）+ `libs/fsauth`、`libs/netauth`（规则判定）。**配置解析、grant 生命周期、持久化留在 pod（libs/host）**，每次调用编译快照传入。

### 11.2 vsh 调用接口（aic / aic-pod 同构）

包名全量改为 `vsh`（后续分叉开发、不保证与上游原样兼容）；LICENSE 改 Apache-2.0 + NOTICE 标明上游 `ewhauser/gbash` 与基线 SHA `88728c5`。接口即上游公开 API 改名：

```go
import "github.com/veypi/vsh"

rt, _ := vsh.New(                       // 进程级单例
    vsh.WithRegistry(reg),              // glue 组合 Registry（内建 + 平台/原生命令）
    vsh.WithFileSystem(fsFactory),      // glue FS 适配器（cloud=UFS / host=OS）
    vsh.WithNetworkClient(nc),          // glue 注入 → curl 自动注册
    vsh.WithLimitOverrides(limits),     // §6 定稿值
    vsh.WithTracing(traceCfg),          // 审计（含 FS 写路径，§5.1）
)
sess, _ := rt.NewSession(ctx)           // 每次 exec 一个（P95=130µs，实测）
res, _ := sess.Exec(ctx, &vsh.ExecutionRequest{Script: s, WorkDir: wd, Env: env, Stdin: in, Timeout: to})
```

glue 使用的公开子包（现成扩展点，零补丁）：`commands`（Command/CommandRegistry/CommandSpec/Invocation）、`fs`（gbfs.FileSystem）、`policy`（Limits）、`network`（Client）、`shell/syntax`（analyze AST）、`trace`（审计）。

### 11.3 vbox 接口（三段式：规则模型 → 编译 → 执行）

规则模型 = **单一有序表，从前向后扫、首命中生效；新规则一律插表头**。行为完全由行序决定，无任何"谁压谁"的特殊规则：

**行序（§9.10）**：
- temp（session grant）构建时插表头 → 先于一切行命中（用户拍板"用户点就点了"）；
- host（Snapshot 纯拼接）：`temp → cfg/permanent → builtin deny → 便利根`；
- cloud：`temp → 便利根 rw 会话目录 → ro /u/{uid}、ro /u/{uid}/skills`（+ DefaultWrite 兜底）；
- 无层级特权、无打洞特判——审批策略属产品层，不进 vbox。

```go
package vbox

// ---- 规则模型（平台无关纯数据，有序表） ----
type Effect uint8
const (
    EffDeny Effect = iota // 读写双拒
    EffRO                 // 读放写拒
    EffRW                 // 读写
)
type Rule struct {
    Pattern string // 段内 *、跨层 **、?；裸路径覆盖子树
    Effect  Effect
    Class   Class  // temp | cfg | builtin | convenience —— 仅参与行序拼接（§9.10），判定时不看
}
type FSRuleSet struct {
    Rules        []Rule // 有序：从头扫、首命中生效；新规则插表头（同类插同类段头）
    DefaultWrite Effect // 未命中写兜底（EffDeny|EffRW = fs_policy）；读默认开放
}
func (FSRuleSet) Match(path string, op FileOp) Decision
// MatchNoFollow：unlink/rename 语义（rm/mv 作用于链接本身，末段 symlink 不跟随）——
// 现状 fsauth.DecideNoFollow 的保留项（2026-09-22 实测 .venv 删除被阻的修复），迁移不得丢。
func (FSRuleSet) MatchNoFollow(path string, op FileOp) Decision

type NetRule struct { HostPort string; Allow bool } // "example.com:443"，端口可 *
type NetRuleSet struct {
    Rules   []NetRule // 有序：从头扫、首命中生效；新规则插表头
    Default bool      // 未命中兜底 = net_policy open/deny
}
func (NetRuleSet) Match(hostport string) bool

type Policy struct { FS FSRuleSet; Net NetRuleSet }

// ---- 编译：fail-closed——当前平台不可表达的规则 → error，启动前拒绝 ----
func Compile(p Policy) (*Spec, error)
func Capabilities() Caps                // darwin/linux/windows 表达能力查询

// ---- 执行（进程组终止 / 墙钟 / 输出上限 收进 Box） ----
type Request struct {
    Argv []string; Env map[string]string; Dir string
    Stdin io.Reader; Stdout, Stderr io.Writer
    WallClock      time.Duration
    MaxOutputBytes int64
    NoSandbox      bool                 // 免沙箱须亮明身份（调用方已走 Critical(4) 审批）
}
type Result struct { ExitCode int; TimedOut bool /* ... */ }

type Box interface {
    Run(ctx context.Context, req *Request) (*Result, error)
}
func NewBox(spec *Spec) (Box, error)
```

**关键设计点**：vbox 只有"有序表 + 纯判定 + OS 落地"，无策略组合逻辑。进程内门（FSProvider）与 OS 沙箱（Box.Run）消费同一份 `[]Rule`，规则语义只有一份定义。

### 11.4 调用关系

```
aic（cloud server）
  └─ aic-pod/libs/vsh（glue: engine/fs_ufs[接 vbox 规则表]/netclient/analyze/cmds）
       └─ vsh

aic-pod（device）
  ├─ libs/host（状态层：全局规则表 + tempTables map[sid][]vbox.Rule + grant 生命周期/持久化）
  │    └─ Snapshot(sid) vbox.Policy = temp[sid] ++ 全局表 ++ 便利根行（按 §9.10 行序纯拼接）
  ├─ libs/vsh（glue: engine/netclient/analyze/cmds/native）
  │    ├─ FSProvider = vsh fs.FileSystem 实现：canonicalize → vbox.Match → 放行/硬拒绝
  │    │    （cloud backing = UFS+系统目录内存层；host backing = OS；两端共用同一 matcher）
  │    ├─ vsh ── WithFileSystem(FSProvider) / WithRegistry / WithNetworkClient
  │    └─ vbox ── native.go：原生命令 → Compile(Snapshot) → Box.Run（fail-closed）
  └─ NetClient 持同一 Snapshot 的 NetRuleSet，net 判定同源

第三方（远期）：vsh 可独立 embed；vbox 可独立用于任何"跑不可信子进程"场景
```

### 11.5 落地节奏

- go.work `use` 增加 `./vsh`、`./vbox`（M1 时）；
- vsh M1：gbash 拷入（剪枝 website/packages/docker/flake 等非 Go 资产，删 fork 内 go.work，contrib 按需保留）+ module/包名改名 + LICENSE/NOTICE + 上游相关包测试绿 + 内建 --help/rg 行为核对；
- vbox 两阶段抽离（**v3 修正：语义反转不是机械迁移**）：
  - 现状 fsauth/netauth 是**后命中者胜**（builtin deny 表头、cfg 追加表尾、扫全表取最后命中）；vbox 是**首命中生效**。迁移 = 语义反转：拼接顺序倒置 + 行序（§9.10）+ `fsauth_test.go`（738 行）与三平台 sandbox 测试（约 1800 行）全部按新语义重写（不是搬迁）；darwin seatbelt 的 M3 行序映射同步反转；
  - **存量 config.yaml 注意**：开发者机器上现有 fs_rules 行序在两种语义下含义相反——测试版无用户可接受，但 mbp/win 本机配置迁移时要人工过一遍（FORK.md 记一句）；
  - 阶段一（M2–M3）：接口定稿（§11.3），实现暂留 pod 内（native.go 调 exec_procs 沙箱），接口形状对齐 vbox；同时完成 fsauth/netauth 的**状态层/纯 matcher 拆分**——状态层（cfg 耦合、Reconcile、tempTables、持久化）留 libs/host，纯 matcher（有序表判定，含 Match/MatchNoFollow 与 canonicalize/dual-forms 逻辑）按新语义重写后物理迁入 vbox；`ScrubEnv` 归 vbox（沙箱邻接），`defaultDenyPaths`/`CacheRoots` 归 pod（默认策略源，注入全局表）；
  - 阶段二（M4 后）：`sandbox_*.go` 物理迁入 vbox，exec_procs 删除，pod 改 import；**删除 grant.go 的 DenyHit 拒批逻辑与 grantExec 的 exec_deny 拒批**（permission_rules.md §3 "session 层不得放宽 deny"条款作废，审批策略上移产品层）。
