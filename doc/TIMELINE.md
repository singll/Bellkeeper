# Bellkeeper 演进时间线

> 本文件是**唯一的历史记录**：技术栈演进 + 发展路线里程碑。
> 只增不改，一条里程碑一行（日期 + 结果）。当前状态见 [STATUS.md](STATUS.md)，架构见 [ARCHITECTURE.md](ARCHITECTURE.md)。
> 各里程碑的一次性计划文档已删（可查 git 历史）；仍生效的运维/知识库文档见 [ops/](ops/)、[knowledge-base/](knowledge-base/)。

---

## 关键技术选型演进

| 维度 | 早期 | 现状 | 原因 |
|------|------|------|------|
| 检索 | RAGFlow（重型向量库） | **Meilisearch** | 轻量、文件级派生、无需重型向量库；RAGFlow 已完全退役 |
| LLM 接入 | new-api（第三方聚合） | **自建 LLM Proxy（llmgateway）** | 任务感知路由 / 真实余额 / 限流学习，与 Matrix/日志/配置融合 |
| 知识治理 | n8n 无脑爬取入库 | **PKB 三层漏斗 + 知识骨架 + 双库** | 根治"信息垃圾场"，raw→archive/vault 分流 + 结构化合成 |
| 爬虫调度 | 内存 DomainCoordinator（三桶分层） | **DB 公平调度 DequeueFair + 冷却** | 无状态、可观测、公平轮转，见 ADR-0003 |
| 模块通信 | Service 直接互调 | **NATS JetStream 事件总线** | 解耦、可追溯（TraceID）、崩溃兜底 |
| 部署形态 | keeper 单机全栈 | **app 层(keeper) ↔ 数据层(silkdata) 分离** | 生命周期解耦、备份独立、爆炸半径隔离 |
| 可观测性 | 仅 `/metrics` 端点 | **Prometheus + Loki + Grafana 集中栈** | 三机指标/日志集中，grafana.singll.net |
| 认证 | Authelia Forward Auth | **noauth（纯内网）** | 生产纯内网、无公网暴露；LLM Token 鉴权独立保留 |

---

## 发展路线里程碑

### 2026-04 · 起步
- 前端四大域重构；CrawlQueue 上线；检索切换 Meilisearch；Matrix Gateway 上线；**RAGFlow 退役启动**。

### 2026-05-30 ~ 06-01 · LLM Proxy 地基
- LLM Proxy Tier 0–9 审计整改：Token / 计费 / 余额 / 路由 / 粘性 / 限流 / 熔断 / 告警 / Gemini / Rerank。

### 2026-06-06 ~ 06-09 · PKB MVP + 优化冲刺
- **PKB MVP**：三层存储（raw/archive/vault）+ `pkb-curate` CLI + digest 综述 + 提示词外置 + `llm_jobs` 队列 + n8n 纳管。
- LLM UI 10→5 页收敛 + 统一凭证模型。
- 爬取/标签/RSSHub 优化 P0–P4（域名限速 / LLM 规则 / 标签置信度 / feed 验证）。
- 架构审查 P0/P1 整改（token 权限 / 路径逃逸 / 并发 / 契约 / 迁移 / lint）。

### 2026-06-10 ~ 06-12 · Matrix 平台 + 1.0 tag
- RSS 熔断死锁修复 + 自主恢复探测（`rss_recovery.go`）；6 个 feed 换官方 RSS 直连。
- 通知/监控重构：后端驱动日报（DailyReportService）+ O02/K08 退役。
- **Matrix 平台深度优化 T1–T9**：通知聚合去重 + 命令模型为唯一事实源 + 流水线异步化 + Agent MVP + todo 写工具 + 权限分级。
- 死代码清理 / RAGFlow 代码层退役 / golang-migrate 接入 / 31 Repository 全覆盖测试 / golangci-lint error 清零。
- **v1.0.0 tag**（`77a1e65`）。

### 2026-06-13 ~ 06-16 · 爬虫调度重设计 + n8n 治理
- 日报 O01/K08 自动触发 bug 根治（n8n 表达式须 `=` 前缀）+ 断档补发。
- **爬虫调度层重设计**：`DequeueFair` 公平轮转窗口 SQL + `next_allowed_at` 指数退避冷却 + 入队配额，删除内存 `DomainCoordinator`（ADR-0003）。
- n8n 工作流无损回流机制（spool 1.1 `export --to-source`）；K01–K04 webhook 修到 clean 路径。

### 2026-06-17 ~ 06-19 · PKB 知识骨架 + 双库
- PKB 原子知识网 Phase A–D：提示词 v2 + 一文多卡 + 语义去重 + 分层 MOC。
- **PKB 知识骨架 + 双库** Phase F–H：骨架为结构唯一真相源 + 缺口填充 + 资讯库 + LLM 晋升闸（ADR-0004 / ADR-0005）。
- PKB 自动闭环（server 内置多任务调度）+ 域 CRUD + 状态概览 + 前端骨架页。
- **知识库模块重做**：Matrix agent 通电（chat/direct 房间）+ Web 问答去拒答/真多轮 + 总览页 + 数据集前端退役（ADR-0006 前端边界）。

### 2026-07-02 · txhk Matrix homeserver 迁移
- Conduit 0.10.11 → **tuwunel**（弃旧库 fresh start、清白重来）；自建 **headscale** 控制面坐镇 txhk；新增 rdp-gateway 应急入口。全部 systemd 裸进程托管。

### 2026-07-03 ~ 07-06 · 1.0 重构冲刺（M1–M5）
- **M1 解耦地基**：`eventbus` 一级共享基础设施 + Event 契约（ULID+TraceID）+ 删僵尸 commands stream。
- **M2 LLM 独立化**：`llmgateway` 独立包 + Gateway 进程内直调（Chat/Rerank/ChatStream）+ 分层例外清零。
- **M3 KB 链路事件化**：crawl/extract/index 三 worker + 域名健康度（HealthScore/Pause）+ 提示词治理统一 + 问答 rerank + golden set 评估。
- **M4 日志补齐**：pattern 正则告警 + `trace_id` 全链路 + goroutine panic 护栏。
- **M5 前端收敛**：Matrix 7→2 页 + 爬取队列可视化 + 问答 SSE 流式。
- **1.0 GA 发布**：M1–M5 全绿，代码终审拟合度约 97%（P0 全修）。

### 2026-07-10 ~ 07-17 · 成本优化
- openclash 机场流量治理：定位真凶 = knowledge firecrawl playwright 整页渲染。
- firecrawl **fetch 引擎优先级翻转**（普通抓取先走纯 HTTP，流量降 95%+）+ 媒体资源拦截 + `recrawl-cooldown` 入队去重（`d467767`）。

### 2026-07-25 · 应用/数据分离
- keeper **应用层 ↔ silkdata 数据层拆分**：PostgreSQL/Meilisearch/Redis/NATS/CouchDB 迁至 silkdata(192.168.7.231)，keeper 只跑应用（bellkeeper/n8n/rsshub/memos）。
- app 靠 `extra_hosts` 别名连数据层，连接串零改动；PG/CouchDB 迁移无损（跳过纯日志表历史）；一次停机窗约 30min，全绿。

### 2026-07-26 · M6 可观测性 —— 1.0 里程碑收官
- 观测栈上线：**Prometheus + Loki + Grafana@silkdata** + 三机 cAdvisor/Promtail，`grafana.singll.net`。
- **1.0 里程碑（M1–M6）全部完成，系统进入稳定运行状态。**

---

## 历史文档去向

- **一次性计划/审查**（1.0-REVAMP、MILESTONES-1.0、CODE-REVIEW-1.0-\*、KEEPER-DATA-APP-SPLIT、各 PKB/Matrix/爬取 \*-PLAN 等）：成果已并入本时间线里程碑 + [STATUS.md](STATUS.md) + [ARCHITECTURE.md](ARCHITECTURE.md) + [ADR](../docs/adr/)，**原文已删除**（可查 git 历史）。
- **仍生效的运维 SOP** → [ops/](ops/)：istoreos 子网路由基线、离线托底、TrueNAS NFS 挂载。
- **知识库运营资产** → [knowledge-base/](knowledge-base/)：Obsidian 单 Vault 设计、frontmatter/命名/同步规范、Templater 模板、每周整理 SOP。
- **架构决策记录**（现行有效）→ [../docs/adr/](../docs/adr/)。
