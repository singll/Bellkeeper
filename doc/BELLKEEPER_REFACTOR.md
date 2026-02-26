# Knowledge Management 重构计划

> **项目代号**: Bellkeeper (钟守者)
>
> **文档版本**: v2.0
>
> **创建时间**: 2026-02-12
>
> **完成时间**: 2026-02-26
>
> **目标**: 将 knowledge-management 重构为企业级中大型项目
>
> **状态**: ✅ 重构完成
>
> **仓库地址**: 独立仓库，与 SilkSpool 平级

---

## 目录

1. [现有项目分析](#1-现有项目分析)
2. [技术栈选择](#2-技术栈选择)
3. [项目命名方案](#3-项目命名方案)
4. [项目组织结构](#4-项目组织结构)
5. [认证方案评估](#5-认证方案评估)
6. [配置管理方案](#6-配置管理方案)
7. [功能扩展规划](#7-功能扩展规划)
8. [架构设计](#8-架构设计)
9. [实施计划](#9-实施计划)

---

## 1. 现有项目分析

### 1.1 当前技术栈

| 层级 | 技术 | 版本 | 评价 |
|------|------|------|------|
| **后端框架** | Python Flask | 3.0 | 轻量但扩展性一般 |
| **ORM** | SQLAlchemy | - | 功能完善 |
| **数据库** | SQLite | - | 不适合中大型项目 |
| **前端框架** | Vue 3 | 3.4.21 | 现代化，生态丰富 |
| **UI组件库** | Element Plus | 2.6.3 | 功能完善但稍显笨重 |
| **构建工具** | Vite | 5.2.8 | 快速，现代 |
| **包管理** | uv/pnpm | - | 优秀 |

### 1.2 现有功能模块

```
knowledge-management/
├── RagFlow 文档管理
│   ├── 字符串内容上传
│   ├── 批量上传
│   ├── 文档列表/删除
│   ├── 解析状态追踪
│   └── URL 去重检查
├── 数据源管理
│   ├── CRUD 操作
│   ├── 分类筛选
│   └── 标签关联
├── RSS 订阅管理
│   ├── CRUD 操作
│   └── 抓取时间记录
├── 标签系统
│   ├── 统一标签管理
│   └── 自定义颜色
├── Webhook 工作流
│   ├── 多 Webhook 配置
│   ├── 手动触发
│   └── 调用历史
└── 知识库映射
    ├── Dataset 映射配置
    ├── 标签关联
    └── 默认知识库
```

### 1.3 数据模型

```
tags (标签)
├── id, name, description, color
└── 多对多: data_sources, rss_feeds, datasets

data_sources (数据源)
├── id, name, url, type, category
└── is_active, tags[]

rss_feeds (RSS订阅)
├── id, name, url, category
└── is_active, last_fetched, tags[]

webhook_configs (Webhook配置)
├── id, name, url, method, content_type
└── headers, body_template, timeout

webhook_history (Webhook历史)
├── id, webhook_id, payload, status
└── response_code, response_body, duration

dataset_mappings (知识库映射)
├── id, name, display_name, dataset_id
└── is_default, parser_id, tags[]

article_tags (文章标签关联)
├── id, document_id, dataset_id, tag_id
└── article_title, article_url
```

### 1.4 当前问题

| 问题 | 影响 | 优先级 |
|------|------|--------|
| SQLite 不支持并发 | 多用户场景性能差 | 高 |
| Flask 单线程模型 | 高并发时响应慢 | 高 |
| 无认证系统 | 安全隐患 | 高 |
| API Key 硬编码在环境变量 | 运维不便 | 中 |
| 无服务状态监控 | 故障发现慢 | 中 |
| 前端 Element Plus 体积大 | 加载速度慢 | 低 |

---

## 2. 技术栈选择

### 2.1 后端语言对比: Go vs Rust

| 维度 | Go | Rust | 推荐 |
|------|-----|------|------|
| **学习曲线** | 简单，1-2周上手 | 陡峭，1-3月上手 | Go |
| **Claude Code 支持** | 优秀，生成代码质量高 | 良好，但需更多提示 | Go |
| **并发模型** | Goroutine，简单高效 | async/await，更底层 | Go |
| **编译速度** | 快 (1-5秒) | 慢 (30秒-数分钟) | Go |
| **内存安全** | GC 管理 | 编译时保证 | Rust |
| **性能** | 优秀 | 极致 | Rust |
| **生态成熟度** | 非常成熟 | 快速发展中 | Go |
| **运维友好度** | 单二进制，简单 | 单二进制，简单 | 平手 |
| **Web框架** | Gin/Fiber/Echo | Axum/Actix | Go |
| **社区资源** | 丰富 | 相对较少 | Go |

#### 2.1.1 Go 优势

```go
// 1. 简洁的并发模型
go func() {
    result := fetchData(url)
    ch <- result
}()

// 2. 清晰的错误处理
data, err := service.FetchDocument(id)
if err != nil {
    return nil, fmt.Errorf("fetch document: %w", err)
}

// 3. 内置 HTTP 服务器
http.HandleFunc("/api/health", healthHandler)
http.ListenAndServe(":8080", nil)
```

#### 2.1.2 Rust 优势

```rust
// 1. 内存安全保证
fn process_document(doc: &Document) -> Result<String, Error> {
    // 编译时防止悬垂指针、数据竞争
}

// 2. 零成本抽象
#[derive(Serialize, Deserialize)]
struct Document {
    id: String,
    content: String,
}

// 3. 强类型系统
enum DocumentStatus {
    Pending,
    Processing,
    Completed,
    Failed(String),
}
```

### 2.2 最终推荐: **Go**

**理由**:

1. **Claude Code 亲和度**: Go 代码生成质量高，模式固定，Claude 能写出生产级代码
2. **开发效率**: 快速迭代，编译即运行
3. **运维友好**: 单二进制部署，Docker 镜像小
4. **并发原生**: Goroutine 天然适合 API 服务
5. **生态完善**: Gin、GORM、Swagger 等成熟工具链

### 2.3 Go Web 框架选择

| 框架 | 性能 | 特点 | 推荐度 |
|------|------|------|--------|
| **Gin** | 高 | 最流行，中间件丰富 | ⭐⭐⭐⭐⭐ |
| Fiber | 极高 | Express 风格，基于 fasthttp | ⭐⭐⭐⭐ |
| Echo | 高 | 简洁，自带文档生成 | ⭐⭐⭐⭐ |
| Chi | 高 | 轻量，标准库兼容 | ⭐⭐⭐ |

**推荐: Gin**

```go
// Gin 示例
r := gin.Default()

r.GET("/api/tags", handlers.ListTags)
r.POST("/api/tags", handlers.CreateTag)
r.PUT("/api/tags/:id", handlers.UpdateTag)
r.DELETE("/api/tags/:id", handlers.DeleteTag)

r.Run(":8080")
```

### 2.4 前端框架选择

| 框架 | 性能 | 与 Go 契合度 | Claude 支持 | 推荐度 |
|------|------|--------------|-------------|--------|
| **SolidJS** | 极高 | 优秀 | 良好 | ⭐⭐⭐⭐⭐ |
| Vue 3 | 高 | 优秀 | 优秀 | ⭐⭐⭐⭐ |
| React | 高 | 优秀 | 优秀 | ⭐⭐⭐⭐ |
| Svelte | 极高 | 优秀 | 良好 | ⭐⭐⭐⭐ |

**推荐: SolidJS + TailwindCSS**

**理由**:

1. **极致性能**: 无 Virtual DOM，直接 DOM 操作
2. **小体积**: 压缩后 ~7KB，比 Vue/React 小很多
3. **类 React 语法**: 熟悉度高，学习成本低
4. **响应式原语**: 细粒度更新，性能优异
5. **与 Go 契合**: 都追求简洁高效

```tsx
// SolidJS 示例
import { createSignal, For } from 'solid-js'

function TagList() {
  const [tags, setTags] = createSignal([])

  onMount(async () => {
    const data = await fetchTags()
    setTags(data)
  })

  return (
    <div class="grid gap-4">
      <For each={tags()}>
        {tag => <TagCard tag={tag} />}
      </For>
    </div>
  )
}
```

### 2.5 CSS 框架选择

| 框架 | 性能 | 开发体验 | 推荐度 |
|------|------|----------|--------|
| **TailwindCSS** | 极高 | 优秀 | ⭐⭐⭐⭐⭐ |
| UnoCSS | 极高 | 优秀 | ⭐⭐⭐⭐⭐ |
| Pico CSS | 高 | 简单 | ⭐⭐⭐ |

**推荐: TailwindCSS**

**理由**:

1. 按需生成，生产构建极小
2. 原子化 CSS，复用性强
3. 与 SolidJS 完美配合
4. Claude 生成质量高

### 2.6 数据库选择

| 数据库 | 性能 | 运维复杂度 | 推荐度 |
|--------|------|------------|--------|
| **PostgreSQL** | 极高 | 中 | ⭐⭐⭐⭐⭐ |
| MySQL | 高 | 中 | ⭐⭐⭐⭐ |
| SQLite | 中 | 极低 | ⭐⭐⭐ |

**推荐: PostgreSQL**

**理由**:

1. JSONB 支持，灵活存储元数据
2. 全文检索内置
3. 并发性能优异
4. 与 Go 的 GORM/sqlx 完美配合

### 2.7 技术栈总结

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        重构后技术栈                                          │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│   后端                                   前端                                 │
│   ┌─────────────────────────────┐       ┌─────────────────────────────┐     │
│   │ Go 1.22+                     │       │ SolidJS 1.8+                │     │
│   │ ├── Gin (Web框架)            │       │ ├── solid-router (路由)     │     │
│   │ ├── GORM (ORM)               │       │ ├── TailwindCSS (样式)      │     │
│   │ ├── Viper (配置管理)         │       │ ├── Vite (构建)             │     │
│   │ ├── Zap (日志)               │       │ └── TypeScript              │     │
│   │ └── Swagger (API文档)        │       │                              │     │
│   └─────────────────────────────┘       └─────────────────────────────┘     │
│                                                                              │
│   数据层                                 基础设施                            │
│   ┌─────────────────────────────┐       ┌─────────────────────────────┐     │
│   │ PostgreSQL 16               │       │ Docker                       │     │
│   │ ├── 主数据存储               │       │ ├── docker-compose          │     │
│   │ └── JSONB 元数据             │       │ └── 多阶段构建              │     │
│   │                              │       │                              │     │
│   │ Redis (可选)                 │       │ Nginx / Caddy               │     │
│   │ └── 缓存/Session             │       │ └── 反向代理                │     │
│   └─────────────────────────────┘       └─────────────────────────────┘     │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 3. 项目命名方案

### 3.1 命名原则

遵循 SilkSpool 命名风格，结合《空洞骑士》和《空洞骑士：丝之歌》世界观:

- 使用游戏中的地点、角色、道具名称
- 体现"知识"、"记录"、"织网"等概念
- 简洁易记，便于 CLI 使用

### 3.2 最终命名: **Bellkeeper** (钟守者)

| 属性 | 说明 |
|------|------|
| **英文名** | Bellkeeper |
| **中文名** | 钟守者 |
| **来源** | 灵感来自游戏中的钟塔元素 |
| **含义** | 记录时间、守护知识、提醒与告警 |
| **CLI** | `bellkeeper start`, `bellkeeper status` |

**选择理由**:

1. **记录与提醒**: 钟代表时间记录，契合知识管理的"记录"属性
2. **守护者**: Keeper 暗示对知识的守护和管理
3. **告警功能**: 钟声可以提醒，呼应系统监控和通知功能
4. **独特性**: 在开源社区中较少见，易于搜索
5. **CLI 友好**: 简短好记

### 3.3 项目结构

```
Bellkeeper/                      # 独立 Git 仓库
├── cmd/
│   └── bellkeeper/
│       └── main.go              # 入口: bellkeeper serve
│
├── internal/
│   ├── handler/                 # API 处理器
│   ├── service/                 # 业务逻辑
│   ├── repository/              # 数据访问
│   ├── model/                   # 数据模型
│   └── middleware/              # 中间件
│
├── web/                         # 前端 (SolidJS)
│   └── ...
│
├── config/
│   └── bellkeeper.yaml
│
└── docker/
    └── docker-compose.yml
```

### 3.4 内部模块命名

| 模块 | 命名 | 来源 | 说明 |
|------|------|------|------|
| 标签系统 | Seal | 游戏中的印记 | 标记与分类 |
| 数据源管理 | Fountain | 泪之城的喷泉 | 信息源头 |
| RSS 订阅 | Echo | 回声 | 消息回响 |
| Webhook | Thread | 丝线 | 连接触发 |
| 知识库映射 | Cocoon | 茧 | 知识包裹 |
| 健康监控 | Pulse | 脉搏 | 心跳检测 |
| 设置管理 | Chime | 钟声 | 配置调节 |

---

## 4. 项目组织结构

### 4.1 目录布局方案

Bellkeeper 作为独立项目，应与 SilkSpool **平级存放**，而非嵌套在 SilkSpool 内部。

```
/home/ubuntu/
├── SilkSpool/                    # 基础设施项目 (现有)
│   ├── bundles/
│   ├── lib/
│   ├── doc/
│   │   └── KNOWLEDGE_MANAGEMENT_REFACTOR.md  # 本文档
│   └── knowledge-management/     # 旧版 (重构后移除)
│
├── Bellkeeper/                   # 知识管理系统 (新建独立仓库)
│   ├── cmd/
│   ├── internal/
│   ├── web/
│   ├── config/
│   ├── docker/
│   ├── go.mod
│   └── README.md
│
└── hollownest.code-workspace     # VSCode 多根工作区文件
```

### 4.2 VSCode 多根工作区配置

创建 `/home/ubuntu/hollownest.code-workspace` 文件，实现一个 VSCode 窗口管理多个项目:

```json
{
  "folders": [
    {
      "name": "SilkSpool",
      "path": "SilkSpool"
    },
    {
      "name": "Bellkeeper",
      "path": "Bellkeeper"
    }
  ],
  "settings": {
    "files.exclude": {
      "**/node_modules": true,
      "**/.git": true,
      "**/dist": true
    },
    "search.exclude": {
      "**/node_modules": true,
      "**/dist": true
    }
  },
  "extensions": {
    "recommendations": [
      "golang.go",
      "bradlc.vscode-tailwindcss",
      "dbaeumer.vscode-eslint"
    ]
  }
}
```

### 4.3 使用方式

#### 4.3.1 打开工作区

```bash
# 方式一: 命令行打开
code /home/ubuntu/hollownest.code-workspace

# 方式二: VSCode 菜单
# File → Open Workspace from File → 选择 hollownest.code-workspace
```

#### 4.3.2 Claude Code 工作

Claude Code 支持工作区模式，在工作区中可以:
- 同时访问两个项目的文件
- 在项目间搜索和引用代码
- 独立管理每个项目的 `.claude/` 配置

```bash
# 在工作区根目录启动 Claude Code
cd /home/ubuntu
claude

# Claude 会识别 .code-workspace 文件
# 可以访问 SilkSpool/ 和 Bellkeeper/ 两个项目
```

#### 4.3.3 Git 仓库独立管理

```bash
# SilkSpool 仓库
cd /home/ubuntu/SilkSpool
git remote -v  # 现有远程仓库

# Bellkeeper 仓库 (新建)
cd /home/ubuntu/Bellkeeper
git init
git remote add origin git@github.com:yourname/Bellkeeper.git
```

### 4.4 项目关系

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        项目关系图                                            │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│   ┌─────────────────────┐         ┌─────────────────────┐                   │
│   │     SilkSpool       │         │     Bellkeeper      │                   │
│   │   (基础设施层)       │         │   (知识管理层)       │                   │
│   ├─────────────────────┤         ├─────────────────────┤                   │
│   │ • DNS/网络配置       │ ──────▶ │ • 知识库管理         │                   │
│   │ • Docker 编排       │ 提供    │ • 数据源/RSS         │                   │
│   │ • Caddy/Authelia   │ 基础设施 │ • RagFlow 集成       │                   │
│   │ • 部署模板          │         │ • 标签系统           │                   │
│   │ • n8n 工作流        │         │ • Webhook 管理       │                   │
│   └─────────────────────┘         └─────────────────────┘                   │
│            │                                 │                               │
│            │                                 │                               │
│            ▼                                 ▼                               │
│   ┌─────────────────────────────────────────────────────────────────────┐   │
│   │                         共享服务                                     │   │
│   │  • RagFlow (知识库引擎)                                              │   │
│   │  • n8n (工作流编排)                                                  │   │
│   │  • PostgreSQL (数据存储)                                             │   │
│   │  • Authelia (统一认证)                                               │   │
│   └─────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 4.5 为什么选择独立仓库

| 因素 | 嵌套在 SilkSpool | 独立仓库 (推荐) |
|------|------------------|-----------------|
| **版本控制** | 混合提交历史 | 独立提交历史 |
| **CI/CD** | 复杂的路径过滤 | 简单触发 |
| **权限管理** | 无法分离 | 可独立设置 |
| **依赖管理** | 可能冲突 | 完全隔离 |
| **代码复用** | 直接引用 | 通过 API/包引用 |
| **团队协作** | 容易冲突 | 独立迭代 |

### 4.6 项目初始化步骤

```bash
# 1. 创建 Bellkeeper 项目目录
mkdir -p /home/ubuntu/Bellkeeper
cd /home/ubuntu/Bellkeeper

# 2. 初始化 Git 仓库
git init

# 3. 初始化 Go 模块
go mod init github.com/yourname/bellkeeper

# 4. 创建基础目录结构
mkdir -p cmd/bellkeeper internal/{handler,service,repository,model,middleware} \
         web/src config docker migrations

# 5. 创建 .gitignore
cat > .gitignore << 'EOF'
# Go
*.exe
*.exe~
*.dll
*.so
*.dylib
*.test
*.out
go.work

# IDE
.idea/
.vscode/
*.swp
*.swo

# Build
/bin/
/dist/

# Config
*.local.yaml
.env
.env.local

# Database
*.db
*.sqlite

# Frontend
web/node_modules/
web/dist/
EOF

# 6. 创建工作区文件
cat > /home/ubuntu/hollownest.code-workspace << 'EOF'
{
  "folders": [
    { "name": "SilkSpool", "path": "SilkSpool" },
    { "name": "Bellkeeper", "path": "Bellkeeper" }
  ],
  "settings": {
    "go.useLanguageServer": true,
    "files.exclude": {
      "**/node_modules": true
    }
  }
}
EOF

# 7. 初次提交
git add .
git commit -m "Initial commit: Bellkeeper project scaffold"
```

---

## 5. 认证方案评估

### 5.1 是否需要登录功能？

#### 5.1.1 使用场景分析

| 场景 | 需要登录 | 理由 |
|------|----------|------|
| **纯内网部署** | 可选 | 已有网络隔离保护 |
| **Tailscale/VPN 访问** | 可选 | VPN 已提供认证层 |
| **公网暴露** | 必须 | 防止未授权访问 |
| **多用户共享** | 必须 | 区分用户权限 |
| **敏感数据存储** | 建议 | 即使内网也需保护 |

#### 5.1.2 结论

根据 PERSONAL_SYSTEM_SOLUTION.md 中的架构:
- 系统已部署 Authelia 作为 SSO
- 所有服务都通过 Caddy forward_auth 保护
- 建议: **不自建登录，集成 Authelia**

### 5.2 Authelia 集成方案

#### 5.2.1 集成方式对比

| 方式 | 复杂度 | 安全性 | 推荐度 |
|------|--------|--------|--------|
| **Forward Auth (门禁)** | 低 | 高 | ⭐⭐⭐⭐⭐ |
| OIDC 完整集成 | 高 | 最高 | ⭐⭐⭐⭐ |
| Header 信任 | 最低 | 中 | ⭐⭐⭐ |

#### 5.2.2 推荐: Forward Auth + Header 信任

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         Authelia 集成方案                                     │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│   用户访问 https://bellkeeper.singll.net                                    │
│                     │                                                        │
│                     ▼                                                        │
│   ┌─────────────────────────────────────────────────────────────────────┐   │
│   │                         Caddy                                        │   │
│   │   ┌───────────────────────────────────────────────────────────┐     │   │
│   │   │              forward_auth → Authelia                      │     │   │
│   │   │                                                           │     │   │
│   │   │   已登录？ ─── 是 ──→ 转发请求到 Bellkeeper               │     │   │
│   │   │      │                 + 附加 Header:                     │     │   │
│   │   │      │                   X-Remote-User: admin             │     │   │
│   │   │      │                   X-Remote-Email: admin@x.com      │     │   │
│   │   │      │                   X-Remote-Groups: admins          │     │   │
│   │   │      │                                                     │     │   │
│   │   │      └─── 否 ──→ 302 重定向到 https://auth.singll.net     │     │   │
│   │   └───────────────────────────────────────────────────────────┘     │   │
│   └─────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
│                                  │                                           │
│                                  ▼                                           │
│   ┌─────────────────────────────────────────────────────────────────────┐   │
│   │                       Bellkeeper Backend                             │   │
│   │                                                                      │   │
│   │   func authMiddleware(c *gin.Context) {                              │   │
│   │       user := c.GetHeader("X-Remote-User")                          │   │
│   │       if user == "" {                                                │   │
│   │           c.AbortWithStatus(401)                                     │   │
│   │           return                                                     │   │
│   │       }                                                              │   │
│   │       c.Set("user", user)                                           │   │
│   │       c.Next()                                                       │   │
│   │   }                                                                  │   │
│   └─────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

#### 5.2.3 Caddy 配置示例

```caddyfile
bellkeeper.singll.net {
    import common
    import authelia

    reverse_proxy http://192.168.7.220:8080 {
        header_up X-Remote-User {http.authelia.user}
        header_up X-Remote-Email {http.authelia.email}
        header_up X-Remote-Name {http.authelia.name}
        header_up X-Remote-Groups {http.authelia.groups}
    }
}
```

#### 5.2.4 Go 中间件实现

```go
package middleware

import (
    "net/http"
    "github.com/gin-gonic/gin"
)

type UserInfo struct {
    Username string   `json:"username"`
    Email    string   `json:"email"`
    Name     string   `json:"name"`
    Groups   []string `json:"groups"`
}

func AutheliaAuth() gin.HandlerFunc {
    return func(c *gin.Context) {
        user := c.GetHeader("X-Remote-User")

        // 开发模式跳过认证
        if gin.Mode() == gin.DebugMode && user == "" {
            user = "dev-user"
        }

        if user == "" {
            c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
                "error": "Unauthorized",
                "message": "Please login via Authelia",
            })
            return
        }

        userInfo := UserInfo{
            Username: user,
            Email:    c.GetHeader("X-Remote-Email"),
            Name:     c.GetHeader("X-Remote-Name"),
            Groups:   parseGroups(c.GetHeader("X-Remote-Groups")),
        }

        c.Set("user", userInfo)
        c.Next()
    }
}

func parseGroups(header string) []string {
    if header == "" {
        return []string{}
    }
    return strings.Split(header, ",")
}
```

### 5.3 外网暴露需求评估

| 因素 | 分析 |
|------|------|
| **当前架构** | 已通过 Tailscale + Caddy 统一入口 |
| **公网需求** | 无必要，Tailscale 可全球访问 |
| **安全风险** | 公网暴露增加攻击面 |
| **性能影响** | 无明显差异 |

**结论**: 无需公网暴露，Tailscale + Authelia 方案已满足需求

---

## 6. 配置管理方案

### 6.1 RagFlow API Key 配置

#### 6.1.1 当前问题

- API Key 存储在环境变量中
- 修改需要重启服务
- 无法通过 Web UI 管理

#### 6.1.2 推荐方案: 分层配置

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        配置管理分层架构                                       │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│   优先级 (高 → 低)                                                           │
│                                                                              │
│   1. 数据库配置 (最高优先级，可通过 Web UI 修改)                              │
│      └── settings 表: api_keys, preferences, feature_flags                  │
│                                                                              │
│   2. 环境变量 (容器启动时设置)                                               │
│      └── RAGFLOW_API_KEY, DATABASE_URL, etc.                                │
│                                                                              │
│   3. 配置文件 (yaml/toml，版本控制友好)                                      │
│      └── config/bellkeeper.yaml                                             │
│                                                                              │
│   4. 默认值 (代码内置)                                                       │
│      └── config/defaults.go                                                 │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

#### 6.1.3 数据库配置表设计

```sql
CREATE TABLE settings (
    id SERIAL PRIMARY KEY,
    key VARCHAR(255) UNIQUE NOT NULL,
    value TEXT,
    value_type VARCHAR(50) DEFAULT 'string', -- string, int, bool, json
    category VARCHAR(100),                    -- api, feature, ui
    description TEXT,
    is_secret BOOLEAN DEFAULT false,          -- 是否敏感信息
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- 示例数据
INSERT INTO settings (key, value, value_type, category, description, is_secret) VALUES
('ragflow.api_key', 'encrypted_value', 'string', 'api', 'RagFlow API Key', true),
('ragflow.base_url', 'http://ragflow:9380', 'string', 'api', 'RagFlow Base URL', false),
('n8n.webhook_base_url', 'http://n8n:5678', 'string', 'api', 'n8n Webhook Base URL', false),
('feature.auto_parse', 'true', 'bool', 'feature', '自动解析上传文档', false),
('ui.theme', 'dark', 'string', 'ui', 'UI 主题', false);
```

#### 6.1.4 Go 配置管理器

```go
package config

import (
    "github.com/spf13/viper"
    "gorm.io/gorm"
)

type Config struct {
    db    *gorm.DB
    viper *viper.Viper
}

// 获取配置值，优先级: DB > ENV > File > Default
func (c *Config) Get(key string) string {
    // 1. 尝试从数据库获取
    var setting Setting
    if err := c.db.Where("key = ?", key).First(&setting).Error; err == nil {
        return setting.Value
    }

    // 2. 尝试从 Viper (ENV + File) 获取
    if val := c.viper.GetString(key); val != "" {
        return val
    }

    // 3. 返回默认值
    return defaults[key]
}

// 设置配置值到数据库
func (c *Config) Set(key, value string) error {
    return c.db.Save(&Setting{
        Key:       key,
        Value:     value,
        UpdatedAt: time.Now(),
    }).Error
}
```

#### 6.1.5 Web UI 设置页面

```
设置页面
├── API 配置
│   ├── RagFlow
│   │   ├── Base URL: [________________]
│   │   └── API Key:  [******修改******]
│   ├── n8n
│   │   └── Webhook URL: [________________]
│   └── Firecrawl
│       └── API Key:  [******修改******]
│
├── 功能开关
│   ├── [✓] 自动解析上传文档
│   ├── [✓] URL 去重检查
│   └── [ ] 启用 AI 摘要
│
└── 界面设置
    ├── 主题: [深色 ▼]
    └── 语言: [简体中文 ▼]
```

### 6.2 配置文件示例

```yaml
# config/bellkeeper.yaml

server:
  host: 0.0.0.0
  port: 8080
  mode: release  # debug, release

database:
  driver: postgres
  host: localhost
  port: 5432
  name: bellkeeper
  user: bellkeeper
  password: ${DB_PASSWORD}  # 从环境变量读取

ragflow:
  base_url: http://ragflow:9380
  api_key: ${RAGFLOW_API_KEY}
  timeout: 30s

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

---

## 7. 功能扩展规划

### 7.1 Phase 1: 核心重构

#### 7.1.1 Dataset 映射配置 (Web UI)

在 Web UI 中提供完整的 Dataset 映射管理:

```
知识库映射页面
├── 映射列表
│   ├── [搜索] [新建映射]
│   │
│   ├── ┌────────────────────────────────────────────────────────┐
│   │   │ 安全知识库 (security)                     [默认] [编辑] │
│   │   │ Dataset ID: abc123...                                   │
│   │   │ 关联标签: [漏洞] [Web安全] [渗透测试]                   │
│   │   │ 解析器: naive                                           │
│   │   └────────────────────────────────────────────────────────┘
│   │
│   └── ┌────────────────────────────────────────────────────────┐
│       │ AI 知识库 (ai)                                  [编辑] │
│       │ Dataset ID: def456...                                   │
│       │ 关联标签: [机器学习] [深度学习] [NLP]                   │
│       │ 解析器: naive                                           │
│       └────────────────────────────────────────────────────────┘
│
└── 新建/编辑对话框
    ├── 映射名称: [________________] (用于 API 调用)
    ├── 显示名称: [________________]
    ├── Dataset ID: [________________] [从 RagFlow 选择]
    ├── 描述:      [________________]
    ├── 关联标签:  [选择标签...]
    ├── 默认解析器: [naive ▼]
    └── [✓] 设为默认知识库
```

#### 7.1.2 n8n 工作流自动路由

n8n 工作流不再需要硬编码 dataset_id，改为调用 Bellkeeper API:

```json
// n8n 调用示例
POST /api/ragflow/upload/with-routing
{
  "content": "文章内容...",
  "filename": "article.md",
  "title": "文章标题",
  "url": "https://example.com/article",
  "tags": ["安全", "漏洞"],
  "category": "security"
}

// Bellkeeper 自动路由逻辑
// 1. 根据 tags 匹配关联相同标签的 Dataset
// 2. 若无匹配，根据 category 查找映射名为 "security" 的 Dataset
// 3. 若仍无匹配，使用默认 Dataset
```

### 7.2 Phase 2: 控制面板

#### 7.2.1 服务状态监控页面

```
系统监控仪表板
├── 服务状态卡片
│   ├── ┌─────────────┐ ┌─────────────┐ ┌─────────────┐
│   │   │ Bellkeeper  │ │ RagFlow     │ │ n8n         │
│   │   │ ● 运行中    │ │ ● 运行中    │ │ ● 运行中    │
│   │   │ 延迟: 5ms   │ │ 延迟: 23ms  │ │ 延迟: 12ms  │
│   │   │ 内存: 128MB │ │ 内存: 2.1GB │ │ 内存: 512MB │
│   │   └─────────────┘ └─────────────┘ └─────────────┘
│   │
│   └── ┌─────────────┐ ┌─────────────┐ ┌─────────────┐
│       │ PostgreSQL  │ │ Firecrawl   │ │ Ollama      │
│       │ ● 运行中    │ │ ● 运行中    │ │ ○ 离线      │
│       │ 连接数: 5   │ │ 延迟: 45ms  │ │ 最后: 2h前  │
│       └─────────────┘ └─────────────┘ └─────────────┘
│
├── 最近活动
│   ├── 10:23 - 文档上传成功: "CVE-2024-1234 分析"
│   ├── 10:22 - RSS 抓取完成: 12 条新文章
│   ├── 10:20 - Webhook 触发: manual-ingest
│   └── 10:15 - 健康检查: 所有服务正常
│
└── 快速操作
    ├── [刷新状态] [触发健康检查] [查看日志]
    └── [手动入库] [触发 RSS 抓取]
```

#### 7.2.2 n8n 工作流状态展示

```
工作流监控
├── 工作流列表
│   ├── ┌────────────────────────────────────────────────────────┐
│   │   │ 01-scheduled-fetch (定时抓取)                          │
│   │   │ 状态: ● 已激活    最后运行: 10分钟前    成功率: 98%    │
│   │   │ 触发方式: Cron 每天 6:00, 18:00                        │
│   │   │ [查看执行历史] [手动触发] [暂停]                        │
│   │   └────────────────────────────────────────────────────────┘
│   │
│   └── ┌────────────────────────────────────────────────────────┐
│       │ 02-core-crawler (核心爬取)                              │
│       │ 状态: ● 已激活    最后运行: 2分钟前     成功率: 95%    │
│       │ 触发方式: Webhook                                       │
│       │ [查看执行历史] [手动触发] [暂停]                        │
│       └────────────────────────────────────────────────────────┘
│
└── 执行历史
    ├── 时间         工作流              状态    耗时    输入
    ├── 10:23:45    manual-ingest       成功    2.3s    1 URL
    ├── 10:20:12    core-crawler        成功    5.1s    3 URLs
    ├── 10:15:00    health-monitor      成功    0.8s    -
    └── 10:10:33    core-crawler        失败    1.2s    1 URL
```

#### 7.2.3 系统健康检查 API

```go
// GET /api/health/detailed
{
  "status": "healthy",
  "timestamp": "2026-02-12T10:30:00Z",
  "services": {
    "bellkeeper": {
      "status": "up",
      "latency_ms": 5,
      "version": "1.0.0"
    },
    "ragflow": {
      "status": "up",
      "latency_ms": 23,
      "documents_count": 1234
    },
    "n8n": {
      "status": "up",
      "latency_ms": 12,
      "active_workflows": 7
    },
    "postgresql": {
      "status": "up",
      "connections": 5,
      "max_connections": 100
    },
    "firecrawl": {
      "status": "up",
      "latency_ms": 45
    },
    "ollama": {
      "status": "down",
      "last_seen": "2026-02-12T08:30:00Z",
      "error": "connection refused"
    }
  },
  "metrics": {
    "documents_today": 23,
    "rss_feeds_active": 15,
    "webhooks_triggered_today": 8
  }
}
```

### 7.3 Phase 3: 高级功能

#### 7.3.1 工作流分工明确

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           工作流分工架构                                      │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│   n8n 职责                                                                   │
│   ┌─────────────────────────────────────────────────────────────────────┐   │
│   │ • 定时调度 (Cron 触发 RSS 抓取、健康检查)                           │   │
│   │ • 网页爬取 (调用 Firecrawl)                                         │   │
│   │ • CouchDB/Memos 写入 (存储原始内容)                                 │   │
│   │ • AI 调用编排 (Ollama/DeepSeek 调用)                                │   │
│   │ • 错误重试队列                                                       │   │
│   │ • 复杂条件分支                                                       │   │
│   └─────────────────────────────────────────────────────────────────────┘   │
│                                     │                                        │
│                                     ▼                                        │
│   Bellkeeper Backend 职责                                                    │
│   ┌─────────────────────────────────────────────────────────────────────┐   │
│   │ • RagFlow 上传 (文档入库、解析管理)                                 │   │
│   │ • URL 去重检查 (防止重复入库)                                       │   │
│   │ • 标签管理 (创建、匹配、关联)                                       │   │
│   │ • 知识库路由 (根据标签选择 Dataset)                                 │   │
│   │ • 元数据存储 (PostgreSQL)                                           │   │
│   │ • 配置管理 (API Key、Feature Flags)                                 │   │
│   │ • 服务状态监控                                                       │   │
│   └─────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

#### 7.3.2 新增 API 接口

```
POST /api/ragflow/upload/with-routing   # 智能路由上传
GET  /api/ragflow/check-url             # URL 去重检查
POST /api/tags/batch-create             # 批量创建标签
GET  /api/health/detailed               # 详细健康检查
GET  /api/workflows/status              # n8n 工作流状态
POST /api/workflows/trigger/{name}      # 触发工作流
GET  /api/settings                      # 获取配置列表
PUT  /api/settings/{key}                # 更新配置
GET  /api/metrics                       # 系统指标
```

---

## 8. 架构设计

### 8.1 目录结构

```
bellkeeper/
├── cmd/
│   └── bellkeeper/
│       └── main.go              # 入口
│
├── internal/
│   ├── config/                  # 配置管理
│   │   ├── config.go
│   │   └── defaults.go
│   │
│   ├── handler/                 # HTTP 处理器
│   │   ├── tag.go
│   │   ├── datasource.go
│   │   ├── rss.go
│   │   ├── webhook.go
│   │   ├── ragflow.go
│   │   ├── dataset_mapping.go
│   │   ├── health.go
│   │   └── settings.go
│   │
│   ├── service/                 # 业务逻辑
│   │   ├── tag_service.go
│   │   ├── ragflow_service.go
│   │   ├── health_service.go
│   │   └── n8n_service.go
│   │
│   ├── repository/              # 数据访问
│   │   ├── tag_repo.go
│   │   ├── datasource_repo.go
│   │   └── ...
│   │
│   ├── model/                   # 数据模型
│   │   ├── tag.go
│   │   ├── datasource.go
│   │   ├── rss_feed.go
│   │   ├── webhook.go
│   │   ├── dataset_mapping.go
│   │   └── setting.go
│   │
│   ├── middleware/              # 中间件
│   │   ├── auth.go
│   │   ├── logger.go
│   │   └── cors.go
│   │
│   └── pkg/                     # 内部工具包
│       ├── response/
│       ├── validator/
│       └── crypto/
│
├── api/
│   └── openapi.yaml             # API 文档
│
├── migrations/                  # 数据库迁移
│   ├── 001_init.up.sql
│   └── 001_init.down.sql
│
├── config/
│   ├── bellkeeper.yaml              # 默认配置
│   └── bellkeeper.example.yaml
│
├── web/                         # 前端 (SolidJS)
│   ├── src/
│   │   ├── components/
│   │   ├── pages/
│   │   ├── stores/
│   │   ├── api/
│   │   └── App.tsx
│   ├── package.json
│   └── vite.config.ts
│
├── docker/
│   ├── Dockerfile
│   └── docker-compose.yml
│
├── Makefile
├── go.mod
└── README.md
```

### 8.2 数据库 Schema (PostgreSQL)

```sql
-- 标签表
CREATE TABLE tags (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) UNIQUE NOT NULL,
    description TEXT,
    color VARCHAR(20) DEFAULT '#409EFF',
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- 数据源表
CREATE TABLE data_sources (
    id SERIAL PRIMARY KEY,
    name VARCHAR(200) NOT NULL,
    url VARCHAR(1000) NOT NULL,
    type VARCHAR(50) DEFAULT 'website',
    category VARCHAR(100),
    description TEXT,
    is_active BOOLEAN DEFAULT true,
    metadata JSONB,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- RSS 订阅表
CREATE TABLE rss_feeds (
    id SERIAL PRIMARY KEY,
    name VARCHAR(200) NOT NULL,
    url VARCHAR(1000) UNIQUE NOT NULL,
    category VARCHAR(100),
    description TEXT,
    is_active BOOLEAN DEFAULT true,
    last_fetched_at TIMESTAMP,
    fetch_interval_minutes INT DEFAULT 60,
    metadata JSONB,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- Webhook 配置表
CREATE TABLE webhook_configs (
    id SERIAL PRIMARY KEY,
    name VARCHAR(200) NOT NULL,
    url VARCHAR(1000) NOT NULL,
    method VARCHAR(10) DEFAULT 'POST',
    content_type VARCHAR(100) DEFAULT 'application/json',
    headers JSONB,
    body_template TEXT,
    timeout_seconds INT DEFAULT 30,
    description TEXT,
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- Webhook 调用历史表
CREATE TABLE webhook_history (
    id SERIAL PRIMARY KEY,
    webhook_id INT REFERENCES webhook_configs(id),
    request_url VARCHAR(1000),
    request_method VARCHAR(10),
    request_headers JSONB,
    request_body TEXT,
    status VARCHAR(20) DEFAULT 'pending',
    response_code INT,
    response_headers JSONB,
    response_body TEXT,
    duration_ms INT,
    error_message TEXT,
    created_at TIMESTAMP DEFAULT NOW()
);

-- 知识库映射表
CREATE TABLE dataset_mappings (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) UNIQUE NOT NULL,
    display_name VARCHAR(200),
    dataset_id VARCHAR(100) NOT NULL,
    description TEXT,
    is_default BOOLEAN DEFAULT false,
    is_active BOOLEAN DEFAULT true,
    parser_id VARCHAR(50) DEFAULT 'naive',
    metadata JSONB,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- 文章标签关联表
CREATE TABLE article_tags (
    id SERIAL PRIMARY KEY,
    document_id VARCHAR(100) NOT NULL,
    dataset_id VARCHAR(100) NOT NULL,
    tag_id INT REFERENCES tags(id),
    article_title VARCHAR(1000),
    article_url VARCHAR(2000),
    created_at TIMESTAMP DEFAULT NOW(),
    UNIQUE(document_id, tag_id)
);

-- 设置表
CREATE TABLE settings (
    id SERIAL PRIMARY KEY,
    key VARCHAR(255) UNIQUE NOT NULL,
    value TEXT,
    value_type VARCHAR(50) DEFAULT 'string',
    category VARCHAR(100),
    description TEXT,
    is_secret BOOLEAN DEFAULT false,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- 关联表: 数据源-标签
CREATE TABLE datasource_tags (
    datasource_id INT REFERENCES data_sources(id) ON DELETE CASCADE,
    tag_id INT REFERENCES tags(id) ON DELETE CASCADE,
    PRIMARY KEY (datasource_id, tag_id)
);

-- 关联表: RSS-标签
CREATE TABLE rss_tags (
    rss_id INT REFERENCES rss_feeds(id) ON DELETE CASCADE,
    tag_id INT REFERENCES tags(id) ON DELETE CASCADE,
    PRIMARY KEY (rss_id, tag_id)
);

-- 关联表: 知识库映射-标签
CREATE TABLE dataset_mapping_tags (
    mapping_id INT REFERENCES dataset_mappings(id) ON DELETE CASCADE,
    tag_id INT REFERENCES tags(id) ON DELETE CASCADE,
    PRIMARY KEY (mapping_id, tag_id)
);

-- 索引
CREATE INDEX idx_tags_name ON tags(name);
CREATE INDEX idx_data_sources_category ON data_sources(category);
CREATE INDEX idx_rss_feeds_category ON rss_feeds(category);
CREATE INDEX idx_webhook_history_webhook_id ON webhook_history(webhook_id);
CREATE INDEX idx_webhook_history_created_at ON webhook_history(created_at);
CREATE INDEX idx_article_tags_document_id ON article_tags(document_id);
CREATE INDEX idx_article_tags_dataset_id ON article_tags(dataset_id);
CREATE INDEX idx_settings_key ON settings(key);
CREATE INDEX idx_settings_category ON settings(category);
```

### 8.3 API 设计 (RESTful)

```yaml
# /api/openapi.yaml

openapi: 3.0.3
info:
  title: Bellkeeper API
  version: 1.0.0
  description: SilkSpool 知识管理系统 API

paths:
  # 标签
  /api/tags:
    get:
      summary: 获取标签列表
      parameters:
        - name: page
          in: query
          schema: { type: integer, default: 1 }
        - name: per_page
          in: query
          schema: { type: integer, default: 20 }
        - name: keyword
          in: query
          schema: { type: string }
    post:
      summary: 创建标签
      requestBody:
        content:
          application/json:
            schema:
              type: object
              properties:
                name: { type: string }
                description: { type: string }
                color: { type: string }

  /api/tags/{id}:
    get:
      summary: 获取标签详情
    put:
      summary: 更新标签
    delete:
      summary: 删除标签

  # RagFlow
  /api/ragflow/upload/with-routing:
    post:
      summary: 智能路由上传文档
      requestBody:
        content:
          application/json:
            schema:
              type: object
              required: [content, filename]
              properties:
                content: { type: string }
                filename: { type: string }
                title: { type: string }
                url: { type: string }
                tags: { type: array, items: { type: string } }
                category: { type: string }
                auto_create_tags: { type: boolean, default: true }

  /api/ragflow/check-url:
    get:
      summary: 检查 URL 是否已存在
      parameters:
        - name: url
          in: query
          required: true
          schema: { type: string }

  # 健康检查
  /api/health:
    get:
      summary: 基本健康检查

  /api/health/detailed:
    get:
      summary: 详细健康检查

  # 设置
  /api/settings:
    get:
      summary: 获取所有设置

  /api/settings/{key}:
    get:
      summary: 获取单个设置
    put:
      summary: 更新设置

  # 工作流
  /api/workflows/status:
    get:
      summary: 获取 n8n 工作流状态

  /api/workflows/trigger/{name}:
    post:
      summary: 触发指定工作流
```

---

## 9. 实施计划

### 9.1 阶段划分

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           实施阶段                                           │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│   Phase 1: 基础重构 (2-3 周)                                                │
│   ┌─────────────────────────────────────────────────────────────────────┐   │
│   │ □ 搭建 Go 项目骨架                                                  │   │
│   │ □ 实现核心数据模型 (GORM)                                          │   │
│   │ □ 迁移标签、数据源、RSS 等基础 CRUD                                │   │
│   │ □ 实现 Authelia Header 认证                                         │   │
│   │ □ 搭建 SolidJS 前端骨架                                             │   │
│   │ □ 实现基础页面 (标签、数据源、RSS)                                  │   │
│   │ □ PostgreSQL 数据库迁移                                              │   │
│   └─────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
│   Phase 2: 核心功能 (2 周)                                                  │
│   ┌─────────────────────────────────────────────────────────────────────┐   │
│   │ □ 迁移 RagFlow 集成                                                 │   │
│   │ □ 实现智能路由上传 API                                              │   │
│   │ □ 实现 URL 去重检查                                                 │   │
│   │ □ 迁移 Webhook 功能                                                 │   │
│   │ □ 迁移知识库映射功能                                                │   │
│   │ □ 实现设置管理 (Web UI)                                             │   │
│   │ □ 更新 n8n 工作流调用新 API                                         │   │
│   └─────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
│   Phase 3: 控制面板 (2 周)                                                  │
│   ┌─────────────────────────────────────────────────────────────────────┐   │
│   │ □ 实现服务状态监控页面                                              │   │
│   │ □ 实现 n8n 工作流状态展示                                           │   │
│   │ □ 实现系统健康检查 API                                              │   │
│   │ □ 实现快速操作面板                                                   │   │
│   │ □ 添加系统指标 (metrics)                                            │   │
│   └─────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
│   Phase 4: 优化与文档 (1 周)                                                │
│   ┌─────────────────────────────────────────────────────────────────────┐   │
│   │ □ 性能优化                                                           │   │
│   │ □ 单元测试覆盖                                                       │   │
│   │ □ API 文档完善 (Swagger)                                             │   │
│   │ □ 部署文档                                                           │   │
│   │ □ 数据迁移脚本 (从旧系统)                                           │   │
│   └─────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 9.2 里程碑

| 里程碑 | 目标 | 预计时间 |
|--------|------|----------|
| M1 | 后端 API 可用，前端首页完成 | Week 2 |
| M2 | 核心功能完成，n8n 可调用新 API | Week 4 |
| M3 | 控制面板完成，监控功能可用 | Week 6 |
| M4 | 全面测试，文档完善，正式上线 | Week 7 |

### 9.3 数据迁移策略

```bash
# 1. 导出旧数据
python export_data.py --output ./backup/

# 2. 启动新系统
docker-compose up -d

# 3. 导入数据
./bellkeeper migrate --from ./backup/

# 4. 验证数据
./bellkeeper verify

# 5. 切换 DNS/反向代理
# 更新 Caddy 配置指向新服务
```

### 9.4 回滚计划

1. 保留旧系统 Docker 镜像
2. 保留旧数据库备份
3. 使用蓝绿部署，新旧系统并行运行 1 周
4. 确认无问题后停用旧系统

---

## 附录

### A. 技术栈版本

| 技术 | 版本 | 备注 |
|------|------|------|
| Go | 1.22+ | 最新稳定版 |
| PostgreSQL | 16 | 最新 LTS |
| SolidJS | 1.8+ | 最新稳定版 |
| TailwindCSS | 3.4+ | 最新稳定版 |
| Vite | 5.x | 最新稳定版 |
| Docker | 24+ | 最新稳定版 |

### B. 参考资源

- [Gin Web Framework](https://gin-gonic.com/)
- [GORM](https://gorm.io/)
- [SolidJS](https://www.solidjs.com/)
- [TailwindCSS](https://tailwindcss.com/)
- [Authelia](https://www.authelia.com/)

### C. 命名备选方案历史

以下是曾考虑的其他命名方案（最终选择了 Bellkeeper）:

| 方案 | 英文 | 含义 | 游戏来源 |
|------|------|------|----------|
| **Bellkeeper** | 钟守者 | 记录与提醒 | 游戏中的钟塔元素 |
| Weaver | 织者 | 织网收集信息 | 丝之歌中的织者一族 |
| Archive | 档案馆 | 知识存储 | 教师档案馆 (Teacher's Archive) |
| Lorekeeper | 知识守护者 | 守护知识 | 游戏中的 lore 概念 |
| SilkThread | 丝线 | 连接知识 | 丝之歌核心元素 |
| Dreamcatcher | 捕梦者 | 捕捉信息 | 梦境概念 |
| Webspinner | 织网者 | 构建知识网络 | 蜘蛛/织者概念 |

---

## 10. 部署与测试规则

### 10.1 标准部署流程

所有代码变更必须遵循以下流程进行部署测试：

```bash
# 1. 本地代码提交到 GitHub
cd /home/ubuntu/Bellkeeper
git add .
git commit -m "描述变更内容"
git push origin main

# 2. 使用 SilkSpool 部署到测试环境
cd /home/ubuntu/SilkSpool

# 首次部署或模板变更时，运行 setup 合并所有 YAML 模板
./spool.sh bundle knowledge setup knowledge

# 后续仅更新单个服务时，使用 service 命令（不影响其他服务）
./spool.sh bundle knowledge service knowledge bellkeeper up

# 3. 验证服务状态
./spool.sh status knowledge bellkeeper
./spool.sh status knowledge bellkeeper-db

# 4. 验证健康检查
./spool.sh exec knowledge "curl -s http://localhost:8090/api/health"

# 5. 查看日志排查问题
./spool.sh logs knowledge bellkeeper 100
```

### 10.2 独立服务调试命令

```bash
# 服务操作语法
./spool.sh bundle <bundle> service <host> <service> <action>

# 可用 actions:
#   up      - 启动/重建服务（包含 build）
#   down    - 停止服务
#   build   - 仅重新构建镜像
#   logs    - 查看实时日志
#   restart - 重启服务

# 示例：
./spool.sh bundle knowledge service knowledge bellkeeper up
./spool.sh bundle knowledge service knowledge bellkeeper logs
./spool.sh bundle knowledge service knowledge bellkeeper restart
```

### 10.3 常见问题排查

#### 10.3.1 SSH 连接 GitHub 失败

**症状**: `Connection closed by fdfe:dcba:9876::4 port 22`

**原因**: OpenClash 代理将 SSH 流量路由到代理节点，代理无法正确处理 SSH 协议

**解决方案**: 在 iStoreOS 上添加 SSH 端口直连规则

```bash
# 编辑 OpenClash 配置
ssh root@192.168.7.1

# 在 /etc/openclash/config/kycloud.yaml 的 rules: 部分最前面添加：
# - DST-PORT,22,DIRECT

# 重启 OpenClash
/etc/init.d/openclash restart
```

#### 10.3.2 服务找不到 (no such service)

**原因**: 新增服务模板后未运行 setup 命令

**解决方案**:
```bash
./spool.sh bundle knowledge setup knowledge
```

#### 10.3.3 数据库连接失败

**检查步骤**:
```bash
# 1. 检查数据库容器状态
./spool.sh status knowledge bellkeeper-db

# 2. 检查环境变量
./spool.sh exec knowledge "cat /opt/silkspool/knowledge/.env | grep BELLKEEPER"

# 3. 手动测试连接
./spool.sh exec knowledge "docker exec sp-bellkeeper-db pg_isready -U bellkeeper"
```

### 10.4 SilkSpool 配置要点

#### 10.4.1 新增服务模板

1. 在 `bundles/knowledge/templates/` 创建 `XX-servicename.yaml`
2. 在 `bundles/knowledge/templates/00-base.yaml` 添加数据卷
3. 在 `config.ini` 的 `SERVICES_KNOWLEDGE` 添加服务注册
4. 在 `bundles/knowledge/defaults.sh` 添加默认环境变量
5. 在 `bundles/knowledge/remote.sh` 添加仓库克隆逻辑

#### 10.4.2 模板命名规则

- `00-base.yaml` - 基础网络和卷定义
- `10-infra.yaml` - 基础设施 (Redis, MySQL 等)
- `20-xxx.yaml` - 核心服务
- `40-apps.yaml` - 应用服务
- `60-xxx.yaml` - 扩展服务
- `70-bellkeeper.yaml` - Bellkeeper 服务

---

## 11. 进度记录

### 11.1 已完成阶段

| 阶段 | 状态 | 完成日期 | 说明 |
|------|------|----------|------|
| Phase 1.1 | ✅ 完成 | 2026-02-12 | Go 后端骨架搭建 |
| Phase 1.2 | ✅ 完成 | 2026-02-12 | 核心数据模型 (Tag, DataSource, RSS, Webhook, DatasetMapping) |
| Phase 1.3 | ✅ 完成 | 2026-02-12 | 基础 CRUD API 实现 |
| Phase 1.4 | ✅ 完成 | 2026-02-12 | Authelia Header 认证中间件 |
| Phase 1.5 | ✅ 完成 | 2026-02-12 | SolidJS 前端骨架 + TailwindCSS |
| Phase 1.6 | ✅ 完成 | 2026-02-12 | 前端基础页面 (标签、数据源、RSS、Webhook、知识库映射) |
| Docker | ✅ 完成 | 2026-02-12 | 多阶段构建 Dockerfile |
| SilkSpool | ✅ 完成 | 2026-02-12 | 70-bellkeeper.yaml 模板集成 |
| 首次部署 | ✅ 完成 | 2026-02-12 | knowledge 机器部署测试通过 |
| Phase 2.1 | ✅ 完成 | 2026-02-12 | RagFlow API 集成 (Upload, UploadWithRouting, CheckURL, ListDocuments, DeleteDocument) |
| Phase 2.2 | ✅ 完成 | 2026-02-12 | 前端 Documents 页面 (文档管理、智能路由上传、URL 去重检查) |
| Phase 2.3 | ✅ 完成 | 2026-02-12 | 设置管理页面 (Web UI 配置管理) |
| Phase 2.4 | ✅ 完成 | 2026-02-12 | n8n 工作流集成 (工作流列表、状态管理、触发、执行历史) |
| Phase 3.1 | ✅ 完成 | 2026-02-12 | Workflows 页面 (工作流管理、激活/停用、手动触发) |
| Phase 3.2 | ✅ 完成 | 2026-02-12 | Dashboard 快捷操作 (工作流一键触发) |
| UI 重构 | ✅ 完成 | 2026-02-26 | 全面重构前端 UI - 现代化暗色主题 |
| 站点配置 | ✅ 完成 | 2026-02-26 | Caddy + DNS + Homepage + Headscale 配置 |
| Healthcheck | ✅ 完成 | 2026-02-26 | 修复容器健康检查 (wget --spider → wget -qO) |
| Phase 5 | ✅ 完成 | 2026-02-26 | 系统测试与修复 (P0/P1 全部修复) |

### 11.2 重构完成总结

所有核心重构阶段 (Phase 1-3, 5) 已全部完成。系统已投入生产使用。

#### Phase 1: 基础重构 ✅

- [x] Go 项目骨架 (Gin + GORM + Cobra)
- [x] 核心数据模型 (8 个 model)
- [x] 基础 CRUD API (45+ 端点)
- [x] Authelia Header 认证中间件
- [x] SolidJS 前端骨架 + TailwindCSS
- [x] 前端基础页面 (9 个页面)
- [x] PostgreSQL 数据库迁移

#### Phase 2: 核心功能 ✅

- [x] RagFlow API 集成 (上传、智能路由、文档管理、URL 去重)
- [x] 前端 Documents 页面
- [x] 设置管理页面 (Web UI 动态配置)
- [x] n8n 工作流集成 (列表、状态、触发、执行历史)

#### Phase 3: 控制面板 ✅

- [x] Dashboard 服务状态监控
- [x] n8n 工作流状态展示
- [x] 系统健康检查 API (`/api/health/detailed`)
- [x] 快速操作面板
- [x] 基础系统指标 (标签/数据源/RSS/知识库计数)

#### Phase 5: 系统测试与修复 ✅

- [x] 全量 API 端点测试
- [x] P0 阻断性问题修复 (认证 Header、零值覆盖、假数据)
- [x] P1 重要问题修复 (设置种子化、Webhook Headers、上传验证、错误提示)
- [x] 部署验证通过

### 11.3 后续优化方向

以下为非阻断性的持续优化项目，可在日常迭代中逐步完成：

- [ ] 单元测试覆盖
- [ ] API 文档 (Swagger/OpenAPI)
- [ ] P2/P3 体验优化 (分页、排序、批量操作、字段级验证)
- [ ] 性能优化 (按需)

---

## 12. 系统测试报告

> **测试时间**: 2026-02-26
>
> **测试方式**: API 端点逐一测试 + 前端代码审计 + 后端 Handler 审计
>
> **部署环境**: https://bellkeeper.singll.net (production mode)
>
> **修复状态**: ✅ P0/P1 全部修复并验证通过

### 12.1 问题总览

| 优先级 | 数量 | 说明 |
|--------|------|------|
| **P0 - 阻断性** | 3 | 阻止生产环境正常使用 |
| **P1 - 重要** | 4 | 主要功能缺陷 |
| **P2 - 一般** | 4 | 应修复的问题 |
| **P3 - 优化** | 3 | 体验优化 |

### 12.2 P0 - 阻断性问题

#### P0-1: 认证 Header 名称不匹配 ✅ 已修复

- **现象**: 通过 Caddy + Authelia 访问时，所有 API 返回 401 Unauthorized
- **根因**: 后端 `auth.go:21` 检查 `X-Remote-User`，但 Caddy forward_auth 传递的是 `Remote-User`（Authelia 标准 Header）
- **修复**: 将中间件的 Header 名称从 `X-Remote-*` 改为 `Remote-*`

#### P0-2: Update 操作零值覆盖导致数据丢失 ✅ 已修复

- **现象**: 编辑数据源/RSS/知识库时，`is_active` 被意外设为 `false`
- **根因**: Go 结构体使用非指针布尔值 `IsActive bool`，JSON 反序列化时未提供的字段默认为 `false`
- **修复**: 请求结构体中布尔字段使用指针类型 `*bool`，更新时仅赋值非 nil 字段；Webhook handler 重构为独立请求结构体

#### P0-3: 工作流返回假数据 ✅ 已修复

- **现象**: `/api/workflows/status` 在 n8n 未配置时返回 3 条硬编码的假工作流
- **修复**: 未配置 n8n 时返回空列表，连接失败时返回错误信息而非静默降级

### 12.3 P1 - 重要问题

#### P1-1: 无默认设置项 ✅ 已修复

- **现象**: Settings 页面为空，`GET /api/settings` 返回 `{"data":[]}`
- **修复**: 在 `model/db.go` 添加 `SeedSettings()`，服务启动时自动种子化默认配置项（API/功能开关/UI）

#### P1-2: Webhook 编辑缺少 headers 字段 ✅ 已修复

- **现象**: 创建/编辑 Webhook 时无法设置自定义 Headers
- **修复**: 在 `Webhooks.tsx` 表单中添加 Headers JSON 编辑器

#### P1-3: Documents 页面上传验证不足 ✅ 已修复

- **现象**: 未选择知识库时可以提交上传
- **修复**: 添加文件名、内容、知识库的前置验证

#### P1-4: n8n 工作流执行记录不可用 ✅ 已修复

- **现象**: `/api/workflows/executions` 返回 `"n8n API key not configured"`
- **修复**: 配合 P1-1 种子化设置解决；Workflows.tsx 添加错误状态展示和重试按钮

### 12.4 P2 - 一般问题

#### P2-1: Webhook 触发结果显示异常

- **涉及**: `Webhooks.tsx:104-118`
- **问题**: 触发后 response 结构与前端 WebhookHistory 类型不完全匹配
- **修复**: 对齐前后端响应类型

#### P2-2: Settings 布尔值处理不一致

- **涉及**: `Settings.tsx:160-167`
- **问题**: 下拉选择的 "true"/"false" 作为字符串保存，后端期望可能不一致
- **修复**: 前端提交前根据 value_type 转换类型

#### P2-3: Webhook 历史无分页

- **涉及**: `Webhooks.tsx:145-147`
- **问题**: 历史记录最多显示 20 条，无分页控件
- **修复**: 添加分页或虚拟滚动

#### P2-4: RagFlow 服务中 JSON 错误静默忽略

- **涉及**: `service/ragflow.go:149,166`、`service/workflow.go:321`
- **问题**: `json.Marshal/Unmarshal` 错误被 `_` 忽略
- **修复**: 添加错误处理和日志

### 12.5 P3 - 体验优化

#### P3-1: 缺少表单字段级验证

- **涉及**: 所有表单页面
- **问题**: 验证失败仅显示 Toast 通知，无字段级错误提示
- **修复**: 添加字段下方的错误提示

#### P3-2: 列表无排序功能

- **涉及**: 所有列表页面
- **问题**: 只能搜索/筛选，不能按列排序
- **修复**: 添加列头点击排序

#### P3-3: 无批量操作

- **涉及**: 所有列表页面
- **问题**: 不支持批量删除、启用/禁用
- **修复**: 添加复选框选择和批量操作按钮

### 12.6 修复记录

```
修复批次                    状态     涉及文件
────────────────────────────────────────────────────────────────
Batch 1: 认证与数据安全      ✅ 完成
├── 修复 Auth Header         middleware/auth.go
├── 修复零值覆盖             handler/datasource.go, rss.go, dataset.go, webhook.go
└── 移除 Placeholder 数据    service/workflow.go

Batch 2: 设置与配置           ✅ 完成
├── 种子化默认设置           model/db.go (SeedSettings)
├── 服务启动时自动迁移       cmd/bellkeeper/main.go
└── n8n 集成修复             (配合 Batch 1)

Batch 3: 前端功能修复         ✅ 完成
├── Documents 上传验证       web/src/pages/Documents.tsx
├── Webhook Headers 编辑     web/src/pages/Webhooks.tsx
└── Workflows 错误提示       web/src/pages/Workflows.tsx
```

### 12.7 修复后 API 测试结果

| 端点 | 方法 | 状态 | 说明 |
|------|------|------|------|
| `/api/health` | GET | ✅ 正常 | 无需认证 |
| `/api/health/detailed` | GET | ✅ 正常 | 返回 n8n/ragflow 状态和指标 |
| `/api/tags` | CRUD | ✅ 正常 | 创建/读取/更新/删除均正常 |
| `/api/datasources` | CRUD | ✅ 正常 | 零值覆盖已修复 |
| `/api/rss` | CRUD | ✅ 正常 | 零值覆盖已修复 |
| `/api/webhooks` | CRUD | ✅ 正常 | 零值覆盖已修复；Headers 编辑已添加 |
| `/api/webhooks/:id/trigger` | POST | ✅ 正常 | 触发并记录历史 |
| `/api/webhooks/:id/history` | GET | ✅ 正常 | 返回执行历史 |
| `/api/datasets` | CRUD | ✅ 正常 | 零值覆盖已修复 |
| `/api/settings` | GET/PUT | ✅ 正常 | 默认设置已种子化 (10 条) |
| `/api/workflows/status` | GET | ✅ 正常 | 未配置时返回空列表 |
| `/api/workflows/executions` | GET | ✅ 正常 | 需配置 n8n API Key |
| `/api/ragflow/documents` | GET | ✅ 正常 | 连接 RagFlow 正常 |
| `/api/ragflow/check-url` | GET | ✅ 正常 | URL 检查正常 |
| 通过 Caddy 认证访问 | ALL | ✅ 正常 | Remote-User Header 已修复 |

---

**文档状态**: 重构完成

**更新日期**: 2026-02-26
