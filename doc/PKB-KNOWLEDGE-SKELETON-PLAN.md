# PKB 知识骨架与双库改进计划

> **定位**：[PKB-ATOMIC-KNOWLEDGE-PLAN.md](PKB-ATOMIC-KNOWLEDGE-PLAN.md) 的下一代。原子计划解决了「卡片够不够原子、卡片之间够不够网状」（已全部落地）；本文件解决**上一层**——「整个领域的知识有没有一棵完整、人可掌舵、可持续填充的目标结构」，以及「时效资讯与稳定知识如何分流」。
>
> **基座**：原子计划的三层节点（原子卡 / 主题 MOC / 领域 index）、出处与置信度、audit、scheduler 全部沿用。本文件把领域 index 从「digest 涌现猜树」升级为「人调方向的知识骨架」，并新增缺口填充、双库分流、晋升闸、调方向前端。
>
> **决策依据**：2026-06-16-17 与用户的逐项 grill（见下「决策记录」），并受 [ADR-0004](../docs/adr/0004-knowledge-skeleton-as-structure-source-of-truth.md)、[ADR-0005](../docs/adr/0005-knowledge-base-and-feed-base-split.md) 约束。术语以 [CONTEXT.md](../CONTEXT.md) 为准。
>
> **适用分支**：`feat/pkb-knowledge-skeleton`。

---

## 0. 最高约束（沿用 PKB-IMPLEMENTATION §0，不重述）

奥卡姆剃刀（§0.1）、成熟工具优先（§0.2）、AI 主导人调方向（§0.3）、落地 MD + Obsidian 为中心（§0.4）、MVP 优先（§0.5）、可增长可维护随时可调（§0.6）。本计划每个决策都已过这 6 条。

---

## 1. 决策记录（2026-06-16~17 grill 对齐）

| # | 决策点 | 选定方案 | 取代/影响 |
|---|--------|---------|----------|
| **Q1** | 结构来源 | **C 混合且持续**：LLM 自上而下生成目标骨架（脊梁）+ 缺口定向填充 + RSS 涌现卡挂靠 | 否决纯涌现（永远填不满完整结构）与纯教纲（丢 RSS 增量） |
| **Q2** | 骨架怎么长 | **A1+B3+涌现回流**：人只播大方向（领域+一句话 scope），LLM 生成多层树（权威 TOC 校正），挂不上的卡积簇触发 LLM 提议扩展、人批准式微调 | 否决人工定子领域（违背「人只调方向」） |
| **Q3** | 骨架 vs 涌现树 | **M1 单真相源**：骨架是结构唯一真相源，digest 不再猜树，改为渲染骨架 | 见 ADR-0004；否决 M2 双真相源（必漂移） |
| **Q4** | 挂不上的卡 | **待归位区**：领域级 `vault/<领域>/_待归位.md` + 全局 `vault/_待归位.md`；「待归位」作为 audit 新指标 | 复用 audit.go，不另起炉灶 |
| **Q5** | 库的二分 | **知识库 / 资讯库双子系统 + 晋升闸** | 见 ADR-0005 |
| **Q6** | 缺口怎么填 | **F3 偏 F1**：稳定缺口 LLM 生成为主，前沿/易变缺口定向爬；定向爬退化为次要路径 | 修正用户最初「知识结构的爬取」措辞 |
| **Q7** | 引用可信 | **V2 抓取核实**：模型自报引用必须真去 fetch + LLM 校验支撑；F1/F2 界限消失，都落到「卡背靠真实核实源」 | 否决 V1 信任自报引用（编造风险） |
| **Q8** | 骨架存哪 | **S1 vault markdown**：骨架就活在 `_index.md`/`topics`，无第二存储 | 否决 S2 config YAML / S3 DB 表（前端非文件编辑器，见 Q9） |
| **Q9** | 骨架谁写 | **W1 机器独占写**：digest 增量维护，人不手改、只 Obsidian 同步查看；调方向经前端作为独立输入喂 digest | 否决 W2 人机共写（合并地狱） |
| **Q10** | 缺口抓取走哪 | **G3**：用 `ExtractorService.Extract` 原语拿内容当场核实 + 读 `CrawlDomainProfile.next_allowed_at` 冷却让路；失败不进失败档案，只降级卡片 | 否决 G1 入完整队列（语义不匹配）/ G2 裸抓（不让路） |
| **Q11** | 填充节奏 | **独立命令 + 每领域每轮限额（可配不写死）+ 自顶向下广度优先 + 每领域开关 + 一键总开关 + 默认开关配置化** | 全配置驱动，不改代码 |
| **Q12** | 骨架变更批准 | **P1 起步 + P3 升级**：按影响半径分级——小动作快照后自动应用，大动作（删/合并/重构，触及卡片 > 阈值）推 Matrix 批准 | 允许删/合并；前端到位前用 Matrix 闸过渡 |
| **Q13** | 资讯库形态 | **每日存档**：`资讯/<领域>/<YYYY-MM-DD>.md`，与日报联动；资讯不存独立卡、只活在每日存档；只追加不删 | 用户改周报为日报 |
| **Q14** | 模型路由 | **强模型内部按场景分级**：生成（骨架/缺口起草）走顶级推理档，判断（核实/归位/晋升）走快强档；值映射到模型组，LLM Proxy 解析，provider 无关 | 沿用 domains.yaml 每任务一 model_group + ADR-0002 队列 |
| **Q15** | 落档 | 本计划文档 + ADR-0004 + ADR-0005；**调方向前端不写 ADR**（DC 未被推翻，前端形态待定） | — |

**关键澄清（Q15 用户更正）**：DC「不做前端」**未被推翻**。用户要的是**调方向前端**（批准提议/设大方向/剪节点的最小掌舵面），**不是** §0.4/DC 否决的「完整接入/知识库浏览器/Web 文件编辑页」。浏览仍归 Obsidian，文件仍不手编。

---

## 2. 目标架构

```
                        ┌─────────────────────────────────────────┐
   人（调方向）──────────▶│  调方向前端（形态待定，窄掌舵面）            │
                        │  批准 LLM 骨架提议 / 设领域大方向 / 剪节点    │
                        └───────────────┬─────────────────────────┘
                                        │ 调方向输入（非文件编辑）
                                        ▼
┌───────────────────────────────────────────────────────────────────────┐
│  知识库（稳定体系 · 骨架组织 · 激进去重 · 衰减慢）                            │
│                                                                         │
│   知识骨架（vault md，digest 独占写）                                      │
│     vault/<领域>/_index.md      根：领域知识体系树（pkb_map）              │
│     vault/<领域>/topics/<主题>.md 中：主题 MOC（pkb_topic）               │
│     vault/<领域>/<concept>.md   叶：原子卡（pkb_card，带出处/置信度）       │
│     vault/<领域>/_待归位.md      挂不上节点的卡（投影视图）                  │
│     vault/_待归位.md            连领域都归不上的卡                         │
│                                                                         │
│   缺口填充（独立命令 pkb-curate fill）：                                   │
│     遍历缺口 → LLM 起草卡(+提议权威源) → Extract 抓取(冷却让路) → V2核实 → 归位 │
└───────────────────────────────────────────────────────────────────────┘
        ▲ 晋升闸（durability+novelty 把耐久知识点捞进骨架）
        │
┌───────┴───────────────────────────────────────────────────────────────┐
│  资讯库（时效流 · 每日组织 · 宽松去重 · 衰减快 · 只追加）                     │
│     资讯/<领域>/<YYYY-MM-DD>.md   每日存档（复用 brief/日报，与日报联动）     │
│     ← RSS 爬取入库（现有公平调度 + 域名冷却管线，不变）                       │
└───────────────────────────────────────────────────────────────────────┘
```

---

## 3. 知识骨架（ADR-0004 的落地）

### 3.1 骨架的物理形态（S1 + W1）

骨架不引入新存储，就活在 vault markdown 里：
- `vault/<领域>/_index.md`（`pkb_map`）= 领域知识体系树（根），frontmatter 新增 `root_concepts`（顶层主题）。
- `vault/<领域>/topics/<主题>.md`（`pkb_topic`）= 主题 MOC（中），承载该主题的局部树。
- 树的每个节点要么链到一张已填原子卡（`[[concept]]`），要么标 `[缺口]`。

`digest` 独占写这些文件（W1）：复用原子计划护栏 6（先写 `.next.md` 校验 → 快照旧版到 `digest/` → 原子替换）。人不手改，只 `spool sync pull` / Obsidian 同步下来查看。

> 与原子计划的衔接：原子计划 §2.2 的 `digest.v2.md` 已经在生成 `_index.md` 的「知识树」章节。本计划把它的语义从「LLM 凭卡片摘要猜层级」**收紧**为「LLM 在**已声明关系边**（一般化↔特例定父子、前置↔后续定先后）+ **人调骨架大方向**约束下渲染树」。提示词增量见 §3.4。

### 3.2 初次生成（A1 + B3）

新领域上线：人在 `config/pkb/domains.yaml` 给领域加一句话 `scope`（大方向）。`pkb-curate skeleton <领域>`（新子命令）：
1. 用 `skeleton_model`（顶级推理档）按 scope 生成多层目标概念树；
2. 可选喂入权威源 TOC（MS Learn / Oracle Docs / OWASP，B3 校正参考）；
3. 经护栏 6 落 `_index.md` + 各达标主题的 `topics/<主题>.md`；
4. 所有节点初始为 `[缺口]`，等缺口填充与涌现卡归位。

### 3.3 持续生长（涌现回流 + 影响半径闸）

- **归位**：每轮 digest 用最新骨架对所有原子卡（含 RSS 涌现卡、缺口填充卡）做匹配（`match_model` 快强档），挂到节点。挂不上的进**待归位区**。
- **回流提议**：待归位卡积成同主题簇 → LLM 提议给骨架加节点 / 调层级。
- **影响半径闸**（Q12 / ADR-0004）：提议落地前算 `影响半径` = 触及的已挂卡片数。
  - ≤ `skeleton_change_approval_threshold`（可配）→ 小动作，快照后自动应用（含加/小范围删合并）。
  - > 阈值 → 大动作（删/合并/重构），生成一条 Matrix 待批提议（`!pkb approve <id>` / `reject <id>`），批准后才应用。
- 过渡期（前端未到）：用上述 Matrix 闸 + config 大方向掌舵；前端到位后接管「批准/设大方向/剪节点」。

### 3.4 提示词增量（config/pkb/prompts/，沿用外置可调）

| 提示词 | 动作 | 要点 |
|--------|------|------|
| `digest.v2.md` | 改 | 「知识树」章节：层级**优先照搬已声明关系边 + 当前骨架结构**，禁止凭空臆造；接收 `{{existing_skeleton}}` 增量更新不重写 |
| `skeleton.md` | 新增 | 按领域 scope（+可选 TOC）生成多层目标概念树；输出可被 digest 渲染为 `_index.md`/`topics` 的结构 |
| `skeleton_propose.md` | 新增 | 输入待归位卡簇 + 当前骨架，输出结构变更提议（加/删/合并/重构 + 影响节点） |
| `match.md` | 新增 | 卡片 ↔ 骨架节点匹配判定（归位），输出目标 concept 或「待归位」 |

---

## 4. 缺口填充（Q6/Q7/Q10/Q11）

### 4.1 独立子命令

`pkb-curate fill [--domain <领域>]`：与 RSS 驱动的 `pkb-curate` 分开（不同节奏、不同限流、不同观测）。可挂独立 cron。

### 4.2 填充循环（单个缺口）

```
1. LLM 起草（gapfill_model 顶级推理档）：
     - 该缺口自评易变性 + 置信度
     - 写原子卡草稿（沿用原子计划 v2 四章结构）
     - 提议 1-2 个权威源 URL
2. V2 核实（Q7）：
     - 用 ExtractorService.Extract 抓提议源（G3：先查 next_allowed_at，冷却中跳过/排队）
     - verify_model（快强档）校验该页是否支撑卡片
     - 支撑 → source: crawled/verified，置信度 高
     - 抓不到/不支撑 → source: unverified（有引用未核实，中）或 llm-only（无引用，低）
3. 归位：写卡 + 挂到该缺口节点（缺口 → 已填）
4. 低置信卡进 audit 计数（网健康度指标）
```

> 前沿/易变缺口（步骤 1 自评为时效性）跳过「LLM 起草」直接走定向爬：Extract 抓权威源 → 走现有 reconstruct 原子化。这是 F2 路径，与 F1 在步骤 2 之后合流。

### 4.3 配额与优先级（Q11，全配置）

| 配置项 | 默认 | 用途 |
|--------|------|------|
| `gap_fill_per_run` | 10 | 每领域每轮最多填几个缺口（可改） |
| `gap_fill_order` | breadth | 自顶向下广度优先：先填根/主题层缺口 |
| `gap_fill_enabled.<领域>` | 见下 | 每领域开关（打样：先只开 C#） |
| `gap_fill_enabled_all` | false | 一键总开关 |
| `gap_fill_default` | false | 新领域默认开/关 |

### 4.4 改动点

- `internal/pkb/`：新增 `skeleton.go`（生成/渲染/归位/提议）、`gapfill.go`（填充循环）。
- `internal/pkb/client.go`：新增对 `ExtractorService.Extract` 的调用入口（HTTP 端点或进程内，依 pkb-curate 现有调用方式）。
- `cmd/bellkeeper/main.go`：`pkb-curate` 加 `skeleton` / `fill` 子命令。
- `internal/pkb/domains.go` + `config/pkb/domains.yaml`：新增上述配置项 + Go 字段 + 默认值（随消费方 Phase 落地，无死配置）。

---

## 5. 资讯库（ADR-0005 的落地）

### 5.1 形态（Q13）

- 落 `资讯/<领域>/<YYYY-MM-DD>.md`，一天一文件，只追加不删。
- 复用现有 brief/日报机制（K08 20:00 资讯摘要 + DailyReportService），从「推完即逝」改为「分领域分日持久化」；与日报联动（日报可链/聚合当日各领域资讯文件）。
- 资讯条目**不存独立原子卡**（控 vault 膨胀），只活在每日存档。
- `资讯/` 目录加入 `collectDigestCards` 排除集（不污染知识库 digest 候选）。

### 5.2 晋升闸

- 资讯综述生成时，`promote_model`（快强档）识别耐久知识点（原理/方法/模式，非事件），由 `durability`+`novelty` 把闸。
- 通过的知识点 → 当作一个「缺口填充请求」走 §4.2（含 V2 核实），晋升为知识库正经原子卡并归位。
- 事件性资讯不晋升，留资讯库自然过期。

---

## 6. 可观测（扩展原子计划 audit）

`pkb-curate audit` / `audit_on_run` 新增指标（复用 audit.go 的建图能力，只读）：
- **待归位率**：未挂任何骨架节点的卡占比（Q4）。
- **缺口覆盖率**：骨架已填节点 / 总节点。
- **低置信卡占比**：`llm-only` + `unverified` 卡占比（Q7）。
- **骨架结构健康**：超级 hub 节点（应拆为主题 MOC）、断链（concept slug 漂移）。

---

## 7. 模型路由（Q14，扩展 domains.yaml）

| 任务 | 配置键 | 档位 | 说明 |
|------|--------|------|------|
| 骨架生成 | `skeleton_model` | 顶级推理（pool-reason） | 画整棵树，高价值低频 |
| 缺口起草卡 | `gapfill_model` | 顶级推理 | 写正经原子卡 |
| V2 核实 | `verify_model` | 快强（pool-fast） | 「页面是否支撑」高频判断 |
| 归位匹配 | `match_model` | 快强 | 卡↔节点匹配 |
| 晋升判定 | `promote_model` | 快强 | 耐久性判断 |

全部走 llm_jobs 队列（ADR-0002 批处理场景）；值映射到模型组，由 LLM Proxy 解析到具体档位（deepseek-v4pro / flash 等），provider 无关、可随时改 config。

---

## 8. 实施路线

### 8.1 依赖

```
Phase F (骨架生成+渲染+归位) ──→ 原子计划（已落地）
Phase G (缺口填充)          ──→ Phase F + ExtractorService（已存在）
Phase H (资讯库+晋升闸)      ──→ Phase F（晋升复用缺口填充）
Phase I (调方向前端)         ──→ Phase F（形态待定，最后做）
```

### 8.2 分阶段

| Phase | 范围 | 验收要点 |
|-------|------|---------|
| **F：知识骨架** ✅已完成(2026-06-17, commits 3291f6d/6bc31a0/7e3c1f6/16f02b2，本地未推) | `skeleton.go` 生成/渲染/归位/提议 + `digest.v2.md` 收紧 + `skeleton`/提议提示词 + 待归位区 + 影响半径闸（含 Matrix 过渡） + 配置项 | 给 C# 播大方向能生成多层骨架；存量卡全量归位；挂不上的进 `_待归位.md`；大动作走 Matrix 批准、有快照可回滚 |
| **G：缺口填充** ✅已完成(2026-06-17, commits 9942c79/c45da39/aef08d2/32b7858，本地未推) | `gapfill.go` 填充循环 + V2 核实（Extract+verify）+ G3 冷却让路 + 配额/开关 + 出处/置信度落 frontmatter | C# 打样：`pkb-curate fill programming` 自顶向下补缺口；卡带 `source`/`verification`/`confidence`；冷却域名被让路；低置信卡进 audit |
| **H：资讯库+晋升** ✅已完成(2026-06-17, commits f9fdd7b/e696edd/59c4ee5/55e917b，本地未推) | 分领域分日落盘 + 与日报联动 + `资讯/` 排除 + 晋升闸 | `资讯/<领域>/<日期>.md` 每日生成；耐久知识点晋升为知识库卡（走 V2）；事件资讯不晋升 |
| **I：调方向前端** | 窄掌舵面（批准提议/设大方向/剪节点）——**形态待用户定，本计划不细化** | 待定 |

### 8.3 验收自检（CLAUDE.md §3 + 原子计划清单的增量）

```
☑ go build / go vet 绿；新包有真实调用方（导入 > 0）                      [Phase F ✅]
☑ digest 不再自行猜树：层级源自已声明关系 + 骨架（ADR-0004 落实）          [Phase F ✅ Slice3]
☑ 骨架 md 机器独占写：人不手改路径，快照在替换前执行（W1+护栏6）          [Phase F ✅]
☑ 缺口填充卡必带 source/verification/confidence；V2 真去 fetch（grep 验证调 Extract）   [Phase G ✅ G-2]
☑ G3：抓取前查 next_allowed_at；失败不进 crawl_failures，只降级卡          [Phase G ✅ G-2]
☑ 影响半径闸：大动作走 Matrix、小动作快照后自动应用                        [Phase F ✅ Slice4]
☑ 资讯不进骨架、不存独立卡；资讯/ 已排除出 digest 候选                      [Phase H ✅ H-1]
☑ 晋升走缺口填充同一 V2 路径（不旁路核实）                                 [Phase H ✅ H-3]
☑ 新增配置项随消费方 Phase 落地（无死配置）；全配置驱动可调方向            [Phase F ✅]
☑ 模型路由：生成顶级档 / 判断快强档，走 llm_jobs 队列                      [Phase F ✅ skeleton_model 顶级 / match_model 快强]
```

> Phase F 落点速查：`pkb-curate skeleton <域>`（生成骨架）/ `match <域>`（归位）/ `propose <域>`（涌现回流提议+闸）/ `proposals list|approve|reject <id>`（CLI 审批）；Matrix `!pkb list|approve|reject <id>`（过渡审批）。digest 每轮自动归位。骨架结构真相源活在 `vault/<域>/_index.md`，待批提议在 `vault/_提议/<id>.md`。
> Phase G 落点速查：`pkb-curate fill <域>`（缺口填充，独立子命令，可挂独立 cron；`--per-run`/`--dry-run`）；新增 server `POST /api/files/extract`（bare 抽取不入库，供 V2 核实）。卡落 `source` / `verification`(verified/unverified/llm-only) / `confidence`(high/medium/low)；F1 稳定缺口起草+核实，F2 易变缺口(volatility=volatile 且有源)定向爬走 reconstruct。开关在 `domains.yaml` 的 `gap_fill_enabled.<域>`（打样开 programming）；模型路由 `gapfill_model`(顶级,回退 skeleton_model)/`verify_model`(快强,回退 match_model)。
> Phase H 落点速查：`news` 领域配 `feed: true` 成资讯库容器（digest/audit 领域遍历跳过、日报卡统计排除 `vault/资讯`——领域级跳过等价「资讯/ 排除出 digest 候选」）；`pkb-curate feed`（资讯库生成，独立子命令可挂 cron；`--date`/`--dry-run`/`--no-promote`）遍历 raw+archive 取当日资讯类(`pkb_type∈feed_content_types`)→按 `pkb_domain` 分领域 `promote_model` 综述→落 `vault/资讯/<领域>/<日期>.md`(type:pkb_feed，不存独立卡)。晋升闸：`promote_model` 识别耐久知识点→`shouldPromote` 把闸(非事件 && durability≥`promote_durability_min`)→复用 `fillOneGap` 走同一 V2 路径晋升+`placeCardsOntoSkeleton` 归位、标 `pkb_promoted`；事件性不晋升。日报「今日资讯存档」节弱联动(`FeedArchivesByDate`)。配置 `promote_model`(快强,回退 match_model)/`feed_content_types`(默认 news,release)/`promote_enabled`/`promote_durability_min`(默认7)。
> **未运行时验证**（vault 在生产 keeper，本地无 LLM）：build/vet/test 全绿、契约测试守住格式，实际生成质量待部署后 `pkb-curate skeleton programming` 自验。

---

## 9. 风险与回滚

| 风险 | 缓解 |
|------|------|
| 首次为已有领域生成骨架后存量卡全量归位量大/误归位 | 归位 `match_model` 判定 + 待归位区兜底（误归不丢，进待归位）；快照可回滚 _index |
| 缺口填充 LLM 编造引用（伪 verified） | V2 强制真 fetch + 校验支撑；抓不到一律降级，不允许「自报即 verified」 |
| 缺口填充成本失控 | `gap_fill_per_run` 限额 + 每领域开关 + 判断走快强档；打样单领域先验证 |
| 骨架自生长跑偏 | 影响半径闸把大动作交人批准；快照可回滚；config 大方向收口 |
| 资讯库撑大 vault | 资讯不存独立卡、只每日存档；只追加不删但单文件粒度小 |
| 骨架 md 被人误手改 | W1 约定 + 下轮 digest 覆盖前快照；前端到位后彻底无需手改 |

**回滚**：Phase F-H 各自 Git revert；骨架/资讯文件可手动清理或从 `digest/` 快照恢复；缺口填充/晋升停用 = 关 `gap_fill_enabled_all` / 移除晋升步骤（配置级，不重编）。

---

*本文件随实施推进更新；每完成一 Phase 回写验收勾选并在 [STATUS.md](STATUS.md) 追加主线动作。需求层面变更回 [ROADMAP.md](ROADMAP.md)。*
