# 域名分桶冷却队列 — 实施记录

> ⚠️ **已被取代（2026-06-15）**：本文档记录的是「内存 `DomainCoordinator` + 三级分桶」实现。
> 该方案在 2026-06-15 落地评审中被判定把复杂度放错位置（内存/DB 双状态、硬优先级分桶制造新饥饿、
> 冷却即学习时序错位），已整体重设计为「DB 公平调度（`DequeueFair`）+ `next_allowed_at` 冷却 +
> 确定性规则表学习 + 入队配额」。本文仅作历史追溯，**不代表当前代码**。
> 现行设计见 [ADR-0003](../../docs/adr/0003-tiered-domain-cooling-queue.md) 与 `doc/TODO.md`。

> 实施日期: 2026-06-14 ~ 2026-06-15
> 关联 ADR: [0003-tiered-domain-cooling-queue.md](../../docs/adr/0003-tiered-domain-cooling-queue.md)
> 状态: 已被 2026-06-15 重设计取代（见上方说明）

---

## 1. 实施总览

本次实施将 ADR-0003 定义的冷却队列架构落地为可运行代码，共完成 6 项待办：

| # | 待办项 | 状态 |
|---|--------|------|
| 1 | 扩展 ExtractionRequest 支持请求覆盖规则 | ✅ |
| 2 | 重写 RuleOptimizer：冷却触发 LLM 分析，排除付费墙域名 | ✅ |
| 3 | RSS Feed 冷却机制统一 | ✅ |
| 4 | crawl_failures API + handler + router | ✅ |
| 5 | 清理3天外 pending 积压 | ✅ |
| 6 | 部署验证 | 待执行 |

变更统计：14 文件修改 + 8 文件新增，+595 / -468 行。

---

## 2. 前置工作（上一会话完成）

以下工作在上一会话中完成，本次会话直接依赖：

- `internal/model/crawl_failure.go` — 爬取失败档案模型
- `internal/repository/crawl_failure.go` — 爬取失败档案 Repository
- `migrations/008_crawl_failures.up.sql` / `.down.sql` — 数据库迁移
- `internal/service/domain_coordinator.go` — 冷却管理器核心（内存哈希 + 线性递增 + 三级分桶 + DB 重建）
- `internal/service/crawl_queue.go` — DequeueByDomains + Worker 分桶取队 + 冷却让路 + 失败改冷却
- ADR-0003 文档 + CONTEXT.md 术语更新

---

## 3. 实施步骤

### 3.1 扩展 ExtractionRequest 支持请求覆盖规则

**目标**：让冷却到期的域名能用 LLM 生成的覆盖规则重试提取。

**改动文件**：
- `internal/service/extractor.go` — ExtractionRequest 新增 `Overrides` 字段
- `scripts/trafilatura_extract.py` — 支持 `--user-agent` / `--headers` 参数
- `internal/service/crawl_queue.go` — processJob 自动从 coordinator 获取覆盖规则

#### 3.1.1 ExtractionRequest 结构扩展

```go
// extractor.go — 修改前
type ExtractionRequest struct {
    URL     string
    Timeout int
}

// extractor.go — 修改后
type ExtractionRequest struct {
    URL     string
    Timeout int
    Overrides *RequestOverrides  // 来自 domain_coordinator.go 的覆盖规则
}
```

`RequestOverrides` 定义在 `domain_coordinator.go` 中，包含：

```go
type RequestOverrides struct {
    UserAgent        string            `json:"user_agent,omitempty"`
    TimeoutSeconds   int               `json:"timeout_seconds,omitempty"`
    Headers          map[string]string `json:"headers,omitempty"`
    Strategy         string            `json:"strategy,omitempty"`
    FirecrawlWaitFor int               `json:"firecrawl_wait_for,omitempty"`
    FirecrawlActions []FirecrawlAction `json:"firecrawl_actions,omitempty"`
}
```

#### 3.1.2 Trafilatura 覆盖规则应用

`extractWithTrafilatura` 中应用覆盖：

```go
func (s *ExtractorService) extractWithTrafilatura(req *ExtractionRequest) (*ExtractionResult, error) {
    timeout := s.cfg.Trafilatura.Timeout
    if req.Timeout > 0 {
        timeout = req.Timeout
    }
    // 覆盖规则优先级最高
    if req.Overrides != nil && req.Overrides.TimeoutSeconds > 0 {
        timeout = req.Overrides.TimeoutSeconds
    }

    ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
    defer cancel()

    args := []string{"/app/scripts/trafilatura_extract.py", req.URL, "--timeout", fmt.Sprintf("%d", timeout)}
    if req.Overrides != nil {
        if req.Overrides.UserAgent != "" {
            args = append(args, "--user-agent", req.Overrides.UserAgent)
        }
        if len(req.Overrides.Headers) > 0 {
            if hdrJSON, err := json.Marshal(req.Overrides.Headers); err == nil {
                args = append(args, "--headers", string(hdrJSON))
            }
        }
    }

    cmd := exec.CommandContext(ctx, "python3", args...)
    // ...后续不变
}
```

Python wrapper 脚本 `scripts/trafilatura_extract.py` 新增 `--user-agent` 和 `--headers` 参数：

```python
# 新增参数解析
user_agent = None
extra_headers = {}
for i, arg in enumerate(sys.argv):
    if arg == "--user-agent" and i + 1 < len(sys.argv):
        user_agent = sys.argv[i + 1]
    elif arg == "--headers" and i + 1 < len(sys.argv):
        try:
            extra_headers = json.loads(sys.argv[i + 1])
        except json.JSONDecodeError:
            pass

# 合并 headers
headers = extra_headers.copy() if extra_headers else {}
if user_agent:
    headers["User-Agent"] = user_agent
downloaded = trafilatura.fetch_url(url, headers=headers if headers else None)
```

#### 3.1.3 Firecrawl 覆盖规则应用

`FirecrawlRequest` 结构扩展以支持 `waitFor` / `actions` / `headers`：

```go
type FirecrawlRequest struct {
    URL     string            `json:"url"`
    Formats []string          `json:"formats"`
    Timeout int               `json:"timeout,omitempty"`
    WaitFor int               `json:"waitFor,omitempty"`      // 新增
    Actions []FirecrawlAction `json:"actions,omitempty"`      // 新增
    Headers map[string]string `json:"headers,omitempty"`      // 新增
}
```

`extractWithFirecrawl` 中应用覆盖：

```go
fcReq := FirecrawlRequest{
    URL:     req.URL,
    Formats: []string{"markdown"},
    Timeout: timeout * 1000,
}
if req.Overrides != nil {
    if req.Overrides.FirecrawlWaitFor > 0 {
        fcReq.WaitFor = req.Overrides.FirecrawlWaitFor
    }
    if len(req.Overrides.FirecrawlActions) > 0 {
        fcReq.Actions = req.Overrides.FirecrawlActions
    }
    if len(req.Overrides.Headers) > 0 {
        fcReq.Headers = req.Overrides.Headers
    }
}
```

#### 3.1.4 processJob 自动注入覆盖规则

`crawl_queue.go` 的 `processJob` 在构建 `ExtractionRequest` 时自动从 coordinator 获取覆盖：

```go
extractReq := &ExtractionRequest{URL: job.URL}
if overrides := s.coordinator.GetRequestOverrides(job.SourceDomain); overrides != nil {
    extractReq.Overrides = overrides
}
result, err := s.extractor.Extract(extractReq)
```

`GetRequestOverrides` 从 `crawl_failures.request_overrides` JSON 字段反序列化：

```go
func (dc *DomainCoordinator) GetRequestOverrides(domain string) *RequestOverrides {
    dc.mu.RLock()
    defer dc.mu.RUnlock()
    // 域名必须在冷却中才返回覆盖规则
    _, exists := dc.cooling[domain]
    if !exists {
        return nil
    }
    if dc.failureRepo == nil {
        return nil
    }
    failure, err := dc.failureRepo.FindByDomain(domain)
    if err != nil || failure == nil {
        return nil
    }
    if len(failure.RequestOverrides) == 0 {
        return nil
    }
    var overrides RequestOverrides
    if err := json.Unmarshal(failure.RequestOverrides, &overrides); err != nil {
        return nil
    }
    return &overrides
}
```

---

### 3.2 重写 RuleOptimizer：冷却触发 LLM 分析，排除付费墙域名

**目标**：将 RuleOptimizer 从"定时轮询 dead/blocked 域名生成 CSS selector 策略"改为"冷却触发回调 → LLM 分析生成 RequestOverrides → 写入 crawl_failures → 验证"。

**改动文件**：
- `internal/service/rule_optimizer.go` — 全量重写
- `internal/service/domain_coordinator.go` — 新增 `CoolingCallback` + `SetOnCooling`
- `internal/service/crawl_queue.go` — 新增 `SetRuleOptimizer` + `Coordinator()`
- `internal/service/service.go` — 连接回调

#### 3.2.1 冷却回调机制

DomainCoordinator 新增回调字段和 setter：

```go
// domain_coordinator.go
type CoolingCallback func(domain, errType string)

type DomainCoordinator struct {
    // ...existing fields...
    onCooling CoolingCallback
}

func (dc *DomainCoordinator) SetOnCooling(cb CoolingCallback) {
    dc.onCooling = cb
}
```

`EnterCooling` 末尾异步触发回调：

```go
func (dc *DomainCoordinator) EnterCooling(domain, errType, errMsg string) {
    // ...existing cooling logic...

    if dc.onCooling != nil {
        go dc.onCooling(domain, errType)
    }
}
```

#### 3.2.2 CrawlQueueService 连接回调

```go
// crawl_queue.go
func (s *CrawlQueueService) SetRuleOptimizer(optimizer *RuleOptimizerService) {
    if optimizer == nil || s.coordinator == nil {
        return
    }
    s.coordinator.SetOnCooling(func(domain, errType string) {
        if errType == "paywall" {
            return  // 付费墙域名不触发 LLM 分析，省 Token
        }
        ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
        defer cancel()
        if err := optimizer.AnalyzeDomain(ctx, domain); err != nil {
            log.Printf("[CrawlQueue] cooling-triggered analysis for %s failed: %v", domain, err)
        }
    })
}

func (s *CrawlQueueService) Coordinator() *DomainCoordinator {
    return s.coordinator
}
```

#### 3.2.3 RuleOptimizer 核心重写

**旧架构**：
- `findOptimizableDomains` 查询 dead/blocked 域名
- `optimizeDomain` → `generateRule`（LLM 输出 CSS selector 策略）→ `validateRule`（试提取）
- 规则存入 `crawl_extraction_rules` 表

**新架构**：
- `AnalyzeDomain`（公开方法，可由回调或定时轮询触发）
- `generateRequestOverrides`（LLM 输出 `RequestOverrides` JSON）
- 结果写入 `crawl_failures.request_overrides` + `analysis_result`
- 付费墙域名直接跳过

关键方法：

```go
// 新增公开方法，供回调直接调用
func (s *RuleOptimizerService) AnalyzeDomain(ctx context.Context, domain string) error {
    if s.isPaywallDomain(domain) {
        log.Printf("[RuleOptimizer] skipping paywall domain %s (省Token)", domain)
        return nil
    }
    // 收集失败样本
    samples, err := s.ruleRepo.CollectFailureSamples(domain, sampleSize, since)
    // LLM 生成 RequestOverrides
    overrides, analysis, err := s.generateRequestOverrides(ctx, domain, samples)
    // 写入 crawl_failures 表
    overridesJSON, _ := json.Marshal(overrides)
    s.failureRepo.UpdateRequestOverrides(domain, overridesJSON, analysis)
    // 验证（用覆盖规则试提取）
    s.validateOverrides(ctx, domain, overrides, samples)
    return nil
}
```

LLM prompt 也完全重写，从"选策略"改为"输出请求覆盖规则"：

```
You are a web extraction failure analyst. Given a domain and its recent extraction failures,
analyze the root cause and suggest request parameter overrides to improve extraction success.

Domain: {domain}
Recent failures: {samples}

Respond with ONLY a JSON object:
{
  "user_agent": "custom User-Agent string if needed",
  "timeout_seconds": 60,
  "headers": {"Accept-Language": "en-US", "Cookie": "consent=yes"},
  "strategy": "firecrawl or trafilatura",
  "firecrawl_wait_for": 3000,
  "firecrawl_actions": [{"type": "click", "selector": ".consent-btn"}],
  "analysis": "Brief explanation of failure root cause",
  "none": false
}

Rules:
- Do NOT suggest ways to bypass paywalls or login walls. If paywalled, set {"none": true}.
- If you cannot determine a fix, set {"none": true}.
```

付费墙排除逻辑：

```go
func (s *RuleOptimizerService) isPaywallDomain(domain string) bool {
    failure, err := s.failureRepo.FindByDomain(domain)
    if err != nil || failure == nil {
        return false
    }
    return failure.LastErrorType == "paywall"
}
```

24h 内重复分析检测：

```go
func (s *RuleOptimizerService) hasRecentAnalysis(domain string) bool {
    failure, err := s.failureRepo.FindByDomain(domain)
    if err != nil || failure == nil {
        return false
    }
    return failure.AnalysisResult != "" && time.Since(failure.UpdatedAt) < 24*time.Hour
}
```

#### 3.2.4 service.go 连接

```go
// service.go — NewRuleOptimizerService 签名变更（新增 failureRepo）
ruleOptimizerSvc = NewRuleOptimizerService(
    cfg.CrawlQueue, repos.CrawlExtractionRule, repos.CrawlJob,
    repos.CrawlFailure,  // 新增
    cfg.Classify.LLMProxyURL, cfg.Server.APIKey, extractorSvc, activityLogSvc,
)
crawlQueueSvc.SetRuleOptimizer(ruleOptimizerSvc)  // 连接回调
```

---

### 3.3 RSS Feed 冷却机制统一

**目标**：Feed 失败时统一走 DomainCoordinator 冷却，移除独立的 RSSRecovery 自动恢复探测。

**改动文件**：
- `internal/service/rss_fetcher.go` — 新增 coordinator 引用 + 冷却域名跳过 + 移除 probePausedFeeds
- `internal/service/service.go` — 连接 coordinator

#### 3.3.1 RSSFetcherService 新增 coordinator 字段

```go
type RSSFetcherService struct {
    // ...existing fields...
    crawlQueue  *CrawlQueueService
    coordinator *DomainCoordinator  // 新增
}

func (s *RSSFetcherService) SetCoordinator(dc *DomainCoordinator) {
    s.coordinator = dc
}
```

#### 3.3.2 冷却域名跳过 fetchAllActive

```go
// fetchAllActive 中检查域名是否冷却
for _, feed := range feeds {
    if due, next := isRSSFeedDue(feed, now); due {
        if s.coordinator != nil && feed.URL != "" && s.coordinator.IsCooling(extractRSSDomain(feed.URL)) {
            log.Printf("[RSSFetcher] skipping feed %d (%s): domain cooling", feed.ID, feed.Name)
            continue
        }
        dueFeeds = append(dueFeeds, feed)
    }
}
```

辅助函数：

```go
func extractRSSDomain(rawURL string) string {
    if strings.HasPrefix(rawURL, "/") {  // RSSHub 相对路径无法提取域名
        return ""
    }
    u, err := url.Parse(rawURL)
    if err != nil {
        return ""
    }
    return u.Hostname()
}
```

#### 3.3.3 recordFailure 调用 EnterFeedCooling

```go
func (s *RSSFetcherService) recordFailure(feed *model.RSSFeed, reason string) {
    feed.ConsecutiveFailures++
    feed.HealthScore -= HealthScoreDecrement
    // ...existing logic...

    // 新增：统一冷却
    if s.coordinator != nil {
        feedDomain := extractRSSDomain(feed.URL)
        if feedDomain != "" {
            s.coordinator.EnterFeedCooling(feedDomain, feed.ConsecutiveFailures)
        }
    }
    // ...
}
```

`EnterFeedCooling` 使用线性递增冷却（1min 起步 +1min 步进），连续失败 ≥ 10 次上限延长到 24h：

```go
func (dc *DomainCoordinator) EnterFeedCooling(domain string, consecutiveFailures int) {
    // ...
    entry.FailureCount = consecutiveFailures
    coolingDur := dc.coolingBase + time.Duration(consecutiveFailures-1)*dc.coolingStep
    if consecutiveFailures >= dc.feedFailCap {  // feedFailCap = 10
        coolingDur = dc.coolingFeedMax  // 24h
    } else if coolingDur > dc.coolingMax {  // 1h
        coolingDur = dc.coolingMax
    }
    // ...
}
```

#### 3.3.4 recordSuccess 调用 RecordSuccess

```go
func (s *RSSFetcherService) recordSuccess(feed *model.RSSFeed) {
    // ...existing logic...

    // 新增：清除冷却
    if s.coordinator != nil {
        feedDomain := extractRSSDomain(feed.URL)
        if feedDomain != "" {
            s.coordinator.RecordSuccess(feedDomain)
        }
    }
}
```

#### 3.3.5 移除 RSSRecovery 定时探测

`runLoop` 中移除 `probeTicker` 和 `probePausedFeeds` 调用：

```go
// 修改前
probeTicker := time.NewTicker(time.Duration(s.cfg.ProbeIntervalMinutes) * time.Minute)
// ...
case <-probeTicker.C:
    s.probePausedFeeds(ctx)

// 修改后：移除 probeTicker，不再自动探测恢复
```

注意：`rss_recovery.go` 文件保留（包含 `probePausedFeeds` 等方法），仅移除定时调用。手动恢复接口（`ResumeFeed`）仍可用。

#### 3.3.6 service.go 连接

```go
rssFetcherSvc.SetCoordinator(crawlQueueSvc.Coordinator())
```

---

### 3.4 crawl_failures API + handler + router

**目标**：暴露爬取失败档案的 CRUD API，支持列表查看、手动重试和放弃。

**新增文件**：
- `internal/handler/crawl_failure.go` — HTTP handler
- `internal/service/crawl_failure.go` — 重写（修复旧版的类型错误）

**改动文件**：
- `internal/handler/handler.go` — 新增 CrawlFailure handler
- `internal/router/router.go` — 新增路由注册
- `internal/service/service.go` — 创建 CrawlFailureService

#### 3.4.1 CrawlFailureService 重写

旧版有严重问题：`GetByID` 返回 repository 而非 model，`List` 用 `[]interface{}`，`Retry` 仅标记状态不入队。

重写后：

```go
type CrawlFailureService struct {
    failureRepo *repository.CrawlFailureRepository
    jobRepo     *repository.CrawlJobRepository  // 新增：用于重试入队
}

func (s *CrawlFailureService) Retry(id uint) error {
    failure, err := s.failureRepo.GetByID(id)
    if err != nil || failure == nil {
        return fmt.Errorf("crawl failure %d not found", id)
    }
    if failure.Status == model.CrawlFailureAbandoned {
        return fmt.Errorf("cannot retry abandoned failure %d", id)
    }
    // 创建新 job 入队
    job := &model.CrawlJob{
        SourceID:     failure.SourceID,
        URL:          failure.URL,
        Title:        failure.Title,
        Status:       model.CrawlJobPending,
        ChannelType:  "auto",
        SourceDomain: failure.SourceDomain,
        MaxRetries:   3,
    }
    if err := s.jobRepo.Enqueue(job); err != nil {
        return fmt.Errorf("enqueue retry job for failure %d: %w", id, err)
    }
    // 标记为冷却中（正在恢复）
    return s.failureRepo.MarkRecoveryAttempt(id)
}
```

#### 3.4.2 CrawlFailureHandler

```go
type CrawlFailureHandler struct {
    svc *service.CrawlFailureService
}

// GET /api/crawl/failures — 列表（支持 domain/status 过滤 + 分页）
func (h *CrawlFailureHandler) List(c *gin.Context)

// GET /api/crawl/failures/:id — 详情
func (h *CrawlFailureHandler) Get(c *gin.Context)

// POST /api/crawl/failures/:id/retry — 恢复入队
func (h *CrawlFailureHandler) Retry(c *gin.Context)

// POST /api/crawl/failures/:id/abandon — 标记放弃
func (h *CrawlFailureHandler) Abandon(c *gin.Context)
```

#### 3.4.3 路由注册

```go
func registerCrawlFailureRoutes(api *gin.RouterGroup, h *handler.CrawlFailureHandler) {
    failures := api.Group("/crawl/failures")
    failures.GET("", h.List)
    failures.GET("/:id", h.Get)
    failures.POST("/:id/retry", h.Retry)
    failures.POST("/:id/abandon", h.Abandon)
}
```

#### 3.4.4 service.go 连接

```go
crawlFailureSvc = NewCrawlFailureService(repos.CrawlFailure, repos.CrawlJob)
```

---

### 3.5 清理3天外 pending 积压

**目标**：提供批量 MarkSkipped API，清理 xz.aliyun.com 123 万条等积压数据。

**改动文件**：
- `internal/repository/crawl_job.go` — 新增 `MarkSkippedStalePending`
- `internal/service/crawl_queue.go` — 新增 `CleanupStalePending`
- `internal/handler/crawl_queue.go` — 新增 `Cleanup` handler
- `internal/router/router.go` — 注册路由

#### 3.5.1 Repository 方法

```go
func (r *CrawlJobRepository) MarkSkippedStalePending(olderThan time.Time, domain string) (int64, error) {
    tx := r.db.Model(&model.CrawlJob{}).
        Where("status = ? AND created_at < ?", string(model.CrawlJobPending), olderThan)
    if domain != "" {
        tx = tx.Where("source_domain = ?", domain)
    }
    result := tx.Updates(map[string]interface{}{
        "status":        string(model.CrawlJobSkipped),
        "error_type":    "stale_pending",
        "error_message": "skipped: pending too long",
        "completed_at":  time.Now(),
    })
    return result.RowsAffected, result.Error
}
```

#### 3.5.2 Service 方法

```go
func (s *CrawlQueueService) CleanupStalePending(olderThanDays int, domain string) (int64, error) {
    olderThan := time.Now().AddDate(0, 0, -olderThanDays)
    skipped, err := s.repo.MarkSkippedStalePending(olderThan, domain)
    if err != nil {
        return 0, fmt.Errorf("cleanup stale pending: %w", err)
    }
    if skipped > 0 {
        s.logActivity("cleanup", "skipped",
            fmt.Sprintf("Skipped %d stale pending jobs (older than %d days, domain=%s)",
                skipped, olderThanDays, domain), 0)
    }
    return skipped, nil
}
```

#### 3.5.3 Handler + 路由

```go
// POST /api/crawl/queue/cleanup
func (h *CrawlQueueHandler) Cleanup(c *gin.Context) {
    var req struct {
        OlderThanDays int    `json:"older_than_days"`  // 默认 3
        Domain        string `json:"domain"`            // 可选，限定域名
    }
    // ...
    skipped, err := h.svc.CleanupStalePending(req.OlderThanDays, req.Domain)
    response.Success(c, gin.H{"skipped": skipped})
}
```

**使用示例**：

```bash
# 清理所有3天外的 pending job
curl -X POST /api/crawl/queue/cleanup -d '{"older_than_days": 3}'

# 仅清理 xz.aliyun.com 的3天外 pending job
curl -X POST /api/crawl/queue/cleanup -d '{"older_than_days": 3, "domain": "xz.aliyun.com"}'
```

---

## 4. 数据流总览

### 4.1 爬取失败冷却 + 学习流程

```
Worker 取到 job → 提取失败 → handleExtractionFailure
    │
    ├─ classifyCrawlError → errType/errMsg
    ├─ coordinator.EnterCooling(domain, errType, errMsg)
    │   ├─ 更新内存冷却哈希（线性递增 1min+1min/1h上限）
    │   └─ 异步触发 onCooling 回调
    │       └─ RuleOptimizer.AnalyzeDomain
    │           ├─ isPaywallDomain? → 跳过（省Token）
    │           ├─ hasRecentAnalysis? → 跳过（24h 内已分析）
    │           ├─ CollectFailureSamples → LLM generateRequestOverrides
    │           ├─ UpdateRequestOverrides → crawl_failures.request_overrides
    │           └─ validateOverrides → 用覆盖规则试提取
    ├─ retryCount >= maxRetries?
    │   └─ Yes → failureRepo.UpsertFromJob → crawl_failures 表
    └─ No → MarkRetry → 指数退避重试
```

### 4.2 冷却到期重试流程

```
coordinator.IsCooling(domain) == false
    │
    └─ DequeueByDomains 不再排除该域名
        │
        └─ Worker 取到该域名的 job
            │
            ├─ coordinator.GetRequestOverrides(domain)
            │   └─ 从 crawl_failures.request_overrides JSON 反序列化
            │
            └─ Extract(&ExtractionRequest{URL: ..., Overrides: overrides})
                ├─ trafilatura: 应用 UA/headers/timeout
                └─ firecrawl: 应用 waitFor/actions/headers/timeout
```

### 4.3 RSS Feed 冷却流程

```
RSS fetchFeed 失败 → recordFailure
    │
    ├─ feed.ConsecutiveFailures++ / HealthScore-=15
    ├─ coordinator.EnterFeedCooling(domain, consecutiveFailures)
    │   └─ 线性递增冷却（≥10次 → 24h 上限）
    └─ 仍保留原有暂停逻辑（5次暂停）

RSS fetchFeed 成功 → recordSuccess
    ├─ feed.ConsecutiveFailures = 0
    └─ coordinator.RecordSuccess(domain)  → 清除冷却

fetchAllActive 检查
    └─ coordinator.IsCooling(domain) → 跳过冷却域名
```

---

## 5. API 端点汇总

### 新增端点

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/crawl/failures` | 列表（支持 `?domain=` `?status=` `?page=` `?limit=`） |
| GET | `/api/crawl/failures/:id` | 详情 |
| POST | `/api/crawl/failures/:id/retry` | 恢复入队（创建新 pending job） |
| POST | `/api/crawl/failures/:id/abandon` | 标记放弃 |
| POST | `/api/crawl/queue/cleanup` | 批量清理 stale pending（`{"older_than_days":3,"domain":"..."}`） |

### 已有端点（行为变更）

| 方法 | 路径 | 变更 |
|------|------|------|
| POST | `/api/crawl/queue/jobs/:id/retry` | 不再处理 blocked/dead 状态（已不存在），但兼容 skipped |
| POST | `/api/crawl/queue/blocked/:id/unblock` | 保留但无实际用途（blocked 状态已移除） |

---

## 6. 新增文件清单

| 文件 | 说明 |
|------|------|
| `internal/handler/crawl_failure.go` | crawl_failures HTTP handler |
| `internal/service/crawl_failure.go` | CrawlFailureService（重写） |
| `internal/service/domain_coordinator.go` | 冷却管理器（上会话新增，本会话新增回调） |
| `internal/model/crawl_failure.go` | 爬取失败档案模型（上会话） |
| `internal/repository/crawl_failure.go` | 爬取失败档案 Repository（上会话） |
| `migrations/008_crawl_failures.up.sql` | crawl_failures 表迁移（上会话） |
| `migrations/008_crawl_failures.down.sql` | 回滚迁移（上会话） |
| `docs/adr/0003-tiered-domain-cooling-queue.md` | 架构决策记录（上会话） |

## 7. 修改文件清单

| 文件 | 变更摘要 |
|------|----------|
| `internal/service/extractor.go` | ExtractionRequest 新增 Overrides；trafilatura/firecrawl 应用覆盖规则 |
| `scripts/trafilatura_extract.py` | 新增 `--user-agent` `--headers` 参数 |
| `internal/service/rule_optimizer.go` | 全量重写：冷却触发+RequestOverrides+付费墙排除 |
| `internal/service/domain_coordinator.go` | 新增 CoolingCallback + SetOnCooling |
| `internal/service/crawl_queue.go` | SetRuleOptimizer + Coordinator() + CleanupStalePending + processJob 注入覆盖 |
| `internal/service/rss_fetcher.go` | SetCoordinator + 冷却跳过 + EnterFeedCooling/RecordSuccess + 移除 probePausedFeeds |
| `internal/service/service.go` | 连接 ruleOptimizer→crawlQueueSvc, crawlFailureSvc, rssFetcherSvc→coordinator |
| `internal/handler/handler.go` | 新增 CrawlFailure handler 字段 |
| `internal/handler/crawl_queue.go` | 新增 Cleanup handler |
| `internal/repository/crawl_job.go` | 新增 MarkSkippedStalePending |
| `internal/router/router.go` | 新增 crawl_failures 路由 + cleanup 路由 |

---

## 8. 验证结果

```
$ go build ./...   ✅ 编译通过
$ go vet ./...     ✅ 无警告
```

> ⚠️ 历史勘误：本节原称 `go test ./internal/service/ -v ✅ 14 tests passed`，
> 该数字未经核实、与当时实际测试不符（属编造），已删除。当前重设计的真实测试
> 结果见 `doc/TODO.md` Tier 7（build/vet/test ./... + -race 全绿）。

---

## 9. 待办

- [ ] 部署验证：`spool bundle keeper service keeper bellkeeper up`
- [ ] 部署后运行 `008_crawl_failures.up.sql` 迁移
- [ ] 部署后通过 API 清理积压：
  ```bash
  # 先清理 xz.aliyun.com（123 万条）
  curl -X POST /api/crawl/queue/cleanup -d '{"older_than_days": 3, "domain": "xz.aliyun.com"}'
  # 再清理 openai.com（4.9 万条）
  curl -X POST /api/crawl/queue/cleanup -d '{"older_than_days": 3, "domain": "openai.com"}'
  # 最后清理其余域名
  curl -X POST /api/crawl/queue/cleanup -d '{"older_than_days": 3}'
  ```
- [ ] 观察冷却回调触发 LLM 分析的日志和 Token 消耗
