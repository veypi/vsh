# vsh 文档地图

统一架构：根目录只放 `README.md` / `README.en.md` / `CHANGELOG.md` / `AGENTS.md` /
`SECURITY.md` / `VERSION`，其余文档进 `docs/`，按 `architecture/`（架构设计）、
`reference/`（契约规范）、`guides/`（使用操作）、`development/`（计划与过程文档）分类。

## 入口

| 文档 | 内容 | 何时读 |
| --- | --- | --- |
| [README.md](../README.md) / [README.en.md](../README.en.md) | 定位、安装、快速开始、CLI、扩展点、contrib 表、平台矩阵、发布方式 | 使用本库前 |
| [AGENTS.md](../AGENTS.md) | 工程约定、SPEC 同步纪律、发布与版本规则 | 在本仓写代码前必读 |
| [SECURITY.md](../SECURITY.md) | 安全支持面与报告渠道 | 报告安全问题时 |
| [CHANGELOG.md](../CHANGELOG.md) | 已完成变更（根模块）；contrib 各自带 CHANGELOG | 发版与升级时 |
| [VERSION](../VERSION) | 根模块当前版本 | 发版时 |

## reference

| 文档 | 内容 | 何时读 |
| --- | --- | --- |
| [reference/spec.md](reference/spec.md) | 现行架构契约：脚本语义、命令解析、统一准入 `AllowExec`、虚拟资源、包布局 | 改运行时或命令策略边界前必读；契约变化需同轮更新 |
| [reference/threat-model.md](reference/threat-model.md) | 威胁模型与边界：vsh 不是 OS 沙箱，能力由调用方注入 | 评估安全语义、接入新能力前 |

## development

| 文档 | 内容 | 何时读 |
| --- | --- | --- |
| [development/todo.md](development/todo.md) | 未完成工作（跨平台验证、测试环境限制） | 评估计划与验证缺口时；已完成事实不写在这里 |

## 随代码的子文档

| 文档 | 内容 |
| --- | --- |
| [commands/AGENTS.md](../commands/AGENTS.md) | 命令作者参考（注册、准入、测试） |
| [cmd/README.md](../cmd/README.md) | 命令行入口 |
| [contrib/README.md](../contrib/README.md) | 独立 contrib 模块（awk/jq/sqlite3/yq 等） |
| [internal/builtins/AGENTS.md](../internal/builtins/AGENTS.md) | 内建命令工作笔记 |
| [examples/](../examples/) | 各嵌入示例自带的 README |

## 约定

- 文档只描述通用虚拟 shell 库，不写具体嵌入产品或外部进程沙箱的配置域。
- 已完成的变更写 CHANGELOG，未来计划只写 `development/todo.md`。
- SPEC 是唯一的架构契约真相；其他文档不得与之并列一套说法。
