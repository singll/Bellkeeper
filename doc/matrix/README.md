# Matrix 控制平面

> **状态**: ✅ Phase 1 MVP 已完成 — Step 1-7 已完成，Step 8-9 待测试验证
>
> **定位**: 把 Matrix 从"若干 workflow 的交互壳"提升为"长期运行、可治理、可扩展的基础设施平台"

---

## ⚠️ 重要说明

**本目录下的设计文档已进入实施阶段。**

当前进度（2026-04-09）：
- ✅ Step 1: 基础设施已部署（NATS 2.10 + Redis 7 运行中）
- ✅ Step 2: 数据模型已创建（7 个 Model + Repository，编译通过）
- ✅ Step 3: Matrix Gateway 实现完成
- ✅ Step 4: Notification Gateway 实现完成
- ✅ Step 5: Command Router 实现完成
- ✅ Step 6: 简单命令处理器实现完成
- ✅ Step 7: Admin API 实现完成
- 🔜 Step 8: 集成测试与迁移（待执行）
- 🔜 Step 9: 监控与文档（待执行）

---

## 文档结构

### 总览与定位
- [MATRIX-PLATFORM-README.md](MATRIX-PLATFORM-README.md) — Matrix 平台总入口，定位说明

### 架构设计
- [MATRIX-ARCHITECTURE.md](MATRIX-ARCHITECTURE.md) — 总体架构、服务拆分、部署拓扑、职责边界

### 实施计划
- [MATRIX-IMPLEMENTATION-PLAN.md](MATRIX-IMPLEMENTATION-PLAN.md) — 实施阶段、MVP 到正式版路线图、里程碑与验收口径

### 数据模型
- [MATRIX-DATA-MODEL.md](MATRIX-DATA-MODEL.md) — 表结构、核心实体、状态与索引建议

### API 契约
- [MATRIX-API-CONTRACTS.md](MATRIX-API-CONTRACTS.md) — Admin API、通知 API、命令执行 API、内部连接器契约

### 命令模型
- [MATRIX-COMMAND-MODEL.md](MATRIX-COMMAND-MODEL.md) — 命令模型、权限模型、房间作用域、处理器模型

### 通知平面
- [notifications/](notifications/) — 通知系统子目录
  - [notifications/README.md](notifications/README.md) — 通知系统总览
  - [notifications/NOTIFICATION-MODEL.md](notifications/NOTIFICATION-MODEL.md) — 事件模型、频道模型、模板与路由
  - [notifications/NOTIFICATION-OPERATIONS.md](notifications/NOTIFICATION-OPERATIONS.md) — 运维、排障、重试、降噪与审计

---

## 目标架构摘要

```text
Internet
  ↓
txhk (Matrix Homeserver)
  ├─ Conduit
  └─ Caddy
  ↓
keeper (Bellkeeper Matrix Bot Core)
  ├─ Matrix Gateway / Sync Loop
  ├─ Event Router
  ├─ Permission Engine
  ├─ Command Handlers
  ├─ Notification Gateway
  ├─ Admin API / Registry
  └─ Audit / Metrics
```

---

## 核心能力（设计）

Matrix 平台分为三个平面：

### 1. 命令平面 (Command Plane)
**职责**: 接收和处理 Matrix 命令
- Matrix 命令接收与解析（`!搜`, `!问`, `!待办` 等）
- 权限校验（用户级、房间级）
- 命令路由到处理器
- 房间治理与作用域管理

**核心组件**:
- Matrix Gateway / Sync Loop
- Event Router
- Permission Engine
- Command Handlers

### 2. 通知平面 (Notification Plane)
**职责**: 统一多系统通知出口

详见 [notifications/](notifications/) 子目录

**核心能力**:
- 多系统通知入口（`POST /api/matrix/notify`）
- 逻辑频道路由（`alerts`, `daily`, `todo`, `qa` 等）
- 模板渲染与格式化
- 频控、去重、聚合
- 重试与死信处理
- 发送审计与追踪

**核心组件**:
- Notification Gateway
- Channel Router
- Template Engine
- Retry Queue

### 3. 治理平面 (Governance Plane)
**职责**: 平台配置与管理
- 房间配置管理
- 命令注册与权限配置
- 连接器管理
- 模板管理
- 通知策略配置

**核心组件**:
- Admin API
- Registry
- Audit / Metrics

---

## 与 n8n 的关系

### Bellkeeper 负责
- Matrix 长连接与事件监听
- 命令解析与权限校验
- **简单命令的直接处理**（如 `!列表`、`!新增` 直接调 Memos/RAGFlow API）
- 房间治理与状态管理
- 通知网关与频道路由

### n8n 负责
- **需要多步骤编排的复杂流程**（如 RSS采集→解析→分类→入库→总结）
- **定时任务调度**（如每日摘要、健康监控）
- **不适合在 Bellkeeper 中实现的重逻辑**

**交互方式**: 
- 简单命令：Bellkeeper 直接调用后端 API（Memos、RAGFlow、TrueNAS）
- 复杂流程：Bellkeeper 通过 HTTP 调用 n8n webhook 触发工作流

---

## 实施前的准备工作

**详细清单**: 请查看 [IMPLEMENTATION-CHECKLIST.md](IMPLEMENTATION-CHECKLIST.md)

### 技术选型（已确认）
- ✅ **Matrix SDK**: mautrix-go
- ✅ **消息队列**: NATS (JetStream)
- ✅ **缓存/状态存储**: Redis 7.x
- ✅ **MVP 范围**: 完整 MVP（通知网关 + 命令路由 + 简单命令处理器）

### 关键准备项
1. **基础设施**: Docker Compose 新增 Redis 和 NATS 服务
2. **数据模型**: 7 张核心表（rooms, channels, commands, events, notifications, command_logs, sync_state）
3. **代码结构**: 新增 `internal/matrix/` 模块（gateway, command, notify, policy, registry, worker, queue）
4. **环境变量**: Matrix bot token、Redis 配置、NATS 配置
5. **实施周期**: 约 15-20 个工作日

### 快速开始
1. 阅读 [IMPLEMENTATION-CHECKLIST.md](IMPLEMENTATION-CHECKLIST.md) 了解完整技术方案
2. 阅读 [MATRIX-ARCHITECTURE.md](MATRIX-ARCHITECTURE.md) 了解架构设计
3. 阅读 [MATRIX-DATA-MODEL.md](MATRIX-DATA-MODEL.md) 了解数据模型
4. 按照清单中的 Step 1-9 逐步实施

---

## 与文件入库模块的并行开发

### 代码隔离
- Matrix: `internal/service/matrix_*.go`, `internal/handler/matrix_*.go`
- 文件入库: `internal/service/file_*.go`, `internal/handler/file_*.go`
- **无共享代码，可并行开发**

### 路由隔离
- Matrix: `/api/matrix/*`
- 文件入库: `/api/files/*`
- **无路由冲突**

### 数据库隔离
- Matrix: `matrix_*` 系列表（新建）
- 文件入库: `article_tags` 表（已存在）
- **无表依赖**

### 配置隔离
- Matrix: `matrix` 配置块
- 文件入库: `file_ingestion` 配置块
- **独立配置命名空间**

**结论**: ✅ 可以同步进行，互不干扰

---

## 参考资料

### Matrix 协议
- [Matrix Spec](https://spec.matrix.org/)
- [Client-Server API](https://spec.matrix.org/latest/client-server-api/)
- [Application Service API](https://spec.matrix.org/latest/application-service-api/)

### Go Matrix SDK
- [mautrix-go](https://github.com/mautrix/go) — 推荐的 Go Matrix SDK

### SilkSpool 相关文档
- `SilkSpool/doc/modules/matrix/README.md` — Matrix 控制平面定位
- `SilkSpool/doc/old/evaluations/MATRIX-BOT-EVALUATION.md` — Matrix 机器人方案评估

---

## 一句话定义

**Bellkeeper Matrix Bot Platform 是 SilkSpool 的控制与通知中枢：它把 Matrix 从"若干 workflow 的交互壳"提升为"长期运行、可治理、可扩展、可审计、可对接多系统的基础设施平台"。**
