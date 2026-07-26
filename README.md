# Bellkeeper

> **Bellkeeper（钟守者）** 是 SilkSpool 的知识治理中台 + LLM 代理网关 + Matrix 控制平面 + 事件驱动平台。**1.0 已 GA、稳定运行**。
> 运维一律走 SilkSpool `spool` CLI（禁止裸 ssh/docker/rsync），规则见 [CLAUDE.md](CLAUDE.md) §3。

它承担 n8n 工作流做不了的有状态工作：长连接（Matrix sync、LLM 代理）、持久化队列（爬取、解析）、分类与去重、Meilisearch 检索、文件治理。知识真相源是 **Obsidian Vault + Markdown 文件**（`/mnt/knowledge/`），Bellkeeper 从中派生索引、搜索与问答；检索由 Meilisearch 承担，RAGFlow 已完全退役。

> **部署形态（2026-07-25 应用/数据分离）**：keeper(192.168.7.230) 只跑应用（bellkeeper/n8n/rsshub/memos）；PostgreSQL/Meilisearch/Redis/NATS/CouchDB 迁至数据层 silkdata(192.168.7.231)；firecrawl 在 knowledge(192.168.7.220)；观测栈（Prometheus+Loki+Grafana）在 silkdata。完整拓扑见 [doc/STATUS.md](doc/STATUS.md)。

## 功能概览

### 知识管线（文件优先）

- **文件入库** — Trafilatura 主提取 + Firecrawl 兜底，统一落地为带 YAML frontmatter 的 Markdown 写入 `/mnt/knowledge/raw|working`
- **爬取队列 (CrawlQueue)** — 持久化任务队列 + Worker 池 + 熔断 + 反爬，承接 K01/K02 工作流的入库请求
- **URL 去重** — DB 内三级匹配（精确/归一化/模糊），不再依赖 RAGFlow
- **分类与标签** — LLM 驱动分类，标签置信度/规范化/来源记录，frontmatter + Meilisearch + DB 三处持久化
- **个人知识库 PKB** — `bellkeeper pkb-curate` CLI：raw 层 AI 打分分流（vault/archive/discard）→ 高分原子化重构成 Obsidian 卡片 → 知识骨架归位 + 缺口填充 + 资讯库 → 领域 digest 综述；提示词外置 `config/pkb/`
- **Meilisearch 检索** — archive/vault 层索引 → `/api/files/search|ask` 提供搜索与 RAG 问答（raw 层不进索引）
- **文件浏览 API** — `/api/knowledge/files/tree|list|read` 保留（K01 等调用）；前端不做文件浏览，Vault 浏览归 Obsidian（见 ADR-0006 前端边界）

### LLM 代理池

- **多渠道路由** — SiliconFlow、DashScope、DeepSeek、Moonshot、Kimi Code、new-api 系、Gemini 等，DB 动态配置 + 热重载
- **虚拟模型组** — `pool-chat-free` / `pool-chat-balanced` / `pool-summary` / `pool-pkb` 等，任务感知分层路由（coding 按复杂度选 tier）、`least_latency` / `balance_aware` 多策略
- **协议转换** — OpenAI 兼容入口，渠道侧转换 Anthropic（含 tool use 与流式）/ Gemini；`/v1/rerank` 端点
- **Token 体系与计费** — 调用方专用 Token（模型白名单 + 配额）、定价表 + cached tokens 折扣计费、用量聚合
- **真实余额同步** — DeepSeek / Moonshot / new-api 系 / 阿里云 BSS 余额拉取，估算 vs 真实对比
- **限速与熔断** — 令牌桶 RPM/RPD + 429 自适应限流学习；错误码语义化熔断（配额/认证/限流分类）
- **会话粘性** — `X-Conversation-ID` 绑定渠道保护 prompt cache；`X-Task-Key` 任务级粘性
- **持久任务队列** — `llm_jobs` 表 + worker，内部批处理（PKB/分类/问答）统一排队与长退避重试
- **告警聚合** — 5min 合并 + 1h 去重 → Matrix ops 频道

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

> **RAGFlow 已完全退役**：Go / 前端代码层零引用，仅个别 n8n 工作流 JSON 残留历史字段。

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
| 认证 | noauth（纯内网）+ API Key + LLM Token | - |
| 配置 | Viper + Cobra | - |
| 日志 | Zap (结构化) | 1.26 |
| 监控 | Prometheus + Loki + Grafana（M6，@silkdata）| - |
| 容器 | Docker (多阶段构建) | Alpine |

## 项目结构

```
bellkeeper/
├── cmd/bellkeeper/                 # Go 后端入口
│   └── main.go                     #   serve / migrate / pkb-curate / version 子命令
│
├── internal/                       # Go 内部代码（不允许外部项目导入）
│   ├── app/                        # 应用装配 (DB → repo → service → handler → matrix → 后台任务)
│   ├── auth/                       # 认证相关（Authelia Forward Auth 解析、API Key、LLM Token 校验）
│   ├── config/                     # Viper 配置加载与结构定义
│   ├── eventbus/                   # NATS JetStream 一级共享事件总线（6 stream + Event 契约）
│   ├── handler/                    # HTTP 处理器（按业务域拆分）
│   ├── llmgateway/                 # LLM 代理池独立包（Gateway 进程内直调、协议转换、余额、错误分类）
│   ├── llmclient/                  # 内部统一 LLM 调用 SDK（进程外 CLI/n8n）
│   ├── pkb/                        # PKB 编排（curator / score / reconstruct / digest / skeleton / scheduler）
│   ├── n8n_workflows/              # n8n 工作流 JSON 事实源
│   ├── matrix/                     # Matrix 集成模块
│   │   ├── command/                #   命令 parser / router / handlers
│   │   ├── gateway/                #   mautrix-go sync 客户端
│   │   ├── infra/                  #   Redis / NATS 连接管理
│   │   ├── notify/                 #   通知网关
│   │   ├── policy/                 #   权限引擎
│   │   ├── queue/                  #   消息队列
│   │   ├── registry/               #   Matrix 房间/频道注册表
│   │   └── worker/                 #   后台 worker
│   ├── metrics/                    # Prometheus 指标收集
│   ├── middleware/                 # Gin 中间件（认证、CORS、限速、日志）
│   ├── model/                      # GORM 模型定义 + AutoMigrate
│   ├── pkg/                        # 内部通用包（不允许依赖上层）
│   │   ├── crypto/                 #   加密工具
│   │   ├── defaults/               #   业务常量与默认值
│   │   ├── errors/                 #   错误定义
│   │   ├── httpclient/             #   HTTP 客户端封装
│   │   ├── meili/                  #   Meilisearch 客户端
│   │   ├── response/               #   统一 HTTP 响应格式
│   │   ├── sanitizer/              #   输入清洗
│   │   ├── urlutil/                #   URL 处理工具
│   │   └── validator/              #   校验工具
│   ├── repository/                 # GORM 数据访问层（Repository 模式）
│   ├── router/                     # 路由分组注册
│   └── service/                    # 业务逻辑层（Service 模式）
│
├── web/                            # 前端 (SolidJS + Vite)
│   ├── src/
│   │   ├── api/                    #   类型安全的 API 客户端
│   │   ├── components/             #   通用组件 (Layout / Toast / Modal)
│   │   ├── hooks/                  #   SolidJS 自定义 hooks
│   │   ├── pages/                  #   页面路由组件 (Knowledge / LLM / Logs / Matrix)
│   │   ├── stores/                 #   全局状态管理
│   │   ├── types/                  #   TypeScript 类型定义
│   │   └── utils/                  #   前端工具函数
│   ├── dist/                       #   构建产物（由 Vite 生成，.gitignore 忽略）
│   └── index.html
│
├── config/                         # 配置文件
│   ├── bellkeeper.yaml             #   默认配置（可被 .env / .local.yaml 覆盖；LLM 渠道清单仅作首次 seed）
│   └── pkb/                        #   PKB 领域配置 + 提示词包（domains.yaml + prompts/ + registry.yaml）
│
├── docker/                         # Docker 构建与编排
│   ├── Dockerfile                  #   多阶段构建（生产用）
│   └── docker-compose.yml          #   本地开发依赖（Postgres 等）
│
├── migrations/                     # golang-migrate 显式迁移（删除类）+ GORM AutoMigrate（建新表）
│   ├── 001_init.up.sql
│   └── 001_init.down.sql
│
├── scripts/                        # 辅助脚本（Python，独立运行）
│   ├── scan_dups.py                #   重复文件扫描
│   └── trafilatura_extract.py      #   网页内容提取
│
├── doc/                            # 项目文档（ architecture / guides / roadmap ）
│
├── bin/                            # 构建输出目录（.gitignore 忽略）
│
├── go.mod / go.sum                 # Go 模块定义
├── Makefile                        # 构建、测试、开发任务
└── README.md
```

## 前端导航 (web/)

四大核心系统域:

- **Knowledge**: `/knowledge/overview`（总览）/ `/knowledge/skeleton`（知识骨架）/ `/knowledge/search` / `/knowledge/ask`（问答）+ 采集子分区 `/rss` `/tags`（数据集前端已退役）
- **LLM**: `/llm` 总览 + `/llm/channels` + `/llm/groups-routing` + `/llm/usage-billing` + `/llm/logs-alerts`
- **Logs**: `/logs` + `/logs/dashboard|sources|alerts`
- **Matrix**: `/matrix`（总览）+ `/matrix/console`（控制台，7→2 页收敛）

## 外部依赖

```
        Caddy (反向代理, TLS) — 内网 noauth
                    │
   keeper (.230) 应用层                          silkdata (.231) 数据层
┌───────────────────────────┐   extra_hosts  ┌───────────────────────────────┐
│  Bellkeeper Backend        │   别名连数据层  │  PgSQL · Meili · NATS · Redis  │
│  Handler→Service→Repo      │──────────────▶│  CouchDB                       │
│  + SolidJS 前端（嵌入）     │               │  ──────────────────────────── │
│  n8n · rsshub · memos      │               │  Prometheus · Loki · Grafana  │
└─────────────┬─────────────┘               └───────────────────────────────┘
              │ firecrawl 端点(3002)
              ▼
   knowledge (.220) firecrawl          /mnt/knowledge（Markdown 知识真相源）
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
| ANY | `/api/llm/v1/*` | OpenAI 兼容代理（LLM Token 鉴权；含 `/v1/rerank`；Anthropic/Gemini 为渠道侧转换） |
| GET | `/api/llm/health` | 渠道健康状态 |
| GET | `/api/llm/groups/status` | 模型组状态 |
| CRUD | `/api/llm/config/channels` `/api/llm/config/groups` | 渠道/模型组配置（DB，热重载） |
| CRUD | `/api/llm/tokens` `/api/llm/pricing` | Token 与定价管理 |
| GET | `/api/llm/usage` `/api/llm/balances` `/api/llm/rate-limits` `/api/llm/alerts` | 用量/余额/限流学习/告警 |

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
# 单服务代码部署（须重建镜像，见 CLAUDE.md §3）
spool bundle keeper service keeper bellkeeper up
# 仅重启 / 状态 / 日志
spool restart keeper bellkeeper
spool service keeper status bellkeeper
spool logs keeper bellkeeper 100
```

线上 Bellkeeper 经 Caddy 暴露：`https://bellkeeper.singll.net`（纯内网解析，noauth 模式）。数据层在 silkdata、观测栈见 [doc/STATUS.md](doc/STATUS.md)。

### 本地开发

**前置要求**
- Go 1.25+
- Node.js 20+ + pnpm
- Docker + Docker Compose（用于本地数据库）
- air（可选，用于后端热重载）：`go install github.com/cosmtrek/air@latest`

**环境准备**
```bash
# 1. 安装 Go 依赖
make deps

# 2. 安装前端依赖
cd web && pnpm install && cd ..

# 3. 启动本地数据库（docker-compose.yml 包含 Postgres）
make docker-up

# 4. 数据库迁移（GORM AutoMigrate）
make migrate
```

**启动开发服务器**
```bash
make dev          # 后端 air 热重载（端口 8080）
make dev-frontend # 前端 Vite（端口 3000）
```

### 构建

```bash
make build           # 构建全部（后端 + 前端）
make build-backend   # 仅构建后端 → 输出到 bin/bellkeeper
make build-frontend  # 仅构建前端 → 输出到 web/dist/
make docker-build    # Docker 镜像构建
```

> **构建产物规范**
> - 后端二进制：`bin/bellkeeper`（由 Makefile 统一输出，.gitignore 忽略）
> - 前端静态文件：`web/dist/`（由 Vite 生成，.gitignore 忽略）
> - 生产镜像：`bellkeeper:latest`

### 测试

```bash
make test            # 运行所有 Go 测试
make test-coverage   # 生成覆盖率报告（coverage.html）
```

### 代码质量

```bash
make fmt             # 格式化 Go 代码
make lint            # 运行 golangci-lint（需提前安装）
make all             # fmt + lint + test + build
```

### 常用维护命令

```bash
make clean           # 删除构建产物（bin/、coverage.out 等）
make docker-up       # 启动本地依赖容器（Postgres）
make docker-down     # 停止本地依赖容器
make migrate         # 执行数据库迁移
make swagger         # 生成 Swagger 文档（输出到 api/docs/）
```

## 配置

主配置：`config/bellkeeper.yaml`。所有项支持 `BELLKEEPER_` 前缀的环境变量覆盖。运行时配置（API Key、渠道、模型组、Matrix 凭证）统一通过 Web UI 写入 `system_settings` 表 + LLM Proxy 专用表，运行时热重载。

关键环境变量（线上写在 `hosts/keeper/.env`）：

| 变量 | 用途 |
|------|------|
| `DB_PASSWORD` | PostgreSQL 密码 |
| `BELLKEEPER_API_KEY` | API 内部调用 Key |
| `BELLKEEPER_CREDENTIAL_KEY` | 渠道凭证 AES-256-GCM 加密密钥（未设则降级为明文存储并启动告警）|
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

生产为**纯内网部署、无公网暴露**，运行在 `noauth` 模式（`BELLKEEPER_SERVER_MODE=noauth`），无需登录。三种机制：

| 机制 | 用途 |
|------|------|
| noauth | 纯内网默认，无需认证（预期最终状态） |
| API Key（`X-API-Key`） | 内部服务调用 |
| LLM Token（`Authorization: Bearer sk-bk-*`） | `/api/llm/v1/*` 专用，带模型白名单与配额 |

> 历史：早期经 Caddy + Authelia Forward Auth（`Remote-User` 等 header）注入身份；纯内网化后移除，改 noauth。

## License

MIT
