# Bellkeeper

**Bellkeeper (钟守者)** 是 SilkSpool 的知识治理中台 + LLM 代理网关 + Matrix 控制平面。它承担 n8n 工作流做不了的有状态工作：长连接（Matrix sync、LLM 代理）、持久化队列（爬取、解析）、分类与去重、Meilisearch 检索、文件治理。

知识真相源是 **Obsidian Vault + Markdown 文件**（落地在 TrueNAS `data/knowledge/`），Bellkeeper 从中派生索引、搜索与问答能力，不再以 RAGFlow 为中心。

## 功能概览

### 知识管线（文件优先）

- **文件入库** — Trafilatura 主提取 + Firecrawl 兜底，统一落地为带 YAML frontmatter 的 Markdown 写入 `/mnt/knowledge/raw|working`
- **爬取队列 (CrawlQueue)** — 持久化任务队列 + Worker 池 + 熔断 + 反爬，承接 K01/K02 工作流的入库请求
- **URL 去重** — DB 内三级匹配（精确/归一化/模糊），不再依赖 RAGFlow
- **分类与标签** — LLM 驱动分类（SiliconFlow Qwen3-8B），标签作为分区元数据
- **Meilisearch 检索** — 文件内容索引 → `/api/files/search|ask` 提供搜索与 RAG 问答
- **文件浏览** — `/api/knowledge/files/tree|list|read` 为前端提供 Obsidian Vault 只读视图

### LLM 代理池

- **多渠道路由** — 7+ 预配置渠道（SiliconFlow、DashScope、DeepSeek、Kimi、Qwen、new-api 等）
- **虚拟模型组** — `pool-chat-free` / `pool-chat-balanced` / `pool-summary`，跨渠道智能选路
- **协议双栈** — OpenAI 兼容 + Anthropic 协议（流式），单点对接两套 SDK 生态
- **令牌桶限速** — 每渠道 RPM/RPD 限制
- **熔断器** — 连续 5 次失败自动熔断，120 秒冷却后半开探测
- **粘性路由** — `X-Task-Key` / `X-Caller-ID` 绑定同一渠道
- **DB 动态配置** — 渠道/模型组 CRUD + 热重载，运行态健康/限速不丢失

### Matrix 控制平面

- **Matrix Gateway** — 基于 mautrix-go 的长连接 sync，取代原 n8n 轮询
- **Command Router** — 前缀命令（`!help`、`!问`、`!搜`、`!待办`），权限引擎，转发到 n8n M01-M03 webhook
- **通知网关** — NATS JetStream 队列 + 频道路由 + 重试逻辑，n8n 工作流统一通过 `/api/matrix/notify` 推送
- **Admin API + 前端** — 房间/频道/命令/角色/通知规则/事件日志的 Web UI 管理

### 运维能力

- **日志中心 (LogCenter)** — 日志条目、源管理、仪表盘、告警规则
- **活动日志 (ActivityLog)** — 跨模块审计 (`crawl` / `classify` / `ingest` 等)
- **Dashboard** — 系统概览、服务状态、LLM Proxy 健康、模型组状态
- **健康检查** — `/api/health/live|ready|detailed` + 外部服务连通性
- **Prometheus Metrics** — `/metrics` 端点
- **n8n 工作流集成** — 列表/激活/触发/执行历史的统一 API

> **遗留 RAGFlow 兼容层**：`handler/ragflow.go` + `service/ragflow_*.go` 还在编译，主链已不调用，等待二阶段清理。详见 `../SilkSpool/doc/ROADMAP.md`。

## 技术栈

| 层级 | 技术 | 版本 |
|------|------|------|
| 后端框架 | Go + Gin | Go 1.25, Gin 1.9 |
| ORM | GORM | 1.25 |
| 数据库 | PostgreSQL | 16 |
| 消息队列 | NATS JetStream | latest |
| 缓存 | Redis | latest |
| 检索引擎 | Meilisearch | latest |
| Matrix SDK | mautrix-go | latest |
| 前端框架 | SolidJS + TypeScript | SolidJS 1.8 |
| UI 样式 | TailwindCSS | 3.4 |
| 构建工具 | Vite | 5.x |
| 认证 | Authelia (Forward Auth) + API Key | - |
| 配置 | Viper + Cobra | - |
| 日志 | Zap (结构化) | 1.26 |
| 监控 | Prometheus | - |
| 容器 | Docker (多阶段构建) | Alpine |

## 项目结构

```
bellkeeper/
├── cmd/bellkeeper/main.go          # 入口 (serve / migrate / version)
│
├── internal/
│   ├── app/                        # 应用装配 (DB → repo → service → handler → matrix → 后台任务)
│   ├── router/                     # 路由分组注册
│   ├── handler/                    # HTTP 处理器 (26 个，按业务域拆分)
│   ├── service/                    # 业务逻辑 (30+ 个 service)
│   ├── repository/                 # GORM 数据访问
│   ├── model/                      # GORM 模型 + AutoMigrate
│   ├── matrix/                     # Matrix 集成模块
│   │   ├── gateway/                #   mautrix-go sync 客户端
│   │   ├── command/                #   命令 parser / router / handlers
│   │   ├── infra/                  #   Redis / NATS
│   │   ├── notify/                 #   通知网关
│   │   ├── policy/                 #   权限引擎
│   │   ├── queue/                  #   消息队列
│   │   └── worker/                 #   后台 worker
│   ├── middleware/                 # 认证 / CORS / 限速 / 日志
│   ├── metrics/                    # Prometheus 指标
│   └── pkg/                        # 内部工具包 (httpclient / meili / response / errors / defaults)
│
├── web/                            # 前端 (SolidJS + Vite)
│   ├── src/
│   │   ├── api/index.ts            # 类型安全的 API 客户端
│   │   ├── types/index.ts          # TypeScript 类型定义
│   │   ├── components/             # Layout / Toast / Modal
│   │   └── pages/                  # 四大核心域: Knowledge / LLM / Logs / Matrix
│   └── vite.config.ts
│
├── config/bellkeeper.yaml          # 默认配置
├── docker/Dockerfile               # 多阶段构建 (生产用)
├── doc/                            # 项目文档 (见 doc/README.md)
├── go.mod / go.sum
├── Makefile
└── README.md
```

## 前端导航 (web/)

四大核心系统域 (2026-04 重构后):

- **Knowledge**: `/knowledge/files` (Vault 浏览) / `/knowledge/search` / `/knowledge/ask` (RAG 问答) + `/rss` `/tags` `/datasets`
- **LLM**: `/llm` 仪表盘 + `/llm/channels|groups|config|logs`
- **Logs**: `/logs` + `/logs/dashboard|sources|alerts|parse-tasks`
- **Matrix**: `/matrix` + `rooms|channels|commands|notifications|events|command-logs`

## 外部依赖

```
Caddy (反向代理) + Authelia (Forward Auth)
        │
        ▼
┌──────────────────────────────────────────┐
│         Bellkeeper Backend                │
│  Middleware → Handler → Service → Repo    │
│       │           │                       │
│       ▼           ▼                       │
│  SolidJS 前端（嵌入）                      │
└──┬─────┬─────┬─────┬──────┬──────┬───────┘
   │     │     │     │      │      │
┌──▼─┐ ┌─▼──┐ ┌▼──┐ ┌▼────┐ ┌▼───┐ ┌▼────┐
│PgSQL│ │Meili│ │NATS│ │Redis│ │ n8n│ │Matrix│
└─────┘ └─────┘ └────┘ └─────┘ └────┘ └─────┘
                                  │
                          ┌───────▼─────────┐
                          │ /mnt/knowledge  │
                          │ (TrueNAS NFS)   │
                          └─────────────────┘
```

## API 端点速览

完整 API 参考见 [doc/API.md](doc/API.md)。

### 知识库

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/files/ingest/url` | URL 提取入库 |
| GET | `/api/files/search` | Meilisearch 全文搜索 |
| POST | `/api/files/ask` | RAG 问答 |
| GET | `/api/files/stats` | 索引统计 |
| POST | `/api/files/rebuild` | 重建索引 |
| GET | `/api/knowledge/files/tree\|list\|read` | Vault 文件浏览 |

### 爬取队列

| 方法 | 路径 | 说明 |
|------|------|------|
| GET/POST | `/api/crawl/queue` | 任务列表 / 入队 |
| POST | `/api/crawl/queue/:id/retry\|cancel` | 任务控制 |
| GET | `/api/crawl/stats` | 队列统计 |

### LLM Proxy

| 方法 | 路径 | 说明 |
|------|------|------|
| ANY | `/api/llm/v1/*` | OpenAI 兼容代理 |
| ANY | `/api/llm/anthropic/*` | Anthropic 协议代理 |
| GET | `/api/llm/health` | 渠道健康状态 |
| GET | `/api/llm/groups/status` | 模型组状态 |
| CRUD | `/api/llm/channels` `/api/llm/groups` | 配置管理 |

### Matrix

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/matrix/notify` | 发送通知（n8n 主入口）|
| CRUD | `/api/matrix/admin/{rooms,channels,commands,roles,notifications}` | 后台管理 |

### n8n 工作流

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/workflows/status` | 工作流列表 |
| POST | `/api/workflows/:id/activate\|deactivate` | 激活控制 |
| POST | `/api/workflows/trigger/:name` | 触发 |
| GET | `/api/workflows/executions` | 执行历史 |

## 部署

### 生产 (SilkSpool 集成)

```bash
cd /home/ubuntu/SilkSpool

# 首次部署
./spool.sh bundle keeper setup keeper

# 单独更新 Bellkeeper
./spool.sh bundle keeper service keeper bellkeeper up

# 状态 / 日志
./spool.sh status keeper bellkeeper
./spool.sh logs keeper bellkeeper 100
```

线上 Bellkeeper 通过 Caddy + Authelia 暴露：`https://bellkeeper.singll.net`。

### 本地开发

```bash
# Go 依赖
make deps

# 前端依赖
cd web && pnpm install && cd ..

# 启动数据库 (docker-compose.yml 中包含 Postgres)
make docker-up

# 数据库迁移（GORM AutoMigrate）
make migrate

# 后端 + 前端开发服务器
make dev          # 后端 air 热重载
make dev-frontend # 前端 Vite
```

### 常用命令

```bash
make build           # 构建前后端
make test            # 运行测试
make fmt / make lint # 格式化 / 检查
```

## 配置

主配置：`config/bellkeeper.yaml`。所有项支持 `BELLKEEPER_` 前缀的环境变量覆盖。运行时配置（API Key、渠道、模型组、Matrix 凭证）统一通过 Web UI 写入 `system_settings` 表 + LLM Proxy 专用表，运行时热重载。

关键环境变量（线上写在 `hosts/keeper/.env`）：

| 变量 | 用途 |
|------|------|
| `DB_PASSWORD` | PostgreSQL 密码 |
| `BELLKEEPER_API_KEY` | API 内部调用 Key |
| `BELLKEEPER_MATRIX_*` | Matrix Gateway 凭证 |
| `LLM_*_API_KEY` | LLM Proxy 渠道凭证（DashScope、SiliconFlow、DeepSeek、Kimi、Qwen、new-api 等）|
| `MEILI_MASTER_KEY` | Meilisearch 主密钥 |

新增环境变量必须同步在 `bellkeeper-init.sh` 中 `export` 列入运行时上下文。

## 开发规范

详见 [doc/DEVELOPMENT-GUIDE.md](doc/DEVELOPMENT-GUIDE.md)。要点：

- **分层单向**：Router → Handler → Service → Repository → Model
- **统一响应**：所有 Handler 必须使用 `internal/pkg/response` 包
- **不直接 c.JSON**：例外是代理端点直接透传上游响应
- **常量集中**：所有硬编码值收入 `internal/pkg/defaults`
- **手动 DI**：构造函数注入，无 DI 容器，无全局变量，无 `init()`
- **错误处理**：永不忽略 error，Service 层返回错误由 Handler 决定 HTTP 状态码
- **前端配色**：深炭灰主题（不用纯黑），层级间必须有可辨识对比度差异
- **不过度设计**：三行重复优于一个不必要的抽象

## 认证

Bellkeeper 使用 **Authelia Forward Auth**。Caddy 反向代理把认证用户信息通过 HTTP Header 传给后端：

| Header | 说明 |
|--------|------|
| `Remote-User` | 用户名（必须）|
| `Remote-Email` | 邮箱 |
| `Remote-Name` | 显示名 |
| `Remote-Groups` | 用户组（逗号分隔）|

`debug` 模式下未提供认证 Header 时自动使用 `dev-user`。

## License

MIT
