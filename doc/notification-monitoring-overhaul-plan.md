# 通知/监控模块系统性重构计划

> 创建日期：2026-06-11
> 状态：已完成（commit 595e70d，线上验证通过）
> 前置文档：`doc/daily-report-fix-plan.md`（2026-06-11 已执行，解决的是「工作流跑不起来」；本计划解决「跑起来了但内容全错」+ 架构性根治）

---

## 一、问题报告（用户 2026-06-11 手动触发 O01 的实际输出）

1. **服务状态全 ❌**：Bellkeeper / RagFlow / Firecrawl 三行全部 ❌，但服务实际正常。
2. **监控的服务过时**：RagFlow、Firecrawl 已不是当前架构的核心服务，真正在跑的服务没有被监控。
3. **数据完全对不上**：
   - 「今日活动概览」显示*无活动统计数据*——实际今日 activity_logs 有 8 万+ 条；
   - 「成功入库 1 篇」——实际今日爬取成功 253 篇、PKB 新卡片 6 张；
   - 「知识库入库统计」恒为空；
   - 「AI 亮点总结」恒为兜底文案。

## 二、诊断结论（全部经线上实测验证，2026-06-11）

### 2.1 根因 A：响应包装层 `{"data": ...}` 未剥离 → 服务状态全 ❌、活动概览恒空

Bellkeeper 所有 API 经 `internal/pkg/response` 统一包装为 `{"data": <payload>}`。

实测 `GET /api/health/detailed` 返回：

```json
{"data":{"status":"healthy","services":{"meilisearch":{"status":"up"},"n8n":{"status":"up"},"rss_fetcher":{"status":"up"}},"metrics":{...}}}
```

而 O01「构建统一Markdown」节点取的是 `healthData.services`（缺 `.data` 一层）→ 永远 `undefined`；兜底判断 `healthData.status === 'healthy'` 同样取不到 → 三个服务全部 ❌。

同理「整理数据」节点取 `statsData.total / statsData.by_module`，而 `/api/logs/stats` 实际返回 `{"data":[{"module":"rss_fetch","count":20099},...]}`——**既有包装层问题，又有结构假设错误**（期望对象，实际是数组，且无 `total`/`by_module` 字段）→「今日活动概览」恒空。

> 注：「获取RSS采集详情」碰巧写对了（`.json.data?.items`），所以 RSS 部分有数字——这暴露了同一工作流里每个 Code 节点各自手写解析、对错全凭运气的现状。

### 2.2 根因 B：监控的服务清单过时

| 工作流监控的 | 实际状态 |
|--------------|----------|
| Bellkeeper | 在跑，但 `/api/health/detailed` 的 `services` 里**没有**这个 key（接口由它自己提供，能响应即活着） |
| RagFlow | **已废弃**——Go 代码中 `ragflow` 零引用，入库链路已迁移到 Meilisearch + PKB |
| Firecrawl | 仅是抽取器兜底层（`FirecrawlConfig.FallbackOnly`），不是独立监控对象，且不在 keeper 主机部署 |

而 keeper 上实际在跑、值得监控的服务（`spool service keeper status` 实测）：
`bellkeeper`、`bellkeeper-db(postgres)`、`n8n`、`meilisearch`、`memos`、`rsshub`、`couchdb`、`nats`、`redis`。

后端 `HealthService.Detailed()`（`internal/service/health.go:58`）目前只检查 n8n、meilisearch、rss_fetcher 三项，且**清单硬编码**，无法扩展。

### 2.3 根因 C：统计语义错误——「用最近 50 条日志采样冒充全天计数」

O01 的「今日内容采集」逻辑：拉 `/api/logs?module=rss_fetch&since=今天&limit=50`，数其中 `status==='success'` 的条数当作「成功入库篇数」。三处错误叠加：

1. **采样当计数**：今日 rss_fetch 日志实测 20,099 条，limit=50 只是最近 50 条，数出来的数字毫无意义；
2. **状态值拼错**：后端写入的失败状态是 `failure`（`rss_fetcher.go:375`），工作流过滤的是 `failed` → 失败数恒 0；
3. **动作不区分**：rss_fetch 模块下有 `fetch`/`enqueue`/`ingest`/`health`/`recovery` 等多种 action，真正代表「文章入库」的是 `ingest`，工作流不分 action 全数。

「知识库入库统计」更是查询已死数据源：`module=ragflow_upload&action=upload_with_routing`——该模块今日 0 条（历史遗留 2052 条），且 Code 节点里还硬编码着 5 个 RagFlow dataset ID 的中文名映射。**K08-每日资讯摘要 用的是同一个死数据源，所以 K08 也恒为空**（连带「AI 亮点总结」恒为兜底文案，因为 aiPrompt 由 KB 数据构建）。

实际今日的真实数据（`/api/dashboard/stats` 实测）：爬取新增 6631 / 成功 253，PKB 新卡片 6 张，LLM 24h 调用 531 次 / 156.9 万 token——日报却说「成功入库 1 篇、无活动」。

### 2.4 根因 D（架构性，前三者反复发生的总根源）：数据加工逻辑长在 n8n Code 节点里

- 统计口径、响应结构解析、服务清单、dataset 映射全部以 **JS 字符串**形式埋在工作流 JSON 里：无类型检查、无单元测试、grep 不到、code review 覆盖不到；
- 后端每次演进（响应包装、模块改名、RagFlow 下线、dashboard 重构）n8n 侧都**静默坏掉**——不报错，只是输出错误数字；
- 同样的数据获取逻辑在 O01 / O02 / K08 三处重复实现，口径互相漂移；
- `onError: continueRegularOutput` 让 HTTP 失败静默吞掉，日报把「取数失败」呈现为「今日无数据」，**错误被伪装成事实**。

这正是 6 月内第二次修日报却仍然全错的原因：上一轮修的是执行层（串行化/onError/filter），数据层的腐烂没有动。**只要加工逻辑还在 n8n 里，第三次腐烂是必然的。**

## 三、修复原则（「健壮、易用、可扩展」的具体定义）

| 原则 | 落地手段 |
|------|---------|
| **健壮** | 统计口径全部收进 Go Service 层，带单元测试；数据获取失败必须在日报中显式标注「⚠️ 获取失败」，禁止伪装成 0 或「无数据」 |
| **易用** | n8n 工作流退化为「定时触发 → 调一个后端端点」；手动触发 = 一条 curl；新人排错只看 Go 代码 |
| **可扩展** | 健康检查服务清单配置化（YAML 列表）；日报新增一个统计板块 = 后端加一个 collector 函数 + 单测，不碰 n8n |
| **单一事实源** | 日报、Dashboard 主页、未来周报共用同一套 Service 层统计函数，杜绝口径漂移 |

## 四、目标架构

### 4.1 总览

```
【现状】n8n O01: 触发 → 7个HTTP节点 → 3个Code节点(几百行JS) → 写文件 → Matrix通知
【目标】n8n O01: 触发 → POST /api/reports/daily/generate → (失败时告警)

后端 DailyReportService.Generate(date):
  收集(并行,errgroup) → 渲染Markdown → LLM亮点总结 → 写vault文件 → Matrix推送 → 返回结果
   ├─ 服务健康   ← HealthService(配置化清单)
   ├─ 爬取统计   ← DashboardService.fillCrawl(复用,今日新增/成功/失败/feed状态)
   ├─ 入库统计   ← 新增 ActivityLog 聚合查询(module+action+status 分组计数)
   ├─ PKB 统计   ← PKBReportService(VaultStats + daily cards)
   ├─ LLM 统计   ← DashboardService.fillLLM(复用)
   ├─ 失败明细   ← rss_fetch/crawl 今日 failure 日志(限10条,带原因)
   └─ 待办数量   ← Memos client(后端已有集成,internal/matrix/command/handler_memos.go)
```

每个 collector 独立容错：单项失败记入 `report.Errors[]`，日报对应章节渲染「⚠️ 数据获取失败: <原因>」，其余章节正常输出。

### 4.2 新增/修改的后端组件

| 组件 | 文件 | 说明 |
|------|------|------|
| `DailyReportService` | `internal/service/daily_report.go`（新） | 核心编排：Collect → Render → Summarize → Persist → Notify；每步可独立调用便于测试 |
| 聚合查询 | `internal/repository/activity_log.go` | 新增 `GetActionStats(module string, since time.Time) ([]ActionStat, error)`：`SELECT action, status, COUNT(*) GROUP BY action, status`——一次查询取代「拉50条日志数数」 |
| 健康检查配置化 | `internal/service/health.go` + `internal/config/config.go` | `HealthConfig.Services []ServiceProbe{Name, URL, Type(http/tcp), Timeout}`；硬编码三项迁入默认配置；新增 memos/rsshub/couchdb/nats/redis/postgres 探测 |
| 渲染器 | `internal/service/daily_report_render.go`（新） | 纯函数 `RenderDailyReport(data *DailyReportData) string`——输入结构体输出 Markdown，最易单测的一层 |
| LLM 总结 | `DailyReportService` 内部 | 直接调用进程内 `LLMProxyService`（不走 HTTP 回环），模型沿用 `pool-summary`，失败降级为「(AI总结暂不可用)」并记入 Errors。⚠️ **偏离**：实际走 `localhost` HTTP 回环（`llmclient.New`），复用其重试/超时/日志逻辑；见 `internal/service/service.go` DEVIATION 注释 |
| Handler + 路由 | `internal/handler/report.go` + `internal/router/router.go` | `GET /api/reports/daily-data?date=`（返回结构化 JSON，供调试/前端复用）；`POST /api/reports/daily/generate`（一条龙，body 可选 `{date, dry_run, skip_notify}`） |

写文件复用 `ReportService.WriteMessage`（增量合并已实现），Matrix 推送复用现有 notify service——**不新造轮子**。

### 4.3 数据口径定义表（单一事实源，写进代码注释与本表）

| 日报指标 | 口径 | 来源 |
|----------|------|------|
| 服务状态 | 配置清单中每个服务的探活结果（up/down/degraded + 延迟） | HealthService（配置化后） |
| 今日爬取 | crawl_jobs 今日 created / success / failed（自然日，Local 时区） | `crawlJobRepo.ActivitySince(dayStart)`（dashboard 已用） |
| 今日 RSS 入库 | activity_logs `module=rss_fetch, action=ingest` 按 status 分组计数（success / duplicate / failure） | 新 `GetActionStats` |
| 今日文件入库 | activity_logs `module=file_ingestion` 按 status 分组计数 | 新 `GetActionStats` |
| 今日分类 | activity_logs `module=classify` success 计数 | 新 `GetActionStats` |
| PKB | vault 总卡片数、今日新卡片数（含标题列表 top10）、知识树数 | `PKBReportService` |
| LLM | 24h 请求数 / 错误数 / token / 费用（分） | `DashboardService.fillLLM` |
| 失败明细 | rss_fetch + crawl 今日 `failure` 最近 10 条（summary + ref） | activity_logs 查询 |
| 待办 | Memos `"待办" in tags` 计数 | Memos client ⏭️ **未实现**：Memos collector 暂未接入，日报渲染「⏭️ 暂未接入」占位；待实现 Memos 集成后补上 |
| AI 亮点 | 基于「今日新 PKB 卡片标题 + 今日爬取成功文章标题(top30)」生成——替代已死的 RagFlow dataset 分组 | LLMProxyService |

### 4.4 n8n 侧目标形态

| 工作流 | 处置 | 理由 |
|--------|------|------|
| **O01-每日日报** | 改造为 3 节点：定时触发(21:00) → `POST /api/reports/daily/generate` → IF 失败则发 Matrix 告警 | 数据加工全部移入后端 |
| **O02-每日摘要报告** | **退役（停用并归档）**，其独有板块（爬取队列明细/Worker/LLM健康/知识库索引）并入后端日报的「运维」章节 | 与 O01 同时段(21:00)、数据高度重叠、维护两份必然漂移；归档而非删除，保留回滚能力 |
| **K08-每日资讯摘要** | **退役（停用并归档）**，「资讯摘要」即日报的 AI 亮点章节，数据源（ragflow_upload）已死无可修 | 同上 |
| **O01-服务健康监控**（5分钟级告警） | 保留独立工作流（告警与日报职责不同），但改为调 `GET /api/health/detailed` 后只判断顶层 `data.status != "healthy"` 即告警，服务明细由后端给出；移除 Firecrawl 直连探测 | 告警逻辑简单化，服务清单只在后端维护一份 |
| **O03-磁盘告警 / O04-容器健康 / O05-自动备份** | 本期不动（依赖的 `/api/system/*` 端点存在且独立） | 控制范围 |
| **B01-通知发送器 / B02-错误告警** | 保留 | 通知出口，工作正常 |

## 五、分期实施（每期一次提交，全程绿色构建）

### Tier 1：后端数据层（聚合查询 + 数据收集）

1. `repository/activity_log.go` 新增 `ActionStat{Action, Status string; Count int64}` 与 `GetActionStats(module, since)`（GORM 查询构建器，禁止拼 SQL）；
2. `service/daily_report.go` 新增 `DailyReportService`（构造函数注入 health/dashboard/pkbReport/activityLog/memos 等依赖）与 `Collect(date) (*DailyReportData, error)`；
3. `DailyReportData` 含 `Errors []CollectError`，每个 collector 失败不阻断整体；collectors 用 errgroup 并行 + 各自 timeout；
4. **单测**：`daily_report_test.go`——用 sqlite 内存库灌活动日志，断言 `GetActionStats` 与 `Collect` 的真实返回（禁止假测试）。⏭️ **跳过**：repository 整包无测试基础设施（无内存 DB fixture），数据层零测试覆盖；渲染器和健康检查已有真测试，但 GetActionStats/GetRecentFailures/Collect/GenerateBrief 无回归保护。后续需补建测试 fixture 后再补 |

验收：`go build ./...` / `go vet ./...` / `go test ./internal/...` 全绿。

### Tier 2：后端渲染 + 一条龙生成

1. `daily_report_render.go`：`RenderDailyReport(data)` 纯函数，章节包括：服务状态 / 今日采集 / 入库统计 / PKB / LLM / AI亮点 / 失败明细 / 待办；数据缺失章节渲染「⚠️ <来源>获取失败」；
2. `Generate(date, opts)`：Collect → Render → LLM 总结（进程内调用，120s 超时，失败降级）→ `ReportService.WriteMessage` → Matrix 推送；`dry_run` 跳过写入与推送；
3. Handler：`GET /api/reports/daily-data`、`POST /api/reports/daily/generate`，统一 `response` 包；路由注册 + `router.go` 接线（**同提交内接入真实调用方，杜绝死代码**）；
4. **单测**：渲染器表驱动测试（满数据 / 部分失败 / 全空三种输入断言输出 Markdown 片段）。

### Tier 3：健康检查配置化 + 服务清单扩充

1. `config.go` 新增 `HealthConfig{Services []ServiceProbe}`，支持 `${ENV}` 展开；默认值在 `pkg/defaults` 给出 keeper 全家桶清单（bellkeeper-db/redis/memos/rsshub/couchdb/nats/meilisearch/n8n）；
2. `HealthService.Detailed()` 按配置遍历探测（http 200-399 即 up；带并发限制信号量与统一 timeout），保留 n8n 专用 API-Key 探测与 rss_fetcher 内部状态两个特例；
3. 新探测项写入 `config/config.yaml` 模板 + 新环境变量同步 `bellkeeper-init.sh`；
4. **单测**：httptest 假服务断言 up/down/degraded 判定。

### Tier 4：n8n 工作流改造与推送

1. 重写 `internal/n8n_workflows/O01-daily-report.json`（3 节点形态，HTTP 节点 `onError: continueErrorOutput` 走告警分支——注意与上一轮不同：**失败必须告警，不能静默续跑**）;
2. O02 / K08 仓库 JSON 移入 `internal/n8n_workflows/archive/`（同步删除 n8n 侧或停用）；
3. `O01-health-monitor.json` 更新：移除 Firecrawl 探测节点，改判 `data.status`；
4. 部署：`git push` → `spool bundle keeper service keeper bellkeeper up` → `POST /api/workflows/definitions/push-all` → `spool n8n list` 核对 active 状态（O02/K08 手动停用）。

### Tier 5：端到端验证

```bash
# 1. 结构化数据正确性（与 dashboard/stats、psql 抽查比对）
spool exec keeper "curl -s 'http://localhost:8090/api/reports/daily-data?date=2026-06-11'"

# 2. 干跑（不写文件不推送）
spool exec keeper "curl -s -X POST http://localhost:8090/api/reports/daily/generate -d '{\"dry_run\":true}'"

# 3. 真实生成 + 检查产物
spool exec keeper "curl -s -X POST http://localhost:8090/api/reports/daily/generate"
spool exec keeper "cat /mnt/NAS/data/knowledge/vault/daily/$(date +%F).md"

# 4. n8n 手动触发 O01，确认 Matrix 收到推送、内容与 daily-data 一致
# 5. 故意停掉一个被探测服务（如 memos），确认日报显式标注 down 而非 ❌全错
```

验收清单（对照 CLAUDE.md 第 3 节）：

```
□ 三个用户报告的问题逐一复验：服务状态真实、清单现行、数字与 dashboard/psql 对账一致
□ 单项数据源失败时日报显式标注，不伪装成 0
□ O02/K08 已停用归档，n8n 上无重复日报
□ 新增 Service/Handler 有真实调用方(路由接线) + 真实单测
□ go build / go vet / go test 全绿；无硬编码密钥(Memos token 走配置)
```

## 六、风险与回滚

| 风险 | 缓解 |
|------|------|
| LLM 进程内调用阻塞日报生成 | 120s 超时 + 失败降级文案；generate 端点整体 180s 超时 |
| 新健康探测把内网抖动放大成告警噪音 | 探测 timeout 短(3s)、O01-health-monitor 保持现有连续失败阈值逻辑 |
| O02/K08 退役后发现独有信息缺失 | 归档不删除；其板块已并入后端渲染器，缺啥加 collector 即可 |
| 回滚 | n8n 工作流 JSON 全在 git，`git revert` + `push-all` 即回旧版；后端新增端点独立、不改既有行为，回滚无连带 |

## 七、待用户确认的决策点

1. **O02、K08 退役并入 O01**——本计划按「合并为单一日报」设计（推荐：同时段三份报告必然漂移）；若希望保留 20:00 资讯摘要 + 21:00 日报的双推送节奏，T2 渲染器拆出「资讯章节」单独端点即可，工作量 +0.5 tier。
2. **服务探测清单**——默认纳入 keeper 全家桶 9 项；couchdb/nats 若认为属于「坏了也不急」可降为不进日报只进 health 接口。
3. **报告推送渠道**——维持 Matrix `daily` 频道不变；是否需要同时推 Memos/邮件等，本期不做。
