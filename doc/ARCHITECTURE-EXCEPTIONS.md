# Architecture Exceptions

This file records intentional deviations from the default layering rule:

> **状态标注（1.0 重构）**：M2 已完成 LLM 代理池包重组 + 进程内直调 + 分层例外
> 消化。原两条 LLM 例外（①token 鉴权 ②管理 handler 持 repo）**已清零**：
> ① `router.Setup` 改传 `auth.LLMTokenStore` 接口，生产路径注入 `*llmgateway.LLMAdminService`；
> ② `LLMProxyHandler` 只依赖 `*LLMProxyService` + `*LLMAdminService`，token/pricing
> 管理 CRUD 下沉 `LLMAdminService`（`internal/llmgateway/admin.go`）。下方保留
> 历史记录以示退出计划已执行。

`Router -> Handler -> Service -> Repository -> Model`

The default remains strict: handlers should call services, services should call
repositories, and repositories should own persistence details. Exceptions must be
small, named, and have an exit plan.

## LLM Proxy Token Authentication

> **状态：✅ 已清零**（1.0 重构消化）。`router.Setup` 与 `registerLLMProxyRoutes`
> 形参改为 `auth.LLMTokenStore` 接口，`app.go` 注入 `*llmgateway.LLMAdminService`
> （实现该接口）。中间件不再依赖具体 repository，退出计划落地。

- Scope: `internal/router/router.go` passes `LLMTokenRepository` to `auth.LLMTokenAuth`.
- Reason: `/api/llm/v1/*` intentionally bypasses the normal `/api` middleware so
  external LLM tokens can authenticate without a web session. The auth middleware
  must resolve token hash, quota counters, and model-group scope before the request
  reaches the proxy handler.
- Guardrails: middleware depends on the `auth.LLMTokenStore` interface, not a
  concrete repository implementation.
- Exit plan: introduce a dedicated `TokenScopeService` owned by the service layer
  and have middleware depend on that interface instead of repository methods.

## LLM Proxy Management Handler

> **状态：✅ 已清零**（1.0 重构消化）。`LLMProxyHandler` 只依赖
> `*llmgateway.LLMProxyService` + `*llmgateway.LLMAdminService`。token/pricing/usage
> 管理 CRUD 与计费试算下沉 `LLMAdminService`（`internal/llmgateway/admin.go`），
> handler 不再持有任何 repository。退出计划落地。

- Scope: `internal/handler/handler.go` constructs `LLMProxyHandler` with token,
  usage, and pricing repositories for management endpoints.
- Reason: these endpoints predate the current service split and mostly expose CRUD
  over token/pricing administration.
- Guardrails: ordinary proxy routing, billing, channel routing, credentials, and
  alert logic remain in `LLMProxyService`.
- Exit plan: move token and pricing administration into `LLMAdminService`; then
  `LLMProxyHandler` should depend only on services.

## Agent Service Adapter Injection

- Scope: `internal/matrix/agent/adapters.go` defines interfaces that wrap multiple
  services (KnowledgeSearchService, DashboardService, LLMProxyService, etc.) for
  Agent tool execution. The `AgentService` is constructed with these adapters rather
  than the concrete service implementations.
- Reason: Agent tools need access to diverse service capabilities, but coupling to
  concrete services would make testing impossible and violate the agent package's
  independence. The adapter interfaces ensure agent code depends only on narrow
  function signatures, not full service APIs.
- Guardrails: each adapter interface exposes only the single method needed by its
  tool (e.g., `KnowledgeSearcher.Search(query string) ([]SearchResult, error)`).
  No repository is accessed directly; all data flows through service-layer adapters.
- Exit plan: if the tool set stabilizes, consider a `ToolContext` service that
  aggregates the needed capabilities, reducing the number of constructor parameters.

## DailyReport Service Direct HTTP Calls

- Scope: `internal/service/daily_report.go` makes direct HTTP calls to external
  services (Memos API for todo data) instead of going through a repository or
  dedicated service.
- Reason: the Memos todo data lives in an external service with its own API;
  there is no local repository for it. Creating a full MemosService/Repository
  pair for read-only todo access would be over-engineering for a single collector.
- Guardrails: the HTTP client uses `internal/pkg/httpclient` with proper timeout
  and error handling; the collector interface isolates external calls from the
  core DailyReportService logic.
- Exit plan: if Memos integration deepens (e.g., write operations from Agent),
  create a proper `MemosService` + `MemosRepository` and route all access through it.
