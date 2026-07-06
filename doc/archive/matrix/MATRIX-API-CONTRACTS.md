# Matrix Bot Platform API 契约

> 本文档定义 Bellkeeper 中 Matrix 平台的外部 API 与内部调用契约。

---

## 1. API 分类

建议分为四类：

1. **Notify API** — 多系统通知入口
2. **Admin API** — 管理与治理接口
3. **Command API** — 命令执行与调试接口
4. **Connector API** — 外部系统适配与回调入口

---

## 2. Notify API

### 2.1 `POST /api/matrix/notify`

用途：
- 统一接收来自 n8n、运维脚本、Bellkeeper 子模块、其他服务的通知请求

请求示例：

```json
{
  "channel": "alerts",
  "title": "磁盘告警",
  "message": "knowledge 根分区使用率 87%",
  "html": "<b>磁盘告警</b><br>knowledge 根分区使用率 87%",
  "severity": "warning",
  "source": "ops-monitor",
  "dedupe_key": "disk-knowledge-root",
  "metadata": {
    "host": "knowledge",
    "mount": "/"
  }
}
```

响应示例：

```json
{
  "success": true,
  "data": {
    "trace_id": "ntf_01...",
    "job_id": 123,
    "status": "queued"
  }
}
```

规则：
- `channel` 必填
- `message` 与 `html` 至少一个非空
- `dedupe_key` 建议由调用方提供
- 平台负责路由、审计、重试

### 2.2 `POST /api/matrix/notify/test`

用途：
- 发送测试通知
- 验证频道与房间映射

---

## 3. Admin API

### 3.1 频道与房间

- `GET /api/matrix/channels`
- `POST /api/matrix/channels`
- `PUT /api/matrix/channels/:id`
- `GET /api/matrix/rooms`
- `POST /api/matrix/rooms`
- `PUT /api/matrix/rooms/:id`
- `POST /api/matrix/channel-bindings`
- `DELETE /api/matrix/channel-bindings/:id`

### 3.2 命令注册

- `GET /api/matrix/commands`
- `POST /api/matrix/commands`
- `PUT /api/matrix/commands/:id`
- `POST /api/matrix/commands/:id/aliases`

### 3.3 权限策略

- `GET /api/matrix/policies`
- `POST /api/matrix/policies`
- `PUT /api/matrix/policies/:id`
- `DELETE /api/matrix/policies/:id`

### 3.4 审计与运行态

- `GET /api/matrix/events`
- `GET /api/matrix/deliveries`
- `GET /api/matrix/jobs/commands`
- `GET /api/matrix/jobs/notifications`
- `POST /api/matrix/jobs/notifications/:id/replay`
- `GET /api/matrix/health`
- `GET /api/matrix/status`

### 3.5 连接器管理

- `GET /api/matrix/connectors`
- `POST /api/matrix/connectors`
- `PUT /api/matrix/connectors/:id`

---

## 4. Command API

### 4.1 `POST /api/matrix/commands/execute`

用途：
- 管理台或内部系统进行命令调试/模拟执行

请求示例：

```json
{
  "room_id": "!qa:matrix.singll.net",
  "sender": "@singll:matrix.singll.net",
  "command": "问",
  "args": "RAGFlow 解析状态",
  "trace_id": "cmd_test_001"
}
```

响应示例：

```json
{
  "success": true,
  "data": {
    "trace_id": "cmd_test_001",
    "status": "accepted"
  }
}
```

用途说明：
- 不替代真实 Matrix 命令入口
- 主要用于调试、回归与管理台操作

---

## 5. 内部返回模型

建议所有 command handler 返回统一结构：

```json
{
  "reply": {
    "text": "处理完成",
    "html": "<b>处理完成</b>"
  },
  "side_effects": [
    {
      "type": "notify",
      "channel": "ops"
    }
  ],
  "metadata": {
    "duration_ms": 120
  }
}
```

notify worker 返回统一结构：

```json
{
  "delivery": {
    "room_id": "!alerts:matrix.singll.net",
    "txn_id": "txn_xxx",
    "status": "sent"
  }
}
```

---

## 6. Connector 契约

### 6.1 n8n connector

Bellkeeper 调用 n8n：
- 触发 workflow
- 查询执行状态
- 获取回调结果

建议抽象字段：
- `workflow_name`
- `payload`
- `timeout_seconds`
- `callback_mode`

### 6.2 Memos connector

命令 handler 只拿统一 todo 语义，不直接感知 Memos 细节：
- `list_todos`
- `create_todo`
- `complete_todo`
- `delete_todo`

### 6.3 QA connector

统一抽象为：
- `search`
- `ask`

由 connector 内部决定调用 Bellkeeper 现有 RAG/LLM 能力。

---

## 7. 认证建议

内部系统调用 `/api/matrix/notify`：
- 继续使用 `X-API-Key`

管理接口：
- 使用现有 Authelia + API Key 模式

后续如需更细粒度鉴权，可为 Matrix 平台单独增加 connector 级 token。
