# LLM Proxy 使用与配置指南

> 更新:2026-06-10(对齐 Tier 0–9 整改 + LLM UI 重设计后的现状)。
> Bellkeeper 内置 LLM Proxy,提供 OpenAI 兼容的统一入口,支持多渠道路由、虚拟模型组、任务感知分层路由、Token 鉴权与计费、熔断、会话粘性、自适应限流学习、真实余额同步、协议转换(Anthropic / Gemini / Rerank)。
> 深入的架构与提示词分析见 [LLM-PROMPT-AGENT-REVIEW.md](LLM-PROMPT-AGENT-REVIEW.md)。

## 架构概览

```
调用方(内部服务 / n8n / Claude Code / Open WebUI 等)
  ↓  Authorization: Bearer <llm-token>(LLMTokenAuth 鉴权)
Bellkeeper LLM Proxy (/api/llm/v1/*)
  ├─ 任务类型检测(X-Task-Type 头 / X-Caller-ID 启发式 / 模型名)
  ├─ 会话粘性(X-Conversation-ID 绑定渠道,保护 prompt cache)
  ├─ 虚拟模型组解析(pool-* → 真实渠道,任务感知 + tier 分层 + 多策略)
  ├─ 协议转换(OpenAI ↔ Anthropic / Gemini;/v1/rerank 直通)
  ├─ 令牌桶限速(RPM/RPD,自适应限流学习覆盖配置值)
  ├─ 错误码语义化熔断(配额/认证/限流分类,差异化熔断时长)
  ├─ 计费(定价表 + cached tokens 折扣,micro-cent 精度)
  └─ 告警聚合(5min 合并 + 1h 去重 → Matrix ops)
  ↓
上游 LLM API(SiliconFlow / 百炼 / DeepSeek / Moonshot / Kimi Code / new-api 系 / Gemini ...)
```

## 配置方式:DB 动态配置(不再用 YAML 渠道清单)

**渠道与模型组全部存 DB、通过 Web UI 管理、热重载生效**;`bellkeeper.yaml` 的 `llm_proxy.channels` 仅作首次启动 seed。日常增删渠道**不需要**改 YAML、不需要重建镜像。

- 渠道管理:Web `/llm/channels`(就地编辑 + 统一凭证区)或 `CRUD /api/llm/config/channels`
- 模型组管理:Web `/llm/groups-routing` 或 `CRUD /api/llm/config/groups`
- 改完自动 reload;手动触发:`POST /api/llm/reload`

### 凭证管理

渠道 API Key 支持两种来源,统一在渠道编辑界面配置:

1. **环境变量引用**:填环境变量名(如 `LLM_DEEPSEEK_API_KEY`),变量写在 `SilkSpool/hosts/keeper/.env` 并经 `bellkeeper-init.sh` export。新增变量后:`spool sync push keeper` + `spool restart keeper bellkeeper`。
2. **加密凭证表**:直接粘贴 key,AES-256-GCM 加密落库(密钥来自 `BELLKEEPER_CREDENTIAL_KEY`),用于余额查询等非转发用途的额外凭证(如 new-api session)。

## Token 体系(调用方鉴权)

`/api/llm/v1/*` 不走普通 `/api` 认证,使用专用 LLM Token(`llm_tokens` 表):

- Web `/llm/usage-billing` 的 Token 区管理;创建后弹窗一次性展示完整 key
- 每 Token 可配:允许模型白名单、日请求/Token 配额、月成本配额、过期时间
- 首次启动自动从 `server.api_key` seed 一条 default token,内部服务无感迁移
- 配额超 80%/95%/100% 三档告警 → Matrix

## 虚拟模型组与任务感知路由

调用方把 `model` 写成组名(如 `pool-summary`),Proxy 在组内做健康感知调度:

| 机制 | 说明 |
|------|------|
| 选择策略 | `priority_health` / `weighted` / `least_latency` / `balance_aware`(按真实余额) / 任务感知 tier 分层 |
| 任务类型 | `X-Task-Type` 头显式声明(coding/classify/summary/qa/long_context/chat),未声明按 caller-id/模型名启发式 |
| Coding 分层 | 成员按 free/standard/premium tier;策略 `free_first` / `quality_first` / `complexity_aware`(默认,按 prompt 复杂度选起点) |
| 粘性 | taskKey(`X-Task-Key` 或 caller+model)绑定渠道,TTL 续期;失败清绑换下一成员 |
| 会话粘性 | `X-Conversation-ID`(或 cache_control 隐式哈希)绑定渠道,保护 prompt cache;`X-Allow-Channel-Switch: true` 可跳过 |

当前主要组(以 `/llm/groups-routing` 实际为准):`pool-chat-free` / `pool-chat-balanced` / `pool-summary`(分类、打分等批量任务)/ `pool-pkb`(kimi-k2.6,PKB 重构/综述用强模型)。

### 调用示例

```bash
curl -X POST https://<host>/api/llm/v1/chat/completions \
  -H "Authorization: Bearer sk-bk-xxx" \
  -H "X-Caller-ID: my-service" \
  -H "X-Task-Type: summary" \
  -d '{"model": "pool-summary", "messages": [{"role": "user", "content": "..."}]}'
```

内部 Go 服务请使用 `internal/llmclient`(自动带 CallerID/TaskType 头),批处理任务优先经 `llm_jobs` 持久队列(`LLMJobQueueService.EnqueueChat`,带幂等键与长退避重试)。

## 协议转换

| 上游 provider_type | 行为 |
|------|------|
| openai(默认) | 直通;剥模型后缀(`-high/-low` reasoning effort、`-thinking-N`) |
| anthropic | OpenAI 请求 ↔ `/v1/messages` 双向转换,含 tool use 与 SSE 流式;system 消息合并;max_tokens 缺省 4096 |
| gemini | OpenAI ↔ `:generateContent` 转换 |
| rerank | `POST /api/llm/v1/rerank` 直通(Cohere/Jina schema),仅路由 rerank 渠道 |

## 熔断与限流

- **错误码语义化熔断**(`internal/llm/errors/classifier.go`):配额耗尽(长熔断至刷新周期)/ 认证失效(立即+告警)/ 限流(短熔断按 Retry-After)/ 5xx(指数退避)分类处理;Kimi Code 订阅无余额接口,靠 403/429 检测 + 5h 探测自恢复
- **自适应限流学习**:观察 429 自动把学习安全 RPM 降到配置值 ~85%,持久化 `llm_model_rate_limits`,可 UI 锁定/重置
- 客户端令牌桶 + 上游 429 指数退避重试

## 计费与余额

- 定价表 `llm_model_pricing`(input/output/cached 单价,micro-cent 精度),流式响应同样统计 usage
- 真实余额同步:DeepSeek / Moonshot / new-api 系 / 阿里云 BSS 四类 provider,30min 拉取 + 快照历史,供 `balance_aware` 策略与 Dashboard 对比
- 用量聚合:`GET /api/llm/usage?group_by=token|channel|model|date`

## Web UI(2026-06-07 重设计后,5 页)

| 页面 | 内容 |
|------|------|
| `/llm` 总览 | KPI、估算 vs 真实余额、趋势、告警 |
| `/llm/channels` | 渠道运行时 + 就地配置编辑 + 统一凭证 + 余额徽章 + 熔断重置 |
| `/llm/groups-routing` | 模型组运行时 + 配置 + 任务路由/Coding 策略 + 粘性管理 |
| `/llm/usage-billing` | Token 管理 + 定价 + 用量计费切片 |
| `/llm/logs-alerts` | 请求日志(cost/cached 列)+ 告警历史 |

## 常用管理接口

| 接口 | 说明 |
|------|------|
| `GET /api/llm/channels/status` / `/health` / `/groups/status` | 运行时状态 |
| `POST /api/llm/channels/:name/reset` | 重置熔断 |
| `DELETE /api/llm/groups/:name/sticky` | 清粘性绑定 |
| `GET /api/llm/stats?hours=24` / `/logs` / `/rate-limit-events` / `/alerts` | 统计与日志 |
| `GET /api/llm/rate-limits`,`POST .../lock|reset` | 限流学习状态管理 |
| `GET /api/llm/balances`,`POST /api/llm/balances/refresh` | 真实余额 |
| `GET/DELETE /api/llm/conversations[/:id]` | 会话粘性绑定管理 |
| `CRUD /api/llm/tokens`,`POST /api/llm/tokens/:id/regenerate` | Token 管理 |
| `CRUD /api/llm/pricing` | 定价管理 |
| `POST /api/llm/reload` | 配置热重载 |

## 注意事项

- 渠道配置在 DB,**改 YAML 不会影响已运行实例的渠道清单**(仅 seed)
- `/api/llm/anthropic/*` 这样的独立 Anthropic 入口**不存在**——Anthropic 是渠道侧协议转换,入口统一为 OpenAI 兼容的 `/api/llm/v1/*`
- 同一模型可配多渠道,按策略与健康度自动 failover
- 禁止 `docker compose down`;部署遵循 CLAUDE.md §4.2(`spool bundle keeper service keeper bellkeeper up`)
