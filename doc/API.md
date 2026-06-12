# Bellkeeper REST API 参考

> 更新:2026-06-12,基于 `internal/router/router.go` 实际注册路由整理。
> 响应统一走 `internal/pkg/response` 包装(`{code, message, data}`);唯一例外是 LLM 代理端点透传上游。

## 认证

| 方式 | 适用 | 说明 |
|------|------|------|
| `X-API-Key: <server.api_key>` | `/api/*` 内部服务调用 | n8n / CLI 等 |
| noauth(纯内网) | `/api/*` 前端用户 | 生产为预期状态 |
| `Authorization: Bearer sk-bk-*`(LLM Token) | `/api/llm/v1/*` | 专用 token,带模型白名单与配额 |

健康检查端点无需认证。

## 健康与监控

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/health` `/api/health/detailed` `/api/health/live` `/api/health/ready` | 健康检查 |
| GET | `/metrics` | Prometheus 指标 |

## 标签 `/api/tags`

| 方法 | 路径 | 说明 |
|------|------|------|
| GET/POST | `/api/tags` | 列表 / 创建 |
| GET/PUT/DELETE | `/api/tags/:id` | 单条 CRUD |
| GET | `/api/tags/all` | 全量 |
| POST | `/api/tags/batch` | 批量 get-or-create |
| POST | `/api/tags/match` `/api/tags/by-names` | 匹配 / 按名查询 |

## RSS `/api/rss`

| 方法 | 路径 | 说明 |
|------|------|------|
| GET/POST | `/api/rss` | 源列表 / 创建 |
| GET/PUT/DELETE | `/api/rss/:id` | 单源 CRUD |
| POST | `/api/rss/fetch/:id` `/api/rss/fetch-all` | 手动拉取 |
| GET | `/api/rss/fetch-status` | 拉取状态 |
| POST | `/api/rss/validate` | feed 验证 |

## Dataset(索引分区)`/api/datasets`

| 方法 | 路径 | 说明 |
|------|------|------|
| GET/POST + GET/PUT/DELETE `/:id` + GET `/all` `/by-name/:name` | Dataset CRUD |
| POST | `/api/datasets/by-tag` | 按标签推荐 dataset |
| POST/GET | `/api/datasets/article-tags[/:document_id]` | 文章-标签关联 |
| GET | `/api/datasets/articles-by-tag/:tag_id` | 按标签查文章 |
| POST | `/api/datasets/check-url` | URL 去重检查 |

## 文件入库与知识检索

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/files/ingest/url` | URL 提取入库(`{url, title?, tags?, category?, layer?, content?}`) |
| GET | `/api/files/metadata/:id` `/api/files/list` | 元数据 / 文件列表 |
| POST | `/api/files/search` `/api/files/ask` | Meilisearch 搜索 / RAG 问答 |
| GET | `/api/files/stats` `/api/files/health` | 索引统计 / 健康 |
| POST | `/api/files/rebuild` | 重建索引 |
| GET | `/api/knowledge/files/tree\|list\|read\|stats\|search` | Vault 只读浏览 |

## PKB 与报告

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/pkb/daily` | PKB 日报数据 |
| GET | `/api/pkb/vault-cards` `/api/pkb/digests/latest` | vault 卡片 / 最新综述 |
| POST | `/api/reports/write` | 报告写入(n8n 日报交接) |
| GET | `/api/reports/daily-data` `/api/reports/brief-data` | 日报 / 晨报数据 |
| POST | `/api/reports/daily/generate` `/api/reports/brief/generate` | 生成日报 / 晨报 |

## Dashboard

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/dashboard/stats` | 聚合统计(爬取/PKB/LLM 费用) |

## 爬取队列与规则

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/crawl/queue/stats\|audit\|domains\|jobs\|workers\|blocked` | 队列统计 / 审计 / 域名 / 任务 / Worker / 封锁 |
| POST | `/api/crawl/queue/enqueue`、`/jobs/:id/retry`、`/blocked/:id/unblock` | 入队 / 重试 / 解封 |
| GET | `/api/crawl/sources/health` `/api/crawl/jobs` | 源健康 / 爬取任务 |
| POST | `/api/crawl/sources/:id/pause\|resume`、`/sources/batch/...`、`/sources/all/...` | 源暂停恢复 |
| POST | `/api/crawl/fetch/:sourceId` | 手动抓取 |
| GET/POST | `/api/crawl/rules` | LLM 提取规则列表 / 创建 |
| GET | `/api/crawl/rules/domain/:domain`、`/rules/:id/trials` | 按域名 / 试用记录 |
| PUT | `/api/crawl/rules/:id/status` | 规则启用 / 回滚 |

## 分类

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/classify/article` | LLM 文章分类 |

## LLM Proxy(代理入口)

| 方法 | 路径 | 说明 |
|------|------|------|
| ANY | `/api/llm/v1/*path` | OpenAI 兼容代理(chat/completions、rerank 等;LLM Token 鉴权;响应透传) |

## LLM Proxy(管理,前缀 `/api/llm`)

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/channels/status` `/health` `/groups/status` `/stats` `/logs` `/rate-limit-events` `/alerts` | 运行时状态与日志 |
| POST / DELETE | `/channels/:name/reset` / `/groups/:name/sticky` | 重置熔断 / 清粘性 |
| CRUD | `/config/channels[/:id]`、`/config/groups[/:id]` | 渠道 / 模型组配置(DB,热重载) |
| CRUD | `/config/channels/:id/credentials`、`/config/credentials/:id` | 加密凭证 |
| POST | `/reload` | 配置热重载 |
| GET / POST | `/channels/:name/balance[/history]`、`/balances` / `/balances/refresh` | 余额查询 / 刷新 |
| CRUD | `/tokens[/:id]`;POST `/tokens/:id/regenerate`;GET `/tokens/:id/usage` | Token 体系 |
| CRUD | `/pricing[/:id]`;POST `/pricing/test-calc` | 定价管理 / 成本试算 |
| GET/DELETE | `/conversations[/:id]` | 会话粘性绑定 |
| GET | `/usage?group_by=token\|channel\|model\|date&from=&to=` | 用量聚合 |
| GET / POST | `/rate-limits` / `/rate-limits/:id/lock`、`/rate-limits/:id/:model/reset` | 限流学习 / 锁定 / 重置 |
| GET/POST | `/coding-strategy` | Coding 路由策略 |

## Matrix

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/matrix/notify` | 发送通知(n8n 主入口) |
| GET | `/api/matrix/notify/:id`、`/notify/channels` | 通知状态 / 频道列表 |
| POST | `/api/matrix/notify/channels/reload` | 频道重载 |
| GET | `/api/matrix/admin/rooms` | 房间列表 |
| POST | `/api/matrix/admin/rooms` | 创建房间 |
| PUT | `/api/matrix/admin/rooms/:id` | 更新房间(enabled/is_admin/auto_discover 等) |
| DELETE | `/api/matrix/admin/rooms/:id` | 删除房间 |
| GET | `/api/matrix/admin/channels` | 频道列表 |
| POST | `/api/matrix/admin/channels` | 创建频道 |
| PUT | `/api/matrix/admin/channels/:name` | 更新频道 |
| DELETE | `/api/matrix/admin/channels/:name` | 删除频道 |
| GET | `/api/matrix/admin/commands` | 命令列表 |
| PUT | `/api/matrix/admin/commands/:name` | 更新命令策略覆盖 |
| GET | `/api/matrix/admin/command-logs` | 命令日志 |
| GET | `/api/matrix/admin/events` | 事件日志 |
| GET | `/api/matrix/admin/notifications` | 通知日志 |
| POST | `/api/matrix/admin/notifications/:id/retry` | 重试通知 |
| GET | `/api/matrix/admin/stats` | 统计 |
| GET | `/api/matrix/admin/roles` `/roles/:user_id` | 管理员角色列表 / 查询 |
| POST/DELETE | `/api/matrix/admin/roles` `/roles/:user_id` | 设置 / 删除管理员角色 |

## 日志中心与审计

| 方法 | 路径 | 说明 |
|------|------|------|
| GET/POST | `/api/logs/entries[/:id]` | LogCenter 日志条目 |
| CRUD | `/api/logs/sources[/:id]` | 日志源 |
| GET | `/api/logs/dashboard[/:period]` | 仪表盘 |
| CRUD | `/api/logs/alerts[/:id]` | 告警规则 |
| GET/POST | `/api/logs`、`/modules`、`/stats` | 活动日志(跨模块审计) |

## n8n 工作流 `/api/workflows`

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/definitions[/:key]` | 工作流 JSON 定义(仓库事实源) |
| PUT/DELETE | `/definitions/:key` | 保存 / 删除定义 |
| POST | `/definitions/:key/push`、`/definitions/push-all` | 推送到 n8n |
| GET | `/status`、`/executions`、`/:id` | 状态 / 执行历史 |
| POST | `/trigger/:name`、`/:id/activate\|deactivate` | 触发 / 激活 |

## 其他

| 方法 | 路径 | 说明 |
|------|------|------|
| GET/PUT | `/api/settings[/:key]` | 运行时 KV 配置 |
| GET | `/api/search` | 全局搜索 |
| GET | `/api/todos/export[/plain]` | todo.txt 导出 |
| POST | `/api/system/restart`、`/backup`、`/containers/:name/restart` | 系统操作 |
| GET | `/api/system/disk`、`/containers` | 系统信息 |
| GET/PUT | `/api/logging/level` | 日志级别 |
| POST | `/api/config/reload[/llm-proxy]` | 配置重载 |
