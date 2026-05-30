# Bellkeeper 架构文档

> 版本: A4（文档重组后）
> 更新日期: 2026-04-08
> 状态标注: ✅ 已实施 | 🚧 实施中 | 📋 设计完成

---

## 定位与职责

Bellkeeper 是知识管理中台 + LLM 代理网关，充当 n8n 工作流的"能力增强器"。

**核心价值**：做 n8n 做不好的事——有状态的限速、去重、路由、日志。

### Bellkeeper 负责

| 能力 | 状态 | 原因 |
|------|------|------|
| LLM Proxy（多渠道限速代理） | ✅ | 需要内存中令牌桶状态 + 持久化日志 |
| 文件入库与治理 | 🚧 | 需要文件系统操作 + 提取器编排 + 元数据管理 |
| URL 去重（精确/归一化/模糊） | ✅ | 需要数据库存储已入库 URL |
| 标签体系管理 | ✅ | 分类路由的核心数据 |
| 分类与路由 | ✅ | 需要 LLM 调用 + 数据库映射 |
| RSS 订阅源管理 | ✅ | n8n K02 工作流的数据来源 |
| Matrix 控制平面 | 🚧 | Gateway + CommandRouter + Admin API + 通知网关已实现，权限引擎待完善 |
| 通知网关 | ✅ | NATS 队列 + 频道路由 + 重试逻辑已实现 |
| n8n 工作流触发 + 状态查看 | ✅ | 统一认证入口（API Key 而非 n8n Key） |
| 运行时配置（Settings KV） | ✅ | 无需重启即可修改配置 |
| RAGFlow 集成（兼容层） | ✅ | 保留兼容，不再作为主链路 |

### n8n 负责

- 事件驱动的编排流程（采集→清洗→入库→通知）
- 定时任务调度（Cron）
- 多步骤串联逻辑
- 与外部服务的简单 HTTP 交互

---

## 技术栈

| 层 | 技术 |
|----|------|
| 后端语言 | Go 1.25 |
| HTTP 框架 | Gin 1.9 |
| ORM | GORM 1.25 + PostgreSQL 16 |
| 消息队列 | NATS |
| 缓存 | Redis |
| 配置 | Viper（YAML + 环境变量） |
| 前端 | SolidJS 1.8 + TailwindCSS 3.4 + Vite 5.x |
| Matrix SDK | mautrix-go |
| 认证 | Authelia Forward Auth + API Key |
| 监控 | Prometheus |
| 日志 | Zap 结构化日志 |
| 部署 | Docker Compose（Alpine 多阶段构建） |

---

## 代码结构

```
Bellkeeper/
├── cmd/bellkeeper/main.go      # 入口：初始化 DB、服务、路由
├── internal/
│   ├── config/config.go        # Viper 配置加载，支持 ${ENV_VAR} 展开
│   ├── handler/                # HTTP 处理器（请求解析 → 调用 service → 返回响应）
│   │   ├── handler.go          # Handlers 聚合结构体
│   │   ├── tag.go
│   │   ├── rss.go
│   │   ├── dataset.go
│   │   ├── ragflow.go
│   │   ├── setting.go
│   │   ├── workflow.go
│   │   ├── health.go
│   │   ├── system.go
│   │   ├── llm_proxy.go
│   │   ├── file_ingestion.go   # 文件入库 API
│   │   ├── matrix_notify.go    # Matrix 通知 API
│   │   ├── matrix_admin.go     # Matrix 管理 API
│   │   ├── search.go           # 全局搜索
│   │   ├── activity_log.go     # 活动日志查询
│   │   └── todotxt_export.go   # todo.txt 导出
│   ├── middleware/
│   │   ├── auth.go             # Authelia SSO + API Key 双模式认证
│   │   ├── cors.go
│   │   └── logger.go
│   ├── model/                  # GORM 数据模型
│   │   ├── db.go               # InitDB + AutoMigrate + SeedSettings
│   │   ├── tag.go
│   │   ├── rss.go
│   │   ├── dataset.go          # DatasetMapping + ArticleTag
│   │   ├── setting.go
│   │   ├── llm_*.go            # LLM Proxy 模型 (channels, model_groups, logs)
│   │   └── matrix.go           # Matrix 实体模型
│   ├── pkg/
│   │   ├── defaults/defaults.go  # 全局常量（超时、颜色等）
│   │   ├── response/response.go  # 统一响应格式
│   │   └── urlutil/normalize.go  # URL 归一化 + 模糊匹配
│   ├── repository/             # 数据访问层（GORM 查询封装）
│   │   ├── repository.go       # Repositories 聚合结构体
│   │   ├── tag.go, rss.go, dataset.go, setting.go
│   │   ├── llm_*.go            # LLM Proxy 仓储
│   │   └── matrix_*.go         # Matrix 实体仓储
│   ├── router/router.go        # 路由注册
│   ├── matrix/                 # Matrix 集成模块
│   │   ├── command/            # 命令系统
│   │   ├── gateway/            # Matrix 网关
│   │   ├── infra/              # 基础设施 (Redis/NATS)
│   │   ├── notify/             # 通知网关
│   │   ├── policy/             # 权限引擎
│   │   ├── queue/              # 消息队列
│   │   ├── registry/           # 注册中心
│   │   └── worker/             # 后台工作者
│   ├── middleware/             # HTTP 中间件
│   │   ├── auth.go             # Authelia + API Key 认证
│   │   ├── cors.go, logger.go, ratelimit.go
│   ├── metrics/                # Prometheus 指标
│   └── pkg/                    # 内部工具包
│       ├── response/           # 统一 API 响应
│       ├── errors/             # 错误类型定义
│       ├── defaults/           # 常量和默认值
│       ├── urlutil/            # URL 规范化
│       └── sanitizer/          # HTML 清理
│   └── service/                # 业务逻辑层
│       ├── service.go          # Services 聚合结构体
│       ├── tag.go
│       ├── rss.go
│       ├── dataset.go          # URL 去重 + 标签路由
│       ├── ragflow.go          # RAGFlow HTTP 调用封装
│       ├── ragflow_parse_queue.go # RAGFlow 解析队列
│       ├── setting.go
│       ├── workflow.go         # n8n API 封装
│       ├── health.go
│       ├── llm_proxy.go        # 多渠道代理
│       ├── llm_model_group.go  # 虚拟模型组
│       ├── llm_channel_health.go # 熔断器
│       ├── classify.go         # LLM 分类
│       ├── file_ingestion.go   # 文件入库服务
│       ├── extractor.go        # 内容提取器
│       ├── notification.go     # 通知服务
│       ├── command.go          # Matrix 命令路由
│       ├── admin.go            # Matrix 管理服务
│       └── activity_log.go     # 活动日志
├── migrations/                 # SQL 迁移文件（参考用，实际用 AutoMigrate）
├── web/                        # SolidJS 前端
│   ├── src/
│   │   ├── api/index.ts        # 所有 API 调用封装
│   │   ├── components/         # Layout, Modal, Toast
│   │   ├── pages/              # 各功能页面
│   │   └── types/index.ts      # TypeScript 类型定义
│   └── package.json
└── doc/                        # 项目文档（本目录）
```

---

## 数据模型

### 核心表

| 表 | 模型 | 用途 |
|----|------|------|
| `tags` | `Tag` | 标签分类体系，贯穿路由逻辑 |
| `rss_feeds` | `RSSFeed` | RSS 订阅源，n8n K02 工作流读取 |
| `dataset_mappings` | `DatasetMapping` | 标签→RAGFlow 数据集映射 |
| `article_tags` | `ArticleTag` | 文章-标签关联 + URL 去重记录 |
| `settings` | `Setting` | 运行时 KV 配置 |
| `llm_proxy_logs` | `LLMProxyLog` | LLM 代理请求日志 |
| `llm_channels` | `LLMChannel` | LLM 渠道配置 |
| `llm_model_groups` | `LLMModelGroup` | 虚拟模型组配置 |
| `activity_logs` | `ActivityLog` | 跨模块操作审计日志 |
| `matrix_rooms` | `MatrixRoom` | Matrix 房间管理 |
| `matrix_channels` | `MatrixChannel` | 通知频道路由 |
| `matrix_commands` | `MatrixCommand` | 命令注册与权限 |
| `matrix_events` | `MatrixEvent` | 事件记录 |
| `matrix_notifications` | `MatrixNotification` | 通知队列 |
| `matrix_command_logs` | `MatrixCommandLog` | 命令执行日志 |
| `matrix_sync_state` | `MatrixSyncState` | 同步状态追踪 |

### 关联关系

```
Tag ←→ RSSFeed          (many2many: rss_tags)
Tag ←→ DatasetMapping   (many2many: dataset_mapping_tags)
ArticleTag → Tag        (foreign key: tag_id)
ArticleTag → DatasetMapping (via dataset_id 字段，非外键)
```

> 注意：`data_sources` 和 `webhook_configs`/`webhook_history` 表仍存在于数据库（保留历史数据），但后端代码已不再操作这些表。

---

## 模块详解

### 已实施模块

#### LLM Proxy（`/api/llm/v1/*`） ✅

OpenAI 兼容代理，核心机制：

1. **渠道选择**：按模型名匹配（精确→大小写不敏感→子串），按 `priority` 排序
2. **令牌桶限速**：每个渠道独立的 `TokenBucket`，支持 RPM（每分钟）和 RPD（每日）双重限制
3. **指数退避重试**：上游 429 时自动退避，公式 `2^(attempt+1) * (1 + 0~50% jitter)`，上限 60s
4. **请求日志**：异步写入 `llm_proxy_logs`，记录渠道、模型、耗时、Token 用量

配置示例（`bellkeeper.yaml`）：
```yaml
llm_proxy:
  enabled: true
  default_timeout: 60
  max_retries: 3
  channels:
    - name: deepseek
      base_url: ${LLM_DEEPSEEK_BASE_URL}
      api_key: ${LLM_DEEPSEEK_API_KEY}
      rpm: 60
      rpd: 1000
      priority: 1
      models: ["deepseek-chat", "deepseek-reasoner"]
      is_enabled: true
```

#### RSS 订阅管理（`/api/rss/*`） ✅

提供 RSS 源的 CRUD 管理，供 n8n K02 工作流读取。

#### 分类与路由 ✅

- **ClassifyService**: 基于 LLM 的文章分类
- **DatasetService**: 标签路由与 URL 去重
- **TagRepository**: 标签体系管理

#### RAGFlow 集成（`/api/ragflow/*`） ✅ (兼容层)

**状态**: 保留作为兼容层，不再作为主链路

RAGFlow HTTP API 的代理层，增加了：

- **智能路由**（`/upload/with-routing`）：标签→数据集映射，自动选择目标数据集
- **URL 去重**（`/check-url`）：精确匹配 → 归一化匹配，自动清理 RAGFlow 中已删除的陈旧记录
- **节流解析**（`/documents/parse/throttled`）：分批提交，避免 Embedding 限速

---

### 实施中模块

#### 文件入库与治理（`/api/files/*`） 🚧

**目标**: 从 RAGFlow 中心转向文件优先架构

**核心链路**:
```
RSS/URL → Bellkeeper /api/files/ingest/url → 提取器编排 → 文件落地 → 索引排队
```

**关键组件**:
- **ExtractorService**: Trafilatura 主力 + Firecrawl 兜底
- **FileIngestionService**: 去重、分类、frontmatter、文件落地
- **ArticleTagRepository**: 元数据管理

**详细文档**: [documents/IMPLEMENTATION.md](documents/IMPLEMENTATION.md)

---

### 设计完成模块

#### Matrix 控制平面（`/api/matrix/*`） 🚧

**状态**: Phase 1 MVP 已实施（Gateway + CommandRouter + Admin API + 通知网关），权限引擎待完善

**三大平面**:
1. **命令平面**: Matrix 命令接收、权限校验、路由
2. **通知平面**: 多系统通知入口、频道路由、模板渲染
3. **治理平面**: 房间配置、命令注册、Admin API

**详细文档**: [matrix/README.md](matrix/README.md)

---

### 已实施模块详细说明

#### 标签路由（`DatasetService`）

URL 去重的三级匹配策略：
1. **精确匹配**：`article_tags.article_url = rawURL`
2. **归一化匹配**：去除 UTM 参数、统一大小写后比较
3. **模糊匹配**：路径包含关系（`minPathLen=10`）

每次匹配后都会通过 `DocumentVerifier` 验证文档是否仍存在于 RAGFlow，自动清理陈旧记录。

### 认证机制

两种认证方式，任一通过即可：

1. **API Key**：`X-API-Key: <server.api_key>` — 用于 n8n 等内部服务调用
2. **Authelia SSO**：`Remote-User` 头由 Authelia 反向代理注入 — 用于前端用户访问

`debug` 模式下无需认证（本地开发用）。

---

## 配置说明

配置文件路径（按优先级）：
1. `--config` 命令行参数指定的路径
2. `./config/bellkeeper.yaml`
3. `/etc/bellkeeper/bellkeeper.yaml`

环境变量覆盖：前缀 `BELLKEEPER_`，`.` 替换为 `_`。
例：`BELLKEEPER_SERVER_PORT=9090` 覆盖 `server.port`。

LLM Proxy 渠道配置中支持 `${ENV_VAR}` 语法展开环境变量（在 `config.Load()` 中处理）。

---

## 部署

通过 SilkSpool 的 `spool.sh` 管理：

```bash
# 重启（热加载 .env）
./spool.sh restart bellkeeper

# 查看日志
./spool.sh logs bellkeeper

# 不要直接 docker compose down（会停全部服务）
```

容器名：`sp-bellkeeper`
内部端口：`8080`
外部访问：通过 Authelia 反向代理，路径 `/bellkeeper/`
