# Bellkeeper 架构文档

> 更新日期: 2026-06-12
> 状态标注: ✅ 已实施 | 🚧 实施中 | ⚠️ 待清理

---

## 定位与职责

Bellkeeper 是 SilkSpool 的**知识治理中台 + LLM 代理网关 + Matrix 控制平面**,充当 n8n 工作流的"能力增强器"。

**核心价值**:做 n8n 做不好的事——有状态的限速、去重、爬取队列、路由、日志、治理;同时承担 Matrix Gateway、LLM Proxy 和 Agent 三个长连接/交互式服务。知识真相源是 **Obsidian Vault + Markdown 文件**(TrueNAS `data/knowledge/`),Bellkeeper 从中派生索引、搜索与问答。

### Bellkeeper 负责

| 能力 | 状态 | 说明 |
|------|------|------|
| LLM Proxy(多渠道代理) | ✅ | Token 鉴权、虚拟模型组、任务感知分层路由、熔断、会话粘性、自适应限流学习、计费、真实余额、Anthropic/Gemini/Rerank 协议;详见 [LLM_PROXY_GUIDE.md](LLM_PROXY_GUIDE.md) |
| LLM 持久任务队列(`llm_jobs`) | ✅ | 内部批处理调用排队 + 长退避重试 + 幂等键,worker 复用 Proxy 路由/计费/熔断 |
| 个人知识库 PKB | ✅ MVP | `pkb-curate` CLI:打分分流(raw/archive/vault)→ 原子化重构 → digest 综述;提示词外置 `config/pkb/`;详见 [PKB-IMPLEMENTATION.md](PKB-IMPLEMENTATION.md) |
| 文件入库与治理 | ✅ | Trafilatura 主力 + Firecrawl 兜底,Markdown + frontmatter 落盘 `/mnt/knowledge/` |
| 爬取队列 CrawlQueue | ✅ | 持久化任务队列 + Worker 池 + 域名级限速/健康画像 + 错误分类退避 + LLM 爬取规则优化闭环 |
| URL 去重(精确/归一化/模糊 + 内容哈希) | ✅ | DB 内多级匹配 |
| 分类与标签 | ✅ | LLM 驱动分类,标签置信度/规范化/来源记录(tag_source),frontmatter+Meili+DB 三处持久化 |
| RSS 订阅源管理 | ✅ | 源 CRUD、按源 fetch_interval 调度、RSSHub 参数支持、feed 验证 API |
| Meilisearch 检索与问答 | ✅ | `/api/files/search\|ask`,archive+vault 层索引;raw 不进索引 |
| Matrix 控制平面 | ✅ | Gateway(mautrix-go 长连接)+ Command Router + 权限两层制(IsAdmin+IsRoomEnabled)+ 通知网关(NATS)+ 通知聚合去重(5min合并+1h抑制)+ Admin API |
| Agent 系统 | ✅ MVP | AgentService(回合循环+工具执行)+ 6只读工具(system_health/dashboard_stats/knowledge_search/knowledge_ask/llm_usage/crawl_status)+ 3写工具(todo_list/todo_add/todo_done)+ workflow触发(trigger_workflow)+ Redis会话(房间级多轮,20条上限,30min TTL)+ 限速(每房间30回合/小时)+ 权限分级(readonly/write/danger) |
| 日报系统 DailyReport | ✅ | 后端驱动(DailyReportService)+ 并行采集器(服务状态/LLM用量/PKB/爬取/待办)+ n8n 仅触发;O02/K08 已退役 |
| 日志中心 LogCenter | 🚧 | entries/sources/dashboard/alerts 骨架已有;全文检索、trace 关联待做 |
| 活动日志 ActivityLog | ✅ | 跨模块审计 |
| n8n 工作流治理 | ✅ | `internal/n8n_workflows/` JSON 事实源 + `/api/workflows/definitions*` 管理 + 触发/状态 API |
| 运行时配置(Settings KV) | ✅ | 无需重启修改配置 |
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
| 监控/日志 | Prometheus `/metrics` + Zap |
| 部署 | Docker 多阶段构建,SilkSpool `spool` 编排 |
| 迁移 | golang-migrate(显式迁移 005-007) + GORM AutoMigrate |

---

## 代码结构(关键目录)

```
Bellkeeper/
├── cmd/bellkeeper/             # 入口:serve / migrate / pkb-curate / version 子命令
├── internal/
│   ├── app/                    # 手动 DI 装配(DB → repo → service → handler → matrix → 后台任务)
│   ├── auth/                   # LLM Token 鉴权(LLMTokenAuth)
│   ├── config/                 # Viper 配置加载
│   ├── handler/                # HTTP 处理器(按业务域拆分,31 个文件)
│   ├── llm/                    # LLM Proxy 子系统
│   │   ├── balance/            #   真实余额 provider(deepseek/moonshot/newapi/aliyun)
│   │   ├── converter/          #   Gemini 协议转换
│   │   └── errors/             #   错误码语义化分类(驱动熔断)
│   ├── llmclient/              # 内部统一 LLM 调用 SDK(CallerID/TaskType/重试/Tools)
│   ├── pkb/                    # PKB 编排(curator/score/reconstruct/digest/scheduler)
│   ├── n8n_workflows/          # n8n 工作流 JSON 事实源(20 个工作流)
│   ├── matrix/                 # Matrix 集成
│   │   ├── agent/              #   AgentService(回合循环+工具执行+会话+限速+权限)
│   │   ├── command/            #   命令路由与处理器(!问/!搜/!待办/!reset)
│   │   ├── gateway/            #   mautrix-go 长连接(sync/client)
│   │   ├── infra/              #   基础设施适配器(NATS/Redis)
│   │   ├── policy/             #   权限引擎(IsAdmin/IsRoomEnabled)
│   │   └── worker/             #   通知 worker
│   ├── metrics/  middleware/   # Prometheus 指标;Gin 中间件(auth/CORS/ratelimit/logger)
│   ├── model/                  # GORM 模型(26 个文件)+ AutoMigrate
│   ├── pkg/                    # 通用包(response/defaults/errors/crypto/meili/urlutil/sanitizer/textutil/httpclient/envutil)
│   ├── repository/             # 数据访问层(31 个文件 + 31 个测试文件)
│   ├── router/                 # 路由注册(408 行)
│   └── service/                # 业务逻辑层(49 个文件 + 22 个测试文件)
├── config/
│   ├── bellkeeper.yaml         # 默认配置(渠道清单仅作 seed,运行态以 DB 为准)
│   └── pkb/                    # PKB 领域配置 + 提示词包(domains.yaml + prompts/*.md + registry.yaml)
├── web/                        # SolidJS 前端(四大域:Knowledge / LLM / Logs / Matrix)
├── migrations/                 # SQL 迁移(001-007,主链 AutoMigrate + 显式 runtime 表迁移)
└── doc/                        # 项目文档(本目录)
```

**分层规则**:`Router → Handler → Service → Repository → Model` 严格单向;已知例外登记在 [ARCHITECTURE-EXCEPTIONS.md](ARCHITECTURE-EXCEPTIONS.md)。

---

## 数据模型(核心表)

| 域 | 表 | 用途 |
|----|----|------|
| 知识 | `article_tags` | 文章元数据 + URL/内容哈希去重 + PKB 处理账本(pkb_status/decision/score) |
| 知识 | `tags` / `dataset_mappings` | 标签体系;Dataset 作为索引分区概念 |
| 爬取 | `crawl_tasks` / `crawl_domain_profiles` | 队列任务;域名健康画像与限速 |
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
n8n K02(RSS 调度) → Bellkeeper CrawlQueue → 提取(Trafilatura/Firecrawl/规则) → 分类打标
  → Markdown 落盘 /mnt/knowledge/raw/ → Meili 索引(archive+vault)
pkb-curate(CLI/调度器,经 llm_jobs 队列调 LLM):
  raw → 五维打分 → vault(原子化重构卡片) / archive / discard
pkb-curate digest:领域高分卡片 → 周/月综述
Obsidian ← LiveSync(CouchDB) ← vault/
```

### LLM 代理

见 [LLM_PROXY_GUIDE.md](LLM_PROXY_GUIDE.md);内部消费者(classify / knowledge-ask / rule_optimizer / pkb / agent)统一经 `internal/llmclient` 或 `llm_jobs` 队列调用 `/api/llm/v1`,提示词分析见 [LLM-PROMPT-AGENT-REVIEW.md](LLM-PROMPT-AGENT-REVIEW.md)。

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
