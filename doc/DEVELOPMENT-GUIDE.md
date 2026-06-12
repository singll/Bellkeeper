# Bellkeeper 开发指南

> **面向对象**: 初级开发者
> **最后更新**: 2026-04-09
> **适用版本**: v0.3.0+
>
> 本文档是 Bellkeeper 项目的完整开发指南，包含架构说明、开发规范、禁止事项和验收标准。
> 在进行任何开发工作前，请**完整阅读**本文档。

---

## 目录

1. [项目概述](#项目概述)
2. [当前架构](#当前架构)
3. [代码组织](#代码组织)
4. [开发环境搭建](#开发环境搭建)
5. [开发规范](#开发规范)
6. [新功能开发示例](#新功能开发示例)
7. [测试规范](#测试规范)
8. [禁止事项](#禁止事项)
9. [常见问题](#常见问题)
10. [验收标准](#验收标准)
11. [继续开发计划](#继续开发计划)

---

## 项目概述

### 定位

Bellkeeper 是 SilkSpool 生态的**知识管理中台 + LLM 代理网关**。

核心价值: **做 n8n 做不好的事** — 有状态的限速、去重、路由、日志、治理。

### 技术栈

| 层 | 技术 | 版本 |
|----|------|------|
| 后端 | Go + Gin | Go 1.22+, Gin 1.9 |
| 数据库 | PostgreSQL + GORM | PG 16, GORM 1.25 |
| 前端 | SolidJS + TailwindCSS + Vite | SolidJS 1.8, Vite 5.x |
| 消息队列 | NATS JetStream | 2.10 |
| 缓存 | Redis | 7.x |
| Matrix SDK | mautrix-go | 0.26 |
| 认证 | Authelia SSO + API Key | - |
| 配置 | Viper + Cobra | - |
| 日志 | Zap (结构化日志) | 1.26 |
| 部署 | Docker 多阶段构建 | Alpine |

### 功能模块状态

| 模块 | 状态 | 说明 |
|------|------|------|
| LLM Proxy | ✅ 已实施 | 多渠道限速代理，虚拟模型组，熔断与粘性路由 |
| RSS 管理 | ✅ 已实施 | 订阅源 CRUD，供 n8n 工作流使用 |
| 标签体系 | ✅ 已实施 | 分类路由核心数据 |
| URL 去重 | ✅ 已实施 | 三级匹配（精确→归一化→模糊） |
| 分类路由 | ✅ 已实施 | LLM 驱动的文章分类 |
| RAGFlow 集成 | ✅ 兼容层 | 保留但不再增强 |
| n8n 集成 | ✅ 已实施 | 工作流管理 API |
| 文件入库 | 🚧 实施中 | 提取器编排 + 文件落地 |
| Matrix 平台 | 🚧 部分实施 | 命令路由 + 通知（核心功能待完成） |

---

## 当前架构

### 系统架构图

```
┌─────────────────────────────────────────────────────────────┐
│                       外部系统                               │
├──────────┬──────────┬──────────┬──────────┬─────────────────┤
│  n8n     │ RAGFlow  │ Matrix   │  前端 UI  │  其他 API 调用方  │
│ (工作流)  │ (向量库)  │(机器人)   │ (SolidJS) │                │
└──────┬───┴────┬─────┴─────┬────┴─────┬────┴────────┬────────┘
       │        │           │          │             │
       ▼        ▼           ▼          ▼             ▼
┌─────────────────────────────────────────────────────────────┐
│                    Bellkeeper 应用层                          │
│                                                               │
│  ┌─────────────┐  ┌─────────────┐  ┌──────────────────────┐ │
│  │  HTTP Router │→ │  Middleware  │→ │    Handler 层         │ │
│  │  (Gin)      │  │  (Auth/CORS) │  │  (请求解析+响应封装)   │ │
│  └─────────────┘  └─────────────┘  └──────────┬───────────┘ │
│                                                 │             │
│  ┌──────────────────────────────────────────────▼───────────┐ │
│  │                     Service 层                           │ │
│  │                     (业务逻辑)                            │ │
│  │                                                           │ │
│  │  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐   │ │
│  │  │ LLMProxy │ │ Dataset  │ │ Classify │ │ RSSFeed  │   │ │
│  │  │ Service  │ │ Service  │ │ Service  │ │ Service  │   │ │
│  │  └──────────┘ └──────────┘ └──────────┘ └──────────┘   │ │
│  │  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐   │ │
│  │  │ RagFlow  │ │ Workflow │ │ Health   │ │ Notify   │   │ │
│  │  │ Service  │ │ Service  │ │ Service  │ │ Service  │   │ │
│  │  └──────────┘ └──────────┘ └──────────┘ └──────────┘   │ │
│  └──────────────────────────────┬───────────────────────────┘ │
│                                  │                             │
│  ┌───────────────────────────────▼──────────────────────────┐ │
│  │                   Repository 层                           │ │
│  │                   (数据访问)                               │ │
│  │                                                           │ │
│  │  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐   │ │
│  │  │ Tag      │ │ RSS      │ │ Dataset  │ │ LLMProxy │   │ │
│  │  │ Repo     │ │ Repo     │ │ Repo     │ │ Repo     │   │ │
│  │  └──────────┘ └──────────┘ └──────────┘ └──────────┘   │ │
│  └──────────────────────────────┬───────────────────────────┘ │
│                                  │                             │
└──────────────────────────────────┼─────────────────────────────┘
                                   │
┌──────────────────────────────────▼─────────────────────────────┐
│                      基础设施层                                  │
│                                                                  │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────────────┐  │
│  │PostgreSQL│ │  Redis   │ │  NATS    │ │  TrueNAS (挂载)   │  │
│  │  (数据)   │ │  (缓存)  │ │  (队列)  │ │  (文件存储)       │  │
│  └──────────┘ └──────────┘ └──────────┘ └──────────────────┘  │
└────────────────────────────────────────────────────────────────┘
```

### 请求处理流程

```
HTTP 请求
  ↓
Gin Router (路由匹配)
  ↓
Middleware (CORS → Logger → Auth)
  ↓
Handler (请求解析)
  ↓
Service (业务逻辑)
  ↓
Repository (数据访问)
  ↓
Database (PostgreSQL)
  ↓
Response (统一格式)
```

### 数据流向

```
外部数据源
  │
  ├─ RSS Feed → n8n K02 工作流 → Bellkeeper /api/ragflow/upload/with-routing
  │                                  ↓
  │                              分类路由 → URL去重 → RAGFlow 上传
  │
  ├─ URL 入库 → Bellkeeper /api/files/ingest/url
  │                  ↓
  │              提取器编排 → 文件落地 → 索引排队
  │
  └─ Matrix 命令 → Matrix Gateway → Command Router → Handler → 响应
```

---

## 代码组织

### 目录结构

```
Bellkeeper/
├── cmd/bellkeeper/
│   └── main.go                    # 入口：Cobra 命令（serve/migrate/version）
│
├── internal/                      # 内部包（Go 约定，不对外暴露）
│   ├── config/
│   │   └── config.go              # Viper 配置加载
│   │
│   ├── handler/                   # Handler 层：HTTP 请求处理
│   │   ├── handler.go             # Handlers 聚合结构体
│   │   ├── health.go              # 健康检查
│   │   ├── tag.go                 # 标签管理
│   │   ├── rss.go                 # RSS 订阅管理
│   │   ├── dataset.go             # 知识库映射
│   │   ├── ragflow.go             # RAGFlow 代理
│   │   ├── setting.go             # 运行时设置
│   │   ├── workflow.go            # n8n 工作流
│   │   ├── system.go              # 系统管理
│   │   ├── llm_proxy.go           # LLM 代理管理
│   │   ├── classify.go            # 分类
│   │   ├── file_ingestion.go      # 文件入库
│   │   ├── matrix_notify.go       # Matrix 通知
│   │   └── matrix_admin.go        # Matrix 管理
│   │
│   ├── service/                   # Service 层：业务逻辑
│   │   ├── service.go             # Services 聚合结构体
│   │   ├── tag.go                 # 标签业务
│   │   ├── rss.go                 # RSS 业务
│   │   ├── dataset.go             # 数据集 + URL去重 + 标签路由
│   │   ├── ragflow.go             # RAGFlow API 封装
│   │   ├── ragflow_parse_queue.go # 解析队列
│   │   ├── workflow.go            # n8n API 封装
│   │   ├── setting.go             # 设置管理
│   │   ├── health.go              # 健康检查
│   │   ├── llm_proxy.go           # LLM 代理核心（令牌桶 + 路由）
│   │   ├── llm_channel_health.go  # 熔断器
│   │   ├── llm_model_group.go     # 虚拟模型组 + 粘性路由
│   │   ├── classify.go            # LLM 分类
│   │   ├── extractor.go           # 内容提取器
│   │   ├── file_ingestion.go      # 文件入库
│   │   ├── activity_log.go        # 操作日志
│   │   ├── command.go             # Matrix 命令
│   │   ├── notification.go        # Matrix 通知
│   │   ├── notification_sender.go # 通知发送器
│   │   └── admin.go               # Matrix 管理
│   │
│   ├── repository/                # Repository 层：数据访问
│   │   ├── repository.go          # Repositories 聚合结构体
│   │   ├── tag.go
│   │   ├── rss.go
│   │   ├── dataset.go
│   │   ├── setting.go
│   │   ├── llm_proxy.go
│   │   ├── llm_channel.go
│   │   ├── llm_model_group.go
│   │   └── activity_log.go
│   │
│   ├── model/                     # 数据模型（GORM）
│   │   ├── db.go                  # 数据库初始化 + AutoMigrate + Seed
│   │   ├── tag.go
│   │   ├── rss.go
│   │   ├── dataset.go
│   │   ├── llm_proxy.go
│   │   ├── llm_channel.go
│   │   ├── llm_model_group.go
│   │   ├── activity_log.go
│   │   ├── matrix.go
│   │   └── setting.go
│   │
│   ├── middleware/                # HTTP 中间件
│   │   ├── auth.go               # 认证（Authelia + API Key）
│   │   ├── cors.go               # 跨域处理
│   │   └── logger.go             # 请求日志
│   │
│   ├── router/
│   │   └── router.go             # 路由注册
│   │
│   ├── matrix/                   # Matrix 平台模块
│   │   ├── gateway/              # Matrix 网关
│   │   │   ├── client.go         # Matrix 客户端
│   │   │   └── sync.go           # Sync Loop
│   │   ├── command/              # 命令系统
│   │   │   ├── parser.go         # 命令解析
│   │   │   ├── router.go         # 命令路由
│   │   │   ├── handler.go        # 处理器接口
│   │   │   └── handlers.go       # 内置处理器
│   │   ├── notify/               # 通知网关
│   │   ├── policy/               # 权限引擎
│   │   ├── registry/             # 注册中心
│   │   ├── queue/                # 消息队列
│   │   ├── worker/               # 后台工作者
│   │   │   └── notification_worker.go
│   │   └── infra/                # 基础设施
│   │       ├── redis.go
│   │       └── nats.go
│   │
│   └── pkg/                      # 内部公共包
│       ├── response/             # 统一响应格式
│       │   └── response.go
│       ├── defaults/             # 默认常量
│       │   └── defaults.go
│       └── urlutil/              # URL 工具
│           └── normalize.go
│
├── web/                          # 前端代码（SolidJS）
│   ├── src/
│   │   ├── api/index.ts          # API 客户端
│   │   ├── types/index.ts        # TypeScript 类型
│   │   ├── components/           # 通用组件
│   │   │   ├── Layout.tsx
│   │   │   ├── Toast.tsx
│   │   │   └── Modal.tsx
│   │   ├── pages/                # 页面组件
│   │   │   ├── Dashboard.tsx
│   │   │   ├── Tags.tsx
│   │   │   ├── RSSFeeds.tsx
│   │   │   ├── Datasets.tsx
│   │   │   ├── Documents.tsx
│   │   │   ├── Workflows.tsx
│   │   │   └── Settings.tsx
│   │   ├── index.tsx
│   │   └── index.css
│   ├── package.json
│   ├── vite.config.ts
│   └── tsconfig.json
│
├── config/
│   └── bellkeeper.yaml           # 默认配置文件
│
├── migrations/                   # SQL 迁移文件（参考用）
│   ├── 001_init.up.sql
│   └── 001_init.down.sql
│
├── docker/
│   ├── Dockerfile                # 多阶段构建
│   └── docker-compose.yml        # 开发编排
│
├── doc/                          # 项目文档
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

### 分层职责

| 层 | 目录 | 职责 | 规则 |
|----|------|------|------|
| **Handler** | `internal/handler/` | HTTP 请求解析、参数校验、调用 Service、返回响应 | 禁止包含业务逻辑 |
| **Service** | `internal/service/` | 业务逻辑、编排、外部 API 调用 | 核心逻辑在这里 |
| **Repository** | `internal/repository/` | 数据库 CRUD 操作、GORM 查询封装 | 只做数据操作 |
| **Model** | `internal/model/` | GORM 数据模型定义 | 只定义结构体和标签 |
| **Middleware** | `internal/middleware/` | HTTP 中间件（认证、日志、CORS） | 通用横切关注点 |
| **Router** | `internal/router/` | 路由注册 | 只做路由映射 |
| **Package** | `internal/pkg/` | 内部公共工具 | 可被任何层使用 |

**调用链**: Handler → Service → Repository → Model

**严格禁止**:
- Handler 直接调用 Repository
- Service 直接操作 HTTP 响应
- Repository 包含业务逻辑

### 依赖注入模式

```go
// 1. 创建 Repositories（数据层）
repos := repository.NewRepositories(db)

// 2. 创建 Services（业务层），注入 Repositories
services := service.NewServices(repos, cfg, version)

// 3. 创建 Handlers（HTTP 层），注入 Services
handlers := handler.NewHandlers(services, shutdownChan)

// 4. 注册路由
router.Setup(r, handlers, cfg.Server.Mode, cfg.Server.APIKey)
```

---

## 开发环境搭建

### 前提条件

- Go 1.22+
- Node.js 20+ (前端开发)
- PostgreSQL 16+
- Docker + Docker Compose
- Make

### 本地开发

```bash
# 1. 克隆项目
git clone https://github.com/singll/Bellkeeper.git
cd Bellkeeper

# 2. 安装 Go 依赖
go mod download

# 3. 配置
cp config/bellkeeper.yaml config/bellkeeper.local.yaml
# 编辑 config/bellkeeper.local.yaml，设置数据库连接等

# 4. 启动数据库（使用 Docker）
docker compose -f docker/docker-compose.yml up -d sp-bellkeeper-db

# 5. 运行数据库迁移
go run cmd/bellkeeper/main.go migrate --config config/bellkeeper.local.yaml

# 6. 启动后端服务
go run cmd/bellkeeper/main.go serve --config config/bellkeeper.local.yaml

# 7. 前端开发（另一个终端）
cd web
npm install
npm run dev
```

### Docker Compose 开发

```bash
# 启动全部服务
make docker-up

# 查看日志
make docker-logs

# 停止服务
make docker-down

# 重新构建
make docker-build
```

### Makefile 常用命令

```bash
make build          # 构建后端
make run            # 运行后端
make test           # 运行测试
make test-coverage  # 运行测试并生成覆盖率报告
make lint           # 代码检查
make docker-up      # Docker Compose 启动
make docker-down    # Docker Compose 停止
```

---

## 开发规范

### 命名规范

| 对象 | 规范 | 示例 |
|------|------|------|
| 文件名 | 小写 + 下划线 | `llm_proxy.go`, `file_ingestion.go` |
| 包名 | 小写单词 | `service`, `handler`, `repository` |
| 结构体 | 大驼峰 | `LLMProxyService`, `TokenBucket` |
| 公开方法 | 大驼峰 | `GetChannel()`, `TryAcquire()` |
| 私有方法 | 小驼峰 | `selectChannel()`, `doCleanup()` |
| 常量 | 大驼峰 | `DefaultTimeout`, `MaxRetries` |
| 接口 | er 结尾或名词 | `Handler`, `Repository` |
| 测试文件 | `*_test.go` | `llm_proxy_test.go` |

### 错误处理规范

#### 规则 1: 永远不要忽略错误

```go
// ✅ 正确
if err := doSomething(); err != nil {
    return fmt.Errorf("do something failed: %w", err)
}

// ❌ 禁止
_ = doSomething()

// ❌ 禁止
doSomething()  // 返回值被忽略
```

#### 规则 2: 使用 %w 包装错误

```go
// ✅ 正确 - 保留错误链
return fmt.Errorf("failed to create tag: %w", err)

// ❌ 不推荐 - 丢失错误链
return fmt.Errorf("failed to create tag: %v", err)

// ❌ 不推荐 - 创建新错误
return errors.New("failed to create tag")
```

#### 规则 3: 异步操作必须处理错误

```go
// ✅ 正确
go func() {
    if err := asyncOperation(); err != nil {
        log.Printf("[ERROR] async operation failed: %v", err)
    }
}()

// ❌ 禁止
go func() {
    _ = asyncOperation()
}()
```

#### 规则 4: Handler 层统一返回错误

```go
// ✅ 正确
func (h *TagHandler) Create(c *gin.Context) {
    var req CreateTagRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        response.BadRequest(c, "invalid request: "+err.Error())
        return
    }
    
    tag, err := h.svc.Create(req)
    if err != nil {
        response.InternalError(c, "failed to create tag: "+err.Error())
        return
    }
    
    response.Success(c, tag)
}
```

### 并发安全规范

#### 规则 1: 共享状态必须有锁保护

```go
type Cache struct {
    mu    sync.RWMutex
    items map[string]interface{}
}

// 读操作用读锁
func (c *Cache) Get(key string) interface{} {
    c.mu.RLock()
    defer c.mu.RUnlock()
    return c.items[key]
}

// 写操作用写锁
func (c *Cache) Set(key string, value interface{}) {
    c.mu.Lock()
    defer c.mu.Unlock()
    c.items[key] = value
}
```

#### 规则 2: Goroutine 必须可停止

```go
type Worker struct {
    stopCh chan struct{}
    wg     sync.WaitGroup
}

func (w *Worker) Start() {
    w.wg.Add(1)
    go func() {
        defer w.wg.Done()
        ticker := time.NewTicker(time.Second)
        defer ticker.Stop()
        for {
            select {
            case <-ticker.C:
                w.doWork()
            case <-w.stopCh:
                return
            }
        }
    }()
}

func (w *Worker) Stop() {
    close(w.stopCh)
    w.wg.Wait()  // 等待 goroutine 结束
}
```

#### 规则 3: 禁止无限制创建 Goroutine

```go
// ❌ 禁止
for _, item := range items {
    go process(item)  // 数量不可控
}

// ✅ 正确：使用 WaitGroup + 限制并发数
sem := make(chan struct{}, 10)  // 最多 10 个并发
var wg sync.WaitGroup
for _, item := range items {
    wg.Add(1)
    sem <- struct{}{}  // 获取信号量
    go func(item Item) {
        defer wg.Done()
        defer func() { <-sem }()  // 释放信号量
        process(item)
    }(item)
}
wg.Wait()
```

### 数据库操作规范

#### 规则 1: 使用 GORM 查询构建器

```go
// ✅ 正确
db.Where("name = ? AND status = ?", name, status).Find(&items)

// ❌ 禁止 - SQL 注入风险
db.Raw("SELECT * FROM items WHERE name = '" + name + "'").Scan(&items)
```

#### 规则 2: 使用事务保证原子性

```go
func (r *Repository) CreateWithTags(article *Article, tags []Tag) error {
    return r.db.Transaction(func(tx *gorm.DB) error {
        if err := tx.Create(article).Error; err != nil {
            return err
        }
        return tx.Model(article).Association("Tags").Append(tags)
    })
}
```

#### 规则 3: 使用 Preload 避免 N+1 查询

```go
// ✅ 正确
db.Preload("Tags").Find(&articles)

// ❌ 禁止
db.Find(&articles)
for _, a := range articles {
    db.Model(&a).Association("Tags").Find(&a.Tags)
}
```

#### 规则 4: 批量操作代替循环

```go
// ✅ 正确
db.Where("id IN ?", ids).Find(&items)

// ❌ 禁止
for _, id := range ids {
    db.First(&item, id)
}
```

### API 设计规范

#### RESTful 风格

```
GET    /api/tags          # 列表
POST   /api/tags          # 创建
GET    /api/tags/:id      # 详情
PUT    /api/tags/:id      # 更新
DELETE /api/tags/:id      # 删除
```

#### 统一响应格式

```json
// 成功
{
    "success": true,
    "data": { ... }
}

// 列表（分页）
{
    "success": true,
    "data": {
        "items": [ ... ],
        "total": 100,
        "page": 1,
        "page_size": 20
    }
}

// 错误
{
    "success": false,
    "error": "具体的错误信息"
}
```

#### 使用 response 包

```go
import "github.com/singll/bellkeeper/internal/pkg/response"

// 成功
response.Success(c, data)

// 列表
response.SuccessList(c, items, total, page, pageSize)

// 错误
response.BadRequest(c, "invalid request")
response.NotFound(c, "tag not found")
response.InternalError(c, "database error")
```

---

## 新功能开发示例

以下是添加一个新模块 "Bookmark"（书签管理）的完整示例。

### 步骤 1: 定义数据模型

文件: `internal/model/bookmark.go`

```go
package model

import (
    "gorm.io/gorm"
    "time"
)

// Bookmark 书签模型
type Bookmark struct {
    ID          uint           `json:"id" gorm:"primaryKey"`
    URL         string         `json:"url" gorm:"uniqueIndex;not null"`
    Title       string         `json:"title"`
    Description string         `json:"description"`
    TagID       *uint          `json:"tag_id"`
    Tag         *Tag           `json:"tag,omitempty" gorm:"foreignKey:TagID"`
    IsRead      bool           `json:"is_read" gorm:"default:false"`
    CreatedAt   time.Time      `json:"created_at"`
    UpdatedAt   time.Time      `json:"updated_at"`
    DeletedAt   gorm.DeletedAt `json:"-" gorm:"index"`
}
```

注意事项:
- 使用 `gorm.DeletedAt` 支持软删除
- JSON tag 控制 API 输出
- 外键关系明确标注

### 步骤 2: 在 db.go 中注册模型

文件: `internal/model/db.go` (修改)

```go
func AutoMigrate(db *gorm.DB) error {
    return db.AutoMigrate(
        // ... 现有模型
        &Bookmark{},  // 添加新模型
    )
}
```

### 步骤 3: 创建 Repository

文件: `internal/repository/bookmark.go`

```go
package repository

import (
    "github.com/singll/bellkeeper/internal/model"
    "gorm.io/gorm"
)

type BookmarkRepository struct {
    db *gorm.DB
}

func NewBookmarkRepository(db *gorm.DB) *BookmarkRepository {
    return &BookmarkRepository{db: db}
}

func (r *BookmarkRepository) List(page, pageSize int) ([]model.Bookmark, int64, error) {
    var bookmarks []model.Bookmark
    var total int64
    
    query := r.db.Model(&model.Bookmark{})
    
    if err := query.Count(&total).Error; err != nil {
        return nil, 0, err
    }
    
    err := query.
        Preload("Tag").
        Offset((page - 1) * pageSize).
        Limit(pageSize).
        Order("created_at DESC").
        Find(&bookmarks).Error
    
    return bookmarks, total, err
}

func (r *BookmarkRepository) Create(bookmark *model.Bookmark) error {
    return r.db.Create(bookmark).Error
}

func (r *BookmarkRepository) GetByID(id uint) (*model.Bookmark, error) {
    var bookmark model.Bookmark
    err := r.db.Preload("Tag").First(&bookmark, id).Error
    if err != nil {
        return nil, err
    }
    return &bookmark, nil
}

func (r *BookmarkRepository) Update(bookmark *model.Bookmark) error {
    return r.db.Save(bookmark).Error
}

func (r *BookmarkRepository) Delete(id uint) error {
    return r.db.Delete(&model.Bookmark{}, id).Error
}

func (r *BookmarkRepository) GetByURL(url string) (*model.Bookmark, error) {
    var bookmark model.Bookmark
    err := r.db.Where("url = ?", url).First(&bookmark).Error
    if err != nil {
        return nil, err
    }
    return &bookmark, nil
}
```

### 步骤 4: 注册到 Repositories 聚合体

文件: `internal/repository/repository.go` (修改)

```go
type Repositories struct {
    // ... 现有 Repositories
    Bookmark *BookmarkRepository
}

func NewRepositories(db *gorm.DB) *Repositories {
    return &Repositories{
        // ... 现有初始化
        Bookmark: NewBookmarkRepository(db),
    }
}
```

### 步骤 5: 创建 Service

文件: `internal/service/bookmark.go`

```go
package service

import (
    "fmt"
    "github.com/singll/bellkeeper/internal/model"
    "github.com/singll/bellkeeper/internal/repository"
)

type BookmarkService struct {
    repo *repository.BookmarkRepository
}

func NewBookmarkService(repo *repository.BookmarkRepository) *BookmarkService {
    return &BookmarkService{repo: repo}
}

// CreateBookmarkRequest 创建书签请求
type CreateBookmarkRequest struct {
    URL         string `json:"url" binding:"required"`
    Title       string `json:"title"`
    Description string `json:"description"`
    TagID       *uint  `json:"tag_id"`
}

func (s *BookmarkService) List(page, pageSize int) ([]model.Bookmark, int64, error) {
    if page <= 0 {
        page = 1
    }
    if pageSize <= 0 || pageSize > 100 {
        pageSize = 20
    }
    return s.repo.List(page, pageSize)
}

func (s *BookmarkService) Create(req CreateBookmarkRequest) (*model.Bookmark, error) {
    // 检查 URL 是否已存在
    existing, _ := s.repo.GetByURL(req.URL)
    if existing != nil {
        return nil, fmt.Errorf("bookmark with URL %s already exists", req.URL)
    }
    
    bookmark := &model.Bookmark{
        URL:         req.URL,
        Title:       req.Title,
        Description: req.Description,
        TagID:       req.TagID,
    }
    
    if err := s.repo.Create(bookmark); err != nil {
        return nil, fmt.Errorf("failed to create bookmark: %w", err)
    }
    
    return bookmark, nil
}

func (s *BookmarkService) GetByID(id uint) (*model.Bookmark, error) {
    bookmark, err := s.repo.GetByID(id)
    if err != nil {
        return nil, fmt.Errorf("bookmark not found: %w", err)
    }
    return bookmark, nil
}

func (s *BookmarkService) Delete(id uint) error {
    if err := s.repo.Delete(id); err != nil {
        return fmt.Errorf("failed to delete bookmark: %w", err)
    }
    return nil
}
```

### 步骤 6: 注册到 Services 聚合体

文件: `internal/service/service.go` (修改)

```go
type Services struct {
    // ... 现有 Services
    Bookmark *BookmarkService
}

func NewServices(repos *repository.Repositories, cfg *config.Config, version string) *Services {
    return &Services{
        // ... 现有初始化
        Bookmark: NewBookmarkService(repos.Bookmark),
    }
}
```

### 步骤 7: 创建 Handler

文件: `internal/handler/bookmark.go`

```go
package handler

import (
    "strconv"
    
    "github.com/gin-gonic/gin"
    "github.com/singll/bellkeeper/internal/pkg/response"
    "github.com/singll/bellkeeper/internal/service"
)

type BookmarkHandler struct {
    svc *service.BookmarkService
}

func NewBookmarkHandler(svc *service.BookmarkService) *BookmarkHandler {
    return &BookmarkHandler{svc: svc}
}

func (h *BookmarkHandler) List(c *gin.Context) {
    page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
    pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
    
    bookmarks, total, err := h.svc.List(page, pageSize)
    if err != nil {
        response.InternalError(c, "failed to list bookmarks: "+err.Error())
        return
    }
    
    response.SuccessList(c, bookmarks, total, page, pageSize)
}

func (h *BookmarkHandler) Create(c *gin.Context) {
    var req service.CreateBookmarkRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        response.BadRequest(c, "invalid request: "+err.Error())
        return
    }
    
    bookmark, err := h.svc.Create(req)
    if err != nil {
        response.InternalError(c, err.Error())
        return
    }
    
    response.Success(c, bookmark)
}

func (h *BookmarkHandler) Get(c *gin.Context) {
    id, err := strconv.ParseUint(c.Param("id"), 10, 32)
    if err != nil {
        response.BadRequest(c, "invalid id")
        return
    }
    
    bookmark, err := h.svc.GetByID(uint(id))
    if err != nil {
        response.NotFound(c, err.Error())
        return
    }
    
    response.Success(c, bookmark)
}

func (h *BookmarkHandler) Delete(c *gin.Context) {
    id, err := strconv.ParseUint(c.Param("id"), 10, 32)
    if err != nil {
        response.BadRequest(c, "invalid id")
        return
    }
    
    if err := h.svc.Delete(uint(id)); err != nil {
        response.InternalError(c, err.Error())
        return
    }
    
    response.Success(c, nil)
}
```

### 步骤 8: 注册到 Handlers 聚合体

文件: `internal/handler/handler.go` (修改)

```go
type Handlers struct {
    // ... 现有 Handlers
    Bookmark *BookmarkHandler
}

func NewHandlers(services *service.Services, shutdownChan chan struct{}) *Handlers {
    return &Handlers{
        // ... 现有初始化
        Bookmark: NewBookmarkHandler(services.Bookmark),
    }
}
```

### 步骤 9: 注册路由

文件: `internal/router/router.go` (修改)

```go
func Setup(r *gin.Engine, h *handler.Handlers, mode, apiKey string) {
    api := r.Group("/api")
    api.Use(middleware.Auth(apiKey))
    
    // ... 现有路由
    
    // Bookmark 路由
    bookmarks := api.Group("/bookmarks")
    {
        bookmarks.GET("", h.Bookmark.List)
        bookmarks.POST("", h.Bookmark.Create)
        bookmarks.GET("/:id", h.Bookmark.Get)
        bookmarks.DELETE("/:id", h.Bookmark.Delete)
    }
}
```

### 步骤 10: 添加测试

文件: `internal/service/bookmark_test.go`

```go
package service

import (
    "testing"
    
    "github.com/stretchr/testify/assert"
)

func TestBookmarkService_Create(t *testing.T) {
    // TODO: 设置测试数据库
    // svc := setupBookmarkService(t)
    
    tests := []struct {
        name    string
        req     CreateBookmarkRequest
        wantErr bool
    }{
        {
            name: "valid bookmark",
            req: CreateBookmarkRequest{
                URL:   "https://example.com",
                Title: "Test",
            },
            wantErr: false,
        },
        {
            name: "empty URL",
            req: CreateBookmarkRequest{
                URL: "",
            },
            wantErr: true,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // bookmark, err := svc.Create(tt.req)
            // if tt.wantErr {
            //     assert.Error(t, err)
            // } else {
            //     assert.NoError(t, err)
            //     assert.NotNil(t, bookmark)
            // }
            _ = assert.Equal // 占位
        })
    }
}
```

### 开发清单

每次添加新模块时，检查以下步骤：

- [ ] 1. 定义数据模型 (`internal/model/`)
- [ ] 2. 注册到 AutoMigrate (`internal/model/db.go`)
- [ ] 3. 创建 Repository (`internal/repository/`)
- [ ] 4. 注册到 Repositories (`internal/repository/repository.go`)
- [ ] 5. 创建 Service (`internal/service/`)
- [ ] 6. 注册到 Services (`internal/service/service.go`)
- [ ] 7. 创建 Handler (`internal/handler/`)
- [ ] 8. 注册到 Handlers (`internal/handler/handler.go`)
- [ ] 9. 注册路由 (`internal/router/router.go`)
- [ ] 10. 添加测试 (`*_test.go`)
- [ ] 11. 更新 API 文档 (`doc/API.md`)
- [ ] 12. 前端页面（如需要）

---

## 测试规范

### 测试文件命名

| 类型 | 命名 | 说明 |
|------|------|------|
| 单元测试 | `*_test.go` | 测试单个函数/方法 |
| 集成测试 | `*_integration_test.go` | 测试多个组件协作 |
| 基准测试 | `*_bench_test.go` | 性能测试 |

### 测试结构

使用 **AAA 模式**（Arrange-Act-Assert）：

```go
func TestFunction(t *testing.T) {
    // Arrange（准备）
    input := "test"
    
    // Act（执行）
    result, err := function(input)
    
    // Assert（断言）
    assert.NoError(t, err)
    assert.Equal(t, "expected", result)
}
```

### 表驱动测试

```go
func TestValidation(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        wantErr bool
    }{
        {"valid input", "hello", false},
        {"empty input", "", true},
        {"too long", strings.Repeat("a", 1000), true},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := validate(tt.input)
            if tt.wantErr {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
            }
        })
    }
}
```

### 覆盖率目标

| 模块 | 最低覆盖率 |
|------|-----------|
| LLM Proxy | 80% |
| Dataset Service | 70% |
| Repository | 60% |
| Handler | 50% |
| 其他 Service | 60% |

### 运行测试

```bash
# 运行全部测试
go test -v ./...

# 运行特定包
go test -v ./internal/service/...

# 生成覆盖率报告
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

---

## 禁止事项

> **以下禁止事项是项目的硬性规则，违反将导致代码审查不通过。**

### 代码禁止事项

#### 1. 禁止忽略错误 ❌

```go
// ❌ 绝对禁止
_ = doSomething()
doSomething()  // 返回 error 被忽略

// ✅ 必须处理
if err := doSomething(); err != nil {
    return fmt.Errorf("failed: %w", err)
}
```

#### 2. 禁止在 init() 中忽略错误 ❌

```go
// ❌ 绝对禁止
func init() {
    logger, _ = zap.NewProduction()
}

// ✅ 使用显式初始化函数
func InitLogger() error {
    var err error
    logger, err = zap.NewProduction()
    return err
}
```

#### 3. 禁止无限制创建 Goroutine ❌

```go
// ❌ 绝对禁止
for _, item := range items {
    go process(item)
}

// ✅ 使用 WaitGroup + 并发控制
var wg sync.WaitGroup
sem := make(chan struct{}, 10)
for _, item := range items {
    wg.Add(1)
    sem <- struct{}{}
    go func(item Item) {
        defer wg.Done()
        defer func() { <-sem }()
        process(item)
    }(item)
}
wg.Wait()
```

#### 4. 禁止硬编码敏感信息 ❌

```go
// ❌ 绝对禁止
apiKey := "sk-1234567890"
password := "admin123"

// ✅ 从环境变量或配置读取
apiKey := os.Getenv("API_KEY")
password := cfg.Database.Password
```

#### 5. 禁止手动拼接 SQL ❌

```go
// ❌ SQL 注入风险
db.Raw("SELECT * FROM users WHERE name = '" + name + "'").Scan(&users)

// ✅ 使用 GORM 查询构建器
db.Where("name = ?", name).Find(&users)
```

#### 6. 禁止直接使用 panic ❌

```go
// ❌ 绝对禁止
if err != nil {
    panic(err)
}

// ✅ 返回错误
if err != nil {
    return fmt.Errorf("critical error: %w", err)
}
```

#### 7. 禁止在循环中执行数据库查询 ❌

```go
// ❌ N+1 查询
for _, id := range ids {
    db.First(&item, id)
}

// ✅ 批量查询
db.Where("id IN ?", ids).Find(&items)
```

#### 8. 禁止 Handler 包含业务逻辑 ❌

```go
// ❌ 业务逻辑泄漏到 Handler
func (h *Handler) Create(c *gin.Context) {
    // 不要在这里写复杂的业务判断
    if someComplexCondition {
        // 复杂处理...
    }
}

// ✅ Handler 只做请求解析和响应
func (h *Handler) Create(c *gin.Context) {
    var req CreateRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        response.BadRequest(c, err.Error())
        return
    }
    result, err := h.svc.Create(req)  // 业务逻辑在 Service 层
    if err != nil {
        response.InternalError(c, err.Error())
        return
    }
    response.Success(c, result)
}
```

### 架构禁止事项

#### 1. 禁止跨层调用 ❌

```go
// ❌ Handler 直接调用 Repository
func (h *Handler) Get(c *gin.Context) {
    item, _ := h.repo.GetByID(id)  // 跳过 Service 层
}

// ✅ 必须经过 Service 层
func (h *Handler) Get(c *gin.Context) {
    item, _ := h.svc.GetByID(id)
}
```

#### 2. 禁止使用全局变量存储状态 ❌

```go
// ❌ 全局可变状态
var cache = map[string]interface{}{}

// ✅ 使用结构体字段
type Service struct {
    cache map[string]interface{}
    mu    sync.RWMutex
}
```

#### 3. 禁止不可停止的后台任务 ❌

```go
// ❌ 无法停止
go func() {
    for {
        time.Sleep(time.Minute)
        doWork()
    }
}()

// ✅ 可通过 channel 停止
go func() {
    ticker := time.NewTicker(time.Minute)
    defer ticker.Stop()
    for {
        select {
        case <-ticker.C:
            doWork()
        case <-stopCh:
            return
        }
    }
}()
```

### 运维禁止事项

#### 1. 禁止直接操作生产数据库 ❌

所有数据操作必须通过 Repository 层。

#### 2. 禁止在生产环境使用 Debug 模式 ❌

```yaml
# ❌ 生产环境
server:
  mode: debug

# ✅ 生产环境
server:
  mode: release
```

#### 3. 禁止 git add -f 强制添加被忽略的文件 ❌

`.gitignore` 中的规则是有意设置的。

#### 4. 禁止 docker compose down 停全部服务 ❌

```bash
# ❌ 会停全部服务
docker compose down

# ✅ 只重启指定服务
./spool.sh restart bellkeeper
```

---

## 常见问题

### Q1: 如何添加新的配置项？

1. 在 `internal/config/config.go` 的 `Config` 结构体中添加字段
2. 在 `config/bellkeeper.yaml` 中添加默认值
3. 如果是敏感信息，使用 `${ENV_VAR}` 语法

```go
// config.go
type Config struct {
    // ...
    NewModule NewModuleConfig `mapstructure:"new_module"`
}

type NewModuleConfig struct {
    Enabled bool   `mapstructure:"enabled"`
    APIKey  string `mapstructure:"api_key"`
}
```

```yaml
# bellkeeper.yaml
new_module:
  enabled: true
  api_key: ${NEW_MODULE_API_KEY}
```

### Q2: 如何添加新的环境变量？

1. 在 `.env` 文件中添加变量
2. 在 `bellkeeper-init.sh` 中添加 `export`
3. 使用 `spool sync push` 同步到远程
4. 重启服务

### Q3: 如何调试 LLM Proxy？

```bash
# 查看渠道状态
curl http://localhost:8080/api/llm/channels/status -H "X-API-Key: your-key"

# 查看模型组状态
curl http://localhost:8080/api/llm/groups/status -H "X-API-Key: your-key"

# 查看请求日志
curl http://localhost:8080/api/llm/logs?limit=10 -H "X-API-Key: your-key"

# 重置熔断器
curl -X POST http://localhost:8080/api/llm/channels/channel-name/reset \
  -H "X-API-Key: your-key"
```

### Q4: 如何处理数据库迁移？

当前使用 AutoMigrate，修改模型后重启即自动迁移。

**注意**: AutoMigrate 只会添加列、索引，不会删除或修改现有列。
如需删除或修改列，需要手动编写 SQL。

### Q5: 如何部署到生产环境？

```bash
# 在 SilkSpool 目录
cd /home/ubuntu/SilkSpool

# 重启 Bellkeeper
./spool.sh restart bellkeeper

# 查看日志
./spool.sh logs bellkeeper 100

# 查看状态
./spool.sh status bellkeeper
```

---

## 验收标准

### 代码质量

| 项目 | 标准 | 检查方式 |
|------|------|---------|
| 编译通过 | 无编译错误和警告 | `go build ./...` |
| 测试通过 | 所有测试通过 | `go test ./...` |
| 覆盖率 | 核心模块 > 60% | `go test -cover` |
| 代码规范 | 通过 golint/golangci-lint | `make lint` |
| 无数据竞争 | 通过 race detector | `go test -race ./...` |

### 功能验收

| 项目 | 标准 | 检查方式 |
|------|------|---------|
| API 正常 | 所有端点返回正确响应 | HTTP 请求测试 |
| 错误处理 | 错误响应格式正确 | 错误场景测试 |
| 认证 | API Key 和 Authelia 都正常 | 认证测试 |
| 分页 | 列表 API 分页正常 | 分页参数测试 |
| 并发安全 | 无数据竞争 | `go test -race` |

### 文档验收

| 项目 | 标准 |
|------|------|
| API 文档 | 所有新 API 在 API.md 中有记录 |
| 架构文档 | 重大变更更新 ARCHITECTURE.md |
| 配置文档 | 新配置项有说明 |
| 代码注释 | 公开函数有 godoc 注释 |

### 安全验收

| 项目 | 标准 |
|------|------|
| 无硬编码密钥 | 敏感信息从环境变量读取 |
| 输入验证 | 所有输入经过验证 |
| SQL 注入 | 使用 GORM 查询构建器 |
| XSS 防护 | HTML 输出经过清理 |

---

## 继续开发计划

### Phase 6: Matrix 前端界面（优先级 P0）

| 任务 | 工作量 | 验收标准 | 状态 |
|------|--------|---------|------|
| 类型定义和 API 客户端 | 1 天 | 类型完整，API 可用 | ✅ |
| Matrix 总览页 | 1 天 | 统计卡片+事件列表 | ✅ |
| 房间管理页 | 1 天 | CRUD 正常 | ✅ |
| 频道管理页 | 1 天 | CRUD 正常 | ✅ |
| 命令管理页 | 1 天 | CRUD+测试功能 | ✅ |
| 通知管理页 | 1 天 | 列表+筛选+重试 | ✅ |
| 事件日志页 | 1 天 | 列表+筛选 | ✅ |
| 命令日志页 | 1 天 | 列表+详情 | ✅ |
| 导航菜单+路由 | 1 天 | 页面可访问 | ✅ |

### Phase 1: 基础设施修复（优先级 P0）

| 任务 | 工作量 | 前置条件 | 验收标准 | 状态 |
|------|--------|---------|---------|------|
| 引入 testify 库 | 1 天 | 无 | `go test` 可运行 | ✅ |
| 修复 logger.go init() 错误 | 0.5 天 | 无 | 不忽略错误 | ✅ |
| 修复 activity_log 异步错误 | 0.5 天 | 无 | 错误被记录 | ✅ |
| 统一错误类型定义 | 1 天 | 无 | internal/pkg/errors/ | ✅ |
| Goroutine 生命周期管理 | 2 天 | 无 | 所有 goroutine 可停止 | ⚠️ |
| 优雅关闭完善 | 1 天 | 上项 | Matrix/NATS/Redis 优雅关闭 | ✅ |
| LLM Proxy 单元测试 | 3 天 | testify | 覆盖率 > 80% | ⏳ |
| Dataset Service 测试 | 2 天 | testify | 覆盖率 > 70% | ⏳ |

### Phase 2: 文件入库模块（优先级 P1）

| 任务 | 工作量 | 前置条件 | 验收标准 | 状态 |
|------|--------|---------|---------|------|
| ExtractorService 实现 | 3 天 | 无 | Trafilatura + Firecrawl 可用 | ✅ |
| FileIngestionService 实现 | 3 天 | ExtractorService | URL → 文件落地正常 | ✅ |
| 文件入库 API 完善 | 2 天 | Service 完成 | API 端点正常 | ⚠️ |
| 与 n8n 工作流集成 | 2 天 | API 完成 | K01/K02 切换完成 | ⏳ |
| 单元测试 | 2 天 | Service 完成 | 覆盖率 > 60% | ⏳ |

### Phase 3: Matrix 平台完善（优先级 P1）

| 任务 | 工作量 | 前置条件 | 验收标准 | 状态 |
|------|--------|---------|---------|------|
| 评估并简化设计文档 | 2 天 | 无 | 设计与实际匹配 | ✅ |
| 权限引擎实现 | 3 天 | 设计评估 | 基础权限校验可用 | ❌ |
| 通知网关完善 | 3 天 | 设计评估 | 通知发送正常 | ✅ |
| 命令路由完善 | 2 天 | 权限引擎 | 命令执行正常 | ✅ |
| Admin API | 2 天 | 上述完成 | 管理接口可用 | ✅ |
| 用户角色管理 | 1 天 | 权限引擎 | 角色 API 可用 | ✅ |
| 单元测试 | 2 天 | 上述完成 | 覆盖率 > 60% | ⏳ |

### Phase 4: 监控与运维（优先级 P2）

| 任务 | 工作量 | 前置条件 | 验收标准 | 状态 |
|------|--------|---------|---------|------|
| Prometheus metrics | 3 天 | 无 | /metrics 端点可用 | ✅ |
| 健康检查改进 | 1 天 | 无 | readiness/liveness 区分 | ✅ |
| 日志改进 | 1 天 | 无 | 日志级别可动态调整 | ✅ |
| 配置热重载 | 2 天 | 无 | 配置变更无需重启 | ✅ |

### Phase 5: 数据库与安全（优先级 P2）

| 任务 | 工作量 | 前置条件 | 验收标准 | 状态 |
|------|--------|---------|---------|------|
| 引入 golang-migrate | 2 天 | 无 | 版本化迁移可用 | ❌ |
| API Key 常量时间比较 | 0.5 天 | 无 | 安全加固 | ✅ |
| 请求限流中间件 | 1 天 | 无 | 限流可配置 | ✅ |
| HTML 清理（防 XSS） | 1 天 | 无 | 输出安全 | ✅ |

---

## 附录

### A. 统一响应格式参考

```go
// internal/pkg/response/response.go

package response

import (
    "github.com/gin-gonic/gin"
    "net/http"
)

func Success(c *gin.Context, data interface{}) {
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "data":    data,
    })
}

func SuccessList(c *gin.Context, items interface{}, total int64, page, pageSize int) {
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "data": gin.H{
            "items":     items,
            "total":     total,
            "page":      page,
            "page_size": pageSize,
        },
    })
}

func BadRequest(c *gin.Context, message string) {
    c.JSON(http.StatusBadRequest, gin.H{
        "success": false,
        "error":   message,
    })
}

func NotFound(c *gin.Context, message string) {
    c.JSON(http.StatusNotFound, gin.H{
        "success": false,
        "error":   message,
    })
}

func InternalError(c *gin.Context, message string) {
    c.JSON(http.StatusInternalServerError, gin.H{
        "success": false,
        "error":   message,
    })
}
```

### B. 配置文件结构参考

```yaml
# config/bellkeeper.yaml

server:
  host: "0.0.0.0"
  port: 8080
  mode: "debug"       # debug / release
  api_key: "${BELLKEEPER_API_KEY}"
  shutdown_timeout: 10

database:
  host: "localhost"
  port: 5432
  user: "bellkeeper"
  password: "${DB_PASSWORD}"
  dbname: "bellkeeper"
  sslmode: "disable"

logging:
  level: "info"        # debug / info / warn / error
  format: "json"       # json / text
  output: "stdout"     # stdout / file

llm_proxy:
  enabled: true
  default_timeout: 60
  max_retries: 3
  default_bucket_rpm: 60

ragflow:
  base_url: "http://sp-ragflow:9380"
  api_key: "${RAGFLOW_API_KEY}"

n8n:
  base_url: "http://sp-n8n:5678"
  api_key: "${N8N_API_KEY}"
  webhook_base_url: "http://sp-n8n:5678/webhook"

matrix:
  homeserver_url: "https://matrix.singll.net"
  bot_user_id: "@bellkeeper:matrix.singll.net"
  bot_access_token: "${MATRIX_BOT_TOKEN}"
  device_id: "BELLKEEPER"
  max_retry: 3

redis:
  host: "sp-redis"
  port: 6379
  password: ""
  db: 0

nats:
  url: "nats://sp-nats:4222"
  stream_name: "BELLKEEPER"

file_ingestion:
  enabled: true
  base_path: "/mnt/knowledge"
  trafilatura:
    enabled: true
    timeout: 15
  firecrawl:
    enabled: true
    fallback_only: true
    api_key: "${FIRECRAWL_API_KEY}"
```

### C. 有用的 Git 命令

```bash
# 查看最近的提交
git log --oneline -20

# 查看某个文件的修改历史
git log --follow -p -- internal/service/llm_proxy.go

# 查看谁修改了某一行
git blame internal/service/llm_proxy.go

# 创建功能分支
git checkout -b feature/bookmark-module

# 合并分支
git checkout main
git merge feature/bookmark-module
```

### D. 常用开发命令

```bash
# Go
go build ./...             # 编译
go test ./...              # 测试
go test -race ./...        # 竞争检测
go test -cover ./...       # 覆盖率
go vet ./...               # 静态分析
go mod tidy                # 整理依赖

# Docker
docker compose up -d       # 启动
docker compose logs -f     # 日志
docker compose restart     # 重启
docker compose exec app sh # 进入容器

# 前端
cd web
npm run dev                # 开发
npm run build              # 构建
npm run lint               # 检查
```
