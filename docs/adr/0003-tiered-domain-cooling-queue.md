# ADR-0003: 数据库驱动的公平调度 + 域名冷却队列

> 注：文件名 `tiered-domain-cooling-queue` 沿用旧草案命名；本 ADR 的实际决策是 **DequeueFair 公平轮转 + 冷却**（非三级分桶），为当前生效方案。

## Status

Accepted（2026-06-15）。本文档取代早先的「按域名通过率三级分桶 + 内存协调器」草案——该草案在落地评审中被判定为把复杂度放错位置（详见末尾「被取代的方案」）。

## Context

爬取队列存在三个严重问题：

1. **全局队列单点阻塞**：xz.aliyun.com 的 ~123 万 pending job 占据绝大部分队列，Worker 反复取到被限速域名的 job 空转。
2. **永久阻断无学习**：blocked/dead 是终态，域名一旦 paywall/forbidden 即永久封禁，无恢复机制。
3. **RuleOptimizer 低效**：91.9% 的 LLM 生成规则被 rejected，对付费墙/反爬站点几乎无效。

根因有两层：**消费侧**单一慢域名拖垮整个队列、失败域名被一刀切断没有恢复机会；**生产侧**入队无每域名配额，发现速度一旦超过消费速度（被限速时必然如此）队列就无界膨胀——123 万只是时间问题。

## Decision

### 1. 按域名公平轮转的单条 SQL 调度（`DequeueFair`）

取队不再依赖内存桶/域名集合，而是一条窗口函数 SQL：

```sql
SELECT id FROM (
  SELECT j.id, j.created_at,
    ROW_NUMBER() OVER (PARTITION BY j.source_domain
                       ORDER BY j.priority DESC, j.created_at, j.id) AS rn
  FROM crawl_jobs j
  LEFT JOIN crawl_domain_profiles p ON p.domain = j.source_domain AND p.deleted_at IS NULL
  WHERE j.deleted_at IS NULL
    AND j.status IN ('pending','retrying')
    AND (j.next_retry_at IS NULL OR j.next_retry_at <= now)
    AND (p.next_allowed_at IS NULL OR p.next_allowed_at <= now)   -- 冷却过滤
    [AND j.channel_type IN (:channel,'auto')]
) t WHERE t.rn = 1 ORDER BY t.created_at LIMIT 8;
```

- 每个域名每轮只露出**队首一条**，xz.aliyun.com 的百万 pending 无法挤占其它域名（防垄断）。
- `LEFT JOIN` 让无 profile 的新域名也能取到；`next_allowed_at` 过滤天然排除冷却中的域名。
- PostgreSQL 不允许 `FOR UPDATE` 与窗口函数同层，故分两步：① 无锁选候选 id；② 逐个 `UPDATE ... WHERE status IN (pending,retrying)` 原子认领，`RowsAffected==1` 即成功，否则试下一个候选（乐观并发，无需 SKIP LOCKED 跨窗口函数）。

### 2. 冷却 = `crawl_domain_profiles.next_allowed_at`（DB 持久，指数退避）

冷却不再是内存状态，而是 profile 表的两列：

- `EnterCooling(domain, base, max)`：`failure_count += 1`，`next_allowed_at = now + min(base*2^(failure_count-1), max)`（指数退避，网页爬取 base=1min/max=1h）。
- `ClearCooling(domain)`：成功后 `failure_count=0, next_allowed_at=NULL`。
- 取队时由上面的 SQL 过滤，无内存态、重启不丢、无锁竞争。

所有失败类型（含 paywall）统一走 `EnterCooling`，不再产生 blocked/dead 新终态；`blocked` 仅作历史数据保留查询。

### 3. 入队每域名配额（治本，消费侧之外的安全阀）

`Enqueue` 在插入前检查 `CountPendingByDomain(domain)`，达到 `domain_pending_cap`（默认 5000）即**显式拒绝**（`⏭️` activity，不静默丢弃）。这是阻止单域名无界积压的根治手段——任何取队策略都治标。

### 4. 学习：确定性规则表为主，LLM 仅兜底

`RuleOptimizer.AnalyzeDomain`：

1. 先按主导 `error_type` 查确定性规则表 `defaultOverridesFor`：`forbidden`→浏览器 UA+Referer；`timeout`/`network`→firecrawl+长 timeout；`empty_content`→firecrawl+waitFor；`rate_limited`/`server_error`→不改（靠退避）。命中即写 profile，**不调 LLM**。
2. 仅规则表未覆盖（unknown）时才调 LLM 兜底。

overrides/analysis 存 `crawl_domain_profiles.request_overrides`/`analysis_result`（域名级，本就是冷却载体），不再存 URL 级表。可优化域名来源从「dead/blocked job」改为「冷却中 + 无 overrides 的 profile」。失败样本采集纳入 `skipped`（新路径耗尽重试后归档为 skipped）。

### 5. 失败档案 = 人工待办清单（不背配置职责）

`crawl_failures` 仅记录 url/domain/failure_count/last_error/status，供人工 `retry`/`abandon`。`UpsertFromJob(job, errType, errMsg)` 显式传入分类结果（不再读 job 上可能陈旧的 ErrorType）。URL 成功后 `ResolveByURL` 将其移出清单。

### 6. RSS Feed 冷却统一到同一机制

Feed 失败/成功复用 `EnterCooling`/`ClearCooling`/`IsCooling`（feed base=1min/max=24h），与网页爬取共用 `crawl_domain_profiles`，移除内存协调器中的独立 feed 冷却。

## Consequences

- **正面**：删除整个内存 `DomainCoordinator`（tierMap + cooling map），调度/冷却均为 DB 单一事实源；慢域名自动让路、可恢复；入队配额根治积压；规则表学习省 Token、可预测、可单测。
- **负面**：每次取队多一次窗口函数扫描（活跃域名极多时需靠入队配额控制规模）；overrides 读取在 processJob 增加一次按域名索引查询。
- **风险/权衡**：feed 与文章同域名时共享一行 profile 冷却（有意的统一）；唯一索引去重已由 `Enqueue` 的 COUNT 承担，未升级为 partial unique index（并发竞态窗口极小，列为可选后续）。

## 被取代的方案（为何不做三级分桶 + 内存协调器）

早先草案用内存 `tierMap` 按 24h 通过率把域名分三桶、Worker 按桶优先级降级取队，并在内存 `cooling` map 里维护冷却。评审发现：① 它在消费侧新建内存子系统，却把已有的 `next_allowed_at` DB 机制废弃成死代码，造成内存/DB 双状态不一致；② 硬优先级分桶把「单域名垄断」换成「高通过率域名群垄断」，制造新饥饿，且承诺的 worker 5:3:2 配额从未实现；③ 冷却即学习的 1min 冷却 vs 3min LLM 分析时序错位，且样本信息不足，大概率重蹈 91.9% 失败。本 ADR 的 DB 公平调度 + 规则表方案以更少的运行态和代码消除了这些问题。
