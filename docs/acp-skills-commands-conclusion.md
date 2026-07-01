# ACP (Agent Client Protocol) Skills / Slash Commands 结论

> 基于 ACP 官方文档调研，回答：ACP 是否定义了获取 agent skills 或 slash commands 的方式？

---

## 结论：ACP 协议层**不定义** skills / slash commands

无论是 **Initialization** 还是 **Additional Directories RFD**，ACP 协议都明确将 skills、slash commands、instruction directory 等内容排除在协议范围之外。

---

## 详细说明

### 1. Initialization 阶段

**端点**: `POST /protocol/v1/initialization`

返回内容仅包含：

| 字段 | 说明 |
|------|------|
| `protocolVersion` | 协议版本协商 |
| `agentCapabilities` | 高层级能力声明（MCP、session load、prompt 内容类型等） |
| `agentInfo` | agent 名称、标题、版本 |
| `authMethods` | 认证方式 |

**没有**任何字段用于列出 skills、slash commands 或工具列表。

### 2. Additional Directories RFD

**文档**: `/rfds/additional-directories`

FAQ 明确回答：

> **Q: Does ACP define `.agents`, `skills`, or instruction directory conventions?**
>
> **A: No.** ACP does not define `.agents`, `skills`, or other instruction directory conventions. This proposal only defines additional accessible roots.

非目标 (Non-goals)：

- 不规定必须的目录名或布局（如 `.agents/`、`skills/`、`.<agent>/`）
- 不定义标准的 instruction、skill 或配置文件格式
- 不定义发现行为（discovery behavior）

---

## 如果需要列出 skills / commands，替代方案

| 方案 | 说明 |
|------|------|
| **MCP tools/list** | 通过 MCP 协议的 `tools/list` 获取 agent 注册的工具（仅限于 MCP 层面） |
| **自定义 `_meta` 扩展** | ACP 支持通过 `_meta` 字段扩展自定义能力，需双方约定格式 |
| **实现自行定义** | skills / commands 完全由各 agent 实现自行定义，ACP 不做规定 |

---

## 一句话总结

> ACP 只负责 client 与 agent 之间的**连接协商**和**文件系统根目录声明**，Skills、Slash Commands、Instruction 目录等均属于**实现层关注点**，不由协议标准化。
