# Bellkeeper MEMORY

> 最后更新: 2026-06-14

## 当前进度

- **v1.0.0**: 已打 tag ✅
- **提示词 P0 修复**: ✅ 全部完成(6项)
- **域名分桶冷却队列重构**: ✅ 核心架构+全部6项待办已完成，仅剩部署验证
  - ADR-0003 已写入 `docs/adr/0003-tiered-domain-cooling-queue.md`
  - CONTEXT.md 已新增术语：分桶、冷却、冷却让路、学习分析、请求覆盖规则、爬取失败档案

## 关键决策 — 域名分桶冷却队列

1. **三级分桶**：按24h滑动窗口通过率分桶（high≥30%/medium10-30%/low<10%），Worker按5:3:2分配，降级取队不空转
2. **冷却替代阻断**：所有失败类型(含paywall)进冷却，不再有blocked/dead终态。冷却1min起步，每次+1min，上限1h，成功重置
3. **冷却让路**：Dequeue SQL层排除冷却域名，Worker直接取到可处理域名job
4. **冷却即学习**：域名冷却时立即触发LLM分析失败原因，输出完整请求覆盖规则(所有HTTP header/timeout/strategy/firecrawl参数)，冷却到期用新方案重试
5. **省Token**：付费墙域名进冷却但不触发RuleOptimizer
6. **爬取失败档案**：独立`crawl_failures`表，job达max_retries后写入，支持查询和恢复入队
7. **RSS Feed冷却统一**：线性递增+24h上限(连续失败≥10次)，移除RSSRecovery自动恢复
8. **请求覆盖规则**：ExtractionRequest新增Overrides字段，trafilatura支持UA/headers，firecrawl支持waitFor/actions/headers
9. **RuleOptimizer重写**：冷却触发回调→LLM生成RequestOverrides→写入crawl_failures→验证提取

## 关键文件 — 重构新增/修改

- `internal/service/domain_coordinator.go` — 冷却管理器(核心中枢) + CoolingCallback + SetOnCooling
- `internal/model/crawl_failure.go` — 爬取失败档案模型
- `internal/repository/crawl_failure.go` — 爬取失败档案Repository
- `internal/service/crawl_queue.go` — 已重写: DequeueByDomains+分桶+冷却让路+SetRuleOptimizer+Coordinator()+CleanupStalePending
- `internal/service/rule_optimizer.go` — 已重写: 冷却触发+RequestOverrides生成+付费墙排除+验证
- `internal/service/crawl_failure.go` — 已重写: List/GetByID/Retry(真实入队)/Abandon
- `internal/service/rss_fetcher.go` — 已修改: SetCoordinator+冷却域名跳过+EnterFeedCooling+移除probePausedFeeds
- `internal/service/extractor.go` — ExtractionRequest新增Overrides+trafilatura/firecrawl应用覆盖规则
- `scripts/trafilatura_extract.py` — 支持--user-agent/--headers参数
- `internal/handler/crawl_failure.go` — List/Get/Retry/Abandon handler
- `internal/handler/crawl_queue.go` — 新增Cleanup handler
- `migrations/008_crawl_failures.up.sql` — crawl_failures表迁移
- `docs/adr/0003-tiered-domain-cooling-queue.md` — 架构决策记录

## 运行时数据快照 (2026-06-14)

- pending队列: 1,284,542 (97%)，其中xz.aliyun.com 123万、openai.com 4.9万
- 成功率: 0.11% (1,486/1,326,544)
- blocked: 338，dead: 50，retrying: 486(474到期未执行)
- RuleOptimizer: 483规则中444 rejected(91.9%)
- PostgreSQL连接池偶尔耗尽

## 下一步

1. 部署验证（spool bundle keeper service keeper bellkeeper up）

## 环境备忘

- Docker 已可用（ubuntu 用户已加入 docker 组）
- 测试 PG 容器：`docker ps` 应显示 `bellkeeper-test-postgres`
- 启动命令：`docker run -d --name bellkeeper-test-postgres -e POSTGRES_DB=bellkeeper_test -e POSTGRES_USER=bellkeeper -e POSTGRES_PASSWORD=testpass -p 15432:5432 postgres:16-alpine`
- Agent 配置项：`matrix.agent.enabled/model/max_turns_per_hour/session_ttl/max_tool_iterations/system_prompt`
- bellkeeper-init.sh 已新增 Agent 环境变量导出
