# Bellkeeper 运行状态

> 最后更新: 2026-07-29 · **1.0 已 GA，系统进入稳定运行状态**
>
> 本文档记录**当前线上真实状态**。历史演进见 [TIMELINE.md](TIMELINE.md)；架构细节见 [ARCHITECTURE.md](ARCHITECTURE.md)；待办见 [ROADMAP.md](ROADMAP.md)。

---

## 主机拓扑（5 机 + TrueNAS 存储）

```
                          Internet / 机场出口
                                 │
              ┌──────────────────┴───────────────────┐
              ▼                                        ▼
   istoreos (192.168.7.1) 网关                 txhk (腾讯云 HK)
   openclash 透明代理 + caddy(TLS)             tuwunel (Matrix) + authelia
   + dnsmasq + tailscaled(子网路由)            + headscale 控制面 + rdp-gateway
              │  内网 192.168.7.0/24                    ▲ 告警/问答
   ┌──────────┼───────────────────────┐                │
   ▼          ▼                        ▼                │
 keeper     silkdata                knowledge ──────────┘
 (.230)     (.231)                  (.220)
 应用层     数据层 + 观测中心栈       firecrawl
```

| 主机 | 地址 | 规格 | 角色 | 主要服务 |
|------|------|------|------|----------|
| **keeper** | 192.168.7.230 | LXC 4核8G | 应用层 | bellkeeper(8090→8080)、n8n(5678)、rsshub、memos |
| **silkdata** | 192.168.7.231 | LXC 8核24G | 数据层 + 观测中心 | PostgreSQL(5432)、Meilisearch(7700)、Redis(6379)、NATS(4222)、CouchDB(5984)；Prometheus(9090)、Loki(3100)、Grafana(3000) |
| **knowledge** | 192.168.7.220 | KVM 16核15G | 抓取引擎 | firecrawl（api/playwright/db/rabbitmq，端点 3002）+ sp-redis |
| **TrueNAS** | 192.168.7.121 | NAS | 存储后端 | 经 NFS4 向 keeper 提供 `/mnt/NAS/data/knowledge`（知识 raw/archive/vault 落盘真源）；不入 tailnet 服务列 |
| **istoreos** | 192.168.7.1 | 路由器 | 网关 | openclash 透明代理、caddy 反代(TLS)、dnsmasq、tailscaled 子网路由（advertise 192.168.7.0/24） |
| **txhk** | 腾讯云 HK | 2核3.6G | Matrix + VPN 控制面 | **tuwunel**(Matrix)、authelia、**headscale 控制面**、caddy、tailscaled、rdp-gateway（全 systemd 裸进程，无 docker） |

**部署形态**：keeper 只跑无状态应用，靠 `extra_hosts` 别名（`sp-*` + 裸名 → 192.168.7.231）连数据层，连接串零硬编码。发版重建 keeper 镜像不影响数据层；数据层可独立备份/快照。知识文件经 **NFS4 从 TrueNAS(192.168.7.121) 挂载**至 keeper 宿主 `/mnt/NAS/data/knowledge`，再 bind 进容器 `/mnt/knowledge`。运维一律走 SilkSpool `spool` CLI（见根 CLAUDE.md §3）。

---

## 可观测性（M6，2026-07-26 上线）

- **中心栈**（silkdata）：Prometheus（15d 保留）+ Loki（7d）+ Grafana，经 `grafana.singll.net`（caddy+TLS，Grafana 自带 admin 登录）。
- **采集端**（三机各一份）：cAdvisor（容器指标）+ Promtail（容器日志推 Loki）。
- **验收**：Prometheus 6/6 targets UP（bellkeeper `/metrics` + cadvisor×3 + loki + self）；Loki 三机日志入库；Grafana 2 看板出数。
- **已知遗留**：keeper 跑 Docker 29，cAdvisor 单容器指标缺失（有 bellkeeper 应用指标 + 机器级指标 + 全量日志兜底）；silkdata/knowledge 单容器指标正常。

---

## 核心数据流

```
RSS(rsshub + 官方直连) ─→ 后端 RSSFetcherService(fetch+enqueue)
  └→ CrawlQueue：DequeueFair 公平轮转 + next_allowed_at 冷却 + recrawl-cooldown 去重 + 域名健康度
       └→ 事件链三 worker：crawl → extract → index
            提取：trafilatura(直连,省流量) 优先 → firecrawl(fetch 引擎优先) 兜底
            落盘 /mnt/knowledge/raw + PG 入库 + Meili 索引
  └→ PKB 漏斗(pkb-curate 五维打分)：≥7 → vault(原子卡) / 4~7 → archive / <4 → discard
       └→ 知识骨架确定性归位 + 缺口填充 + 资讯库 + 晋升闸
  └→ 日报/资讯摘要(DailyReportService) ─→ Matrix 推送
```

- **LLM**：进程内调用方（KB/Agent/日报/分类/规则优化）经 `llmgateway.Gateway` 直调；外部（CLI/n8n）经 `/api/llm/v1/*` OpenAI 兼容 + Token 鉴权。
- **Matrix**：告警推送 + Agent 问答（chat 知识库 / direct 直连大模型两类房间）+ 命令路由。
- **知识真相源**：Obsidian Vault（Markdown），经 CouchDB LiveSync 同步。

---

## 模块成熟度

| 模块 | 状态 | 说明 |
|------|------|------|
| 基础设施 | ✅ 稳定 | 5 机、反代、Headscale VPN、SilkSpool IaC；app/data 分离 |
| 事件总线 | ✅ 1.0 | `internal/eventbus/` NATS JetStream 一级共享；6 stream + Event 契约（ULID+TraceID） |
| Bellkeeper 后端 | ✅ 1.0 | eventbus 贯穿；llmgateway 独立包 + 进程内直调；KB 链路事件化；trace_id 全链路 |
| Bellkeeper 前端 | ✅ 1.0 | SolidJS SPA；Matrix 7→2 页；爬取队列可视化；知识总览/骨架/问答/搜索；问答 SSE 流式 |
| LLM 代理 | ✅ 1.0 | `internal/llmgateway/`；多 provider 路由/熔断/粘性/自适应限流学习/计费/真实余额；Anthropic/Gemini/Rerank 协议转换 |
| LLM 持久队列 | ✅ 1.0 | `llm_jobs` DB 状态机 + `llm.job.submit` 事件驱动 + 原子 claim + recovery 兜底 |
| 个人知识库 PKB | ✅ | `pkb-curate` 漏斗 + 原子化重构 + 知识骨架/双库 + 自动闭环调度 + golden set 评估；提示词外置 |
| 爬取与提取 | ✅ 1.0 | DequeueFair 公平调度 + 域名冷却/健康度 + 三 worker 事件链 + LLM 规则优化 + recrawl 去重 |
| RSS | ✅ 稳定 | feed CRUD/验证 + 后端调度 + **自动暂停恢复（2026-07-27 接线修复，此前 probePausedFeeds 死代码致熔断源不自愈）**；停用源见记忆 |
| Meilisearch 检索 | ✅ 1.0 | `/api/files/search\|ask`；入库即触发索引；问答 rerank 精排 |
| Matrix 控制平面 | ✅ 稳定 | Gateway + Command Router + Agent + 通知聚合去重 + 权限两层 + Admin API |
| Agent 系统 | ✅ MVP | 回合循环 + 9 工具 + Redis 房间会话 + 限速 + 权限分级；进程内直调 Gateway |
| 日报系统 | ✅ 稳定 | 后端驱动 + 并行采集 + n8n 仅触发；进程内直调 Gateway |
| n8n 工作流治理 | ✅ 落地 | 8 活跃工作流（源真相 `internal/n8n_workflows/`）；10 已退役归档 |
| 日志中心 | ✅ 1.0 | threshold + pattern 告警 + 归档调度 + trace_id 全链路；**Loki/Grafana 已外挂（M6）** |
| 可观测性 | ✅ **M6 上线** | Prometheus + Loki + Grafana@silkdata + 三机 cAdvisor/Promtail |
| 认证 | ✅ 无需 | 纯内网 noauth；LLM Token 鉴权独立保留 |
| 测试 / lint | ✅ | 31 Repository 全覆盖（真实 PG）+ 核心链路 + 协议转换 + pkb eval；golangci-lint 零 error |

**1.0 里程碑 M1–M6 全部完成**（详见 [TIMELINE.md](TIMELINE.md)）。

---

## 待决与收尾事项

| 类型 | 事项 | 状态 |
|------|------|------|
| silkdata 收尾 | 稳定 ≥7 天后删 keeper 旧数据卷(~15G)、删停用 LXC 107/108/109、knowledge 降内存 | 🔶 计划中 |
| 观测遗留 | keeper 单容器 CPU/内存指标缺失（Docker 29 + cAdvisor）；可试更新 cadvisor 或改回 overlay2 | 🔶 不影响核心 |
| 成本 | openclash 省流量 VPS 采购；firecrawl fetch 翻转后流量已降 95%+，或不必买 | ⏸️ 观察中 |
| PKB 运营 | **打分重构 Tier 0-8 已上线(2026-07-29)**：相关度硬门+配置化+atomic_potential 修复+领域配额+拒收台账+污染防护(反爬页/NO_CARD)+propose 单轮上限分批；已手动 `propose` 跑一轮消化各域存量待归位（skeleton 加节点填盲区，快照可回滚）。**Tier 9 存量清理已执行(2026-07-29)**：快照瘦身(36→20)、资讯合并(39份5子目录→17日文件)、daily 迁 logs-archive(52)、删 reCAPTCHA 污染卡、离群卡改名对齐。存量 raw 分批跑 + cron 固化；`feature_pkb_auto_fill/feed/propose` 默认关、按需开 | 🔶 数据运营 |
| LLM | prompt cache 命中率监控（new-api 容器已退役下线） | 🔶 待决策 |
| 爬虫运营 | 新源批量导入 + 7 天成功率验收；周源健康报告 | 🔶 持续 |
| RSS 源 | SeebugPaper 上游 TLS 证书链不完整（缺中间 CA）直连验证失败，暂保留停用；余 6 源（HN×3/HF Papers/Krebs/嘶吼）已 07-27 恢复 | 🔶 待修 |

---

## 核心设计决策

| 选择 | 原因 |
|------|------|
| Markdown / Obsidian Vault 为知识真相源 | 数据主权、纯文本、长寿、可手工整理 |
| PKB 用一次性 CLI（pkb-curate）而非常驻 agent | 流程固定的 LLM 批处理；提示词外置 config/pkb 可调方向不改代码 |
| Meilisearch 替代 RAGFlow | 轻量、文件级派生、无需重型向量库 |
| 自建 LLM Proxy 对标 new-api | 任务感知路由/真实余额/限流学习，与 Matrix/日志/配置融合 |
| DB 公平调度替代内存分桶 | 无状态、可观测、公平轮转（ADR-0003） |
| NATS 事件总线解耦模块 | 可追溯（TraceID）、崩溃兜底、多消费者 |
| app/data 分离部署 | 生命周期解耦、备份独立、爆炸半径隔离 |
| n8n 仅做编排 | 重逻辑下沉 Bellkeeper |

---

## 文档导航

- [TIMELINE.md](TIMELINE.md) — 演进时间线与技术栈选型（历史）
- [ARCHITECTURE.md](ARCHITECTURE.md) — 架构、模块职责、数据模型、关键链路
- [ROADMAP.md](ROADMAP.md) — 演进待办
- [API.md](API.md) · [LLM-GATEWAY-API.md](LLM-GATEWAY-API.md) — API 契约
- [DEVELOPMENT-GUIDE.md](DEVELOPMENT-GUIDE.md) · [ASSISTANT-GUIDELINES.md](ASSISTANT-GUIDELINES.md) — 规范与守则
- [ARCHITECTURE-EXCEPTIONS.md](ARCHITECTURE-EXCEPTIONS.md) — 分层例外登记
- [../docs/adr/](../docs/adr/) — 架构决策记录（现行有效）
- [ops/](ops/) — 网络 / 存储 / 应急运维 SOP
- [knowledge-base/](knowledge-base/) — Obsidian/PKB 知识库运营规范、模板与流程
