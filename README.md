# Bellkeeper

**Bellkeeper (钟守者)** 是一个知识管理系统，用于采集、组织和检索信息。集成 RagFlow 向量知识库和 n8n 工作流引擎，提供从信息采集到知识入库的完整管理链路。

## 功能概览

### 核心功能

- **标签系统** — 统一的知识分类标签，支持自定义颜色，与所有实体关联
- **数据源管理** — 管理各类信息来源 URL，按类型/分类组织
- **RSS 订阅** — RSS Feed 管理，可配置抓取间隔
- **Webhook 管理** — 自定义 Webhook 端点配置、手动触发、完整的请求/响应历史记录
- **知识库映射** — 将标签映射到 RagFlow Dataset，实现智能路由

### 集成能力

- **RagFlow 集成** — 文档上传、智能路由上传、文档管理、URL 去重检查
- **n8n 工作流** — 查看/激活/停用工作流、执行历史、手动触发
- **系统设置** — Web UI 动态配置 API Key、功能开关等

### 监控面板

- **Dashboard** — 系统概览、服务状态、快捷操作
- **健康检查** — 外部服务连通性监测 (RagFlow、n8n)、系统指标统计

## 技术栈

| 层级 | 技术 | 版本 |
|------|------|------|
| **后端框架** | Go + Gin | Go 1.22, Gin 1.9 |
| **ORM** | GORM | 1.25 |
| **数据库** | PostgreSQL | 16 |
| **前端框架** | SolidJS + TypeScript | SolidJS 1.8 |
| **UI 样式** | TailwindCSS | 3.4 |
| **构建工具** | Vite | 5.x |
| **包管理** | Go Modules / pnpm | - |
| **认证** | Authelia (Forward Auth) | - |
| **配置** | Viper + Cobra | - |
| **日志** | Zap (结构化日志) | 1.26 |
| **容器** | Docker (多阶段构建) | Alpine |

## 项目结构

```
bellkeeper/
├── cmd/bellkeeper/
│   └── main.go                    # 入口 (serve / migrate / version)
│
├── internal/
│   ├── config/                    # 配置管理 (Viper)
│   │   ├── config.go              #   配置结构体定义
│   │   └── defaults.go            #   默认值
│   │
│   ├── handler/                   # HTTP 处理器 (8 个)
│   │   ├── health.go              #   健康检查
│   │   ├── tag.go                 #   标签 CRUD
│   │   ├── datasource.go          #   数据源 CRUD
│   │   ├── rss.go                 #   RSS 订阅 CRUD
│   │   ├── webhook.go             #   Webhook CRUD + 触发/历史
│   │   ├── dataset.go             #   知识库映射 CRUD
│   │   ├── ragflow.go             #   RagFlow 文档管理
│   │   └── workflow.go            #   n8n 工作流管理
│   │
│   ├── service/                   # 业务逻辑层 (9 个)
│   │   ├── health.go              #   服务健康检查逻辑
│   │   ├── tag.go                 #   标签业务 (含 GetOrCreateByNames)
│   │   ├── datasource.go          #   数据源业务
│   │   ├── rss.go                 #   RSS 业务
│   │   ├── webhook.go             #   Webhook 执行 + 历史记录
│   │   ├── dataset.go             #   知识库映射 + 标签路由
│   │   ├── ragflow.go             #   RagFlow API 调用 + 智能路由
│   │   ├── workflow.go            #   n8n REST API 调用
│   │   └── setting.go             #   配置管理 (含秘钥掩码)
│   │
│   ├── repository/                # 数据访问层 (6 个)
│   │   ├── tag.go
│   │   ├── datasource.go
│   │   ├── rss.go
│   │   ├── webhook.go
│   │   ├── dataset.go
│   │   └── setting.go
│   │
│   ├── model/                     # 数据模型 (GORM)
│   │   ├── db.go                  #   数据库初始化 + AutoMigrate + SeedSettings
│   │   ├── tag.go                 #   Tag (多对多关联)
│   │   ├── datasource.go          #   DataSource
│   │   ├── rss_feed.go            #   RSSFeed
│   │   ├── webhook.go             #   WebhookConfig + WebhookHistory
│   │   ├── dataset_mapping.go     #   DatasetMapping + ArticleTag
│   │   └── setting.go             #   Setting (含 MaskedValue)
│   │
│   └── middleware/                 # HTTP 中间件
│       ├── auth.go                #   Authelia Forward Auth (Remote-User)
│       ├── cors.go                #   CORS
│       └── logger.go              #   Zap 结构化日志
│
├── web/                           # 前端 (SolidJS)
│   ├── src/
│   │   ├── api/index.ts           #   类型安全的 API 客户端
│   │   ├── types/index.ts         #   TypeScript 类型定义
│   │   ├── components/
│   │   │   ├── Layout.tsx         #   主布局 + 侧边栏导航
│   │   │   ├── Toast.tsx          #   通知组件
│   │   │   └── Modal.tsx          #   模态框组件
│   │   ├── pages/
│   │   │   ├── Dashboard.tsx      #   仪表板
│   │   │   ├── Tags.tsx           #   标签管理
│   │   │   ├── DataSources.tsx    #   数据源管理
│   │   │   ├── RSSFeeds.tsx       #   RSS 订阅管理
│   │   │   ├── Datasets.tsx       #   知识库映射
│   │   │   ├── Documents.tsx      #   RagFlow 文档管理
│   │   │   ├── Webhooks.tsx       #   Webhook 管理
│   │   │   ├── Workflows.tsx      #   n8n 工作流
│   │   │   └── Settings.tsx       #   系统设置
│   │   ├── index.tsx              #   前端入口
│   │   └── index.css              #   全局样式
│   ├── package.json
│   ├── vite.config.ts
│   ├── tsconfig.json
│   └── tailwind.config.js
│
├── config/
│   └── bellkeeper.yaml            # 默认配置文件
│
├── docker/
│   ├── Dockerfile                 # 多阶段构建 (Node + Go + Alpine)
│   └── docker-compose.yml         # 本地开发编排
│
├── migrations/
│   └── 001_init.up.sql            # 初始数据库迁移
│
├── doc/
│   └── BELLKEEPER_REFACTOR.md     # 架构设计文档
│
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

## API 端点

### 公开端点 (无需认证)

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/health` | 基本健康检查 |
| GET | `/api/health/detailed` | 详细健康检查 (含外部服务状态和系统指标) |

### 认证端点 (需 Authelia Forward Auth)

#### 标签

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/tags` | 标签列表 (支持 `page`, `per_page`, `keyword`) |
| POST | `/api/tags` | 创建标签 |
| GET | `/api/tags/:id` | 获取标签详情 |
| PUT | `/api/tags/:id` | 更新标签 |
| DELETE | `/api/tags/:id` | 删除标签 |

#### 数据源

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/datasources` | 数据源列表 (支持 `category`, `keyword`) |
| POST | `/api/datasources` | 创建数据源 (支持 `tag_ids` 关联) |
| GET | `/api/datasources/:id` | 获取详情 |
| PUT | `/api/datasources/:id` | 更新数据源 |
| DELETE | `/api/datasources/:id` | 删除数据源 |

#### RSS 订阅

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/rss` | RSS 列表 (支持 `category`, `keyword`) |
| POST | `/api/rss` | 创建订阅 |
| GET | `/api/rss/:id` | 获取详情 |
| PUT | `/api/rss/:id` | 更新订阅 |
| DELETE | `/api/rss/:id` | 删除订阅 |

#### Webhook

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/webhooks` | Webhook 列表 |
| POST | `/api/webhooks` | 创建 Webhook |
| GET | `/api/webhooks/:id` | 获取详情 |
| PUT | `/api/webhooks/:id` | 更新 Webhook |
| DELETE | `/api/webhooks/:id` | 删除 Webhook |
| POST | `/api/webhooks/:id/trigger` | 触发执行 |
| GET | `/api/webhooks/:id/history` | 执行历史 |

#### 知识库映射

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/datasets` | 映射列表 |
| POST | `/api/datasets` | 创建映射 (支持 `tag_ids` 关联) |
| GET | `/api/datasets/:id` | 获取详情 |
| PUT | `/api/datasets/:id` | 更新映射 |
| DELETE | `/api/datasets/:id` | 删除映射 |

#### RagFlow 文档

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/ragflow/upload` | 上传文档到指定 Dataset |
| POST | `/api/ragflow/upload/with-routing` | 智能路由上传 (根据标签/分类自动选择 Dataset) |
| GET | `/api/ragflow/check-url` | URL 去重检查 |
| GET | `/api/ragflow/documents` | 文档列表 |
| DELETE | `/api/ragflow/documents/:id` | 删除文档 |

#### 系统设置

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/settings` | 获取所有设置 (支持 `category` 筛选) |
| GET | `/api/settings/:key` | 获取单个设置 |
| PUT | `/api/settings/:key` | 更新设置值 |

#### n8n 工作流

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/workflows/status` | 工作流列表 |
| GET | `/api/workflows/:id` | 工作流详情 |
| POST | `/api/workflows/:id/activate` | 激活工作流 |
| POST | `/api/workflows/:id/deactivate` | 停用工作流 |
| GET | `/api/workflows/executions` | 执行历史 |
| POST | `/api/workflows/trigger/:name` | 按名称触发工作流 |

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

## 架构设计

```
┌─────────────────────────────────────────────────────────────┐
│                      Caddy (反向代理)                        │
│                    + Authelia (认证)                          │
└───────────────────────────┬─────────────────────────────────┘
                            │ Remote-User Header
┌───────────────────────────▼─────────────────────────────────┐
│                     Bellkeeper Backend                        │
│  ┌─────────────┐  ┌──────────────┐  ┌────────────────────┐  │
│  │  Middleware  │→ │   Handler    │→ │     Service        │  │
│  │  Auth/CORS  │  │  (HTTP API)  │  │  (Business Logic)  │  │
│  │  Logger     │  │              │  │                    │  │
│  └─────────────┘  └──────────────┘  └────────┬───────────┘  │
│                                               │              │
│                                    ┌──────────▼───────────┐  │
│                                    │    Repository        │  │
│                                    │  (Data Access)       │  │
│                                    └──────────┬───────────┘  │
│                                               │              │
│  ┌────────────────────────────────────────────┼───────────┐  │
│  │               SolidJS Frontend (嵌入)       │           │  │
│  │  Dashboard | Tags | DataSources | RSS      │           │  │
│  │  Datasets | Documents | Webhooks           │           │  │
│  │  Workflows | Settings                      │           │  │
│  └────────────────────────────────────────────┼───────────┘  │
└───────────────────────────────────────────────┼──────────────┘
          │                                     │
    ┌─────▼─────┐  ┌──────▼──────┐  ┌─────────▼─────────┐
    │ PostgreSQL │  │   RagFlow   │  │       n8n         │
    │  (数据库)  │  │ (向量知识库) │  │  (工作流引擎)     │
    └───────────┘  └─────────────┘  └───────────────────┘
```

## License

MIT
