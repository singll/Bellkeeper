# Bellkeeper 架构文档（1.0 终态）

> 更新日期: 2026-07-06
> 状态标注: ✅ 已实施 | 🔶 运维待建 | 📋 规划中
> 权威事件源: [BELLKEEPER-1.0-REVAMP-PLAN.md](BELLKEEPER-1.0-REVAMP-PLAN.md)

---

## 定位与职责

Bellkeeper 是 SilkSpool 的**知识治理中台 + LLM 代理网关 + Matrix 控制平面 + 事件驱动平台**,充当 n8n 工作流的"能力增强器"。

**核心价值**:做 n8n 做不好的事——有状态的限速、去重、爬取队列、路由、日志、治理;同时承担 Matrix Gateway、LLM Proxy 和 Agent 三个长连接/交互式服务。知识真相源是 **Obsidian Vault + Markdown 文件**(TrueNAS `data/knowledge/`),Bellkeeper 从中派生索引、搜索与问答。

### Bellkeeper 负责

| 能力 | 状态 | 说明 |
|------|------|------|
| **事件总线 NATS JetStream** | ✅ | `internal/eventbus/` 一级共享基础设施；6 条 stream（notifications/knowledge/llm/matrix/system/logs）；Event envelope 契约（ULID+TraceID）；禁止绕过直连 |
| LLM 代理池 | ✅ | `internal/llmgateway/` 独立包；Gateway 进程内直调接口（Chat/Rerank）；多 provider 路由、熔断、粘性、自适应限流学习、计费、余额、Anthropic/Gemini/Rerank 协议转换；对外 OpenAI 兼容 API `/api/llm/v1/*`；详见 [LLM-GATEWAY-API.md](LLM-GATEWAY-API.md) |
| LLM 持久任务队列(`llm_jobs`) | ✅ | 事件驱动（`llm.job.submit` NATS 事件 + DB 状态机）；原子 claim 反防重；recovery 兜底重投 |
| 个人知识库 PKB | ✅ | `pkb-curate` CLI: 漏斗（raw/archive/vault）+ 原子化重构 + digest 综述 + golden set 评估；全闭环无人值守（Scheduler 驱动 + 域管理）；P2 骨架/缺口/闸审核 + 资讯库闸；提示词外置 `config/pkb/prompts/` |
| 文件入库与治理 | ✅ | Trafilatura 主力 + Firecrawl 兜底,Markdown + frontmatter 落盘 `/mnt/knowledge/` |
| 爬取队列 CrawlQueue | ✅ | 事件驱动三 worker 链（crawl→extract→index）+ 域名级限速/健康画像（HealthScore/Pause 自动启停）+ 错误分类退避 + LLM 爬取规则优化闭环 |
| URL 去重(精确/归一化/模糊 + 内容哈希) | ✅ | DB 内多级匹配 |
| 分类与标签 | ✅ | LLM 驱动分类,提示词外置 `config/prompts/`（system/user 分离 + json_object 结构化 + 自修复重试）；标签置信度/规范化/来源记录(tag_source),frontmatter+Meili+DB 三处持久化 |
| RSS 订阅源管理 | ✅ | 源 CRUD、按源 fetch_interval 调度、RSSHub 参数支持、feed 验证 API；源配置 `config/rss-sources.json` |
| Meilisearch 检索与问答 | ✅ | `/api/files/search\|ask`,事件驱动实时索引（入库即刷新，替代定时扫描）；问答 SSE 流式 + rerank 精排 |
| Matrix 控制平面 | ✅ | Gateway(mautrix-go 长连接)+ Command Router + 权限两层制(IsAdmin+IsRoomEnabled)+ 通知网关(eventbus)+ 通知聚合去重(5min合并+1h抑制)+ Admin API |
| Agent 系统 | ✅ | AgentService(回合循环+工具执行)+ 9 工具(system_health/dashboard_stats/knowledge_search/knowledge_ask/llm_usage/crawl_status + todo_list/todo_add/todo_done + trigger_workflow)+ Redis会话(房间级多轮,20条上限,30min TTL)+ 限速(每房间30回合/小时)+ 权限分级(readonly/write/danger) |
| 日报系统 DailyReport | ✅ | 后端驱动(DailyReportService)+ 并行采集器(服务状态/LLM用量/PKB/爬取/待办)+ n8n 仅触发；进程内直调 Gateway |
| 日志中心 LogCenter | ✅ | threshold + pattern（正则）告警 + CleanOldEntries 每日调度 + 业务审计 activity_log；全文检索/SSE 交 Loki 外挂（LogQL 查询） |
| 活动日志 ActivityLog | ✅ | 跨模块审计, TraceID 全链路贯穿（HTTP 中间件→eventbus→log_center→Loki） |
| n8n 工作流治理 | ✅ | `internal/n8n_workflows/` JSON 事实源 + `/api/workflows/definitions*` 管理 + 触发/状态 API |
| 运行时配置(Settings KV) | ✅ | 无需重启修改配置 |
| 前端 | ✅ | SolidJS SPA 四域重构（Knowledge/LLM/Logs/Matrix）；Matrix 7→2 页收敛；爬取队列可视化；问答 SSE 流式 |
| Dashboard | ✅ | 聚合统计(爬取/PKB/LLM 费用) |

### n8n 负责

- 定时调度与事件编排(B/K/M/O 四类工作流);重逻辑一律下沉 Bellkeeper
- CrawlQueue 是爬取主链路,n8n 只触发和汇报
- 日报仅触发,数据采集与渲染全在 Bellkeeper

---

## 技术栈

| 层 | 技术 |
|----|------|
| 后端语言 | Go 1.25 + Gin 1.9 |
| ORM | GORM 1.25 + PostgreSQL 16 |
| 消息队列 | NATS JetStream |
| 缓存 | Redis |
| 检索 | Meilisearch |
| 配置 | Viper(YAML + `BELLKEEPER_` 环境变量 + `${ENV_VAR}` 展开) |
| 前端 | SolidJS 1.8 + TailwindCSS 3.4 + Vite 5 |
| Matrix SDK | mautrix-go |
| 认证 | noauth(纯内网);LLM Token 鉴权独立保留 |
| 监控/日志 | Prometheus `/metrics` + Zap（JSON 结构化 stdout）；Loki+Promtail 外挂采集；TraceID 全链路 |
| 部署 | Docker 多阶段构建,SilkSpool `spool` 编排 |
| 迁移 | golang-migrate(显式迁移 005-007) + GORM AutoMigrate |

---

## 代码结构(关键目录)

```
Bellkeeper/
├── cmd/bellkeeper/             # 入口:serve / migrate / pkb-curate / version / eval 子命令
├── internal/
│   ├── app/                    # 手动 DI 装配(DB → repo → service → handler → matrix → eventbus → 后台任务)
│   ├── auth/                   # LLM Token 鉴权(LLMTokenAuth, 依赖 TokenScopeService 接口)
│   ├── config/                 # Viper 配置加载
│   ├── eventbus/               # **1.0 新增** NATS JetStream 一级共享事件总线（6 条 stream + Event 契约）
│   ├── handler/                # HTTP 处理器(按业务域拆分,31 个文件)
│   ├── llmgateway/             # **1.0 重组** LLM 代理池独立包（从 llm/+service/llm_*.go 迁入）
│   │   ├── balance/            #   真实余额 provider
│   │   ├── converter/          #   Gemini/Anthropic 协议转换
│   │   ├── errors/             #   错误码语义化分类(驱动熔断)
│   │   ├── gateway.go          #   Gateway 进程内直调接口(Chat/Rerank)
│   │   └── admin.go            #   TokenScopeService + LLMAdminService 管理面
│   ├── llmclient/              # 内部统一 LLM 调用 SDK(进程外,供 CLI/n8n)
│   ├── pkb/                    # PKB 编排(curator/score/reconstruct/digest/scheduler/eval)
│   ├── n8n_workflows/          # n8n 工作流 JSON 事实源
│   ├── matrix/                 # Matrix 集成
│   │   ├── agent/              #   AgentService(进程内直调 Gateway)
│   │   ├── command/            #   命令路由与处理器
│   │   ├── gateway/            #   mautrix-go 长连接(已异步化 dispatch)
│   │   ├── policy/             #   权限引擎
│   │   └── worker/             #   通知 worker(eventbus 消费)
│   ├── metrics/  middleware/   # Prometheus 指标;Gin 中间件(auth/CORS/ratelimit/logger/trace)
│   ├── model/                  # GORM 模型(26 个文件)+ AutoMigrate
│   ├── pkg/                    # 通用包
│   ├── repository/             # 数据访问层(31 个文件 + 31 个测试文件)
│   ├── router/                 # 路由注册(TraceID 最前链路)
│   └── service/                # 业务逻辑层(含 kb_* 事件 worker + PromptLoader + SafeGo)
├── config/
│   ├── bellkeeper.yaml         # 默认配置(含 NATS 6 条 stream + 爬取阈值)
│   ├── bellkeeper-init.sh      # 环境变量初始化清单
│   ├── pkb/                    # PKB 领域配置 + 提示词包 + eval 评估集
│   ├── prompts/                # **1.0 新增** 非 PKB 专属提示词外置(classify/ask/rule_optimizer + registry)
│   └── rss-sources.json        # RSS 源配置(数据资产)
├── web/                        # SolidJS 前端(四域:知识库/LLM/日志/Matrix)
├── migrations/                 # SQL 迁移
└── doc/                        # 项目文档(1.0 终态结构,旧方案归档 archive/)
```

**分层规则**:`Router → Handler → Service → Repository → Model` 严格单向;已知例外登记在 [ARCHITECTURE-EXCEPTIONS.md](ARCHITECTURE-EXCEPTIONS.md)。

---

## 数据模型(核心表)

| 域 | 表 | 用途 |
|----|----|------|
| 知识 | `article_tags` | 文章元数据 + URL/内容哈希去重 + PKB 处理账本(pkb_status/decision/score) |
| 知识 | `tags` / `dataset_mappings` | 标签体系;Dataset 作为索引分区概念 |
| 爬取 | `crawl_tasks` / `crawl_domain_profiles` | 队列任务（含 crawled 中间态）；域名健康画像（ConsecutiveFailures/HealthScore/IsPaused） |
| 爬取 | `crawl_failures` | 失败档案事件（→ events 域名健康度 worker） |
| 爬取 | `crawl_extraction_rules` / `crawl_rule_trials` | LLM 生成的提取规则及试用记录 |
| RSS | `rss_feeds` | 订阅源(fetch_interval、RSSHub 参数) |
| LLM | `llm_channels` / `llm_model_groups` | 渠道与虚拟模型组(DB 动态配置) |
| LLM | `llm_tokens` / `llm_token_usage_daily` | 调用方 Token 与日用量 |
| LLM | `llm_model_pricing` / `llm_proxy_logs` | 定价;请求日志(cost/cached) |
| LLM | `llm_channel_credentials` / `llm_channel_balance_snapshots` | 加密凭证;余额快照 |
| LLM | `llm_model_rate_limits` / `llm_conversation_bindings` / `llm_alert_events` | 自适应限流学习;会话粘性;告警事件 |
| LLM | `llm_jobs` | 持久任务队列 |
| Matrix | `matrix_rooms` | 房间(enabled/is_admin/auto_discover) |
| Matrix | `matrix_channels` | 通知频道绑定 |
| Matrix | `matrix_commands` | 命令策略覆盖(仅 n8n_webhook 类型) |
| Matrix | `matrix_events` | 事件日志(90 天清理) |
| Matrix | `matrix_notifications` | 通知(dedup_key/severity/聚合窗口) |
| Matrix | `matrix_command_logs` | 命令执行日志(handler_type 含 agent_tool) |
| Matrix | `matrix_sync_state` | 同步游标 |
| Matrix | `matrix_user_roles` | 管理员白名单 |
| 运维 | `activity_logs` / `settings` | 审计;运行时 KV |

---

## 关键链路

### 知识入库与 PKB 漏斗

```
n8n K02(RSS 调度) → Bellkeeper CrawlQueue(事件驱动三 worker 链)
  → crawl-worker: 抓取+提取 → 发 knowledge.crawl.done
  → extract-worker: IngestURL 入库 → 发 knowledge.extract.done
  → index-worker: IndexFile 刷新 Meili → 近实时可见
  → Markdown 落盘 /mnt/knowledge/raw/ → Meili 索引(archive+vault)
pkb-curate(CLI/调度器,经 Gateway 进程内直调 LLM):
  raw → 五维打分 → vault(原子化重构卡片) / archive / discard
pkb-curate digest:领域高分卡片 → 周/月综述
Obsidian ← LiveSync(CouchDB) ← vault/
```

### LLM 代理

进程内调用方（KB/Agent/日报/分类/RSS 规则优化）经 `llmgateway.Gateway` 接口直调，
绕 HTTP+Token 鉴权但保留路由/熔断/限流；外部客户端（CLI/n8n）仍经 `POST /api/llm/v1/*`
OpenAI 兼容协议 + Bearer Token 鉴权。详见 [LLM-GATEWAY-API.md](LLM-GATEWAY-API.md)。

### NATS 事件总线

```
eventbus.Client（一级共享基础设施，禁止绕过直接 nats.Connect）
├── notifications（WorkQueue）→ 通知 worker（Matrix 推送）
├── knowledge（WorkQueue）→ KB 三 worker 链（crawl→extract→index）
├── llm（WorkQueue）→ LLM Job worker（DB 状态机 + 事件驱动）
├── matrix（WorkQueue）→ Agent reply worker（长任务回投）
├── system（Interest/广播）→ 日报/告警/Matrix 多消费者
└── logs（Limits/MaxAge 7d）→ Loki Promtail 外挂采集
```

### Matrix + Agent

```
用户消息 → Gateway(mautrix-go) → Router
  ├─ !命令 → Command Router(权限)→ 内部服务或 n8n webhook
  └─ 普通消息 → AgentService(权限检查→限速→会话加载→LLM+工具循环→回复)
Agent 工具: readonly(所有人) / write(admin) / danger(需确认)
会话: Redis matrix:agent:session:<roomID>, 20条上限, 30min TTL
```

### 日报

```
n8n 定时触发 → POST /api/reports/daily/generate
  → DailyReportService(并行采集: 服务状态/LLM用量/PKB/爬取/待办)
  → 渲染模板 → Matrix 通知
```

---

## 认证机制

1. **API Key**:`X-API-Key`(server.api_key)— 内部服务调用
2. **noauth 模式**:生产为纯内网部署、无公网暴露,`noauth` 模式为预期状态
3. **LLM Token**:`Authorization: Bearer sk-bk-*` — `/api/llm/v1/*` 专用,带模型白名单与配额

---

## 部署

通过 SilkSpool 管理(规则见仓库根 CLAUDE.md §4):

```bash
spool bundle keeper service keeper bellkeeper up   # 单服务代码部署
spool restart keeper bellkeeper                    # 仅重启
spool logs keeper bellkeeper 100                   # 日志
```

容器 `sp-bellkeeper`,内部端口 8080,经 Caddy 反代暴露。
