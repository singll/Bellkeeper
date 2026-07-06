# Bellkeeper 1.0 重构与架构演进规划

> **本文档是 Bellkeeper 1.0 的终极统一收口（Single Source of Truth）。**
> 更新日期：2026-07-03
> 适用范围：`internal/` 全部子包 + `web/` 前端 + n8n 工作流 + 基础设施部署。
> 冲突规则：本文件与 `doc/` 下任何旧文档冲突时，**以本文为准**；旧文档按 §5 规整处置。
> 事实基线：所有现状陈述均经本地代码基底核验，引用具体文件路径。

---

## 0. 现状摘要（基于代码事实核验）

| 维度 | 现状 | 关键证据 |
|------|------|----------|
| 分层 | ✅ 严格 `router→handler→service→repository→model`，手动 DI | `internal/app/app.go:119-462`、`internal/service/service.go:43-170` |
| NATS | ⚠️ **仅 Matrix 通知单链路在用**；`commands` stream 建而不用（僵尸配置） | `matrix/infra/nats.go:64-99`（仅建 `notifications`+`commands`）；全仓 `.Publish`/`.Subscribe` 仅 3 处：`service/notification.go:225,350`、`matrix/worker/notification_worker.go:67-126` |
| LLM 代理池 | ✅ 功能完整（2447 行）；⚠️ **被 7 处调用方全经 HTTP 自回环**，无进程内直调接口 | `service/llm_proxy.go`；调用方：`matrix/agent/agent.go:161,250`、`service/knowledge_ask.go:192`、`service/daily_report.go:450`、`service/classify.go:166`、`service/rule_optimizer.go:46`、`service/llm_job_queue.go:185`、`pkb/client.go:255-276` |
| 知识库 PKB | ✅ 原子网+骨架+闭环已落地，提示词已外置 `config/pkb/prompts/`（15 文件+registry.yaml）；server 内置多任务 Scheduler 已驱动全闭环 | `internal/pkb/`（23 文件）、`app.go:407-422` |
| Matrix Bot | ✅ Gateway+Agent+命令+通知齐全；**消息处理已异步化**（有界 worker 池） | `matrix/gateway/sync.go:259-279`（`dispatchCommand` 用 `dispatchSem`+goroutine）；`matrix/` 含 9 个子包 |
| 日志中心 | ⚠️ **已是轻量**：仅 DB entry CRUD + threshold 告警 + dashboard 聚合；**全文检索/SSE/归档调度代码根本不存在**（系 ROADMAP 待办，非未完成设计） | `service/log_center.go`（335 行）、`handler/log_center.go`（331 行） |
| trace_id | ⚠️ 字段+索引+查询已有，**无自动生成与全链路传播** | `model/log_entry.go:21`、`repository/log_entry.go:60-61`；全仓无 HTTP 中间件生成、无 LLM 透传 |
| 文档 | ⚠️ `doc/` 下 26 个 .md，标准不一，含已废弃方案 | 见 §5 清单 |

**核心判断**：Bellkeeper 架构骨架正确，痛点经核验集中在三处真实问题——
1. **LLM 代理池无进程内接口**：7 个内部调用方全部经 `localhost HTTP` 自回环，绕完整 HTTP+鉴权栈，每请求多余一次序列化+Token 校验；且管理 handler 持有 repo（已登记 2 条分层例外）。
2. **NATS 利用不足且有僵尸配置**：`commands` stream 建而不用；KB 爬取完成、LLM 任务完成等关键事件全同步函数调用，无队列缓冲。
3. **提示词治理割裂**：PKB 已完整外置+registry，但 `classify`/`knowledge_ask`/`rule_optimizer` 仍硬编码在 Go 代码且无 system 角色分离。

日志中心**不需要瘦身**（已是轻量），需做的是**补齐 trace_id 传播**与**外挂 Loki**做全文检索。

---

## 1. 1.0 架构蓝图与边界设计

### 1.1 目标拓扑

```
                        ┌─────────────────────────────┐
                        │   NATS JetStream（事件总线）  │
                        │  knowledge.> / llm.> /       │
                        │  matrix.> / system.> / logs.>│
                        └──────────────┬──────────────┘
                                       │ Pub/Sub
        ┌──────────────┬───────────────┼───────────────┬──────────────┐
        ▼              ▼               ▼               ▼              ▼
  ┌───────────┐ ┌─────────────┐ ┌─────────────┐ ┌───────────┐ ┌───────────┐
  │ 知识库(KB) │ │ LLM 代理池   │ │ Matrix Bot  │ │ 日志(轻量) │ │ 治理中台   │
  │ pkb/      │ │ llmgateway/  │ │ matrix/     │ │ log_center│ │ service/  │
  │ 爬取+提取  │ │ 进程内直调   │ │ ChatOps     │ │ +Loki外挂 │ │ 编排+配置  │
  │ +检索+问答 │ │ +HTTP 兼容   │ │ 事件驱动     │ │           │ │           │
  └───────────┘ └─────────────┘ └─────────────┘ └───────────┘ └───────────┘
        │              │               │                            │
        └──────────────┴───────────────┴────────────────────────────┘
                              共享：DB / Redis / Meilisearch
```

### 1.2 NATS JetStream 事件总线设计

**现状问题（核验）**：`matrix/infra/nats.go:64-99` 把 NATS 客户端锁死在 matrix 子包内，`ensureStreams` 只建 `notifications`+`commands` 两条 stream，其中 `commands` stream **全仓无任何 Publish/Subscribe**（僵尸配置）。KB 爬取完成、LLM 任务完成等关键事件是同步函数调用，无队列缓冲。

**1.0 目标**：把 NATS 客户端提升为**一级共享基础设施**（`internal/eventbus/`），清理僵尸 stream，按需新增 stream。

#### 1.2.1 Stream 划分（核验后修正）

| Stream | Subjects | Retention | 用途 | 消费者 | 现状 |
|--------|----------|-----------|------|--------|------|
| `notifications` | `notifications.>` | WorkQueue | Matrix 通知投递 | `bellkeeper-notify-worker` | ✅ 已用（保留） |
| ~~`commands`~~ | ~~`commands.>`~~ | — | — | — | ❌ **僵尸 stream，删除**（全仓无 Publish/Subscribe） |
| `knowledge` | `knowledge.crawl.done`<br>`knowledge.extract.done`<br>`knowledge.crawl.failed` | WorkQueue | 爬取完成→提取、提取完成→索引、失败→健康评估 | `bellkeeper-extract-worker`<br>`bellkeeper-index-worker`<br>`bellkeeper-domain-health-worker` | 新增 |
| `llm` | `llm.job.submit`<br>`llm.job.done` | WorkQueue | LLM 异步任务（替代 `llm_jobs` DB 轮询，DB 保留状态机） | `bellkeeper-llm-worker` | 新增（`llm_jobs` 表保留为状态真相源） |
| `matrix` | `matrix.agent.reply` | WorkQueue | Agent 长任务结果回投（命令已异步，见下） | `bellkeeper-reply-worker` | 新增 |
| `system` | `system.daily.tick`<br>`system.health.alert` | Interest | 定时触发、健康告警广播 | 多消费者（日报/告警/Matrix） | 新增 |
| `logs` | `logs.entry.created` | Limits (MaxAge 7d) | 日志事件流（供 Loki 外挂消费） | `bellkeeper-log-shipper` | 新增 |

**设计约束**：
- 每条 stream 用 `nats.FileStorage` + `AckExplicit`，至少一次投递。
- 消费者用**持久 Pull 模式**（`PullSubscribe` + `Fetch`），复用现有 `notification_worker.go:108` 的 `Fetch(10, MaxWait=5s)` 模式。
- 失败重试走 `NakWithDelay` 退避。

**注意**：Matrix 命令处理**已异步化**（`sync.go:259-279` 有界 worker 池），无需再事件化；仅 Agent 长任务结果回投走 `matrix.agent.reply` 事件，避免阻塞 sync loop。

#### 1.2.2 事件契约规范

所有事件用 JSON，统一 envelope，放 `internal/eventbus/event.go`：

```go
type Event struct {
    EventID    string          `json:"event_id"`     // ULID
    Type       string          `json:"type"`         // 如 "knowledge.crawl.done"
    Source     string          `json:"source"`       // kb/llm/matrix/system
    OccurredAt time.Time       `json:"occurred_at"`
    Subject    string          `json:"subject"`      // 业务主键
    Payload    json.RawMessage `json:"payload"`
    TraceID    string          `json:"trace_id"`     // 贯穿日志（§4.4）
}
```

#### 1.2.3 从 matrix/infra 提升到 eventbus/

**迁移步骤**：
1. 新建 `internal/eventbus/` 包，把 `matrix/infra/nats.go` 的 `NATSClient` + `ensureStreams` 迁入，扩展多 stream 管理；**删除僵尸 `commands` stream**。
2. `app.go:215` 创建单一 `eventbus.Client`，注入各模块。
3. `matrix/infra/nats.go` 改为薄封装委托给 `eventbus.Client`（或直接删除，matrix 持有 `eventbus.Client`）。
4. `service/notification.go:225,350` 的 `s.nats.Publish` 改为 `s.bus.Publish("notifications.<channel>", ...)`，行为不变。
5. `matrix/worker/notification_worker.go:67` 的 `Subscribe` 改走 `eventbus.Client`，验证通知链路无回归。
6. 新增 KB / LLM / system 事件发布点（见 §2）。

**禁止**：任何模块绕过 `eventbus` 直接 `nats.Connect`。

### 1.3 模块边界与职责定义

| 模块 | 边界（做什么） | 禁止（不做什么） | 包路径 |
|------|----------------|------------------|--------|
| **知识库 KB** | 爬取调度、内容提取、三层存储、Meili 索引、问答检索、PKB 漏斗 | 直接调用 LLM provider；直接操作 Matrix；管 LLM 凭证 | `internal/pkb/` + `service/knowledge_*.go` + `service/crawl_*.go` |
| **LLM 代理池** | 多 provider 路由、熔断、粘性、限流学习、计费、余额、Token 鉴权、Gemini/Anthropic 转换；对外 OpenAI 兼容 API；**提供进程内直调接口** | 关心业务语义；操作 DB 业务表；发 Matrix 通知 | `internal/llmgateway/`（从 `service/llm_proxy.go` + `llm/` 抽出） + `internal/llmclient/`（保留供外部/n8n） |
| **Matrix Bot** | 命令路由、Agent 对话、通知投递、ChatOps 工具、权限 | 承载管理 UI（弱化）；直接爬取/提取；直接管 LLM 凭证 | `internal/matrix/`（9 子包） |
| **日志（轻量）** | zap 结构化日志、activity_log 业务审计、log_center 告警规则（threshold） | 全文检索（交 Loki）；SSE 实时流（交 Loki tail）；日志归档长存（交 Loki） | `middleware/logger.go` + `service/log_center.go`（已是轻量，保留） |
| **治理中台** | 配置管理、定时编排、n8n 工作流纳管、健康聚合、Dashboard | 实现具体业务逻辑 | `service/*.go`（剩余） |

---

## 2. 模块专项优化与重构方案

### 2.1 知识库：爬取稳定性 + 队列调优 + 提示词治理统一

#### 2.1.1 爬取稳定性

**现状（核验）**：`service/crawl_queue.go` 的 `DequeueFair` + 域名冷却 + 配额拒绝已实现；`crawl_domain_profile` 表已有 `ConsecutiveFailures`/`HealthScore`；`crawl_failure.go` 已有失败档案实体；`rule_optimizer.go` 已有 LLM 规则优化闭环。

**改进**：

| 项 | 现状 | 1.0 目标 | 落地点 |
|----|------|----------|--------|
| 抓取失败事件化 | `crawl_failure.go` 同步记录 | 发布 `knowledge.crawl.failed` 事件，独立 worker 做域名健康度评估 + 自动暂停 | `service/crawl_failure.go` + 新增 eventbus 消费者 |
| 域名健康度阈值配置化 | 硬编码 `ConsecutiveFailures>=5` 暂停、`HealthScore>=30` 恢复 | 阈值进配置 + 每日健康报告发 `system.health.alert` | `config/crawl_queue` + `service/crawl_failure.go` |
| 内容质量打分前置 | digest 阈值 7.0 已配置化 | 爬取阶段即打分，低分直接拒绝入库（不进 raw） | `service/crawl_queue.go` 入队前 gate |

#### 2.1.2 队列调优

**现状**：`crawl_queue.go` 是 DB 轮询 + 乐观锁出队（`DequeueFair`）；`llm_job_queue.go:142-200` 是 DB 轮询 worker。消费速度受 DB 轮询间隔限制。

**1.0 目标**：把 KB 内部队列从 DB 轮询改为 NATS 消费者，DB 只存状态机。

- `knowledge.crawl.done` 事件 → `extract-worker` 消费 → 调提取器 → 写 archive + 发 `knowledge.extract.done`。
- `knowledge.extract.done` → `index-worker` 消费 → 更新 Meili。
- Pull 模式 `Fetch(batch=10, MaxWait=5s)`（复用 `notification_worker.go:108` 模式）。
- DB 表 `crawl_jobs.status` 保留为状态真相源，NATS 消息携带 `job_id`，消费者更新状态。
- `llm_jobs` 同理：DB 保留状态机，NATS `llm.job.submit` 驱动 worker。

**收益**：消费速度从"轮询间隔"限制变为"worker 并发数"限制；失败消息自动 NATS 重试。

#### 2.1.3 提示词治理统一（核验后修正重点）

**现状割裂（核验）**：
- PKB 提示词**已完整外置** `config/pkb/prompts/`（15 个 .md + `registry.yaml`），成熟度高。
- `classify.go:60-85` 的 `defaultClassifyPrompt` **硬编码在 Go 代码**，虽有 `s.cfg.Prompt` 可覆盖但无 registry 外置；且 `callLLM` 全走 `user` 角色（`classify.go:147,170`）。
- `knowledge_ask.go:161-164` 的 systemPrompt 硬编码，已做 system/user 分离但未外置。
- `rule_optimizer.go` 提示词硬编码。
- `llmclient.ChatRequest`（`client.go:28-34`）**无 `ResponseFormat`/`MaxTokens` 字段**，proxy 仅透传部分参数。

**1.0 改进清单**：

1. **提示词统一外置 + registry**：`knowledge_ask`/`rule_optimizer`/`classify` 提示词迁到 `config/prompts/`（新建目录，与 `config/pkb/prompts/` 并列，因这三者非 PKB 专属）+ 复用 PKB 的 `registry.yaml` 模式。落地点：`service/knowledge_ask.go:161`、`service/rule_optimizer.go`、`service/classify.go:60`。
2. **`llmclient.ChatRequest` 增加 `ResponseFormat`/`MaxTokens` 字段**，proxy 透传给底层 provider；PKB score/reconstruct/verify + classify 改用 `json_object` + schema 校验，失败带错误回喂一次（自修复重试）。
3. **system/user 角色分离**：`classify`/`rule_optimizer` 把规则段移到 `system` 角色（`knowledge_ask` 已做），利用 provider prompt cache 降低成本。
4. **上下文压缩**：`knowledge_ask.buildContext`（当前 `knowledge_ask.go:136-158` 直接拼接+截断）改为召回 top-20 → 每片段独立摘要 → 拼接 top-5；接 LLM 代理池 rerank 路由（proxy 已有）。
5. **golden set 评估**：`config/pkb/eval/*.json` 放 10-20 篇带预期样本，`pkb-curate eval` 子命令跑评分回归。

### 2.2 LLM 代理池：剥离为独立内部基础设施 + 进程内直调

#### 2.2.1 现状诊断（核验）

- 功能完整：`service/llm_proxy.go`（2447 行）+ `service/llm_*.go`（11 文件）+ `llm/{balance,converter,errors}`。
- **问题 1：7 处调用方全经 HTTP 自回环**。所有内部调用方（`matrix/agent/agent.go:161`、`service/knowledge_ask.go:192`、`service/daily_report.go:450`、`service/classify.go:166`、`service/rule_optimizer.go:46`、`service/llm_job_queue.go:185`、`pkb/client.go:255`）都构造 `llmclient.Client` 调 `http://localhost:{port}/api/llm/v1`，经 Gin 路由 → `LLMTokenAuth` 中间件 → `LLMProxyHandler.Proxy` → `LLMProxyService`。进程内调用绕完整 HTTP+鉴权栈，每请求多余一次 JSON 序列化+网络栈+Token 校验。
- **问题 2：管理 handler 持有 repo（2 条已登记分层例外）**。`ARCHITECTURE-EXCEPTIONS.md` 登记：①`router.go` 传 `LLMTokenRepository` 给 `auth.LLMTokenAuth`；②`LLMProxyHandler` 构造时持有 token/usage/pricing repo。均有退出计划但未执行。
- **问题 3：`llmclient` 是进程外客户端**，外部/n8n 调用需要，但 Go 进程内不应绕 HTTP。

#### 2.2.2 1.0 目标：独立包 + 进程内直调 + 消化分层例外

**包重组**：

```
internal/llmgateway/
├── gateway.go          // Gateway 核心，持有 channel/router/breaker/limiter（从 service/llm_proxy.go 迁入）
├── router.go           // 任务感知路由（从 service/llm_task_router.go 迁入）
├── breaker.go          // 熔断 + Kimi probe 自恢复
├── limiter.go          // 自适应限流学习器（从 service/llm_rate_limit_learner.go 迁入）
├── balance/            // 余额查询（从 internal/llm/balance/ 迁入）
├── converter/          // 协议转换（从 internal/llm/converter/ 迁入）
├── errors/             // 错误分类（从 internal/llm/errors/ 迁入）
├── repository.go       // 凭证/渠道/计费 DB 访问收拢
├── admin.go            // Token/Pricing 管理（消化分层例外②，从 handler 迁入 service）
└── handler.go          // HTTP handler：/api/llm/v1/*（从 handler/llm_proxy.go 迁入）
```

**对外契约（不变）**：

| 端点 | 用途 | 鉴权 |
|------|------|------|
| `POST /api/llm/v1/*path`（`proxy.Any`） | OpenAI 兼容（含流式） | `sk-bk-*` Bearer（`auth/llm_token.go`） |
| `GET /api/llm/channels/status` 等 | 管理端点 | Authelia + API Key |

**进程内直调接口（核心新增）**：

```go
// internal/llmgateway/gateway.go
type Gateway interface {
    Chat(ctx context.Context, req ChatRequest, opts ChatOptions) (*ChatResponse, error)
    ChatStream(ctx context.Context, req ChatRequest, opts ChatOptions) (<-chan StreamChunk, error)
}
```

`app.go` 注入 `Gateway` 给 KB/Matrix Agent/日报/分类/RSS规则优化，替代 7 处 `llmclient.New(...)` 构造。`llmclient` **保留**供外部脚本/n8n（进程外调用）。

**消化分层例外**：
- 例外①：`auth.LLMTokenAuth` 改为依赖 `llmgateway.TokenScopeService` 接口（service 层），不直接持有 repo。
- 例外②：Token/Pricing 管理 CRUD 迁入 `llmgateway/admin.go`，`LLMProxyHandler` 只依赖 service。

**收益**：省去 7 处进程内 HTTP 自回环；分层例外清零；LLM 代理池可独立演进。

### 2.3 Matrix Bot：弱化管理 UI，补齐事件回投

#### 2.3.1 去留分析

**结论：保留 Matrix Bot，重新定义重心。**

- Matrix Bot 是**控制平面 + 交互入口**，价值在 ChatOps（QA/Todo/Daily/Alerts/Agent 对话），不在 Web 管理 UI。
- `matrix/` 9 个子包（agent/command/gateway/infra/notify/policy/queue/registry/worker）架构健康。
- **命令处理已异步化**（`sync.go:259-279` 有界 worker 池），无需再改。
- 问题在前端：7 个管理页死板，重复后端 API。

#### 2.3.2 1.0 边界

| 能力 | 归属 | 说明 |
|------|------|------|
| QA 问答 | Matrix Agent（对话房间）+ `!问`/`!kb`（强制 RAG）+ Web 问答页 | 双入口，Agent 优先 |
| Todo | Matrix `!待办`/`!todo` + Memos 同步 | Web 不做 |
| Daily Report | Matrix 推送 + Web 只读 | 触发走 Matrix/定时 |
| Alerts | Matrix 通知房间 | Web 不做告警中心 |
| 命令/房间管理 | Matrix `!commands`/`!rooms` + Admin API | Web 仅只读列表 |
| 配置管理 | Web Settings | 唯一保留的 Web 管理页 |
| Dashboard | Web 只读 | 保留 |

#### 2.3.3 前端 7→3 页重构

按 `matrix-platform-overhaul-plan.md` T9 执行：
- **Dashboard**：合并原 Overview/Health/Alerts，只读观测。
- **Rooms**：合并原 Rooms/Commands/Notifications，只读 + 极少写（仅改 room_type）。
- **Settings**：保留配置管理。

#### 2.3.4 事件回投补齐（非命令异步化）

**现状（核验）**：`command.go:130-189` 的 `ExecuteMessage` 在 worker 池内同步执行 Agent（`agent.HandleMessage`），Agent 多轮工具循环可能阻塞 worker。`matrix/gateway/sync.go:259-279` 的 `dispatchCommand` 已异步派发到 worker 池，但 Agent 长任务仍占用 worker。

**1.0 改进**：
- 简单命令（ping/status/help）保持 worker 池内同步。
- Agent 长任务（多轮工具调用）改为：worker 内发起 → 结果通过 `matrix.agent.reply` 事件回投 → 独立 `reply-worker` 消费后投递消息，释放 dispatch worker。
- 通知聚合去重（`notification.go:155-185` 已有）保留，消费链路从 `notification_worker` 直连 `eventbus`。

### 2.4 日志架构：保持轻量 + 外挂 Loki + trace_id 全链路

#### 2.4.1 现状（核验后修正）

**草案原文称"log_center 是过重未完成设计，含 Meili 全文检索双写/SSE/归档调度"——经核验失实**。`service/log_center.go`（335 行）实际只有：
- DB entry CRUD（`LogActivity`/`ListEntries`/`GetEntry`）
- threshold 告警规则（`AlertCondition` 仅 `Module`/`Level`/`Threshold`/`WindowMinutes` 四字段，`log_center.go:322-327`）
- dashboard 聚合（`GetDashboard`）
- `CleanOldEntries`（有方法但无调度调用）

**全文检索/SSE/归档调度代码根本不存在**——那是 ROADMAP §7 的待办项，不是"未完成的过重设计"。因此**无需瘦身**，需做的是补齐 trace_id 与外挂 Loki。

#### 2.4.2 1.0 方案：内外分层

| 层 | 归属 | 工具 | 保留范围 |
|----|------|------|----------|
| **应用日志** | Bellkeeper 内部 | zap（`middleware/logger.go`，production config） | 结构化 JSON 输出 stdout |
| **业务审计日志** | Bellkeeper 内部 | `activity_log` 表 + `log_center.LogActivity` | 仅业务关键动作（入库/审批/配置变更/命令执行） |
| **告警规则** | Bellkeeper 内部（轻量） | `log_center` threshold 告警 | 触发后发 `system.health.alert` 事件 → Matrix 通知。**新增 pattern（正则）类型** |
| **系统日志聚合** | 外挂 | **Loki + Promtail** | Promtail 采集容器 stdout → Loki → Grafana 查询 |
| **全文检索** | 外挂 | Loki LogQL | 不在 Bellkeeper 内建 Meili logs 索引 |
| **实时日志流** | 外挂 | Grafana/Loki tail | 不在 Bellkeeper 内建 SSE |

#### 2.4.3 落地动作

1. **`log_center` 保持现状**（已是轻量）；新增 `pattern` 告警类型（正则匹配 Summary），补齐 ROADMAP §7 的 pattern 需求。
2. **`CleanOldEntries` 接入调度**：当前有方法无调用（`log_center.go:277`），在 `app.go` `startBackgroundTasks` 加每日清理 goroutine。
3. **部署 Loki + Promtail**：通过 `spool bundle` 在 keeper 主机部署，Promtail 采集 bellkeeper + n8n + NATS 容器日志。
4. **trace_id 全链路（核心）**：
   - HTTP 中间件生成 `trace_id`（`middleware/logger.go` 新增），注入 context + zap logger。
   - `llmgateway.Gateway.Chat` 透传 `TraceID` 到 `X-Trace-Id` header（proxy 已有 header 透传能力）。
   - `eventbus.Event.TraceID` 贯穿事件链。
   - `log_center.LogActivity` 从 context 取 trace_id（当前 `log_center.go:44` 字段已有但无来源）。
5. **`/api/logs/*` 路由保持**：告警规则 CRUD + dashboard + entries 列表（供审计查询）；全文检索引导用户用 Grafana/Loki。

**收益**：DB 不膨胀；日志检索能力由 Loki LogQL 提供；Bellkeeper 代码不增反减（trace_id 复用 context）。

---

## 3. 历史文档规整与收口规范

### 3.1 现状清单与处置

`doc/` 下文档分三类处置：

#### A. 保留为活跃事实源（随代码同步更新）

| 文档 | 处置 |
|------|------|
| `doc/README.md` | 保留，更新导航指向本文件 + 下列活跃文档 |
| `doc/STATUS.md` | 保留，作为**实施状态**回写处 |
| `doc/ROADMAP.md` | 保留，但**待办项以本文件 §4 为准**；ROADMAP 仅保留已完成里程碑表 + 本文件 §4 镜像 |
| `doc/ARCHITECTURE.md` | 保留，按 §1 蓝图更新；标"1.0 终态" |
| `doc/ARCHITECTURE-EXCEPTIONS.md` | 保留，分层例外登记（llmgateway 重组后两条 LLM 例外应清零） |
| `doc/API.md` | 保留，按 §2.2 LLM Gateway + 现有路由更新 |
| `doc/DEVELOPMENT-GUIDE.md` | 保留，编码规范权威 |
| `doc/ASSISTANT-GUIDELINES.md` | 保留 |
| `docs/adr/0001-0006` | 保留，ADR 永不删 |

#### B. 归档（移入 `doc/archive/`，只读）

| 文档 | 归档原因 |
|------|----------|
| `doc/PKB-IMPLEMENTATION.md` | MVP 已落地，残留项转 §4 |
| `doc/PKB-ATOMIC-KNOWLEDGE-PLAN.md` | Phase A-D 已完成，Phase E 遗留转 §4 |
| `doc/PKB-KNOWLEDGE-SKELETON-PLAN.md` | Phase F-H 已完成 |
| `doc/KNOWLEDGE-MODULE-REVAMP-PLAN.md` | 阶段 1-2 已完成 |
| `doc/LLM-PROMPT-AGENT-REVIEW.md` | P0 已修，P1 转 §4 |
| `doc/LLM_PROXY_GUIDE.md` | 被 `doc/LLM-GATEWAY-API.md`（§2.2 新建）取代 |
| `doc/matrix-platform-overhaul-plan.md` | T1-T8 已完成，T9 转 §4 |
| `doc/notification-monitoring-overhaul-plan.md` | 已落地 |
| `doc/reliability-audit-plan.md` | Tier 2-3 已完成 |
| `doc/daily-report-fix-plan.md` | 已落地 |
| `doc/matrix/` 全部 | 合并进 `doc/ARCHITECTURE.md` Matrix 章节 + `doc/API.md`，原文件归档 |
| `doc/modules/` 全部 | 模块总览，合并进 `doc/ARCHITECTURE.md`，原文件归档 |
| `doc/rss/` + `doc/documents/` | 合并进 `doc/ARCHITECTURE.md` 知识库章节，原文件归档 |
| `doc/architecture/` | 基础设施类，合并进 `doc/ARCHITECTURE.md`，原文件归档 |

#### C. 删除或冻结

| 文档 | 处置 |
|------|------|
| `1.0-PROGRESS.md`（根目录，.gitignore） | 冻结，里程碑已达成 |
| `CONTEXT.md`（根目录） | 内容合并进 `doc/ARCHITECTURE.md` 后删除 |
| `doc/rss-sources.json` | 移到 `config/rss-sources.json`（数据资产归配置） |
| `doc/TODO.md` | 删除，待办以本文件 §4 + `doc/ROADMAP.md` 为准 |

### 3.2 1.0 文档体系最终结构

```
Bellkeeper/
├── CLAUDE.md                    # 协作约定（红线 + 运维指针，瘦）
├── doc/
│   ├── README.md                # 导航
│   ├── ARCHITECTURE.md          # 架构事实（1.0 终态，含原 modules/matrix/rss/documents/architecture 合并内容）
│   ├── API.md                   # REST + LLM Gateway API 参考
│   ├── LLM-GATEWAY-API.md       # LLM 代理池对外 API 契约（新建）
│   ├── DEVELOPMENT-GUIDE.md     # 编码规范
│   ├── ASSISTANT-GUIDELINES.md  # AI 助手守则
│   ├── ARCHITECTURE-EXCEPTIONS.md
│   ├── STATUS.md                # 实施状态回写
│   ├── ROADMAP.md               # 仅已完成里程碑 + 本文件 §4 镜像
│   └── archive/                 # 所有归档计划（只读）
├── docs/
│   └── adr/                     # ADR 0001-0006（+ 1.0 新增）
└── config/                      # 配置（含 rss-sources.json、pkb/prompts、prompts/）
```

### 3.3 维护规则（收口后强制）

1. **新任务一律加到本文件 §4**，不另开新 PLAN 文档；大型计划单独立文档后完成后立即归档，残留转 §4。
2. **完成一项**：§4 打勾 → STATUS.md 回写 → 大架构变化同步 ARCHITECTURE.md。
3. **禁止在归档文档上追加内容**。
4. **配置即数据**：`rss-sources.json`/`domains.yaml`/`prompts/` 归 `config/`，不进 `doc/`。
5. **唯一事实源**：架构看 `ARCHITECTURE.md`，API 看 `API.md`+`LLM-GATEWAY-API.md`，待办看本文件 §4，状态看 `STATUS.md`。重复描述不跨文件维护，用链接引用。

---

## 4. 1.0 演进 To-Do List

> 优先级：P0 = 阻塞稳定运行；P1 = 1.0 核心交付；P2 = 增强体验；P3 = 远期。
> 每项标注落地点（文件/包），便于直接动手。

### P0 — 解耦基础设施（必须先做，是其他项的前提）

- [ ] **[eventbus] 新建 `internal/eventbus/` 包**，从 `matrix/infra/nats.go` 提升 `NATSClient` + `ensureStreams`；**删除僵尸 `commands` stream**（全仓无 Publish/Subscribe）；扩展 `knowledge`/`llm`/`system`/`logs` 多 stream。落地点：新建包 + `app.go:215` 改注入。
- [ ] **[eventbus] 定义 `Event` envelope 契约**（§1.2.2），放 `internal/eventbus/event.go`。
- [ ] **[eventbus] `service/notification.go:225,350` + `matrix/worker/notification_worker.go:67` 迁移到 eventbus.Client**，行为不变，验证通知链路无回归。
- [ ] **[dead-code] 清理 `agent.go:56-64` 死代码**：`maxIter` 定义后 `_ = maxIter` 丢弃又在 132-135 重新定义，删除 56-64 整段。
- [ ] **[config] NATS stream 配置化**：`config.NATSStreamsConfig` 增加 `knowledge`/`llm`/`system`/`logs` 字段（`config.go:535-536` 现仅 notifications/commands），`config/bellkeeper.yaml` 补默认值。

### P1 — 模块专项交付

#### LLM 代理池（优先，因影响 7 处调用方）

- [ ] **[llmgw] 包重组**：从 `service/llm_proxy.go` + `service/llm_*.go` + `internal/llm/` 抽出 `internal/llmgateway/`，保持对外路由 `/api/llm/v1/*` + `/api/llm/*` 不变。
- [ ] **[llmgw] 进程内直调接口**：`Gateway.Chat(ctx, req, opts)`，`app.go` 注入给 7 处调用方（`matrix/agent/agent.go:66`、`service/knowledge_ask.go:55`、`service/daily_report.go:51`、`service/classify.go:27`、`service/rule_optimizer.go:46`、`service/llm_job_queue.go:49`、`pkb/client.go:53`），替代 `llmclient.New` HTTP 构造。
- [ ] **[llmgw] 消化分层例外**：`auth.LLMTokenAuth` 依赖 `TokenScopeService` 接口；Token/Pricing 管理迁入 `llmgateway/admin.go`，`LLMProxyHandler` 只依赖 service。完成后更新 `ARCHITECTURE-EXCEPTIONS.md` 清零两条 LLM 例外。
- [ ] **[llmgw] 文档**：新建 `doc/LLM-GATEWAY-API.md`，旧 `LLM_PROXY_GUIDE.md` 归档。
- [ ] **[llmgw] cache 命中率监控** + 调用方 base_url 迁移 + new-api 停服决策（ROADMAP §3 遗留）。

#### 知识库

- [ ] **[kb-queue] 爬取完成事件化**：`service/crawl_queue.go` 出队完成后发 `knowledge.crawl.done`，新增 `extract-worker` 消费者替代 DB 轮询。
- [ ] **[kb-queue] 提取完成→索引重建事件链**：`knowledge.extract.done` → `index-worker` → 更新 Meili。
- [ ] **[kb-queue] `llm_jobs` 事件化**：`llm.job.submit` → worker 消费，DB 保留状态机（`llm_job_queue.go:142-200` 现 DB 轮询）。
- [ ] **[kb-stability] 抓取失败事件 + 域名健康度 worker**：`knowledge.crawl.failed` → 评估 + 自动暂停/恢复（复用 `crawl_domain_profile`）。
- [ ] **[kb-prompt] `knowledge_ask`/`rule_optimizer`/`classify` 提示词外置**到 `config/prompts/`（新建）+ `registry.yaml`。
- [ ] **[kb-prompt] `llmclient.ChatRequest` 增加 `ResponseFormat`/`MaxTokens` 字段**（`client.go:28-34`），proxy 透传，PKB + classify 结构化输出改 `json_object` + schema 校验 + 自修复重试。
- [ ] **[kb-prompt] system/user 角色分离**（classify/rule_optimizer 规则段入 system，`knowledge_ask` 已做）。
- [ ] **[kb-ask] 问答接 rerank**：召回 top-20 → rerank → top-5；上下文按片段摘要压缩。落地点：`service/knowledge_ask.go:136-158`。
- [ ] **[kb-eval] golden set 评估**：`config/pkb/eval/` + `pkb-curate eval` 子命令。
- [ ] **[kb-run] 存量 ~308 篇 raw 分批跑完 + 线上验收**（PKB Scheduler 已驱动闭环，`app.go:407-422`；遗留是数据运营非代码）。

#### Matrix Bot

- [ ] **[matrix-reply] Agent 长任务结果事件回投**：`matrix.agent.reply` 事件 → `reply-worker` 消费 → 投递消息，释放 dispatch worker（命令已异步，`sync.go:259-279`，无需再改）。
- [ ] **[matrix-ui] 前端 7→3 页重构**：Dashboard/Rooms/Settings（overhaul T9）。
- [ ] **[matrix-ui] 删除死板管理操作，保留只读 + 极少写**（room_type）。

#### 日志（保持轻量 + 外挂）

- [ ] **[log-pattern] `log_center` 新增 pattern（正则）告警类型**（当前仅 threshold，`log_center.go:322-327`）。
- [ ] **[log-schedule] `CleanOldEntries` 接入每日调度**（`log_center.go:277` 有方法无调用，加到 `app.go` `startBackgroundTasks`）。
- [ ] **[log-loki] 部署 Loki + Promtail**（`spool bundle` 在 keeper 主机），采集 bellkeeper/n8n/NATS 容器日志。
- [ ] **[log-trace] trace_id 全链路**：HTTP 中间件生成（`middleware/logger.go`）+ `llmgateway` 透传 + `eventbus.Event.TraceID` + zap 注入 + `log_center.LogActivity` 从 context 取。
- [ ] **[log-guard] goroutine panic 护栏**：`log_center.go:48`、`activity_log.go:56`、`knowledge_index.go:51,88` 等所有 `go func(){}` 加 recover + zap 记录。

#### 前端

- [ ] **[fe] 爬取队列可视化页**（后端 API 完整 `router.go:394-406`，前端缺失）。
- [ ] **[fe] Vault 预览增强**：Markdown 渲染 + frontmatter 折叠 + `[[wikilink]]` 跳转。
- [ ] **[fe] 知识问答 SSE 流式**（后端需配合改造为 SSE）。

### P2 — 增强与可观测性

- [ ] **[obs] Grafana 看板**（Prometheus 端点已有 `internal/metrics/metrics.go`，缺 Grafana 容器 + dashboard JSON）。
- [ ] **[obs] cAdvisor 容器资源压力检测**。
- [ ] **[eng] API 契约测试或 OpenAPI/类型生成**。
- [ ] **[eng] 配置热重载推广**到 PKB/RSS（LLM Proxy + 通知已实现）。
- [ ] **[kb-ask] 历史会话持久化**：`qa_sessions/qa_messages` + Matrix thread 上下文。
- [ ] **[kb-ask] 引用结构化**：`line_range/score` + 前端跳转。
- [ ] **[fe] Dashboard 时间序列图表**。

### P3 — 远期

- [ ] K07 Obsidian 回流端到端验证。
- [ ] 文件级权限标签 + 检索过滤。
- [ ] 存量知识批量导入。
- [ ] Vault 在线编辑。
- [ ] 备份恢复验证 + n8n 工作流 SLA 指标。

---

## 5. 里程碑

| 里程碑 | 内容 | 依赖 |
|--------|------|------|
| **M1 解耦地基** | P0 全部完成：eventbus 包 + Event 契约 + 删僵尸 commands stream + 通知迁移验证 + agent 死代码清理 | 无 |
| **M2 LLM 独立化** | llmgateway 包重组 + 进程内直调（7 处调用方迁移）+ 消化 2 条分层例外 + API 文档 | M1 |
| **M3 KB 链路事件化** | KB P1 队列/稳定性项 + 提示词统一治理 | M1/M2 |
| **M4 日志补齐** | log_center pattern 告警 + CleanOldEntries 调度 + Loki 部署 + trace_id 全链路 + goroutine 护栏 | M1 |
| **M5 前端收敛** | Matrix 7→3 + 爬取队列页 + 问答 SSE | M3/M4 |
| **M6 可观测性** | Grafana + cAdvisor | M4 |
| **1.0 GA** | M1-M5 完成 + STATUS 回写 + ARCHITECTURE 更新 + ARCHITECTURE-EXCEPTIONS 清零 LLM 两条 | 全部 |

---

## 6. 风险与护栏

| 风险 | 护栏 |
|------|------|
| eventbus 迁移引入回归 | 通知链路先迁、先验证，再迁 KB/LLM；每步 `go build ./...` + `go vet ./...` + 现有测试全绿 |
| llmgateway 重组破坏 `/api/llm/v1/*` 契约 | 对外路由与请求/响应结构不变；用现有 LLM 协议转换测试（`llm_anthropic_test.go` 30+ 用例、`gemini_test.go` 10 用例）守住 |
| 进程内直调跳过 Token 鉴权 | `Gateway.Chat` 不走 `LLMTokenAuth`（进程内可信），但须保留计费/限流/熔断/路由；外部调用仍走 HTTP + Token 鉴权 |
| KB 队列从 DB 轮询改 NATS 丢消息 | DB 状态机保留为真相源；NATS 至少一次 + AckExplicit + NakWithDelay 重试 |
| Agent 长任务事件回投导致回复延迟 | 简单命令保持 worker 池内同步；仅 Agent 多轮工具循环走事件回投；回投失败兜底直接发消息 |
| 日志补齐误改告警能力 | log_center 现有 threshold 告警 + 去重保留；pattern 为新增；`log_entries` 表用途不变 |
| goroutine panic 导致进程崩溃 | 所有 `go func()` 加 recover + zap 记录；纳入 P0/P1 护栏项 |
| 文档迁移丢上下文 | 归档不删除；ARCHITECTURE.md 合并时逐节对照，链接保留 |

---

## 附录：与现有 ROADMAP 的关系

本文件 §4 **取代** `doc/ROADMAP.md` 的活跃待办部分。ROADMAP.md 保留：
- 已完成里程碑表（历史记录）。
- 本文件 §4 的镜像（便于不读本文时也能看到待办）。

所有 §4 新增任务以本文件为准；ROADMAP 仅做镜像同步，不再独立增删。

**与原 ROADMAP 的关键修正**：
- ROADMAP §7 "日志中心"列了 Meili 全文检索/SSE/归档调度为待办——经核验这些**代码从未存在**，本文件改为"外挂 Loki 提供，Bellkeeper 内部不建"。
- ROADMAP §8.1 "Matrix 前端 7→3"保留；但"命令异步化"已完成（`sync.go:259-279`），本文件仅保留"Agent 长任务事件回投"。
- ROADMAP §3 "LLM Proxy"新增"进程内直调 + 分层例外清零"，这是原 ROADMAP 未覆盖但代码核验发现的核心痛点。
