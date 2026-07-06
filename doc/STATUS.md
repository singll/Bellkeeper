# SilkSpool 项目进度

> 最后更新: 2026-07-03
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
| **事件总线** | ✅ **1.0 落地** | `internal/eventbus/` 一级共享基础设施；6 条 stream（notifications/knowledge/llm/matrix/system/logs）+ Event envelope（ULID+TraceID）；删僵尸 commands stream |
| Bellkeeper 后端 | ✅ **1.0 重构** | eventbus 贯穿；LLM 代理池独立 `llmgateway` 包 + 进程内直调；KB 链路事件化（crawl/extract/index/llm.job）；trace_id 全链路 |
| Bellkeeper 前端 | ✅ **1.0 收敛** | Matrix 7→2 页（Dashboard+Console）；爬取队列可视化页；知识问答 SSE 流式 |
| LLM 代理 | ✅ **1.0 独立化** | `internal/llmgateway/` 独立包 + `Gateway` 进程内直调接口（Chat/Rerank）+ 7 处调用方迁移 + 分层例外清零（TokenScopeService+LLMAdminService） |
| LLM 持久队列 | ✅ **1.0 事件化** | `llm_jobs` DB 状态机 + NATS `llm.job.submit` 事件驱动（DequeueByID 原子 claim + recoveryLoop 重投兜底） |
| 个人知识库 PKB | ✅ 原子知识网 | Phase A–D 全部落地；骨架 Phase F–H + 自动闭环 + 域管理 + 前端骨架页；**1.0 新增 golden set 评估（pkb-curate eval）+ 提示词外置统一（config/prompts + ResponseFormat + system/user 分离 + 自修复重试）** |
| 爬取与提取 | ✅ **1.0 事件化** | DequeueFair + 域名冷却 + LLM 规则优化；**processJob 拆分（crawl/extract/index 三 worker）+ crawled 中间态 + 域名健康度（HealthScore/Pause 自动暂停恢复）** |
| 标签管线 | ✅ 增强完成 | 置信度 + 规范化 + tag_source 三处持久化 |
| RSS | ✅ 稳定 | feed 验证 API + RSSHub 参数支持 + 自动暂停/恢复 |
| Meilisearch 检索 | ✅ **1.0 事件驱动** | `/api/files/search\|ask`；archive+vault 三层索引隔离；**入库即触发 index-worker 刷新（替代定时扫描）** |
| Matrix 控制平面 | ✅ 稳定 | Gateway + Command Router + Agent + 通知网关（eventbus）+ 权限两层制 + 通知聚合去重 + Admin API |
| Agent 系统 | ✅ MVP | AgentService + Redis 会话 + 限速 + 权限分级；**1.0 进程内直调 Gateway 替代 HTTP 自回环** |
| 日报系统 | ✅ 稳定 | 后端驱动 + 并行采集器 + n8n 仅触发；**1.0 进程内直调 Gateway** |
| n8n 工作流治理 | ✅ 落地 | 8 个活跃工作流；10 个已退役归档 |
| 日志中心 | ✅ **1.0 补齐** | threshold + **pattern（正则）告警** + CleanOldEntries 调度 + **trace_id 全链路**（HTTP 中间件+eventbus+log_center）+ goroutine panic 护栏；🔶 Loki+Promtail 外挂部署待运维 |
| 架构治理 | ✅ **1.0 例外清零** | Phase 0-10 完成；**两条 LLM 分层例外已清零（router 用接口+handler 不持 repo）** |
| 认证 | ✅ 无需 | 纯内网 noauth；LLM Token 鉴权独立保留 |
| 测试 | ✅ 核心覆盖 | 31 Repository 全覆盖 + 核心链路行为测试 + LLM 协议转换测试（anthropic/gemini）+ pkb eval 回归 |
| lint | ✅ 零 error | golangci-lint v2；error 清零 |

---

## 1.0 重构里程碑状态

| 里程碑 | 状态 | 交付摘要 |
|--------|------|----------|
| **M1 解耦地基** | ✅ 完成 | eventbus 一级共享基础设施 + Event 契约 + 删僵尸 commands stream + 通知链路迁移 + agent 死代码清理 |
| **M2 LLM 独立化** | ✅ 完成 | llmgateway 包重组（converter/errors/balance + 13 个 llm_*.go）+ Gateway 进程内直调（Chat/Rerank）+ 6 处调用方迁移 + 分层例外清零（TokenScopeService+LLMAdminService）+ LLM-GATEWAY-API.md |
| **M3 KB 链路事件化** | ✅ 完成 | crawl/extract/index 三 worker 事件链 + llm.job.submit 事件化 + 域名健康度（HealthScore/Pause）+ 提示词外置统一（config/prompts+ResponseFormat+system/user 分离+自修复重试）+ 问答 rerank + golden set 评估 |
| **M4 日志补齐** | ✅ 代码完成 | pattern 告警 + CleanOldEntries 调度 + trace_id 全链路 + goroutine 护栏；🔶 Loki+Promtail 部署待运维（spool bundle） |
| **M5 前端收敛** | ✅ 完成 | Matrix 7→2 页（Dashboard+Console）+ 爬取队列可视化页 + 知识问答 SSE 流式 |
| **M6 可观测性** | 🔶 待运维 | Grafana + cAdvisor 部署（spool bundle 运维项） |
| **1.0 GA** | 🔶 收口中 | M1-M5 代码完成；M4-5 Loki + M6 Grafana 为运维部署项；待 ARCHITECTURE/EXCEPTIONS 最终核对 + 全量回归 |

---

## 最近主线动作

| 时间 | 动作 |
|------|------|
| 2026-07-03 | **Bellkeeper 1.0 重构启动 — M1 解耦地基 P0**：① 新建 `internal/eventbus/` 包，从 `matrix/infra/nats.go` 提升 `NATSClient`+`ensureStreams` 为一级共享基础设施 `eventbus.Client`；**删除僵尸 `commands` stream**（全仓无 Publish/Subscribe）；按 §1.2.1 表扩展 `knowledge`/`llm`/`matrix`/`system`/`logs` 五条 stream（WorkQueue/Interest/Limits 分级 + MaxAge 分级）；`config.NATSStreamsConfig` 增字段 + `bellkeeper.yaml` 补默认值。② 定义 `Event` envelope 契约（§1.2.2）于 `internal/eventbus/event.go`：ULID EventID + Type/Source/OccurredAt/Subject/Payload(json.RawMessage)/TraceID，`New()`+`PublishEvent()`+`UnmarshalEvent()`；引入 `oklog/ulid/v2`。③ **通知链路迁移至 eventbus.Client**：`service/notification.go`（字段+构造+2处Publish）+ `worker/notification_worker.go`（字段+构造+1处Subscribe）改依赖 `*eventbus.Client`；`app.go` 字段 `natsClient` 改 `eventBus` 用 `eventbus.NewClient`；迁移后 `matrix/infra/nats.go` 全仓零引用已删除（仅留 redis.go）。`go build`+`go vet`+config/eventbus/matrix/service 测试全绿。④ **清理 agent.go 死代码**：删除构造函数中 `maxIter` 定义后 `_ = maxIter` 丢弃又在 `HandleMessage` 重新定义的重复段（原 56-64 + 83 行），仅保留真正使用的局部变量。**M1 解耦地基完成**。
| 2026-07-03 | **M2 LLM 独立化**：① **M2-1** — 把 `internal/llm/{converter,errors,balance}` 三个子包整体 `git mv` 迁为 `internal/llmgateway/{converter,errors,balance}`（纯目录迁移 + import 路径替换，gemini_test 守护）。② **M2-2** — 把 `service/llm_anthropic.go`+test 迁入 `llmgateway/converter`（与 gemini 同属协议转换），`anthropicVersion` 导出为 `converter.AnthropicVersion`，30+ anthropic 测试守住契约。③ **M2-3+M2-4** — 整体把 `service/llm_*.go`（除 `llm_alert_notifier.go` 外，共 13 文件含 llm_proxy.go 2447 行）`git mv` 迁入 `internal/llmgateway/` 包，`package service`→`package llmgateway`；导出 `LLMJobIdempotencyKey`（原未导出被跨包引用）；批量给 service/handler/app/router/classify/daily_report/rule_optimizer/knowledge_ask/pkb/matrix-agent/auth/llmclient/cmd 中 138 处引用加 `llmgateway.` 前缀 + 补 import；`llm_alert_notifier.go` 留 service 包实现 `llmgateway.AlertNotifier` 接口（避免 llmgateway→service 反向依赖，循环破除）；`NewMatrixAlertNotifier` 留 service 包。**对外路由 `/api/llm/v1/*`+`/api/llm/*` 不变**。`go build`+`go vet`+全量测试（converter/service/handler/pkb/auth 等）全绿。
④ **M2-5 进程内直调** — 新建 `internal/llmgateway/gateway.go` 定义 `Gateway` 接口（`Chat(ctx, req, opts) (*ChatResponse, error)`，复用 `llmclient.ChatRequest/ChatResponse/ChatOptions` 类型契约），`*LLMProxyService.Chat` 实现之（内部构造 OpenAI `/v1/chat/completions` headers/body 调 `ProxyRequest`，绕 HTTP+Token 鉴权但保留路由/熔断/限流/粘性/计费）；6 处进程内调用方迁移替代 `llmclient.New` HTTP 构造——`matrix/agent/agent.go`、`service/classify.go`、`service/daily_report.go`、`service/knowledge_ask.go`、`service/rule_optimizer.go`、`llmgateway/llm_job_queue.go`，字段改 `llmgateway.Gateway`，`.ChatCompletion`→`.Chat`+`.Content`；`service.go` 构造顺序调整（LLMProxy 提前构造传给各调用方）；`llmclient.Client` 加 `Chat` 方法实现 `Gateway` 接口供进程外 CLI（`cmd/bellkeeper` 的 pkb-curate 子命令仍经 localhost HTTP+Token）。`pkb/client.go` 保留 llmclient（进程外 CLI 性质）。`go build`+`go vet`+llmgateway/llmclient/service/handler/pkb/matrix 测试全绿。**待办**：分层例外消化（①`TokenScopeService` ②`LLMAdminService`）为独立下一原子任务。
⑤ **[llmgw] 消化分层例外（清零两条）** — 新建 `internal/llmgateway/admin.go`：① 定义 `TokenScopeService` 接口（= `auth.LLMTokenStore` 方法集），`*LLMAdminService` 实现之；`router.Setup`/`registerLLMProxyRoutes` 形参 `*repository.LLMTokenRepository` → `auth.LLMTokenStore` 接口，`app.go` 注入 `services.LLMAdmin`。② `LLMProxyHandler` 删 `pricer`/`tokenRepo`/`tokenUsageRepo`/`pricingRepo` 4 字段，只持 `*LLMProxyService`+`*LLMAdminService`；11 个 token/pricing 管理 handler（ListTokens/CreateToken/UpdateToken/DeleteToken/RegenerateTokenKey/GetTokenUsage/ListPricing/CreatePricing/UpdatePricing/DeletePricing/TestPricingCalc）改为调 `LLMAdminService` 业务方法（`CreateTokenRequest`/`UpdateTokenRequest` 等 DTO），handler 不再碰 repo/pricer；`Services` 加 `LLMAdmin` 字段，`service.go` 构造 `LLMAdminService`（复用 pricer）注入 handler。`ARCHITECTURE-EXCEPTIONS.md` 两条 LLM 例外标 ✅ 已清零。`go build`+`go vet`+llmgateway/handler/router/service/auth 测试全绿。**M2 LLM 独立化里程碑完成**。
| 2026-07-03 | **M3 KB 链路事件化 — M3-1**：① `crawl_jobs` 状态机新增 `crawled` 中间态（爬取完成待 extract-worker 入库）。② 拆分 `CrawlQueueService.processJob`：crawl worker 只做抓取+提取（`extractor.Extract`），成功后发 `knowledge.crawl.done` 事件（payload 含 content，≤512KB）+ 状态置 `crawled`；入库 `IngestURL` 与 Meili 索引剥离到独立 worker；超限/无 publisher 走 `ingestAndFinalize` fallback 同步入库（行为不变）。③ 新建 `KBEventPublisher`（`kb_events.go`，crawl.done/extract.done 发布 + payload 契约）。④ 新建 `KBExtractWorker`（`kb_extract_worker.go`）消费 `knowledge.crawl.done` → `IngestURL` 入库 → 发 `knowledge.extract.done`；NakWithDelay 重试 + 超限 Ack 落 failed。⑤ 新建 `KBIndexWorker`（`kb_index_worker.go`）消费 `knowledge.extract.done` → `KnowledgeIndexService.IndexFile` 刷新 Meili。⑥ 护栏：`RecoverStaleCrawledJobs` 回收卡在 crawled 超时 job（防 extract-worker 崩溃卡死）；`CrawlQueueStats` 加 crawled 计数。⑦ app.go 接线 publisher + 两 worker Start/Stop。`go build`+`go vet`+service/repository/eventbus 测试全绿。
| 2026-07-03 | **M3 KB 链路事件化 — M3-2 llm.job.submit 事件化**：`LLMJobQueueService` 从 DB 轮询改为 NATS 事件驱动。① `EnqueueChat` 写 DB 后发 `llm.job.submit` 事件（payload 仅 job_id，DB 为状态真相源），发布失败不阻断（recovery 兜底）。② 新增 `repo.DequeueByID` 原子 claim（pending/retrying→running，SKIP LOCKED），保证重复事件只有一个消费者处理。③ `Start` 区分事件/轮询模式：bus≠nil 时 PullSubscribe + `eventWorkerLoop` Fetch 消费；bus==nil 降级 `workerLoop` DB 轮询（行为不变）。④ `recoveryLoop` 事件模式下新增 `republishReadyJobs`：查 `ListReadyIDs` 到期 pending/retrying job 重发事件，兜底 EnqueueChat 事件丢失/worker 崩溃。⑤ `SetEventBus` 方法供 app.go 在 eventBus 创建后注入（Start 前注入）；cmd CLI 7 处传 nil（进程外仍 DB 轮询）。`go build`+`go vet`+llmgateway/repository/service 测试全绿。
| 2026-07-03 | **M3 KB 链路事件化 — M3-3 域名健康度事件化**：① `CrawlDomainProfile` model 扩展 `ConsecutiveFailures`/`HealthScore`/`IsPaused`/`PausedReason`/`PausedAt` 字段（对齐文档 §2.1.1，原表仅 `FailureCount`/`NextAllowedAt`）；`DequeueFair` SQL 加 `is_paused=false` 过滤，暂停域名不出队。② repo `EnterCooling` 同步增 `ConsecutiveFailures`+降 `HealthScore`（-10）；`ClearCooling` 重置失败计数 + 回血 HealthScore（+20）；新增 `EvaluateDomainHealth`（ConsecutiveFailures≥阈值暂停、HealthScore≥阈值恢复，返回 action）。③ 阈值进 `CrawlQueueConfig.DomainPauseThreshold`（默认5）/`DomainResumeThreshold`（默认30）。④ `handleExtractionFailure`/`handleEmptyContent` 发布 `knowledge.crawl.failed` 事件（payload: domain/job_id/errType/terminal 标志）。⑤ 新建 `KBHealthWorker`（`kb_health_worker.go`）消费 crawl.failed → `EvaluateDomainHealth` → 暂停/恢复时发 `system.health.alert`（经 NotificationService 投 Matrix alerts 频道，dedup_key 防刷屏）。⑥ app.go 接线 worker Start/Stop。`go build`+`go vet`+service/repository/config 测试全绿。
| 2026-07-03 | **M3 KB 链路事件化 — M3-4 提示词治理统一**：① `llmclient.ChatRequest` + `EnqueueLLMChatOptions` 新增 `ResponseFormat`/`MaxTokens` 字段，proxy 透传 provider。② 新建 `config/prompts/` 目录（与 `config/pkb/prompts/` 并列）+ `registry.yaml` + 5 个外置提示词（classify_system/user、knowledge_ask_system、rule_optimizer_system/user），复用 PKB registry 模式。③ 新建 `internal/service/prompts.go` 通用加载器 `KBPromptLoader`（线程安全 once 加载 + 路径安全 + `GetWithDefault` 回退）。④ classify/rule_optimizer 改 system/user 分离（规则段入 system 利用 provider cache），`ResponseFormat=json_object` 强制结构化输出；classify 加自修复重试（JSON 解析失败带错误回喂一次）。⑤ knowledge_ask system prompt 外置（loader 缺失回退内置）。⑥ service.go 构造 loader 注入 classify/rule_optimizer，app.go 注入 ask；`cfg.Prompt` 保留向后兼容（整段覆盖）。`go build`+`go vet`+service/llmclient/llmgateway/config 测试全绿。
| 2026-07-03 | **M3 KB 链路事件化 — M3-5 问答 rerank + 上下文压缩**：① `llmclient` 新增 `RerankRequest/RerankResult/RerankResponse` 类型 + `Client.Rerank`（HTTP `/v1/rerank`）。② `Gateway` 接口加 `Rerank` 方法 + `*LLMProxyService.Rerank` 进程内直调（路由 `/v1/rerank`，绕 HTTP+鉴权，保留熔断/限流/rerank channel 路由）。③ `AskService.Ask` 改召回 top-20 → `rerankHits`（Gateway.Rerank 重排，model=pool-rerank，documents=title+snippet）→ top-5 → buildContext；rerank 失败降级 Meili 原始顺序取 topN（不阻断问答）。④ `buildContext` 改为 rerank 后片段标注序号 + 单片段限长 1200 runes（避免长文挤占，snippet 已是 Meili 高亮摘要性质，跳过额外 LLM 摘要压缩以控成本/延迟）。`go build`+`go vet`+service/llmclient/llmgateway 测试全绿。
| 2026-07-03 | **M3 KB 链路事件化 — M3-6 golden set 评估**：① 新建 `config/pkb/eval/` 目录 + 2 个示例 golden 样本（`sample-sqli.json` 安全 PoC 高分 + `sample-marketing.json` 营销稿 discard，用户可扩充到 10-20）。② 新建 `internal/pkb/eval.go`：`EvalSample`/`EvalExpected`/`EvalCaseResult`/`EvalReport` 类型 + `RunEval(cfg, opts)` 加载 golden set → 构造 DryRun Curator 复用 `scoreArticle` → 对比各维度分差（tolerance 默认2）+ matched_domains 交集 + content_type 匹配 + decision（vault/archive/discard）→ 输出 accuracy + 各维度 MAE。③ `cmd/bellkeeper/main.go` 加 `pkb-curate eval` 子命令（`--json`/`--tolerance`/`--pkb-config`）+ `runPkbEval` + `printEvalReport`（✓/✗ 标记 + 分差 + MAE）。**M3 KB 链路事件化里程碑完成**。`go build`+`go vet`+pkb 测试全绿。
| 2026-07-03 | **M4 日志补齐**：① **[log-pattern]** `AlertCondition` 加 `Pattern` 字段 + `checkPatternRule`（正则匹配 summary，scan 上限 1000 防全表扫描，与 Module/Level/Window 叠加）。② **[log-schedule]** `CleanOldEntries` 接入 `logCleanupLoop`（启动先执行一次 + 每 `CleanupIntervalHrs` 周期）；新增 `LogCenterConfig{RetentionDays, CleanupIntervalHrs}` + 默认值 30/24。③ **[log-trace] trace_id 全链路**：新建 `middleware/trace.go` `TraceID()` 中间件（优先复用上游 `X-Trace-Id`，否则生成 UUID，注入 gin.Context + context.WithValue + 回写 response header）+ `TraceIDFromContext` 辅助；router 全局注册（最前）；`eventbus.traceIDFromContext` 接入（替换占位，事件链贯穿）；`ActivityLogService.LogActivityCtx(ctx, p)` 从 ctx 取 trace_id 写 `log_entries.TraceID`。④ **[log-guard]** 新建 `service/goroutine.go` `SafeGo(name, fn)`（panic recover + zap 记录）；log_center.LogActivity + activity_log 两处 goroutine 改 SafeGo；knowledge_index StartFullScan/StartIncrementalScan 加 defer recover。`go build`+`go vet`+service/config/middleware/eventbus 测试全绿。**待办**：`[log-loki]` Loki+Promtail 部署为 spool bundle 运维项（非代码）。
| 2026-07-03 | **M5 前端收敛**：① **[matrix-ui] 7→2 页重构**（§2.3.3 T9）：新建 `MatrixConsole.tsx` 合并原 Rooms/Commands/Notifications/Events/CommandLogs 5 页为 tab 视图（只读 + 仅 room_type 可改），删 5 个旧页面；Layout 导航 Matrix 只留 总览+控制台；MatrixDashboard 保留（只读观测）。② **[fe] 爬取队列可视化页**：新建 `CrawlQueue.tsx`（统计概览含 crawled 中间态 + 域名健康度表 HealthScore/Paused + 任务列表含重试），前端 `crawlQueueApi`（stats/jobs/domains/retry）；路由 `/crawl-queue` + Layout 导航。③ **[fe] 知识问答 SSE 流式**：后端 `AskService.AskStream`（模拟流式：Ask 拿完整 answer 按句切片推送 references/delta/done 事件）+ `KnowledgeHandler.AskStream` SSE 端点（`/api/files/ask/stream`，text/event-stream + flusher）；前端 KnowledgeAsk 改 fetch+ReadableStream 消费 SSE（打字机体验，逐字渲染）。`pnpm build`+`go build`+`go vet`+service/handler/router 测试全绿。**M5 前端收敛里程碑完成** |
| 2026-06-18 | PKB 自动闭环 + 知识骨架域管理/状态概览:① **server 内置 pkbScheduler 扩展为多任务调度**(系统 cron 无写权限,改 server 驱动)——curate 后自动 digest 全域(渲染骨架+新卡确定性归位+综述,内含 placeCardsOntoSkeleton)、fill/feed/propose 独立开关(`feature_pkb_auto_*` 默认关)+ 间隔(`pkb_*_interval_minutes` 默认1440)、串行锁防并发、注入 CrawlDomainProfileRepository 供 fill 冷却;闭环=爬取(已有)→打分落卡(已有)→digest 挂骨架→fill 补缺口→propose 长节点→feed 资讯库,无人值守。② **知识领域 CRUD**(`pkb.AddDomain/DeleteDomain/SetDomainDisplay` 外科式写回 domains.yaml 保注释):新建(display+scope,name/路径派生)/删除(仅删配置保留 vault 文件)/改名(仅 display 零迁移),兜底/资讯流域不可删。③ **域状态概览** `pkb.DomainStatsOverview`(卡片数/当天·近7天新增/缺口/已挂/待归位/低置信/最近digest/有无骨架) + `GET /api/pkb/domains/stats`。④ **生成骨架触发** `POST /api/pkb/domains/:name/skeleton`(后台异步 RunSkeleton)。⑤ 前端知识骨架页扩展(新建表单/改名/删除/生成骨架/状态行)。本地未推,待部署 keeper 验证(部署=push→keeper git pull→spool bundle up build) |
| 2026-06-18 | PKB 调方向前端 Phase I 落地(ADR-0004 Q9/Q12/Q15,窄掌舵面):复用现有 SolidJS SPA 加「知识骨架」页(`/knowledge/skeleton`),前端表单直接写、Matrix `!pkb`/CLI 保留兜底。①待批骨架提议批准/驳回走 REST(`GET /api/pkb/proposals`、`POST .../:id/approve|reject`,复用 `pkb.ListPendingProposals/ApplySkeletonProposal/RejectSkeletonProposal` 同一路径);②设领域大方向 scope 走 REST(`GET /api/pkb/domains`、`PUT /api/pkb/domains/:name/scope`,`pkb.SetDomainScope` 外科式逐行写回 domains.yaml 保注释、资讯流/兜底域拒设);浏览仍归 Obsidian、骨架机器独占写(W1)不变。③剪节点本期不做(后端零机制 + 触 W1,下一 slice 先定「删节点如何被记住不被 digest 重生」机制)。本地未推,待部署 keeper 自验 |
| 2026-06-17 | PKB 资讯库+晋升闸 Phase H 落地(ADR-0005 §5):`news` 升级资讯库容器(feed:true,digest/audit 领域遍历跳过、不进骨架/不计知识卡);`pkb-curate feed` 遍历 raw+archive 取当日资讯类(`pkb_type∈news/release`)→按 `pkb_domain` 分领域 promote_model 综述→落 `vault/资讯/<领域>/<日期>.md`(type:pkb_feed,不存独立卡);晋升闸 promote_model 识别耐久知识点→`shouldPromote` 把闸(非事件 && durability≥阈值)→复用 `fillOneGap` 走同一 V2 路径晋升为知识卡+归位,事件性不晋升;日报「今日资讯存档」弱联动;本地未推,待部署 keeper 自验 |
| 2026-06-17 | PKB 缺口填充 Phase G 落地(ADR-0004 §4):`pkb-curate fill <域>` 自顶向下补骨架缺口——gapfill_model 起草+提议源 → V2 真核实(G3 冷却让路 + Extract 抓取 + verify_model 判支撑) → 落 `source`/`verification`/`confidence` → 复用 Phase F 归位;F1 稳定缺口起草核实 / F2 易变缺口定向爬 reconstruct;新增 `POST /api/files/extract`;低置信卡进 audit;打样开 programming;本地未推,待部署 keeper 自验 |
| 2026-06-17 | PKB 知识骨架 Phase F 落地(ADR-0004/0005):骨架=结构真相源,`pkb-curate skeleton/match/propose` + digest 每轮确定性归位 + 待归位区 + 影响半径闸(小动作自动应用/大动作 Matrix `!pkb` 审批);本地未推,待部署 keeper 自验 |
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
| **1.0 GA 收口** | M1-M5 代码完成；待 ARCHITECTURE/EXCEPTIONS 最终核对 + 全量回归 + 部署验证 |
| **运维部署** | 🔶 M4-5 Loki+Promtail（spool bundle）；🔶 M6 Grafana+cAdvisor（spool bundle） |
| PKB | 存量 ~308 篇 raw 待批跑（数据运营，非代码）；Phase E audit 缺独立 API 端点 |
| LLM | cache 命中率监控；new-api 停服决策（调用方 base_url 迁移已随 M2 完成） |
| 提示词 | ✅ 1.0 已完成（response_format 透传 + 自修复重试 + golden set + 角色分离） |
| 爬虫 | 新源批量导入与 7 天成功率验收；周源健康报告 |
| 日志 | ✅ 1.0 已完成（pattern 告警 + 归档调度 + trace_id 全链路）；全文检索/SSE 交 Loki 外挂 |
| 前端 | ✅ 1.0 已完成（爬取队列页 + 问答 SSE）；🔶 Vault 预览增强（Markdown+wikilink）待 P2 |
| Matrix | ✅ 1.0 已完成（7→2 页重构） |
| 可靠性 | Watchdog 早期返回路径无日志;LLMJobQueue 心跳未写 activity_log;Tier 4 端到端验证未做 |

---

## 文档导航

- [doc/README.md](README.md) — 文档总览与目录结构
- [doc/ROADMAP.md](ROADMAP.md) — 演进规划（含 1.0 重构里程碑）
- [doc/MILESTONES-1.0.md](MILESTONES-1.0.md) — **1.0 重构里程碑详细记录**
- [doc/ARCHITECTURE.md](ARCHITECTURE.md) — 架构文档
- [doc/API.md](API.md) — REST API 参考
- [doc/LLM-GATEWAY-API.md](LLM-GATEWAY-API.md) — LLM 代理池 API 契约
- [doc/ARCHITECTURE-EXCEPTIONS.md](ARCHITECTURE-EXCEPTIONS.md) — 分层例外（LLM 两条已清零）
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
