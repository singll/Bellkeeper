# Bellkeeper MEMORY

> 最后更新: 2026-07-03

## 当前进度

- **v1.0.0**: 已打 tag ✅
- **提示词 P0 修复**: ✅ 全部完成(6项)
- **域名分桶冷却队列重构**: ✅ 已被 DB 公平调度(DequeueFair)取代，代码已推送
- **PKB 原子知识网 Phase A-D**: ✅ 全部落地(提示词v2+一文多卡+语义去重+分层MOC+身份模型)
- **PKB 骨架 Phase F-H**: ✅ 全部落地(骨架+归位+缺口填充+资讯库+晋升闸)
- **PKB 自动闭环+域管理**: ✅ 全部落地(多任务调度+领域CRUD+域状态+骨架触发+前端骨架页)
- **知识库模块重做 阶段1-2**: ✅ 全部落地(Matrix agent通电+Web问答不拒答+真多轮+搜索重定位+总览页)
- **可靠性加固 Tier 2-3**: ✅ 已完成(n8n退役10个+K01/K02修复)
- **LLM Proxy**: ✅ 限流学习器+Kimi熔断恢复+4 provider余额拉取

## 关键决策 — 域名公平调度(取代分桶冷却)

1. **DequeueFair**: 按域名公平轮转窗口函数SQL(两步乐观认领)+冷却迁`next_allowed_at`
2. **冷却替代阻断**: 所有失败进冷却，不再有blocked/dead终态
3. **域名配额**: `domain_pending_cap`(默认5000)+Enqueue每域名配额显式拒绝
4. **确定性规则优先**: `defaultOverridesFor`规则表为主、LLM仅兜底
5. **爬取失败档案**: 独立`crawl_failures`表，job达max_retries后写入

## 下一步

1. 部署验证（spool bundle keeper service keeper bellkeeper up）
2. PKB 存量 raw 批跑 + cron 固化
3. 提示词基础设施(response_format/模板校验/自修复/golden set/角色分离)
4. 日志中心优化(Meili全文/归档/SSE/告警增强/trace_id传播)
5. 前端(爬取队列可视化/Vault预览/问答SSE流式/Matrix 7→3页重构)
