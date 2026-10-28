# LLM Gateway API

> Bellkeeper LLM 代理池的对外契约与进程内调用契约。旧 `doc/LLM_PROXY_GUIDE.md` 已归档至 `doc/archive/`。
> 实现包：`internal/llmgateway/`。架构见 [ARCHITECTURE.md](ARCHITECTURE.md)。

---

## 1. 定位

`internal/llmgateway/` 是 Bellkeeper 的 LLM 代理池一级基础设施，独立于业务 service 层：

- **多渠道路由 / 熔断 / 粘性 / 自适应限流学习 / 计费 / 余额 / Token 鉴权 / 协议转换**
- 对外暴露 OpenAI 兼容 API（外部/n8n 经 HTTP + Token）
- 对内提供进程内直调接口 `Gateway`（Bellkeeper 自身调用方经此，绕 HTTP+鉴权）

### 1.1 协议（protocol）与厂商（vendor）

运行时只支持三种协议，由渠道的 `provider_type` 决定：

| protocol | 含义 | 请求/响应处理 |
|----------|------|----------------|
| `openai` | OpenAI 兼容协议（默认） | 原样转发；仅做模型后缀解析、厂商 hook、usage 提取 |
| `anthropic` | Anthropic Messages 协议 | OpenAI ↔ Anthropic 请求/响应转换；流式 Anthropic SSE → OpenAI SSE |
| `rerank` | Cohere/Jina 风格 rerank 协议 | 仅路由到 `provider_type=rerank` 的渠道；请求/响应原样透传 |

除上述三种协议外的转换器已移除，`converter/` 仅保留 Anthropic ↔ OpenAI 转换。

`vendor` 与 `protocol` 是两个独立维度：

- **protocol** 只决定请求/响应 wire format 与鉴权头（如 `Authorization: Bearer` / `x-api-key`）。
- **vendor** 决定厂商错误语义、余额口径与特殊 hook（如 DeepSeek 官方剥离 reasoning、OpenCode Go 注入 `x-opencode-session`）。
- `vendor` 为空时按 `base_url` 推断（`InferVendor`）；显式配置的 `vendor` 优先。
- 错误分类只使用 `vendor`，绝不把 `provider_type` 当厂商使用。例如 Anthropic 协议渠道若实际是 Kimi Code，错误语义仍按 `vendor=kimi-code` 处理。

---

## 2. 对外 HTTP API

### 2.1 代理入口

| 端点 | 方法 | 用途 | 鉴权 |
|------|------|------|------|
| `/api/llm/v1/*path` | Any | OpenAI 兼容代理（含流式 `/v1/chat/completions`、`/v1/rerank`） | `sk-bk-*` Bearer（`auth/llm_token.go` 的 `LLMTokenAuth`）+ 服务端 `api_key` 兜底 |

路由注册见 `internal/router/router.go:registerLLMProxyRoutes`，handler 见 `internal/handler/llm_proxy.go`，业务核心为 `LLMProxyService.ProxyRequest` / `ProxyStreamRequest`。

- 代理入口注册在 engine 上而非 `/api` group，以绕过 Web 会话鉴权，仅接受 LLM Token / 服务端 API Key。
- 响应透传上游状态码与 `Content-Type`；管理面统一使用 `internal/pkg/response` 包装。
- 缓冲与流式请求都走同一套渠道选择、熔断、限流、计费与错误分类路径。

### 2.2 管理端点（前缀 `/api/llm`）

| 方法 | 路径 | 用途 |
|------|------|------|
| GET | `/channels/status` | 渠道运行时状态（桶、健康、余额窗口） |
| GET | `/health` | 渠道健康/熔断状态 |
| GET | `/groups/status` | 虚拟模型组状态（成员、策略、粘性、成员级熔断） |
| GET | `/stats` | 代理统计 |
| GET | `/logs` | 最近代理日志 |
| GET | `/rate-limit-events` | 限流事件 |
| GET | `/alerts` | 聚合告警事件 |
| POST | `/channels/:name/reset` | 重置渠道熔断器 |
| DELETE | `/groups/:name/sticky` | 清除模型组粘性绑定 |
| GET/POST | `/config/channels` | 渠道列表 / 创建 |
| PUT/DELETE | `/config/channels/:id` | 渠道更新 / 删除 |
| GET/POST | `/config/channels/:id/credentials` | 渠道凭证列表 / 创建（加密存储，仅返回掩码） |
| PUT/DELETE | `/config/credentials/:id` | 凭证更新 / 删除 |
| GET/POST | `/config/groups` | 模型组列表 / 创建 |
| PUT/DELETE | `/config/groups/:id` | 模型组更新 / 删除 |
| POST | `/reload` | 从 DB 重新加载渠道/模型组/凭证（保留健康与限流桶状态） |
| GET | `/channels/:name/balance` | 最近一次余额快照 |
| GET | `/channels/:name/balance/history` | 余额历史快照 |
| GET | `/balances` | 全部渠道余额快照 |
| POST | `/balances/refresh` | 立即刷新余额 |
| GET/POST | `/tokens`；PUT/DELETE `/tokens/:id` | Token 管理 |
| POST | `/tokens/:id/regenerate` | 重新生成 Token Key |
| GET | `/tokens/:id/usage` | Token 用量 |
| GET/POST | `/pricing`；PUT/DELETE `/pricing/:id` | 定价管理 |
| POST | `/pricing/test-calc` | 计费试算 |
| GET/DELETE | `/conversations[/:id]` | 会话粘性绑定 |
| GET | `/usage` | 用量聚合（`group_by=token\|channel\|model\|date`） |
| GET | `/rate-limits` | 自适应限流学习状态 |
| POST | `/rate-limits/:id/:model/reset` | 重置渠道×模型限流 |
| POST | `/rate-limits/:id/lock` | 锁定/解锁限流记录 |
| GET/POST | `/coding-strategy` | Coding 路由策略（`free_first` / `quality_first` / `complexity_aware`） |
| POST | `/api/config/reload/llm-proxy` | 兼容入口，效果同 `/reload` |

### 2.3 渠道配置字段

`GET /api/llm/config/channels` 返回 `ChannelConfigView`（`model.LLMChannel` + 密钥状态投影）：

| 字段 | 说明 |
|------|------|
| `provider_type` | 协议：`openai` / `anthropic` / `rerank` |
| `vendor` | 厂商标识；空则按 `base_url` 推断 |
| `base_url` | 上游根地址，不得以 `/v1` 结尾 |
| `rpm` / `rpd` | 客户端限速 |
| `quota_window_seconds` | `0` = 自然日重置；`>0` = 滚动窗口（如 18000 = 5h） |
| `models` | JSON 数组字符串，登记上游真实模型名 |
| `task_types` | JSON 数组字符串；空 = 所有任务可用 |
| `tier` | `free` / `standard` / `premium`；空则由 `is_free` 推导 |
| `balance_provider_type` | `deepseek` / `moonshot` / `newapi` / `aliyun` / `opencodego`；空 = 不监控 |
| `balance_config_json` | 余额 provider 的非密参数 |
| `model_rpm_overrides` | JSON：模型 → RPM |
| `api_key_status` | `configured` / `missing` / `direct` |
| `api_key_preview` | 掩码预览（如 `sk-o...Lvr`），永不下发完整密钥 |

凭证按 `purpose` 区分：`api` 供聊天/补全，`balance` 供余额监控；`balance` 缺失时回退 `api`。凭证 API 只返回 `credential_preview`，不返回密文或明文。

### 2.4 模型组字段

| 字段 | 说明 |
|------|------|
| `strategy` | `priority-health` / `best-weight` / `least_latency` / `balance_aware` |
| `sticky_ttl_seconds` | 任务级粘性 TTL |
| `members[]` | `channel_name`、`model`、`weight`、`max_context_tokens`、`max_output_tokens` |

- `members[].model` 必须已登记在对应渠道的 `models` 列表中，否则配置保存失败（模型目录以渠道登记为唯一事实源）。
- `max_context_tokens`：成员上下文窗口，`0` = 未知；请求预算超出时该成员被预过滤。
- `max_output_tokens`：成员最大输出，`0` = 未知；请求的 `max_tokens` / `max_completion_tokens` 超出时自动封顶。

### 2.5 模型目录与连接测试（现状）

当前**没有独立的模型目录 HTTP 端点**，也没有面向管理面的“连接测试”端点：

- 模型目录由 `GET /api/llm/config/channels` 的 `models` 字段承担；新增模型必须先保存到渠道，再在模型组/定价中选择。
- 连接验证目前只由后台恢复探针承担：`probeChannel` / `probeMemberQuotaExhausted` 使用 `UpstreamAdapter.ProbeRequest` 生成 1-token/rerank 探针，经 `tryChannel(..., tokenID=0)` 发出。探针不写 Token 用量、不经过计费归集，成功/失败只用于恢复熔断状态，不向调用方回显任何密钥。
- 若后续新增管理面连接测试 API，必须保持相同约束：不发真实业务请求计费、不写入 Token 用量、不把测试失败计入渠道健康/熔断（或仅按显式策略处理）、响应中不得回显密钥。

### 2.6 新增模型向导与表单联动

当前前端以“渠道 → 凭证 → 模型组 → 定价”的顺序引导新增模型，表单之间共享渠道登记清单：

1. **渠道页**：选择协议（`openai` / `anthropic` / `rerank`），填写 `vendor`、`base_url`、`models`（上游真实模型名 JSON 数组）、`quota_window_seconds` 等；保存后自动热重载。
2. **凭证**：在渠道卡片打开凭证管理，添加 `purpose=api`（聊天/补全）密钥；余额监控再添加 `purpose=balance`（缺省回退 `api`）。
3. **模型组与路由**：新增/编辑模型组时，成员渠道与模型均为下拉选择，模型选项来自该渠道已登记清单；可设置权重、`max_context_tokens`、`max_output_tokens`。
4. **用量与计费**：新增定价时同样从渠道/模型下拉选择，未配置定价的模型按 0 成本记账。

模型组保存时后端会校验成员模型确实存在于渠道清单，避免“向导里能选、运行时路由不到”的漂移。

---

## 3. 进程内直调接口

### 3.1 `Gateway` 接口

定义于 `internal/llmgateway/gateway.go`：

```go
type Gateway interface {
    Chat(ctx context.Context, req llmclient.ChatRequest, opts llmclient.ChatOptions) (*llmclient.ChatResponse, error)
    ChatStream(ctx context.Context, req llmclient.ChatRequest, opts llmclient.ChatOptions) (*llmclient.ChatStreamResult, error)
    Rerank(ctx context.Context, req llmclient.RerankRequest) (*llmclient.RerankResponse, error)
}
```

- **类型契约**：复用 `internal/llmclient` 的 `ChatRequest` / `ChatResponse` / `ChatOptions` / `ChatStreamResult` / `RerankRequest` / `RerankResponse`。
- **实现**：`*LLMProxyService`（编译期 `var _ Gateway = (*LLMProxyService)(nil)` 守卫）。
- **行为**：内部构造 OpenAI `/v1/chat/completions` 或 `/v1/rerank` 请求，调用 `ProxyRequest` / `ProxyStreamRequest`；**绕过 `LLMTokenAuth`（进程内可信）**，但保留路由 / 熔断 / 限流 / 粘性 / 计费。
- **计费归集**：`callerID="internal"`，`tokenID=0`（无外部 Token，按 callerID 归集）。
- **透传**：`opts.CallerID` / `TaskType` / `ConversationID` → `X-Caller-ID` / `X-Task-Type` / `X-Conversation-ID` header。
- **流式**：`ChatStream` 后台消费 SSE，`FinalizeStream` 负责在流结束后落库用量与计费。

### 3.2 调用方注入点

`app.go` 注入 `*LLMProxyService`（实现 `Gateway`）给下表进程内调用方，替代原 `llmclient.New(localhost HTTP)` 构造：

| 调用方 | 文件 | 注入方式 |
|--------|------|----------|
| Matrix Agent | `internal/matrix/agent/agent.go` | `NewAgentService(..., llmGateway, ...)` |
| Classify | `internal/service/classify.go` | `NewClassifyService(cfg, llmGateway, activityLog)` |
| DailyReport | `internal/service/daily_report.go` | `NewDailyReportService(..., llmGateway, llmJobs)` |
| KnowledgeAsk | `internal/service/knowledge_ask.go` | `NewAskService(search, llmGateway)` |
| RuleOptimizer | `internal/service/rule_optimizer.go` | `NewRuleOptimizerService(..., llmGateway, ...)` |
| LLMJobQueue | `internal/llmgateway/llm_job_queue.go` | `NewLLMJobQueueService(cfg, repo, llmGateway)` |

### 3.3 进程外 CLI（保留 `llmclient`）

`cmd/bellkeeper` 的 `pkb-curate` 等子命令是独立进程，无 `*LLMProxyService` 实例，仍经 localhost HTTP + Token 调代理池。`llmclient.Client` 实现 `Gateway` 接口，可作为 `Gateway` 注入 `LLMJobQueueService`：

```go
llmclient.New(llmclient.Options{BaseURL: cfg.Classify.LLMProxyURL, ...})
// 作为 llmgateway.Gateway 传入
```

`internal/pkb/client.go` 同理保留 `llmclient`（CLI 进程外性质）。

---

## 4. 包结构

```
internal/llmgateway/
├── gateway.go              // Gateway 接口（Chat/ChatStream/Rerank）+ 进程内直调
├── llm_service.go          // LLMProxyService 生命周期、Options/Repos 构造、余额/告警装配
├── llm_runtime.go          // DB → 内存快照、Reload、凭证解析、ChannelConfigView
├── llm_proxy_request.go    // 缓冲代理、模型组/直连路由、4xx failover、失败归因
├── llm_stream.go           // 流式代理、流式 failover、FinalizeStream
├── llm_upstream.go         // Channel、prepareUpstreamRequest、tryChannel/tryChannelStream、退避
├── llm_adapter.go          // UpstreamAdapter：openai/anthropic/rerank 适配器 + Anthropic SSE + 厂商 hook
├── llm_usage.go            // usage 提取、流式 usage tracker、异步入库、token 估算
├── llm_background.go       // 余额快照、quota 恢复探针、配额告警、日志归档、会话清理
├── llm_bucket.go           // 令牌桶 + 自然日/滚动窗口计数
├── llm_management.go       // 管理面只读状态、统计、会话、限流学习、coding strategy
├── llm_config_service.go   // 渠道/模型组/凭证写入（LLMConfigService），写后 Reload
├── llm_validation.go       // 渠道/模型组校验、Vendor 推断、ValidationError
├── llm_model_group.go      // 虚拟模型组、粘性、优先级/权重/延迟/余额策略、成员熔断
├── llm_task_router.go      // 任务感知路由（coding/classify/summary/...）
├── llm_rate_limit_learner.go // 429 自适应限流学习
├── llm_channel_health.go   // 滑动窗口健康度 + 熔断状态机
├── llm_alert.go            // 告警聚合（AlertNotifier 接口）
├── llm_pricing.go          // 定价与成本计算
├── llm_credential.go       // 凭证解析与掩码视图
├── llm_conversation.go     // 会话粘性
├── llm_job_queue.go        // LLM 异步任务队列
├── llm_job_util.go         // IdempotencyKey 工具
├── converter/
│   ├── anthropic.go        // OpenAI ↔ Anthropic 转换 + AnthropicSSEConverter
│   └── anthropic_test.go
├── errors/                 // 错误语义分类（vendor 驱动）
│   └── classifier.go
└── balance/                // 真实余额 provider（Kind: usd/fraction/quota）
```

`LLMProxyService` 不再承载渠道/模型组/凭证写操作；配置写入统一由 `LLMConfigService` 完成，写后调用 `Reload()`。运行时只负责读快照、路由、熔断、计费与后台任务。

---

## 5. UpstreamAdapter 与请求生命周期

### 5.1 接口

```go
type UpstreamAdapter interface {
    Protocol() string
    PrepareRequest(body []byte, path, model string, ch *Channel) (PreparedRequest, error)
    ApplyAuth(req *http.Request, ch *Channel, prep PreparedRequest)
    DecodeBuffered(respBody []byte, statusCode int, model string) ([]byte, Usage, error)
    WrapStream(rc io.ReadCloser) io.ReadCloser
    ErrorClass(statusCode int, body []byte, ch *Channel) llmerrors.Result
    ProbeRequest(model string) (body []byte, path string)
}
```

`adapterFor(ch)` 按 `Config.ProviderType` 返回 `openAIAdapter` / `anthropicAdapter` / `rerankAdapter`；未知协议直接返回错误，不做静默降级。

### 5.2 统一路径

缓冲与流式都经过 `prepareUpstreamRequest`：

1. `adapter.PrepareRequest` 做协议转换、模型后缀解析、vendor hook。
2. 拼接 `base_url + path`，复制调用方 header，删除 `X-API-Key` 与 `Accept-Encoding`。
3. `adapter.ApplyAuth` 注入上游鉴权头（OpenAI/Rerank `Authorization: Bearer`；Anthropic `x-api-key` + `anthropic-version`）。
4. `tryChannel` / `tryChannelStream` 执行令牌桶、熔断半开计数、上游请求、响应解码/流包装与日志。

因此协议转换、鉴权、厂商 hook、错误分类与探测形状只存在一份实现，不会出现“缓冲路径支持、流式路径遗漏”的能力漂移。

### 5.3 响应与流

- `DecodeBuffered`：OpenAI/Rerank 原样返回并提取 usage；Anthropic 先读原始 usage（保留 cache token 计费），再转换为 OpenAI 响应；5xx 不做转换，直接透传上游错误。
- `WrapStream`：Anthropic 包装为 `anthropicSSEReader`，把 Anthropic SSE 事件转换为 OpenAI SSE；OpenAI/Rerank 原样透传。
- usage tracker 观察上游原始流，再交给适配器转换，确保 Anthropic 流式 usage 不因转换丢失。

---

## 6. 路由、故障转移与错误

### 6.1 路由顺序

1. `/v1/rerank` 直接走 `proxyRerank`，只选 `provider_type=rerank` 渠道，跳过模型组、粘性与 chat 转换。
2. 会话粘性绑定有效时直连绑定渠道；失败则清除绑定并进入正常路由。
3. 模型名命中虚拟模型组时走 `proxyViaGroup` / `proxyStreamViaGroup`。
4. 否则按真实模型名匹配渠道列表（精确 → 忽略大小写 → 子串）。

### 6.2 failover 规则

- 缓冲与流式行为一致：只有 2xx 才视为成功；**4xx 也会继续尝试下一个成员/渠道**，避免单渠道模型不支持、单模型配额、协议差异直接中断调用方任务。
- 所有成员/渠道都失败时，**透传最后一次真实上游响应**（状态码/响应体/响应头）或最后一个传输错误，不合成 503 掩盖真实原因。
- 流式请求一旦开始返回 2xx 流，不再重试；非 2xx 在流开始前按 failover 处理。
- 模型组按 `(channel, model)` 去重并逐成员排除，避免重复成员和无限重试。

### 6.3 错误分类与健康影响

错误分类入口为 `classifyChannelError` → `adapter.ErrorClass` → `llmerrors.Classify(status, body, vendor)`：

| 类别 | 典型来源 | 健康影响 |
|------|----------|----------|
| `quota_exhausted` | 429/403 额度耗尽 | 渠道或成员级熔断；按 `resets in` 动态时长，默认 5h |
| `balance_zero` | 401/403 余额耗尽 | 长熔断，触发充值告警 |
| `auth_failed` | 401/403 鉴权失败 | 永久熔断，critical 告警 |
| `session_expired` | new-api 401 | 长熔断 |
| `subscription_invalid` | 402 | 1h 成员/渠道熔断 |
| `rate_limited_retry` | 429 瞬时限流 | 短退避，不计长熔断 |
| `server_error` | 5xx | 短熔断 |
| `context_too_long` | 400 上下文超限 | **不计健康**，仅跳过/换成员 |
| `max_output_exceeded` | 400 max_tokens 超限 | **不计健康**，封顶后换成员 |

成员级作用域：`quota_exhausted` / `balance_zero` / `subscription_invalid` 只冷却对应 `(channel, model)`，不牵连同渠道其他模型。`context_too_long` / `max_output_exceeded` 是请求与模型的能力匹配问题，路由通过 `max_context_tokens` / `max_output_tokens` 预过滤与封顶处理，绝不写渠道健康。

### 6.4 余额与 `balance_aware`

`balance.Info.Kind` 标记余额单位：

- `usd`：货币金额，`Currency` 为 USD/CNY 等。
- `fraction`：`[0,1]` 剩余比例（如 OpenCode Go 滚动窗口）。
- `quota`：厂商自定义绝对配额单位。

`balance_aware` 策略先按 `Kind` 分组，再以同组最大剩余量归一化到 `[0,1]`，避免把美元余额与窗口比例直接比较；无余额数据时退化为 `priority-health`。评分公式：`latency_ewma × (1 + error_rate) / normalized_remaining`，越低越优。

### 6.5 熔断与恢复探测

- 渠道级：滑动窗口 + 连续失败阈值 + 半开并发上限（`circuit_breaker` 配置）。
- 成员级：`(channel, model)` 独立冷却，适用于同渠道独立积分池。
- 后台 `kimiCodeProbeLoop` 按 `circuit_breaker.probe_interval_minutes`（下限 10min）对已过期 `quota_exhausted` 渠道/成员发 1-token 探针；成功则恢复，失败则按新分类重新计时。

---

## 7. 配置与数据迁移

### 7.1 `LLMProxyConfig`

`config.LLMProxyConfig` 是代理池主配置：

| 字段 | 说明 |
|------|------|
| `default_timeout` | 上游超时（秒） |
| `max_retries` | 单渠道重试次数（429/桶等待） |
| `max_wait_seconds` | 令牌桶最大等待 |
| `backoff_cap_seconds` / `backoff_jitter` | 指数退避上限/抖动 |
| `default_bucket_rpm` | 无 RPM 配置时的兜底 |
| `balance_sync_interval` | 余额同步间隔（秒），0 = 关闭 |
| `log_retention_days` | 代理日志保留天数，0 = 默认 30 |
| `circuit_breaker` | 熔断与探测参数 |
| `coding_routing_strategy` | `free_first` / `quality_first` / `complexity_aware` |
| `complexity` | 复杂度阈值与关键词 |
| `channels` / `model_groups` | 首次 seed 数据（DB 为运行时事实源） |

**`llm_proxy.enabled` 已移除**：LLM 网关是核心基础设施（且承担进程内调用），始终启用，不再提供启动/路由开关。

### 7.2 依赖构造

```go
svc := llmgateway.NewLLMProxyService(llmgateway.Options{
    Config: cfg.LLMProxy,
    Repos: llmgateway.Repos{
        Proxy:               repos.LLMProxy,
        Channel:             repos.LLMChannel,
        Group:               repos.LLMModelGroup,
        TokenUsage:          repos.LLMTokenUsage,
        RateLimit:           repos.LLMRateLimit,
        ConversationBinding: repos.LLMConversationBinding,
        Token:               repos.LLMToken,
        Credential:          repos.LLMChannelCredential,
        BalanceSnapshot:     repos.LLMChannelBalanceSnapshot,
    },
    Pricer:      pricer,
    SettingRepo: repos.Setting,
})
```

配置写入：

```go
configSvc := llmgateway.NewLLMConfigService(
    repos.LLMChannel, repos.LLMModelGroup, repos.LLMChannelCredential, svc,
)
```

`LLMProxyHandler` 依赖运行时 `*LLMProxyService`、管理面 `*LLMAdminService` 与配置写入 `*LLMConfigService`。

### 7.3 migration 009

`migrations/009_llm_runtime_columns.up.sql` 增加运行时列（`IF NOT EXISTS`，兼容已由 GORM AutoMigrate 建列的库）：

- `llm_channels.vendor`：厂商标识
- `llm_channels.quota_window_seconds`：滚动额度窗口
- `llm_model_group_members.max_context_tokens`：成员上下文窗口
- `llm_model_group_members.max_output_tokens`：成员最大输出

down 迁移按相反顺序删除上述列。

### 7.4 其它配置

- `config.LLMJobQueueConfig` — 异步队列配置
- `config.ClassifyConfig.LLMProxyURL` — 进程外 CLI 侧的代理池 URL
- `config.Server.APIKey` — 服务端 api_key（外部 LLM Token 鉴权兜底）

---

## 8. 分层与例外

- `llmgateway` 包不依赖 `service` 包（循环破除）。
- `service/llm_alert_notifier.go`（`NewMatrixAlertNotifier`）留 service 包，依赖 `NotificationService`，实现 `llmgateway.AlertNotifier` 接口——`llmgateway` 经接口依赖通知能力，不反向 import service。
- 历史分层例外（`router.go` 直传 tokenRepo、`handler.go` 持有 repo）已清零；管理面写入进一步下沉 `LLMConfigService`。见 [ARCHITECTURE-EXCEPTIONS.md](ARCHITECTURE-EXCEPTIONS.md)。
