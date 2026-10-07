# Changelog

本仓版本从 0.1.0（fork 起点）开始记录；0.1.0 之前的历史即上游
[gbash](https://github.com/ewhauser/gbash)（基线 `88728c5`，见 [FORK.md](FORK.md)）。
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

### 新增

- 导出 `commands.BrokenPipe`：断管（EPIPE）判定收敛为单点，内建命令与自定义命令
  共用同一实现。

### 修复

- **重定向落盘错误不再静默**：虚拟文件系统在 Close 时落盘，此前 `Close()` 的错误被
  丢弃——配额拒绝或写回失败会被报成 exit 0（例如 `>>` 追加超限静默丢数据）。现在
  Close 失败终结语句并返回非零退出。
- **PATH 搜索**：修复 Registry 与真实 PATH 混排时的解析与回退顺序（含显式路径场景）。
- **`access` 内建**：root 身份按 POSIX 真实行为绕过权限位检查。

### 文档

- README 重写为独立库用法（嵌入 API、命令行、扩展点、contrib、examples、已知边界）。
- 新增本变更记录；`docs/todo.md` 明确标注为集成路线的内部开发记录。

## 0.1.0 — 2026-09-29

- 自 gbash 基线 `88728c5a0618cf8d8278a6602ae9e1cf05a2159d` fork：module 与包名改为
  `vsh`，剪枝上游 website / 打包 / CI 资产，保留 `contrib/`、`examples/` 与上游规格
  文档（SPEC.md、THREAT_MODEL.md、SECURITY.md、AGENTS.md）。细节见 [FORK.md](FORK.md)。
