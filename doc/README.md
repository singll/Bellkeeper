# Bellkeeper 文档总览

> **Bellkeeper** 是 SilkSpool 的知识治理中台与 LLM 代理网关，提供文件入库、爬取队列、LLM 代理池、Matrix 控制平面、Meilisearch 检索等核心能力。
>
> **定位**: 做 n8n 做不好的事 — 有状态的限速、去重、爬取队列、路由、日志、治理；同时承担 Matrix Gateway 和 LLM Proxy 两个长连接服务。

---

## 快速导航

### 开发必读

| 文档 | 用途 |
|------|------|
| [开发指南 (DEVELOPMENT-GUIDE.md)](DEVELOPMENT-GUIDE.md) | **编码规范** — 架构说明、编码标准、禁止事项 |
| [AI 助手守则 (ASSISTANT-GUIDELINES.md)](ASSISTANT-GUIDELINES.md) | **AI 助手行为标准** — 「完成」的定义、禁止事项、验证要求 |

### 参考文档

| 文档 | 用途 |
|------|------|
| [架构总览 (ARCHITECTURE.md)](ARCHITECTURE.md) | 系统定位、模块职责、技术选型 |
| [API 文档 (API.md)](API.md) | 完整 REST API 参考 |
| [LLM 代理池指南 (LLM_PROXY_GUIDE.md)](LLM_PROXY_GUIDE.md) | 渠道配置、模型组、熔断器使用 |

### 演进规划

| 文档 | 用途 |
|------|------|
| [ROADMAP.md](ROADMAP.md) | 当前优化方向（LLM Proxy / n8n / 日志 / 界面与功能） |
| [STATUS.md](STATUS.md) | 实施状态与最新动作 |

---

## 功能模块

| 模块 | 状态 | 文档入口 | 说明 |
|------|------|---------|------|
| **LLM 代理池** | ✅ 稳定 | [LLM_PROXY_GUIDE.md](LLM_PROXY_GUIDE.md) | 多渠道限速代理、虚拟模型组、熔断、粘性、DB 动态配置、Anthropic + OpenAI 协议 |
| **文件入库 + 知识检索** | ✅ 稳定 | [documents/](documents/) | Trafilatura/Firecrawl 提取 → Markdown 落盘 → Meilisearch 索引；Obsidian Vault 为主库 |
| **爬取队列 (CrawlQueue)** | ✅ 稳定 | — | 持久化任务队列 + Worker 池 + 熔断 + 反爬，2026-04 新增 |
| **RSS 与提取** | ✅ 稳定 | [rss/](rss/) | RSS 订阅管理、正文提取策略 |
| **Matrix 控制平面** | ✅ 稳定 | [matrix/](matrix/) | Gateway (mautrix-go sync) + Command Router + 通知网关 (NATS) + Admin API + 前端 |
| **标签 + 分类路由** | ✅ 稳定 | — | LLM 驱动分类，标签作为知识库分区元数据 |
| **日志中心 (LogCenter)** | 🚧 进行中 | — | entries/sources/dashboard/alerts；分析与告警规则待完善 |
| **活动日志 (ActivityLog)** | ✅ 稳定 | — | 跨模块审计：crawl/ragflow_upload/classify 等 |
| **RAGFlow 兼容层** | ⚠️ 待清理 | — | 历史代码仍在编译进二进制，路由仍注册，但主链不再使用 |

**图例**: ✅ 稳定 | 🚧 进行中 | ⚠️ 待清理

---

## 模块文档索引

### 文件入库与检索 (documents/)

- [documents/README.md](documents/README.md) — 模块总览
- [documents/FILE-INGESTION.md](documents/FILE-INGESTION.md) — 入库流程与提取器编排
- [documents/FILE-INDEXING.md](documents/FILE-INDEXING.md) — Meilisearch 索引模型与派生物边界
- [documents/SEARCH-AND-QA.md](documents/SEARCH-AND-QA.md) — 搜索与问答 API
- [documents/NFS-SETUP.md](documents/NFS-SETUP.md) — TrueNAS NFS 挂载配置

### Matrix 控制平面 (matrix/)

- [matrix/README.md](matrix/README.md) — Matrix 平台总览
- [matrix/MATRIX-ARCHITECTURE.md](matrix/MATRIX-ARCHITECTURE.md) — 架构设计
- [matrix/MATRIX-DATA-MODEL.md](matrix/MATRIX-DATA-MODEL.md) — 数据模型
- [matrix/MATRIX-API-CONTRACTS.md](matrix/MATRIX-API-CONTRACTS.md) — API 契约
- [matrix/MATRIX-COMMAND-MODEL.md](matrix/MATRIX-COMMAND-MODEL.md) — 命令模型
- [matrix/MATRIX-IMPLEMENTATION-PLAN.md](matrix/MATRIX-IMPLEMENTATION-PLAN.md) — 实施计划
- [matrix/IMPLEMENTATION-CHECKLIST.md](matrix/IMPLEMENTATION-CHECKLIST.md) — 实施清单
- [matrix/notifications/](matrix/notifications/) — 通知平面文档

### RSS 与提取 (rss/)

- [rss/README.md](rss/README.md) — RSS 模块总览
- [rss/EXTRACTION-PIPELINE.md](rss/EXTRACTION-PIPELINE.md) — 提取链路设计

### 架构总览 (architecture/)

- [architecture/overview.md](architecture/overview.md) — 系统架构总览
- [architecture/infrastructure.md](architecture/infrastructure.md) — 主机清单、网络拓扑、IaC 配置
- [architecture/offline-fallback.md](architecture/offline-fallback.md) — VPN 不可用时的应急策略
- [architecture/istoreos-headscale-subnet-route-baseline.md](architecture/istoreos-headscale-subnet-route-baseline.md) — iStoreOS + Headscale 子网路由配置基线

### 工具评估 (evaluations/)

- [evaluations/MEMOS-TODOTXT-FUSION.md](evaluations/MEMOS-TODOTXT-FUSION.md) — Memos 与 Todo.txt 融合方案评估
- [evaluations/SLEEK-VS-MEMOS.md](evaluations/SLEEK-VS-MEMOS.md) — Sleek vs Memos 对比

### 功能模块 (modules/)

- [modules/automation/README.md](modules/automation/README.md) — n8n 自动化编排
- [modules/knowledge/README.md](modules/knowledge/README.md) — 知识工作层与检索增强
- [modules/matrix/README.md](modules/matrix/README.md) — Matrix 控制平面
- [modules/notes/README.md](modules/notes/README.md) — Obsidian 笔记系统
- [modules/rss/README.md](modules/rss/README.md) — RSS 采集链路
- [modules/storage/README.md](modules/storage/README.md) — TrueNAS 存储分层

### 数据资产

- [rss-sources.json](rss-sources.json) — RSS 源种子文件
