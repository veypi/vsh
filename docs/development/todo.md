# vsh 未来开发计划

状态：2026-10-27。

本文只记录虚拟 shell 自身的未完成工作，不描述具体嵌入产品或外部进程沙箱实现。已完成事实见根目录 `CHANGELOG.md`，现行契约见 `docs/reference/spec.md`。

## 跨平台验证

- 在 linux / windows 真实环境运行统一 `AllowExec`、文件命令解析器、嵌套执行和拒绝无副作用回归；当前开发机结果不能替代这些平台的行为验证。

## 测试环境

- `shell/expand` 的 `TestFieldsGlobIgnoreCharClass` 在当前设备执行环境中创建临时 `.env` 时返回 `operation not permitted`；该失败在本次命令架构改动前已存在，需要在可写临时目录的测试环境复验，不能把它记作本阶段通过。
