# 通知模型

> 本文档定义 Matrix Notification Plane 的事件模型、频道模型、模板模型与路由原则。

---

## 1. 核心目标

通知系统必须支持：
- 多系统统一入口
- 逻辑频道路由
- 文本与 HTML 双格式
- 幂等去重
- 重试与死信
- 降噪与聚合
- 审计追踪

---

## 2. 通知事件模型

建议请求体标准如下：

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

字段说明：
- `channel`：逻辑频道，必填
- `title`：可选标题
- `message`：纯文本内容
- `html`：富文本内容
- `severity`：`info` / `warning` / `error` / `critical`
- `source`：来源系统
- `dedupe_key`：幂等与降噪关键字段
- `metadata`：结构化扩展字段

---

## 3. 逻辑频道模型

建议预置频道：

| 频道 | 用途 |
|------|------|
| `alerts` | 通用告警 |
| `daily` | 日报、摘要、例行输出 |
| `todo` | 待办系统输出 |
| `qa` | 知识问答结果 |
| `ops` | 运维任务结果 |
| `infra` | 基础设施状态 |
| `security` | 安全相关告警或事件 |

原则：
- 调用方关心频道，不关心具体房间
- 房间变更由平台治理层处理

---

## 4. 路由模型

`channel -> bindings -> room`

支持：
- 一个频道多个房间
- primary / fallback / broadcast 模式
- 频道级默认严重性与模板

示例：
- `alerts` → `admin-room` (primary), `infra-room` (fallback)
- `daily` → `daily-room` (broadcast)

---

## 5. 模板模型

通知模板应分离为：
- 纯文本模板
- HTML 模板

模板可以按以下维度选择：
- channel
- severity
- source
- event type

示例：
- `alerts.warning.default`
- `ops.success.backup`
- `daily.digest.summary`

---

## 6. 幂等与去重

### 6.1 幂等键

优先使用调用方提供的 `dedupe_key`。

示例：
- `disk-knowledge-root`
- `backup-keeper-2026-04-08`

### 6.2 默认 dedupe 策略

如果调用方未提供，则由平台按：
- `channel + source + title + normalized_message`
生成默认 key。

---

## 7. 重试模型

建议：
- 首次失败后指数退避
- 达到阈值后进入死信
- 可由 Admin API 重放

---

## 8. 聚合与降噪

应支持：
- 同一 dedupe key 在窗口期内合并
- 同类告警计数累加
- 夜间静默/仅升级推送

这部分优先在规则层配置，不写死在 handler 里。

---

## 9. 审计要求

每条通知都要能追溯：
- 谁发起
- 来自哪个系统
- 路由到哪些房间
- 成功还是失败
- 失败原因是什么

---

## 10. 建议首版范围

首版至少支持：
- 单频道单房间
- text/html 双格式
- dedupe key
- 基础重试
- 发送审计

聚合、静默、升级策略可放到第二阶段增强。
