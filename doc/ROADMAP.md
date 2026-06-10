# SilkSpool & Bellkeeper 演进规划

> 重新规划日期: 2026-06-10(上一版 2026-05-26,其中 LLM Proxy §2 与 PKB §10 两大 P0/P1 已基本落地,本版将其收缩为「已完成」并重排剩余优先级)
>
> 反馈环:[STATUS.md](STATUS.md)(现状)→ ROADMAP(演进)→ git commit(实施)→ STATUS(回写)。已完成计划的原始文档在 [archive/](archive/)。

---

## 0. 优先级总览

| 优先级 | 类别 | 摘要 | 估算 |
|--------|------|------|------|
| **P0** | PKB 运行收尾 | 存量 ~308 篇 raw 分批跑完 + cron 固化 + 线上验收(§2.1) | 0.5 天 + 观察 |
| **P0** | 提示词 P0 修复 | LLM-PROMPT-AGENT-REVIEW §5.1 六个具体 bug(任务路由死逻辑/字节截断/硬编码模型等)(§4.1) | 0.5–1 天 |
| **P0** | 代码清理 | RAGFlow 退役:剩余 ~8 文件 + n8n 工作流 + bundles 配置(§5) | 2 天 |
| **P1 ⭐** | PKB 原子知识网 | [PKB-ATOMIC-KNOWLEDGE-PLAN.md](PKB-ATOMIC-KNOWLEDGE-PLAN.md) Phase A–E(原子卡/语义去重/分层 MOC/audit)(§2.2) | ~4.75 天 |
| **P1** | 提示词基础设施 | response_format + 模板校验 + 自修复重试 + golden set eval(§4.2) | 1.5–2 天 |
| **P1** | LLM Proxy 验收 | 运行时验证清单 + 调用方迁移 + new-api 停服评估(§3) | 0.5 天 + 7 天观察 |
| **P1** | 爬虫运营验收 | 新源批量导入(20–40 个)+ 7 天成功率指标 + 周健康报告(§6) | 持续迭代 |
| **P1** | 日志中心 | Meili 全文检索 + 告警增强 + trace_id 关联(§7) | 4 天 |
| **P1** | 前端 | 爬取队列可视化页 + Vault 预览增强 + 问答多轮/流式(§8) | 3 天 |
| **P2** | 工程质量 | 测试覆盖 60% + API 契约测试 + golang-migrate(§9) | 5 天 |
| **P2** | 问答优化 | knowledge_ask 接 Rerank + 引用跳转 + 上下文压缩 + 历史会话(§10) | 3 天 |
| **P2** | 可观测性 | Prometheus 抓取 + Grafana 看板(§11) | 3 天 |
| **P3** | 知识库延伸 | K07 验证、权限标签、存量批量导入(§12) | 按需 |
| **P3** | 前端远期 | Vault 在线编辑 + 元数据批量操作(§12) | 5 天 |

---

## 已完成里程碑(2026-05-30 ~ 2026-06-09,详见 STATUS.md)

本版从规划中移除以下已落地大项,残留收尾项整合进下文对应章节:

| 原章节 | 内容 | 落地 | 归档 |
|--------|------|------|------|
| 旧 §2(26.5 天) | LLM Proxy 对标 new-api:Token 体系、定价/计费、真实余额、任务感知分层路由、Coding 三策略、会话粘性、自适应限流学习、错误码熔断、告警聚合、Gemini/Rerank、Kimi Code 接入 | 9 轮迭代 + Tier 0–9 审计修复(05-30~06-01) | 设计细节见 git 历史与 [LLM_PROXY_GUIDE.md](LLM_PROXY_GUIDE.md) |
| 旧 §2.7 + UI 重设计 | LLM 前端 10 页 → 5 页收敛 + 统一凭证模型(消灭凭证表死代码) | 06-07 | [archive/LLM-UI-REDESIGN.md](archive/LLM-UI-REDESIGN.md) |
| 旧 §10(P0 ⭐) | PKB MVP:三层存储隔离、`pkb-curate` 打分分流 + 原子化重构 + digest、提示词外置 config/pkb + registry、llm_jobs 队列、自动调度、n8n 工作流 JSON 纳管 | 06-06~06-09 | [PKB-IMPLEMENTATION.md](PKB-IMPLEMENTATION.md)(保留为活跃实施文档) |
| —(05-26 后新增) | 爬取/标签/RSSHub 优化 P0–P4:域名级限速与健康画像、LLM 规则优化闭环、标签置信度/规范化/tag_source、feed 验证 + RSSHub 参数 | 06-09 五连提交 | [archive/CRAWL-TAGGING-...md](archive/CRAWL-TAGGING-RSSHUB-OPTIMIZATION-PLAN-2026-06-09.md) |
| —(05-26 后新增) | 架构审查 P0/P1 整改:token 权限、路径逃逸、CrawlQueue 原子抢占、Matrix 并发、契约修复、显式迁移、lint 工具链、分层例外登记 | 06-08 审查 → 06-09 整改 | [archive/ARCHITECTURE-REVIEW-2026-06-08.md](archive/ARCHITECTURE-REVIEW-2026-06-08.md) |
| 旧 §3.1 部分 | K02 RSS 解析下沉:RSSFetcher 已内置 Bellkeeper(按源 fetch_interval 调度),CrawlQueue 确立为爬取主链路,n8n 只触发/汇报 | 06-09 | — |

---

## 1. 安全:认证层(✅ 已完成关闭,2026-06-10)

**结论**:生产环境运行在**纯内网**中——虽配有域名但仅内网解析、不出外网,不存在公网暴露面。纯内网环境无需认证,`noauth` 模式即为预期的最终状态,原「恢复认证层」任务不再需要,就此关闭。LLM Token 鉴权(`/api/llm/v1/*`)保持独立不受影响。

---

## 2. PKB:运行收尾 + 原子知识网(P0 收尾 / P1 主线 ⭐)

### 2.1 MVP 运行收尾(P0)

- [ ] 存量 ~308 篇 raw 按预算分批跑完(`pkb-curate`,经 llm_jobs 队列)
- [ ] cron / 调度参数固化(当前自动调度开关已有,确认间隔与预算配置合理)
- [ ] 线上验收:真实样本抽验打分合理性、Obsidian LiveSync 同步范围(raw 不进)、Meili rebuild 结果
- [ ] 新增领域后存量 `archive/` 批量重打分聚合(缺专门命令/流程)
- [ ] `spool n8n export` 冷备进 Git + 线上漂移检测(SilkSpool 侧)

### 2.2 原子知识网升级(P1 ⭐,实施中)

按 [PKB-ATOMIC-KNOWLEDGE-PLAN.md](PKB-ATOMIC-KNOWLEDGE-PLAN.md) 执行,该文档是唯一事实源:

- [ ] Phase A:Tier 1 提示词 v2(原子提取/体系地图/atomic_potential)+ Tier 5 权重(novelty 0→0.15)+ validateCard 双栈(0.5 天)— **已开始**:工作区有未提交实施(score/reconstruct/digest.go + digest.v2.md 等)
- [ ] Phase B:Tier 3 一文多卡 + 同源双向链接 + concept slug 身份(1 天)
- [ ] Phase C:Tier 2 语义去重 + supplement 归并(1 天)
- [ ] Phase D:Tier 4 分层 MOC(`_index.md` + `topics/`)(1.5 天)
- [ ] Phase E:Tier 6 网健康度 audit + Dataview 模板(0.75 天)

---

## 3. LLM Proxy:运行时验收与 new-api 停服(P1)

开发已完成(见已完成里程碑),剩余为**运行时验证**(用户自验)与迁移:

- [ ] prompt cache 命中率:带 `X-Conversation-ID` 连续请求绑定同一渠道,Anthropic cache hit > 80%
- [ ] 自适应限流学习曲线:虚高配置 30min 内回落 / 虚低配置 24h 内上调;学习历史图可见
- [ ] Kimi Code 订阅:403 → 长熔断 → 5h 探测自恢复;配额告警聚合到 Matrix
- [ ] 各 provider 真实余额拉取正常(DeepSeek / Moonshot / new-api 系 / 阿里云 BSS)
- [ ] 调用方 base_url 迁移(Open WebUI 等 → `https://<host>/api/llm/v1` + 专用 Token)
- [ ] 观察 7 天稳定后停 new-api 容器(**暂缓中**,用户仍在使用)+ 清理 bundles 模板

---

## 4. LLM / PKB 提示词工程(P0 修复 + P1 基础设施,新增)

来源:[LLM-PROMPT-AGENT-REVIEW.md](LLM-PROMPT-AGENT-REVIEW.md)(2026-06-10 审查)。

### 4.1 P0 具体修复(§5.1,互相独立可小步提交)

- [ ] 任务路由 token 启发式死逻辑:`DetectTaskType/DetectComplexity` 的 promptTokens 恒传 0(llm_proxy.go:1087/1241/1871),长上下文与复杂度分档永不触发 → 入口粗估 token 传入
- [ ] knowledge_ask `buildContext` 按字节截断中文(knowledge_ask.go:136-150)→ 改 rune,截断长度进配置
- [ ] digest 入选阈值硬编码 7.0(pkb/digest.go:183)→ 用领域 `vault_threshold`
- [ ] rule_optimizer 硬编码 "gpt-4o-mini" + 未注册 TaskType(rule_optimizer.go:223/226)→ 模型/温度进 config,规则优化闭环可能一直静默 400,修复后验证
- [ ] OpenAI→Anthropic 转换丢 `tool_choice`(llm_anthropic.go:66-70)→ 补转换
- [ ] Anthropic 缺省 max_tokens=4096 偏小 → 提高缺省;`stripJSONFence` 双实现去重

### 4.2 P1 基础设施(配合 PKB Phase A 落地)

- [ ] llmclient 支持 `max_tokens` / `response_format: json_object` 并在 proxy 透传(JSON 任务解析失败的主治方案)
- [ ] 提示词模板渲染校验:渲染后残留 `{{...}}` 报错;registry 加载时校验必需占位符
- [ ] 内容级自修复重试:JSON/结构校验失败时带错误回喂一次(优先 reconstruct——最贵调用)
- [ ] 提示词 golden set 评估:10–20 篇带预期样本 + `pkb-curate eval`,支撑 v2 提示词切换验收
- [ ] system/user 角色分离(PKB 三提示词规则段放 system,利用上游 prompt cache)
- [ ] 提示词管理统一(P2):knowledge_ask / rule_optimizer / classify 提示词外置 + `{{var}}` 统一 + registry 模式推广

---

## 5. 代码清理:RAGFlow 退役(P0,延续)

**现状校准(2026-06-10)**:`grep -ri ragflow internal/ web/src/ config/` 仍有 **8 个文件**命中。

- [ ] 删除 RAGFlow handler/service 文件与路由注册、`Services.RagFlow` 装配、`RagFlowConfig`、模型字段、指标、错误码、默认值
- [ ] 前端:Datasets 页解耦 RAGFlow 概念(改为索引分区/Collection 语义)、api 客户端方法、日志模块筛选项
- [ ] n8n 工作流:K06 删除;K07/K08 重写或删除;O01/O02 移除 RagFlow 服务卡片(先 `spool n8n export` 核对线上版本)
- [ ] SilkSpool 配置:`bundles/knowledge/` 整体下线;`bellkeeper-init.sh` 与 `.env` 移除 `RAGFLOW_*`
- 验收:`grep -ri ragflow internal/ web/src/ config/ hosts/keeper/n8n-workflows/` 无输出;构建全绿;启动日志无 RAGFlow

---

## 6. 爬虫运营验收(P1,代码已落地)

代码侧 P0–P4 已完成(归档计划),剩余为运营动作与指标验证:

- [ ] 用 `/api/rss/validate` 批量验证候选源池,导入第一批 20–40 个高成功率源
- [ ] 7 天观察:RSS 拉取成功率 > 90%、正文提取成功率按域名可见
- [ ] 周源健康报告 + 低质量源自动暂停的实际效果确认
- [ ] LLM 规则优化闭环实跑验证:至少 3 个失败域名自动生成 candidate rule 并通过 trial(依赖 §4.1 rule_optimizer 修复)
- [ ] 标签质量抽样:50 篇平均标签数 3–8,噪声 < 10%

---

## 7. 日志中心优化(P1,延续)

- [ ] **全文检索**:复用 Meilisearch 建 `logs` 索引,LogCenter 双写,`/api/logs/search` + 前端搜索框
- [ ] **保留与归档**:`retention_days` + 每日归档 `/mnt/knowledge/logs-archive/`(LLM proxy 日志归档已有,推广到 activity/LogCenter)
- [ ] **实时日志流**:`/api/logs/stream` SSE + 前端实时模式
- [ ] **告警规则增强**:threshold / pattern / silence 三类 + 去重 + 规则编辑器
- [ ] **trace_id 跨服务关联**:n8n 起始生成 → Bellkeeper → LLM Proxy 全链路,前端按 trace 聚合视图

---

## 8. Bellkeeper 前端(P1–P2,延续并校准)

- [ ] **P1 爬取队列可视化**(后端 API 全齐,前端无页面):`/knowledge/queue` 任务列表/重试/取消 + 域名健康/限速状态(对应 06-09 新增的 domains/audit/blocked 端点)+ Worker 详情
- [ ] **P1 Vault 预览增强**:Markdown 渲染、frontmatter 折叠、`[[wikilink]]` 可点跳转(PKB 原子知识网落地后价值放大)、附件内联
- [ ] **P1 知识问答改造**:多轮上下文、引用展示(路径+片段+评分+跳转)、SSE 流式、失败降级提示
- [ ] **P2 Datasets → Collection 改造**:解耦 RAGFlow 含义(随 §5 一起做)
- [ ] **P2 Matrix Admin 补全**:消灭「未实现」toast;命令 DB 动态加载;通知规则可视化编辑
- [ ] **P2 Dashboard 重做**:核心指标卡 + 时间序列 + 活动流

---

## 9. 工程质量(P2,延续 + 架构审查第三阶段残留)

- [ ] 单元/集成测试:`file_ingestion` / `crawl_queue` / `llm_proxy` / `classify` / `pkb` 核心 case;testcontainers 端到端;目标覆盖 60%
- [ ] API 契约测试或 OpenAPI/类型生成(防前后端契约漂移——审查报告第三阶段)
- [ ] 清理假测试,补行为断言(ASSISTANT-GUIDELINES 红线)
- [ ] golang-migrate 接入:`bellkeeper migrate up/down`(已有显式 runtime 表迁移,补版本化与回滚)
- [ ] 配置热重载推广:`system_settings` 通用动态配置 + `/settings` 分类完善

---

## 10. 知识问答优化(P2,校准)

> Rerank 端点(`/api/llm/v1/rerank`)已随 LLM Proxy Tier 7 建成;本节是**消费侧**接入。

- [ ] knowledge_ask 接 Rerank:召回 top-20 → rerank → top-5(现成端点零新增基建);抽样 20 问对比精度
- [ ] 上下文压缩:片段独立摘要后拼接,超阈值才启用
- [ ] 引用结构化:`{file_path, line_range, score, excerpt}` + 前端跳转高亮
- [ ] 多源检索:可选包含 vault(PKB 长青卡片)/ todos
- [ ] 历史会话:`qa_sessions/qa_messages` + Matrix thread 上下文

---

## 11. 运维与可观测性(P2,延续)

- [ ] Prometheus 抓取 + Grafana 看板(bundles/observability/):全局概览 / Bellkeeper 内部 / n8n SLA
- [ ] 容器资源压力检测(cAdvisor)+ O04 阈值告警
- [ ] 备份恢复验证:每月自动恢复演练 + 失败告警
- [ ] n8n 工作流 SLA 指标:executions 推送/拉取 → `workflow_executions` 表 + 看板

---

## 12. 远期(P3)

- [ ] K07 Obsidian 回流端到端验证(或确认删除)
- [ ] 文件级权限标签(`access: public|private|shared`)+ 检索过滤
- [ ] 存量知识批量导入(OWASP/MITRE 等,`bulk_import` 走 PKB 打分管线 + 指定领域)
- [ ] Vault 在线编辑(CodeMirror/Monaco + LiveSync 冲突检测)
- [ ] 元数据批量操作(多选改 tag/移动/删除,二次确认)
- [ ] 多用户与 ACL(待评估必要性)

**已取消项**:
- ~~智能归档建议(旧 §9.2:扫描 working/ 超 30 天文档 LLM 评估沉淀)~~ — 已被 PKB `pkb-curate` 漏斗全面取代(raw 层打分分流即同一目的,且已常态化调度)
- ~~Embedding 端点(旧 §2.5.2)~~ — Meili 主用全文,无消费方;有真实需求再立项(防死代码)
- ~~n8n 通知链路降层(旧 §3.2)~~ — 维持 B01 模板渲染 + NATS 现状,无实际痛点

---

## 13. 里程碑

### 2026-06 内
- [x] §1 认证层 — 已关闭:生产为纯内网环境,无需认证(2026-06-10)
- [ ] §2.1 PKB 存量批跑 + 线上验收;§2.2 原子知识网 Phase A–B
- [ ] §4.1 提示词 P0 修复;§4.2 response_format + eval 骨架
- [ ] §5 RAGFlow 全退役
- [ ] §6 第一批新源导入 + 7 天指标

### 2026-07 内
- [ ] §2.2 原子知识网 Phase C–E
- [ ] §3 LLM 运行时验收 + new-api 停服决策
- [ ] §7 日志中心(全文检索 + trace_id)
- [ ] §8 爬取队列前端 + 问答多轮/流式

### 2026-08 内
- [ ] §9 测试覆盖 + 契约测试 + golang-migrate
- [ ] §10 问答 Rerank 接入;§11 Prometheus + Grafana
- [ ] §12 按需启动

---

## 维护规则

1. 完成一项:本文打勾/移入「已完成里程碑」表 → STATUS.md「最近主线动作」追加 → 大架构变化同步 ARCHITECTURE.md。
2. 新增任务:按 P0–P3 评估,加入对应章节,不另开新文档;大型计划单独立文档并在 §0 索引。
3. 取消任务:移入 §12「已取消项」加删除线 + 理由,三个月后清理。
4. 计划类文档(\*-PLAN/\*-REVIEW)完成后移 `archive/`,残留转本文。
