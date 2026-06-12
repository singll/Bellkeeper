# Bellkeeper 文档总览

> **Bellkeeper** 是 SilkSpool 的知识治理中台 + LLM 代理网关 + Matrix 控制平面。
> 本目录被 `.gitignore` 忽略,仅本地维护。**进度状态的单一事实源是 [STATUS.md](STATUS.md),演进规划是 [ROADMAP.md](ROADMAP.md)**;已完成计划的原始文档归档在 [archive/](archive/),不要在归档文档上继续追加内容。

---

## 目录结构与维护规则

```
doc/
├── README.md                    # 本文件:导航
├── STATUS.md                    # 现状(模块成熟度 + 最近动作)← 完成事项回写这里
├── ROADMAP.md                   # 演进规划(活跃待办)← 新任务加这里
├── ARCHITECTURE.md              # 架构事实(模块/分层/数据模型/链路)
├── ARCHITECTURE-EXCEPTIONS.md   # 分层例外登记
├── API.md                       # REST API 端点参考
├── LLM_PROXY_GUIDE.md           # LLM Proxy 使用与配置指南
├── DEVELOPMENT-GUIDE.md         # 编码规范(权威,较长)
├── ASSISTANT-GUIDELINES.md      # AI 助手开发守则
├── PKB-IMPLEMENTATION.md        # PKB MVP 实施文档(活跃)
├── PKB-ATOMIC-KNOWLEDGE-PLAN.md # PKB 原子知识网改进计划(实施中 ⭐)
├── LLM-PROMPT-AGENT-REVIEW.md   # LLM/PKB 提示词体系审查(2026-06-10)
├── archive/                     # 已完成计划与历史评估(只读)
└── architecture/ documents/ matrix/ modules/ rss/   # 模块级文档
```

**文档生命周期**:计划类文档(`*-PLAN` / `*-REVIEW` / 实施蓝图)完成后移入 `archive/`,残留待办转入 ROADMAP,完成事实回写 STATUS;事实类文档(ARCHITECTURE / API / GUIDE)随大改动同步更新,不留过时描述。

---

## 快速导航

### 开发必读

| 文档 | 用途 |
|------|------|
| [DEVELOPMENT-GUIDE.md](DEVELOPMENT-GUIDE.md) | **编码规范** — 架构说明、编码标准、禁止事项 |
| [ASSISTANT-GUIDELINES.md](ASSISTANT-GUIDELINES.md) | **AI 助手行为标准** — 「完成」的定义、防死代码/假测试 |
| [ARCHITECTURE-EXCEPTIONS.md](ARCHITECTURE-EXCEPTIONS.md) | 分层例外登记(新增例外必须登记) |

### 参考文档

| 文档 | 用途 |
|------|------|
| [ARCHITECTURE.md](ARCHITECTURE.md) | 系统定位、模块职责、数据模型、关键链路(2026-06-10 更新) |
| [API.md](API.md) | REST API 端点参考(基于 router.go,2026-06-10 更新) |
| [LLM_PROXY_GUIDE.md](LLM_PROXY_GUIDE.md) | LLM Proxy:DB 动态配置、Token、模型组、任务路由、计费(2026-06-10 重写) |

### 演进规划

| 文档 | 用途 |
|------|------|
| [ROADMAP.md](ROADMAP.md) | 活跃待办与优先级(2026-06-10 重新规划) |
| [STATUS.md](STATUS.md) | 实施状态与最新动作 |

### 知识库(PKB)

| 文档 | 状态 | 用途 |
|------|------|------|
| [PKB-IMPLEMENTATION.md](PKB-IMPLEMENTATION.md) | MVP 已落地 | 三层漏斗 + pkb-curate CLI + 提示词外置的实施文档;运行残留项见 ROADMAP |
| [PKB-ATOMIC-KNOWLEDGE-PLAN.md](PKB-ATOMIC-KNOWLEDGE-PLAN.md) | ⭐ 实施中 | 下一代「原子知识网」:原子卡 + 语义去重 + 分层 MOC + 网可观测(Tier 1–6 / Phase A–E) |
| [LLM-PROMPT-AGENT-REVIEW.md](LLM-PROMPT-AGENT-REVIEW.md) | 审查结论 | LLM Proxy 与 PKB 的 agent/提示词架构分析与优化建议(P0 修复项已转 ROADMAP) |

---

## 功能模块状态

| 模块 | 状态 | 文档入口 |
|------|------|---------|
| **LLM 代理池** | ✅ 开发完成(运行时验收待做) | [LLM_PROXY_GUIDE.md](LLM_PROXY_GUIDE.md) |
| **个人知识库 PKB** | ✅ MVP(原子知识网升级中) | [PKB-IMPLEMENTATION.md](PKB-IMPLEMENTATION.md) |
| **文件入库 + 知识检索** | ✅ 稳定 | [documents/](documents/) |
| **爬取队列 CrawlQueue** | ✅ 稳定(06-09 智能化增强) | [rss/EXTRACTION-PIPELINE.md](rss/EXTRACTION-PIPELINE.md) |
| **RSS 与提取** | ✅ 稳定 | [rss/](rss/) |
| **Matrix 控制平面** | ✅ 稳定 | [matrix/](matrix/) |
| **标签 + 分类路由** | ✅ 稳定(06-09 管线增强) | — |
| **日志中心 LogCenter** | 🚧 进行中 | — |
| **RAGFlow 兼容层** | ⚠️ 待清理(~8 文件) | ROADMAP §RAGFlow |

---

## 模块文档索引

### 文件入库与检索(documents/)
- [documents/README.md](documents/README.md) — 模块总览
- [documents/FILE-INGESTION.md](documents/FILE-INGESTION.md) — 入库流程与提取器编排
- [documents/FILE-INDEXING.md](documents/FILE-INDEXING.md) — Meilisearch 索引模型
- [documents/SEARCH-AND-QA.md](documents/SEARCH-AND-QA.md) — 搜索与问答 API
- [documents/NFS-SETUP.md](documents/NFS-SETUP.md) — TrueNAS NFS 挂载

### Matrix 控制平面(matrix/)
- [matrix/README.md](matrix/README.md) — 平台总览(架构/数据模型/API 契约/命令模型等子文档见目录)
- [matrix/notifications/](matrix/notifications/) — 通知平面

### RSS 与提取(rss/)
- [rss/README.md](rss/README.md) — RSS 模块总览
- [rss/EXTRACTION-PIPELINE.md](rss/EXTRACTION-PIPELINE.md) — 提取链路设计

### 架构与基础设施(architecture/)
- [architecture/overview.md](architecture/overview.md) — 系统架构总览
- [architecture/infrastructure.md](architecture/infrastructure.md) — 主机清单、网络拓扑
- [architecture/offline-fallback.md](architecture/offline-fallback.md) — VPN 应急策略
- [architecture/istoreos-headscale-subnet-route-baseline.md](architecture/istoreos-headscale-subnet-route-baseline.md) — 子网路由基线

### 周边模块(modules/)
- [modules/automation/](modules/automation/README.md) n8n 编排 · [modules/knowledge/](modules/knowledge/README.md) 知识工作层 · [modules/matrix/](modules/matrix/README.md) Matrix · [modules/notes/](modules/notes/README.md) Obsidian 笔记系统 · [modules/rss/](modules/rss/README.md) RSS 采集 · [modules/storage/](modules/storage/README.md) TrueNAS 存储

### 数据资产
- [rss-sources.json](rss-sources.json) — RSS 源种子文件

---

## 归档(archive/,只读)

| 文档 | 归档原因 |
|------|---------|
| [ARCHITECTURE-REVIEW-2026-06-08.md](archive/ARCHITECTURE-REVIEW-2026-06-08.md) | 架构审查报告;P0/P1 整改已于 06-09 落地,残留项转 ROADMAP |
| [LLM-UI-REDESIGN.md](archive/LLM-UI-REDESIGN.md) | LLM 前端 10→5 页收敛 + 统一凭证模型,06-07 落地 |
| [CRAWL-TAGGING-RSSHUB-OPTIMIZATION-PLAN-2026-06-09.md](archive/CRAWL-TAGGING-RSSHUB-OPTIMIZATION-PLAN-2026-06-09.md) | 爬取/标签/RSSHub 优化 P0–P4 代码侧 06-09 全部落地;运营验收转 ROADMAP |
| [MEMOS-TODOTXT-FUSION.md](archive/MEMOS-TODOTXT-FUSION.md) / [SLEEK-VS-MEMOS.md](archive/SLEEK-VS-MEMOS.md) | 历史工具评估快照 |
