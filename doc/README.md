# Bellkeeper 文档总览

> **Bellkeeper** 是 SilkSpool 的知识治理中台 + LLM 代理网关 + Matrix 控制平面 + 事件驱动平台。
> **1.0 已 GA，稳定运行**。当前状态以 [STATUS.md](STATUS.md) 为准。

---

## 当前状态（随代码常新）

| 文档 | 用途 |
|------|------|
| [STATUS.md](STATUS.md) | **主状态文档** — 主机拓扑、服务分布、观测、数据流、待决事项 |
| [ARCHITECTURE.md](ARCHITECTURE.md) | 系统定位、模块职责、数据模型、关键链路、部署形态 |
| [ROADMAP.md](ROADMAP.md) | 演进待办（前瞻） |
| [TIMELINE.md](TIMELINE.md) | 演进时间线与技术栈选型（历史留痕，只增不改） |

## API 契约

| 文档 | 用途 |
|------|------|
| [API.md](API.md) | 后端 REST 端点参考（`/api/*` + Matrix Admin API） |
| [LLM-GATEWAY-API.md](LLM-GATEWAY-API.md) | LLM 代理池 OpenAI 兼容 API + 进程内直调 Gateway 契约 |

## 规范

| 文档 | 用途 |
|------|------|
| [DEVELOPMENT-GUIDE.md](DEVELOPMENT-GUIDE.md) | 编码规范、分层职责、禁止事项、验收标准 |
| [ASSISTANT-GUIDELINES.md](ASSISTANT-GUIDELINES.md) | AI 助手守则（「完成」定义、防死代码/假测试） |
| [ARCHITECTURE-EXCEPTIONS.md](ARCHITECTURE-EXCEPTIONS.md) | 分层例外登记（范围/原因/护栏/退出计划） |

## 架构决策

- [../docs/adr/](../docs/adr/) — ADR 0001–0006，均现行有效。

---

## 维护规则（强制）

1. **唯一事实源**：现状看 `STATUS.md`，架构看 `ARCHITECTURE.md`，API 看 `API.md`+`LLM-GATEWAY-API.md`，待办看 `ROADMAP.md`，历史看 `TIMELINE.md`。
2. **完成一项**：ROADMAP 打勾 → STATUS 回写 → 大架构变化同步 ARCHITECTURE → 里程碑追加 TIMELINE。
3. **一次性计划/审查（\*-PLAN/\*-REVIEW）完成后从 doc/ 删除**（git 历史留痕），残留待办转 ROADMAP；仍生效的运维/知识库文档归 `doc/ops/`、`doc/knowledge-base/`。
4. **配置即数据**：`rss-sources.json`/`domains.yaml`/`prompts/` 归 `config/`，不进 `doc/`。

---

## 运维与知识库资产

| 目录 | 用途 |
|------|------|
| [ops/](ops/) | 网络（istoreos 子网路由基线）、存储（TrueNAS NFS）、离线应急运维 SOP |
| [knowledge-base/](knowledge-base/) | Obsidian 单 Vault 设计、frontmatter/命名/同步规范、Templater 模板、每周整理 SOP |

> 已完成的一次性计划/审查文档（1.0-REVAMP、CODE-REVIEW、各专项 PLAN）已删除，成果并入现状文档 + [TIMELINE.md](TIMELINE.md) + [ADR](../docs/adr/)，原文可查 git 历史。
