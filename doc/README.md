# Bellkeeper 1.0 文档总览

> **Bellkeeper** 是 SilkSpool 的知识治理中台 + LLM 代理网关 + Matrix 控制平面 + 事件驱动平台。
> **权威事件源**：[BELLKEEPER-1.0-REVAMP-PLAN.md](BELLKEEPER-1.0-REVAMP-PLAN.md) —— 1.0 重构终极统一收口（Single Source of Truth）。
> **实施状态**：[STATUS.md](STATUS.md)；**演进规划**：[ROADMAP.md](ROADMAP.md)。
> 已完成计划的原始文档归档在 [archive/](archive/)，仅只读，禁止追加。

---

## 1.0 文档体系

| 文档 | 层级 | 用途 |
|------|------|------|
| [BELLKEEPER-1.0-REVAMP-PLAN.md](BELLKEEPER-1.0-REVAMP-PLAN.md) | 🏠 规划 | 1.0 重构终极统一收口（M1-M5 方案 + §4 待办 + 里程碑） |
| [ARCHITECTURE.md](ARCHITECTURE.md) | 📐 架构 | 系统定位、模块职责、数据模型、关键链路（1.0 终态） |
| [LLM-GATEWAY-API.md](LLM-GATEWAY-API.md) | 📡 API | LLM 代理池对外 OpenAI 兼容 API 契约（进程内直调 + HTTP 兼容） |
| [API.md](API.md) | 📡 API | REST API 端点参考（后端 /api/* 路由 + Matrix Admin API） |
| [STATUS.md](STATUS.md) | 📊 状态 | 模块成熟度评分 + 里程碑状态回写 |
| [ROADMAP.md](ROADMAP.md) | 🗺️ 规划 | 演进待办（镜像 BELLKEEPER-1.0-REVAMP-PLAN §4） |

### 编码规范

| 文档 | 用途 |
|------|------|
| [DEVELOPMENT-GUIDE.md](DEVELOPMENT-GUIDE.md) | 编码规范、分层职责、禁止事项、验收标准 |
| [ASSISTANT-GUIDELINES.md](ASSISTANT-GUIDELINES.md) | AI 助手守则（「完成」定义、防死代码/假测试） |
| [ARCHITECTURE-EXCEPTIONS.md](ARCHITECTURE-EXCEPTIONS.md) | 分层例外登记（LLM 两条已清零 ✅） |

### 审查与里程碑

| 文档 | 用途 |
|------|------|
| [MILESTONES-1.0.md](MILESTONES-1.0.md) | M1-M5 里程碑详细实施记录 |
| [CODE-REVIEW-1.0-FINAL.md](CODE-REVIEW-1.0-FINAL.md) | 1.0 GA 终审报告 |

---

## 维护规则（强制）

1. **唯一事实源**：架构看 `ARCHITECTURE.md`，API 看 `API.md`+`LLM-GATEWAY-API.md`，待办看 `BELLKEEPER-1.0-REVAMP-PLAN.md` §4，状态看 `STATUS.md`。
2. **新任务**一律加到 `BELLKEEPER-1.0-REVAMP-PLAN.md` §4，不另开新 PLAN 文档。
3. **完成一项**：§4 打勾 → STATUS.md 回写 → 大架构变化同步 ARCHITECTURE.md。
4. **禁止在归档文档上追加内容**。
5. **配置即数据**：`rss-sources.json`/`domains.yaml`/`prompts/` 归 `config/`，不进 `doc/`。

---

## 归档（archive/，只读）

| 文档 | 归档原因 |
|------|---------|
| PKB-IMPLEMENTATION.md | MVP 已落地，残留项转 1.0 规划 §4 |
| PKB-ATOMIC-KNOWLEDGE-PLAN.md | Phase A-D 已完成 |
| PKB-KNOWLEDGE-SKELETON-PLAN.md | Phase F-H 已完成 |
| KNOWLEDGE-MODULE-REVAMP-PLAN.md | 阶段 1-2 已完成 |
| LLM-PROMPT-AGENT-REVIEW.md | P0 已修 |
| LLM_PROXY_GUIDE.md | 被 LLM-GATEWAY-API.md 取代 |
| matrix-platform-overhaul-plan.md | T1-T8 已完成 |
| notification-monitoring-overhaul-plan.md | 已落地 |
| reliability-audit-plan.md | Tier 2-3 已完成 |
| daily-report-fix-plan.md | 已落地 |
| matrix/ 全部 | 合并进 ARCHITECTURE.md Matrix 章节 |
| modules/ 全部 | 模块总览，合并进 ARCHITECTURE.md |
| rss/ + documents/ | 合并进 ARCHITECTURE.md 知识库章节 |
| architecture/ | 基础设施类，合并进 ARCHITECTURE.md |
| 以上所有 + 历史评估 | 详见 [archive/](archive/) 目录 |
