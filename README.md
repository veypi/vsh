# vsh

纯 Go 的**进程内 shell 引擎**，可直接嵌进任意 Go 程序：解析、执行、90+ 内建命令，
命令表 / 文件系统 / 网络出口全部由调用方注入。默认不启动任何宿主程序，也不要求
系统里存在 `bash`。

本仓是从 [gbash](https://github.com/ewhauser/gbash) 永久 fork 而来（基线
`88728c5a0618cf8d8278a6602ae9e1cf05a2159d`，Apache-2.0）。来源、剪枝与改名见
[FORK.md](FORK.md) 与 [NOTICE](NOTICE)；运行契约见 [SPEC.md](SPEC.md)；安全模型见
[THREAT_MODEL.md](THREAT_MODEL.md) 与 [SECURITY.md](SECURITY.md)。

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
	fmt.Print(res.Stdout)      // 3
	fmt.Println(res.ExitCode)  // 0
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

`contrib/*` 是**独立 Go 模块**（各自带 `go.mod`），按需引入：`jq`、`yq`、`awk`、
`python`、`sqlite3`、`bashtool`、`extras`、`htmltomarkdown`、`codingtools`、`nodejs`。
根模块不依赖它们。

## examples

[`examples/`](examples) 是独立模块，覆盖常见嵌入方式：OpenTelemetry 观测、sqlite 作为
文件系统后端、事务化工作区、OpenAI 工具调用、ADK bash 会话、自定义命令（zstd）等。

## 已知边界

- 目标是 **bash 兼容子集**，不是 bash 完整实现；契约以 [SPEC.md](SPEC.md) 为准。
- 默认没有原生执行：`ls` 之类走内建实现，只有显式配置 `WithNativeExec` 才可能碰到真实程序。
- Registry 命中优先于 PATH；显式路径（`/bin/ls`）只访问真实文件，不会生成/修复 stub 目录。
- 单个 Session 内执行是串行的；并发请用多个 Session。
- Windows 的路径与环境在入口按 `/c/...` 规范形归一、OS 出口还原；不解析原生分号
  `PATH`。
- `contrib/jq` 的 stream 错误模式用例在 Go 1.27 下与上游基线同样失败（见
  [FORK.md](FORK.md)），与命令解析无关。

## 测试

```sh
go test ./...
go vet ./...

cd contrib/jq && go test ./...    # contrib 是独立模块，需分别验证
```

部分一致性用例需要外部 oracle（`VSH_CONFORMANCE_RIPGREP`、`VSH_CONFORMANCE_DIFF`
指向固定版本的 ripgrep / diff）；缺少时应报告「未验证」，不能把跳过当通过。

## 版本与许可

- 当前版本见 [VERSION](VERSION)，变更记录见 [CHANGELOG.md](CHANGELOG.md)。
- Apache-2.0（[LICENSE](LICENSE)）；fork 来源署名见 [NOTICE](NOTICE)，与上游的差异
  见 [FORK.md](FORK.md)。
- 不承诺定期 rebase 上游，安全修复按需 cherry-pick（步骤见 FORK.md）。

## 参考集成

[aic-pod](https://github.com/veypi/aic-pod) 把 vsh 嵌在自己的 Go 进程里执行 exec
脚本，只注入本端可用的平台命令、文件系统和原生出口。`docs/todo.md` 是这条集成路线的
内部开发记录，不属于库 API。
