# Bellkeeper 1.0 代码审查报告（基线审计）

> 审查日期：2026-07-06
> 审查基线：《Bellkeeper 1.0 重构与架构演进规划》（`BELLKEEPER-1.0-REVAMP-PLAN.md`）
> 审查范围：`internal/` 全部子包 + `web/` 前端 + `config/` 配置

---

## 1. 总体结论摘要

当前代码基底与 1.0 架构文档拟合度**约 85%**。M1-M5 架构骨架正确落地，LLM 分层例外已清零，NATS 事件总线覆盖 KB 全链路。然而审查发现 **5 个生产级严重缺陷**：`generateID()` 永远返回 `"0"`（Gemini 协议转换），阿里云余额查询缺失 HMAC-SHA1 签名，`GetLogger()` 存在读锁下写入的 data race，`activity_logs` 表 trace_id **永久丢失**（模型无字段），以及 `knowledge_index.go` 内层 goroutine 无 panic recover。此外存在 1 处空壳方法、1 处未登记的分层例外、若干错误静默吞没。M5（前端收敛）完成但 `AskStream` 为模拟流式非真 SSE 流式。

---

## 2. 模块审查清单

### P0 — 解耦基础设施

| 功能点 | 状态 |
|--------|------|
| `internal/eventbus/` 包新建，从 matrix/infra 提升 | [✅ 已完成] |
| 删除僵尸 `commands` stream | [✅ 已完成] |
| 扩展 6 条 stream（notifications/knowledge/llm/matrix/system/logs） | [✅ 已完成] |
| `Event` envelope 契约定义 | [✅ 已完成] |
| `service/notification.go` + `matrix/worker/notification_worker.go` 迁移到 eventbus.Client | [✅ 已完成] |
| `matrix/infra/nats.go` 删除 | [✅ 已完成] |
| `agent.go:56-64` 死代码清理 | [✅ 已完成] |
| NATS stream 配置化 | [✅ 已完成] |

### P1 — LLM 代理池

| 功能点 | 状态 |
|--------|------|
| 包重组为 `internal/llmgateway/` | [✅ 已完成] |
| 进程内直调 `Gateway.Chat` 接口 | [✅ 已完成] |
| 6 处内部调用方迁移（agent/classify/daily_report/knowledge_ask/rule_optimizer/llm_job_queue） | [✅ 已完成] |
| 消化分层例外①（`auth.LLMTokenAuth` 依赖接口） | [✅ 已完成] |
| 消化分层例外②（`LLMProxyHandler` 不持 repo） | [✅ 已完成] |
| `doc/LLM-GATEWAY-API.md` 新建 | [✅ 已完成] |

---

**Gemini 协议转换 `generateID()`** — [❌ 未实现/不合规]

- **确凿证据源**：`internal/llmgateway/converter/gemini.go:147-149` — `generateID()` 方法体
- **异常描述**：`return fmt.Sprintf("%d", 0)` 永远返回字符串 `"0"`，注释写 `"in production use crypto/rand"` 但从未实现。多个 Gemini 响应会因 ID 冲突导致客户端无法区分不同 chunk/响应。
- **验收标准 (AC)**：每次调用返回唯一的非零 ID
- **强制实现要求**：必须使用 `crypto/rand` 生成唯一 ID（如 ULID 或 UUID），替换当前 `return fmt.Sprintf("%d", 0)`

---

**阿里云 BSS 余额查询** — [❌ 未实现/不合规]

- **确凿证据源**：`internal/llmgateway/balance/aliyun.go:46-66` — `Fetch()` 方法
- **异常描述**：第 52-61 行构造了完整的 OpenAPI 请求参数（含 `SignatureMethod=HMAC-SHA1`），但**从未计算 Signature 参数**。阿里云 BSS OpenAPI 会因缺少签名直接返回 `InvalidSignature` 错误，此 provider 完全不可用。注释第 49 行承认"应使用正确的阿里云 SDK 签名"。
- **AC**：阿里云渠道能正确查询到账户余额
- **强制实现要求**：使用 HMAC-SHA1 对规范化参数字符串签名并追加 `Signature` 参数，或直接集成阿里云 Go SDK

---

**自适应限流学习器 `RecordSuccess`** — [⚠️ 仅有骨架]

- **确凿证据源**：`internal/llmgateway/llm_rate_limit_learner.go:154-157` — `RecordSuccess()` 方法
- **异常描述**：方法体完全为空（只有注释"未来跟踪桶压力比例以触发更快的探测"），全仓无任何调用方
- **AC**：成功请求能被记录并用于限流参数的自适应上调
- **强制实现要求**：实现 bucket 压力跟踪逻辑，或在确认不需要后删除空壳方法

---

**`TrackHalfOpenRequest` 死代码** — [⚠️ 仅有骨架]

- **确凿证据源**：`internal/llmgateway/llm_channel_health.go:223-229` — `TrackHalfOpenRequest()` 方法
- **异常描述**：方法有完整实现，但全仓 grep 确认无任何调用方。熔断器半开探测逻辑缺少此调用可能产生并发问题。
- **AC**：半开状态发送请求前应调用此方法进行并发控制
- **强制实现要求**：在 `gateway.go` 或 `router.go` 的请求发送路径中，半开状态时调用 `TrackHalfOpenRequest()`；或在确认不需要后删除

---

### P1 — 知识库

| 功能点 | 状态 |
|--------|------|
| 爬取→提取→索引事件化（crawl.done / extract.done） | [✅ 已完成] |
| `llm.job.submit` 事件化 + DB 状态机双路径 | [✅ 已完成] |
| 域名健康度事件化（`knowledge.crawl.failed` + `KBHealthWorker`） | [✅ 已完成] |
| 提示词外置到 `config/prompts/` + registry | [✅ 已完成] |
| `classify`/`rule_optimizer` system/user 角色分离 | [✅ 已完成] |
| `llmclient.ChatRequest` 增加 `ResponseFormat`/`MaxTokens` | [✅ 已完成] |
| 问答 rerank（召回 top-20 → rerank → top-5） | [✅ 已完成] |
| golden set 评估（`pkb-curate eval`） | [✅ 已完成] |

### P1 — Matrix Bot

| 功能点 | 状态 |
|--------|------|
| Agent 长任务结果事件回投（`matrix.agent.reply`） | [✅ 已完成] |
| 前端 7→2 页重构（MatrixConsole 合并 5 tab） | [✅ 已完成] |
| 删除死板管理操作，保留只读 + room_type 可改 | [✅ 已完成] |

### P1 — 日志补齐

| 功能点 | 状态 |
|--------|------|
| `log_center` 新增 pattern（正则）告警类型 | [✅ 已完成] |
| `CleanOldEntries` 接入每日调度 | [✅ 已完成] |
| trace_id HTTP 中间件生成 + context 注入 | [✅ 已完成] |
| goroutine panic 护栏（`SafeGo`） | [✅ 已完成] |
| Loki + Promtail 部署 | [🔶 待运维] |

---

**trace_id 写入 `activity_logs` 表** — [❌ 未实现/不合规]

- **确凿证据源**：`internal/service/activity_log.go:93-109` + `internal/model/activity_log.go:6-16`
- **异常描述**：`writeActivityLog` 接受 `traceID string` 参数（第 93 行），`LogActivityCtx` 也正确调用 `middleware.TraceIDFromContext(ctx)` 传入（第 62 行），但 `model.ActivityLog` 结构体**根本没有 `TraceID` 字段**（`activity_log.go:6-16`），traceID 在构造 `entry` 时被丢弃（第 100-108 行）。`log_entries` 表能正确记录 trace_id，但 `activity_logs` 表永久丢失 trace_id。
- **AC**：`activity_logs` 每条记录携带其对应请求的 trace_id
- **强制实现要求**：1) 在 `model.ActivityLog` 增加 `TraceID string` 字段（带 gorm tag + 索引）；2) `writeActivityLog` 第 100 行构造 entry 时写入 `TraceID: traceID`；3) 新增 DB 迁移或依赖 AutoMigrate

---

**`GetLogger()` 并发安全** — [❌ 未实现/不合规]

- **确凿证据源**：`internal/middleware/logger.go:61-66` — `GetLogger()` 方法
- **异常描述**：第 61-62 行使用 `RLock()`（读锁），但第 65 行在读锁保护下**写入** `defaultLogger`（`defaultLogger, _ = zap.NewProduction()`）。多个 goroutine 同时首次调用会使写操作并发执行，触发 Go 的 data race 检测器。此外第 65 行 `_` 丢弃了 `zap.NewProduction()` 的错误。
- **AC**：并发安全，经 `-race` 检测无警告
- **强制实现要求**：在 `RLock` 后检查 `defaultLogger == nil` 为真时，先 `RUnlock` 再获取写锁（`Lock()`），双重检查后初始化，或使用 `sync.Once`

---

**`knowledge_index.go` 内层 goroutine recover** — [❌ 未实现/不合规]

- **确凿证据源**：`internal/service/knowledge_index.go:172-182` — `indexFiles()` 方法内 `go func(f FileInfo)`
- **异常描述**：第 174 行 `defer func() { <-sem }()` 只释放信号量，**无 `defer recover()`**。10 个并发 goroutine 中任一个在 `s.indexFile(ctx, &f)` panic 都会直接导致进程崩溃。`StartFullScan:55-60` 和 `StartIncrementalScan:97-103` 已正确加入 recover，但 `indexFiles` 内部遗漏。
- **AC**：任何单文件索引失败不导致进程崩溃
- **强制实现要求**：在第 172 行的 goroutine 内增加 `defer recover()` 并 zap 记录 panic 信息

---

### P1 — 前端

| 功能点 | 状态 |
|--------|------|
| 爬取队列可视化页（`CrawlQueue.tsx`） | [✅ 已完成] |
| 问答 SSE 流式（后端 `AskStream` + 端点） | [✅ 已完成] |

---

**AskStream 真流式改造** — [⚠️ 仅有骨架]

- **确凿证据源**：`internal/service/knowledge_ask.go:159-163` — `AskStream()` 方法注释
- **异常描述**：注释第 163 行明确写"当前实现为模拟流式：复用 Ask 拿到完整 answer…Gateway.ChatStream 待后续落地"。当前实现是先同步获取完整答案再切片推送，不是真正的 LLM token 级流式。虽然给用户提供了打字机体验，但首字节延迟等于完整答案延迟，失去了流式的核心价值。P1 任务标注为"已完成"，但真流式属未完成。
- **AC**：通过 `Gateway.ChatStream` 实现 token 级真流式推送
- **强制实现要求**：用 `llmgateway.Gateway.ChatStream` 替换当前 `s.Ask(ctx, req)` 同步调用，将 stream chunk 转换为 SSE delta event 实时推送到前端

---

### P2 — 可观测性

| 功能点 | 状态 |
|--------|------|
| Grafana 看板 + cAdvisor | [🔶 待运维] |

---

## 3. 架构规范违规警告

### 违规 1：`ExtractionRuleHandler` 持 repo — 未登记的分层例外

- **确凿证据源**：`internal/handler/extraction_rule.go:14-17`
  ```go
  type ExtractionRuleHandler struct {
      ruleSvc  *service.RuleOptimizerService
      ruleRepo *repository.CrawlExtractionRuleRepository  // ← handler 直接持 repo
  }
  ```
- **异常描述**：5 个 HTTP 方法（`ListRules`、`GetRule`、`CreateRule`、`UpdateRuleStatus`、`ListTrials`）全部绕过 `ruleSvc` 直接调用 `h.ruleRepo`。`handler/handler.go:83-86` 允许 `ruleSvc` 为 nil 时回退到 repo 直调。这违反了 `Router → Handler → Service → Repository` 的严格单向分层规则（`CLAUDE.md §2.1`），且此例外**未在 `ARCHITECTURE-EXCEPTIONS.md` 登记**。
- **AC**：handler 不直接持有 repo 引用
- **强制实现要求**：将所有 rule CRUD 方法沉入 `RuleOptimizerService` 或新建 `ExtractionRuleAdminService` 作为 service 层；handler 仅调用 service；同时在 `ARCHITECTURE-EXCEPTIONS.md` 登记（或直接修复后不予登记）

### 违规 2：`LLM-GATEWAY-API.md` 状态滞后

- **确凿证据源**：`doc/LLM-GATEWAY-API.md:95` — 仍标注分层例外为"消化中"，但 `doc/ARCHITECTURE-EXCEPTIONS.md` 已将两条 LLM 例外标为 `✅ 已清零`
- **强制实现要求**：更新 `LLM-GATEWAY-API.md` 第 95 行为"已清零（1.0 重构完成）"

### 违规 3：错误被静默丢弃（9 处）

| 文件 | 行号 | 代码 |
|------|------|------|
| `service/log_center.go` | 300-301 | `json.Unmarshal` 失败后 `continue`，无日志 |
| `service/log_center.go` | 338-339 | `regexp.Compile` 失败返回空，无日志 |
| `service/log_center.go` | 349-350 | `entryRepo.List` 失败返回空，无日志 |
| `service/log_center.go` | 54-57 | `json.Marshal` error 被 `_` 丢弃 |
| `service/crawl_queue.go` | 244 | `json.Marshal(metadata)` error 被 `_` 丢弃 |
| `service/kb_extract_worker.go` | 142 | `msg.Metadata()` error 被 `_` 丢弃 |
| `middleware/logger.go` | 65 | `zap.NewProduction()` error 被 `_` 丢弃 |
| `handler/knowledge.go` | 89 | `json.Marshal` error 被 `_` 丢弃（SSE 路径） |
| `llmgateway/llm_proxy.go` | 238 | `LoadCache()` error 被 `_` 丢弃 |

违反 `CLAUDE.md §2.3`（"永不忽略 error，禁止 `_ = f()`"）。

### 违规 4：日志输出不一致（4 处）

| 文件 | 行号 | 现状 | 应改为 |
|------|------|------|--------|
| `service/activity_log.go` | 111 | `log.Printf` | `middleware.GetLogger()` |
| `service/log_center.go` | 73 | `log.Printf` | `middleware.GetLogger()` |
| `service/knowledge_index.go` | 180 | `log.Printf` | `middleware.GetLogger()` |
| `matrix/gateway/sync.go` | 270-271 | `log.Printf` | `middleware.GetLogger()` |

违反架构文档 §2.4.2"应用日志使用 zap"的统一日志策略。

---

## 4. 修复优先级汇总

### P0（生产级缺陷，必须修）

| # | 问题 | 位置 |
|---|------|------|
| 1 | `generateID()` 永远返回 `"0"` | `llmgateway/converter/gemini.go:149` |
| 2 | 阿里云余额查询缺失签名 | `llmgateway/balance/aliyun.go:46-66` |
| 3 | `GetLogger()` RLock 下写入 data race | `middleware/logger.go:61-66` |
| 4 | `activity_logs` trace_id 永久丢失 | `model/activity_log.go:6-16` + `service/activity_log.go:100-108` |
| 5 | `indexFiles` 内层 goroutine 无 recover | `service/knowledge_index.go:172-182` |

### P1（1.0 GA 前应修）

| # | 问题 | 位置 |
|---|------|------|
| 6 | `RecordSuccess` 空壳方法 | `llmgateway/llm_rate_limit_learner.go:154-157` |
| 7 | `TrackHalfOpenRequest` 无调用方（死代码） | `llmgateway/llm_channel_health.go:223-229` |
| 8 | `ExtractionRuleHandler` 未登记分层例外 | `handler/extraction_rule.go:14-17` |
| 9 | `AskStream` 为模拟流式，非真 SSE | `service/knowledge_ask.go:163` |
| 10 | `LLM-GATEWAY-API.md:95` 状态过期 | `doc/LLM-GATEWAY-API.md:95` |

### P2（改进建议）

| # | 问题 |
|---|------|
| 11 | 9 处 error 被 `_` 丢弃，需加日志或错误处理 |
| 12 | 4 处 `log.Printf` 改为 zap |
