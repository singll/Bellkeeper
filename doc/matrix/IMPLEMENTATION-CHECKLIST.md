# Matrix Bot Platform 实施准备清单

> 基于技术选型：mautrix-go + Redis + Queue + 完整 MVP（通知 + 命令）
> 
> 生成时间：2026-04-08 | 最后更新：2026-04-08

---

## 一、技术选型确认

### 1.1 Matrix SDK
- ✅ **选择**: mautrix-go (https://github.com/mautrix/go)
- **版本**: v0.18+ (支持 Matrix v1.11)
- **依赖**: `go get maunium.net/go/mautrix`
- **核心能力**:
  - 完整的 Client-Server API 封装
  - 自动 sync loop 管理
  - 事件去重与 token 持久化
  - E2EE 支持（可选）
  - 丰富的事件类型支持

### 1.2 Redis
- ✅ **选择**: Redis 7.x
- **用途**:
  - Sync token 存储（高频读写）
  - 会话状态缓存（粘性路由、多轮对话）
  - 分布式锁（防止重复处理）
  - 限流计数器（命令频控、通知降噪）
- **部署**: Docker Compose 新增 `sp-redis` 服务
- **持久化**: AOF + RDB 混合模式
- **Go 客户端**: `github.com/redis/go-redis/v9`

### 1.3 消息队列
- ✅ **选择**: NATS (https://nats.io/)
- **理由**:
  - 轻量级，单二进制部署
  - 原生支持 JetStream（持久化队列）
  - Go 客户端成熟
  - 比 RabbitMQ 更适合 Go 生态
- **用途**:
  - 异步通知队列（`notifications.pending`）
  - 命令执行队列（`commands.pending`）
  - 重试队列（`notifications.retry`, `commands.retry`）
  - 死信队列（`notifications.dlq`, `commands.dlq`）
- **部署**: Docker Compose 新增 `sp-nats` 服务
- **Go 客户端**: `github.com/nats-io/nats.go`

### 1.4 MVP 范围
- ✅ **Phase 1 完整 MVP**:
  - Matrix Gateway + Sync Loop
  - Command Router + 简单命令处理器（`!help`, `!status`, `!列表`, `!新增`）
  - Notification Gateway + 频道路由
  - 基础权限模型
  - 审计日志

---

## 二、基础设施准备

### 2.1 Docker Compose 新增服务

#### Redis 服务
```yaml
sp-redis:
  image: redis:7-alpine
  container_name: sp-redis
  restart: unless-stopped
  command: redis-server --appendonly yes --appendfsync everysec
  volumes:
    - ./data/redis:/data
  networks:
    - silkspool
  healthcheck:
    test: ["CMD", "redis-cli", "ping"]
    interval: 10s
    timeout: 3s
    retries: 3
```

#### NATS 服务
```yaml
sp-nats:
  image: nats:2.10-alpine
  container_name: sp-nats
  restart: unless-stopped
  command: -js -sd /data
  volumes:
    - ./data/nats:/data
  networks:
    - silkspool
  healthcheck:
    test: ["CMD", "nats-server", "--healthz"]
    interval: 10s
    timeout: 3s
    retries: 3
```

### 2.2 环境变量新增

在 `keeper/.env` 中添加：

```bash
# Matrix Bot Configuration
MATRIX_HOMESERVER_URL=https://matrix.singll.net
MATRIX_BOT_USER_ID=@bellkeeper:matrix.singll.net
MATRIX_BOT_ACCESS_TOKEN=<从 Conduit 获取>
MATRIX_BOT_DEVICE_ID=BELLKEEPER_KEEPER

# Redis Configuration
REDIS_HOST=sp-redis
REDIS_PORT=6379
REDIS_DB=0
REDIS_PASSWORD=

# NATS Configuration
NATS_URL=nats://sp-nats:4222
NATS_STREAM_NOTIFICATIONS=notifications
NATS_STREAM_COMMANDS=commands

# Matrix Platform Configuration
MATRIX_SYNC_TIMEOUT=30000
MATRIX_COMMAND_PREFIX=!,！
MATRIX_MAX_RETRY=3
MATRIX_NOTIFICATION_BATCH_SIZE=10
```

### 2.3 Bellkeeper 配置新增

在 `bellkeeper.yaml` 中添加：

```yaml
matrix:
  homeserver_url: ${MATRIX_HOMESERVER_URL}
  bot_user_id: ${MATRIX_BOT_USER_ID}
  bot_access_token: ${MATRIX_BOT_ACCESS_TOKEN}
  device_id: ${MATRIX_BOT_DEVICE_ID}
  sync_timeout: ${MATRIX_SYNC_TIMEOUT}
  command_prefix: ${MATRIX_COMMAND_PREFIX}
  max_retry: ${MATRIX_MAX_RETRY}

redis:
  host: ${REDIS_HOST}
  port: ${REDIS_PORT}
  db: ${REDIS_DB}
  password: ${REDIS_PASSWORD}

nats:
  url: ${NATS_URL}
  streams:
    notifications: ${NATS_STREAM_NOTIFICATIONS}
    commands: ${NATS_STREAM_COMMANDS}
```

---

## 三、数据模型准备

### 3.1 核心表结构

#### matrix_rooms (房间注册表)
```sql
CREATE TABLE matrix_rooms (
    id SERIAL PRIMARY KEY,
    room_id VARCHAR(255) NOT NULL UNIQUE,
    room_name VARCHAR(255),
    room_type VARCHAR(50) NOT NULL, -- 'command', 'notification', 'admin'
    is_active BOOLEAN DEFAULT true,
    config JSONB, -- 房间级配置
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_matrix_rooms_type ON matrix_rooms(room_type);
CREATE INDEX idx_matrix_rooms_active ON matrix_rooms(is_active);
```

#### matrix_channels (逻辑频道)
```sql
CREATE TABLE matrix_channels (
    id SERIAL PRIMARY KEY,
    channel_name VARCHAR(100) NOT NULL UNIQUE, -- 'alerts', 'daily', 'todo', 'qa'
    room_id VARCHAR(255) NOT NULL,
    is_active BOOLEAN DEFAULT true,
    priority INT DEFAULT 0, -- 优先级，用于多房间路由
    config JSONB, -- 频道级配置（限流、模板等）
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (room_id) REFERENCES matrix_rooms(room_id) ON DELETE CASCADE
);

CREATE INDEX idx_matrix_channels_name ON matrix_channels(channel_name);
CREATE INDEX idx_matrix_channels_active ON matrix_channels(is_active);
```

#### matrix_commands (命令注册表)
```sql
CREATE TABLE matrix_commands (
    id SERIAL PRIMARY KEY,
    command_name VARCHAR(100) NOT NULL, -- '列表', 'list', '新增', 'add'
    handler_type VARCHAR(100) NOT NULL, -- 'memos_todo', 'ragflow_qa', 'n8n_workflow'
    handler_config JSONB, -- 处理器配置（API endpoint、参数映射等）
    permission_level VARCHAR(50) DEFAULT 'user', -- 'admin', 'user', 'guest'
    room_scope VARCHAR(50) DEFAULT 'any', -- 'any', 'specific', 'admin_only'
    is_active BOOLEAN DEFAULT true,
    description TEXT,
    usage_example TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX idx_matrix_commands_name ON matrix_commands(command_name);
CREATE INDEX idx_matrix_commands_handler ON matrix_commands(handler_type);
CREATE INDEX idx_matrix_commands_active ON matrix_commands(is_active);
```

#### matrix_events (事件审计表)
```sql
CREATE TABLE matrix_events (
    id BIGSERIAL PRIMARY KEY,
    event_id VARCHAR(255) NOT NULL UNIQUE,
    room_id VARCHAR(255) NOT NULL,
    sender VARCHAR(255) NOT NULL,
    event_type VARCHAR(100) NOT NULL, -- 'm.room.message', 'm.room.member'
    content JSONB,
    processed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    processing_status VARCHAR(50) DEFAULT 'pending', -- 'pending', 'processed', 'failed', 'ignored'
    error_message TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_matrix_events_event_id ON matrix_events(event_id);
CREATE INDEX idx_matrix_events_room ON matrix_events(room_id);
CREATE INDEX idx_matrix_events_status ON matrix_events(processing_status);
CREATE INDEX idx_matrix_events_created ON matrix_events(created_at DESC);
```

#### matrix_notifications (通知记录表)
```sql
CREATE TABLE matrix_notifications (
    id BIGSERIAL PRIMARY KEY,
    notification_id VARCHAR(100) NOT NULL UNIQUE, -- 业务系统生成的幂等键
    channel_name VARCHAR(100) NOT NULL,
    room_id VARCHAR(255),
    message_type VARCHAR(50) DEFAULT 'text', -- 'text', 'html', 'markdown'
    message_content TEXT NOT NULL,
    metadata JSONB, -- 业务元数据
    status VARCHAR(50) DEFAULT 'pending', -- 'pending', 'sent', 'failed', 'retrying'
    retry_count INT DEFAULT 0,
    last_error TEXT,
    sent_event_id VARCHAR(255), -- Matrix 返回的 event_id
    sent_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_matrix_notifications_id ON matrix_notifications(notification_id);
CREATE INDEX idx_matrix_notifications_channel ON matrix_notifications(channel_name);
CREATE INDEX idx_matrix_notifications_status ON matrix_notifications(status);
CREATE INDEX idx_matrix_notifications_created ON matrix_notifications(created_at DESC);
```

#### matrix_command_logs (命令执行日志)
```sql
CREATE TABLE matrix_command_logs (
    id BIGSERIAL PRIMARY KEY,
    event_id VARCHAR(255) NOT NULL,
    room_id VARCHAR(255) NOT NULL,
    sender VARCHAR(255) NOT NULL,
    command_name VARCHAR(100) NOT NULL,
    command_args TEXT,
    handler_type VARCHAR(100),
    execution_status VARCHAR(50) DEFAULT 'pending', -- 'pending', 'success', 'failed'
    execution_time_ms INT,
    error_message TEXT,
    response_event_id VARCHAR(255), -- bot 回复的 event_id
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMP
);

CREATE INDEX idx_matrix_command_logs_event ON matrix_command_logs(event_id);
CREATE INDEX idx_matrix_command_logs_room ON matrix_command_logs(room_id);
CREATE INDEX idx_matrix_command_logs_command ON matrix_command_logs(command_name);
CREATE INDEX idx_matrix_command_logs_status ON matrix_command_logs(execution_status);
CREATE INDEX idx_matrix_command_logs_created ON matrix_command_logs(created_at DESC);
```

#### matrix_sync_state (Sync 状态表)
```sql
CREATE TABLE matrix_sync_state (
    id SERIAL PRIMARY KEY,
    bot_user_id VARCHAR(255) NOT NULL UNIQUE,
    next_batch VARCHAR(255), -- Matrix sync token
    filter_id VARCHAR(100),
    last_sync_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

### 3.2 迁移方式（已调整）
- ❌ 不使用独立 SQL 迁移脚本 `008_matrix_platform.sql`
- ✅ 使用 **GORM AutoMigrate**（与现有 Bellkeeper 模式一致）
- ✅ 在 `internal/model/db.go` 中注册 Matrix 模型
- ✅ 在 Bellkeeper 启动时自动建表 + Seed 初始数据

---

## 四、代码结构准备

### 4.1 新增模块目录

```text
internal/
  matrix/
    gateway/          # Matrix Gateway (sync loop, event ingress)
      client.go       # mautrix-go 客户端封装
      sync.go         # Sync loop 实现
      event.go        # 事件标准化
    
    command/          # Command Plane
      router.go       # 命令路由
      parser.go       # 命令解析
      handler.go      # Handler 接口定义
      handlers/       # 具体 handler 实现
        help.go
        status.go
        memos.go      # Memos todo 命令
        ragflow.go    # RAGFlow QA 命令
        n8n.go        # n8n workflow 触发
    
    notify/           # Notification Plane
      gateway.go      # 通知网关入口
      router.go       # 频道路由
      sender.go       # Matrix 发送封装
      template.go     # 模板渲染
      retry.go        # 重试逻辑
    
    policy/           # Permission & Policy
      permission.go   # 权限校验
      ratelimit.go    # 限流
      scope.go        # 房间作用域
    
    registry/         # Registry & Admin
      room.go         # 房间管理
      channel.go      # 频道管理
      command.go      # 命令管理
    
    worker/           # Background Workers
      notification_worker.go  # 通知队列消费者
      command_worker.go       # 命令队列消费者
      retry_worker.go         # 重试队列消费者
    
    queue/            # Queue 封装
      nats.go         # NATS 客户端封装
      producer.go     # 消息生产者
      consumer.go     # 消息消费者
```

### 4.2 新增 Model

```text
internal/model/
  matrix_room.go
  matrix_channel.go
  matrix_command.go
  matrix_event.go
  matrix_notification.go
  matrix_command_log.go
  matrix_sync_state.go
```

### 4.3 新增 Repository

```text
internal/repository/
  matrix_room_repo.go
  matrix_channel_repo.go
  matrix_command_repo.go
  matrix_event_repo.go
  matrix_notification_repo.go
  matrix_command_log_repo.go
  matrix_sync_state_repo.go
```

### 4.4 新增 Service

```text
internal/service/
  matrix_gateway_service.go      # Gateway 服务
  matrix_command_service.go      # 命令服务
  matrix_notification_service.go # 通知服务
  matrix_admin_service.go        # 管理服务
```

### 4.5 新增 Handler

```text
internal/handler/
  matrix_notify_handler.go   # POST /api/matrix/notify
  matrix_admin_handler.go    # Admin API
  matrix_room_handler.go     # 房间管理 API
  matrix_command_handler.go  # 命令管理 API
  matrix_audit_handler.go    # 审计查询 API
```

---

## 五、实施步骤建议

### Step 1: 基础设施准备 ✅ 已完成（2026-04-08）
- [x] 在 `docker-compose.yaml` 中添加 NATS 服务（`bundles/keeper/templates/15-nats.yaml`）
- [x] 更新 `.env` 配置（Matrix bot、NATS 环境变量）
- [x] 更新 `bellkeeper-init.sh` 导出 Matrix/Redis/NATS 环境变量
- [x] 更新 `50-bellkeeper.yaml` 添加对 redis/nats 的 depends_on
- [x] 同步到远程并部署（`bundle keeper up keeper`）
- [x] 测试 Redis 和 NATS 连接（均 healthy）

**实际产出**:
- `bundles/keeper/templates/00-base.yaml` — 新增 `kp-nats-data` volume
- `bundles/keeper/templates/15-nats.yaml` — 新增 NATS 服务定义
- `bundles/keeper/templates/50-bellkeeper.yaml` — 添加 redis/nats 依赖
- `bundles/keeper/templates/bellkeeper-init.sh` — 导出 Matrix/Redis/NATS 环境变量
- `hosts/keeper/.env` — 新增 Matrix/NATS 配置项

**注意**: Redis 已有（被 RSSHub 使用），无需新增。Bot token 复用现有 `MATRIX_BOT_TOKEN`。

### Step 2: 数据模型实施 ✅ 已完成（2026-04-08）
- [x] 创建 7 个 GORM Model（`internal/model/matrix.go`）
- [x] 注册到 AutoMigrate（`internal/model/db.go`）
- [x] 创建 SeedMatrixPlatform 种子函数（4 频道 + 6 命令）
- [x] 创建 7 个 Repository（`internal/repository/matrix_*.go`）
- [x] 注册到 Repositories 聚合结构（`internal/repository/repository.go`）
- [x] 编译通过（`go build ./...`）

**实际产出**:
- `internal/model/matrix.go` — MatrixRoom, MatrixChannel, MatrixCommand, MatrixEvent, MatrixNotification, MatrixCommandLog, MatrixSyncState + SeedMatrixPlatform
- `internal/repository/matrix_room.go` — 房间 CRUD
- `internal/repository/matrix_channel.go` — 频道 CRUD
- `internal/repository/matrix_command.go` — 命令 CRUD
- `internal/repository/matrix_event.go` — 事件审计 + 去重查询
- `internal/repository/matrix_notification.go` — 通知记录 + 状态更新
- `internal/repository/matrix_command_log.go` — 命令日志 + 完成更新
- `internal/repository/matrix_sync_state.go` — Sync token 持久化

**注意**: 未使用独立 SQL 迁移脚本，改用 GORM AutoMigrate（与现有模式一致）。表结构会在 Bellkeeper 下次重建时自动创建。

---

### ✅ Step 3: Matrix Gateway 实施 ✅ 已完成（2026-04-09）
- [x] 在 `internal/config/config.go` 中添加 MatrixConfig / RedisConfig / NATSConfig 结构体
- [x] 在 `config/bellkeeper.yaml` 中添加 matrix / redis / nats 配置段
- [x] `go get maunium.net/go/mautrix` 引入 SDK 依赖
- [x] `go get github.com/redis/go-redis/v9` 引入 Redis 客户端
- [x] `go get github.com/nats-io/nats.go` 引入 NATS 客户端

**核心实现**:
- [x] 实现 Redis 客户端封装（`internal/matrix/infra/redis.go`）
- [x] 实现 NATS 客户端封装（`internal/matrix/infra/nats.go`）
- [x] 实现 mautrix-go 客户端初始化（`internal/matrix/gateway/client.go`）
- [x] 实现 Sync Loop（`internal/matrix/gateway/sync.go`）
- [x] 实现事件去重和审计（`matrix_events` 表 + Redis 缓存）
- [x] 实现 sync token 持久化（Redis 热存 + PostgreSQL 冷存）
- [x] 在 Bellkeeper 启动流程中注册 Matrix Gateway 生命周期

### ✅ Step 4: Notification Gateway 实施 ✅ 已完成（2026-04-09）
- [x] 实现 `/api/matrix/notify` HTTP 接口
- [x] 实现频道路由（`matrix_channels` 表）
- [x] 实现 NATS 异步队列
- [x] 实现 notification worker
- [x] 实现重试逻辑

### ✅ Step 5: Command Router 实施 ✅ 已完成（2026-04-09）
- [x] 实现命令解析（`!` 前缀识别、参数提取）
- [x] 实现命令路由（`matrix_commands` 表）
- [x] 实现基础权限校验
- [x] 实现 `!help` 和 `!status` 命令

### ✅ Step 6: 简单命令处理器实施 ✅ 已完成（2026-04-09）
- [x] 实现 Memos todo handler（`!列表`, `!新增`, `!完成`）
- [x] 实现 RAGFlow QA handler（`!问`, `!搜`）
- [x] 实现 n8n workflow trigger handler

### ✅ Step 7: Admin API 实施 ✅ 已完成（2026-04-09）
- [x] 实现房间管理 API
- [x] 实现频道管理 API
- [x] 实现命令管理 API
- [x] 实现审计查询 API

### Step 8: 集成测试与迁移 ✅ 已完成（2026-04-13）
- [x] 端到端测试（命令 + 通知）
- [x] 命令从 DB 加载（`router.go` 已实现 `loadCommandsFromDB`）
- [x] Knowledge Handlers 已启用（`!问/!搜` 调用 `AskService/SearchService`）
- [x] 命令日志记录（`ExecuteFromMessage` 自动记录）

**实际产出**:
- `internal/matrix/command/router.go` — `loadCommandsFromDB()` 从数据库加载命令
- `internal/service/command.go` — `SetKnowledgeHandlers()` 注册知识问答处理器
- `cmd/bellkeeper/main.go` — `commandSvc.SetKnowledgeHandlers()` 启用知识命令

### Step 9: 监控与文档 ✅ 已完成（2026-04-13）
- [x] Prometheus metrics 已添加（MatrixCommandsTotal, MatrixMessagesTotal）
- [x] 健康检查接口已完善（`/api/health/detailed`）
- [x] RSS Fetcher 健康检查已添加（`/api/health/detailed` 包含 rss_fetcher 状态）
- [x] 本文档更新

**实际产出**:
- `internal/metrics/metrics.go` — Matrix 命令和消息指标已定义
- `internal/service/health.go` — `checkRSSFetcher()` 方法
- `/api/health/detailed` — 包含 meilisearch 和 rss_fetcher 服务状态

**进度**: Step 1-9 全部完成

---

## 六、风险评估与缓解

### 风险 1: mautrix-go 学习曲线
- **影响**: 中
- **缓解**: 先实现最小 sync loop，参考官方示例和文档
- **预留时间**: +2天

### 风险 2: NATS JetStream 配置复杂
- **影响**: 中
- **缓解**: 使用默认配置，先实现基础队列，后续再优化
- **预留时间**: +1天

### 风险 3: 事件重复处理
- **影响**: 高
- **缓解**: 
  - event_id 唯一索引
  - Redis 分布式锁
  - 幂等键设计
- **测试重点**: 重启、网络抖动、并发场景

### 风险 4: 通知风暴
- **影响**: 高
- **缓解**:
  - 频道级限流（Redis 计数器）
  - 批量发送（每批 10 条，间隔 1 秒）
  - 降噪策略（相同内容 5 分钟内去重）
- **测试重点**: 大量通知场景

### 风险 5: Bellkeeper 复杂度上升
- **影响**: 中
- **缓解**:
  - 严格模块拆分
  - 接口清晰定义
  - 单元测试覆盖
  - 代码审查

### 风险 6: 与现有功能冲突
- **影响**: 低
- **缓解**:
  - 路由隔离（`/api/matrix/*`）
  - 数据库表隔离（`matrix_*`）
  - 配置命名空间隔离
- **验证**: 并行开发测试

---

## 七、验收标准

### 功能验收 ✅ 已实现
- [x] bot 能正常接收 Matrix 消息
- [x] bot 重启后 sync 正常续跑，不重复处理旧消息
- [x] `!help` 命令返回命令列表
- [x] `!status` 命令返回系统状态
- [x] `!ping` 命令测试响应
- [x] `!列表` 命令通过 n8n webhook 调用 Memos
- [x] `!新增 <内容>` 命令创建 Memos todo
- [x] `!问 <问题>` 命令调用知识问答
- [x] `POST /api/matrix/notify` 能发送通知到指定频道
- [x] 通知失败自动重试（最多 N 次）
- [x] 命令执行日志可查询
- [x] 通知发送日志可查询
- [x] Admin API 提供管理接口

### 性能验收 🔜 待测试
- [ ] Sync loop 延迟 < 5 秒
- [ ] 命令响应时间 < 2 秒（简单命令）
- [ ] 通知发送延迟 < 10 秒（正常情况）
- [ ] 队列吞吐 > 100 msg/s

### 稳定性验收 🔜 待测试
- [ ] 连续运行 24 小时无崩溃
- [ ] 网络抖动后自动恢复
- [ ] Redis/NATS 重启后自动重连
- [ ] 无内存泄漏

### 可观测性验收 🔜 待完善
- [x] 所有关键操作有日志
- [ ] Prometheus metrics 可用
- [x] 健康检查接口正常

---

## 八、后续优化方向

### Phase 2 优化
- 命令别名支持（`!list` = `!列表`）
- 多轮对话支持（会话状态管理）
- 命令参数校验增强
- 通知模板系统
- 通知聚合与静默

### Phase 3 优化
- Application Service 迁移（降低延迟）
- 多 bot identity 支持
- 插件化 handler 注册
- 管理 UI
- 值班规则与升级策略

---

## 九、关键决策记录

| 决策 | 理由 | 日期 |
|------|------|------|
| 使用 mautrix-go | Go 生态最成熟的 Matrix SDK | 2026-04-08 |
| 使用 NATS 而非 RabbitMQ | 轻量级，Go 原生支持，适合单机部署 | 2026-04-08 |
| 立即引入 Redis | 避免后续迁移成本，sync token 和限流需要高性能存储 | 2026-04-08 |
| 完整 MVP（通知+命令） | 一次性建立平台骨架，避免多次重构 | 2026-04-08 |
| 简单命令直接处理 | 减少 n8n 调用开销，提升响应速度 | 2026-04-08 |

---

## 十、参考资料

### 官方文档
- [mautrix-go Documentation](https://pkg.go.dev/maunium.net/go/mautrix)
- [NATS JetStream Guide](https://docs.nats.io/nats-concepts/jetstream)
- [Redis Go Client](https://redis.uptrace.dev/)

### 示例代码
- [mautrix-go Examples](https://github.com/mautrix/go/tree/main/example)
- [NATS Go Examples](https://github.com/nats-io/nats.go/tree/main/examples)

### SilkSpool 相关文档
- [MATRIX-ARCHITECTURE.md](MATRIX-ARCHITECTURE.md)
- [MATRIX-IMPLEMENTATION-PLAN.md](MATRIX-IMPLEMENTATION-PLAN.md)
- [MATRIX-DATA-MODEL.md](MATRIX-DATA-MODEL.md)
- [MATRIX-API-CONTRACTS.md](MATRIX-API-CONTRACTS.md)
