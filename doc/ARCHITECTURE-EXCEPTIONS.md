# Architecture Exceptions

This file records intentional deviations from the default layering rule:

`Router -> Handler -> Service -> Repository -> Model`

The default remains strict: handlers should call services, services should call
repositories, and repositories should own persistence details. Exceptions must be
small, named, and have an exit plan.

## LLM Proxy Token Authentication

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
