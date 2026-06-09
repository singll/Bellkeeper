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
