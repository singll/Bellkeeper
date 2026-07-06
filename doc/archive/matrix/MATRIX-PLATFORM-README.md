# Bellkeeper Matrix 基础设施总览

> 本文档是 Bellkeeper 中 Matrix 机器人/通知基础设施的总入口。
> 当前口径：**Matrix 协议入口在 txhk，Matrix Bot Platform 控制中心在 keeper，由 Bellkeeper 承载核心运行时、治理与管理 API；n8n 退居编排层。**

---

## 定位

Bellkeeper 不再只作为知识治理与 API 聚合层的一部分，而是承担 SilkSpool 的 **Matrix Bot Platform** 实施落地：

- **Command Plane**：承接 Matrix 命令、权限校验、房间治理、命令路由
- **Notification Plane**：承接多系统通知入口、逻辑频道路由、发送审计、降噪与重试
- **Governance Plane**：承接房间、命令、权限、连接器、模板、通知策略的统一配置与管理

这意味着：

- Matrix 仍然是 SilkSpool 的控制平面
- `txhk` 继续承载 Matrix homeserver / 公网入口
- `keeper` 上的 Bellkeeper 成为 Matrix bot 的**正式控制中心与实现主体**
- n8n 继续保留，但只作为被调用的编排与自动化后端

---

## 文档结构

### 设计与架构

1. [MATRIX-ARCHITECTURE.md](MATRIX-ARCHITECTURE.md)
   - 总体架构、服务拆分、部署拓扑、职责边界
2. [MATRIX-IMPLEMENTATION-PLAN.md](MATRIX-IMPLEMENTATION-PLAN.md)
   - 实施阶段、MVP 到正式版路线图、里程碑与验收口径
3. [MATRIX-DATA-MODEL.md](MATRIX-DATA-MODEL.md)
   - 表结构、核心实体、状态与索引建议
4. [MATRIX-API-CONTRACTS.md](MATRIX-API-CONTRACTS.md)
   - Admin API、通知 API、命令执行 API、内部连接器契约
5. [MATRIX-COMMAND-MODEL.md](MATRIX-COMMAND-MODEL.md)
   - 命令模型、权限模型、房间作用域、处理器模型

### 通知子目录

通知系统单独放在 [notifications/](notifications/)：

- [notifications/README.md](notifications/README.md) — 通知系统总览
- [notifications/NOTIFICATION-MODEL.md](notifications/NOTIFICATION-MODEL.md) — 通知事件模型、频道模型、模板与发送策略
- [notifications/NOTIFICATION-OPERATIONS.md](notifications/NOTIFICATION-OPERATIONS.md) — 通知运行、排障、重试、降噪与审计

---

## 目标架构摘要

```text
Internet
  ↓
txhk
  ├─ Conduit / Matrix Homeserver
  └─ Caddy / 公网入口

keeper
  ├─ Bellkeeper Matrix Bot Core
  │   ├─ Matrix Gateway / Sync Loop
  │   ├─ Event Router
  │   ├─ Permission Engine
  │   ├─ Command Handlers
  │   ├─ Notification Gateway
  │   ├─ Admin API / Registry
  │   └─ Audit / Metrics
  ├─ PostgreSQL
  ├─ Redis
  ├─ Queue (RabbitMQ / NATS JetStream)
  ├─ n8n
  ├─ Memos
  ├─ RAG / Search / Bellkeeper 其他能力
  └─ 其他家庭网络服务
```

---

## 与 SilkSpool 文档的关系

SilkSpool `doc/modules/matrix/README.md` 只保留：

- Matrix = 控制平面的定位
- Matrix homeserver 部署位置（txhk）
- Bellkeeper 是 Matrix 基础设施的实施与权威文档入口
- 到本目录的跳转链接

所有真正的实施、架构、表结构、API 契约、命令模型、通知模型与路线图，统一以下面目录为准：

- `Bellkeeper/doc/MATRIX-*.md`
- `Bellkeeper/doc/notifications/*`

---

## 一句话定义

**Bellkeeper Matrix Bot Platform 是 SilkSpool 的控制与通知中枢：它把 Matrix 从“若干 workflow 的交互壳”提升为“长期运行、可治理、可扩展、可审计、可对接多系统的基础设施平台”。**
