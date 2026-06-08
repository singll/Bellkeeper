# PKB 个人知识库 — 实施文档（最小 CLI 批处理 · 提示词外置 · MVP 优先）

> **定位**：本文件是 [ROADMAP.md](ROADMAP.md) **§10「个人知识库成熟化」** 的实施细化。§10 是**需求与蓝图**（做什么、为什么），本文件是**实施方案**（怎么做、改哪些文件、分几步、如何验收）。需求变更改 §10，落地细节改本文件。
>
> **适用分支**：`fix/llm-proxy-p1-audit`（或后续专开的 `feat/pkb-mvp`）。
>
> **本文件基于 2026-06-05 的代码实测调研**，所有「现状」结论均带 `文件:行号` 锚点。**实施时以代码为准**——若锚点与代码漂移，先核对再动手。

---

## ⚠️ 方向沿革（两次转向，诚实记录）

知识库的**设计**（漏斗筛选 → 原子化重构 → 体系化合成的蓝图）三版一致，变的只是**由谁来执行维护编排**：

| 版本 | 维护 runner | 调方向入口 | 为什么放弃 |
|------|------------|-----------|-----------|
| v1（已废弃） | Bellkeeper 内置 `EvaluatorService`/`ReconstructService` **常驻 Go worker** | 改 `.go` 代码 + 重启 | 常驻进程重、prompt 硬编码、改方向要重编重启 |
| v2（已废弃） | **Claude Agent（Claude Code 客户端）+ 可编辑 Skill** | 编辑 `.claude/skills/pkb-curator/` | **绑死 Claude Code + Anthropic**；用户明确不用 Claude Code 做自动化管理 agent |
| **v3（本文件 · 现行）** | **Bellkeeper 一次性 CLI 子命令 `pkb-curate`**（cron/手动触发，跑完即退） | 编辑 `config/pkb/`（提示词 + 领域 + 阈值，git 版本化） | — |

**v2 → v3 的转向理由（2026-06-05 用户决策）**：
1. **不用 Claude Code 当自动化 agent**（用户明确）——v2 把编排寄生在 Claude Code 客户端里，绑死 Anthropic、必须有人在 Claude Code 里触发，不适合无人值守。
2. **任务本质不需要"自主 agent"**——「列举 → 读 → 打分 → 分流 →（高分）重构 → 落盘 → 重索引」是**流程固定**的批处理，LLM 只做"打分""重构"两件确定的事，没有"让模型自主决定调哪个工具、循环几次"的开放式 agentic 需求。用 n8n AI Agent 节点 / LangGraph / CrewAI 这类自主 agent 框架是「**引入重工具只用一小点**」，违反 §0.1。
3. **最小落点 = 一段读外置提示词的脚本化 pipeline**——代码即可实现，且复用 Bellkeeper 现成能力最多、新增实体最少。

> **v3 不是回退到 v1**：v1 被否的三点（常驻 worker / prompt 硬编码 / 改方向要重启）v3 全部规避——`pkb-curate` 跑完即退（非常驻，无后台 goroutine）、prompt 外置成 `config/pkb/prompts/*.md`、改提示词文件下次运行即生效（不重编不重启）。v3 保留了 v2 转向的**内核**（prompt 外置可编辑、AI 主导、不增 service/表），只是把 runner 从"Claude Code Skill"换成"Bellkeeper 自家 CLI 命令"。
>
> 旧版若需考古，见 git 历史中本文件的前两版。

---

## 0. 核心原则（最高约束 · 一切实施决策的宪法）

> 这一节是本次转向的**指导方向**，由用户明确要求写入。后续每一个「要不要做某功能 / 要不要引入某工具」的判断，**先过这 6 条**。与下文任何具体方案冲突时，**以本节为准**。

**0.1 如非必要，勿增实体（奥卡姆剃刀）** —— 最高原则。
能用现有能力组合解决的，绝不新增模块/表/端点/服务。每加一个「实体」（代码、依赖、工具、配置项）都会推高系统复杂度，复杂度是知识库长期可维护性的头号敌人。新增前先自问：**不加它，链路是否仍能跑通？** 能，就不加。
> 推论（v3 关键）：「最小」= **新增的实体/部署/运行时/维护面最少**，不是「代码行数最少」。一个复用现成 Go 二进制的 CLI 子命令，胜过一个行数更少、却多一个独立运行时/部署单元的脚本。

**0.2 优先用成熟开源工具，自研是最后手段** —— 自研与引入都要过「评估门槛」。
- **查看 / 图谱 / 双链 / 关联 / 检索可视化** → 一律交给 **Obsidian**（把 vault 拉到本地用），**不在 Bellkeeper 里自研**这些能力。
- 需要某能力时，决策顺序：① 现有能力能否组合满足 → ② 成熟开源工具能否直接用（Obsidian / Meilisearch / n8n / spool / LLM Proxy 已在手）→ ③ 实在没有，才**完备评估后**自研。
- **引入新开源工具/库也要过评估**（见 §0.7 评估清单）。目标是**工具集尽量小**——工具越多，运行维护的复杂度越爆炸。

**0.3 AI 主导维护，人只调方向** —— 知识库由大模型（经 LLM Proxy 调用，provider 无关）日常维护；用户的角色是**调方向**，不是手动整理。
「调方向」的唯一入口 = 编辑 `config/pkb/`（提示词 / 领域 / 阈值）。人不碰具体某篇文章的去留，只调**策略**。

**0.4 落地为 MD 文件，Obsidian 是查看与知识网络的中心** —— 知识库的最终形态是一堆相互 `[[wikilink]]` 的 Markdown 文件，本身就是合法 Obsidian Vault。
检索（Meili）、维护（`pkb-curate`）都围绕这堆文件转；**看、图谱、关联用 Obsidian**。Bellkeeper 不做"知识库浏览器 UI"（见 §3.3 文件浏览器下线）。

**0.5 MVP 优先，渐进迭代，骨架不大改** —— 先把最小可用闭环跑通、真正用起来，再让**真实痛点**驱动下一步迭代（而非预先过度建设）。
基础框架（三层存储 / CLI+提示词外置 / 原子能力边界）一次定对，使后续增量「可插入、不重构」。

**0.6 可增长、可维护、随时可调** —— 新增一个领域、调一次打分权重、换一个重构模板、换一个驱动模型，都应是「改一个文件」的小动作，不触发连锁改造。这是「骨架一次定对」的检验标准。

**0.7 新增/引入的评估清单（自研或引入开源工具前逐条过）**：
```
□ 现有能力（Obsidian/Meili/n8n/spool/LLM Proxy/Bellkeeper 现有 API+CLI）组合能否满足？
□ 若引入开源工具：它是否成熟、活跃、低维护？是否会与现有工具职责重叠？
□ 若自研：它是否「必须」由我们写才能接入知识库？工作量与收益是否匹配？
□ 它会新增多少长期维护负担（部署、升级、监控、故障面）？
□ 不做它，MVP / 当前迭代是否仍能闭环？（能 → 暂不做，记入 §6 路线图待触发）
```

---

## 1. 关键决策记录（2026-06-05 与用户对齐）

| # | 决策点 | 选定方案 | 取代/影响 |
|---|--------|---------|----------|
| **DA** | AI 维护的驱动架构 | **`bellkeeper pkb-curate` 一次性 CLI 批处理**：提示词/领域/阈值外置在 `config/pkb/`，CLI 运行时读取并编排「列举→打分→分流→重构→落盘→重索引」；LLM 走 LLM Proxy（provider 无关） | 取代 v2「Claude Agent + Skill」；亦不回退 v1 常驻 service（见沿革表） |
| **DB** | MVP 边界 | **三层分流漏斗 + 高分原子化重构**：第一版产出即「能看的结构化卡片」 | 体系化合成 / n8n 纳管 → §6 路线图 |
| **DC** | 「调方向」入口 | **`config/pkb/`**（`domains.yaml` + `prompts/{score,reconstruct}.md`）：改文件即调方向，git 版本化（`config/` 不被 .gitignore，无需 `git add -f`） | 不做 Web 编辑页（§0.1）；未来若痛点强烈再评估（§6 R4） |
| **DD** | 文件浏览器 | **仅前端下线**：隐藏前端入口与页面；后端 `read`/`list`/`tree` 保留 → 作为 `pkb-curate` 读正文的**HTTP 备选**（CLI 在 keeper 容器内亦可直接 `os` 读文件） | 不物理删后端（读能力仍有用，见 §3.1） |
| **DE** | 三层目录落地 | **重命名复用现有**：`working→archive`、`KNOWLEDGE→vault`，`raw` 不变 | 改 `scan_dirs` + 一次性物理 `mv`，零新增目录结构 |
| **DF** | n8n 工作流纳管 | 维持 n8n 现有「爬取入库」职责（K01–K08 已在生产跑）；Bellkeeper 仓库目录托管 JSON（git）+ `spool n8n export` 冷备 | 移入 §6 路线图（非 MVP 必需）；**n8n 不扩张到 vault 维护**（容器够不着主机文件 + 重工具只用一点） |
| **DG** | 打分结果存哪 | **写文件 frontmatter**（`score`/`decision`/`domains`/`scored_at`）；ArticleTag 不扩列 | §0.1，frontmatter 即 Obsidian 可见、`pkb-curate` 可读 |

**被废弃的旧决策**（明确记录，避免回退）：
- v1：`EvaluatorService`/`ReconstructService` 常驻 worker、`Domain` 领域表、`ArticleTag` 扩 score 列、异步打分队列 —— **全部不做**。
- v2：`.claude/skills/pkb-curator/` Skill、依赖 Claude Code 触发、原 M5「gitignore 放行 `.claude/skills/`」—— **全部不做**（逻辑搬进 CLI + `config/pkb/` 提示词包）。

---

## 2. 目标架构（最小 CLI 批处理驱动）

### 2.1 三层职责

```
┌──────────────────────────────────────────────────────────────────────┐
│  调整入口层（人 = 调方向）                                                │
│  config/pkb/   ← 用户编辑这里就是「调方向」（git 版本化，改完下次运行即生效） │
│    ├─ domains.yaml          领域清单（关键词 / vault 子目录 / 阈值 / 权重） │
│    └─ prompts/score.md       三维打分提示词                              │
│       prompts/reconstruct.md 原子化重构提示词                            │
└───────────────────────────────┬──────────────────────────────────────┘
                                 │ pkb-curate 运行时读取
                                 ▼
┌──────────────────────────────────────────────────────────────────────┐
│  编排执行层（AI = 维护者，无自主 agent 框架，流程固定）                     │
│  bellkeeper pkb-curate（Bellkeeper 的一次性 CLI 子命令，跑完即退）         │
│  触发：手动 / keeper cron（无人值守）。在 sp-bellkeeper 容器内运行          │
│  流程：列待处理 → 读正文 → 打分 → 决策分流 → (高分)重构 → 落盘 → 重索引       │
└───────────────────────────────┬──────────────────────────────────────┘
                                 │ HTTP 调本机 server API + os 直接读写文件
                                 ▼
┌──────────────────────────────────────────────────────────────────────┐
│  原子能力层（Bellkeeper API + LLM Proxy = 工具，不含维护策略）             │
│  Bellkeeper API: list?layer= · knowledge/files/read · search · rebuild   │
│  LLM Proxy:      /api/llm/v1 (pool-summary 等虚拟组，打分/重构 LLM 后端)   │
│  本机文件系统:    os.Rename / os.WriteFile（CLI 与 /mnt/knowledge 同容器卷） │
└───────────────────────────────┬──────────────────────────────────────┘
                                 │ 读写
                                 ▼
┌──────────────────────────────────────────────────────────────────────┐
│  存储与呈现层（MD 文件 = 唯一事实源）                                      │
│  /mnt/knowledge/{raw,archive,vault}/  ←─ Meili 索引(archive+vault)        │
│                                       └─ Obsidian LiveSync(仅 vault) → 本地查看/图谱/双链 │
└──────────────────────────────────────────────────────────────────────┘
```

### 2.2 角色分工（一句话各司其职）

| 角色 | 职责 | 不负责 |
|------|------|--------|
| **`config/pkb/` 提示词包** | 承载维护**策略**（提示词/领域/阈值）；用户调方向的唯一入口 | 不含执行代码 |
| **`pkb-curate` CLI** | 读 `config/pkb/` 编排执行：列举→读→打分→决策→重构→落盘→重索引；跑完即退 | 不写死策略（策略在 `config/pkb/`）；非常驻 |
| **Bellkeeper（server）** | 提供**原子能力** HTTP API（入库落 raw / 列举 / 读正文 / 检索 / RAG / 重索引）+ Meili 索引 | 不含打分/重构/分流的"维护逻辑"（在 CLI 里） |
| **LLM Proxy** | `pool-summary` 等虚拟组作打分/重构的 LLM 后端（已落地，含熔断/限流/故障转移；provider 无关，换组即换模型） | — |
| **n8n** | 爬取入库（K01–K08，已在生产；K05 已接 `/api/llm/v1`） | 不碰 vault 维护（§DF） |
| **Obsidian** | vault 的查看 / 图谱 / 双链 / 关联（拉到本地用） | 不参与入库/打分 |
| **Meilisearch** | archive + vault 的全文检索后端 | — |
| **spool** | 一次性运维（M3 目录迁移 / `docker exec` 触发 CLI / n8n 冷备） | 不参与日常文件分流（CLI 容器内 `os` 直接做） |

### 2.3 MVP 端到端数据流

```
n8n 爬取（K01-article-ingest，现有）
  │  POST /api/files/ingest/url {url, title, content}      [已存在，落 raw/，默认层即 raw]
  ▼
/mnt/knowledge/raw/  ← 原始落盘（frontmatter + 正文 + ArticleTag 账本 + URL/哈希去重）
  │
  ▼  ── bellkeeper pkb-curate 触发（手动 / keeper cron）──
  ① 列待处理:  GET /api/files/list?layer=raw            [已存在，支持 layer 过滤]
  ② 读正文:    GET /api/knowledge/files/read?path=...    [后端保留] 或 os.ReadFile（容器内同卷）
  ③ 打分:      POST /api/llm/v1/chat/completions {model:pool-summary} + prompts/score.md
  │             → {relevance, depth, actionability, matched_domains, reason}
  │             CLI 算 final_score = 0.4*rel + 0.3*depth + 0.3*action（权重读 domains.yaml）
  ④ 决策分流（按 domains.yaml 阈值，以分数为准）:
  │     < 4.0   discard  → 留 raw/，os 改写 frontmatter decision=discard（不删除，可溯源）
  │     4.0–7.0 archive  → os.Rename raw→archive/，写 frontmatter decision/score
  │     >= 7.0  vault    → 进 ⑤ 重构
  ⑤ 原子化重构（高分）: POST /api/llm/v1 + prompts/reconstruct.md +（候选 vault 标题喂 wikilink）
  │     → 完整 Obsidian 卡片（frontmatter + 核心洞察 + 关键要点 + 深度摘要 + [[wikilink]]）
  │     os.WriteFile 到 vault/<domain.vault_subpath>/{YYYYMMDD}_{title}.md；raw/ 原文留底
  ⑥ 重索引:    POST /api/files/rebuild                   [已存在，全量重扫 archive+vault]
  ▼
检索/查看
  ├─ Meili: POST /api/files/search（按 layer 过滤）· POST /api/files/ask（RAG）  [已存在]
  └─ Obsidian: LiveSync 仅挂 vault/ → 本地图谱/双链/阅读（运维侧，非代码）
```

**一致性约定（MVP 取最简）**：**文件系统是 layer 的唯一事实源**。CLI 用 `os` 移动/写文件后，调一次 `rebuild` 让 Meili 与文件对齐。ArticleTag（DB 账本）MVP 阶段仅记录 raw 入库事实与去重哈希，**不强求其 `layer` 字段实时跟随**（避免为同步账本而新增 move 端点——§0.1）。打分/决策结果写进**文件 frontmatter**，Obsidian 与 CLI 都能直接读。

> **为何 CLI 能直接 `os` 读写文件**：`pkb-curate` 在 `sp-bellkeeper` 容器内运行（`docker exec` 触发），与 server 共享 `/mnt/knowledge` 挂载卷。这正好绕开了 v2「n8n 在独立容器够不着主机文件」的痛点——**无需 spool、无需新增 move/write 端点**（§3.4）。LLM/list/read/rebuild 走 HTTP 打本机 server（`localhost`，复用现有端点与 LLM Proxy 的熔断/限速/计费）。

---

## 3. Bellkeeper 原子能力盘点

### 3.1 现有可直接复用（已核实，零改动）

| 能力 | 端点 / 方式 | 锚点 | CLI 用途 |
|------|------------|------|-----------|
| 入库落 raw | `POST /api/files/ingest/url`（含 `Layer`/`Content` 字段，URL+SHA256 双去重） | [file_ingestion.go:79-216](../internal/service/file_ingestion.go#L79-L216) | n8n 入库入口（不变） |
| 列待处理 | `GET /api/files/list?layer=raw&status=&keyword=&page=` | [file_ingestion.go:58-91](../internal/handler/file_ingestion.go#L58-L91) | ① 列 raw 待打分 |
| 读正文 | `GET /api/knowledge/files/read?path=` | [knowledge_files.go:54-68](../internal/handler/knowledge_files.go#L54-L68) | ② 读 raw 正文（HTTP 备选；CLI 亦可 `os.ReadFile`） |
| 列目录/树 | `GET /api/knowledge/files/list` · `/tree` | [knowledge_files.go:34-49](../internal/handler/knowledge_files.go#L34-L49) | 辅助：核对文件落点 |
| 全文检索 | `POST /api/files/search`（支持 `Layers` 过滤） | [router.go:291](../internal/router/router.go#L291) | 重构时找 wikilink 候选 / 用户检索 |
| RAG 问答 | `POST /api/files/ask`（走 `pool-chat-balanced`） | [router.go:292](../internal/router/router.go#L292) | 用户主动问答 |
| 重建索引 | `POST /api/files/rebuild` | [router.go:294](../internal/router/router.go#L294) | ⑥ 分流后与文件对齐 |
| LLM 后端 | `POST /api/llm/v1/chat/completions` `{model:"pool-summary"}` | [router.go:139](../internal/router/router.go#L139) | ③ 打分 / ⑤ 重构 |
| **调 LLM 的现成范本** | `callLLM`：`llmProxyURL + "/chat/completions"`，model=虚拟组 | [knowledge_ask.go:146-167](../internal/service/knowledge_ask.go#L146-L167) | **重构调用直接抄** |
| **调 LLM + 解析 JSON 范本** | 调 LLM Proxy + **剥 markdown 围栏 + 解析 JSON** 已实现 | [classify.go](../internal/service/classify.go) | **打分调用 + JSON 解析直接抄** |
| **CLI 子命令骨架** | cobra `rootCmd.AddCommand(serveCmd, versionCmd, migrateCmd)` | [main.go:48](../cmd/bellkeeper/main.go#L48) | **挂 `pkbCurateCmd` 一行接入** |

> **关键洞察**：
> 1. `ingest/url` 已有 `Content` 字段（免新增 `ingest/file`）；`list` 已支持 `layer` 过滤（免新增"列待处理"端点）；`read` 正好是 CLI 读正文所需——**MVP 不需要为编排新增任何业务端点**。
> 2. 调 LLM（[knowledge_ask.go:146-167](../internal/service/knowledge_ask.go#L146-L167)）+ 解析 JSON（[classify.go](../internal/service/classify.go)）的代码**已存在**，`pkb-curate` 的打分/重构调用是「抄现成模式」，不是从零写。
> 3. CLI 入口**已用 cobra**，加 `pkb-curate` 与现有 `migrate`（一次性、跑完即退）同模式——**这是 v3 工程量极小的根本原因**。

### 3.2 MVP 必需的最小改动

| # | 改动 | 文件 | 性质 | 验收 |
|---|------|------|------|------|
| M1 | scanner 改递归 `WalkDir` | [knowledge_scanner.go:41-90](../internal/service/knowledge_scanner.go#L41-L90)（现 `os.ReadDir` 遇子目录 `continue`，[:55-58](../internal/service/knowledge_scanner.go#L55-L58)） | 增强现有函数；layer 取所属顶层 scan_dir | `vault/security/x.md` 能被 search 命中 |
| M2 | `scan_dirs` 去 raw、改三层 | [config.go:423-426](../internal/config/config.go#L423-L426) `[{raw},{working},{KNOWLEDGE}]` → `[{path:"archive",layer:"archive"},{path:"vault",layer:"vault"}]` | 配置默认值 | rebuild 后 Meili 不含 raw 文档 |
| M3 | 物理迁移三层 | `spool exec keeper`：`working/*→archive/`、`KNOWLEDGE/*→vault/`（**走 spool，禁裸 ssh**，CLAUDE.md §4） | 运维一次性 | 旧目录空、新目录有文件 |
| M4 | 前端下线文件浏览器 | 见 §3.3 | 前端 4 处删改 | 菜单/路由无"文件管理"，后端 API 仍可调 |
| **M5** | **新增 `pkb-curate` CLI 子命令 + `config/pkb/` 提示词包** | [main.go:48](../cmd/bellkeeper/main.go#L48) + 新建 `internal/pkb/`（编排逻辑）+ `config/pkb/`（提示词，见 §4） | **v3 唯一的新增代码逻辑**（一次性命令，非常驻 service） | `bellkeeper pkb-curate --dry-run` 能跑通一轮且不写盘 |

> `default_layer` 已是 `raw`（[config.go:377](../internal/config/config.go#L377)），ingest 默认落 raw，**无需改**。
> **较 v2 的变化**：删除原 M5「gitignore 放行 `.claude/skills/`」——`config/` 不被 .gitignore（[.gitignore](../.gitignore) 仅忽略 `config/*.local.yaml`），`config/pkb/` 直接进 git。新 M5 是「CLI 子命令 + 提示词包」。
> **M1+M5 是 Bellkeeper 侧仅有的代码改动**：M1 是把 `ReadDir` 换 `WalkDir` 的增强；M5 是新增一个一次性 CLI 命令（复用 §3.1 的现成调用模式）——均不引入常驻 service / 新表，符合 §0.1。

### 3.3 文件浏览器前端下线（M4 精确动作）

**删/改**（前端，SolidJS）：
- [Layout.tsx:241](../web/src/components/Layout.tsx#L241)：删菜单项 `{ path: '/knowledge/files', label: '文件管理', ... }`（保留「知识搜索」「知识问答」）
- [index.tsx:21](../web/src/index.tsx#L21)：import 去掉 `KnowledgeFiles`（留 `KnowledgeSearch, KnowledgeAsk`）
- [index.tsx:41](../web/src/index.tsx#L41)：删 `<Route path="/knowledge/files" component={KnowledgeFiles} />`
- [Dashboard.tsx:142](../web/src/pages/Dashboard.tsx#L142)：Knowledge 卡片 `href="/knowledge/files"` 改指 `/knowledge/search`

**保留**（CLI 备选 / 防误删）：`web/src/pages/knowledge/KnowledgeFiles.tsx`、`web/src/api/knowledge.ts`、后端 `knowledge_files.go`(handler/service) + `/api/knowledge/files/*` 路由。

> 验收提交前 `cd web && pnpm build` 必须绿；`grep -rn "KnowledgeFiles" web/src` 仅剩页面文件自身与 api 封装（无菜单/路由引用）。

### 3.4 明确不做（删除旧方案 · §0.1）

- ❌ 不新增 `EvaluatorService` / `ReconstructService` 常驻 worker（逻辑在一次性 CLI 里）
- ❌ 不依赖 Claude Code / 不建 `.claude/skills/`（v2 已废弃）
- ❌ 不引入 n8n AI Agent / LangGraph / CrewAI 等 agent 框架（重工具只用一点，§0.1）
- ❌ 不新增 `Domain` 领域表（领域清单放 `domains.yaml`，§4.2）
- ❌ 不给 `ArticleTag` 扩 score 列（打分写 frontmatter，DG）
- ❌ 不新增 `move` / `ingest/file` / `digest` 等端点（CLI 在容器内 `os` 直接读写 + 现有 API 组合）
- ❌ 不在 Bellkeeper 自研图谱/双链/关联可视化（交 Obsidian，§0.2/§0.4）

---

## 4. PKB 维护逻辑设计（`pkb-curate` CLI + `config/pkb/` = 调方向入口）

> 维护**策略**全部外置在 `config/pkb/`（给人编辑），**编排代码**在 `internal/pkb/`（给 CLI 执行）。两者分离 = v2 转向内核的延续。

### 4.1 目录结构

```
config/pkb/                  # 策略（进 git，改文件即调方向，不重编）
├─ domains.yaml              # 领域清单（用户最常调的文件）
└─ prompts/
   ├─ registry.yaml          # 当前启用的提示词版本（active.score / active.reconstruct）
   ├─ score.md               # 当前打分提示词（可另建 score.v2.md 后在 registry 切换）
   └─ reconstruct.md         # 当前原子化重构提示词

internal/pkb/                # 编排代码（CLI 执行逻辑，新增）
├─ curator.go                # 主流程：列举→读→打分→分流→重构→落盘→重索引
├─ score.go                  # 调 LLM 打分 + 解析 JSON（抄 classify.go）
├─ reconstruct.go            # 调 LLM 重构（抄 knowledge_ask.go callLLM）
└─ domains.go                # 加载/解析 domains.yaml

cmd/bellkeeper/main.go       # rootCmd.AddCommand(pkbCurateCmd)  ← 一行接入
```

### 4.2 `domains.yaml`（用户调方向的主战场）

领域是**配置项不是代码**。用户增删领域、调关键词/阈值/权重，只改这一个文件。初始领域对齐用户的知识体系：

```yaml
# 全局默认（可被单领域覆盖）
defaults:
  vault_threshold: 7.0      # >= 进 vault（重构）
  archive_threshold: 4.0    # >= 进 archive，< 丢弃
  weights: { relevance: 0.35, depth: 0.25, actionability: 0.25, durability: 0.15, novelty: 0.0 }
  score_model: pool-summary       # 打分用的 LLM Proxy 虚拟组（换模型=改这里）
  reconstruct_model: pool-pkb     # 重构用的虚拟组（高分文章才触发，可用强模型）
  score_temperature: 0.2
  reconstruct_temperature: 1.0
  per_run: 5                      # 单轮处理量上限（护栏）
  content_truncate: 8000          # 按 rune 字符截断，避免中文 UTF-8 被截断
  llm_token_env: PKB_LLM_TOKEN    # 可选：专用 LLM token，便于独立配额/计费
  budget:
    max_score_calls_per_run: 50
    max_reconstruct_calls_per_run: 5
    max_digest_calls_per_run: 3
  retry:
    max_attempts: 4
    initial_backoff_seconds: 20
    max_backoff_seconds: 300
    stop_run_on_rate_limit: true

domains:
  - name: programming
    display: 编程
    vault_subpath: vault/编程
    keywords: [java, .net, csharp, golang, go, rust, 前端, react, vue, typescript, 后端, 架构]
  - name: security
    display: 网络安全
    vault_subpath: vault/安全
    keywords: [渗透测试, 代码审计, 逆向, 安卓, android, 内网渗透, 域渗透, 安全合规, owasp, cve, poc, exp]
  - name: cs-fundamentals
    display: 计算机基础
    vault_subpath: vault/基础
    keywords: [算法, 数据结构, 操作系统, 网络协议, tcp, http, 运维, linux, 容器, k8s]
  - name: ai
    display: 人工智能
    vault_subpath: vault/AI
    keywords: [大模型, llm, nlp, 机器学习, 深度学习, 强化学习, rag, agent, transformer, 微调]
  - name: misc
    display: 周边杂项
    vault_subpath: vault/杂项
    keywords: []                    # 兜底领域
    is_default: true
  - name: news
    display: 最新资讯
    vault_subpath: vault/资讯
    keywords: [发布, release, 新版本, 趋势, 动态]
    vault_threshold: 8.0            # 资讯门槛更高（时效性内容少进 vault）
```

> 子目录可用中文（Obsidian 友好）；vault 子目录可检索依赖 M1 scanner 递归。新增领域 = 加一个 `- name:` 块；换驱动模型 = 改 `score_model`/`reconstruct_model`，**均无需改任何代码**（§0.6）。

### 4.3 `pkb-curate` 执行流程（`internal/pkb/curator.go` 要点）

CLI 跑的标准循环（流程固定，无自主 agent）：

```
0. 加载 config（含 LLM Proxy URL / token）+ config/pkb/domains.yaml + prompts/*
1. GET /api/files/list?layer=raw&per_page=<per_run>     # 取一批待处理
2. for 每篇:
   a. 读正文（os.ReadFile，或 GET /api/knowledge/files/read）
   b. 调 score_model + prompts/score.md，注入领域关键词+标题+正文截断
      → 解析 JSON（剥 markdown 围栏，抄 classify.go），算 final_score
   c. 决策（以分数为准，校正 LLM 自报 decision）:
      - < archive_threshold → discard：os 改写 raw 原文 frontmatter decision=discard
      - < vault_threshold   → archive：os.Rename raw→archive/，写 frontmatter decision/score
      - >= vault_threshold  → vault:
          · POST /api/files/search 取该领域已有 vault 卡片标题 top-N 作 wikilink 候选
          · 调 reconstruct_model + prompts/reconstruct.md（注入正文+打分+候选标题）
          · 写盘前校验每个 [[wikilink]] 目标真实存在（防死链）
          · os.WriteFile 卡片到 <vault_subpath>/{YYYYMMDD}_{title}.md（raw 原文留底）
   d. 单篇出错 → 记录并跳过，不中断整批
3. POST /api/files/rebuild                              # 与文件系统对齐
4. 打印本轮处理摘要（处理 N 篇 / vault M / archive K / discard L / 失败 F）；进程退出
```

### 4.4 提示词契约

**`prompts/score.md`（长期知识价值打分）**：System/User 合并在文件提示词中；CLI 注入 `{已配置领域及关键词}`+`{标题}`+`{正文截断}`。输出只允许 JSON：
```json
{ "relevance": 8, "depth": 7, "actionability": 6, "durability": 8, "novelty": 5, "content_type": "tutorial", "matched_domains": ["security"], "reason": "讲清 JWT 攻击面 + 可复用 PoC" }
```
`final_score` 由 CLI 按 `domains.yaml` 权重计算，并按 `content_type` 做轻量修正：`marketing -2.0`、`news -1.0`、`release -0.5`、`tutorial/paper/reference +0.5`、`code/poc +0.7`。CLI 不信任 LLM 自报 decision，以分数和阈值为准。`temperature` 设低（0.2）求稳定；解析沿用「剥 markdown 围栏 + JSON 解析」容错。

**`prompts/reconstruct.md`（原子化重构）**：提示词要求模型直接输出完整 markdown；CLI 注入 `{正文}`+`{打分结果}`+`{已有 vault 卡片标题候选}`。写盘前会校验 frontmatter、固定二级标题、最小长度，并清理候选外 wikilink。要求输出：
```markdown
---
title: <提炼标题>
source: <原始 URL>
ingest_date: <YYYY-MM-DD>
score: <final_score>
domains: [security]
tags: [jwt, auth, poc]
---
## 核心洞察
## 关键技术要点 / 可复用资产
## 深度摘要
## 关联
- [[已有卡片A]]   ← 仅从候选列表选真实存在标题，禁止虚构
```


### 4.4.1 提示词治理（Prompt Registry）

`config/pkb/prompts/registry.yaml` 是当前启用提示词的唯一开关：

```yaml
active:
  score: score.md
  reconstruct: reconstruct.md
```

调 prompt 时推荐新增版本文件（如 `score.v2.md` / `reconstruct.v2.md`），再改 registry 切换；不要直接覆盖旧文件。这样可以 git diff、回滚、AB 对比，并避免「调参靠记忆」。CLI 若找不到 registry，会兼容默认的 `score.md` / `reconstruct.md`。

### 4.4.2 大模型调用治理

- LLM 调用会带 `X-Caller-ID: pkb-curate`，打分带 `X-Task-Type: summary`，重构带 `X-Task-Type: long_context`，便于 LLM Proxy 日志、路由和计费识别。
- `defaults.llm_token_env` 可指定专用 LLM token 环境变量（默认建议 `PKB_LLM_TOKEN`）。若环境变量不存在，回退 server api key。
- `defaults.budget.max_score_calls_per_run` / `max_reconstruct_calls_per_run` 是单轮硬护栏，避免一次运行把付费额度打穿。
- **通用 LLM 持久队列优先**：`llm_job_queue.enabled=true` 时，`pkb-curate` 的打分/重构/digest 不再直接撞 LLM Proxy，而是写入 `llm_jobs`，由 server 里的 `LLMJobQueueService` worker 统一调 LLM Proxy。分类与知识问答也复用同一队列/同一 `internal/llmclient` 调用层。队列负责 pending/running/retrying/success/dead、`next_retry_at`、stale running 恢复、幂等 key 去重和长时间退避。
- **免费模型池职责边界**：LLM Proxy 仍负责短时令牌桶等待、故障转移、熔断、计费和自适应限流学习；`llm_jobs` 负责持久任务队列。`pkb-curate` 仍把 `raw + exclude_processed=true` 当作业务待处理池：只有打分/分流/写盘成功后才写 `pkb_decision` 与 DB 处理标记，429/503/上游耗尽不会误标完成。
- `defaults.retry` 现在是队列关闭时的 fallback 保护；生产默认走 `llm_job_queue`。大量文件跑批时会变慢，但任务会在 `llm_jobs` 中长期重试，不会把队列打崩或错漏。

### 4.5 用户「调方向」对照表

| 想调什么 | 改哪个文件 | 生效方式 |
|---------|-----------|---------|
| 增/删领域、改关键词、改某领域阈值 | `config/pkb/domains.yaml` | 下次 `pkb-curate` 运行即生效 |
| 换打分/重构用的 LLM 模型 | `domains.yaml` 的 `score_model`/`reconstruct_model` | 同上（走 LLM Proxy 虚拟组） |
| 打分标准、维度侧重、JSON 字段 | `config/pkb/prompts/score.md` | 同上 |
| 卡片结构、重构风格、wikilink 规则 | `config/pkb/prompts/reconstruct.md` | 同上 |
| 全局阈值/权重、单轮处理量 | `domains.yaml` defaults | 同上 |

> 全程**不改 Go 代码、不重编、不重启 server**（改的是 `config/pkb/` 下的数据文件，CLI 运行时读取）。这就是「人只调方向」（§0.3）。

### 4.6 触发方式（MVP）

- **手动**：`spool exec keeper "docker exec sp-bellkeeper /bin/sh /app/scripts/bellkeeper-init.sh pkb-curate --per-run 5"`（走 spool，且复用 init 脚本加载 `/app/config/.env`；首跑建议加 `--dry-run` 只打分不写盘）。
- **半自动（无人值守）**：keeper 主机 crontab 调上面那条 `docker exec`（如每日一次）。**待 MVP 跑顺、信任打分质量后再开**（§0.5 痛点驱动；见 §6 R7）。
- CLI 是一次性命令（跑完即退），天然适配 cron，无常驻进程/无后台 goroutine（§0.1、规避 v1）。

### 4.7 安全护栏（写进 `curator.go`，防止 AI 维护失控）

- `--dry-run`：只打分、打印决策，不移动/不写盘（首跑与调提示词时用）。
- discard **只标记不删除**（留 raw 可溯源、可调阈值后重评）。
- wikilink **只能选检索返回的真实卡片标题**，写盘前二次校验目标存在（否则 Obsidian 死链）。
- 单轮处理量上限 `per_run`（domains.yaml）+ 出错单篇跳过并记录，不中断整批。
- 所有写操作（`os.Rename`/`os.WriteFile`）打日志留痕；目录迁移类一次性运维仍走 `spool exec`（CLAUDE.md §4）。


### 4.8 卡片间知识结构

MVP 的结构建设方式是 **Obsidian wikilink 双链**：`pkb-curate` 在高分文章重构前用标题+领域检索已有 vault 卡片，作为候选标题注入 `reconstruct.md`；模型只能从候选里选择真实存在的标题，CLI 写盘前会移除候选外链接，避免死链。

这意味着当前系统已经能建设「卡片 ↔ 卡片」的局部关系，但还没有自动建设「领域索引页 / 专题页 / 知识地图 / 周期性综述」。这些更适合放到后续 `pkb-digest` 或 `pkb-curate digest` 阶段：定期读取某领域高分卡片，让大模型生成专题笔记、索引页和跨卡片关系说明。不要把专题结构混进单篇卡片重构，否则会让每次入库都承担过重上下文和成本。

是否需要大模型介入：
- 单篇卡片的局部关联：需要，但只允许从真实候选中选择，当前已实现。
- 全局知识结构（专题页/领域地图/综合综述）：需要，但应批量、低频、可回滚地运行；这是 §6 R3 的自然扩展。

---

## 5. MVP 分步实施（一步一个可验收原子提交）

> 每步提交前：`go build ./... && go vet ./...` 绿；动前端则 `cd web && pnpm build` 绿（CLAUDE.md §2.8）。

### Step 1 — 三层目录 + 索引基础（M1/M2/M3，约 0.75d）
- M3 物理迁移：`spool exec keeper "mv /mnt/knowledge/working/* /mnt/knowledge/archive/ 2>/dev/null; mv /mnt/knowledge/KNOWLEDGE/* /mnt/knowledge/vault/ 2>/dev/null"`（先 `spool backup keeper` 兜底）
- M2 改 `scan_dirs` 为 `archive`/`vault`
- M1 scanner 改 `WalkDir` 递归，layer 取所属顶层 scan_dir
- **验收**：`POST /api/files/rebuild` 后，`vault/<领域>/x.md` 能被 `search` 命中；raw 文档**不**出现在结果

### Step 2 — 前端下线文件浏览器（M4，约 0.25d）
- 按 §3.3 删改 4 处，后端全保留
- **验收**：`pnpm build` 绿；菜单/路由无「文件管理」；`curl /api/knowledge/files/read?path=...` 仍 200（后端可用）

### Step 3 — `pkb-curate` CLI + `config/pkb/` 提示词包（M5 + §4，约 1.0d）
- 建 `config/pkb/`：`domains.yaml` + `prompts/{score,reconstruct}.md`（按 §4.2/§4.4）
- 建 `internal/pkb/`：`domains.go`（加载 yaml）+ `score.go`（抄 classify.go：调 LLM + 剥围栏解析 JSON）+ `reconstruct.go`（抄 knowledge_ask.go:146-167 callLLM）+ `curator.go`（§4.3 主流程，含 §4.7 护栏 + `--dry-run`/`--per-run` flags）
- `cmd/bellkeeper/main.go`：`rootCmd.AddCommand(pkbCurateCmd)`
- **验收（防死代码 CLAUDE.md §2.7）**：`go build`/`go vet` 绿；`bellkeeper pkb-curate --help` 列出命令；`--dry-run` 能列 raw、打分、打印决策**而不写盘**；`internal/pkb` 被 `cmd/bellkeeper/main.go` 真实导入（`grep` 验证调用方>0）；引用的每个端点都在 §3.1 真实存在

### Step 4 — 跑通一轮维护（端到端验证，约 0.5d）
- 准备：keeper 上确保 `pool-summary` / `pool-pkb` 虚拟组可用（对应 model 调 `/api/llm/v1/chat/completions` 返回 200；若组未入库，经 `POST /api/llm/config/groups` 手建一次）
- 部署带 `pkb-curate` 的新镜像；`spool exec keeper "docker exec sp-bellkeeper /bin/sh /app/scripts/bellkeeper-init.sh pkb-curate --dry-run --per-run 5"` 先验，再去掉 `--dry-run` 实跑
- 人工抽查：低分留 raw、中分进 archive、高分在 vault 生成结构化卡片（5 要素齐全、wikilink 无死链）
- **调提示词**：若分流/重构质量不满意，改 `config/pkb/prompts/*` 重跑（这就是迭代闭环，无需重编）
- **验收**：一轮跑完，三层分流符合预期；`rebuild` 后 archive+vault 可搜

### Step 5 — Obsidian 同步 + 检索验证（运维，约 0.25d）
- Obsidian LiveSync **只挂 `vault/`** → CouchDB（运维侧，非代码）；raw/archive 不下行
- **验收**：vault 卡片在本地 Obsidian 可见、图谱/双链生效；raw 内容**不**出现在本地

**MVP 工期合计 ≈ 2.75 天**（较 v2 的 2.5d 多 0.25d：CLI 编排代码比 Skill 略多，但省了 Skill 调试 + gitignore 折腾；较 v1 的 5.5d 省下两个常驻 service 的开发+测试）。

---

## 6. 路线图骨架（MVP 之后 · 按需 + 评估后迭代）

> 大方向骨架**保留**，但遵守 §0.5「痛点驱动、不预先建设」。每项标**触发条件** + **评估门槛**（动手前过 §0.7）。需求细节见 [ROADMAP.md](ROADMAP.md) §10。

| 阶段 | 内容 | 触发条件 | 评估要点（§0.7） |
|------|------|---------|----------------|
| R1 | **增量索引** 替代全量 rebuild | vault 大到 rebuild 明显变慢 | 改 scanner 支持按 mtime/状态增量；纯增强，无新工具 |
| R2 | **n8n 工作流纳管**（仓库 `n8n_workflows/` git + `spool n8n export` 冷备） | 工作流改动频繁、需 review/回滚 | 仅 git 托管 JSON，**不做** DB/CRUD/UI；用现有 spool |
| R3 | **体系化合成**（定期聚合某领域高分卡片 → 综述笔记 → vault digest） | vault 卡片够多、需要"周综述"缝合 | 复用 vault 文件 + LLM Proxy，写成 `pkb-curate digest` 子命令；第一版只生成 `vault/<领域>/digest/YYYY-Wxx_<领域>周综述.md`，不新增端点 |
| R4 | **领域配置可视化 / Web 调整界面** | `domains.yaml` 手编成为高频痛点 | 先问：Obsidian/编辑器直接改 yaml 是否已够？够则不做（§0.1） |
| R5 | **ArticleTag 打分账本 / move 端点** | 需要按分数在 Bellkeeper 端排序筛选、或 frontmatter 一致性出问题 | 仅当 frontmatter 方案确实不够才加（DG 兜底） |
| R6 | **Matrix `!问<领域>`** 主动问答 | 想在 Matrix 里直接查知识库 | 复用现有 `/api/files/ask`，仅接 Matrix 命令 |
| R7 | **维护定时化（keeper cron 调 `pkb-curate`）** | 手动触发成为负担、且信任打分质量 | CLI 已天然适配 cron；需先有可观测（处理摘要落日志/告警），避免静默误判 |
| R8 | **CLI 进程内直调 service**（省去对本机 server 的 HTTP 往返） | 性能/部署上确有收益 | 纯内部优化；需 wire service 依赖，权衡复杂度 |

> 顺序非固定，由真实使用反馈决定。**新增任何一项前，先回 §0 核对。**

---

## 7. 验收标准（MVP · 映射 §10.9，按本范围裁剪）

| § | 验收项 | 由哪步保证 |
|---|--------|-----------|
| 10.9-1 | `raw/` 内容不出现在本地 Obsidian | Step1(不索引 raw) + Step5(LiveSync 仅 vault) |
| 10.9-2 | `<4.0` 丢弃，不进 archive/vault，仅 raw 可溯源 | Step4（discard 只标记留 raw） |
| 10.9-3 | `>=7.0` 在 vault 生成结构化笔记（5 要素 + wikilink 无死链） | Step4（reconstruct.md + §4.7 护栏） |
| 10.9-5 | 整个 vault 可被 Meili 全局检索 + git/rsync 整体导出 | Step1(M1 递归) |
| — | **调方向不改代码**：增删领域/调提示词/换模型只编辑 `config/pkb/` 即生效 | §4.5 |
| — | **奥卡姆达标**：Bellkeeper 无新增常驻 service/表/业务端点（仅一次性 CLI + 提示词文件） | §3.2/§3.4 |
| — | **provider 无关**：打分/重构走 LLM Proxy，换模型=改 domains.yaml 一字段 | §4.2/§4.5 |
| 10.9-4/6 | 领域批量重打分聚合 / n8n 纳管 | **MVP 不覆盖** → §6 路线图（R2/R3） |

**总自检**（CLAUDE.md §3）：`pkb-curate` 引用的每个端点都在 §3.1 真实存在（防死代码）；`internal/pkb` 被 `cmd/bellkeeper/main.go` 真实导入（调用方>0）；数据链路写→存→读→展示完整（ingest→分流落盘→rebuild→search/Obsidian）；无 `not implemented` 占位；前端下线后 `pnpm build` 绿。

---

## 8. 风险与回滚

| 风险 | 缓解 |
|------|------|
| LLM 打分不稳 / JSON 解析失败 | 低 temperature + 剥围栏容错（抄 classify.go）+ 单篇失败跳过记录，不中断整批 |
| 打分误判丢弃高价值文章 | discard **只标记不删**，留 raw；调 `domains.yaml` 阈值后对 raw 重跑 |
| AI 维护"跑飞"（误删/乱建） | `--dry-run` 预演 + discard 不删 + wikilink 校验 + `per_run` 上限 + MVP 默认手动触发 |
| 全量 rebuild 慢 | MVP 库小可接受；变慢则 R1 增量索引 |
| 提示词/config 未进 git 丢失 | `config/pkb/` 不被 .gitignore，正常 `git add` 即进库（无需 `-f`） |
| keeper noauth 公网暴露 | 见记忆 [[keeper-bellkeeper-noauth]]：内网/VPN 限制，公网需网络层补认证（待用户处理） |

**回滚**：各 Step 独立提交可单独 revert。M3 目录迁移可 `mv` 回；M2 改回 `scan_dirs` 即恢复旧扫描；删除 `pkb-curate` 命令注册即停用 AI 维护（ingest 仍正常落 raw，退化为改造前行为）。

---

## 附录：关键文件索引（实施起点）

| 关注点 | 文件:行 |
|--------|---------|
| ingest 落盘链路（含 Layer/Content/去重） | [internal/service/file_ingestion.go:79-216](../internal/service/file_ingestion.go#L79-L216) |
| list 列待处理（layer 过滤） | [internal/handler/file_ingestion.go:58-91](../internal/handler/file_ingestion.go#L58-L91) |
| read 读正文（CLI 备选） | [internal/handler/knowledge_files.go:54-68](../internal/handler/knowledge_files.go#L54-L68) |
| **调 LLM 范本（重构抄它）** | [internal/service/knowledge_ask.go:146-167](../internal/service/knowledge_ask.go#L146-L167) |
| **调 LLM + 剥围栏解析 JSON（打分抄它）** | [internal/service/classify.go](../internal/service/classify.go) |
| scanner（M1 改递归） | [internal/service/knowledge_scanner.go:41-90](../internal/service/knowledge_scanner.go#L41-L90) |
| scan_dirs 默认（M2） | [internal/config/config.go:423-426](../internal/config/config.go#L423-L426) |
| 元数据账本 ArticleTag（含 Metadata jsonb） | [internal/model/dataset.go:37-59](../internal/model/dataset.go#L37-L59) |
| search / ask / rebuild 路由 | [internal/router/router.go:289-296](../internal/router/router.go#L289-L296) |
| LLM Proxy `/api/llm/v1` | [internal/router/router.go:137-139](../internal/router/router.go#L137-L139) |
| **CLI 入口（M5 挂 pkb-curate）** | [cmd/bellkeeper/main.go:48](../cmd/bellkeeper/main.go#L48) |
| 前端下线点（M4） | [Layout.tsx:241](../web/src/components/Layout.tsx#L241) · [index.tsx:21/41](../web/src/index.tsx#L41) · [Dashboard.tsx:142](../web/src/pages/Dashboard.tsx#L142) |
| gitignore（`config/` 不被忽略，提示词可直接进 git） | [.gitignore](../.gitignore) |
| n8n 工作流(SilkSpool, R2) | `/opt/SilkSpool/hosts/keeper/n8n-workflows/*.json` |

---

*本文件随实施推进更新；每完成一 Step，回写「验收」勾选并在 [STATUS.md](STATUS.md) 追加主线动作。需求层面变更回 [ROADMAP.md](ROADMAP.md) §10。提交须 `git add -f doc/PKB-IMPLEMENTATION.md`（`doc/` 被 .gitignore；注意 `config/pkb/` 不在此列，正常提交）。*
