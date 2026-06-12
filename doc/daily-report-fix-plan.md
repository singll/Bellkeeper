# 日报工作流修复计划

> 创建日期：2026-06-11
> 状态：已执行（仅解决执行层；数据层问题与系统性重构见 `doc/notification-monitoring-overhaul-plan.md`）

## 一、问题概述

keeper 上的日报（K08/O01/O02）全部无法正常生成。根因是**本地工作流定义未同步到 n8n 运行时**，导致 n8n 上运行的工作流缺少关键配置（`continueOnFail`），加上并行结构设计问题、svcStatus 函数传递 bug 和 Memos API filter 语法不兼容，三个日报工作流全部执行失败。

## 二、诊断结果

### 2.1 工作流状态

| 工作流 | n8n ID | Active | 最后执行 | 结果 |
|--------|--------|--------|---------|------|
| K08-每日资讯摘要推送 | `RQniufpXO8VmOxET` | **Inactive** | 从未触发 | 未启动 |
| O01-每日日报 | `gizvPgFKh2MXuzk4` | Active | 6/10 21:00 | **error** |
| O02-每日摘要报告 | `vjEJW9zuhk4oWwS6` | Active | 6/10 21:00 | **error** |

### 2.2 本地定义 vs n8n 运行时差异

#### 差异 1：`continueOnFail` 缺失（最关键）

| 工作流 | 本地定义 | n8n 运行时 |
|--------|---------|-----------|
| O01（11 个 HTTP 节点） | 全部 `continueOnFail: true` | 全部缺失 |
| O02（14 个 HTTP 节点） | 全部 `continueOnFail: true` | 全部缺失 |
| K08（4 个 HTTP 节点） | 全部 `continueOnFail: true` | 全部缺失 |

- n8n 2.9.4 已弃用 `continueOnFail`，改用 `onError` 字段
- `continueOnFail: true` 等价于 `onError: "continueRegularOutput"`
- 修复时直接使用 `onError` 以兼容新版

#### 差异 2：K08 处于 Inactive

本地 `K08-daily-digest.json` 中 `"active": false`，n8n 中也是 Inactive。K08 应激活。

#### 差异 3：Memos API filter 语法不兼容

- 当前 O01：`filter=tag%3D%3D%27%E5%BE%85%E5%8A%9E%27`（`tag=='待办'`）→ **400 Bad Request**
- 当前 O02：`filter=type+%3D%3D+%22MEMO%22+%26%26+tag+%3D%3D+%22%E5%BE%85%E5%8A%9E%22`（`type=="MEMO" && tag=="待办"`）→ **400 Bad Request**
- `type` 是 CEL 保留关键字，与字符串比较类型不匹配
- 正确语法：`filter=%22%E5%BE%85%E5%8A%9E%22+in+tags`（`"待办" in tags`）→ **200 OK**（已验证）

### 2.3 并行结构问题（架构缺陷）

O01 的连接模式：

```
Trigger → [7个并行HTTP] → 整理数据(Code)
```

n8n 的行为：**每个上游分支完成时独立触发下游节点**，不会等待所有分支完成。执行记录证实：只有「获取今日采集统计」完成时，「整理数据」就被触发，而「获取服务状态」等节点还没执行。

Code 节点中 `$('获取服务状态').first().json` 引用不存在的节点数据 → 崩溃。

O02 有类似的并行→合并结构，同样受此问题影响。

### 2.4 O01 svcStatus 函数传递 bug（评审发现）

O01 的「整理数据」Code 节点将 `svcStatus`（一个 JS 函数）放入返回的 json 对象，「构建统一Markdown」节点通过 `data.svcStatus` 取出后调用 `svcStatus('bellkeeper')`。

n8n 节点间数据按 JSON 序列化传递（2.9.x 的 Code 节点跑在 task runner 进程里，必经 IPC 序列化），函数会被丢掉，下游调用时抛 `"svcStatus is not a function"`。

此前此 bug 从未暴露，是因为工作流在更早的「整理数据」就崩了。修好并行问题后它就是下一个必然崩溃点。

**修法**：「整理数据」只传 `healthData` 数据（去掉 svcStatus），「构建统一Markdown」本地重建 svcStatus 函数——参考 O02 的「构建Markdown摘要」写法（`unwrap('获取服务状态')` + 节点内定义 `svc()`）。

### 2.5 `/api/reports/write` 端点

O01/O02/K08 都调用 `$env.BELLKEEPER_URL/api/reports/write` 写入日报文件。Bellkeeper 代码中 `ReportService.WriteMessage()` 已实现（含增量合并逻辑），对应 handler 为 `POST /api/reports/write`。**无需额外修复**。

### 2.6 问题链

```
工作流未同步（本地 vs n8n）
  → continueOnFail 缺失
    → 并行 HTTP 请求失败时分支中断
  → 并行→合并结构设计缺陷
    → Code 节点在数据不完整时被触发
  → svcStatus 函数传递 bug（O01）
    → 修好并行后仍会在构建Markdown节点崩溃
  → Memos filter 语法不兼容
    → 获取待办事项请求 400
  → K08 从未激活
    → 无 20:00 资讯摘要
  → 三个工作流全部失败 → 日报无法生成
```

## 三、修复方案

### 3.1 修改文件清单

| 文件 | 改动 |
|------|------|
| `internal/n8n_workflows/O01-daily-report.json` | 串行化连接 + continueOnFail→onError + Memos filter + svcStatus修复 + executeOnce |
| `internal/n8n_workflows/O02-daily-summary.json` | 串行化连接 + continueOnFail→onError + Memos filter + executeOnce |
| `internal/n8n_workflows/K08-daily-digest.json` | continueOnFail→onError + 激活 + executeOnce |

### 3.2 O01-每日日报

#### 3.2.1 `continueOnFail` → `onError`

所有节点的 `"continueOnFail": true` 替换为 `"onError": "continueRegularOutput"`。

涉及节点：获取今日采集统计、获取RSS采集详情、获取服务状态、获取文章总数、获取KB入库日志、获取PKB日报数据、获取待办事项、调用LLM总结、写入日报文件、发送Matrix通知、记录活动日志。

#### 3.2.2 并行结构改为串行链式连接

**当前（有竞合问题）：**

```
Trigger ──┬→ 获取今日采集统计 ──→ 整理数据
          ├→ 获取RSS采集详情  ──→ 整理数据
          ├→ 获取服务状态     ──→ 整理数据
          ├→ 获取文章总数     ──→ 整理数据
          ├→ 获取KB入库日志   ──→ 整理数据
          ├→ 获取待办事项     ──→ 整理数据
          └→ 获取PKB日报数据  ──→ 整理数据
```

**修改后（串行链式）：**

```
Trigger → 获取今日采集统计 → 获取RSS采集详情 → 获取服务状态 → 获取文章总数 → 获取KB入库日志 → 获取PKB日报数据 → 获取待办事项 → 整理数据
```

性能影响：7 个 HTTP 请求串行执行约 100-200ms（单个请求 10-30ms），可接受。

#### 3.2.3 修复 svcStatus 函数传递 bug

- 「整理数据」：从返回对象中移除 `svcStatus`，改为传递 `healthData`（原始数据）
- 「构建统一Markdown」：从 `data.healthData` 取出健康数据，本地重建 `svcStatus` 函数

#### 3.2.4 修复 Memos filter

- 当前 URL：`http://memos:5230/api/v1/memos?filter=tag%3D%3D%27%E5%BE%85%E5%8A%9E%27&pageSize=100`
- 修改为：`http://memos:5230/api/v1/memos?filter=%22%E5%BE%85%E5%8A%9E%22+in+tags&pageSize=100`

#### 3.2.5 HTTP 节点加 `executeOnce: true`

串行链中每个 HTTP 节点添加 `"executeOnce": true`，防止端点返回数组时 item 膨胀导致下游重复执行。

### 3.3 O02-每日摘要报告

#### 3.3.1 `continueOnFail` → `onError`

同 O01，全部替换。

#### 3.3.2 并行结构改为串行链式连接

**当前（混合并行+串行，有竞合问题）：**

```
Trigger ──┬→ 获取今日采集统计
          ├→ 获取RSS采集详情
          ├→ 获取服务状态 → 获取文章总数 → 获取待办事项 ─┐
          ├→ 获取爬取队列统计                              │
          ├→ 获取今日失败任务                              ├→ 构建Markdown摘要
          ├→ 获取今日阻塞任务                              │
          ├→ 获取爬取Worker状态                            │
          ├→ 获取LLM代理健康                               │
          ├→ 获取知识库索引统计                             │
          └→ 获取PKB日报数据  ─────────────────────────────┘
```

**修改后（串行链式）：**

```
Trigger → 获取今日采集统计 → 获取RSS采集详情 → 获取服务状态 → 获取文章总数 → 获取待办事项 → 获取爬取队列统计 → 获取今日失败任务 → 获取今日阻塞任务 → 获取爬取Worker状态 → 获取LLM代理健康 → 获取知识库索引统计 → 获取PKB日报数据 → 构建Markdown摘要
```

O02 的「构建Markdown摘要」Code 节点全部通过 `$('节点名')` 引用数据且有 try/catch 兜底，串行后引用依然成立，**无需改动 Code 代码**。

#### 3.3.3 修复 Memos filter

- 当前 URL：`http://memos:5230/api/v1/memos?filter=type+%3D%3D+%22MEMO%22+%26%26+tag+%3D%3D+%22%E5%BE%85%E5%8A%9E%22&pageSize=100`
- 修改为：`http://memos:5230/api/v1/memos?filter=%22%E5%BE%85%E5%8A%9E%22+in+tags&pageSize=100`

#### 3.3.4 HTTP 节点加 `executeOnce: true`

同 O01。

### 3.4 K08-每日资讯摘要推送

#### 3.4.1 `continueOnFail` → `onError`

涉及节点：获取今日入库日志、调用K05总结、写入日报文件、发送Matrix通知。

#### 3.4.2 激活工作流

将 `"active": false` 改为 `"active": true`。

K08 本身结构是串行的（Trigger → 获取入库日志 → 整理数据 → ...），无需调整连接。

#### 3.4.3 HTTP 节点加 `executeOnce: true`

同 O01。

## 四、推送与验证

### 4.1 推送步骤

> **关键**：bellkeeper 容器的工作流定义是构建时 `COPY` 进镜像的（Dockerfile:65），`spool sync push` 只更新 n8n 容器挂载目录，不影响 bellkeeper 容器。必须重建镜像才能让 `push-all` API 读到新定义。

```bash
# 1. 本地 git commit + push 到远程仓库
git add internal/n8n_workflows/
git commit -m "fix(n8n): 日报工作流串行化 + onError + Memos filter + svcStatus修复"
git push

# 2. 重建 bellkeeper 镜像并启动（自动 git pull + build + up）
spool bundle keeper service keeper bellkeeper up

# 3. 通过 Bellkeeper API 推送工作流定义到 n8n
#    在 keeper 主机上执行
curl -X POST 'http://localhost:8080/api/workflows/definitions/push-all' \
  -H 'X-API-Key: <BELLKEEPER_API_KEY>'

# 4. 激活 K08（push-all 不改变 active 状态）
spool n8n activate K08-每日资讯摘要推送
```

### 4.2 验证步骤

```bash
# 1. 检查 n8n 中工作流状态
spool n8n list

# 2. 手动触发 O01 测试（通过 n8n UI 或 API 手动执行）

# 3. 检查日报文件是否生成
spool exec keeper "ls -la /mnt/NAS/data/knowledge/vault/daily/"

# 4. 查看日报内容
spool exec keeper "cat /mnt/NAS/data/knowledge/vault/daily/2026-06-11.md"

# 5. 检查 n8n 执行历史
spool logs keeper n8n 100 | grep -i "execut"
```

### 4.3 回滚方案

如果修复后仍然失败：
1. `git revert` 回退本地 JSON 文件到修复前的版本
2. 重新 `spool bundle keeper service keeper bellkeeper up` + `push-all`
3. 不依赖 n8n UI 的工作流历史版本恢复（社区版限制，不可靠）

## 五、注意事项

1. **`prepareWorkflowPushPayload` 会删除 `active` 字段**：推送时不改变工作流的激活状态。K08 需要在推送后通过 `spool n8n activate` 激活。
2. **n8n 2.9.4 的 `onError` 字段**：推送时 n8n API 应接受 `onError: "continueRegularOutput"`，需验证推送后 n8n 是否正确存储了该字段。
3. **Memos 数据为空**：当前 Memos 没有任何 memo 数据，所以「获取待办事项」即使 filter 语法正确也会返回空列表。Code 节点的 `|| []` 兜底已处理此场景。
4. **串行化后 Code 节点数据引用**：串行链式连接中，Code 节点可以通过 `$('节点名').first().json` 引用任何已执行的上游节点数据，且保证数据已就绪。
5. **日报增量合并**：三个工作流都写入 `vault/daily/{date}.md`，`ReportService.WriteMessage()` 的 `mergeMarkdown` 逻辑保证同日多次写入不覆盖（按 `####` 章节增量合并）。K08（20:00）先写入资讯摘要，O01/O02（21:00）后追加其他章节。
6. **`executeOnce: true` 护栏**：防止端点返回数组时 item 膨胀导致串行链下游重复执行。当前所有端点顶层返回对象，不会触发此问题，但作为回归防护加上。

## 六、后续优化方向

**后端聚合端点**：在 Bellkeeper 加一个聚合端点（如 `/api/reports/daily-data`），后端一次性并行取齐所有数据，n8n 只发一个请求。编排逻辑从"不可测试的 JSON 连线"移进可单测的 Go 代码，O01/O02 的十几个 HTTP 节点缩成一个。这是长期更优方案，可待串行化稳定后实施。
