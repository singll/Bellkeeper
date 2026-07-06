# Bellkeeper 1.0 重构里程碑记录

> 记录 2026-07-03 启动的 Bellkeeper 1.0 重构与架构演进各里程碑交付。
> 权威规划见《Bellkeeper 1.0 重构与架构演进规划》；状态镜像见 [STATUS.md](STATUS.md)。
> 本文件为里程碑索引，每里程碑含交付摘要 + 关键产物 + 验证状态。

---

## 里程碑总览

| 里程碑 | 状态 | 完成日期 | 依赖 |
|--------|------|----------|------|
| **M1 解耦地基** | ✅ 完成 | 2026-07-03 | 无 |
| **M2 LLM 独立化** | ✅ 完成 | 2026-07-03 | M1 |
| **M3 KB 链路事件化** | ✅ 完成 | 2026-07-03 | M1/M2 |
| **M4 日志补齐** | ✅ 代码完成 | 2026-07-03 | M1 |
| **M5 前端收敛** | ✅ 完成 | 2026-07-03 | M3/M4 |
| **M6 可观测性** | 🔶 待运维 | — | M4 |
| **1.0 GA** | 🔶 收口中 | — | M1-M6 |

---

## M1 解耦地基 ✅

**目标**：把 NATS 从 matrix 子包提升为一级共享基础设施，建立事件总线契约，清理僵尸配置与死代码。

**交付**：
1. 新建 `internal/eventbus/` 包，从 `matrix/infra/nats.go` 提升 `NATSClient`+`ensureStreams` 为 `eventbus.Client`。
2. **删除僵尸 `commands` stream**（全仓无 Publish/Subscribe）。
3. 按 §1.2.1 表扩展 `knowledge`/`llm`/`matrix`/`system`/`logs` 五条 stream（WorkQueue/Interest/Limits 分级 + MaxAge 分级）。
4. 定义 `Event` envelope 契约（§1.2.2）于 `internal/eventbus/event.go`：ULID EventID + Type/Source/OccurredAt/Subject/Payload(json.RawMessage)/TraceID，`New()`+`PublishEvent()`+`UnmarshalEvent()`。
5. 通知链路迁移至 `eventbus.Client`：`service/notification.go` + `worker/notification_worker.go`；迁移后 `matrix/infra/nats.go` 全仓零引用已删除。
6. `config.NATSStreamsConfig` 增字段 + `bellkeeper.yaml` 补默认值。
7. 清理 `agent.go:56-64` 死代码（`maxIter` 重复定义）。

**关键产物**：`internal/eventbus/{client.go,event.go}`、`config/bellkeeper.yaml`、`doc/STATUS.md`。
**验证**：`go build`+`go vet`+config/eventbus/matrix/service 测试全绿。

---

## M2 LLM 独立化 ✅

**目标**：把 LLM 代理池从 service 层剥离为独立内部基础设施，提供进程内直调接口，消化分层例外。

**交付**（5 子步）：
- **M2-1**：`internal/llm/{converter,errors,balance}` → `internal/llmgateway/{converter,errors,balance}`。
- **M2-2**：`service/llm_anthropic.go`+test → `llmgateway/converter/anthropic.go`，30+ 测试守护契约。
- **M2-3+M2-4**：整体迁移 13 个 `llm_*.go`（含 llm_proxy.go 2447 行）到 `internal/llmgateway/` 包；138 处外部引用加前缀；`llm_alert_notifier.go` 留 service 包实现 `AlertNotifier` 接口（破除循环）。
- **M2-5**：`Gateway` 接口（Chat/Rerank）+ `*LLMProxyService` 进程内直调实现；6 处调用方迁移（agent/classify/daily_report/knowledge_ask/rule_optimizer/llm_job_queue）替代 `llmclient.New` HTTP 构造；`llmclient.Client` 实现 Gateway 供进程外 CLI。
- **分层例外清零**：新建 `internal/llmgateway/admin.go` 的 `TokenScopeService` 接口 + `LLMAdminService`（token/pricing 管理 CRUD + 计费试算）；`router.Setup` 改传 `auth.LLMTokenStore` 接口；`LLMProxyHandler` 只依赖 service 不持 repo。两条 LLM 分层例外标 ✅ 已清零。

**关键产物**：`internal/llmgateway/`（gateway.go/admin.go + converter/errors/balance 子包）、`doc/LLM-GATEWAY-API.md`、`doc/ARCHITECTURE-EXCEPTIONS.md`（清零标注）。
**验证**：`go build`+`go vet`+llmgateway/converter/service/handler/pkb/auth 测试全绿。对外路由 `/api/llm/v1/*`+`/api/llm/*` 契约不变，anthropic/gemini 30+ 用例守护。

---

## M3 KB 链路事件化 ✅

**目标**：KB 内部队列从 DB 轮询改为 NATS 事件驱动，统一提示词治理，补齐稳定性与评估。

**交付**（6 子项）：
- **M3-1 crawl/extract/index 事件化**：`crawl_jobs` 加 `crawled` 中间态；`processJob` 拆分（crawl worker 只抓取提取 → 发 `knowledge.crawl.done` → extract-worker 入库 → 发 `knowledge.extract.done` → index-worker 刷新 Meili）；`RecoverStaleCrawledJobs` 护栏。
- **M3-2 llm.job.submit 事件化**：`EnqueueChat` 发事件；`DequeueByID` 原子 claim；`eventWorkerLoop` NATS 消费；`republishReadyJobs` 兜底；bus==nil 降级 DB 轮询。
- **M3-3 域名健康度事件化**：`CrawlDomainProfile` 加 `ConsecutiveFailures`/`HealthScore`/`IsPaused`；`EvaluateDomainHealth` 自动暂停/恢复；`knowledge.crawl.failed` 事件 + `KBHealthWorker` 消费 + alert。
- **M3-4 提示词治理统一**：`config/prompts/` + registry；`KBPromptLoader` 加载器；classify/rule_optimizer system/user 分离 + `ResponseFormat=json_object` + 自修复重试；knowledge_ask system 外置。
- **M3-5 问答 rerank**：`Gateway.Rerank` + `AskService` 召回 top-20 → rerank → top-5；单片段限长 1200 runes。
- **M3-6 golden set 评估**：`config/pkb/eval/*.json` + `pkb-curate eval` 子命令（accuracy + MAE）。

**关键产物**：`internal/service/{kb_events.go,kb_extract_worker.go,kb_index_worker.go,kb_health_worker.go,prompts.go}`、`internal/pkb/eval.go`、`config/prompts/`、`config/pkb/eval/`。
**验证**：`go build`+`go vet`+service/repository/llmgateway/pkb/config 测试全绿。

---

## M4 日志补齐 ✅（代码完成）

**目标**：保持轻量 + 外挂 Loki + trace_id 全链路 + goroutine 护栏。

**交付**（4 项代码）：
- **[log-pattern]**：`AlertCondition.Pattern` 字段 + `checkPatternRule`（正则匹配 summary，scan 上限 1000）。
- **[log-schedule]**：`CleanOldEntries` 接入 `logCleanupLoop` + `LogCenterConfig{RetentionDays, CleanupIntervalHrs}`（默认 30/24）。
- **[log-trace] trace_id 全链路**：`middleware/trace.go` `TraceID()` 中间件（复用上游/生成 UUID + 注入 gin.Context + context.WithValue + 回写 header）+ `TraceIDFromContext`；router 全局注册；`eventbus.traceIDFromContext` 接入；`LogActivityCtx` 从 ctx 取 trace_id。
- **[log-guard]**：`service/goroutine.go` `SafeGo`（recover + zap）；log_center/activity_log goroutine 改 SafeGo；knowledge_index 两处加 defer recover。

**待运维**：`[log-loki]` Loki+Promtail 部署（spool bundle，非代码）。
**关键产物**：`internal/middleware/trace.go`、`internal/service/goroutine.go`、`internal/service/log_center.go`、`internal/config/config.go`。
**验证**：`go build`+`go vet`+service/config/middleware/eventbus 测试全绿。

---

## M5 前端收敛 ✅

**目标**：Matrix 7→3 页重构 + 爬取队列可视化 + 问答 SSE 流式。

**交付**（3 项）：
- **[matrix-ui] 7→2 页**：新建 `MatrixConsole.tsx`（tab 合并 Rooms/Commands/Notifications/Events/CommandLogs，只读+room_type 可改），删 5 旧页；Layout 导航 Matrix 只留 2 项；配置归全局 Settings。
- **[fe] 爬取队列页**：新建 `CrawlQueue.tsx`（统计含 crawled + 域名健康度 HealthScore/Paused + 任务列表重试）+ `crawlQueueApi`；路由 `/crawl-queue`。
- **[fe] 问答 SSE 流式**：后端 `AskService.AskStream`（模拟流式切片）+ `AskStream` SSE 端点（`/api/files/ask/stream`）；前端 fetch+ReadableStream 消费（打字机渲染）。

**关键产物**：`web/src/pages/{MatrixConsole.tsx,CrawlQueue.tsx}`、`internal/service/knowledge_ask.go`（AskStream）、`internal/handler/knowledge.go`。
**验证**：`pnpm build`+`go build`+`go vet`+service/handler/router 测试全绿。

---

## M6 可观测性 🔶（待运维）

**目标**：Grafana 看板 + cAdvisor 容器资源压力检测。

**状态**：Prometheus 端点已有（`internal/metrics/metrics.go`），缺 Grafana 容器 + dashboard JSON + cAdvisor。
**待办**：通过 `spool bundle` 在 keeper 主机部署 Grafana + cAdvisor。

---

## 1.0 GA 🔶（收口中）

**剩余项**：
1. 🔶 M4-5 Loki+Promtail 部署（spool bundle 运维）
2. 🔶 M6 Grafana+cAdvisor 部署（spool bundle 运维）
3. ARCHITECTURE.md 按 1.0 终态更新
4. ARCHITECTURE-EXCEPTIONS.md 最终核对（LLM 两条已清零）
5. 全量回归测试 + 部署 keeper 验证

**已完成**：M1-M5 代码全部交付，`go build`+`go vet`+全量测试绿。