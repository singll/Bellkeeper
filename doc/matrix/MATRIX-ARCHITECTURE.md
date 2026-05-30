# Matrix Bot Platform 架构设计

> 本文档定义 Bellkeeper 中 Matrix 基础设施的正式目标架构。

---

## 1. 设计目标

目标不是实现一个单点 bot，而是建设一套长期可运行的 **Matrix Bot Platform**，满足：

- 功能齐全：通知、命令、管理、路由、审计、权限
- 长期稳定：具备幂等、重试、去重、恢复与观测能力
- 可治理：房间、频道、命令、连接器与策略可被统一管理
- 可扩展：可以逐步接入更多系统与处理器，不依赖 n8n workflow 分支硬编码
- 可替代：下游系统可以变化，但 Matrix 作为控制与通知中枢的语义稳定

---

## 2. 顶层部署决策

### 2.1 主机职责

- **txhk**：只承载 Matrix homeserver、公网入口、联邦与客户端接入
- **keeper**：承载 Matrix Bot Platform 控制中心与业务平面

原因：

1. Bellkeeper、n8n、Memos、RAG、运维服务都在 keeper 侧，链路最短
2. keeper 更适合作为家庭网络内部的控制中枢
3. txhk 不应继续膨胀成第二应用中心
4. 控制中心和能力后端共址，更利于后续治理、审计与性能控制

---

## 3. 系统分层

```text
Layer A  Transport / Protocol
Layer B  Gateway / Ingress
Layer C  Command & Notification Core
Layer D  Governance / Admin API
Layer E  Connectors / Workers / Automation
Layer F  Storage / Queue / Cache / Observability
```

### 3.1 Layer A — Transport / Protocol

部署位置：`txhk`

职责：
- Matrix homeserver（Conduit）
- Matrix federation / client-server API
- 媒体与反向代理入口

说明：
- 该层不承担 bot 业务逻辑
- 仅提供协议与公网入口能力

### 3.2 Layer B — Gateway / Ingress

部署位置：`keeper`，Bellkeeper 内新模块

职责：
- 维护 bot account 与 Matrix 会话
- 读取 `/sync` 增量事件
- 标准化事件为内部 envelope
- 去重、落审计、投递内部任务队列

说明：
- 第一版采用 Matrix Client-Server API
- Application Service 预留为后续扩展位，不作为首版前提

### 3.3 Layer C — Command & Notification Core

部署位置：`keeper`

拆分为两个核心平面：

#### Command Plane
- 命令解析（`!` / `！`）
- alias / 子命令 / 参数模型
- 房间作用域与权限校验
- handler 调度
- 结构化回复

#### Notification Plane
- 多系统统一通知入口
- channel → room 路由
- 文本/HTML 模板渲染
- 幂等、重试、频控、降噪
- 审计与发送结果跟踪

### 3.4 Layer D — Governance / Admin API

职责：
- 管理房间、逻辑频道、命令注册、权限策略、连接器与模板
- 查看系统健康状态、事件流、发送记录、失败重试情况
- 提供后续管理 UI 的后端接口

### 3.5 Layer E — Connectors / Workers / Automation

职责：
- 对接 n8n、Memos、RAG、运维服务、RSS、备份等系统
- 将外部系统适配为统一的通知/命令后端
- 异步执行长任务与可重试任务

说明：
- n8n 继续存在，但不作为 Matrix runtime
- n8n 应被视为一个下游执行器或 workflow backend

### 3.6 Layer F — Storage / Queue / Cache / Observability

职责：
- PostgreSQL：配置、注册、审计、持久状态
- Redis：短期上下文、去重、限流、会话与幂等键
- Queue：通知任务、命令任务、重试、死信
- Metrics / Logs：运行观测与排障

---

## 4. 服务拆分

推荐服务拆分如下：

### 4.1 `matrix-bot-core`

职责：
- 管理 Matrix 会话
- 执行 sync loop
- 生成事件 envelope
- 事件入队
- 处理基础回复流程

### 4.2 `matrix-command-worker`

职责：
- 从队列消费命令事件
- 命令解析、鉴权、调度 handler
- 生成结构化回复结果

### 4.3 `matrix-notify-worker`

职责：
- 从队列消费通知任务
- 模板渲染、路由、幂等、重试、速率限制
- 将消息发送回 Matrix

### 4.4 `matrix-admin-api`

职责：
- 暴露治理与管理接口
- 提供运行状态查询、配置 CRUD、重试与审计查询

### 4.5 `connector workers`

职责：
- 与 n8n、Memos、RAG、Ops 等系统进行适配
- 管理耗时或强依赖外部系统的任务

> 第一阶段为了减少运维成本，可以先在同一 Bellkeeper 进程中以模块方式实现；但文档与代码结构必须按上述逻辑边界拆分，便于未来独立部署。

---

## 5. 推荐技术栈

- **核心语言**：Go
- **Web/API**：Gin
- **数据库**：PostgreSQL
- **缓存**：Redis
- **队列**：RabbitMQ 或 NATS JetStream
- **日志**：Zap 结构化日志
- **观测**：Prometheus + Grafana + Loki（或至少 Prometheus + 结构化日志）

选型理由：

1. 与 Bellkeeper 现有 Go 栈一致
2. 更适合作为长期守护进程与基础设施组件
3. 并发、内存、部署与可维护性优于将核心 runtime 放入 n8n

---

## 6. 关键边界

### 6.1 Bellkeeper 负责
- Matrix bot runtime
- 通知与命令治理
- 注册中心与管理 API
- 发送审计、失败重试、限速与去重

### 6.2 n8n 负责
- 工作流编排
- 定时任务
- 长链路自动化执行
- 被 Matrix 命令触发后的某些业务流程

### 6.3 SilkSpool 负责
- 整体基础设施与主机分工文档
- Matrix 作为控制平面的总定位
- 指向 Bellkeeper 文档的权威入口

---

## 7. 关键数据流

### 7.1 命令流

```text
Matrix 房间消息
  ↓
Matrix Gateway / Sync Loop
  ↓
Event Envelope + 去重 + 审计
  ↓
Command Queue
  ↓
Command Worker
  ↓
Permission / Scope Check
  ↓
Handler / Connector
  ↓
Reply Result
  ↓
Notification Gateway
  ↓
Matrix 房间回复
```

### 7.2 通知流

```text
外部系统（n8n / Bellkeeper 子模块 / Ops / Backup / RSS）
  ↓
POST /api/notify
  ↓
Notification Gateway
  ↓
Channel Route + Template + Dedupe + Retry
  ↓
Notification Queue
  ↓
Notify Worker
  ↓
Matrix Send API
  ↓
房间通知
```

---

## 8. 高可用与恢复原则

- sync token 持久化
- event id 去重
- 通知幂等键
- worker 可重启、任务可重试
- 死信队列保留失败任务
- 管理 API 提供失败任务重放能力
- 所有发送与命令执行均记录审计日志

---

## 9. 后续扩展位

- Application Service 接入
- 多 bot identity
- 管理 UI
- 插件注册中心
- 多租户 / 多空间隔离
- 通知聚合/静默/升级策略

---

## 10. 总结

Bellkeeper 中的 Matrix 基础设施不是“把 bot 逻辑从 n8n 移进去”这么简单，而是要把 Matrix 升级为一个正式的平台：

- 有运行时
- 有治理面
- 有配置中心
- 有命令平面
- 有通知平面
- 有审计与观测
- 有 worker 与 connector

这是 SilkSpool 后续长期运行、跨系统联动与可治理控制面的正式落地方式。
