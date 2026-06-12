# SilkSpool & Bellkeeper 演进规划

> 更新日期: 2026-06-12
>
> 反馈环:[STATUS.md](STATUS.md)(现状)→ ROADMAP(演进)→ git commit(实施)→ STATUS(回写)。已完成计划的原始文档在 [archive/](archive/)。

---

## 0. 优先级总览

| 优先级 | 类别 | 摘要 | 估算 |
|--------|------|------|------|
| **P0** | PKB 运行收尾 | 存量 ~308 篇 raw 分批跑完 + cron 固化 + 线上验收(§2.1) | 0.5 天 + 观察 |
| **P0** | ~~提示词 P0 修复~~ | ✅ 已完成 2026-06-12 | — |
| **P1 ⭐** | PKB 原子知识网 | [PKB-ATOMIC-KNOWLEDGE-PLAN.md](PKB-ATOMIC-KNOWLEDGE-PLAN.md) Phase A–E(§2.2) | ~4.75 天 |
| **P1** | 提示词基础设施 | response_format + 模板校验 + 自修复重试 + golden set eval(§4.2) | 1.5–2 天 |
| **P1** | LLM Proxy 验收 | 运行时验证清单 + 调用方迁移 + new-api 停服评估(§3) | 0.5 天 + 7 天观察 |
| **P1** | 爬虫运营验收 | 新源批量导入(20–40 个)+ 7 天成功率指标 + 周健康报告(§6) | 持续迭代 |
| **P1** | 日志中心 | Meili 全文检索 + 告警增强 + trace_id 关联(§7) | 4 天 |
| **P1** | 前端 | 爬取队列可视化页 + Vault 预览增强 + 问答多轮/流式(§8) | 3 天 |
| **P1** | Matrix 前端重构 | 7→3 页重构(overhaul plan T9)(§8.1) | 3 天 |
| **P2** | 问答优化 | knowledge_ask 接 Rerank + 引用跳转 + 上下文压缩 + 历史会话(§10) | 3 天 |
| **P2** | 可观测性 | Prometheus 抓取 + Grafana 看板(§11) | 3 天 |

---

## 已完成里程碑

| 内容 | 落地时间 | 归档/文档 |
|------|---------|-----------|
| LLM Proxy Tier 0–9 整改(Token/计费/余额/路由/粘性/限流/熔断/告警/Gemini/Rerank) | 05-30~06-01 | [LLM_PROXY_GUIDE.md](LLM_PROXY_GUIDE.md) |
| LLM UI 10→5 页收敛 + 统一凭证模型 | 06-07 | [archive/LLM-UI-REDESIGN.md](archive/LLM-UI-REDESIGN.md) |
| PKB MVP:三层存储 + pkb-curate + digest + 提示词外置 + llm_jobs + n8n 纳管 | 06-06~06-09 | [PKB-IMPLEMENTATION.md](PKB-IMPLEMENTATION.md) |
| 爬取/标签/RSSHub 优化 P0–P4(域名限速/LLM规则/标签/验证) | 06-09 | [archive/CRAWL-TAGGING-...md](archive/CRAWL-TAGGING-RSSHUB-OPTIMIZATION-PLAN-2026-06-09.md) |
| 架构审查 P0/P1 整改(token权限/路径逃逸/并发/契约/迁移/lint/例外) | 06-08~06-09 | [archive/ARCHITECTURE-REVIEW-...md](archive/ARCHITECTURE-REVIEW-2026-06-08.md) |
| 通知/监控重构:后端驱动日报 + O02/K08 退役 | 06-10 | [notification-monitoring-overhaul-plan.md](notification-monitoring-overhaul-plan.md) |
| Phase 0-4: Matrix 止血重构(T1-T4: bug修/聚合去重/命令重构/异步化/权限/事件清理) | 06-11 | [matrix-platform-overhaul-plan.md](matrix-platform-overhaul-plan.md) |
| Phase 1-6: 死代码清理/RAGFlow退役/golang-migrate/RSS统一/P0 bug修/LLM统一/sanitizer | 06-11 | — |
| Phase 7: 31 Repository 全覆盖 + 核心链路测试 + LLM 协议转换测试 + 日报逻辑测试 | 06-11 | — |
| Phase 8: golangci-lint v2 error 清零 | 06-11 | — |
| Phase 9-10 T5-T8: Agent MVP + 写工具 + API 补齐 + 前端对齐 | 06-12 | — |

---

## 1. 安全:认证层(✅ 已完成关闭)

生产环境纯内网,noauth 模式为预期状态。LLM Token 鉴权(`/api/llm/v1/*`)保持独立。

---

## 2. PKB:运行收尾 + 原子知识网

### 2.1 MVP 运行收尾(P0)

- [ ] 存量 ~308 篇 raw 按预算分批跑完(`pkb-curate`,经 llm_jobs 队列)
- [ ] cron / 调度参数固化
- [ ] 线上验收:打分合理性、LiveSync 同步范围(raw 不进)、Meili rebuild 结果
- [ ] `spool n8n export` 冷备进 Git + 线上漂移检测

### 2.2 原子知识网升级(P1 ⭐)

按 [PKB-ATOMIC-KNOWLEDGE-PLAN.md](PKB-ATOMIC-KNOWLEDGE-PLAN.md) 执行:

- [ ] Phase A:Tier 1 提示词 v2 + Tier 5 权重 + validateCard 双栈(0.5 天)
- [ ] Phase B:Tier 3 一文多卡 + 同源双向链接 + concept slug(1 天)
- [ ] Phase C:Tier 2 语义去重 + supplement 归并(1 天)
- [ ] Phase D:Tier 4 分层 MOC(`_index.md` + `topics/`)(1.5 天)
- [ ] Phase E:Tier 6 网健康度 audit + Dataview(0.75 天)

---

## 3. LLM Proxy:运行时验收与 new-api 停服(P1)

- [ ] prompt cache 命中率:Anthropic cache hit > 80%
- [ ] 自适应限流学习曲线:虚高 30min 回落 / 虚低 24h 上调
- [ ] Kimi Code 订阅:403 → 长熔断 → 5h 探测自恢复
- [ ] 各 provider 真实余额拉取正常
- [ ] 调用方 base_url 迁移
- [ ] 观察 7 天稳定后停 new-api 容器

---

## 4. LLM / PKB 提示词工程

来源:[LLM-PROMPT-AGENT-REVIEW.md](LLM-PROMPT-AGENT-REVIEW.md)。

### 4.1 P0 具体修复(✅ 已完成 2026-06-12)

- [x] 任务路由 token 启发式:estimateTokens 中文低估修复(rune/2→rune*2)
- [x] knowledge_ask `buildContext` 按字节截断中文 → 已改 rune(前序修复)
- [x] digest 入选阈值硬编码 7.0 → 已通过 YAML 配置可调(前序修复)
- [x] rule_optimizer 硬编码模型 + 未注册 TaskType → 默认改 pool-summary + 注册 TaskRuleGeneration
- [x] OpenAI→Anthropic 转换 tool_choice → 仅在 tools 非空时设置
- [x] Anthropic max_tokens 注释修正 + stripJSONFence/stripCardFence 统一为 textutil.StripFence

### 4.2 P1 基础设施

- [ ] llmclient 支持 `max_tokens` / `response_format: json_object` 并在 proxy 透传
- [ ] 提示词模板渲染校验:渲染后残留 `{{...}}` 报错
- [ ] 内容级自修复重试:JSON/结构校验失败时带错误回喂一次
- [ ] 提示词 golden set 评估:10–20 篇带预期样本 + `pkb-curate eval`
- [ ] system/user 角色分离(PKB 提示词规则段放 system,利用 prompt cache)
- [ ] 提示词管理统一(P2):knowledge_ask / rule_optimizer / classify 提示词外置 + registry 模式

---

## 5. 代码清理(✅ RAGFlow 已完全退役)

Phase 1 已删除 RAGFlow handler/service 文件、路由注册、模型字段、指标、错误码、默认值。
前端 Datasets 页仍含 RAGFlow 概念,随 §8 P2 改造清理。

---

## 6. 爬虫运营验收(P1)

- [ ] 批量验证候选源,导入 20–40 个高成功率源
- [ ] 7 天观察:RSS 拉取成功率 > 90%
- [ ] 周源健康报告 + 低质量源自动暂停确认
- [ ] LLM 规则优化闭环实跑验证(依赖 §4.1 rule_optimizer 修复)

---

## 7. 日志中心优化(P1)

- [ ] **全文检索**:Meilisearch 建 `logs` 索引,LogCenter 双写
- [ ] **保留与归档**:`retention_days` + 每日归档
- [ ] **实时日志流**:`/api/logs/stream` SSE
- [ ] **告警规则增强**:threshold / pattern / silence 三类 + 去重
- [ ] **trace_id 跨服务关联**:n8n → Bellkeeper → LLM Proxy 全链路

---

## 8. Bellkeeper 前端(P1–P2)

- [ ] **P1 爬取队列可视化**:任务列表/重试/取消 + 域名健康/限速 + Worker 详情
- [ ] **P1 Vault 预览增强**:Markdown 渲染、frontmatter 折叠、`[[wikilink]]` 跳转
- [ ] **P1 知识问答改造**:多轮上下文、引用展示、SSE 流式
- [ ] **P2 Datasets → Collection**:解耦 RAGFlow 含义
- [ ] **P2 Matrix Admin 补全**:消灭「未实现」toast
- [ ] **P2 Dashboard 重做**:核心指标卡 + 时间序列

### 8.1 Matrix 前端 7→3 页重构(P1)

按 [matrix-platform-overhaul-plan.md](matrix-platform-overhaul-plan.md) T9:
- [ ] 7 页合并为 3 页(Dashboard/Rooms/Settings)
- [ ] API 契约对齐

---

## 9. 工程质量(✅ Phase 7-8 已完成核心)

- [x] 31 Repository 全覆盖(PG 集成测试)
- [x] 核心链路行为测试 + LLM 协议转换测试
- [x] golangci-lint v2 error 清零
- [x] golang-migrate 接入(005-007)
- [ ] API 契约测试或 OpenAPI/类型生成(P2)
- [ ] 配置热重载推广(P2)

---

## 10. 知识问答优化(P2)

- [ ] knowledge_ask 接 Rerank:召回 top-20 → rerank → top-5
- [ ] 上下文压缩:片段独立摘要后拼接
- [ ] 引用结构化:`{file_path, line_range, score, excerpt}` + 前端跳转
- [ ] 多源检索:可选包含 vault / todos
- [ ] 历史会话:`qa_sessions/qa_messages` + Matrix thread 上下文

---

## 11. 运维与可观测性(P2)

- [ ] Prometheus 抓取 + Grafana 看板
- [ ] 容器资源压力检测(cAdvisor)
- [ ] 备份恢复验证
- [ ] n8n 工作流 SLA 指标

---

## 12. 远期(P3)

- [ ] K07 Obsidian 回流端到端验证
- [ ] 文件级权限标签 + 检索过滤
- [ ] 存量知识批量导入
- [ ] Vault 在线编辑
- [ ] 元数据批量操作

**已取消项**:
- ~~智能归档建议~~ — 已被 PKB `pkb-curate` 漏斗全面取代
- ~~Embedding 端点~~ — 无消费方;有真实需求再立项
- ~~n8n 通知链路降层~~ — 维持 B01 模板渲染 + NATS 现状

---

## 13. 里程碑

### 2026-06 内
- [x] Phase 0-10 (v1.0.0) 全部完成
- [ ] §2.1 PKB 存量批跑 + 线上验收;§2.2 原子知识网 Phase A–B
- [ ] §4.1 提示词 P0 修复;§4.2 response_format + eval 骨架
- [ ] §6 第一批新源导入 + 7 天指标

### 2026-07 内
- [ ] §2.2 原子知识网 Phase C–E
- [ ] §3 LLM 运行时验收 + new-api 停服决策
- [ ] §7 日志中心(全文检索 + trace_id)
- [ ] §8 爬取队列前端 + 问答多轮/流式
- [ ] §8.1 Matrix 前端 7→3 页重构

### 2026-08 内
- [ ] §9 API 契约测试
- [ ] §10 问答 Rerank 接入;§11 Prometheus + Grafana
- [ ] §12 按需启动

---

## 维护规则

1. 完成一项:本文打勾/移入「已完成里程碑」表 → STATUS.md「最近主线动作」追加 → 大架构变化同步 ARCHITECTURE.md。
2. 新增任务:按 P0–P3 评估,加入对应章节,不另开新文档;大型计划单独立文档并在 §0 索引。
3. 取消任务:移入 §12「已取消项」加删除线 + 理由,三个月后清理。
4. 计划类文档(\*-PLAN/\*-REVIEW)完成后移 `archive/`,残留转本文。
