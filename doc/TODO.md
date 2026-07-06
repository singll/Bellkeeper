# Bellkeeper TODO

## 爬取调度重设计：DB 公平调度 + 域名冷却 (2026-06-15)

> 取代下方「域名分桶冷却队列重构 (2026-06-14)」草案——见 ADR-0003 与 `doc/archive/tiered-domain-cooling-queue-implementation.md` 顶部的「已被取代」说明。

### 已完成（Tier 1-7，均带真实测试，连真实 PostgreSQL）
- [x] Tier 1: `crawl_domain_profiles` 加 `failure_count`/`request_overrides`/`analysis_result` 字段 + `EnterCooling`/`ClearCooling`/`IsCooling`/`UpdateOverrides`/`FindByDomain`/`FindCoolingWithoutOverrides` 仓储方法
- [x] Tier 2: `DequeueFair` 按域名公平轮转窗口函数 SQL（两步乐观认领）+ 冷却迁 `next_allowed_at` + **删除整个 `domain_coordinator.go`** + service/rss_fetcher rewire（DequeueFair 公平性/冷却跳过单测 3 个）
- [x] Tier 3: 删死代码 `reserveDomainSlot`/`decideDomainThrottle`/`domainThrottleDecision`/`notifyBlocked`/`CoolingHandler`/`SetCooldownHandler`/`truncateStr` + 孤儿测试 + 废弃 `blocked_domains` 死配置
- [x] Tier 4: `CountPendingByDomain` + `domain_pending_cap`(默认5000) + `Enqueue` 每域名配额显式拒绝（⏭️）+ 单测
- [x] Tier 5: `defaultOverridesFor` 确定性规则表为主、LLM 仅兜底 + overrides 迁 `crawl_domain_profiles` + `findOptimizableDomains` 改为「冷却中+无overrides的profile」+ `CollectFailureSamples` 纳入 `skipped` + 单测
- [x] Tier 6: `UpsertFromJob(job, errType, errMsg)` 显式传参（修陈旧 ErrorType）+ 成功后 `ResolveByURL` 清档 + 测试 harness `allModels`/`truncateAll` 补 `CrawlFailure` + 单测
- [x] Tier 7: 重写 ADR-0003 记录被取代方案 + 全量 `go build`/`go vet`/`go test ./...`/`go test -race ./internal/repository ./internal/service` 全绿

### 待办
- [ ] 部署验证：`spool bundle keeper service keeper bellkeeper up`（代码已推送至 git）
- [ ] 部署后用 `POST /api/crawl/queue/cleanup` 分批清理 xz.aliyun.com 历史积压（注意 `MarkSkippedStalePending` 仍为一次性全量更新，超大批量需关注锁与耗时）
- [ ] 运行时观测：取队域名分布是否均衡、冷却到期恢复、配额拒绝是否按预期触发

---

## 域名分桶冷却队列重构 (2026-06-14)（已被 2026-06-15 重设计取代，仅存档）

> 下列条目描述的是内存 `DomainCoordinator` + 三级分桶方案，已在 2026-06-15 评审后整体重设计并替换。保留于此仅为历史追溯。

### 已完成（历史，对应代码已被替换）
- [x] 2026-06-14: ADR-0003 草案 + CONTEXT.md 术语更新（ADR 已于 2026-06-15 重写）
- [x] 2026-06-14: `crawl_failures` 数据模型 + 迁移文件 + Repository（保留，Tier 6 已重构职责为人工待办清单）
- [x] 2026-06-14: `DomainCoordinator` 内存冷却+三级分桶（**已删除**，由 DB `next_allowed_at` + `DequeueFair` 取代）
- [x] 2026-06-14: `DequeueByDomains` + Worker 桶优先级降级取队（**已删除**，由 `DequeueFair` 取代）
- [x] 2026-06-14: 失败全部改为冷却而非 blocked/dead 终态（保留方向，改由 DB 冷却实现）
- [x] 2026-06-14: 移除 `autoLearnDomain`/`isBlockedDomain`/`rebuildBlockedDomains`
- [x] 2026-06-14: 请求覆盖规则 `ExtractionRequest`（保留，类型移至 extractor.go）
- [x] 2026-06-14: RuleOptimizer 冷却触发 LLM 分析（**已改为**规则表为主/LLM 兜底，overrides 改存 profile）
- [x] 2026-06-14: RSS Feed 冷却统一（保留方向，改走 DB 冷却）
- [x] 2026-06-14: `crawl_failures` API + 清理 API（保留）
