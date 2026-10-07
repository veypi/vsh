# vsh

**纯 Go 的进程内 shell 引擎**，可直接嵌进任意 Go 程序：解析、执行、90+ 内建命令，
命令表 / 文件系统 / 网络出口全部由调用方注入。默认不启动任何宿主程序，也不要求系统里
存在 `bash`。

**中文** | [English](README.en.md)

```go
rt, _ := vsh.New()                       // 默认：内存 FS + 内建命令 + 无网络
sess, _ := rt.NewSession(ctx)
res, _ := sess.Exec(ctx, &vsh.ExecutionRequest{
	Script: `for i in 1 2 3; do echo "line $i"; done | wc -l`,
})
fmt.Print(res.Stdout)   // 3
```

运行契约见 [SPEC.md](SPEC.md)；安全模型见 [THREAT_MODEL.md](THREAT_MODEL.md) 与
[SECURITY.md](SECURITY.md)；来源与署名见文末。

## 特点

- **进程内执行**：`cat`/`ls`/`grep`/`sed`/`awk`… 都是 Go 实现，不 fork 宿主 shell；
  需要真实程序时由调用方显式接一条原生出口（`WithNativeExec`），默认没有。
- **一切能力经注入**：命令表（`WithRegistry`）、文件系统（`WithFileSystem` /
  `WithWorkspace`）、网络（`WithNetworkClient` / `WithHTTPAccess`）都由调用方装配。
  引擎只负责 shell 语义，不知道审批、用户、凭据或审计的存在。
- **默认即沙箱**：不带选项时是内存文件系统（`/home/agent`）+ 内建命令 + 静态策略 +
  无网络；每个 `Session` 一份独立状态，同一会话内串行、会话之间隔离。
- **可观测**：`WithTracing` / `WithLogger` / `WithAnalysisObserver` 输出结构化事件。
- **有配额**：命令数、循环次数、glob 次数、输出大小等经 `WithPolicy` /
  `WithLimitOverrides` 收紧，避免不可信脚本拖垮宿主。

## 安装

```sh
go get github.com/veypi/vsh
```

Go 1.26+（见 [go.mod](go.mod)）。

## 快速开始（嵌入）

```go
package main

import (
	"context"
	"fmt"

	"github.com/veypi/vsh"
)

func main() {
	ctx := context.Background()

	rt, err := vsh.New() // 默认：内存 FS + 内建命令 + 无网络
	if err != nil {
		panic(err)
	}
	sess, err := rt.NewSession(ctx)
	if err != nil {
		panic(err)
	}

	res, err := sess.Exec(ctx, &vsh.ExecutionRequest{
		Script: `for i in 1 2 3; do echo "line $i"; done | wc -l`,
	})
	if err != nil {
		panic(err)
	}
	fmt.Print(res.Stdout)     // 3
	fmt.Println(res.ExitCode) // 0
}
```

挂真实目录、开受控网络、追加自定义命令：

```go
rt, err := vsh.New(
	vsh.WithWorkspace("/srv/app"),                 // 宿主目录挂进沙箱
	vsh.WithHTTPAccess("https://api.example.com"), // curl 只允许该前缀
	vsh.WithRegistry(reg),                         // 自定义命令，可覆盖同名内建
	vsh.WithLimitOverrides(policy.Limits{MaxCommandCount: 2000}),
)
res, err := rt.Run(ctx, &vsh.ExecutionRequest{Script: `cat data/*.json | jq -r .id`})
```

`Runtime.Run` 会为单次调用建临时会话；需要连续多脚本共享状态时用
`Runtime.NewSession` 拿一个会话再反复 `Session.Exec`。

## 命令行

```sh
go install github.com/veypi/vsh/cmd/vsh@latest

vsh -c 'echo hello | tr a-z A-Z'                      # HELLO
vsh --root ./project --cwd /home/agent/project -c ls   # 只读挂载 + 内存覆写
vsh -i                                                 # 交互式
```

`vsh --help` 列出 shell 选项和沙箱文件系统选项（`--root` 只读挂载并加内存覆写、
`--readwrite-root` 写回宿主、`--inherit-env` 继承指定宿主环境变量）。`cmd/vsh-gnu`
是跑 GNU coreutils 测试集的对拍入口。

## 扩展点

| 选项 | 用途 |
| --- | --- |
| `WithRegistry` | 命令表；裸名查找顺序 = Registry → 真实 PATH，显式路径只查文件 |
| `WithFileSystem` / `WithWorkspace` / `WithWorkingDir` | 文件系统后端与会话初始 cwd |
| `WithNetworkClient` / `WithNetwork` / `WithHTTPAccess` | 网络出口与允许前缀 |
| `WithPolicy` / `WithLimitOverrides` | 资源与命令配额 |
| `WithNativeExec` | 原生进程出口：只收到已解析的二进制路径，沙箱与权限由调用方决定 |
| `WithTracing` / `WithLogger` / `WithAnalysisObserver` | 结构化事件、日志、只读语义观察 |
| `WithBaseEnv` | 子进程基础环境 |

公开子包：`commands`（命令编写与注册）、`fs`（文件系统后端）、`policy`（限额与策略）、
`network`（HTTP 客户端）、`shell/syntax`（语法树）、`trace`（事件模型）。
`internal/*` 与未在文档中列出的子包不属于公开 API。

## contrib

[`contrib/`](contrib) 下每个工具都是**独立 Go 模块**（各自带 `go.mod`、`CHANGELOG.md`
与版本号），根模块不依赖它们——只引入需要的那一个，其余工具的依赖不会进你的构建图：

```sh
go get github.com/veypi/vsh/contrib/jq@v0.1.0
```

| 模块 | 作用 | 引入方式 |
| --- | --- | --- |
| `contrib/awk` | 沙箱内 `awk` 命令 | 显式，或经 `extras` |
| `contrib/htmltomarkdown` | 沙箱内 `htmltomarkdown` 命令 | 显式，或经 `extras` |
| `contrib/jq` | 沙箱内 `jq` 命令（gojq） | 显式，或经 `extras` |
| `contrib/sqlite3` | 沙箱内 `sqlite3` 命令 | 显式，或经 `extras` |
| `contrib/yq` | 沙箱内 `yq` 命令 | 显式，或经 `extras` |
| `contrib/extras` | 上述 5 个的聚合注册（含 `FullRegistry()`） | 聚合 |
| `contrib/nodejs` | 实验性 `nodejs` 命令（未加固，未进 `extras`） | 显式 |

`extras` 依赖上述 5 个模块，因此 contrib 有跟版义务，约定见下面的「发布」节。

## examples

[`examples/`](examples) 是独立模块，覆盖常见嵌入方式：OpenTelemetry 观测、sqlite 与
agentfs 作为文件系统后端、事务化工作区、自定义命令（zstd）、harness overlay、
OAuth 网络扩展等。

## 已知边界

- 目标是 **bash 兼容子集**，不是 bash 完整实现；契约以 [SPEC.md](SPEC.md) 为准。
- 默认没有原生执行：`ls` 之类走内建实现，只有显式配置 `WithNativeExec` 才可能碰到真实程序。
- Registry 命中优先于 PATH；显式路径（`/bin/ls`）只访问真实文件，不会生成/修复 stub 目录。
- 单个 Session 内执行是串行的；并发请用多个 Session。
- Windows 的路径与环境在入口按 `/c/...` 规范形归一、OS 出口还原；不解析原生分号 `PATH`。
- `contrib/jq` 的 stream 错误模式用例在 Go 1.27 下失败（错误文本/列号断言差异），
  属继承自最早基线的既有失败，与命令解析无关。

## 平台支持

| 平台 | 状态 |
| --- | --- |
| darwin（arm64 / amd64） | **支持**：CI 构建 + 全量测试 |
| linux（amd64 / arm64 / 386） | **支持**：CI 构建 + 全量测试 |
| windows（amd64 / arm64） | **支持**：CI 构建 + 全量测试 |
| freebsd / netbsd / openbsd / dragonfly / solaris / illumos / aix / android / js / wasip1 | 未承诺：当前可编译（`GOOS=… go build ./...` 通过，含 unsupported 桩），但未做行为验证 |
| plan9 | **不支持**：缺 POSIX errno 族与 fork/pipe 语义，构建会显式失败 |

未列出的平台按「未承诺」处理。[`.github/workflows/ci.yml`](.github/workflows/ci.yml)
只跑三个支持平台的构建与全量测试。

## 测试

```sh
go test ./...
go vet ./...

cd contrib/jq && go test ./...    # contrib 是独立模块，需分别验证
```

部分一致性用例需要外部 oracle（`VSH_CONFORMANCE_RIPGREP`、`VSH_CONFORMANCE_DIFF`
指向固定版本的 ripgrep / diff）；缺少时会**自报跳过**，跳过不等于通过。

contrib 模块是独立 module：在其目录内构建/测试时，如果外层套着一个更大的 Go
workspace 且没有 `use` 该模块，需要 `GOWORK=off`（模块内的 `replace ../..` 保证
用的仍是本地根模块）。

## 发布

三个版本面：

| 对象 | tag 形式 | 说明 |
| --- | --- | --- |
| 根模块 | `vX.Y.Z` | shell 引擎本体；版本见 [VERSION](VERSION)，变更见 [CHANGELOG.md](CHANGELOG.md) |
| contrib 模块 | `contrib/<name>/vX.Y.Z` | 各自独立版本，均自 **v0.1.0** 起；各带自己的 CHANGELOG |
| `examples/` | 不单独发版 | 本仓内的用法示例（独立 go.mod） |

批量打 tag 用 [`scripts/release-tags.sh`](scripts/release-tags.sh)（默认 dry-run，加
`--yes` 才建本地 tag；只建不推）：

```sh
scripts/release-tags.sh --list                  # 现状：各模块已发版本
scripts/release-tags.sh --contrib 0.1.0         # 所有 contrib 打同一版本（根不动）
scripts/release-tags.sh --all 0.2.1             # 根 + 所有 contrib 同版本（跟版批量）
scripts/release-tags.sh --only jq=0.1.1         # 单个模块单独发
```

**跟版约定**（v0.x 期间尤其重要）：Go 对 `v0` 没有兼容承诺，根模块升 minor 之后，
下游的 MVS 会把根版本直接顶上去——所以

- 根模块升 **minor**：所有 contrib 必须在同一窗口内各发一版，并把
  `require github.com/veypi/vsh` 指到新的根版本；
- 根模块升 **patch**：没动 contrib 可达 API 时可以不动 contrib；
- contrib 自身按自己的 SemVer 走（行为破坏进位 minor；仍处 v0 时同样只保证同 minor 内兼容）。

发布顺序：

1. 根模块收口 `VERSION` 与 `CHANGELOG.md`，提交；
2. 打根 tag；
3. 各 contrib 的 `go.mod` 把 `require github.com/veypi/vsh` 指到新根版本，`CHANGELOG.md` 补条目；
4. `scripts/release-tags.sh --contrib <version>`（或 `--only <name>=<version>`）打 contrib tag；
5. 推送 tag，Go module proxy 按需拉取。

仓内开发靠各 contrib 的 `replace github.com/veypi/vsh => ../..`；replace 不随模块发布生效，
使用者拿到的是 `require` 里那个已发布版本。

## 来源与署名

本项目从一份 Apache-2.0 的开源 shell 运行时代码库（[gbash](https://github.com/ewhauser/gbash)，
基线 commit `88728c5a0618cf8d8278a6602ae9e1cf05a2159d`）起步，此后由本仓**独立维护与演进**：
期间做了大规模重写、剪枝与自有设计（包结构与模块划分、命令注册与解析、FS/网络抽象、
沙箱边界、contrib 模块化、独立发版流程等），不再跟踪上游，也不承诺与之兼容或定期 rebase；
安全修复按需自行处理。

上游归属署名与许可原文见 [NOTICE](NOTICE) 与 [LICENSE](LICENSE)。

## 许可

Apache-2.0（[LICENSE](LICENSE)）。

## 参考集成

[aic-pod](https://github.com/veypi/aic-pod) 把 vsh 嵌在自己的 Go 进程里执行 exec
脚本，只注入本端可用的平台命令、文件系统和原生出口。
