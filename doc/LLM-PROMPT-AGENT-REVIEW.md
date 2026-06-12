# LLM Proxy 与 PKB 的 Agent / 提示词体系审查

> 生成时间:2026-06-10。基于 main 分支(c41762e)代码实测,所有结论带文件锚点。
> 定位:回答三个问题——**当前架构是什么、细节是什么、还有哪些优化空间**。
> 与 [PKB-ATOMIC-KNOWLEDGE-PLAN.md](PKB-ATOMIC-KNOWLEDGE-PLAN.md) 的关系:该计划已覆盖 PKB 提示词内容层面的升级(原子化/去重/MOC/网状),本文**不重复**其内容;本文聚焦该计划之外的部分——LLM Proxy 侧、提示词工程基础设施、以及横跨各消费者的一致性问题。

---

## 1. 总体结论(TL;DR)

1. **系统里没有真正的 agent loop**。LLM Proxy 是纯透传网关(不注入/不改写提示词,只做协议转换与路由);PKB curator 是**固定步骤的批处理流水线**(score → triage → reconstruct → digest),每步一次 LLM 调用、无多轮自主决策、无工具调用。"agent"能力(tool use)仅以**协议转换透传**的形式存在于 proxy 中,服务于外部 agent 客户端(如 Claude Code)。
2. **提示词管理是三种成熟度并存**:PKB 的「外置 md 文件 + registry 版本切换」最佳;classify 的「Go 内置默认 + 配置覆盖」次之;knowledge_ask / rule_optimizer 的「纯硬编码」最差。值得统一到 PKB 模式。
3. 发现 **6 个计划外的具体问题**(§5.1),其中任务路由的 token 启发式是死逻辑、knowledge_ask 按字节截断中文、digest 阈值硬编码绕过配置,均为低成本可修。
4. 提示词工程基础设施有四块缺失(§5.3):JSON 结构化输出(response_format)、解析失败自修复、模板变量校验、提示词回归评估。

---

## 2. LLM Proxy 架构

### 2.1 请求流

```
外部/内部调用方
  → POST /api/llm/v1/*path(router.go:152-154,LLMTokenAuth 鉴权)
  → handler/llm_proxy.go(透传层,c.JSON 豁免点)
  → LLMProxyService.ProxyRequest / ProxyStreamRequest(llm_proxy.go:1057 / 1850)
      ├─ /v1/rerank → proxyRerank(只路由 provider_type=rerank 渠道,直通)
      ├─ 会话粘性:convMgr 绑定 conversation → 固定渠道(保护上游 prompt cache,llm_proxy.go:1089-1113)
      ├─ 虚拟模型组(pool-*)→ proxyViaGroup → SelectChannel(llm_model_group.go:217)
      └─ 直连渠道匹配 → 逐渠道 failover
  → tryChannel(llm_proxy.go:1347):令牌桶限速 → 协议转换 → 上游 HTTP → 响应回转 → 计费/日志
```

### 2.2 渠道与路由模型

- **Channel**(llm_proxy.go:152):DB 配置(`model.LLMChannel`),带 TokenBucket(RPM/RPD 客户端限速)、熔断器(Health,语义化错误分类决定熔断时长)、EWMA 延迟统计。
- **错误分类**(internal/llm/errors/classifier.go):按状态码+响应体+provider 分类(限流/配额/认证…),驱动差异化熔断与告警。
- **余额感知**(internal/llm/balance/):moonshot/deepseek/aliyun/newapi 多 provider 余额拉取,供 balance_aware 路由策略与快照。
- **自适应限速学习**(llm_rate_limit_learner.go):上游 429 时把学习到的安全 RPM 降到配置值的 ~85%。
- **虚拟模型组**(llm_model_group.go):成员按 tier(free/standard/premium)分层;选择策略 priority_health / weighted / least_latency / balance_aware;sticky binding(taskKey → 渠道,TTL 续期);失败清 sticky 换下一成员。

### 2.3 任务感知路由(TaskRouter)

- 任务类型检测顺序(llm_task_router.go:60-92):`X-Task-Type` 头 → `X-Caller-ID` 启发式(含 classify/summary/qa 关键字)→ 模型名含 coding → promptTokens>32000 → 默认 chat。
- coding 任务再做复杂度检测(token 阈值 + 中英文关键词),驱动 free_first / quality_first / complexity_aware 三种 tier 访问顺序。
- 内部消费者全部显式传 `X-Task-Type` + `X-Caller-ID`(经 llmclient,client.go:83-88),启发式只对外部调用方兜底。

### 2.4 协议转换(proxy 中唯一触碰"提示词"的地方)

Proxy **不注入任何 system prompt、不改写消息内容**,只做格式转换:

| 上游类型 | 请求转换 | 响应转换 | 备注 |
|---------|---------|---------|------|
| openai 兼容 | 仅剥模型后缀(reasoning/thinking,llm_proxy.go:2283) | 无 | 直通 |
| anthropic | OpenAI→`/v1/messages`(llm_anthropic.go:14):system 消息合并进 `system` 字段、tool_calls/tool result 双向转换、stop→stop_sequences、**max_tokens 缺省补 4096** | Anthropic→OpenAI(含 SSE 流式逐事件转换 AnthropicSSEConverter) | 计费在转换前提取 cache_read_input_tokens |
| gemini | OpenAI→`:generateContent`(converter/gemini.go) | Gemini→OpenAI | 路径含真实模型名 |
| rerank | 直通(Cohere/Jina schema) | 直通 | 无会话语义 |

**Agent 相关**:tool use(`tools`/`tool_calls`/`tool` role)在 OpenAI↔Anthropic 间完整转换(llm_anthropic.go:454-551),即外部 agent 客户端经 proxy 调 Claude 渠道可正常走工具循环——agent 循环本身在客户端,proxy 只是翻译官。

### 2.5 内部调用基础设施

- **llmclient**(internal/llmclient/client.go):全项目统一的内部调用 SDK。请求体只支持 `model/messages/temperature` 三个字段(见 §5.3 缺口);带 CallerID/TaskType 头;HTTPError 携带 Retry-After;`RetryDelay`/`IsRetryable` 共享退避逻辑。
- **LLM Job Queue**(llm_job_queue.go):DB 持久队列 + worker(默认 1 worker 慢跑免费池),EnqueueChat 带优先级与幂等键(sha256),`Wait` 轮询取结果。classify、knowledge_ask、PKB 都支持「队列模式优先、直连兜底」双轨。

---

## 3. 内部 LLM 消费者与提示词盘点

| 消费者 | 提示词位置 | 模板方式 | 模型 | 温度 | 输出解析 | 成熟度 |
|--------|-----------|---------|------|------|---------|--------|
| PKB curate/digest(internal/pkb/) | **外置** config/pkb/prompts/*.md + registry.yaml 版本切换 | `{{var}}` ReplaceAll | domains.yaml(pool-summary / pool-pkb) | domains.yaml 按任务分设 | JSON(score)/结构校验(validateCard/validateDigest)+ 剥围栏 | ★★★ |
| classify(service/classify.go:57-85) | Go 常量默认 + `cfg.Prompt` 可覆盖 | `fmt.Sprintf` 位置 `%s`×3 | config(Qwen/Qwen3-8B) | config(0.3) | JSON + 剥围栏,失败即 error | ★★ |
| knowledge_ask(service/knowledge_ask.go:157-161) | **硬编码** Go 字符串 | fmt.Sprintf 拼接 | **硬编码** "pool-chat-balanced" | **硬编码** 0.3 | 纯文本(无解析) | ★ |
| rule_optimizer(service/rule_optimizer.go:199-220) | **硬编码** 英文字符串 | fmt.Sprintf | **硬编码** "gpt-4o-mini" | **硬编码** 0.3 | 首尾大括号截取 + JSON | ★ |
| tag_normalize(service/tag_normalize.go) | 不调 LLM(纯规则归一化) | — | — | — | — | — |

PKB 的提示词契约细节(score 五维打分+枚举 content_type+JSON-only 输出;reconstruct 四固定章节+wikilink 白名单;digest 五章节+候选卡白名单)与代码侧护栏(clamp、pruneWikilinks 防死链、validateCard 结构校验、budget/retry/幂等账本)见 [PKB-IMPLEMENTATION.md](PKB-IMPLEMENTATION.md) §4.4,此处不展开。

### 3.1 PKB 调用链的特殊性

pkb-curate 是**进程外 CLI**:不直接进库,而是经 localhost HTTP 调自家 server(pkb/client.go),从而复用 LLM Proxy 的熔断/限速/计费与 files API。LLM 调用双轨:`--queue` 经持久 llm_jobs 队列(幂等键 `pkb:sha256(...)`,等待超时按重试参数推导、1h–24h 夹紧,pkb/llm_retry.go:107-124);直连模式自带指数退避重试。预算护栏:每轮 score≤50 / reconstruct≤5 / digest≤3 次,超限把剩余文章留到下轮。

### 3.2 「是否有 agent」的明确回答

- **Proxy 内**:无。无编排、无多轮、无 prompt 注入;tool use 只是协议翻译。
- **PKB 内**:无。是确定性流水线,LLM 每步单轮调用,输出不合格直接失败该篇(不会拿错误信息回炉重试同一提示词);"自我修正"只存在于**重试限流错误**层面,不存在于**内容层面**。
- **rule_optimizer**:最接近"agent 味"的闭环——生成规则 → 试用打分(QualityScore≥0.6)→ 不达标重试/拒绝,但每轮 LLM 调用相互独立,失败样本未回喂提示词,属于"生成-验证循环"而非 agent。

---

## 4. 与既有计划的边界

[PKB-ATOMIC-KNOWLEDGE-PLAN.md](PKB-ATOMIC-KNOWLEDGE-PLAN.md) 已诊断并规划了 PKB 提示词**内容层面**的全部主要问题(P1 总结式卡片、P2 语义去重、P3 digest 流水账、P4 novelty 权重、P5-P8 网状结构),含 v2 提示词全文与 Tier 1-6 实施路线。**本文不重复这些**;下文 §5 全部是该计划未覆盖的发现。

---

## 5. 优化空间(计划外新发现)

### 5.1 具体问题(建议优先修,多数是小改动)

| # | 问题 | 锚点 | 影响 | 修法 |
|---|------|------|------|------|
| 1 | **任务路由 token 启发式是死逻辑**:`DetectTaskType(..., 0)` 与 `DetectComplexity(nil, body, 0)` 的 promptTokens 恒传 0,`>32000 → long_context` 与 token 复杂度分档**永远不触发**,只剩关键词匹配生效 | llm_proxy.go:1087、1241、1871;llm_task_router.go:85-90、123-128 | 外部长上下文请求不会被路由到长上下文池;coding 复杂度只靠关键词 | 在 ProxyRequest 入口对 body 做一次粗估(字符数/4 即可),传入两处 |
| 2 | **knowledge_ask 按字节截断中文**:`buildContext` 用 `len()`/`content[:6000]`,UTF-8 中文会截出半个字符;PKB 同类逻辑用 `truncateRunes`(正确) | knowledge_ask.go:136-150 vs pkb/score.go:106 | 上下文尾部出现乱码喂给 LLM | 改 rune 截断;顺手把 6000 提为配置 |
| 3 | **digest 入选阈值硬编码 7.0**,不读 domains.yaml 的 `vault_threshold`(可按领域配 7.0/8.0) | pkb/digest.go:183 | news 领域(阈值 8.0)的 7.x 分卡也会进 digest,与配置语义不一致 | 用 `domain.VaultThresholdOr(defaults)` |
| 4 | **rule_optimizer 模型硬编码 "gpt-4o-mini"**:渠道/组里未必存在该名(grep 配置无此模型),路由可能直接 400 `no channel available`;TaskType "rule_generation" 也不在 TaskRouter 枚举内 | rule_optimizer.go:223、226 | 规则优化闭环可能静默失败(错误只进日志) | 模型与温度进 config(对齐 classify 的做法),TaskType 用既有枚举(如 classify) |
| 5 | **OpenAI→Anthropic 转换丢 `tool_choice`**:直通字段仅 model/stream/temperature/top_p/metadata,tools 有转换但 tool_choice 被丢弃 | llm_anthropic.go:66-70 | 强制工具调用(`tool_choice: required/具体函数`)的 agent 客户端经 Claude 渠道行为退化为 auto | 转换 tool_choice → Anthropic `tool_choice` 格式 |
| 6 | **Anthropic 缺省 max_tokens=4096 偏小**:调用方不传 max_tokens 时长输出(如 PKB 重构卡片如果路由到 Claude 渠道)会被静默截断 | llm_anthropic.go:58-63 | 截断不可见,validateCard 会报"缺章节"但根因难查 | 缺省提高(如 8192)或按模型上限取;同时见 §5.3-① 让调用方能传 |

另有两个观察项(不一定是 bug,建议确认):anthropic 分支不走 `parseModelSuffixes`(llm_proxy.go:1389-1397),模型后缀机制对 Claude 渠道无效,若有意可注释说明;`stripJSONFence` 在 classify.go:181-190 与 pkb/score.go:80-90 重复实现(后者注释自承"抄 classify.go"),应提到公共包。

### 5.2 提示词管理一致性(把三种风格统一成一种)

现状三种成熟度并存(§3 表)。建议以 PKB 模式为目标统一:

1. **所有内部提示词外置**到 `config/prompts/`(或并入 config/pkb 同级),knowledge_ask、rule_optimizer、classify 的提示词改为文件加载 + 内置默认兜底——classify 已有 `cfg.Prompt` 覆盖机制,迁移成本最低。
2. **模板语法统一为 `{{var}}`**:classify 的 `fmt.Sprintf` 位置 `%s` 模板对用户自定义极不友好(必须恰好 3 个 `%s` 且顺序正确,写错即运行时格式错乱)。
3. **模型/温度全部进配置**:消灭 "pool-chat-balanced"、"gpt-4o-mini"、0.3 等散落硬编码(这也是 CLAUDE.md §2.2「硬编码值集中到 defaults」的存量违反项)。
4. **registry 模式推广**:PKB 的 registry.yaml「新增 v2 文件 + 切指针、旧版可回滚」是好实践,统一后其他消费者免费获得版本管理。

### 5.3 提示词工程基础设施缺口(中期,按收益排序)

① **llmclient 支持结构化输出**:`ChatRequest` 仅 model/messages/temperature(llmclient/client.go:28-32),不支持 `max_tokens`、`response_format: {type: json_object}`、`top_p`。当前所有 JSON 任务(classify/score/rule)靠提示词约定"只输出 JSON"+ 剥围栏,这是 JSON 解析失败的主要来源。加上 response_format 并在 proxy 侧透传(openai 兼容渠道原生支持;anthropic 转换可忽略该字段),解析失败率会显著下降。**这是性价比最高的一项**。

② **内容级自修复重试**:目前 JSON/结构校验失败 = 整篇失败(score 解析失败丢弃该篇;validateCard 失败丢弃重构结果,已花的 LLM 费用作废)。建议加一轮带错误反馈的重试:把解析错误 + 原始输出回喂("你上次输出的 JSON 有以下错误…请只输出修正后的 JSON"),用更低温度。一次修复重试通常可挽回大部分格式失败,对 reconstruct(最贵的调用)收益最大。

③ **模板变量校验**:`strings.ReplaceAll` 渲染是静默的——提示词文件里 `{{titel}}` 拼错或代码漏传变量,残留的 `{{xxx}}` 会原样发给 LLM,无任何告警。建议渲染后 grep 残留 `{{.*}}` 即报错(几行代码),并校验 registry 指向的文件包含全部必需占位符(可在 NewCurator 加载时做)。

④ **提示词回归评估(golden set)**:registry 支持版本切换,但「v2 是否比 v1 好」目前只能实跑感受。建议留一组 10-20 篇带人工预期(分数区间/期望决策/期望卡片数)的样本文章,`pkb-curate eval --prompt score.v2.md` 跑对照输出差异报告。原子知识计划 Phase A 的 `--dry-run` 验收正好需要这个能力支撑,可顺路落地。

⑤ **system/user 角色分离**:PKB 三个提示词把"角色设定+规则+数据"全塞 user 消息(ChatCompletion 的 systemPrompt 恒传 "",pkb/score.go:62、digest.go:229)。把规则部分放 system、数据放 user,对支持 prompt cache 的上游(deepseek/moonshot/claude)可让规则段命中缓存,批量打分场景省钱且更稳。

### 5.4 Proxy 侧改进(低优先级)

- **流式/非流式双实现**:`tryChannel` 与 `tryChannelStream`(llm_proxy.go:1347/1946)各自维护转换、限速、计费逻辑,新增 provider 要改两处;可抽公共前置(选路+转换+头处理)收敛。
- **caller-id 启发式可裁剪**:内部消费者已全部显式传 X-Task-Type,DetectTaskType 第 2 步的字符串包含匹配(`Contains(callerID, "ask")` 等)误判面大于收益,可在确认无外部依赖后简化。
- **knowledge_ask 的 RAG 质量**:每文件只取 1 个 snippet、总上下文 6000 字符、无 rerank——而 proxy 明明已支持 /v1/rerank 渠道(Tier 7 已建好)。把检索结果过一遍 rerank 再拼上下文,是现成能力的零新增组合。

---

## 6. 建议路线

| 优先级 | 内容 | 预估 |
|--------|------|------|
| P0(随手修) | §5.1 #1-#6 六个具体问题 + stripJSONFence 去重 | 0.5-1 天,互相独立可逐个小步提交 |
| P1(与原子知识计划 Phase A 同期) | §5.3-①(response_format)+ ③(模板校验)+ ④(eval 骨架,直接服务 Phase A 验收) | 1 天 |
| P1.5 | §5.3-②(自修复重试,优先 reconstruct)+ ⑤(system/user 分离) | 0.5-1 天 |
| P2 | §5.2 提示词管理统一(消费者逐个迁移)+ §5.4 proxy 侧收敛 | 1-2 天,可拆散穿插 |

PKB 内容层面的演进(原子卡/去重/MOC/audit)继续按 [PKB-ATOMIC-KNOWLEDGE-PLAN.md](PKB-ATOMIC-KNOWLEDGE-PLAN.md) 的 Tier/Phase 走,本文 P1 项建议**先于或伴随**其 Phase A 落地——eval 与模板校验就位后,v2 提示词的切换风险会小得多。

---

## 附录:关键文件索引

| 关注点 | 文件 |
|--------|------|
| Proxy 路由主流程 | internal/service/llm_proxy.go(ProxyRequest:1057 / tryChannel:1347 / ProxyStreamRequest:1850) |
| 任务路由 | internal/service/llm_task_router.go |
| 模型组/粘性/分层 | internal/service/llm_model_group.go |
| Anthropic 协议转换 | internal/service/llm_anthropic.go |
| Gemini 协议转换 | internal/llm/converter/gemini.go |
| 错误分类/熔断语义 | internal/llm/errors/classifier.go |
| 内部调用 SDK | internal/llmclient/client.go |
| 持久 LLM 队列 | internal/service/llm_job_queue.go |
| PKB 编排/打分/重构/综述 | internal/pkb/{curator,score,reconstruct,digest}.go |
| PKB 提示词 + registry | config/pkb/prompts/ |
| PKB 领域/权重/预算 | config/pkb/domains.yaml |
| 分类提示词(内置默认) | internal/service/classify.go:57-85 |
| 问答提示词(硬编码) | internal/service/knowledge_ask.go:157-161 |
| 规则优化提示词(硬编码) | internal/service/rule_optimizer.go:199-220 |
