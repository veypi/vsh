# vsh

vsh 是 ivec.ai 平台的进程内 shell 引擎：`github.com/ewhauser/gbash` 的永久 fork
（基线 `88728c5`，Apache-2.0，见 NOTICE / FORK.md），负责 shell 语义——
解析、执行、90+ 内建命令、进程内沙箱原语。它不知道审批、用户、审计的存在；
这些由平台集成层（aic-pod `libs/vsh` glue）注入。

## 定位

- **引擎，不是产品**：Runtime 单例 + 每执行一个 Session，脚本经 `Exec` 运行；
- **一切能力经注入**：命令（组合 Registry：内建 → 平台 → 原生白名单）、文件系统
  （`fs.FileSystem` 适配器，唯一进程内路径权威）、网络（`network.Client`）全部由
  调用方装配，引擎只执行；
- **registry 优先**（fork 分叉点 D14）：命令查找 Registry 命中优先于 PATH 真实
  文件，stub/PATH 目录中的同名真实文件无法 shadow 内建与平台命令；
- **自由分叉**：不承诺跟踪上游 rebase，安全修复按需 cherry-pick（见 FORK.md）。

## 使用

```go
import "github.com/veypi/vsh"

rt, _ := vsh.New(
    vsh.WithRegistry(reg),
    vsh.WithFileSystem(fsFactory),
    vsh.WithNetworkClient(nc),
    vsh.WithLimitOverrides(limits),
)
sess, _ := rt.NewSession(ctx)
res, _ := sess.Exec(ctx, &vsh.ExecutionRequest{Script: s, WorkDir: wd, Env: env})
```

公开扩展点子包：`commands`、`fs`、`policy`、`network`、`shell/syntax`、`trace`。

## 仓库布局

- 根 module `github.com/veypi/vsh`：引擎本体（`internal/` 不对外）；
- `contrib/*`：可选命令模块（jq/yq/awk 等，按需挂入 Registry）；
- `examples/`：用法示例；
- `docs/`：设计文档（design.md = vsh 引擎化契约源，todo.md = 实施跟踪）；
- `scripts/rename.sh`：fork 改名脚本（可重跑，见 FORK.md）。

上游完整文档（SPEC.md / THREAT_MODEL.md / AGENTS.md）保留作参考，其中
"gbash" 字样已机械替换为 "vsh"，语义以代码为准。
