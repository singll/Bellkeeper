# Bellkeeper

**Bellkeeper (钟守者)** 是一个知识管理中台 + LLM 代理网关，充当 n8n 工作流的"能力增强器"。集成 RagFlow 向量知识库、Matrix 通信平台、Memos 待办系统和 n8n 工作流引擎，提供从信息采集到知识入库的完整管理链路。

## 功能概览

### 核心功能

- **标签系统** — 统一的知识分类标签，支持自定义颜色，LLM 驱动智能分类
- **知识库映射** — 将标签映射到 RagFlow Dataset，实现智能路由上传
- **RSS 订阅** — RSS Feed 管理，可配置抓取间隔和标签关联
- **文件入库** — URL 内容提取（Trafilatura + Firecrawl）、YAML frontmatter 生成、元数据管理
- **URL 去重** — 三级匹配策略（精确/归一化/模糊）

### LLM Proxy 代理池

- **多渠道路由** — 7+ 预配置渠道（SiliconFlow、DashScope、DeepSeek 等）
- **虚拟模型组** — 逻辑模型映射到多个真实渠道（pool-chat-free、pool-chat-balanced、pool-summary）
- **令牌桶限速** — 每渠道 RPM/RPD 限制
- **熔断器** — 连续失败自动熔断，冷却后半开探测
- **粘性路由** — 任务绑定同一渠道

### Matrix 控制平面

- **Matrix 机器人** — 前缀命令模式（`!help`、`!待办`、`!问`、`!搜`）
- **通知网关** — 频道路由 + NATS 队列 + 重试逻辑
- **房间/频道管理** — Web UI 管理 Matrix 房间和通知频道
- **命令路由** — 可扩展的命令处理器，支持权限控制

### 集成能力

- **RagFlow 集成** — 文档上传、智能路由上传、解析队列、文档管理
- **n8n 工作流** — 查看/激活/停用工作流、执行历史、手动触发
- **Memos 集成** — 待办管理（通过 Matrix 命令或 n8n 工作流）
- **系统设置** — Web UI 动态配置 API Key、功能开关等

### 监控与运维

- **Dashboard** — 系统概览、服务状态、LLM Proxy 健康状态
- **健康检查** — 外部服务连通性监测、Liveness/Readiness 探针
- **Prometheus Metrics** — `/metrics` 端点
- **活动日志** — 跨模块操作审计

## 技术栈

| 层级 | 技术 | 版本 |
|------|------|------|
| **后端框架** | Go + Gin | Go 1.25, Gin 1.9 |
| **ORM** | GORM | 1.25 |
| **数据库** | PostgreSQL | 16 |
| **消息队列** | NATS | - |
| **缓存** | Redis | - |
| **Matrix SDK** | mautrix-go | - |
| **前端框架** | SolidJS + TypeScript | SolidJS 1.8 |
| **UI 样式** | TailwindCSS | 3.4 |
| **构建工具** | Vite | 5.x |
| **包管理** | Go Modules / pnpm | - |
| **认证** | Authelia (Forward Auth) + API Key | - |
| **配置** | Viper + Cobra | - |
| **日志** | Zap (结构化日志) | 1.26 |
| **监控** | Prometheus | - |
| **容器** | Docker (多阶段构建) | Alpine |

## 项目结构

```
bellkeeper/
├── cmd/bellkeeper/
│   └── main.go                    # 入口 (serve / migrate / version)
│
├── internal/
│   ├── config/                    # 配置管理 (Viper)
│   │   └── config.go
│   │
│   ├── router/                    # 路由注册
│   │   └── router.go
│   │
│   ├── handler/                   # HTTP 处理器
│   │   ├── handler.go             #   Handler 注册中心
│   │   ├── health.go              #   健康检查
│   │   ├── tag.go                 #   标签 CRUD
│   │   ├── rss.go                 #   RSS 订阅 CRUD
│   │   ├── dataset.go             #   知识库映射 CRUD + 智能推荐
│   │   ├── ragflow.go             #   RagFlow 文档管理
│   │   ├── llm_proxy.go           #   LLM Proxy 代理池管理
│   │   ├── file_ingestion.go      #   文件入库 API
│   │   ├── matrix_notify.go       #   Matrix 通知 API
│   │   ├── matrix_admin.go        #   Matrix 管理 API
│   │   ├── todotxt_export.go      #   todo.txt 导出
│   │   ├── search.go              #   全局搜索
│   │   ├── setting.go             #   系统设置
│   │   ├── workflow.go            #   n8n 工作流管理
│   │   └── activity_log.go        #   活动日志查询
│   │
│   ├── service/                   # 业务逻辑层
│   │   ├── service.go             #   Service 注册中心
│   │   ├── health.go              #   服务健康检查
│   │   ├── tag.go                 #   标签业务
│   │   ├── rss.go                 #   RSS 业务
│   │   ├── dataset.go             #   知识库映射 + 标签路由
│   │   ├── ragflow.go             #   RagFlow API 集成
│   │   ├── ragflow_parse_queue.go #   RAGFlow 解析队列
│   │   ├── llm_proxy.go           #   LLM 多渠道代理
│   │   ├── llm_model_group.go     #   虚拟模型组
│   │   ├── llm_channel_health.go  #   熔断器
│   │   ├── classify.go            #   LLM 分类
│   │   ├── file_ingestion.go      #   文件入库服务
│   │   ├── extractor.go           #   内容提取器
│   │   ├── notification.go        #   通知服务
│   │   ├── notification_sender.go #   Matrix 通知发送
│   │   ├── command.go             #   Matrix 命令路由
│   │   ├── admin.go               #   Matrix 管理服务
│   │   ├── workflow.go            #   n8n API 调用
│   │   ├── setting.go             #   配置管理
│   │   └── activity_log.go        #   活动日志
│   │
│   ├── repository/                # 数据访问层
│   │   ├── repository.go          #   Repository 注册中心
│   │   ├── tag.go, rss.go, dataset.go, setting.go ...
│   │   └── matrix_*.go            #   Matrix 实体仓储
│   │
│   ├── model/                     # 数据模型 (GORM)
│   │   ├── db.go                  #   数据库初始化 + AutoMigrate
│   │   ├── tag.go, rss_feed.go, dataset_mapping.go ...
│   │   ├── llm_*.go               #   LLM Proxy 模型
│   │   └── matrix.go              #   Matrix 实体模型
│   │
│   ├── matrix/                    # Matrix 集成模块
│   │   ├── command/               #   命令系统 (parser/router/handlers)
│   │   ├── gateway/               #   Matrix 网关 (client/sync)
│   │   ├── infra/                 #   基础设施 (Redis/NATS)
│   │   ├── notify/                #   通知网关
│   │   ├── policy/                #   权限引擎
│   │   ├── queue/                 #   消息队列
│   │   ├── registry/              #   注册中心
│   │   └── worker/                #   后台工作者
│   │
│   ├── middleware/                 # HTTP 中间件
│   │   ├── auth.go                #   Authelia + API Key 认证
│   │   ├── cors.go, logger.go, ratelimit.go
│   │
│   ├── metrics/                   # Prometheus 指标
│   │
│   └── pkg/                       # 内部工具包
│       ├── response/              #   统一 API 响应
│       ├── errors/                #   错误类型定义
│       ├── defaults/              #   常量和默认值
│       ├── urlutil/               #   URL 规范化
│       └── sanitizer/             #   HTML 清理
│
├── web/                           # 前端 (SolidJS)
│   ├── src/
│   │   ├── api/index.ts           #   类型安全的 API 客户端
│   │   ├── types/index.ts         #   TypeScript 类型定义
│   │   ├── components/            #   Layout / Toast / Modal
│   │   └── pages/                 #   15 个页面 (Dashboard/Tags/RSS/...)
│   ├── index.html
│   ├── package.json
│   └── vite.config.ts
│
├── config/
│   └── bellkeeper.yaml            # 默认配置文件
│
├── docker/
│   ├── Dockerfile                 # 多阶段构建
│   └── docker-compose.yml         # 本地开发编排
│
├── doc/                           # 项目文档
│   ├── ENHANCEMENT-PLAN.md        # 增强规划（当前开发方向）
│   ├── PROGRESS.md                # 进度跟踪与验收检查
│   ├── DEVELOPMENT-GUIDE.md       # 开发规范与架构说明
│   ├── API.md                     # REST API 参考
│   ├── ARCHITECTURE.md            # 系统架构总览
│   ├── LLM_PROXY_GUIDE.md         # LLM 代理池指南
│   ├── documents/                 # 文件入库模块文档
│   ├── matrix/                    # Matrix 平台文档
│   ├── rss/                       # RSS 模块文档
│   └── old/                       # 已归档文档
│
├── go.mod / go.sum
├── Makefile
└── README.md
```

## 架构设计

```
                      Handler 层 (HTTP 请求/响应)
                            │
                    ┌───────┴────────┐
                    │  response 包    │  ← 统一响应格式 (Success/Page/Error)
                    │  ParsePagination│  ← 通用分页解析
                    └───────┬────────┘
                            │
                      Service 层 (业务逻辑)
                            │
                    ┌───────┴────────┐
                    │  defaults 包    │  ← 集中管理的常量
                    └───────┬────────┘
                            │
                     Repository 层 (数据访问)
                            │
                       Model 层 (GORM)
```

### 分层职责

| 层级 | 目录 | 职责 |
|------|------|------|
| **路由** | `internal/router/` | 按功能域分组注册 API 路由，与 main.go 解耦 |
| **处理器** | `internal/handler/` | HTTP 请求解析、参数校验、调用 Service、返回响应 |
| **业务逻辑** | `internal/service/` | 核心业务规则、跨实体操作、外部 API 集成 |
| **数据访问** | `internal/repository/` | GORM 查询封装、分页、过滤 |
| **数据模型** | `internal/model/` | 数据库表结构定义、关联关系、迁移 |
| **中间件** | `internal/middleware/` | 认证、CORS、请求日志 |
| **工具包** | `internal/pkg/` | 统一响应、常量定义、URL 规范化 |

### 依赖注入

```
Config → DB → Repositories → Services → Handlers → Router
```

采用手动构造函数注入，清晰明确，无需 DI 容器。

### 外部集成

```
┌──────────────────────────────────────────────────────────────┐
│                       Caddy (反向代理)                        │
│                     + Authelia (认证)                          │
└────────────────────────────┬─────────────────────────────────┘
                             │ Remote-User / X-API-Key
┌────────────────────────────▼─────────────────────────────────┐
│                      Bellkeeper Backend                        │
│  ┌─────────────┐  ┌──────────────┐  ┌────────────────────┐  │
│  │  Middleware  │→ │   Handler    │→ │     Service        │  │
│  │  Auth/CORS  │  │  (HTTP API)  │  │  (Business Logic)  │  │
│  │  RateLimit  │  │              │  │                    │  │
│  └─────────────┘  └──────────────┘  └────────┬───────────┘  │
│                                               │              │
│                                    ┌──────────▼───────────┐  │
│                                    │    Repository        │  │
│                                    │  (Data Access)       │  │
│                                    └──────────┬───────────┘  │
│                                               │              │
│  ┌────────────────────────────────────────────┼───────────┐  │
│  │               SolidJS Frontend (嵌入)       │           │  │
│  │  Dashboard | Documents | Datasets | Tags   │           │  │
│  │  LLM Proxy | Workflows | Matrix Admin     │           │  │
│  │  RSS | Logs | Settings                     │           │  │
│  └────────────────────────────────────────────┼───────────┘  │
└───────────────────────────────────────────────┼──────────────┘
    │              │              │              │         │
┌───▼───┐  ┌──────▼──────┐  ┌───▼───┐  ┌──────▼───┐ ┌───▼───┐
│ PgSQL │  │   RagFlow   │  │  n8n  │  │  Matrix  │ │ Redis │
│       │  │ (向量知识库) │  │       │  │ (Conduit)│ │ NATS  │
└───────┘  └─────────────┘  └───────┘  └──────────┘ └───────┘
```

## API 端点

完整 API 参考见 [doc/API.md](doc/API.md)。以下为主要端点概览：

### 公开端点

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/health` | 基本健康检查 |
| GET | `/api/health/detailed` | 详细健康检查 |
| GET | `/api/health/live` | Liveness 探针 |
| GET | `/api/health/ready` | Readiness 探针 |
| GET | `/metrics` | Prometheus 指标 |

### 认证端点

#### 标签

| 方法 | 路径 | 说明 |
|------|------|------|
| GET/POST | `/api/tags` | 标签列表 / 创建 |
| GET/PUT/DELETE | `/api/tags/:id` | 标签详情 / 更新 / 删除 |

#### RSS 订阅

| 方法 | 路径 | 说明 |
|------|------|------|
| GET/POST | `/api/rss` | RSS 列表 / 创建 |
| GET/PUT/DELETE | `/api/rss/:id` | 详情 / 更新 / 删除 |

#### 知识库映射

| 方法 | 路径 | 说明 |
|------|------|------|
| GET/POST | `/api/datasets` | 映射列表 / 创建 |
| GET/PUT/DELETE | `/api/datasets/:id` | 详情 / 更新 / 删除 |

#### RagFlow 文档

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/ragflow/upload/with-routing` | 智能路由上传 |
| GET | `/api/ragflow/check-url` | URL 去重检查 |
| GET | `/api/ragflow/documents` | 文档列表 |

#### LLM Proxy

| 方法 | 路径 | 说明 |
|------|------|------|
| ANY | `/api/llm/v1/*` | OpenAI 兼容代理 |
| GET | `/api/llm/health` | 渠道健康状态 |
| GET | `/api/llm/groups/status` | 模型组状态 |

#### Matrix

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/matrix/notify` | 发送通知 |
| GET | `/api/matrix/admin/rooms` | 房间列表 |
| GET | `/api/matrix/admin/commands` | 命令列表 |

#### n8n 工作流

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/workflows/status` | 工作流列表 |
| POST | `/api/workflows/:id/activate` | 激活工作流 |
| POST | `/api/workflows/:id/deactivate` | 停用工作流 |
| GET | `/api/workflows/executions` | 执行历史 |
| POST | `/api/workflows/trigger/:name` | 按名称触发工作流 |

## 文档导读

| 文档 | 用途 | 建议阅读 |
|------|------|----------|
| [doc/ENHANCEMENT-PLAN.md](doc/ENHANCEMENT-PLAN.md) | **增强规划** — 当前开发方向、4 个阶段的详细实施方案 | 参与开发前必读 |
| [doc/PROGRESS.md](doc/PROGRESS.md) | **进度跟踪** — 每项任务的验收检查清单 | 开发中随时对照 |
| [doc/DEVELOPMENT-GUIDE.md](doc/DEVELOPMENT-GUIDE.md) | **开发规范** — 架构说明、编码标准、禁止事项 | 写代码前必读 |
| [doc/API.md](doc/API.md) | **API 参考** — 完整 REST API 文档 | 对接/调试时查阅 |
| [doc/ARCHITECTURE.md](doc/ARCHITECTURE.md) | **架构总览** — 系统定位、模块职责、技术选型 | 了解全局 |
| [doc/LLM_PROXY_GUIDE.md](doc/LLM_PROXY_GUIDE.md) | **LLM 代理池** — 渠道配置、模型组、熔断器使用 | 配置 LLM 时查阅 |
| [doc/documents/](doc/documents/) | **文件入库模块** — 入库流程、索引策略、迁移方案 | 知识管道开发 |
| [doc/matrix/](doc/matrix/) | **Matrix 平台** — 架构、数据模型、API 契约、实施清单 | Matrix 功能开发 |
| [doc/rss/](doc/rss/) | **RSS 模块** — 采集管道、提取策略 | RSS 功能开发 |

## 部署

### 方式一：Docker Compose (推荐)

```bash
# 克隆仓库
git clone https://github.com/singll/Bellkeeper.git
cd Bellkeeper

# 创建环境变量文件
cat > docker/.env << 'EOF'
DB_PASSWORD=your_secure_password
RAGFLOW_API_KEY=your_ragflow_api_key
EOF

# 启动服务
make docker-up

# 查看日志
make docker-logs
```

服务启动后访问 `http://localhost:8080`。

### 方式二：SilkSpool 集成部署

Bellkeeper 已集成到 SilkSpool 部署框架，在 knowledge 主机上作为服务运行：

```bash
cd /home/ubuntu/SilkSpool

# 首次部署 (合并 YAML 模板)
./spool.sh bundle knowledge setup knowledge

# 单独更新 Bellkeeper 服务
./spool.sh bundle knowledge service knowledge bellkeeper up

# 检查状态
./spool.sh status knowledge bellkeeper

# 查看日志
./spool.sh logs knowledge bellkeeper 100
```

生产环境通过 Caddy 反向代理 + Authelia 认证访问：`https://bellkeeper.singll.net`

## 配置

### 配置文件

主配置文件：`config/bellkeeper.yaml`

```yaml
server:
  host: 0.0.0.0
  port: 8080
  mode: release          # debug / release

database:
  driver: postgres
  host: localhost
  port: 5432
  name: bellkeeper
  user: bellkeeper
  password: ${DB_PASSWORD}
  sslmode: disable

ragflow:
  base_url: http://ragflow:9380
  api_key: ${RAGFLOW_API_KEY}
  timeout: 30

n8n:
  webhook_base_url: http://n8n:5678

logging:
  level: info
  format: json
  output: stdout

features:
  auto_parse: true
  url_dedup: true
  ai_summary: false
```

### 环境变量覆盖

所有配置项支持 `BELLKEEPER_` 前缀的环境变量覆盖：

```bash
BELLKEEPER_SERVER_PORT=9090
BELLKEEPER_DATABASE_PASSWORD=secret
BELLKEEPER_RAGFLOW_API_KEY=ragflow-xxx
BELLKEEPER_N8N_WEBHOOK_BASE_URL=http://n8n:5678
```

### 数据库默认设置

服务启动时自动种子化以下默认配置项 (可通过 Web UI 修改)：

| Key | 类别 | 说明 |
|-----|------|------|
| `ragflow_base_url` | api | RagFlow API 地址 |
| `ragflow_api_key` | api | RagFlow API Key (敏感) |
| `n8n_webhook_base_url` | api | n8n Webhook 地址 |
| `n8n_api_base_url` | api | n8n API 地址 |
| `n8n_api_key` | api | n8n API Key (敏感) |
| `feature_auto_parse` | feature | 自动解析上传文档 |
| `feature_url_dedup` | feature | URL 去重检查 |
| `feature_ai_summary` | feature | AI 自动摘要 |
| `ui_page_size` | ui | 默认分页大小 |
| `ui_theme` | ui | 界面主题 |

## 开发规范

### 后端 (Go / Gin)

#### 分层架构

严格遵循 **Router → Handler → Service → Repository → Model** 单向依赖：

- **Handler** 只做 HTTP 协议处理（解析请求、返回响应），不含业务逻辑
- **Service** 承载所有业务规则和外部 API 调用，可跨 Repository 组合
- **Repository** 仅封装 GORM 查询，不引用其他 Repository
- **Model** 纯数据结构定义，不包含行为方法

```
禁止：Handler 直接调用 Repository
禁止：Repository 之间互相引用
禁止：Service 直接操作 *gin.Context
```

#### 统一响应格式

所有 Handler 必须使用 `internal/pkg/response` 包返回响应，禁止直接写 `c.JSON(...)` + `gin.H{...}`：

```go
// 正确 ✓
response.Success(c, data)
response.Page(c, list, total, page, perPage)
response.Created(c, item)
response.Deleted(c)
response.BadRequest(c, "invalid parameter")

// 错误 ✗
c.JSON(http.StatusOK, gin.H{"data": data})
```

**例外**：代理转发上游 API 原始响应的端点（如 RagFlow 文档列表），保持 `c.JSON(http.StatusOK, result)` 直接透传。

#### 参数解析

使用 `response.ParsePagination(c)` 和 `response.ParseID(c, "id")` 替代手动解析，保持一致的默认值和错误处理。

#### 常量管理

所有硬编码值必须收入 `internal/pkg/defaults` 包，禁止在业务代码中直接写魔法数字或字符串：

```go
// 正确 ✓
defaults.DefaultTagColor
defaults.HealthCheckTimeout

// 错误 ✗
"#409EFF"
5 * time.Second
```

#### 路由注册

路由定义集中在 `internal/router/` 包，按功能域分组为独立函数（`registerTagRoutes`、`registerWebhookRoutes` 等），`main.go` 只负责初始化和启动。

#### 依赖注入

采用手动构造函数注入，沿链路传递：

```go
Config → DB → Repositories → Services(repos, cfg, version) → Handlers(services) → Router
```

不使用 DI 容器，不使用全局变量，不使用 `init()` 函数。

#### 错误处理

- **永远不要忽略 error 返回值**，即使是 `json.Marshal`、`io.ReadAll` 等"不太可能失败"的调用
- Service 层返回 `error` 给 Handler，由 Handler 决定 HTTP 状态码
- 外部 API 调用失败时记录日志 (`log.Printf`) 并返回有意义的错误信息
- Repository 层的错误直接向上传播，不做吞没

#### 版本管理

版本号通过 `main.go` 的 `ldflags` 注入，经构造函数链传递到需要的位置，禁止在多处硬编码。

### 前端 (SolidJS / TypeScript / TailwindCSS)

#### 配色体系

采用**深炭灰**主题，避免使用纯黑 (`#000000` / `#020617`) 作为背景：

| 层级 | 用途 | 色阶 |
|------|------|------|
| 页面背景 | `body` | `dark-900` (#0f172a) |
| 卡片/面板 | `.card` | `dark-800/70` |
| 输入框/表格头 | `.input` `.table th` | `dark-700/40` |
| 弹窗 | `.modal` | `dark-800` |
| 悬停态 | hover | 比当前层级亮一档 |

核心原则：**每个层级之间要有可辨识的对比度差异**，用户不需要费力分辨元素边界。

#### 组件规范

- 统一使用 `useToast()` 进行操作反馈，成功/失败/警告有对应样式
- 弹窗统一使用 `<Modal>` 组件，支持 ESC 关闭和点击遮罩关闭
- 列表页标准结构：Header (标题 + 操作按钮) → 搜索/筛选栏 → 数据表格 → 分页
- 加载态使用 `<Show when={!data.loading}>` + spinner fallback
- 空态提供图标 + 文字 + 引导操作

#### CSS 组织

全局样式集中在 `index.css`，定义可复用的组件类（`.card`、`.btn`、`.input`、`.table` 等），页面级组件通过 TailwindCSS 工具类组合，避免重复定义。

### 通用

- **不过度设计**：不为假设的未来需求添加抽象层，三行重复代码优于一个不必要的抽象
- **不引入 interface 抽象**：当前具体类型 DI 足够好，等需要写单测时再加
- **提交粒度**：一个完整功能/修复对应一次提交，不拆分碎片化提交

## 本地开发

### 环境要求

- Go 1.22+
- Node.js 20+
- pnpm
- PostgreSQL 16

### 启动开发环境

```bash
# 安装 Go 依赖
make deps

# 安装前端依赖
cd web && pnpm install && cd ..

# 启动 PostgreSQL (或使用 docker-compose 中的数据库)
make docker-up  # 仅启动数据库

# 运行数据库迁移
make migrate

# 启动后端 (支持热重载，需安装 air)
make dev

# 另一个终端启动前端开发服务器
make dev-frontend
```

### 常用命令

```bash
make build           # 构建前后端
make test            # 运行测试
make fmt             # 格式化代码
make lint            # 代码检查
make all             # 格式化 + 检查 + 测试 + 构建
```

## 认证

Bellkeeper 使用 **Authelia Forward Auth** 进行认证。Caddy 反向代理将认证后的用户信息通过 HTTP Header 传递给后端：

| Header | 说明 |
|--------|------|
| `Remote-User` | 用户名 (必须) |
| `Remote-Email` | 邮箱 |
| `Remote-Name` | 显示名称 |
| `Remote-Groups` | 用户组 (逗号分隔) |

在 `debug` 模式下，未提供认证 Header 时自动使用 `dev-user` 身份。

## License

MIT
