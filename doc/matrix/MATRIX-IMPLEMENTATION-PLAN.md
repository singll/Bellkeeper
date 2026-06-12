# Matrix Bot Platform 实施路线图

> 本文档给出 Bellkeeper 中 Matrix 基础设施的实施分期、服务拆分顺序与验收口径。

---

## 1. 总体策略

采用“先平台骨架、再系统接入、再治理增强”的方式推进，不再继续扩展 n8n 轮询机器人原型。

原则：
- 先建立 Bellkeeper 内的正式 platform 边界
- 再让 n8n 退回编排层
- 最后把现有 Matrix 发送和命令能力迁移到平台化接口

---

## 2. 分阶段路线图

### Phase 1：平台骨架（MVP）

目标：让 Bellkeeper 成为一个可运行的 Matrix Bot Core 原型。

交付内容：
- Matrix Gateway / sync loop
- 基础事件表与 sync state
- 基础 command router
- 基础 notification gateway
- PostgreSQL + Redis 接入
- 最小 Admin API
- 最小房间/频道注册模型
- 最小权限模型

MVP 必须支持：
- 从 Matrix 接收命令
- 执行 `!help` / `!status` / `!commands`
- 接收 `POST /api/notify` 并发到指定逻辑频道
- 记录命令与通知审计日志
- 重启后不重复处理旧消息

验收标准：
- bot 重启后 sync 正常续跑
- 命令回复不重复
- 通知支持 text/html 双格式
- 审计可查

### Phase 2：业务接入

目标：把现有主要能力迁入平台。

交付内容：
- Memos todo connector
- QA / Search connector
- n8n workflow trigger connector
- Ops / backup / digest notify migration
- 逻辑频道 → 房间配置 API

验收标准：
- 现有 `!列表` / `!新增` / `!问` / `!搜` 均能通过 Bellkeeper 平台执行
- 现有 O03/O05/K08 等通知改经 `/api/notify`
- n8n 不再直接承担 Matrix runtime

### Phase 3：治理与运维增强

目标：让平台具备长期运行与可治理能力。

交付内容：
- 完整命令注册表
- 完整权限策略与房间范围控制
- 限流、降噪、通知去重
- 失败重试与死信
- 管理命令：`!rooms` / `!routes` / `!audit` / `!notify test`
- Prometheus metrics

验收标准：
- 可通过 Admin API 查询路由、命令、权限、失败记录
- 可重放失败通知
- 可禁用特定 handler / route

### Phase 4：正式版

目标：从“好用平台”升级到“成熟基础设施”。

交付内容：
- 管理 UI
- 插件化 handler 注册
- 多 bot identity
- 可选 Application Service 扩展
- 通知聚合、静默、升级、值班规则

---

## 3. 代码落点建议

建议在 Bellkeeper 内新增以下模块：

```text
internal/
  matrix/
    gateway/
    envelope/
    router/
    command/
    notify/
    policy/
    registry/
    worker/
    connector/
```

并在现有分层中新增：

- `internal/model/matrix_*.go`
- `internal/repository/matrix_*.go`
- `internal/service/matrix_*.go`
- `internal/handler/matrix_*.go`

推荐新增的 handler：
- `matrix_admin.go`
- `matrix_notify.go`
- `matrix_room.go`
- `matrix_command.go`
- `matrix_audit.go`

---

## 4. 与现有功能的迁移关系

### 4.1 现有 n8n Matrix 工作流

现状：
- M01：轮询 + 命令路由
- B01：通知发送
- M02：todo 命令
- M03：qa 命令

迁移目标：
- M01 → Bellkeeper Matrix Gateway + Command Router
- B01 → Bellkeeper Notification Gateway
- M02 → Bellkeeper Todo Connector / Command Handler
- M03 → Bellkeeper QA Connector / Command Handler

### 4.2 n8n 的后续角色

保留：
- 工作流触发
- 编排与定时
- 复杂多步处理

不再承担：
- sync loop
- 核心命令路由
- 统一 Matrix 发送 runtime

---

## 5. 部署顺序建议

1. 先在 Bellkeeper 内加 Matrix 配置段与 DB 模型
2. 实现最小 sync loop 与 `/api/notify`
3. 实现最小命令 handler
4. 接入 Redis / Queue
5. 迁移通知链路
6. 迁移命令链路
7. 增强 Admin API
8. 最后再考虑管理 UI

---

## 6. 风险与控制

### 风险 1：一次性替换过多
控制：先让新平台以旁路方式运行，再逐步迁移 n8n 调用方

### 风险 2：Matrix 事件处理重复
控制：event_id 去重 + sync token 持久化 + 幂等键

### 风险 3：通知风暴
控制：逻辑频道级限流、聚合、静默与 dedupe key

### 风险 4：Bellkeeper 复杂度上升
控制：严格按模块拆分，避免把 Matrix 实现写成若干散乱 handler

---

## 7. 里程碑建议

- M1：Bellkeeper 能发 Matrix 通知
- M2：Bellkeeper 能收 Matrix 命令并回复
- M3：todo / qa / ops 接入完成
- M4：Admin API 与审计能力完成
- M5：n8n Matrix runtime 下线

---

## 8. 最终验收口径

正式版完成时，应满足：
- Matrix 收发不依赖 n8n runtime
- 所有通知统一经 Bellkeeper `/api/notify`
- 所有命令统一经 Bellkeeper Command Router
- 房间、频道、命令、权限、模板均可治理
- 运行状态、失败、审计可见
- 能稳定支持多系统接入
