# LLM Gateway API

> Bellkeeper 1.0 重构后 LLM 代理池的对外契约。旧 `doc/LLM_PROXY_GUIDE.md` 已归档至 `doc/archive/`。
> 实现包：`internal/llmgateway/`。架构见 [ARCHITECTURE.md](ARCHITECTURE.md)。

---

## 1. 定位

`internal/llmgateway/` 是 Bellkeeper 的 LLM 代理池一级基础设施，独立于业务 service 层：

- **多 provider 路由 / 熔断 / 粘性 / 自适应限流学习 / 计费 / 余额 / Token 鉴权 / 协议转换（Gemini + Anthropic → OpenAI 兼容）**
- 对外暴露 OpenAI 兼容 API（外部/n8n 经 HTTP + Token）
- 对内提供进程内直调接口 `Gateway`（Bellkeeper 自身调用方经此，绕 HTTP+鉴权）

## 2. 对外 HTTP API（契约不变）

| 端点 | 方法 | 用途 | 鉴权 |
|------|------|------|------|
| `/api/llm/v1/*path` | Any | OpenAI 兼容代理（含流式 `/v1/chat/completions`、`/v1/rerank`） | `sk-bk-*` Bearer（`auth/llm_token.go` 的 `LLMTokenAuth`）+ 服务端 `api_key` 兜底 |
| `/api/llm/channels/status` 等 | GET | 管理端点 | Authelia + API Key |

路由注册见 `internal/router/router.go:registerLLMProxyRoutes`，handler 见 `internal/handler/llm_proxy.go`，业务核心 `LLMProxyService.ProxyRequest` / `ProxyStreamRequest`。

## 3. 进程内直调接口（1.0 新增）

### 3.1 `Gateway` 接口

定义于 `internal/llmgateway/gateway.go`：

```go
type Gateway interface {
    Chat(ctx context.Context, req llmclient.ChatRequest, opts llmclient.ChatOptions) (*llmclient.ChatResponse, error)
}
```

- **类型契约**：复用 `internal/llmclient` 的 `ChatRequest` / `ChatResponse` / `ChatOptions`，与进程外 `llmclient.Client.ChatCompletionFull` 完全一致。
- **实现**：`*LLMProxyService.Chat`（编译期 `var _ Gateway = (*LLMProxyService)(nil)` 守卫）。
- **行为**：内部构造 OpenAI `/v1/chat/completions` headers/body，调 `ProxyRequest`；**绕过 `LLMTokenAuth`（进程内可信）**，但保留路由 / 熔断 / 限流 / 粘性 / 计费（`ProxyRequest` 内置）。
- **计费归集**：`callerID="internal"`，`tokenID=0`（无外部 Token，按 callerID 归集）。
- **透传**：`opts.CallerID`/`TaskType`/`ConversationID` → `X-Caller-ID`/`X-Task-Type`/`X-Conversation-ID` header。

### 3.2 调用方注入点

`app.go` 注入 `*LLMProxyService`（实现 `Gateway`）给下表 6 处进程内调用方，替代原 `llmclient.New(localhost HTTP)` 构造：

| 调用方 | 文件 | 注入方式 |
|--------|------|----------|
| Matrix Agent | `internal/matrix/agent/agent.go` | `NewAgentService(..., llmGateway, ...)` |
| Classify | `internal/service/classify.go` | `NewClassifyService(cfg, llmGateway, activityLog)` |
| DailyReport | `internal/service/daily_report.go` | `NewDailyReportService(..., llmGateway, llmJobs)` |
| KnowledgeAsk | `internal/service/knowledge_ask.go` | `NewAskService(search, llmGateway)` |
| RuleOptimizer | `internal/service/rule_optimizer.go` | `NewRuleOptimizerService(..., llmGateway, ...)` |
| LLMJobQueue | `internal/llmgateway/llm_job_queue.go` | `NewLLMJobQueueService(cfg, repo, llmGateway)` |

### 3.3 进程外 CLI（保留 `llmclient`）

`cmd/bellkeeper` 的 `pkb-curate` 等子命令是独立进程，无 `*LLMProxyService` 实例，仍经 localhost HTTP + Token 调代理池。为此 `llmclient.Client` 实现 `Gateway` 接口（`Chat = ChatCompletionFull`），使其可作为 `Gateway` 注入 `LLMJobQueueService`：

```go
llmclient.New(llmclient.Options{BaseURL: cfg.Classify.LLMProxyURL, ...})
// 作为 llmgateway.Gateway 传入
```

`internal/pkb/client.go` 同理保留 `llmclient`（CLI 进程外性质）。

## 4. 包结构

```
internal/llmgateway/
├── gateway.go              // Gateway 接口 + *LLMProxyService.Chat 进程内直调
├── llm_proxy.go             // LLMProxyService 核心（路由/熔断/限流/计费/ProxyRequest）
├── llm_task_router.go       // 任务感知路由
├── llm_rate_limit_learner.go// 自适应限流学习器
├── llm_channel_health.go    // 通道健康度
├── llm_alert.go             // 告警聚合（AlertNotifier 接口）
├── llm_pricing.go           // 计费（Pricer / Usage）
├── llm_credential.go        // 凭证管理
├── llm_conversation.go      // 会话粘性
├── llm_job_queue.go         // LLM 异步任务队列（DB 状态机 + Gateway 调用）
├── llm_model_group.go        // 虚拟模型组
├── llm_job_util.go          // IdempotencyKey 工具
├── converter/               // Gemini + Anthropic ←→ OpenAI 协议转换
│   ├── gemini.go
│   ├── anthropic.go         // 含 AnthropicSSEConverter
│   └── *_test.go            // 30+ 用例守护契约
├── errors/                  // 错误分类（QuotaExhausted 等）
└── balance/                 // provider 余额查询
```

## 5. 分层与例外

- `llmgateway` 包不依赖 `service` 包（循环破除）。
- `service/llm_alert_notifier.go`（`NewMatrixAlertNotifier`）留 service 包，依赖 `NotificationService`，实现 `llmgateway.AlertNotifier` 接口——`llmgateway` 经接口依赖通知能力，不反向 import service。
- 历史分层例外（`router.go` 直传 tokenRepo、`handler.go` 持有 repo）见 [ARCHITECTURE-EXCEPTIONS.md](ARCHITECTURE-EXCEPTIONS.md)，已清零（1.0 重构完成）。

## 6. 配置

- `config.LLMProxyConfig` — 代理池主配置
- `config.LLMJobQueueConfig` — 异步队列配置
- `config.ClassifyConfig.LLMProxyURL` — 进程外 CLI 侧的代理池 URL
- `config.Server.APIKey` — 服务端 api_key（外部 LLM Token 鉴权兜底）