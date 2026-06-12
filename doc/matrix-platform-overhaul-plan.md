# Matrix 平台深度优化方案（架构 + AI Agent 入口 + 前端重构）

> 创建日期：2026-06-11
> 状态：**待执行**
> 关联文档：`doc/notification-monitoring-overhaul-plan.md`（通知/日报内容侧重构，本方案与其互补不重叠）、`doc/matrix/`（原始设计文档，本方案执行后需同步修订）、`doc/LLM_PROXY_GUIDE.md`
> 核心定位转变：**Matrix 从「通知出口 + 几条快捷命令」升级为「整个 SilkSpool/Bellkeeper 体系的统一入口」——AI 对话即入口，命令是快捷方式，通知是回流。**

---

## 一、现状诊断（2026-06-11 全部经代码审读 + 生产 DB/日志实测验证）

### 1.1 模块全景

后端 `internal/matrix/`（约 3300 行）+ 接线层：

| 层 | 文件 | 职责 | 现状 |
|----|------|------|------|
| gateway | `gateway/client.go` (123) / `gateway/sync.go` (255) | mautrix v0.26.4 封装、sync loop、事件入库 | ✅ 在跑，有缺陷（见 1.3） |
| command | `command/router.go` (378) / `parser.go` / `handler*.go` / `todotxt.go` | 命令解析、DB 动态命令、内置命令、Memos/n8n/QA 处理器 | ⚠️ DB 配置与代码工厂脱节（见 1.2-②） |
| policy | `policy/checker.go` / `role.go` | 4 级角色权限（owner/admin/member/guest） | ⚠️ 过度设计且默认拒绝（见 1.2-④） |
| worker | `worker/notification_worker.go` | NATS JetStream 通知消费者 | ✅ 在跑 |
| infra | `infra/nats.go` / `infra/redis.go` | NATS 队列、Redis（sync token/去重/限流/锁） | ⚠️ commands stream 是死基础设施 |
| service | `service/command.go` / `notification.go` / `notification_sender.go` / `admin.go` | 命令服务、通知服务、管理服务 | ✅ 在跑 |
| handler/router | `handler/matrix_notify.go` / `matrix_admin.go`、`router.go:260-286` | `/api/matrix/notify*` + `/api/matrix/admin/*` | ⚠️ 与前端契约大面积不匹配（见 1.4） |
| model | `model/matrix.go`（8 张表） | rooms/channels/commands/events/notifications/command_logs/sync_state/user_roles | ⚠️ rooms 表空、user_roles 无人用 |
| 前端 | `web/src/pages/Matrix*.tsx`（7 页，1578 行） | 总览/房间/频道/命令/通知/事件/命令日志 | ⚠️ 写操作大半是死按钮 |

### 1.2 生产实测结论（keeper:8090，bellkeeper-db 直查）

① **机器人本体还活着，但只剩半条命**：
- 事件持续入库（最近 `matrix_events` 2026-06-11 18:08），通知持续发送（5717 条 sent，当天仍有）；
- 但 `matrix_sync_state.last_sync_at` 停在 **2026-04-09**——sync token 的 **DB 冷备路径断了 2 个月**（Redis 热路径仍工作，所以没被发现）。一旦 Redis 数据丢失，bot 将从零全量同步并**重放全部历史消息**。`sync.go:persistToken` 只 Warn 不告警，是「错误静默」的典型。

② **DB 命令配置是「假配置源」**：`matrix_commands` 表里 13 条命令中，`ragflow_qa`/`ragflow_search`/`memos_todo_list`/`memos_todo_create` 这些 handler_type 在 `router.go:createHandlerFromDB` 的 switch 里**根本不存在**（RAGFlow 已在 c7ed5a8 下线），全部落入 default 静默跳过。现在 `!问`/`!搜` 能用纯粹是因为运行时 `SetKnowledgeHandlers`/`registerHandlers` 用代码硬注册覆盖了——**DB 配置和实际行为已完全脱节**，前端「命令管理」页展示的是幻觉。

③ **命令实际使用情况**（`matrix_command_logs`）：`搜` 22 次成功（最近 2026-06-10，唯一活着的功能）；`问` 最后成功 04-29；`列表`/`新增` 最后 04-27/04-21 且尾部全是 failed。**Memos 待办链路和 QA 链路事实上已荒废。**

④ **房间与权限模型没有落地**：`matrix_rooms` 表 **0 行**（`!rooms` 和前端房间页永远是空的）；`matrix_user_roles` 无人写入，`policy.Checker.GetUserRole` 查不到记录时**默认 guest = 拒绝一切**，等于「除了 config 里硬编码的 @singll 之外谁都不能用」——4 级角色 + 7 种 Permission 的体系（`policy/role.go` 132 行）对单用户系统是纯负担。

⑤ **告警风暴**：`alerts` 频道 14 天 1944 条，近 3 天 Top12 全部是 `[CrawlQueue] URL blocked: https://openai.com/...` 的**同一批 URL 每天重复 22-24 次**（`crawl_queue.go:notifyBlocked` 无去重）。通知层只有频道级 20 条/分钟限流（超限直接丢，不聚合）。真正重要的告警被淹没。

⑥ 频道现状：`alerts`/`daily` 在用；`todo`/`qa` 频道 14 天 0 条通知，名存实亡。

### 1.3 代码级缺陷清单（带证据）

| # | 缺陷 | 位置 | 影响 |
|---|------|------|------|
| B1 | `formatInt` 手写递归数字转换**且是错的**：`formatInt(123)` 返回 `"312"`（个位在前+helper 顺序错），int64 分支 0 返回空串 | `command/handler.go:283-300` | `!health` 显示的所有统计数字都是错的；应直接 `strconv.Itoa` |
| B2 | `StatusHandler` 返回硬编码「全部在线」假状态 | `command/handler.go:118-130` | `!status` 永远报喜，违反「禁止占位残留」 |
| B3 | 命令在 sync 事件回调里**同步执行**（mautrix DefaultSyncer 顺序分发）| `sync.go:handleRoomMessage` → `commandService.ExecuteMessage` | 一条 30s 的 QA/LLM 命令会阻塞后续所有事件处理 |
| B4 | `SyncLoop.stopped` 无锁读写 | `sync.go:24,107-115` | 数据竞争（违反 2.4 并发安全） |
| B5 | worker `msg.Nak()` 无退避，失败消息立即重投 | `worker/notification_worker.go:151` | 失败时打转空耗；且 `RetryCount` 从未递增，`maxRetry` 判断永远走第一个分支 |
| B6 | `stripHTML` 手写 70 行字符串替换 | `notification_sender.go:96-160` | 脆弱且难维护，项目已依赖 bluemonday 可做 strip |
| B7 | sync token DB 持久化断裂（见 1.2-①），`Upsert` 用 `Assign(state).FirstOrCreate(state)` 的写法可疑 | `repository/matrix_sync_state.go:38-41` | 灾备失效 |
| B8 | NATS `commands` stream 创建后零生产者零消费者 | `infra/nats.go:75-99`、配置 `nats.streams.commands` | 死基础设施 |
| B9 | `Router.adminUsers` 字段已标 deprecated 但仍在维护双份状态 | `command/router.go:27,196-204` | 冗余 |
| B10 | `matrix_events` 全量记录所有消息且无清理任务 | `sync.go:170-186` | 无限增长（当前 5841 行，量级尚可但无界） |
| B11 | `command.go:35` 兜底硬编码 `@singll:matrix.singll.net` | `service/command.go:33-36` | 硬编码用户 ID 散落（defaults 里也有 domain） |
| B12 | `NotificationService.checkRateLimit` 超限直接拒绝且调用方多半不重试 | `service/notification.go:226-240` | 限流=丢消息，无聚合降级 |

### 1.4 前端问题清单

前端 `matrixApi`（`web/src/api/index.ts:399-510`）调用了 **9 个后端不存在的端点**，对应页面上的按钮点了就是 404：

| 前端调用 | 后端路由（`router.go:268-286`） | 结论 |
|----------|------|------|
| `PUT /matrix/admin/rooms/:id`（updateRoom） | 无 | ❌ 死按钮 |
| `POST /matrix/admin/channels`（createChannel） | 无（只有 GET + PUT /:name） | ❌ |
| `DELETE /matrix/admin/channels/:id` | 无 | ❌ |
| `POST /matrix/admin/commands`（createCommand） | 无（只有 GET） | ❌ |
| `PUT /matrix/admin/commands/:name` | 无 | ❌ |
| `DELETE /matrix/admin/commands/:id` | 无 | ❌ |
| `POST /matrix/admin/commands/:name/test` | 无 | ❌ |
| `GET/POST /matrix/admin/notifications/:id(/retry)` | 无（只有列表） | ❌ |
| `GET /matrix/admin/events/:id` | 无（只有列表） | ❌ |

此外：7 个菜单项过度铺开（房间页永远空、频道/命令页展示假配置）；Matrix 总览页只有 4 个静态计数卡 + 事件流水，**没有 bot 在线状态、sync 健康度、发送成功率**这些真正该看的东西；主 Dashboard 引用 matrixStats 但只显示「房间 0」这种无意义数字。

### 1.5 AI 接入现状

整个 Matrix 模块的 AI 能力 = `!问`（knowledge ask，走 Meilisearch 检索 + LLM 总结）一条。而 LLM Proxy 侧已具备完整的承载能力（`doc/LLM_PROXY_GUIDE.md`）：OpenAI 兼容 `/api/llm/v1/*`、虚拟模型组（`pool-chat-free/balanced`、`pool-summary`、`pool-pkb`）、任务感知路由、**会话粘性（X-Conversation-ID 保 prompt cache）**、Token 计费限额、熔断与自适应限流、`internal/llmclient` 内部客户端、`llm_jobs` 持久队列。**Matrix 这边一项都没用上**——这就是本方案最大的增量空间。

---

## 二、目标定位与设计原则

```
                    ┌─────────────────────────────────────────────┐
   用户(Matrix客户端) │  Matrix = 统一入口（对话即操作）                │
        │            │  · AI Agent：自然语言 → 工具调用 → 全系统能力   │
        ▼            │  · 命令：! 前缀 = 确定性快捷方式                │
   txhk homeserver   │  · 通知：系统事件经治理(去重/聚合/分级)后回流    │
        │            └─────────────────────────────────────────────┘
        ▼
   keeper Bellkeeper ──── LLM Proxy(pool-*) ──── 上游 LLM
        │
   Web 仪表盘 = 观测平面（看状态/看日志/管配置），不再承担"操作入口"野心
```

设计原则（按本项目教训定制）：

1. **代码是命令的唯一事实源**，DB 只存策略（启用/权限/别名），消除「DB 配置幻觉」（1.2-②的根治）。
2. **单用户现实主义**：权限模型从 4 角色 7 权限收敛为「admin 白名单 + 房间白名单」两层；删掉无人写入的 user_roles 链路。
3. **错误显式化**：sync 冷备断裂、发送失败、工具调用失败都必须可见（进 `/matrix/admin/health` + 告警），不许 Warn 了事。
4. **复用优先**：Agent 工具全部映射到既有 Service 方法；通知聚合复用 LLM Proxy 已验证的「5min 合并 + 1h 抑制」模式；不新造轮子。
5. **与日报重构计划协同**：本方案管「管道与入口」，`notification-monitoring-overhaul-plan.md` 管「内容与口径」；日报继续经 `daily` 频道走本管道。

---

## 三、后端架构重构

### 3.1 消息处理流水线（解决 B3 + 引入 Agent 分流）

`sync.go` 的事件回调改为**只做轻活**（去重、入库、投递），执行移入有界 worker 池：

```
sync 事件 → 去重(Redis) → 审计入库 → MessageDispatcher.Dispatch (非阻塞)
                                         │ 信号量 chan struct{} 限 8 并发, stopCh+wg 可停
                                         ▼
                              ┌── 以 !/！ 开头 → CommandService（现有 Router，保留）
                              ├── 普通消息 & 房间启用 agent → AgentService（新增，§3.2）
                              └── 其余 → 忽略
```

- 执行前先发 `m.typing`（打字指示），>5s 的命令/Agent 回合先回「⏳ 处理中」再用 `m.replace` 编辑为最终结果（mautrix 原生支持）。
- worker 池遵守 2.4：`stopCh + sync.WaitGroup`，`Stop()` 后 `wg.Wait()`。

### 3.2 AgentService —— Matrix 完全接入 AI 的核心（新增 `internal/matrix/agent/`）

**交互模型**：在指定房间（频道配置 `agent_enabled=true`，建议就是现 `qa` 房间改造为「AI 控制台」房）里，任何不带 `!` 前缀的消息直接进入 Agent 回合。@bot 提及在任意已加入房间也触发。

**执行循环**（经 LLM Proxy 的 OpenAI 兼容 tool calling）：

```go
// internal/matrix/agent/agent.go
type AgentService struct {
    llm      *llmclient.Client      // BaseURL=llm_proxy(进程内回环或直连), 复用现有客户端,扩展 tools 支持
    tools    *ToolRegistry          // 工具注册表
    sessions *SessionStore          // Redis: 房间级多轮上下文
    notify   *NotificationService   // 回复发送复用通知管道? 否——直接 client.SendHTMLMessage(同步回复)
}

// 回合循环: messages+tools → LLM → (tool_calls? 执行→追加结果→再调) ×≤5 → 最终文本回复
```

`llmclient.ChatRequest` 需扩展 `Tools []Tool` / `ToolChoice` 字段与响应的 `tool_calls` 解析（OpenAI schema，Proxy 直通上游，anthropic 渠道已有双向转换）。请求头：`X-Caller-ID: matrix-agent`、`X-Conversation-ID: <roomID>`（吃 Proxy 会话粘性保 prompt cache）、模型用新建组 `pool-agent`（需要稳定支持 function calling 的成员，初期可指 `pool-chat-balanced` 验证）。

**工具注册表**（v1 全部只读/低危，映射既有 Service，构造函数注入）：

| 工具 | 映射 | 说明 |
|------|------|------|
| `system_health` | `HealthService.Detailed()`（日报计划配置化后的版本） | 服务状态 |
| `dashboard_stats` | `DashboardService.GetStats()` | 爬取/PKB/LLM 当日统计 |
| `knowledge_search` | `FileSearchService.Search()` | 知识库检索 |
| `knowledge_ask` | `AskService.Ask()` | 知识库问答（RAG） |
| `llm_usage` | `LLMProxyService` 用量/余额/告警查询 | 「今天 LLM 花了多少」 |
| `todo_list` / `todo_add` / `todo_done` | `command/handler_memos.go` 的 Memos client（抽到可复用位置） | 待办 |
| `daily_report` | `DailyReportService.Generate()`（日报计划落地后） | 「重发今天日报」 |
| `crawl_status` / `rss_status` | `CrawlQueueService` / feed 健康查询 | 爬虫与 RSS |
| `trigger_n8n` | 现 `N8NTriggerHandler` 逻辑 | 触发工作流 |

**写操作护栏**：每个工具声明 `Level: readonly|write|danger`。`write` 在非 admin 发起时拒绝；`danger`（v2 才有，如重启服务）需要 bot 回问「回复 `确认` 执行」，确认 token 存 Redis 60s。所有工具调用写 `matrix_command_logs`（handler_type=`agent_tool`），审计闭环。

**会话**：`SessionStore`（Redis，`matrix:agent:session:<roomID>`，TTL 30min，保留最近 20 条 message，含 tool 轮次）。`!reset` 内置命令清会话。**Token 防护**：上下文按字符数截断 + 每房间每小时回合数限制（复用 `IncrRateLimit`），LLM Proxy 的 token 配额是第二道防线。

### 3.3 命令模型重构（根治 1.2-②）

- **删除** `createHandlerFromDB` 的 handler_type 工厂。命令只由代码注册（builtin、memos、knowledge、n8n、agent 控制类）。
- `matrix_commands` 表降级为**策略覆盖表**：`command_name / is_active / permission_level / aliases / description`。Router 注册后用它过滤与改名；表里没有的命令按默认启用。
- 迁移：清理 `ragflow_*`、`memos_todo_list/create` 等死行（migration SQL），保留用户自定义的 n8n webhook 命令（这是 DB 配置仍有价值的唯一场景，改为 `webhook` 类型走专门的 `webhook_commands` 读取路径或保留在策略表 config 字段）。
- 内置命令清单（v1）：`help` `ping` `status`(真实化) `health` `搜/search` `问/qa` `待办/列表/新增/完成` `日报` `用量` `reset` `rooms`。

### 3.4 权限与房间治理简化

- **权限两层制**：`admin_users`（config，可执行一切+写工具）与「房间白名单」（bot 只处理 `matrix_rooms.is_active=true` 的房间消息；被邀请进新房间自动注册为 `is_active=false` 待启用，**不再无条件 auto-join 即生效**——现在任何人拉 bot 进房就能让它入库所有消息，是个小安全洞）。
- **房间自动发现落地**：sync 时（join 事件 + 启动时 joined_rooms 接口）把已加入房间 upsert 进 `matrix_rooms`（房名从 state 取），解决「表永远是空的」。频道配置改为引用 room 记录。
- **删除** `matrix_user_roles` 表、`policy/role.go` 的 Permission 体系、相关 admin API 与前端「角色管理」。`policy.Checker` 收敛为 `IsAdmin(user) + IsRoomEnabled(room)` 两个函数。
- 非白名单用户的消息：忽略（不入库 matrix_events，省得垃圾增长）。

### 3.5 通知治理（管道侧，与日报计划 4.1 衔接）

在 `NotificationService.Send` 内加**去重聚合层**（复用 LLM Proxy 告警的成熟模式，下沉为通用能力）：

- 请求新增 `DedupKey`（如 `crawl_blocked:<domain>:<reason>`）与 `Severity (info|warn|critical)`;
- 同 DedupKey 在 **5min 窗口内合并**（首条立发，窗口内累计，窗口关闭时若 count>1 补一条「同类 ×N」）；**1h 抑制**相同 key 重复首发；critical 不抑制；
- 聚合状态存 Redis（`matrix:notify:agg:*`），实现为 NotificationService 内部 goroutine（stopCh+wg）；
- `crawl_queue.notifyBlocked` 改带 DedupKey + severity=info——风暴当场消失；其余调用方逐步补 key，不带 key 的走旧行为（兼容）。
- 频道规划收敛为 4 个真实房间：`alerts`（warn/critical 实时）、`reports`（日报/周报，现 `daily` 改名或保留）、`ai`（Agent 控制台，现 `qa` 房改造）、`todo`（保留待办提醒）。DB 迁移更新 channel 行。

### 3.6 基础设施取舍

- **保留** NATS 通知队列（已稳定、有持久化与重试语义）+ Redis（token/去重/会话/聚合）。
- **修复** worker：Nak 改 `NakWithDelay(指数退避)`，`RetryCount` 用 JetStream `NumDelivered` 元数据替代（B5）。
- **删除** NATS `commands` stream 及配置（B8）。
- **修复** sync token 冷备（B7）：排查 `Upsert` 写法（换 `clause.OnConflict` 标准 upsert），并把「DB 持久化连续失败 >10min」纳入 health 告警。
- 杂项修复：B1（`strconv.Itoa`）、B2（status 接 AdminService 真实数据）、B4（加锁或 atomic）、B6（bluemonday StripTags + html-to-text）、B9/B11（删冗余、入 defaults）、B10（events 表 90 天清理任务，挂现有定时器体系）。

### 3.7 可观测性（配合仪表盘的关键）

新增 `GET /api/matrix/admin/health`（供前端总览 + 主 Dashboard + Agent `system_health` 工具复用）：

```json
{ "gateway": {"connected": true, "bot_user_id": "...", "last_event_at": "...", "sync_token_db_age_sec": 123},
  "queue":   {"pending": 0, "worker_running": true},
  "notify_24h": {"sent": 412, "failed": 1, "suppressed": 89},
  "command_24h": {"total": 12, "failed": 0},
  "agent_24h": {"turns": 31, "tool_calls": 58, "tokens": 184000} }
```

Prometheus 指标补齐 agent 回合/工具调用计数（`metrics.go` 已有 matrix 消息/命令计数器）。

---

## 四、前端重构（简洁但不缺功能）

### 4.1 信息架构：7 页收敛为 3 页

| 新页面 | 合并自 | 内容 |
|--------|--------|------|
| `/matrix` **总览** | MatrixDashboard + 部分 Events | Bot 健康卡（在线/sync 冷备年龄/队列积压/24h 成功率，数据来自 §3.7）、Agent 用量卡、最近事件与通知时间线（行内展开详情，替代死掉的 detail 端点）、快速发送测试通知 |
| `/matrix/console` **命令与会话** | MatrixCommands + MatrixCommandLogs | 命令清单（代码注册的真实清单 + 启用开关/权限级别编辑 = 策略表 PUT）、命令测试器（room/args 表单 → 新 test 端点）、命令与 Agent 工具调用日志（含耗时/状态筛选） |
| `/matrix/channels` **通知与房间** | MatrixChannels + MatrixRooms + MatrixNotifications | 房间列表（自动发现 + 启用开关 + 类型标注）、频道→房间映射编辑、通知日志（筛选/重发按钮 → 新 retry 端点）、聚合抑制统计 |

菜单从 7 项减到 3 项；删除「角色管理」相关 UI（随 §3.4）。

### 4.2 API 契约补齐（消灭死按钮）

后端新增端点（全部走 `internal/pkg/response`）：

| 端点 | 实现 |
|------|------|
| `GET /api/matrix/admin/health` | §3.7 |
| `PUT /api/matrix/admin/rooms/:id` | 改名/类型/启用（AdminService 已有 repo 能力） |
| `PUT /api/matrix/admin/commands/:name` | 策略表更新（启用/权限/别名）+ `Router.ReloadCommands` |
| `POST /api/matrix/admin/commands/:name/test` | 构造 `command.Context` 直接 `Route()`，返回 Response 与耗时（不经真实房间） |
| `POST /api/matrix/admin/notifications/:id/retry` | 复用 `NotificationSender` 重发单条 |
| `POST /api/matrix/admin/channels` / `DELETE .../:name` | 频道增删（repo 已有 model） |

前端 `matrixApi` 同步修正；不再保留任何指向不存在端点的调用（自检：每个 `matrixApi.*` 在 `router.go` 有对应路由）。

### 4.3 主 Dashboard 联动

主页 Matrix 卡片从「房间 0 →」改为有意义的三元组：**Bot 状态点（绿/红，来自 health.gateway.connected + last_event_at 新鲜度）、24h 通知（含 failed 红字）、24h Agent 回合数**，点击进 `/matrix` 总览。日报/告警在主页的呈现归日报计划管，不重复。

---

## 五、配置变更

```yaml
matrix:
  # 既有项不变: homeserver_url / bot_user_id / bot_access_token / device_id / command_prefix / max_retry / admin_users
  agent:
    enabled: true
    model: "pool-agent"          # LLM Proxy 模型组,需建组(初期可填 pool-chat-balanced)
    max_turns_per_hour: 30       # 每房间限速
    session_ttl: "30m"
    max_tool_iterations: 5
notify:
  aggregation_window: "5m"
  suppression_window: "1h"
# 删除: nats.streams.commands
```

新增环境变量同步 `bellkeeper-init.sh`；`.env` 改动后 `spool sync push keeper` + `spool restart keeper bellkeeper`。

## 六、数据模型迁移

1. `matrix_commands`：删死行（`ragflow_*` 等），结构上新增 `aliases jsonb`（或复用 config 字段）；
2. **drop** `matrix_user_roles`（先备份导出，确认无业务读取后删）；
3. `matrix_notifications` 新增 `dedup_key varchar(255) index`、`severity varchar(20)`、`suppressed_count int`；
4. `matrix_channels`：`todo`/`qa` 行按 §3.5 频道规划更新；
5. `matrix_events` 建 90 天清理任务（不改表）。

---

## 七、实施计划（小步提交，每 Tier 独立绿色可验收）

| Tier | 内容 | 验收 |
|------|------|------|
| **T1 止血修 bug** | B1/B2/B4/B5/B7/B9/B11 + sync 冷备告警 | 单测 formatInt→Itoa 行为；生产 `matrix_sync_state.last_sync_at` 恢复滚动 |
| **T2 通知治理** | §3.5 聚合去重 + crawl_queue 带 DedupKey + 频道迁移 | alerts 房间观测 24h：同类 blocked 告警 ≤1+汇总条；单测聚合窗口 |
| **T3 命令模型重构** | §3.3 删 DB 工厂 + 策略表 + 迁移 + B8 删 commands stream | `!help` 清单与代码注册一致；DB 死行清零；`go build/vet` 绿 |
| **T4 流水线异步化 + 房间治理** | §3.1 worker 池 + typing + §3.4 白名单/自动发现 + 删 user_roles | 长命令不阻塞后续消息（并发发两条验证）；`!rooms` 有真实数据 |
| **T5 Agent MVP** | llmclient tools 扩展 + AgentService + 只读工具 6 个 + 会话 + 限速 | 在 ai 房间自然语言问「今天爬取情况」得到真实数据回答；工具调用入审计日志 |
| **T6 Agent 扩展** | todo 写工具 + 日报/n8n 触发 + 确认机制 | 「帮我加个待办」全链路可用 |
| **T7 后端 API 补齐 + health** | §3.7 + §4.2 全部端点 | 每个前端调用有对应路由（grep 自检） |
| **T8 前端重构** | §4.1 三页 + §4.3 主页卡片 + 删死代码 | `pnpm build` 绿；无 404 按钮；菜单 3 项 |
| **T9 文档与收尾** | 修订 `doc/matrix/*` 与 API.md；归档本计划进度 | 文档与实现一致，无矛盾状态 |

依赖关系：T5 依赖 T1/T4；T2/T3 可与 T4 并行；T8 依赖 T7。日报计划（notification-monitoring-overhaul）建议先行或与 T2 同批，其 `DailyReportService` 是 T6 的 `daily_report` 工具依赖。

## 八、风险与回滚

- **Agent 成本失控**：三道防线（回合限速 / 上下文截断 / LLM Token 配额告警）；`agent.enabled=false` 一键关停，不影响命令与通知。
- **重构期间通知中断**：通知管道改动（T2）保持请求结构向后兼容（无 DedupKey 走旧路径）；NATS 队列不动。
- **sync 行为变化**：T4 异步化保持事件去重语义（Redis dedup 不变），灰度时观察 `matrix_events` 入库连续性。
- **生产验证手段**：`spool logs keeper bellkeeper`、`/api/matrix/admin/health`、DB 直查（本文档 §1.2 的 SQL 可复用）。

## 九、自检清单（每 Tier 提交前）

```
□ go build ./... / go vet ./... / (动前端) pnpm build 全绿
□ 新文件/包 grep 有真实调用方；前端无指向不存在端点的调用
□ 写+存+读+展示链路完整（如:聚合计数 → DB → admin API → 前端卡片）
□ 无占位残留；status/health 不许假数据
□ 新 goroutine 均 stopCh+wg 可停；共享状态有锁
□ 新增环境变量已同步 bellkeeper-init.sh
```
