# SilkSpool 项目进度

> 最后更新: 2026-06-12
>
> 本文档是跨仓库的全局进度视图(SilkSpool IaC + Bellkeeper 应用 + n8n 工作流)。
> 演进规划见 [ROADMAP.md](ROADMAP.md);已完成计划的原始文档在 [archive/](archive/)。

---

## 当前架构口径(2026-06)

```
                        Matrix(控制平面)
                              │
            ┌─────────────────┼────────────────┐
            ▼                 ▼                ▼
       Bellkeeper      n8n(编排层)         TrueNAS
       Matrix Gateway  K* / M* / O*        data/knowledge/
       LLM Proxy                            ├ raw/      (爬虫落盘,不进索引/不同步)
       LLM Job Queue                        ├ archive/  (中分留档,进 Meili)
       CrawlQueue                           ├ vault/    (高分原子卡片,进 Meili)
       Agent(AI工具)                         └ notes-assets/
       DailyReport                                ▲
                                                   │ LiveSync(CouchDB)
                                        Obsidian Vault(PKB 真相源)
```

**核心定位**:
- **Markdown / Obsidian Vault** 是知识真相源(PKB 层);**pkb-curate** 负责 raw→archive/vault 漏斗与原子化重构
- **Bellkeeper** 是治理中台(爬取队列、入库、分类、检索、LLM 代理、Matrix Gateway、Agent、n8n 工作流定义事实源)
- **n8n** 仅承担定时编排与跨服务粘合
- **RAGFlow 已完全退役**:服务不再部署,代码兼容层已在 Phase 1 清理完毕

---

## 模块成熟度

| 模块 | 状态 | 说明 |
|------|------|------|
| 基础设施 | ✅ 稳定 | 6 台主机、反代、Headscale VPN、IaC |
| Bellkeeper 后端 | ✅ 稳定 | 知识入库 + LLM Proxy + Matrix Gateway + CrawlQueue + Agent + DailyReport + PKB |
| Bellkeeper 前端 | ✅ 稳定 | 四大核心域(Knowledge / LLM / Logs / Matrix);LLM 域收敛为 5 页 |
| LLM 代理 | ✅ 开发完成 | Tier 0–9 整改落地:Token/计费/任务感知分层路由/会话粘性/自适应限流/错误码熔断/真实余额/Gemini/Rerank 协议;🔶 运行时验收项待用户自验(见 ROADMAP §3) |
| LLM 持久队列 | ✅ 稳定 | `llm_jobs` + worker,内部批处理统一排队 |
| 个人知识库 PKB | ✅ MVP | `pkb-curate` CLI:打分分流(raw/archive/vault)→ 原子化重构 → digest 综述;提示词外置 `config/pkb/`;下一代「原子知识网」计划进行中(PKB-ATOMIC-KNOWLEDGE-PLAN) |
| 爬取与提取 | ✅ 增强完成 | 域名级限速/健康画像、LLM 规则优化闭环、错误分类退避、RSS 按源调度 |
| 标签管线 | ✅ 增强完成 | 置信度 + 规范化 + tag_source 三处持久化 |
| RSS | ✅ 稳定 | feed 验证 API + RSSHub 参数支持 |
| Meilisearch 检索 | ✅ 稳定 | `/api/files/search\|ask`,archive+vault 三层索引隔离 |
| Matrix 控制平面 | ✅ 稳定 | Gateway + Command Router + Agent(6只读工具+3写工具+workflow触发) + 通知网关(NATS) + 权限两层制 + 通知聚合去重 + Admin API |
| Agent 系统 | ✅ MVP | AgentService(回合循环+工具执行)+ Redis 会话(房间级多轮,20条上限,30min TTL)+ 限速(每房间30回合/小时)+ 权限分级(readonly/write/danger) |
| 日报系统 | ✅ 稳定 | 后端驱动(DailyReportService)+ 并行采集器 + n8n 仅触发;O02/K08 已退役 |
| n8n 工作流治理 | ✅ 落地 | `internal/n8n_workflows/` JSON 事实源 + Web/API 管理 |
| 日志中心 | 🚧 进行中 | 骨架已有;全文检索、告警增强、trace 关联待做 |
| 架构治理 | ✅ 完整 | Phase 0-10 全部完成:审查→止血→重构→测试→lint 清零→Agent+API补齐 |
| 认证 | ✅ 无需 | 生产运行在纯内网,noauth 模式为预期状态;LLM Token 鉴权独立保留 |
| 测试 | ✅ 核心覆盖 | 31 Repository 全覆盖(PG集成测试) + 核心链路行为测试 + LLM 协议转换测试 |
| lint | ✅ 零 error | golangci-lint v2;error 清零,仅剩 3 个可接受 warning |

---

## 最近主线动作

| 时间 | 动作 |
|------|------|
| 2026-06-12 | Phase 9-10 (T5-T8): Agent MVP + 写工具 + API 补齐 + 前端对齐;v1.0.0 收尾 |
| 2026-06-11 | Matrix 平台改造计划定稿(matrix-platform-overhaul-plan.md,T1-T9) |
| 2026-06-10 | LLM Proxy / PKB 提示词体系审查;文档大整理 |
| 2026-06-09/10 | PKB 原子知识网改进计划定稿;爬取/标签/RSSHub 优化落地;架构审查整改;n8n 纳管 |
| 2026-06-08 | LLM 持久任务队列;架构审查;PKB 免费池退避/digest/提示词治理 |
| 2026-06-07 | LLM UI 重设计落地(10→5 页);PKB 混合模型 |
| 2026-06-06 | PKB Step1–3 落地:三层索引隔离 + pkb-curate CLI + 提示词包 |
| 2026-05-30~06-01 | LLM Proxy Tier 0–9 审计修复 |
| 2026-04 | 前端四大域重构;CrawlQueue 上线;切换 Meilisearch;Matrix Gateway 上线;RAGFlow 退役 |

---

## 已知问题与待办(摘要)

详细见 [ROADMAP.md](ROADMAP.md):

| 类型 | 摘要 |
|------|------|
| PKB | 存量 ~308 篇 raw 待批跑 + cron 固化;原子知识网计划 Phase A–E |
| LLM | 运行时验收(prompt cache/限流学习/余额);提示词 P0 修复 6 项;new-api 停服暂缓 |
| 爬虫 | 新源批量导入与 7 天成功率验收;周健康报告 |
| 日志 | 全文检索、告警规则增强、trace_id 关联 |
| 前端 | 爬取队列可视化页缺失;Datasets 页仍含 RAGFlow 概念;Vault 预览/问答多轮 |
| Matrix | 前端 7→3 页重构(overhaul plan T9) |

---

## 文档导航

- [doc/README.md](README.md) — 文档总览与目录结构
- [doc/ROADMAP.md](ROADMAP.md) — 演进规划
- [doc/ARCHITECTURE.md](ARCHITECTURE.md) — 架构文档
- [doc/API.md](API.md) — REST API 参考
- [doc/archive/](archive/) — 已完成计划与历史评估归档

---

## 核心设计决策

| 选择 | 原因 |
|------|------|
| Markdown / Obsidian Vault 为知识真相源 | 数据主权、纯文本、长寿、可手工整理 |
| PKB 用一次性 CLI(pkb-curate)而非常驻 service / agent 框架 | 流程固定的 LLM 批处理,无需自主 agent;提示词外置 config/pkb 可调方向不改代码 |
| Meilisearch 替代 RAGFlow | 轻量、文件级派生、无需重型向量库 |
| Bellkeeper 自建 LLM Proxy 对标 new-api | 可深度定制(任务感知路由/真实余额/限流学习),与 Matrix/日志/配置体系融合 |
| n8n 仅做编排 | 重逻辑下沉 Bellkeeper |
| 三层知识模型(raw/archive/vault) | 漏斗分流,raw 永不进 Obsidian,根治信息垃圾场 |
| Agent 走 function calling(OpenAI schema) | 复用 LLM Proxy 已有能力,无需额外 agent 框架 |
| 日报后端驱动 | 消除 n8n Code Node 数据逻辑,口径一致 |
