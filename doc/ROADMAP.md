# SilkSpool & Bellkeeper 演进规划

> 更新日期: 2026-07-03
>
> 反馈环:[STATUS.md](STATUS.md)(现状)→ ROADMAP(演进)→ git commit(实施)→ STATUS(回写)。已完成计划的原始文档在 [archive/](archive/)。

---

## 0. 优先级总览

> **1.0 重构状态（2026-07-03）**：M1-M5 代码全部完成；剩余 M4-5 Loki 部署 + M6 Grafana 为运维项（spool bundle）。

| 优先级 | 类别 | 摘要 | 估算 |
|--------|------|------|------|
| **P0** | PKB 运行收尾 | 存量 ~308 篇 raw 分批跑完 + 线上验收（数据运营，非代码）(§2.1) | 0.5 天 + 观察 |
| ~~P1~~ | ~~提示词基础设施~~ | ✅ **1.0 M3-4 完成**：response_format + 自修复重试 + golden set + 角色分离 | — |
| ~~P1~~ | ~~LLM Proxy 验收~~ | ✅ **1.0 M2 完成**：llmgateway 独立包 + 进程内直调 + 分层例外清零；🔶 cache 监控 + new-api 停服待决策 | — |
| **P1** | 爬虫运营验收 | 新源批量导入(20–40 个)+ 7 天成功率指标 + 周健康报告(§6) | 持续迭代 |
| ~~P1~~ | ~~日志中心~~ | ✅ **1.0 M4 完成**：pattern 告警 + 归档调度 + trace_id 全链路；全文检索/SSE 交 Loki 外挂 | — |
| ~~P1~~ | ~~前端~~ | ✅ **1.0 M5 完成**：爬取队列页 + 问答 SSE 流式；🔶 Vault 预览增强待 P2 | — |
| ~~P1~~ | ~~Matrix 前端重构~~ | ✅ **1.0 M5 完成**：7→2 页重构（配置归全局 Settings） | — |
| ~~P2~~ | ~~问答优化~~ | ✅ **1.0 M3-5 完成**：Rerank 接入 + 上下文压缩；🔶 引用跳转 + 历史会话待远期 | — |
| **P1** | 🔶 运维部署 | M4-5 Loki+Promtail + M6 Grafana+cAdvisor（spool bundle，非代码） | 1 天 |
| **P2** | 可观测性 | cAdvisor + 备份验证 + n8n SLA（§11） | 2 天 |

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
| PKB 原子知识网 Phase A–D(Tier 1-4):提示词v2+一文多卡+语义去重+分层MOC+身份模型 | 06-17~06-19 | [PKB-ATOMIC-KNOWLEDGE-PLAN.md](PKB-ATOMIC-KNOWLEDGE-PLAN.md) |
| PKB 知识骨架 Phase F–H:骨架+归位+缺口填充+资讯库+晋升闸 | 06-17 | [PKB-KNOWLEDGE-SKELETON-PLAN.md](PKB-KNOWLEDGE-SKELETON-PLAN.md) |
| PKB 自动闭环+域管理:多任务调度+领域CRUD+域状态概览+骨架触发+前端骨架页 | 06-18 | — |
| 知识库模块重做阶段1-2:Matrix agent通电+Web问答不拒答+真多轮+搜索重定位+总览页 | 06-19 | [KNOWLEDGE-MODULE-REVAMP-PLAN.md](KNOWLEDGE-MODULE-REVAMP-PLAN.md) |
| 可靠性加固 Tier 2-3:n8n工作流退役(10个)+K01/K02修复(RagFlow移除+onError+errorWorkflow) | 06-13~06-19 | [reliability-audit-plan.md](reliability-audit-plan.md) |
| LLM Proxy:自适应限流学习器+Kimi Code熔断恢复+4 provider余额拉取 | 06-01~06-12 | [LLM_PROXY_GUIDE.md](LLM_PROXY_GUIDE.md) |
| 爬虫:LLM规则优化闭环+自动暂停低质量源+域名冷却+DequeueFair公平调度 | 06-14~06-15 | ADR-0003 |
| **1.0 重构 M1 解耦地基**:eventbus 一级共享基础设施+Event 契约+删僵尸 commands stream+通知链路迁移+agent 死代码清理 | 07-03 | [STATUS.md](STATUS.md) |
| **1.0 重构 M2 LLM 独立化**:llmgateway 包重组+Gateway 进程内直调(Chat/Rerank)+6 处调用方迁移+分层例外清零(TokenScopeService+LLMAdminService) | 07-03 | [LLM-GATEWAY-API.md](LLM-GATEWAY-API.md) |
| **1.0 重构 M3 KB 链路事件化**:crawl/extract/index 三 worker+llm.job.submit 事件化+域名健康度+提示词外置统一+问答 rerank+golden set 评估 | 07-03 | [STATUS.md](STATUS.md) |
| **1.0 重构 M4 日志补齐**:pattern 告警+CleanOldEntries 调度+trace_id 全链路+goroutine 护栏 | 07-03 | [STATUS.md](STATUS.md) |
| **1.0 重构 M5 前端收敛**:Matrix 7→2 页+爬取队列可视化页+知识问答 SSE 流式 | 07-03 | [STATUS.md](STATUS.md) |

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

### 2.2 原子知识网升级(✅ Phase A–D 已完成,Phase E 核心已实现)

按 [PKB-ATOMIC-KNOWLEDGE-PLAN.md](PKB-ATOMIC-KNOWLEDGE-PLAN.md) 执行:

- [x] Phase A:Tier 1 提示词 v2 + Tier 5 权重(novelty 0→0.15) + validateCard 双栈
- [x] Phase B:Tier 3 一文多卡(`---CARD---`) + 同源双向链接 + concept slug 命名
- [x] Phase C:Tier 2 语义去重(SearchContent) + supplement 归并 + SearchTitles limit=15
- [x] Phase D:Tier 4 分层 MOC(`_index.md` + `topics/` + 关系边 + 快照)
- [x] Phase E:Tier 6 网健康度 audit 核心逻辑(`RunAudit`+`AuditResult`+`Dataview`模板)
- [ ] Phase E 遗留:独立 audit API 端点(`/api/pkb/audit`)

---

## 3. LLM Proxy:运行时验收与 new-api 停服(P1)

- [ ] prompt cache 命中率统计/监控:当前仅提取 cache token 用于计费,未暴露命中率指标
- [x] 自适应限流学习曲线:5 阶段学习器 + token bucket 反馈(`llm_rate_limit_learner.go`)
- [x] Kimi Code 订阅:语义分类 + 30min probe + 5h 熔断自恢复(`kimiCodeProbeLoop`)
- [x] 各 provider 真实余额拉取:DeepSeek/Moonshot/NewAPI/Aliyun + 快照持久化
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

- [ ] `llmclient.ChatRequest` 增加 `MaxTokens`/`ResponseFormat` 结构化字段;`response_format: json_object` proxy 透传(当前 proxy 仅透传 max_tokens)
- [ ] 提示词模板渲染校验:渲染后残留 `{{...}}` 报错
- [ ] 内容级自修复重试:JSON/结构校验失败时带错误回喂一次
- [ ] 提示词 golden set 评估:10–20 篇带预期样本 + `pkb-curate eval`
- [ ] system/user 角色分离:PKB 提示词规则段放 system,利用 prompt cache(knowledge_ask 已分离;PKB score/reconstruct/classify/rule_optimizer 仍全在 user)
- [ ] 提示词管理统一(P2):knowledge_ask / rule_optimizer / classify 提示词外置 + registry 模式(PKB 已完整外置+registry)

---

## 5. 代码清理(✅ RAGFlow 已完全退役)

Phase 1 已删除 RAGFlow handler/service 文件、路由注册、模型字段、指标、错误码、默认值。
Datasets 前端页已退役(路由+导航项已删);`DatasetMapping`/`dataset_id` 后端冻结保留(K01 仍调 `/api/datasets/check-url`)。

---

## 6. 爬虫运营验收(P1)

- [ ] 批量验证候选源,导入 20–40 个高成功率源
- [ ] 7 天观察:RSS 拉取成功率 > 90%
- [ ] 周源健康报告(自动暂停已实现:`ConsecutiveFailures>=5`暂停,`HealthScore>=30`恢复)
- [x] LLM 规则优化闭环:确定性规则优先+LLM兜底+验证提取+评分(`rule_optimizer.go`)

---

## 7. 日志中心优化(P1)

- [ ] **全文检索**:Meilisearch 建 `logs` 索引,LogCenter 双写(当前只写 DB)
- [ ] **保留与归档**:`retention_days` 字段+`CleanOldEntries()`方法已有,但无调度调用;无归档逻辑(只有硬删)
- [ ] **实时日志流**:`/api/logs/stream` SSE
- [ ] **告警规则增强**:当前仅 threshold 类型;需加 pattern(正则)/silence(静默) + 告警去重
- [ ] **trace_id 跨服务关联**:字段+索引+查询已有;缺自动生成和全链路传播机制

---

## 8. Bellkeeper 前端(P1–P2)

- [ ] **P1 爬取队列可视化**:后端 API 完整(Stats/Audit/Domains/ListJobs/Retry/Workers/Cleanup);前端页面/路由/API调用均缺失
- [ ] **P1 Vault 预览增强**:Markdown 渲染、frontmatter 折叠、`[[wikilink]]` 跳转(当前 `<pre>` 纯文本)
- [x] **P1 知识问答改造**:多轮上下文(History 12条)+引用展示(Title/FilePath/Snippet)
- [ ] **P1 知识问答 SSE 流式**:后端同步响应,前端 await 等完整结果
- [x] **P2 Datasets → Collection**:前端页已退役,后端冻结(§5)
- [x] **P2 Matrix Admin 补全**:无「未实现」toast
- [ ] **P2 Dashboard 重做**:指标卡已有;时间序列图表未实现(无图表库)

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
- [ ] 配置热重载推广(P2):LLM Proxy+通知渠道已实现;PKB/RSS 等未推广

---

## 10. 知识问答优化(P2)

- [ ] knowledge_ask 接 Rerank:召回 top-20 → rerank → top-5(Proxy 已有 rerank 路由,AskService 未接入)
- [ ] 上下文压缩:片段独立摘要后拼接(当前直接拼接+截断)
- [ ] 引用结构化:已有 `file_path/snippet`,缺 `line_range/score`;前端无跳转
- [ ] 多源检索:可选包含 vault / todos(仅有基础 layer 过滤)
- [ ] 历史会话:`qa_sessions/qa_messages` + Matrix thread 上下文(当前纯前端传递,无持久化)

---

## 11. 运维与可观测性(P2)

- [ ] Grafana 看板(Prometheus 指标+端点已有:`internal/metrics/metrics.go`;缺 Grafana 容器+dashboard JSON)
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
- [x] §2.2 原子知识网 Phase A–D
- [x] §4.1 提示词 P0 修复
- [x] PKB 骨架 Phase F–H + 自动闭环 + 域管理 + 前端骨架页
- [x] 知识库模块重做阶段1-2(Matrix agent+Web问答+总览页)
- [x] 可靠性加固 Tier 2-3(n8n退役+K01/K02修复)
- [x] 爬取调度重设计(DequeueFair+冷却+配额)
- [ ] §2.1 PKB 存量批跑 + 线上验收
- [ ] §6 第一批新源导入 + 7 天指标

### 2026-07 内（1.0 重构）
- [x] **M1 解耦地基**：eventbus 一级共享基础设施 + Event 契约 + 删僵尸 commands stream + 通知链路迁移 + agent 死代码清理
- [x] **M2 LLM 独立化**：llmgateway 包重组 + Gateway 进程内直调（Chat/Rerank）+ 6 处调用方迁移 + 分层例外清零（TokenScopeService+LLMAdminService）+ LLM-GATEWAY-API.md
- [x] **M3 KB 链路事件化**：crawl/extract/index 三 worker + llm.job.submit 事件化 + 域名健康度（HealthScore/Pause）+ 提示词外置统一（config/prompts+ResponseFormat+system/user 分离+自修复重试）+ 问答 rerank + golden set 评估（pkb-curate eval）
- [x] **M4 日志补齐**（代码）：pattern 告警 + CleanOldEntries 调度 + trace_id 全链路 + goroutine 护栏
- [x] **M5 前端收敛**：Matrix 7→2 页（Dashboard+Console）+ 爬取队列可视化页 + 知识问答 SSE 流式
- [x] §4.2 提示词基础设施（response_format+自修复+golden set+角色分离）✅ 随 M3-4 完成
- [x] §7 日志中心 trace_id 传播 + pattern 告警 + 归档调度 ✅ 随 M4 完成（全文检索/SSE 交 Loki 外挂）
- [x] §8 爬取队列前端 + 问答 SSE 流式 ✅ 随 M5 完成
- [x] §8.1 Matrix 前端 7→3 页重构 ✅ 随 M5 完成（实际 7→2，配置归全局 Settings）
- [x] §10 问答 Rerank 接入 ✅ 随 M3-5 完成
- [ ] 🔶 **M4-5 运维**：Loki+Promtail 部署（spool bundle，非代码）
- [ ] 🔶 **M6 运维**：Grafana + cAdvisor 部署（spool bundle，非代码）
- [ ] §2.2 Phase E 遗留：独立 audit API 端点
- [ ] §3 LLM cache 命中率监控 + new-api 停服决策（base_url 迁移已随 M2 完成）

### 2026-08 内
- [ ] §9 API 契约测试
- [ ] §11 Prometheus + Grafana（M6）
- [ ] §12 按需启动

---

## 维护规则

1. 完成一项:本文打勾/移入「已完成里程碑」表 → STATUS.md「最近主线动作」追加 → 大架构变化同步 ARCHITECTURE.md。
2. 新增任务:按 P0–P3 评估,加入对应章节,不另开新文档;大型计划单独立文档并在 §0 索引。
3. 取消任务:移入 §12「已取消项」加删除线 + 理由,三个月后清理。
4. 计划类文档(\*-PLAN/\*-REVIEW)完成后移 `archive/`,残留转本文。
