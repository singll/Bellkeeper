# Matrix Bot Platform 数据模型

> 本文档定义 Bellkeeper 中 Matrix 基础设施所需的核心实体与表结构建议。

---

## 1. 设计原则

- 命令、通知、治理配置全部持久化
- 运行期短状态放 Redis，长期状态放 PostgreSQL
- 审计优先，所有关键动作都有记录
- 允许后续按模块拆表，不在初版过度抽象

---

## 2. 核心实体总览

建议新增以下表：

| 表名 | 用途 |
|------|------|
| `matrix_sync_states` | 记录 bot 的 sync token、上次处理时间、运行状态 |
| `matrix_rooms` | Matrix 房间注册表 |
| `matrix_channels` | 逻辑频道定义 |
| `matrix_channel_bindings` | 逻辑频道到 Matrix 房间的绑定 |
| `matrix_commands` | 命令注册表 |
| `matrix_command_aliases` | 命令别名 |
| `matrix_policies` | 权限与作用域策略 |
| `matrix_event_logs` | 接收到的 Matrix 事件审计日志 |
| `matrix_message_deliveries` | Matrix 发送记录 |
| `matrix_notification_rules` | 通知频控、静默、聚合、重试规则 |
| `matrix_notification_jobs` | 通知任务执行记录 |
| `matrix_connectors` | 对接外部系统的连接器注册 |
| `matrix_command_jobs` | 命令任务执行记录 |

---

## 3. 表结构建议

### 3.1 `matrix_sync_states`

字段建议：
- `id`
- `bot_name`
- `sync_token`
- `last_event_at`
- `last_success_at`
- `status`
- `error_message`
- `updated_at`

用途：
- 保存 sync 续跑状态
- 支持重启恢复
- 支持状态观测

### 3.2 `matrix_rooms`

字段建议：
- `id`
- `room_id`
- `canonical_alias`
- `display_name`
- `room_type`（admin / qa / todo / alerts / daily / infra ...）
- `is_active`
- `metadata_json`
- `created_at`
- `updated_at`

用途：
- 统一管理所有房间
- 不再在 workflow/env 中硬编码 room id

### 3.3 `matrix_channels`

字段建议：
- `id`
- `name`
- `display_name`
- `description`
- `category`
- `default_severity`
- `is_active`
- `created_at`
- `updated_at`

示例：
- `alerts`
- `daily`
- `todo`
- `qa`
- `ops`
- `security`

### 3.4 `matrix_channel_bindings`

字段建议：
- `id`
- `channel_id`
- `room_id`
- `mode`（broadcast / primary / fallback）
- `priority`
- `is_active`
- `created_at`
- `updated_at`

用途：
- 一个逻辑频道可绑定多个房间
- 支持主房间与兜底房间

### 3.5 `matrix_commands`

字段建议：
- `id`
- `name`
- `display_name`
- `handler`
- `category`
- `description`
- `is_enabled`
- `scope_type`（global / room / role）
- `timeout_seconds`
- `metadata_json`
- `created_at`
- `updated_at`

### 3.6 `matrix_command_aliases`

字段建议：
- `id`
- `command_id`
- `alias`
- `created_at`

示例：
- `!help` ↔ `!帮助`
- `!list` ↔ `!列表`

### 3.7 `matrix_policies`

字段建议：
- `id`
- `subject_type`（user / role / room）
- `subject_ref`
- `resource_type`（command / channel / connector）
- `resource_ref`
- `effect`（allow / deny）
- `conditions_json`
- `priority`
- `is_active`
- `created_at`
- `updated_at`

用途：
- 实现权限与作用域控制

### 3.8 `matrix_event_logs`

字段建议：
- `id`
- `event_id`
- `room_id`
- `sender`
- `event_type`
- `body`
- `trace_id`
- `is_command`
- `status`
- `error_message`
- `received_at`
- `processed_at`

索引建议：
- `event_id` unique
- `room_id, received_at`
- `sender, received_at`

### 3.9 `matrix_message_deliveries`

字段建议：
- `id`
- `channel_name`
- `room_id`
- `txn_id`
- `event_id`
- `message_text`
- `message_html`
- `severity`
- `source_system`
- `dedupe_key`
- `status`
- `error_message`
- `sent_at`
- `created_at`

用途：
- 记录实际发送与回执

### 3.10 `matrix_notification_rules`

字段建议：
- `id`
- `channel_name`
- `dedupe_window_seconds`
- `rate_limit_count`
- `rate_limit_window_seconds`
- `quiet_hours_json`
- `aggregation_strategy`
- `retry_policy_json`
- `is_active`
- `created_at`
- `updated_at`

### 3.11 `matrix_notification_jobs`

字段建议：
- `id`
- `trace_id`
- `channel_name`
- `payload_json`
- `status`
- `attempts`
- `next_retry_at`
- `last_error`
- `created_at`
- `updated_at`

### 3.12 `matrix_connectors`

字段建议：
- `id`
- `name`
- `type`
- `target`
- `config_json`
- `is_enabled`
- `created_at`
- `updated_at`

示例：
- `memos-todo`
- `rag-qa`
- `n8n-workflow`
- `ops-restart`

### 3.13 `matrix_command_jobs`

字段建议：
- `id`
- `trace_id`
- `event_id`
- `command_name`
- `args_json`
- `handler`
- `status`
- `attempts`
- `result_json`
- `error_message`
- `created_at`
- `updated_at`

---

## 4. Redis 中的短期状态

建议放 Redis 的内容：
- `matrix:dedupe:event:{event_id}`
- `matrix:notify:dedupe:{dedupe_key}`
- `matrix:rate:channel:{channel}`
- `matrix:session:user:{user_id}`
- `matrix:trace:{trace_id}`

用途：
- 去重
- 限流
- 短期上下文
- 幂等

---

## 5. 建模建议

### 5.1 第一版不必做的事
- 不必一开始就支持多租户
- 不必一开始就把所有策略 DSL 化
- 不必过早拆分过多微服务表

### 5.2 第一版必须保证的事
- sync 状态持久化
- event_id 唯一去重
- 通知发送审计
- 命令执行审计
- room/channel/command 可配置

---

## 6. 与现有 Bellkeeper 模型的关系

现有模型：
- `settings`
- `activity_logs`
- `llm_proxy_*`
- `dataset_*`

可以复用：
- `activity_logs` 的记录思路
- `settings` 的运行时配置模式

但 Matrix 平台不应继续塞进通用 `settings` 做硬编码键值堆积；应以独立表为主，`settings` 只放少量全局开关。
