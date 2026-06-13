# 全项目可靠性排查与加固计划

> 创建日期：2026-06-13
> 状态：待执行
> 触发事件：日报自 6-04 起静默失败，修复过程中发现系统性问题

---

## 一、排查结论总览

### 1.1 四类问题全部存在

| 类别 | 问题描述 | 严重程度 |
|------|---------|---------|
| A. n8n 工作流静默失败 | 表达式不求值、onError 吞错误、B01 webhook 404 | 🔴 严重 |
| B. Go 后台服务静默失败 | 无心跳、无外部存活检测 | 🟡 中等 |
| C. 契约一致性断裂 | RagFlow API 已删除但 K02/K06/K07/M03 仍在调；B01 webhook 不可用 | 🔴 严重 |
| D. 告警体系不可靠 | B02 ErrorTrigger 从未触发（errorWorkflow 未设置）；B01 webhook 404；Watchdog 缺日志 | 🔴 严重 |

### 1.2 根因链

```
B01 webhook 注册失败(404)
  → 所有走 B01 的告警分支全部死路
  → 加上 continueOnFail 吞错误，失败无人知晓
    → 加上 errorWorkflow 未设置，B02 ErrorTrigger 从不触发
      → 三个防线全部失效：节点容错× 全局错误监听× 告警出口×
        → 任何工作流失败 = 静默
```

---

## 二、关键发现（线上实证）

### 2.1 B01 webhook 不可用

- `POST http://localhost:5678/webhook/notify` → 404 "webhook not registered"
- deactivate/activate 不修复
- K01 的 `webhook/article-ingest` 正常（200），唯独 B01 不行
- 疑似缺少 `webhookId` 字段或 n8n 2.9.4 内部注册 bug
- **影响**：所有走 B01 发告警的工作流（B02、O01-health、K08、O03、O04、O05、M03 等）告警分支全部失效

### 2.2 B02 ErrorTrigger 从未触发

- 执行次数 = 0，但线上有 14 次 error 执行
- **根因**：所有工作流的 `settings.errorWorkflow` 均未设置
- n8n 2.9.x 中 ErrorTrigger 不会自动监听所有工作流，每个工作流必须显式指定 `errorWorkflow` 指向 B02 的 ID

### 2.3 Watchdog 缺少正常日志

- Watchdog 配置正确（`enabled: true`, `watchdog_enabled: true`）且已启动
- 但 `CheckOnce` 只在错误时打日志，正常检查完全静默
- 6-04 到 6-12 未生效是因为 bellkeeper 容器 6-12 才创建，之前无进程运行
- **修复**：每次检查输出日志 `[DailyReportWatchdog] checked: date=X, exists=Y, alert_sent=Z`

### 2.4 O03/O04/O05 API 全部 500

- `GET /api/system/containers` → 500（容器内无 docker socket）
- O04 的 `ContainerRestart` → 同样不可用
- O05 的 `BackupRun` 调用 `/home/ubuntu/SilkSpool/spool.sh`（容器内不存在）
- 这三个工作流设计时假设 bellkeeper 跑在宿主机上，容器化后完全失效

### 2.5 线上残留旧版工作流

- `K08-每日资讯摘要推送`（旧 8 节点版，inactive）仍在 n8n 数据库中
- `O02-每日摘要报告`（inactive）仍在
- 需要清理

### 2.6 本地 JSON vs 线上严重漂移

- K01/K02/K05/K06/M01/M02/M03/O02.1/O03/O04/O05 线上仍用 `continueOnFail=True`，本地 JSON 也是
- 只有 K08 和 O01-daily 已改为 `onError`
- 从未执行过 `push-all` 同步

---

## 三、工作流处置决定

### 3.1 退役（停用 + 归档）

| 工作流 | 退役理由 |
|--------|---------|
| B01-通知发送器 | webhook 404 不可修复，Go 通知 API 已替代 |
| K05-AI智能总结 | 无调用方，AI 总结已换实现 |
| K06-文档解析兜底 | 全依赖 RagFlow（已删除），B01 告警也挂了 |
| K07-Obsidian笔记同步 | 已 inactive，CouchDB 加密无法通过 Bellkeeper 同步 |
| M01-Matrix机器人基础 | Go 命令路由已替代 |
| M02-Memos待办管理 | Agent 工具已替代 |
| M03-知识问答处理器 | 全依赖 RagFlow（已删除），Agent 已替代 |
| O02-每日摘要报告 | 已退役（inactive） |
| O02.1-Todo同步 | 无实质功能，每 5 分钟空跑 |
| O03-磁盘空间告警 | docker socket 不可用，API 500 |
| O04-容器健康检查 | docker socket 不可用，自动重启危险 |
| O05-自动备份 | spool.sh 路径在容器内不存在 |

### 3.2 保留 + 修复

| 工作流 | 修复内容 |
|--------|---------|
| B02-工作流错误告警 | 告警节点改直调 `/api/matrix/notify`；callerPolicy 改 `"any"` |
| K01-文章智能入库 | 关键节点不加 onError（让 B02 兜底）；非关键节点加 `onError: continueErrorOutput` + 本地告警分支；`continueOnFail` → `onError` |
| K02-RSS定时采集 | 移除 RagFlow 节点（`/api/ragflow/documents/parse/smart`）；关键节点不加 onError；非关键节点加 onError + 本地告警；`continueOnFail` → `onError` |
| K03-手动批量提交 | 无需修改（无 continueOnFail） |
| K04-失败重试队列 | 无需修改（无 continueOnFail） |
| K08-每日资讯摘要 | 已改造，保持现状 |
| O01-每日日报 | 已改造，保持现状 |
| O01-服务健康监控 | 验证 `/api/health/detailed` 契约一致性（L3） |

### 3.3 最终保留 8 个活跃工作流

B02、K01、K02、K03、K04、K08、O01-daily、O01-health

---

## 四、K01/K2 onError 分类策略

### 4.1 关键服务节点（失败必须告警，不加 onError，让工作流 error → B02 兜底）

| 工作流 | 节点 | 理由 |
|--------|------|------|
| K01 | Check URL Exists | 入库去重，失败→重复入库 |
| K01 | Scrape URL | 内容提取，失败→无内容 |
| K01 | LLM Classify | 分类打标，失败→无标签 |
| K01 | File Ingest | 文件入库，失败→文章丢失 |
| K02 | 拉取RSS Feed | 采集入口，失败→无文章 |
| K02 | 推送文章到Ingest | 入库链路，失败→文章不入库 |
| O01-daily | 调用日报生成 | 日报核心 |
| O01-health | 获取服务状态 | 健康监控核心 |

### 4.2 非关键节点（失败可继续，加 `onError: continueErrorOutput` + 本地告警分支调 `/api/matrix/notify`）

| 工作流 | 节点 | 理由 |
|--------|------|------|
| K01 | Match Tags | 有默认标签兜底 |
| K02 | 发送采集通知 | 通知不影响主链路 |
| K02 | 写入采集报告 | 报告不影响主链路 |
| K02 | 上报采集详情 | 统计不影响主链路 |
| K08 | 推送Matrix通知 | 通知失败不影响生成 |
| O01-daily | 记录成功日志 | 日志不影响生成 |

---

## 五、Go 侧修复

### 5.1 Watchdog 日志增强

文件：`internal/service/daily_report_watchdog.go`

`CheckOnce` 方法中每次检查输出日志：
```
[DailyReportWatchdog] checked: date=2026-06-13, exists=true
[DailyReportWatchdog] checked: date=2026-06-13, exists=false, alert_sent=true
```

### 5.2 关键服务心跳

给 RSSFetcherService、CrawlQueueService、LLMJobQueueService 加心跳：
- 每 5 分钟写一条 `activity_logs`（`module=heartbeat, action=服务名, status=success`）
- 超过 15 分钟无心跳视为异常
- 外部可通过 `SELECT * FROM activity_logs WHERE module='heartbeat' AND action='rss_fetcher' ORDER BY created_at DESC LIMIT 1` 判断存活

---

## 六、n8n 全局配置修复

### 6.1 errorWorkflow 设置

所有保留的工作流在 `settings` 中加 `"errorWorkflow": "DDPUVypFAgyDOXgn"`（B02 的 n8n ID）。

涉及：K01、K02、K03、K04、K08、O01-daily、O01-health。

### 6.2 B02 告警出口修复

B02 的「发送错误告警」节点从调 B01 webhook 改为直调 Bellkeeper 通知 API：
- 当前：`POST http://localhost:5678/webhook/notify`
- 改为：`POST ={{ $env.BELLKEEPER_URL }}/api/matrix/notify`
- Body 改为 Bellkeeper 通知 API 格式：`{ "channel": "alerts", "message": "...", "message_type": "markdown" }`

### 6.3 B02 callerPolicy 改为 `"any"`

确保不限制哪些工作流的错误能触发 B02。

---

## 七、契约一致性验证（L3）

以下被 n8n 调用的 Bellkeeper API 端点需要端到端验证响应结构：

| 工作流 | 端点 | 验证项 |
|--------|------|--------|
| K01 | `POST /api/datasets/check-url` | 响应包装层 `{"data":...}` 解析 |
| K01 | `POST /api/classify/article` | 同上 |
| K01 | `POST /api/tags/match` | 同上 |
| K01 | `POST /api/files/ingest/url` | 同上 |
| K02 | `GET /api/rss?is_active=true&per_page=100` | 响应结构 |
| K02 | `POST /api/logs` | 响应结构 |
| K02 | `POST /api/reports/write` | 响应结构 |
| K05 | `POST /api/llm/v1/chat/completions` | 已退役但需确认 K01/K02 不依赖 |
| O01-health | `GET /api/health/detailed` | `{"data": {"status":..., "services":...}}` 结构 |

验证方法：`spool exec keeper "docker exec sp-bellkeeper wget -qO- 'http://localhost:8080/api/...' "` 对比工作流 Code 节点中的解析逻辑。

---

## 八、分期实施

### Tier 1：Go 侧修复（Watchdog 日志 + 心跳）

1. `daily_report_watchdog.go`：`CheckOnce` 加正常检查日志
2. `rss_fetcher.go`：每 5 分钟写 heartbeat activity_log
3. `crawl_queue.go`：同上
4. `llm_job_queue.go`：同上
5. 新增 `activity_logs` 心跳写入方法（复用现有 `ActivityLogService`）

验收：`go build ./...` + `go vet ./...` + `go test ./internal/...` 全绿

### Tier 2：n8n 工作流退役 + 清理

1. 以下工作流 JSON 移入 `internal/n8n_workflows/archive/`：
   B01、K05、K06、M01、M02、M03、O02、O02.1、O03、O04、O05
2. 线上 n8n 停用这些工作流（deactivate）
3. 清理线上残留的旧版 K08（`K08-每日资讯摘要推送`）
4. `spool n8n list` 确认只剩 8 个活跃工作流

### Tier 3：K01/K02 工作流修复

1. K02：移除 RagFlow 节点（`智能解析文档` 及其关联的 `按Dataset分组`/`有文档需解析?` 分支）
2. K01/K02：关键节点移除 `continueOnFail`（让 B02 兜底）
3. K01/K2：非关键节点 `continueOnFail` → `onError: continueErrorOutput` + 本地告警分支（调 `/api/matrix/notify`）
4. 所有保留工作流加 `settings.errorWorkflow`
5. B02 告警出口改为直调 `/api/matrix/notify`，callerPolicy 改 `"any"`
6. `executeOnce: true` 加到所有 HTTP 节点

### Tier 4：推送 + 端到端验证

1. `git push` → `spool bundle keeper service keeper bellkeeper up`
2. `POST /api/workflows/definitions/push-all`
3. L3 契约验证（第七节清单）
4. 故意触发一次 B02：手动让某个工作流报错，确认 Matrix alerts 房间收到告警
5. 验证 Watchdog 日志：`spool logs keeper bellkeeper 100 | grep Watchdog`
6. 验证心跳：查 `activity_logs WHERE module='heartbeat'`

---

## 九、风险与回滚

| 风险 | 缓解 |
|------|------|
| K01 关键节点不加 onError 后，单次失败中断整个入库流程 | B02 会立即告警，人工介入；K04 失败重试队列兜底 |
| K02 移除 RagFlow 后文档无 LLM 增强解析 | 降级为纯 Trafilatura 提取，后续可加回 |
| B02 告警改直调后 B01 webhook 不再被任何工作流依赖 | B01 退役，无影响 |
| 回滚 | 所有 JSON 在 git，`git revert` + `push-all` 即回旧版；Go 改动独立，回滚无连带 |

---

## 十、排查证据标准

| 类别 | 证据等级 | 方法 |
|------|---------|------|
| A 类（n8n 工作流） | L2 | `spool n8n list` + 对比本地 JSON + 检查 execution 状态 |
| B 类（Go 后台服务） | L2 | `spool logs` 确认 ticker 在跑 + 心跳 activity_log |
| C 类（契约一致性） | L3 | 实际调 API，断言响应结构 |
| D 类（告警可靠性） | L3 | 故意触发告警，确认 Matrix 收到 |
