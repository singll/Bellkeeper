# Bellkeeper 架构审查报告

审查日期：2026-06-08

## 1. 审查范围与方法

本次审查基于当前仓库代码、配置、迁移、前端 API 调用和已有开发文档，未接入生产日志或真实数据库数据。重点覆盖：

- 后端分层、模块边界、后台任务、并发安全、配置与安全。
- LLM Proxy、个人知识库/PKB、Matrix 控制平面、LogCenter、CrawlQueue 等核心模块。
- 前端 API 契约、页面可用性、类型一致性与工具链约束。
- 现有代码风格、测试质量、迁移治理，以及后续给人和大模型共同遵守的统一开发规范。

已执行验证命令：

| 命令 | 结果 |
| --- | --- |
| `go test ./...` | 通过 |
| `go build ./cmd/bellkeeper` | 通过 |
| `go vet ./internal/...` | 通过 |
| `cd web && pnpm build` | 通过 |

结论需要和上述验证一起理解：项目当前可以构建和测试通过，但仍存在多处运行期、权限、并发、前后端契约和治理层面的风险。

## 2. 总体结论

Bellkeeper 的主干架构已经具备可持续演进的基础：整体分层基本清楚，`Router -> Handler -> Service -> Repository -> Model` 的方向在大多数模块中成立；`internal/app/app.go` 使用手动依赖注入集中组装服务；LLM Proxy、PKB/知识库、CrawlQueue、Matrix、LogCenter 都不是空壳，已经有真实链路。

但当前项目的风险不在“功能是否存在”，而在“功能是否能长期稳定、安全、一致地演进”。最需要优先处理的是：LLM Token 权限字段未完全生效、文件/命令入口存在注入与路径逃逸风险、CrawlQueue 抢占任务不是原子操作、Matrix 通知链路有 nil client 与 data race 风险、LogCenter admin 校验在空 API key 时会放行、前端 Matrix/LLM API 契约与后端响应包装不一致。

架构成熟度建议判断为：功能完整度中高，工程约束成熟度中等偏低，生产安全性需要先完成 P0 修复后再提高暴露面。

## 3. P0 发现：应优先修复

### P0-1 LLM Token 的 `allowed_groups` 未实际生效

证据：

- 模型定义了 `AllowedGroups` 和读写方法：`internal/model/llm_token.go:19-20`、`internal/model/llm_token.go:53-60`。
- 创建/更新 Token 接口允许提交 `allowed_groups`：`internal/handler/llm_proxy.go:539-548`、`internal/handler/llm_proxy.go:602-627`。
- 鉴权中只检查 `AllowedModels`：`internal/auth/llm_token.go:114-123`。
- `rg AllowedGroups internal` 显示除 model/handler 外没有服务层强制校验。
- LLM Proxy 外部代理路由绕过普通 `/api` 认证，依赖 `LLMTokenAuth`：`internal/router/router.go:133-139`。

影响：

如果调用方拿到一个 LLM Token，只要 `allowed_models` 没有精确限制，就可能访问本应由 `allowed_groups` 限制的虚拟模型组或通道组合。这个字段会给管理员造成“已经限制了组”的错觉，是权限缺口。

建议：

- 在 `LLMTokenAuth` 或 LLM Proxy 请求入口建立统一的 token scope 对象，同时校验模型、模型组、caller、配额。
- 对虚拟模型组请求，先解析请求 model 是否命中 group，再检查 `allowed_groups`。
- 增加 Token 权限测试：允许模型、拒绝模型、允许 group、拒绝 group、空 allowlist 表示全部允许。

### P0-2 文件路径与系统命令入口存在安全风险

证据：

- `KnowledgeFilesService` 使用 `strings.HasPrefix(absFull, absBase)` 判断路径是否在 base 内：`internal/service/knowledge_files.go:70-75`、`internal/service/knowledge_files.go:161-166`、`internal/service/knowledge_files.go:219-224`。例如 `/mnt/knowledge2` 会被误判为 `/mnt/knowledge` 内部路径。
- `FileIngestionService` 直接把 `req.Layer` 拼入路径：`internal/service/file_ingestion.go:163-174`，没有限制 layer 枚举，也没有再次校验最终路径。
- 系统接口拼接 shell 命令：`internal/handler/system.go:136` 使用 `docker restart ` + name，`internal/handler/system.go:158` 使用 `./spool.sh backup ` + host。
- 默认配置文件中 `server.mode: noauth`：`config/bellkeeper.yaml:1-5`。

影响：

路径逃逸可能造成知识库目录外读写；命令拼接可能造成命令注入。即使这些接口理论上在内网或管理界面下使用，也不应依赖网络边界作为唯一防线。

建议：

- 路径安全统一使用 `filepath.Rel(absBase, absFull)`，拒绝 `..`、绝对路径、跨 volume 或 symlink 逃逸。
- `Layer` 改为配置内允许值枚举，例如 `raw/archive/vault`，不要直接接受任意路径片段。
- shell 调用改为 `exec.Command("docker", "restart", name)`、`exec.Command("./spool.sh", "backup", host)`，并对 `name`、`host` 做白名单校验。
- `noauth` 只能用于本地开发配置，生产配置启动时应拒绝空 `server.api_key` 和不安全模式。

### P0-3 CrawlQueue `Dequeue` 不是原子抢占

证据：

- `internal/repository/crawl_job.go:55-89` 注释写了 `SELECT FOR UPDATE SKIP LOCKED`，但没有放在 `db.Transaction` 中。
- `First(&job)` 后锁会随语句结束释放，随后再 `Updates(...)`：`internal/repository/crawl_job.go:68-85`。

影响：

多个 worker 并发时可能抢到同一个 crawl job，导致重复抓取、重复写入、状态覆盖。这个问题在单 worker 或低并发测试中不容易出现。

建议：

- 参考 `LLMJobRepository` 的事务式 claim 模式，把 select 和 update 放进同一个 transaction。
- update 时增加条件：`WHERE id = ? AND status IN (...)`，并检查 `RowsAffected == 1`。
- 增加并发测试，启动多个 goroutine 同时 dequeue，同一 job 只能被 claim 一次。

### P0-4 Matrix 通知链路有 nil client 和 data race 风险

证据：

- `setupMatrixInfrastructure` 中先用 nil client 创建 sender 并启动 worker：`internal/app/app.go:212-216`。
- Matrix gateway 成功后才注入 client：`internal/app/app.go:264-267`。
- `NotificationSender.Send` 直接调用 `s.client.SendHTMLMessage`，没有 nil guard：`internal/service/notification_sender.go:56-58`。
- `NotificationSender.UpdateClient` 与 `Send` 对 `client` 无锁：`internal/service/notification_sender.go:30-33`、`internal/service/notification_sender.go:35-58`。
- `NotificationService.channels` 是 map，加载/重载写入：`internal/service/notification.go:68-84`；发送/读取时读 map：`internal/service/notification.go:103-114`、`internal/service/notification.go:200-206`。

影响：

启动窗口内如果有通知，或 Matrix bot token 未配置但通知 worker 仍处理消息，可能 panic 或持续失败。并发 reload/send 时可能触发 map data race。

建议：

- Matrix client 未就绪时不要启动通知 worker，或让 sender 返回明确的 `matrix client not ready` 可重试错误。
- `NotificationSender` 使用 `sync.RWMutex` 或 `atomic.Pointer` 管理 client。
- `NotificationService.channels` 使用 `sync.RWMutex`，重载时构造新 map 后整体替换。
- 对 Matrix 通知链路增加 `go test -race` 覆盖。

### P0-5 LogCenter admin-only 二次校验在空 API key 时放行

证据：

- 多个 admin-only 方法使用 `subtle.ConstantTimeCompare([]byte(apiKey), []byte(h.apiKey))`：`internal/handler/log_center.go:139-145`、`internal/handler/log_center.go:174-180`、`internal/handler/log_center.go:260-266`、`internal/handler/log_center.go:286-292`、`internal/handler/log_center.go:318-324`。
- 如果客户端不传 `X-API-Key` 且 `h.apiKey` 也是空字符串，两边 byte slice 都为空，比较结果为相等。

影响：

在 `server.api_key` 未正确加载或为空时，注册日志源、更新源、创建/更新/删除告警规则等管理能力可能被意外放开。

建议：

- 比较前先要求 `h.apiKey != ""`。
- 更好做法是提取统一 admin middleware，启动时强制校验管理密钥配置。
- 增加空 server key 的 handler 测试。

### P0-6 Matrix 前端 API 契约与后端响应不一致，页面可能显示空数据

证据：

- 后端 `GetStats` 使用 `response.Success(c, stats)`：`internal/handler/matrix_admin.go:185-192`，实际响应是 `{ data: stats }`。
- 前端 `getStats` 注释称直接返回裸 stats，类型也写成 `request<MatrixStats>`：`web/src/api/index.ts:360-364`。
- `MatrixDashboard` 读取 `stats()?.rooms`：`web/src/pages/MatrixDashboard.tsx:7-14`，实际应读 `stats()?.data.rooms`。
- 后端 rooms/events/notifications 均直接 `response.Success(c, rooms/logs)`：`internal/handler/matrix_admin.go:66-73`、`internal/handler/matrix_admin.go:151-182`。
- 前端把 rooms/events/notifications 类型写成分页包装：`web/src/api/index.ts:367-371`、`web/src/api/index.ts:434-456`。

影响：

Matrix dashboard、rooms、events、notifications 页面可能出现统计全 0、列表为空或字段缺失。构建能通过，但用户看到的是错误数据。

建议：

- 确定统一响应协议：所有管理 API 都返回 `APIResponse<T> = { data: T }`，分页返回 `APIResponse<Page<T>>`。
- 后端要分页就真实返回分页结构；不分页则前端类型不要伪造分页。
- 为 `web/src/api/index.ts` 增加契约测试，至少用 mock response 覆盖 Matrix 和 LLM Token 关键接口。

## 4. P1 发现：影响稳定演进，应尽快排期

### P1-1 数据迁移治理落后于模型

证据：

- 实际启动使用 `AutoMigrate`，覆盖大量表：`internal/model/db.go:41-102`。
- `migrations/001_init.up.sql` 只包含 tags、data_sources、rss、webhook、dataset、settings 等旧表：`migrations/001_init.up.sql:4-164`，缺少 LLM、Matrix、LogCenter、CrawlQueue、ActivityLog 等大量表。

影响：

生产 schema 依赖 GORM 自动迁移，缺少可审计的 up/down 迁移路径。回滚、蓝绿发布、跨环境一致性都会变困难。

建议：

- 建立版本化迁移流程，新增/修改表必须提交 `up/down`。
- `AutoMigrate` 可以保留为开发便利，但生产启动应可关闭或只做校验。
- CI 加入迁移从空库构建 schema 的测试。

### P1-2 LLM Proxy streaming 计量、计费和配额不完整

证据：

- 非流式请求读取响应体并解析 token usage 后写日志：`internal/service/llm_proxy.go:1397-1438`。
- 流式路径只记录请求开始，token/cost 均为 0：`internal/service/llm_proxy.go:1691-1696`、`internal/service/llm_proxy.go:1716-1721`、`internal/service/llm_proxy.go:1746-1750`。
- `logRequest` 每次请求启动一个 goroutine 写 DB：`internal/service/llm_proxy.go:1554-1617`。

影响：

流式请求在使用量、计费、成本告警、Token 配额上可能被低估。高流量下每请求一个 goroutine 写库，也可能造成 goroutine 和 DB 写入堆积。

建议：

- 对 SSE 做 usage trailer 解析；无法获取 usage 的供应商需要标记为估算或不计费，不要静默记 0。
- 日志写入改为有界队列 + worker pool + drop/降级策略。
- 配额判断应基于可靠聚合，不应只依赖异步无背压写入。

### P1-3 后台任务生命周期风格不统一

证据：

- `LLMProxyService.New` 构造期间启动多个后台 goroutine：`internal/service/llm_proxy.go:222-280`。
- `KnowledgeIndexService` 启动扫描：`internal/app/app.go:298-302`，但 `Shutdown` 未调用 `knowledgeIndexSvc.Stop()`：`internal/app/app.go:398-424`。
- `KnowledgeIndexService.Stop` 直接 `close(s.stopCh)`，不是幂等：`internal/service/knowledge_index.go:131-135`。
- RateLimiter 构造后启动 cleanup goroutine，缺少 Stop：`internal/middleware/ratelimit.go:27-35`、`internal/middleware/ratelimit.go:65-80`。

影响：

服务 reload、测试、优雅关机和异常恢复会变难；重复 Stop 可能 panic；构造即启动也让依赖注入和单元测试不干净。

建议：

- 所有后台服务统一接口：`Start(ctx context.Context) error`、`Stop(ctx context.Context) error`，Stop 幂等。
- 构造函数只初始化依赖，不启动 goroutine。
- 所有长期 goroutine 都必须有 owner、context、WaitGroup、关闭路径和测试。

### P1-4 配置安全与环境变量展开不完整

证据：

- `config.Load` 只对 LLM、Matrix、Firecrawl、Memos、Meilisearch 部分字段做 `os.ExpandEnv`：`internal/config/config.go:304-323`。
- 默认配置中 `server.mode: noauth`：`config/bellkeeper.yaml:1-5`。
- 凭据加密未启用时继续明文迁移，只打 warning：`internal/model/db.go:366-370`。

影响：

数据库、Redis、NATS 等配置中的 `${VAR}` 如果没有被 Viper 机制覆盖，可能以字面量进入运行时。敏感凭据也可能在未设置密钥时以明文落库。

建议：

- 建立统一配置后处理，对所有 string 字段递归 `os.ExpandEnv`，或明确只允许 `env` 绑定，不允许 YAML 内 `${VAR}`。
- 生产环境不允许 `noauth`、空 admin key、空 credential encryption key。
- 在启动日志中输出安全模式摘要，但不要输出 secret 值。

### P1-5 个人知识库/PKB 层级概念前后不一致

证据：

- 后端默认索引 `archive` 和 `vault`：`internal/config/config.go:441-453`。
- 配置文件说明 `raw` 不进 Meili，`working` 不进知识检索：`config/bellkeeper.yaml:251-264`。
- 前端 Ask 页面默认层级仍是 `raw/working/knowledge`：`web/src/pages/knowledge/KnowledgeAsk.tsx:21-27`。

影响：

用户在 Ask 页面选择的过滤层可能与后端索引层不匹配，导致“明明有数据却问不到”。这是典型易用性问题，也会干扰排障。

建议：

- 前端层级从后端配置/索引统计动态读取。
- 统一术语：建议固定为 `raw/archive/vault`，并明确 `working` 是否只是 report 存档。
- 为 Ask 请求增加层级合法性校验和错误提示。

### P1-6 测试存在“假覆盖”

证据：

- `internal/service/search_test.go` 多处只创建 service 后 `_ = svc`，再 `t.Log` 方法存在：`internal/service/search_test.go:17-45`、`internal/service/search_test.go:86-99`。

影响：

测试通过会给人和大模型制造错误信心。接口签名存在不等于行为正确，尤其无法覆盖 nil repo、limit、scope、错误传播等核心逻辑。

建议：

- 禁止“只证明方法存在”的测试。
- 引入 repository interface 或可替换 fake，让 Service 测试能覆盖行为。
- 对 P0/P1 修复优先补回归测试。

### P1-7 TaskRouter 配置存在但未被使用

证据：

- 配置定义了复杂度阈值和关键词：`internal/config/config.go:62-66`。
- `DetectComplexity` 注释提到 explicit header，但函数没有 header 参数，阈值和关键词硬编码：`internal/service/llm_task_router.go:82-118`。

影响：

配置项给使用者的预期和实际行为不一致，LLM Proxy 的路由策略难以调优。

建议：

- 让 `TaskRouter` 注入 `ComplexityConfig`。
- 函数签名接收 header 或已解析的 routing hint。
- 增加配置驱动路由测试。

## 5. P2 发现：一致性与可维护性改进

### P2-1 分层规范有文档，但代码存在例外且未显式登记

证据：

- `CLAUDE.md` 要求单向调用：`Router -> Handler -> Service -> Repository -> Model`，并禁止 Handler 直接调 Repository。
- `doc/DEVELOPMENT-GUIDE.md` 也写明 Handler 禁止包含业务逻辑、禁止直接调用 Repository。
- 当前 `NewHandlers` 把 `repos` 传入 `LLMProxyHandler`：`internal/handler/handler.go:39-49`。
- `router.Setup` 直接接收 `tokenRepo`：`internal/router/router.go:12`、`internal/router/router.go:133-139`。

判断：

这不一定是立刻要改的 bug，因为 LLM Token 鉴权确实需要 repository。但这种例外必须被架构文档登记，或封装成 auth service/scope service，避免后续大模型照着扩散跨层调用。

### P2-2 响应包装风格未完全统一

项目已有 `internal/pkg/response`，大多数 Handler 使用统一 helper。但 middleware 和部分 raw/proxy 场景仍直接 `c.JSON` 或 `AbortWithStatusJSON`。Raw/proxy 可以作为明确例外；普通 API 建议强制统一 `APIResponse<T>`，否则前端类型会持续漂移。

### P2-3 日志风格混用

不少模块仍使用标准库 `log.Printf`，例如通知、RSS、知识索引等；核心应用层已经使用 zap。建议统一：应用运行期日志全部用 zap logger，测试和极小工具函数可例外。

### P2-4 前端缺少 lint/test 工具链

证据：

- `web/package.json:6-10` 只有 `dev/build/preview`。
- 仓库未发现 `.golangci.yml`、`.editorconfig`、ESLint、Prettier 或 Biome 配置。
- `Makefile:49-51` 有 `golangci-lint run`，但缺少本仓固定配置。

影响：

TypeScript strict 和 Vite build 能抓一部分问题，但抓不到响应契约、未使用代码、格式漂移、可访问性和 UI 约束。大模型协作时尤其容易产生风格漂移。

建议：

- 后端加入 `.golangci.yml`，启用 `govet/staticcheck/gosec/errcheck/bodyclose` 等。
- 前端加入 Biome 或 ESLint + Prettier，并增加 `pnpm lint`、`pnpm test`。
- 增加 `.editorconfig`，统一缩进、换行、末尾空行。

## 6. 模块审查

### 6.1 App/依赖注入

优点：

- `internal/app/app.go` 统一创建数据库、repository、service、handler、router，整体可读。
- Shutdown 已覆盖 HTTP、CrawlQueue、PKB Scheduler、LLMJobQueue、LLMProxy、Matrix sync、通知 worker、NATS、Redis、DB。

风险：

- 部分服务构造即启动 goroutine，破坏生命周期边界。
- KnowledgeIndexService 启动但未在 Shutdown 停止。
- Matrix 通知 worker 与 Matrix gateway 的启动顺序存在 nil client 窗口。

建议：

- App 层成为唯一 goroutine 生命周期 owner。
- 所有服务注册到统一 shutdown list，Stop 幂等。

### 6.2 LLM Proxy

优点：

- 支持通道、模型组、重试、熔断、配额、计价、日志、余额同步、会话绑定，能力完整。
- 非流式路径对 token usage 和 Anthropic cache token 有处理，说明计费设计已经进入较细粒度阶段。

风险：

- `allowed_groups` 未强制校验。
- streaming usage/cost 为 0。
- `logRequest` 无背压。
- TaskRouter 配置未接入。
- `LLMProxyService` 过重，长期建议拆分 TokenScope、Routing、Billing、Logging、Credential、Balance 子域。

建议：

- 先修权限和计量，再做结构拆分。
- 对 `/api/llm/v1/*` 建立专门的安全与契约测试套件。

### 6.3 个人知识库/PKB

优点：

- 已经从文件落地、frontmatter、索引扫描、Meilisearch、Ask/Search 到 PKB scheduler 建立链路。
- 配置中已明确 `raw/archive/vault` 的分层方向，`raw` 不进索引、`archive/vault` 进索引的边界是合理的。

风险：

- 文件路径边界校验有漏洞。
- ingest 的 `Layer` 可逃逸。
- Ask 前端层级仍是旧概念。
- KnowledgeIndexService lifecycle 未完全纳入 App Shutdown。

建议：

- 先统一层级术语和路径安全 helper。
- 所有读取、写入、索引扫描必须共用同一个 `SafeKnowledgePath(base, rel)`。

### 6.4 Matrix 控制平面

优点：

- Gateway、CommandRouter、Admin API、通知队列、事件/通知日志已有完整雏形。
- AdminService 和 policy checker 的方向合理。

风险：

- 通知 worker 和 client 注入并发不安全。
- 前端 Matrix API 契约和后端响应结构不一致。
- rooms/events/notifications 前端假设分页，但后端只返回列表。

建议：

- 先修 API 契约和 notification 并发。
- 再补权限策略测试和 Matrix 页面端到端 smoke test。

### 6.5 LogCenter/System

优点：

- Log source、log entry、alert rule、stats 基础模型完整。
- 对外 source key 和内部 admin key 有分离意识。

风险：

- 空 API key admin bypass。
- 系统命令接口拼 shell。

建议：

- 管理接口全部走统一 admin middleware。
- system 操作接口使用白名单和参数化 `exec.Command`。

### 6.6 前端

优点：

- API 调用集中在 `web/src/api/*`，路由集中，整体结构易扫。
- Solid + Vite 构建通过，TypeScript 已开启 strict。

风险：

- 多个 API 类型把后端 `response.Success` 包装层读错。
- LLM Token 创建/重置 key 前端读取 `res.key`，但后端实际返回 `{ data: { key } }`：`internal/handler/llm_proxy.go:589-593`、`internal/handler/llm_proxy.go:688`、`web/src/pages/llm/LLMUsageAndBilling.tsx:144-146`、`web/src/pages/llm/LLMUsageAndBilling.tsx:172-174`。
- pricing test 也有同类裸返回误判：`internal/handler/llm_proxy.go:797-800`、`web/src/api/index.ts:278-279`。
- 缺少 lint/test，页面级契约问题只能靠人工发现。

建议：

- 定义 `type APIResponse<T> = { data: T }` 并在 request 层统一。
- 对每个 API module 写少量契约测试。
- 长期可从 OpenAPI 或后端 schema 生成 TypeScript 类型。

## 7. 代码风格与统一约束现状

已有基础：

- Go 代码整体能 `go test`、`go vet`、`go build` 通过。
- `Makefile` 提供 `fmt/lint/test/build` 入口。
- 文档中已有清晰分层规范。
- 前端 TypeScript strict + Vite build 通过。

主要缺口：

- 缺少机器可执行的 lint/format 固定配置。
- 分层规范没有自动化检查，也没有例外登记机制。
- API 响应契约没有单一来源，导致前端类型漂移。
- 并发生命周期规范没有统一接口。
- 测试质量没有约束，“无行为断言测试”仍能进入代码库。

## 8. 统一开发规范：人和大模型必须遵守

建议将本节同步到 `CLAUDE.md` 或 `doc/ASSISTANT-GUIDELINES.md`，作为以后所有代码生成和人工开发的硬性约束。

### 8.1 分层边界

1. Router 只注册路由和 middleware，不写业务逻辑。
2. Handler 只做请求解析、参数校验、调用 Service、返回响应。
3. Handler 默认禁止直接访问 Repository。确需例外时，必须在代码注释和架构文档中登记原因、范围和退出计划。
4. Service 承载业务逻辑，不直接操作 Gin Context 或 HTTP response。
5. Repository 只封装数据访问，不写业务编排和外部 API 调用。

### 8.2 API 契约

1. 普通 JSON API 必须使用 `internal/pkg/response`。
2. 前端必须按统一响应类型读取：`APIResponse<T> = { data: T }`。
3. 分页 API 必须真实返回分页结构，不能只在前端类型里假设分页。
4. proxy/raw/streaming endpoint 可以例外，但必须在 API 文件中标注。
5. 修改后端响应结构时，必须同步更新前端类型和至少一个契约测试。

### 8.3 安全

1. 禁止字符串拼接 shell 命令；必须使用 `exec.Command(name, args...)`。
2. 文件路径边界禁止使用 `strings.HasPrefix`；必须使用 `filepath.Rel` 或统一安全 helper。
3. 外部输入中的 path、layer、container、host、model、group 必须经过枚举或白名单校验。
4. 生产模式禁止空 admin key、空 credential encryption key 和 `server.mode: noauth`。
5. 新增鉴权字段后，必须增加“允许”和“拒绝”两个方向的测试。

### 8.4 并发与后台任务

1. 构造函数不得启动 goroutine；启动必须在 `Start(ctx)`。
2. 所有后台任务必须有 context cancel、WaitGroup 和幂等 Stop。
3. 共享 map、client pointer、配置缓存必须加锁或使用 atomic。
4. 队列 claim 必须在事务中完成，且检查 `RowsAffected`。
5. 涉及 goroutine、map、client reload 的修改必须运行 `go test -race` 覆盖相关包。

### 8.5 数据与迁移

1. 新增或修改 model 字段必须同步提交 migration up/down。
2. `AutoMigrate` 只能作为开发便利或启动校验，生产 schema 变更以 migration 为准。
3. seed 数据必须幂等，并清楚说明固定 ID、空字符串、默认值的语义。
4. schema 变更必须考虑回滚、历史数据迁移和索引成本。

### 8.6 测试

1. 禁止只验证“方法存在”“不 panic”的假测试。
2. Service 测试必须覆盖正常路径、错误路径、边界输入。
3. 权限、路径、安全、队列 claim、计费配额属于高风险逻辑，必须有回归测试。
4. 每次提交至少运行 `go test ./...`、`go vet ./internal/...`、`go build ./cmd/bellkeeper`、`cd web && pnpm build`。
5. 并发相关修改额外运行 `go test -race ./...` 或目标包 race test。

### 8.7 前端

1. API client 类型必须反映后端真实响应，不允许“猜 shape”。
2. 页面状态不得依赖错误默认值掩盖 API 契约问题，例如统计失败显示 0 必须伴随错误态。
3. 新页面必须覆盖 loading、error、empty、success 四态。
4. 新增复杂页面时必须补最小组件测试或 API 契约测试。
5. 样式和交互遵循现有设计系统，不为单个页面引入孤立风格。

### 8.8 日志与可观测性

1. 应用运行期日志统一使用 zap。
2. 错误日志必须包含模块、关键 ID、上游/下游名称，但不能输出 secret。
3. 异步写日志或 metrics 必须有背压、drop 策略或批量写入。
4. 重要后台任务必须暴露健康状态或最后成功时间。

## 9. 推荐整改路线图

### 第一阶段：安全和可用性止血

1. 修复 LLM Token `allowed_groups` 校验，并补权限测试。
2. 修复路径逃逸和 shell 注入。
3. 修复 CrawlQueue transaction claim。
4. 修复 Matrix notification nil client/data race。
5. 修复 LogCenter 空 API key bypass。
6. 修复 Matrix/LLM 前端 API 契约错误。

### 第二阶段：稳定性治理

1. 统一后台任务生命周期接口。
2. 补齐 migration up/down。
3. 修复 streaming usage/cost 计量。
4. 统一配置 env 展开与生产安全校验。
5. 修复 KnowledgeAsk 层级概念。

### 第三阶段：工程约束自动化

1. 增加 `.golangci.yml`、`.editorconfig`、前端 lint/test。
2. 建立 API 契约测试或 OpenAPI/类型生成。
3. 清理假测试，补核心模块行为测试。
4. 把第 8 节开发规范同步到大模型协作提示和 CI checklist。

## 10. 最终判断

Bellkeeper 不是需要推倒重来的系统。它已经有明确领域边界和可运行的核心能力，尤其 LLM Proxy 和个人知识库/PKB 的功能链路都已经成形。

当前最重要的是把“能运行”提升到“可被多人和大模型长期安全修改”。只要优先修复 P0，并把第 8 节规范机器化，后续扩展新模块时会明显更稳。
