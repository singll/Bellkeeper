# PKB 原子知识网改进计划

> **定位**：[PKB-IMPLEMENTATION.md](PKB-IMPLEMENTATION.md) 的下一代改进计划。PKB-IMPLEMENTATION 是 MVP 实施文档（漏斗筛选 + 原子化重构 + 体系化合成的骨架），本文件是**骨架上的质量升级**——从「总结性卡片堆」进化为「原子知识网」。
>
> **触发**：vault 卡片实际使用中暴露的质量问题——卡片是总结性的、重复的、无体系的，无法形成可导航的知识网络。
>
> **适用分支**：`feat/pkb-atomic-knowledge`（或从当前工作分支切出）。

---

## 0. 问题诊断

> 基于 2026-06-09 的代码与提示词实测分析。所有「现状」结论均带文件锚点，实施时以代码为准。

### P1：重构提示词引导"总结"而非"原子提取"

**现状**：`config/pkb/prompts/reconstruct.md` 要求 LLM 生成 4 个固定章节——`核心洞察` / `关键技术要点` / `深度摘要` / `关联`。

**问题**：
- 「深度摘要」章节名直接引导 LLM 做**全文压缩复述**，而非提取独立原子知识。
- **一篇文章只生成一张卡片**（`reconstructCard()` 返回单个 `string`），导致一张卡片塞满多个不相关知识点，无法拆分、无法独立引用。
- 没有 `atomic_concept` / `card_type` 等结构化字段，LLM 缺乏"一个知识点 = 一张卡"的约束信号。
- 结果：卡片 = 文章缩写版，读卡片不如读原文。

**锚点**：
- 提示词：`config/pkb/prompts/reconstruct.md:6-10`（四个固定章节）
- 代码：`internal/pkb/reconstruct.go:12`（`reconstructCard` 返回单 string）
- 验证：`internal/pkb/reconstruct.go:79-104`（`validateCard` 只校验章节存在，不校验原子性）

### P2：无语义去重/补充机制

**现状**：PKB 的去重只有三层隐式机制：
1. URL 去重（`ArticleTagRepository.GetByURL()`）
2. 内容哈希去重（`ArticleTagRepository.GetByContentHash()`）
3. PKB 幂等账本（`PkbStatus='processed'` 防重处理）

**问题**：
- **没有语义级别的去重**——两篇不同来源的文章讲同一个知识点，会生成两张高度重叠的卡片。
- 新卡片与已有 vault 卡片之间没有"是否重复"或"是否可补充"的判断。
- wikilink 候选检索只搜标题（`SearchTitles`），query 是 `art.Title+" "+domain.Display`（`curator.go:295`），太粗糙，无法发现内容级重叠。
- 相近知识无法合并/补充到已有卡片，而是生成一张新的平行卡片。

**锚点**：
- 候选检索 + 数量：`internal/pkb/curator.go:295`（SearchTitles query 拼接 + limit=8 太少）
- 去重边界：`internal/repository/article_tag.go`（仅 URL/hash 去重）

### P3：Digest 是"流水账"而非知识体系

**现状**：`config/pkb/prompts/digest.md` 要求生成 5 个章节——`本期核心变化` / `主题簇` / `值得沉淀的知识` / `缺口与后续问题` / `关联卡片`。

**问题**：
- 章节结构偏向**时间线式罗列**（"本期核心变化"），不是知识结构。
- 没有要求构建**知识体系树**——即"这个领域的知识结构是什么，每个节点对应哪些卡片"。
- Digest 每次生成新文件（`vault/<领域>/digest/YYYY-Wxx_领域周综述.md`），不更新已有体系文件，导致体系碎片化。
- Digest 与卡片之间只有平面的 wikilink，没有层级/父子/依赖关系。
- 用户期望：看一篇体系主文件就能看出整个领域的知识全貌和结构。

**锚点**：
- Digest 提示词：`config/pkb/prompts/digest.md:4-7`（五个章节）
- Digest 写入：`internal/pkb/digest.go:238-248`（每次新建文件，不更新已有）
- Digest 验证：`internal/pkb/digest.go:284-308`（validateDigest 只校验章节存在）

### P4：novelty 权重 = 0 导致重复内容进 vault

**现状**：`config/pkb/domains.yaml` 的打分权重为 `relevance:0.35 / depth:0.25 / actionability:0.25 / durability:0.15 / novelty:0.0`。

**问题**：
- novelty（新颖性）权重为 0，意味着**完全重复常识的文章**只要相关+深+可执行也能进 vault。
- 这与"重复的不加入"的需求直接矛盾。
- 5 维权重之和为 1.0，novelty=0 等于放弃了"这篇是否带来新知"的判断维度。

**锚点**：`config/pkb/domains.yaml:8-13`（权重配置）

> 补充洞察（2026-06-10 复核）：打分提示词 `score.md:8` 的 novelty 维度**本身已是硬约束**——已明确写"重复常识给 0–3"。所以 novelty"形同摆设"的根源**不在提示词，纯粹在权重 = 0**（`FinalScore` 用 `w.Novelty*novelty`，[score.go:22-27](../internal/pkb/score.go#L22-L27)，权重 0 直接抹掉该维度贡献）。**激活 novelty 只需 Tier 5 改一个权重，无需动 score 提示词的 novelty 部分**——这修正了原 §2.3「把 novelty 从摆设变硬约束」的归属，详见 Tier 1 §2.3。

### P5–P8：网状结构的四个缺失（本轮新增诊断 · 用户核心诉求所在）

> 上面 P1–P4 是「卡片本身不够原子」的问题。但用户要的是「**网状知识数据库**」——即便 P1–P4 全部修好，按现有 Tier 设计实施完，产出的仍是一棵**单向辐射树**而非网。以下是「网」这一维度的根因诊断，是本版深化的重点。

#### P5：链接是单向辐射，不是双向网

**现状**：卡片关系只在**新卡建立时**单向写入——`reconstructCard` 从候选里挑老卡，在「## 关联」章节写 `A → [[B]]`（`reconstruct.md:10-14`）。

**问题**：
- 老卡 `B` 永远不会因为新卡 `A` 的出现而更新——`reconstructToVault` 只写新文件，从不回写候选卡（`curator.go:301-313`）。
- 关系类型只存在于 `A` 的正文里；`B` 端只有 Obsidian 反链面板能看到「`A` 链接了我」，**但看不到「`A` 把我当前置知识」**——关系类型在反向丢失。
- 网只能从新节点向老节点单向辐射，老节点不会主动长出指向新节点的边 → 拓扑上是**放射状树**，不是网。

**锚点**：单向写入 [curator.go:301-313](../internal/pkb/curator.go#L301-L313)；关系无类型 [reconstruct.md:13](../config/pkb/prompts/reconstruct.md#L13)（仅"一句话说明关系"）。

#### P6：卡片身份不稳定 → 链接易碎

**现状**：文件名 = `YYYYMMDD_{sanitize(标题)}.md`（`curator.go:310`），wikilink 候选与校验都按**标题字符串**匹配（`pruneWikilinks` 按 title 比对，`reconstruct.go:65-72`）。

**问题**：
- LLM 每次提炼标题措辞可能不同；用户在 Obsidian 里改标题；同名标题文件冲突——任何一种都会让已建立的 `[[标题]]` 边变成死链或指错。
- 网的所有边都挂在「标题字符串」这根脆弱的锚上，标题一漂移，边就断。
- Tier 3 原计划把文件名从「标题」改成「`atomic_concept`」，但 `atomic_concept` 同样是 LLM 生成、会漂移的字符串，**没有解决身份稳定性的根本问题**。

**锚点**：文件名取标题 [curator.go:310](../internal/pkb/curator.go#L310)；按 title 匹配 [reconstruct.go:65-72](../internal/pkb/reconstruct.go#L65-L72)。

#### P7：缺中间聚合层（MOC），领域知识树会爆炸

**现状**：vault 只有两个粒度——领域目录（`vault/编程/`）与原子卡。Digest 把整个领域的知识树塞进一个文件。

**问题**：
- 一个领域累积几百张卡后，单文件知识树会爆炸且很快过期，违背「看一篇就懂全貌」的目标。
- 缺少 Zettelkasten / PKM 公认的**中间层**——结构笔记 / MOC（Map of Content）/ 主题页：把同主题卡片聚合成一张导航页。
- **关键发现**：代码其实**已为此预留目录位**——`collectDigestCards` 已显式排除 `digest/`、**`maps/`、`topics/`** 三个子目录与 **`_` 前缀文件**（即 `_index.md`），但文档原 §5.3 只说"已排除 digest/"，既不精确、也完全没利用这个现成钩子。

**锚点**：预留的 MOC 目录排除逻辑 [digest.go:148-151](../internal/pkb/digest.go#L148-L151)。

#### P8：网会退化，但没有任何可观测

**现状**：`pkb-curate` 运行结束只打印分流计数（处理 N / vault M / archive K / discard L，`curator.go:183-184`）。

**问题**：
- 网状库的天敌全程无人度量：**孤儿卡**（0 链接）、**断链**、**重复概念**（多卡讲同一 concept 却各自为政）、**超级 hub**（被过度连接的节点）。
- §0.3「AI 主导、人调方向」需要数据支撑，但当前没有任何「网结构健康度」信号让用户判断该往哪调。

**锚点**：仅有分流计数、无网结构统计 [curator.go:183-184](../internal/pkb/curator.go#L183-L184)。

### 本轮审查结论：方向正确，但需要补保险丝 + 补「网」的脊梁

原方案的大方向没问题：用提示词把「总结卡片」改成「原子知识卡片」，再通过语义查重和体系主文件把 vault 组织起来。但有两类待补强：一是直接按原文实施有几处容易踩坑（保险丝，见下表）；二是 P5–P8 揭示的——原方案**只解决了「卡片够不够原子」，没解决「卡片之间够不够网状」**，需要新增 §1.3「网状知识模型」作为脊梁，并据此强化各 Tier。

| 风险点 | 原方案问题 | 本次优化 |
|--------|------------|----------|
| Phase A 标称零代码 | 新章节会被当前 `validateCard()` 拦截，实际不能只切提示词 | 改为「提示词 + 兼容校验」阶段，先双栈 validator，再切 registry |
| 去重靠重构 prompt 自觉 | prompt 要求「重叠不建卡但在关联章节注明」，模型可能仍输出一张“重复说明卡” | 增加独立的 `atomic_plan` manifest：先判定 `create/supplement/skip`，再重构 |
| `supplement` 类型不一致 | §3 引入 `supplement`，但 §2 的 `card_type` 枚举没有它 | 把 supplement 纳入枚举，并规定必须有 `supplement_to` |
| 一文多卡缺少写入护栏 | 部分卡失败、文件名冲突、重复运行时覆盖都未定义 | 增加逐卡校验、唯一文件名、临时文件 + rename、partial 状态 |
| `_index.md` 覆写偏冒险 | 直接覆写可能损坏体系主文件 | 先写 `_index.next.md` 并验证，通过后快照旧版再原子替换 |
| 评分新字段不兼容 | `score.v2.md` 新增 `atomic_potential`，但 `ScoreResult` 还没有字段 | 明确 Go 结构体同步扩展，旧字段解析保持兼容 |
| 链接单向（P5） | 关系只单向写、反向丢类型，网退化成放射树 | 同源多卡同批**成对**写双向链接；跨卡反向交 Obsidian 反链 + Dataview，并定义**逆关系语义表**（§1.3.2）|
| 身份漂移断链（P6） | 标题/概念措辞一变，`[[]]` 边即断 | 链接锚定 `atomic_concept` slug + frontmatter `aliases` 容错，提示词强约束概念命名稳定（§1.3.3）|
| 领域树爆炸（P7） | 单 `_index.md` 塞满整领域，很快过期 | 分层 MOC：领域 `_index.md`（根）→ 主题 `topics/<主题>.md`（中）→ 原子卡（叶），复用代码已留目录位（§1.3.5 / Tier 4）|
| 网不可观测（P8） | 孤儿/断链/重复/hub 无人发现 | 新增 Tier 6：Dataview 模板（零代码）+ `pkb-curate audit` 只读审计（§7）|

---

## 1. 改进目标与网状模型

### 1.1 改进目标

| 维度 | 现状 | 目标 |
|------|------|------|
| **原子性** | 一篇文章 = 一张总结性卡片 | 一个独立知识点 = 一张原子卡片；一篇文章可产出多张 |
| **去重性** | 只有 URL/hash 去重 | 语义级去重；重复不建卡，相近可补充/归并到已有卡片 |
| **体系性** | Digest 是时间线流水账 | 分层 MOC = 知识树 + 脉络 + 缺口；看一篇即知全貌 |
| **可导航** | 卡片间只有平面 wikilink | 层级关系（父子概念）+ 关系类型（前置/互补/对比）+ 体系树 |
| **双向性** | 链接单向辐射（P5） | 关系双向可见、带类型：同源成对写 + Obsidian 反链/Dataview |
| **身份稳定** | 标题字符串即身份（P6） | `atomic_concept` slug 锚定 + `aliases` 容错，措辞漂移不断链 |
| **可观测** | 只打印分流计数（P8） | 网健康度可量：孤儿率 / 断链 / 度分布 / 重复概念 / hub |

**一句话目标**：把知识库做成一个巨大的知识体系网，每个点都是网的一个节点，看体系主文件就能看出知识点之间的关联和全貌。

### 1.2 设计护栏（比 prompt 更硬的约束）

这次升级的关键不是把提示词写得更漂亮，而是让失败模式可控。后续所有 Tier 都遵守以下护栏：

1. **先计划，后写卡**：LLM 先输出结构化 `atomic_plan`，逐个知识点标记 `create` / `supplement` / `skip`；只有通过计划校验的条目才进入 Markdown 重构。
2. **宁可补充，不要误删**：去重置信度不够时不允许直接 `skip`，降级为 `supplement` 或 `create`；只有硬重复或高置信重复才跳过。
3. **写盘必须幂等**：文件名冲突不覆盖，使用唯一后缀；先写临时文件，校验通过后 `rename`，避免 Obsidian/LiveSync 看到半成品。
4. **部分成功可追踪**：一篇文章多卡时，单卡失败不阻断其他卡；raw frontmatter 记录 `pkb_cards_written` / `pkb_cards_skipped` / `pkb_reconstruct_errors`。
5. **旧版可并行回退**：validator 在过渡期同时接受旧版和 v2 章节；旧 prompt 文件保留，registry 可一键切回。
6. **体系主文件不裸写**：`_index.md` 刷新先生成 `_index.next.md` 并验证，再快照旧版并原子替换。
7. **逆关系成对，不靠猜**：每条关系都有确定的逆关系（§1.3.2 表）。同源批次内成对写两端；跨批次的反向由 Dataview 按逆关系表推导展示，不让 LLM 重新判断方向。
8. **身份锚定 concept，不锚标题**：链接目标统一是 `atomic_concept` slug，配 `aliases` 容错；标题/措辞可变，链接锚不变。
9. **audit 只读，绝不改盘**：网健康度审计（Tier 6）只扫描、只报告，不移动/不写卡/不删链——可观测与维护写操作严格分离。
10. **Dataview/Obsidian 优先于自研**（§0.2）：能用 Obsidian 反链 + Dataview 查询零代码实现的网视图（孤儿、反向关系、重复），就不写 Go 代码；CLI 只做 Obsidian 查不了或需跨文件聚合统计的部分。

### 1.3 网状知识模型（本版脊梁）

> 这是本版深化的核心——把「网」从模糊愿景固化成一个精确的图模型。后续每个 Tier 都服务于这个模型里的某条边或某层节点。**先有模型，再谈提示词与代码**。

#### 1.3.1 三层节点（粒度分层）

| 层 | 节点 | 文件 | type 字段 | 角色 | 由谁生成 |
|----|------|------|-----------|------|---------|
| 叶 | **原子卡** | `vault/<领域>/<concept-slug>.md` | `pkb_card` | 一个独立、可复用、可独立引用的知识点 | Tier 1 + Tier 3 重构 |
| 中 | **主题 MOC** | `vault/<领域>/topics/<主题>.md` | `pkb_topic` | 聚合同主题卡片的导航页（结构笔记） | Tier 4 digest |
| 根 | **领域 index** | `vault/<领域>/_index.md` | `pkb_map` | 该领域知识全貌：知识树 + 脉络 + 缺口 | Tier 4 digest |

> 三层之间用 wikilink 上下贯通：领域 index 链向主题 MOC，主题 MOC 链向原子卡，原子卡之间横向互链。这就是「点—网—全貌」三个尺度。

#### 1.3.2 边类型与逆关系表（让网可双向、可推导层级）

6 种关系类型，每种有明确的**方向性**与**逆关系**。这张表是双向回填（§1.3.4）和知识树自动推导（Tier 4）的形式基础：

| 关系类型 | 方向 | 逆关系 | 网语义 |
|---------|------|--------|--------|
| 前置知识 | 有向 | 后续延伸 | **树边**（学习路径 DAG：先学这个）|
| 后续延伸 | 有向 | 前置知识 | **树边**（学习路径 DAG：再学那个）|
| 一般化 | 有向 | 特例 | **树边**（is-a 分类层级：父概念）|
| 特例 | 有向 | 一般化 | **树边**（is-a 分类层级：子概念）|
| 互补视角 | 无向（对称）| 互补视角 | 网边（横向关联，二者合看更完整）|
| 对比参照 | 无向（对称）| 对比参照 | 网边（横向关联，二者差异有启发）|

**两类边的不同命运**：
- **树边**（前置/后续、一般化/特例）构成有向层级。Tier 4 据此**自动推导知识树**——而不是让 digest LLM 凭空猜哪个是哪个的父节点。
- **网边**（互补/对比）是横向无向关联，构成「网」的横向连接，不进树、只在卡片间与 Dataview 视图里体现。

#### 1.3.3 身份模型：slug 锚定 + aliases 容错（用户决策：不引硬 ID）

- **链接锚 = `atomic_concept`**：中文概念名（Obsidian 原生支持中文文件名与链接）。文件名 = `sanitize(atomic_concept)`，wikilink = `[[atomic_concept]]`；候选检索与 `pruneWikilinks` 校验都从「按 title」改为「按 concept」匹配。
- **稳定性靠两条护栏**：① 提示词强约束 concept 命名规范——名词性、最小完整、跨文章同一知识点应收敛到同一称呼（Tier 1 §2.1）；② frontmatter `aliases: [历史措辞, 同义说法]`，Obsidian 解析 `[[]]` 时也会命中 alias，标题措辞微调不断链。
- **重名处理**：concept slug 冲突时 curator 加最小数字后缀（`concept-2`），并把裸 concept 写进该卡 `aliases`，保证文件名唯一且旧链接仍可命中。**`atomic_concept` frontmatter 字段保持原值不变**（不加后缀）——它是语义身份锚，加了后缀就变成了另一个概念；文件名唯一性靠 slug 后缀解决，concept 字段靠 aliases 容错。Dataview 按同一 `atomic_concept` 归并卡片簇时，多张 concept 相同的卡自然聚合（这正是 supplement 簇的归并机制）；audit 检测"重复概念"时也按 `atomic_concept` 字段值判断，不会因后缀而误判为不同概念。
- **为何不引硬 ID**（§0.1 + 用户决策）：数字/时间戳 ID 会让文件名变丑、偏离 Obsidian 中文 vault 直觉、已生成卡片需迁移；slug + alias 已能覆盖绝大多数漂移场景。硬 ID 列为**触发式路线图项**——当 Tier 6 审计实测断链率过高、slug 方案兜不住时再引入。

#### 1.3.4 双向性策略：成对写入 + Obsidian 反链（用户决策：零代码方案）

三档协作，按成本从低到高；**MVP 只用前两档**：

1. **同源成对写入（零代码 · 本轮做）**：同一篇文章产出的多张卡（Tier 3）天然同批写入——直接在两端互写 wikilink 且关系类型互逆（A 写 `[[B]]：前置知识`，B 写 `[[A]]：后续延伸`）。这部分双向是确定的、无并发风险。
2. **Obsidian 反链 + Dataview（零代码 · 本轮做）**：跨批次（新卡 ↔ 老卡）**不回写老文件**；老卡端用 Obsidian 原生反链面板看到入边，再用一段 Dataview 模板（Tier 6）按逆关系表把「谁把我当作前置/特例/对比」显式列出来，补回反向的关系类型。
3. **`pkb-weave` 主动回填（路线图触发项 · 本轮不做）**：低频批量把逆边实际写回目标卡文件。代价是要改已存在的老文件（LiveSync 并发 / 半成品风险），仅当 Dataview 方案确实不够用时才做。

#### 1.3.5 分层 MOC 物理结构（复用代码已留目录位）

```
vault/<领域>/
├─ <concept-slug>.md      # 叶：原子卡（Tier 1+3）
├─ topics/<主题>.md       # 中：主题 MOC（Tier 4） —— 已被 collectDigestCards 排除
├─ _index.md             # 根：领域知识体系（Tier 4） —— _ 前缀已被排除
├─ maps/                 # 预留：跨领域知识地图 —— 已被排除，本版暂不启用
└─ digest/               # 历史快照 —— 已被排除，不参与节点
```

> `topics/`、`maps/`、`_` 前缀、`digest/` 这四类**都已在 [digest.go:148-151](../internal/pkb/digest.go#L148-L151) 被 `collectDigestCards` 排除**。这意味着 Tier 4 写 MOC / index 时**不必再改排除逻辑**，直接落文件即可，且这些聚合节点不会污染下一轮 digest 的候选卡片池。这是原方案漏掉、本版补回的现成钩子。

---

## 2. Tier 1 — 提示词重构 + 兼容校验（P0，低代码风险）

> 不再宣称「零代码」。新提示词会改变章节名和 JSON 字段，必须同步做最小兼容改动：`ScoreResult` 增字段、`validateCard`/`validateDigest` 支持 v1/v2 双栈。registry 切换必须在 `--dry-run` 验收通过后执行。

### 2.1 新版重构提示词（`reconstruct.v2.md`）

**核心变化**：从"文章总结器"变为"原子知识提取器"。

**新版提示词完整设计**：

```
你是一位原子知识提取专家。你的任务是从原始文章中提取独立的、可长期复用的原子知识点，每个知识点生成一张结构化 Obsidian 知识卡片。

核心原则：
1. 原子性：每张卡片只聚焦一个独立的概念、技术、方法、模式或陷阱。如果一个知识点脱离原文仍能被理解和引用，它就是原子的。
2. 独立性：卡片标题 = 知识点本身（不是"XX文章笔记"），让人看到标题就知道这张卡讲什么。
3. 可组合性：卡片之间通过 wikilink 形成知识网络，而非线性罗列。

提取规则：
1. 先识别文章中的独立知识点（通常包含 1-{{max_cards}} 个），每个知识点生成一张独立卡片。
2. 以下内容不要提取为卡片：文章背景介绍、作者观点、新闻事件的时间线描述、泛泛而谈的总结。
3. 以下内容优先提取：可复用的技术原理、可执行的方法步骤、反直觉的发现、易踩的陷阱、跨场景通用的模式。
4. 如果文章所有内容都围绕同一个知识点展开，则只生成一张卡片。
5. 如果某个知识点与下方「已有 vault 卡片候选」高度重叠，不要重复建卡，而是在关联章节注明"与 [[已有卡片]] 重叠，不重复建卡"。

每张卡片的格式要求：
1. 直接输出完整 Markdown（含 YAML frontmatter），不要输出任何额外解释，不要用 ``` 代码块包裹。
2. frontmatter 必须包含：
   - title：知识点本身（精准、可独立引用，不是"XX笔记"）
   - type：固定为 pkb_card（原子卡片）
   - source：原始 URL
   - ingest_date
   - score
   - domains
   - tags
   - atomic_concept：原子概念名 = 这张卡的**稳定身份锚**（wikilink 靠它指向，不是靠 title）。命名规范：名词性短语、最小且完整、跨文章同一知识点必须收敛到同一称呼（例如统一用「JWT 算法混淆攻击」，不要这次叫「JWT alg 混淆」、下次叫「JWT 算法替换攻击」）。
   - aliases：该概念的其它可能称呼 / 同义说法列表（YAML 数组），用于标题措辞漂移时链接仍能命中（容错）。没有则写 `[]`。
   - card_type：从以下枚举选一 — definition（概念定义）/ method（方法步骤）/ pattern（通用模式）/ pitfall（陷阱/反模式）/ comparison（对比分析）/ fact（事实/数据）/ supplement（对已有卡的补充，此时必须再加 supplement_to: [[目标概念]]）
3. 正文必须含四个二级标题章节，顺序固定：
   ## 定义与本质
   一句话定义 + 核心本质。让人读完这一节就知道这个知识点是什么。
   
   ## 关键细节
   不可省略的技术细节、代码片段、命令、参数、关键推导。保留可操作细节，去掉原文的冗余叙述。
   
   ## 适用场景与边界
   什么时候用、什么时候不用、前提条件、已知限制。这是区分"知道"和"会用"的关键。
   
   ## 与其他知识的关系
   只能从下方「候选概念」列表里选择真实存在的概念，使用 [[概念名]]（链接锚是 atomic_concept，不是文章标题）。
   每个链接后标注关系类型，格式：`- [[概念名]]：关系类型 — 一句话说明`
   关系类型从以下选择（括号内是其逆关系，用于判断方向是否写对）：
     前置知识(↔后续延伸) / 后续延伸(↔前置知识) / 一般化(↔特例) / 特例(↔一般化) / 互补视角(↔互补视角) / 对比参照(↔对比参照)
   选「前置知识 / 一般化」这类有向关系时务必确认方向：A 的前置知识是 B，意味着要先理解 B 才能理解 A。
   严禁虚构候选外概念；若候选为空或都不相关，写「（暂无关联）」。
   【同源多卡成对链接】若本次从同一篇文章提取了多张卡（彼此用 ---CARD--- 分隔）且它们之间存在上述关系，请在涉及的每张卡里**成对**写出互逆的两条边（A 卡写 `[[B]]：前置知识`，则 B 卡必须写 `[[A]]：后续延伸`），让同源卡片之间的双向链接在这一次输出里就闭合。

4. 语言与原文一致（中文文章用中文），客观、去营销化，保留可操作细节与关键代码/命令。
5. 禁止写"本文介绍了…""这篇文章讲述了…"式的总结开头。
6. 禁止把多个不相关知识点塞进一张卡片。

多张卡片之间用以下分隔符分隔（独占一行）：
---CARD---

## 元信息（写入每张卡片的 frontmatter）
title 参考：{{title}}
source：{{url}}
ingest_date：{{date}}
score：{{score}}
domains：{{domains}}
tags：{{tags}}

## 已有 vault 卡片候选概念（wikilink 只能从这里选，锚定 atomic_concept）
{{candidates}}

## 原始正文
{{content}}
```

**与旧版对比**：

| 维度 | 旧版 (reconstruct.md) | 新版 (reconstruct.v2.md) |
|------|----------------------|--------------------------|
| 角色 | 知识卡片重构专家 | 原子知识提取专家 |
| 产出 | 一张总结性卡片 | 1-N 张原子卡片（`---CARD---` 分隔） |
| 章节结构 | 核心洞察/技术要点/深度摘要/关联 | 定义与本质/关键细节/适用场景与边界/与其他知识的关系 |
| 链接锚 | 文章标题（易漂移，P6） | `atomic_concept` 概念名 + `aliases` 容错 |
| 关系类型 | 无分类（一句话说明关系） | 6 种关系类型 + 标注**逆关系**方向；同源多卡**成对**写双向边 |
| Frontmatter | title/source/ingest_date/score/domains/tags | +atomic_concept（身份锚）/ +aliases / +card_type（含 supplement） |
| 去重引导 | 无 | 明确要求与候选重叠时不重复建卡 |
| 禁止项 | 短期新闻写成长文 | +禁止总结式开头 +禁止多知识点混装 |

### 2.2 新版综述提示词（`digest.v2.md`）

**核心变化**：从"时间线综述"变为"知识体系地图"。

**新版提示词完整设计**：

```
你是一位知识体系架构师。你的任务是根据同一领域的一组原子知识卡片，构建该领域的**根索引（_index.md）**——一篇让人看一眼就能理解该领域知识全貌和结构的体系地图。它是该领域的最顶层导航，向下贯通到主题 MOC 与原子卡。

核心原则：
1. 体系优先：不是逐条摘要，而是构建一棵知识树，让人看到概念之间的层级和依赖关系。
2. 脉络清晰：标出领域内最重要的 2-3 条知识链路（从基础到进阶、从原因到结果），这些是理解该领域的骨架。
3. 缺口显式：体系中缺少的知识点要明确标出，这些是后续学习的方向。

要求：
1. 直接输出完整 Markdown（含 YAML frontmatter），不要输出任何额外解释，不要用 ``` 代码块包裹整篇笔记。
2. frontmatter 必须包含：title、type（固定为 pkb_map）、domain、period、generated_at、source_cards、root_concepts（顶层概念列表）。
3. 正文必须含五个二级标题章节，顺序固定：

   ## 体系概览
   一段话（3-5 句）描述该领域的知识结构全貌。读完这段话，读者应该能回答：
   - 这个领域有哪几个核心板块？
   - 板块之间的大致关系是什么？
   - 哪些是基础、哪些是进阶？

   ## 知识树
   用缩进列表展示层级关系，每个节点链接到对应卡片的概念。**层级优先依据候选卡片中已声明的关系边**：「一般化↔特例」决定父子（一般化在上、特例在下），「前置知识↔后续延伸」决定同层先后；能从已声明关系还原的结构就照搬，关系缺失处才由你合理归类，不要凭空臆造层级。格式示例：
   - 概念A [[卡片A]]
     - 子概念A1 [[卡片A1]]
     - 子概念A2 [[卡片A2]]
   - 概念B [[卡片B]]
     - 子概念B1 [[卡片B1]]
   
   如果某个概念没有对应卡片（体系缺口），标注为：`- 概念C [缺口]`
   
   ## 核心脉络
   领域内最重要的 2-3 条知识链路。每条链路**沿「前置知识 → 后续延伸」树边**串联，描述从基础到进阶的依赖/递进/因果关系。
   格式：`链路名：[[起点]] → [[中间]] → [[终点]] — 一句话说明为什么这条链路重要`

   ## 新增与变化
   本周期新增了哪些知识点（标注对应的 [[卡片]]），补充/修正了哪些已有知识。

   ## 缺口与探索方向
   体系中缺少的知识点、薄弱环节、值得深入的方向。每个缺口说明"为什么需要它"和"它应该填补知识树的哪个位置"。

4. 「## 知识树」和「## 核心脉络」中只能使用下方候选卡片的 `atomic_concept`（概念名），格式为 `[[概念名]]`。**链接锚是 atomic_concept，不是卡片标题**——概念名是稳定身份锚，标题可能漂移。
5. 严禁虚构候选外卡片；不要编造来源、数据或结论。
6. 语言使用中文，术语保留原文必要英文。

## 元信息
领域：{{domain_display}}（{{domain_name}}）
周期：{{period}}
生成时间：{{generated_at}}
卡片数量：{{card_count}}

## 候选卡片（每卡列出 atomic_concept + 摘要；链接锚定 concept，不是 title）
{{cards}}
```

**`{{cards}}` 渲染格式**：每张候选卡片必须附带其 `atomic_concept`，供 LLM 锚定链接目标。格式为：

```
- atomic_concept: <概念名> | title: <标题> | card_type: <类型> | 摘要: <前 200 字>
```

LLM 在知识树和核心脉络中必须用 `[[<概念名>]]`（即 `atomic_concept`）建立链接，而非用 `title`。这确保链接锚与 §1.3.3 身份模型一致——concept 稳定、title 可漂移。

**与旧版对比**：

| 维度 | 旧版 (digest.md) | 新版 (digest.v2.md) |
|------|-----------------|-------------------|
| 角色 | 结构化合成专家 | 知识体系架构师 |
| type | pkb_digest | pkb_map |
| 核心产出 | 时间线综述 | 领域根索引 `_index.md`（知识体系地图）|
| 章节结构 | 核心变化/主题簇/沉淀知识/缺口/关联卡片 | 体系概览/知识树/核心脉络/新增与变化/缺口与探索方向 |
| 体系树 | 无 | 有（缩进列表 + wikilink + 缺口标注），**依据卡片已声明关系自动还原层级** |
| 脉络 | 无 | 有（2-3 条核心链路，沿前置→后续树边）|
| 缺口 | 有但模糊 | 精确到知识树位置 + 原因 |
| Frontmatter | title/type/domain/period/generated_at/source_cards | +root_concepts |

### 2.3 新版打分提示词（`score.v2.md`）

**核心变化**：新增 `atomic_potential` 字段（预判一文多卡）；novelty 维度**提示词侧已到位**，本 Tier 不改其措辞。

> **澄清（修正原表述）**：现行 `score.md:8` 的 novelty 说明已是硬约束（"重复常识给 0–3"）。novelty"形同摆设"的根因是**权重 = 0**（§0 P4），激活它**只需 Tier 5 把权重调到 0.15**，无需改 score 提示词的 novelty 段。故 score.v2 相对 score.md 的**唯一实质增量是 atomic_potential**。

**修改点**：
- （可选微调）novelty 说明保持现状即可；若要更狠可补一句"与已广泛传播的常识高度重复给 0–2"，但这**不是**激活 novelty 的必要条件。
- 新增输出字段 `atomic_potential`（0–10 整数）：文章包含多少可独立提取的原子知识点（1 个给 1–3，5+ 个给 8–10）。用途：供 Tier 3 预判该文是否值得走一文多卡，以及 `max_cards_per_article` 的软参考。
- 输出 JSON 格式新增：`"atomic_potential": <0-10>`。
- 同步：`ScoreResult` 结构体加 `AtomicPotential int \`json:"atomic_potential"\`` 字段（Tier 3 代码改动；旧 JSON 无此字段时默认 0，向后兼容，[score.go:10-19](../internal/pkb/score.go#L10-L19)）。

### 2.4 Registry 切换

修改 `config/pkb/prompts/registry.yaml`：

```yaml
active:
  score: score.v2.md
  reconstruct: reconstruct.v2.md
  digest: digest.v2.md
```

旧版文件保留在 `config/pkb/prompts/` 目录，不删除，可随时切回。

---

## 3. Tier 2 — 语义去重与补充（P1）

> 需要改 Go 代码。依赖 Tier 1 的新提示词（新 frontmatter 字段 `atomic_concept`）。

### 3.1 重构前语义查重步骤

在 `reconstructToVault()` 中，调用重构 LLM 之前增加一步语义查重：

**流程变更**：

```
现有流程：
  SearchTitles(标题+领域, limit=8) → reconstructCard() → 写盘

新流程：
  SearchTitles(标题+领域, limit=15)           → 标题级候选
  SearchContent(核心段落, layer=vault, limit=5) → 内容级候选（新增）
  合并去重 → 传入重构 LLM 作为"已有知识上下文"
  LLM 在提取时判断：重叠→跳过 / 补充→标记 / 全新→建卡
```

**实现要点**：
- 新增 `Client.SearchContent()` 方法：调用 `/api/files/search` 端点，返回匹配卡片的 `atomic_concept`（无则回退标题）+ 摘要（前 200 字）。
- 将内容级候选传入重构提示词的 `{{candidates}}` 区域，锚定概念（扩展格式：`- atomic_concept（摘要前 200 字）`），与 §1.3.3 链接锚保持一致。
- 提示词已有"与候选重叠时不重复建卡"的引导（Tier 1 已覆盖），无需额外提示词改动。

### 3.2 Supplement 卡片类型与概念归并

**设计**：
- `card_type: supplement` 的卡片在 frontmatter 中新增 `supplement_to: [[目标概念]]`（锚定 `atomic_concept`，与 §1.3.3 身份模型一致）。
- 写入 vault 时，supplement 卡片正常写入独立文件（不修改目标卡片文件，避免文件冲突）。
- Obsidian 中通过 backlink 自然关联——目标卡片的"反向链接"面板会自动显示补充卡片；Tier 6 的 Dataview 模板进一步按 `supplement_to` 把同一概念的补充卡聚合展示。
- 体系主文件（digest）生成时，supplement 卡片与目标卡片归入同一知识树节点。

**概念归并（去重的终点不是"丢弃"，而是"归并到同一身份"）**：
- 语义去重判定为"讲同一知识点的不同侧面"时，正确处理既不是建平行卡、也不是粗暴跳过，而是产出 `supplement` 卡并令 `supplement_to` 指向同一 `atomic_concept`——让两张卡在**概念身份**上归并。
- 同一 `atomic_concept` 的主卡 + 若干 supplement 卡构成该概念的"卡片簇"；主题 MOC（Tier 4）以 concept 为节点聚合整簇，而非把每张补充卡当独立节点，避免网中出现大量近义重复节点（直接缓解 P8 的"重复概念"）。

**选择理由**（遵循 §0.1 奥卡姆剃刀）：
- 不做"自动追加内容到目标卡片"——文件并发写冲突风险高，且破坏 Obsidian 同步。
- backlink + Dataview 是 Obsidian 原生能力，零代码成本，且"补充/归并"关系天然适合反向链接。

### 3.3 SearchTitles 增强

| 参数 | 现状 | 改进 | 理由 |
|------|------|------|------|
| query | `art.Title+" "+domain.Display` | 提取文章前 200 字核心段落 + 标题组合搜索 | 标题太泛，核心段落更能反映内容 |
| limit | 8 | 15 | 更多候选 = 更精准的去重判断 |
| 返回内容 | 仅标题 | `atomic_concept`（作 wikilink 锚）+ 标题/aliases（语义参考） | 链接锚定概念（§1.3.3）；概念比标题更精准反映知识点 |

---

## 4. Tier 3 — 一文多卡 + 同源双向 + 身份落地（P1）

> 需要改 Go 代码。依赖 Tier 1 的新提示词（`---CARD---` 分隔符）。

### 4.1 `reconstructCard` 返回多卡

**现状**：`reconstructCard()` 返回 `(string, error)`——单张卡片。

**改为**：返回 `([]string, error)`——多张卡片数组。

**实现**：
- LLM 输出按 `---CARD---` 分割为多张卡片。
- **超限处理**：若 LLM 输出卡片数 > `max_cards_per_article`（默认 5），只取前 N 张（按 LLM 输出顺序），其余丢弃并在 `pkb_reconstruct_errors` 记录"超出 max_cards 限制，丢弃 N 张"。这避免一篇长文产出过多卡片导致 vault 膨胀。
- **先收集本批次所有卡的 `atomic_concept`，并入 `pruneWikilinks` 的有效候选集**——否则同源卡之间互链的 `[[B]]`（B 是同批新卡、尚未入库、不在"已有 vault 候选"里）会被当死链 prune 掉，§1.3.4 第 1 档的同源双向链接就被破坏了。有效候选 = 已有 vault 候选 concept ∪ 本批次 concept。
- 每张卡片独立执行 `stripCardFence()` + `pruneWikilinks(有效候选集)` + `validateCard()`。
- 任一卡片验证失败不阻塞其他卡片（记录警告，跳过该卡）。

### 4.2 `reconstructToVault` 多卡写入

**现状**：单卡写入 `vault/<领域>/YYYYMMDD_{标题}.md`（`curator.go:310`）。

**改为**：
- 遍历多卡数组，逐卡写入。
- 文件名格式：`{sanitize(atomic_concept)}.md`——**去掉日期前缀，直接用概念名做文件名**（§1.3.3 身份模型）。这样 Obsidian 里 `[[atomic_concept]]` 能直接命中文件名，无需别名转译；日期信息保留在 frontmatter（`ingest_date` / `pkb_scored_at`），不进文件名。
- **重名处理**：目标文件名已存在且非本文产出时，加最小数字后缀（`concept-2.md`），并把裸 concept 写进该卡 `aliases`，保证唯一且 `[[concept]]` 仍可命中（§1.3.3）。
- 同源成对双向链接：LLM 已在一次输出里成对写好互逆边（Tier 1 §2.1）；代码侧只需保证 §4.1 的"同源 concept 并入候选集"、prune 不误删即可。
- DB 幂等账本：同一篇文章产出的所有卡片共享同一个 `ArticleTag` 记录（`markProcessedDB` 调用一次即可）。
- **护栏 3 — 临时文件 + rename**：每张卡片先写 `<concept-slug>.tmp.md`，校验通过后 `os.Rename` 为正式文件名；校验失败则删除 tmp 文件。这避免 Obsidian/LiveSync 看到半成品（§1.2 护栏 3）。
- **护栏 4 — 部分成功可追踪**：一篇文章多卡写入后，在该文章的 DB 记录（`ArticleTag`）raw frontmatter 中记录 `pkb_cards_written: N` / `pkb_cards_skipped: M` / `pkb_reconstruct_errors: [错误摘要]`。任一卡片验证失败不阻断其他卡片，但失败信息不丢失（§1.2 护栏 4）。

### 4.3 `validateCard` 适配

**新增校验**（`reconstruct.go:79-105` 的 `validateCard`）：
- 章节名改为 v2 四章（定义与本质 / 关键细节 / 适用场景与边界 / 与其他知识的关系），过渡期 v1/v2 双栈（§1.2 护栏 5）。
- `atomic_concept` 字段必须存在且非空（它是身份锚，缺失则无法落文件名与链接）。
- `aliases` 字段必须存在（可为 `[]`）。
- `card_type` 字段必须存在且为枚举值之一；若为 `supplement` 则必须同时有 `supplement_to`。
- 卡片最小长度从 200 rune 降到 100 rune（原子卡片可以很短——一个定义 + 几行细节就够了）。

### 4.4 身份落地（slug + aliases，§1.3.3 的代码侧）

| 关注点 | 落地 |
|--------|------|
| 文件名 | `sanitize(atomic_concept)`，重名加 `-N` 后缀；不带日期前缀 |
| wikilink 锚 | `[[atomic_concept]]`；`pruneWikilinks` 与候选检索都按 concept 匹配（不再按 title）|
| 容错 | frontmatter `aliases` 列出历史措辞；重名被加后缀时，裸 concept 自动并入 aliases |
| 候选来源 | 已有 vault 候选（Tier 2 检索）∪ 本批次同源 concept（§4.1）|
| **prune 有效集** | `pruneWikilinks` 的有效链接目标集 = 所有候选卡的 `concept` ∪ `aliases`。LLM 可能用 alias（同义说法或历史措辞）而非 concept 写 `[[alias]]`，只有把 aliases 也纳入有效集才能避免误删合法链接（§7.2 断链检测同理，按 concept ∪ aliases 匹配）|

**改动文件清单**：
- `internal/pkb/reconstruct.go`：`reconstructCard` 返回 `[]string` + 多卡解析 + `pruneWikilinks` 按 concept + `validateCard` 适配（concept/aliases/supplement/章节双栈/最小长度）
- `internal/pkb/curator.go`：`reconstructToVault` 多卡写入 + 文件名用 concept slug + 重名后缀 + aliases 回写 + **临时文件 + rename（护栏 3）** + **pkb_cards_written/skipped/errors 记录（护栏 4）**

---

## 5. Tier 4 — 分层 MOC 体系（P2）

> 依赖 Tier 1（新 digest 提示词）+ Tier 3（一文多卡，原子卡片是体系树的叶子节点）。

### 5.1 领域根索引 `_index.md`（根 MOC）

每个领域维护一个**持续更新**的根索引 `vault/<领域>/_index.md`（由 `digest.v2.md` 生成，§2.2）：

```yaml
---
title: <领域名>知识体系
type: pkb_map
domain: <domain_name>
period: <周期>
generated_at: <datetime>
source_cards: <N>
root_concepts:
  - 概念A
  - 概念B
---
```

正文即 Tier 1 中 `digest.v2.md` 生成的知识树 + 脉络 + 缺口。

**与现有 digest 的区别**：

| 维度 | 现有 digest | 新体系主文件 |
|------|-----------|------------|
| 文件 | 每周期新建一个 | 持续更新 `_index.md` |
| 命名 | `YYYY-Wxx_领域周综述.md` | `_index.md`（固定文件名） |
| 生命周期 | 快照式（只读不更新） | 活文档（每次 digest 刷新） |
| 关系 | digest 之间无关联 | 始终反映当前最新知识全貌 |

### 5.2 主题 MOC `topics/<主题>.md`（中间层）

领域根索引之下、原子卡之上，增设**主题 MOC**作为中间聚合层（落 `vault/<领域>/topics/<主题>.md`，目录已被 `collectDigestCards` 排除）。它解决 P7「领域树爆炸」：领域卡片多时，根索引只列主题（粗粒度导航），每个主题的细粒度知识树下沉到对应主题 MOC。

**主题如何切分**：digest LLM 生成根索引「知识树」时，顶层节点（`root_concepts`）即天然的主题边界；每个顶层主题聚合其名下的卡片簇，生成一张主题 MOC。

**主题 MOC frontmatter**：

```yaml
---
title: <主题名>
type: pkb_topic
domain: <domain_name>
parent: "[[<领域名>知识体系]]"    # 上链到根索引
last_updated: <datetime>
member_concepts: [概念A, 概念B]
---
```

**主题 MOC 正文**：该主题的局部知识树（缩进列表 + wikilink + 缺口）+ 主题内核心脉络，复用与根索引相同的「依据已声明关系还原层级」逻辑（§2.2），只是范围收窄到单主题。

**主题 MOC 提示词适配**：主题 MOC 复用 `digest.v2.md`，但通过模板参数切换：
- `type`：`pkb_topic`（非 `pkb_map`）
- 章节：只保留「## 知识树」+「## 核心脉络」两个章节（去掉「## 体系概览」/「## 新增与变化」/「## 缺口与探索方向」——这三章是领域根索引特有的，主题级别不需要）
- frontmatter：去掉 `period` / `source_cards`，换为 `parent` / `member_concepts`

代码侧实现：`writeDigest` 接受一个 `mode` 参数（`root` / `topic`），据此渲染不同模板片段。提示词文件仍为同一个 `digest.v2.md`，但代码在调用 LLM 前根据 mode 动态拼接章节指令与 frontmatter 要求。validateDigest 按 mode 校验对应章节。

**三层贯通**：根索引用 `[[主题]]` 链向各主题 MOC；主题 MOC 用 `parent` + 正文 `[[概念]]` 上链根索引、下链原子卡。形成「领域 → 主题 → 卡片」可逐级下钻的导航，满足"看一篇就懂全貌、想深入能下钻"。

> **奥卡姆边界**：主题 MOC 复用 `digest.v2.md` 提示词（仅切换 `type=pkb_topic` 与范围参数），不另写一套；目录排除已就绪。新增的只是「按顶层主题分组、对每组各调一次 digest」的编排循环（Tier 4 代码）。

### 5.3 `RunDigest` 逻辑变更

**现有流程**：
```
collectDigestCards() → writeDigest() → 新建周期文件 → rebuild
```

**新流程**：
```
collectDigestCards()（附带各卡已声明的关系边）
  → 读取已有 _index.md（若存在）作为上下文
  → 按 map_snapshot_on_refresh 快照旧版 _index.md / 旧版主题 MOC 到 digest/（在替换前快照，§5.4）
  → 生成根索引：writeDigest(type=pkb_map) → 原子替换 _index.md（先写 _index.next.md 校验，§1.2 护栏 6）
  → 按根索引顶层主题分组，对每组生成主题 MOC：writeDigest(type=pkb_topic, 范围=该主题) → topics/<主题>.md
  → rebuild
```

**关键改动 `collectDigestCards` 附带关系边**：当前只取每卡摘要前 360 字（`renderDigestCards`，`digest.go:250-262`）。为支撑「依据已声明关系还原层级」（§2.2），`renderDigestCards` 需额外解析每卡「与其他知识的关系」章节的 `[[concept]]：关系类型` 边一并喂给 digest LLM；否则 LLM 仍只能凭摘要猜层级。

**提示词适配**：在 `digest.v2.md` 中增加可选上下文区域：
```
## 已有体系结构（请在此基础上增量更新，不要从零重写）
{{existing_map}}
```

### 5.4 历史快照

- 新增配置项 `map_snapshot_on_refresh: true`（默认开启）。
- 每次刷新 `_index.md` / 主题 MOC 前，将当前版本复制到 `vault/<领域>/digest/YYYYMMDD_HHMM_快照.md`。
- 快照、主题 MOC、根索引都不参与后续 digest 的候选卡片——`collectDigestCards` 已排除 `digest/`、`topics/`、`maps/` 子目录与 `_` 前缀文件（[digest.go:148-151](../internal/pkb/digest.go#L148-L151)，无需新增排除逻辑）。

---

## 6. Tier 5 — 配置调整（P0，零代码风险，改完即生效）

> 与 Tier 1 同步实施，只改 `domains.yaml`，改完即生效。**本 Tier 仅包含真正零代码的配置改动**（权重 + 截断长度）；新增配置项需要在 Go 结构体中加字段，属于代码改动，随各自消费方 Phase 落地（见下 §6.3）。

### 6.1 权重调整

```yaml
# 现有
weights:
  relevance: 0.35
  depth: 0.25
  actionability: 0.25
  durability: 0.15
  novelty: 0.0

# 改为
weights:
  relevance: 0.30
  depth: 0.25
  actionability: 0.20
  durability: 0.10
  novelty: 0.15
```

**调整理由**：
- `novelty: 0 → 0.15`：核心改动，抑制重复内容进 vault。一篇文章如果 depth/actionability 都高但全是老知识，最终分会被 novelty 拉低。
- `relevance: 0.35 → 0.30`：让出空间给 novelty。
- `actionability: 0.25 → 0.20`：并非所有高价值知识都是"可执行的"（如概念定义、原理推导），适当降低权重。
- `durability: 0.15 → 0.10`：让出空间给 novelty。

### 6.2 正文截断长度

```yaml
# 现有
content_truncate: 8000

# 改为
content_truncate: 12000
```

**理由**：原子知识提取需要看到更多原文细节，8000 rune 截断容易丢失后半部分的关键知识点。对于 pool-pkb（Kimi K2.6）的长上下文能力，12000 rune 无压力。

### 6.3 新增配置项（随各自 Phase 落地，不在 Tier 5 一次性加入）

以下配置项需要在 `domains.yaml` 的 `defaults` 中新增，并在 `internal/pkb/domains.go` 的 `Config` 结构体中添加对应字段。**它们不属于 Tier 5（零代码），而是随各自消费方 Phase 同步落地**，避免出现无消费方的死配置（§2.7 纪律）。

| 配置项 | 默认值 | 用途 | 落地 Phase | 消费方 |
|--------|--------|------|-----------|--------|
| `max_cards_per_article` | 5 | 单篇文章最多产出几张原子卡片 | Phase C (Tier 2) | `reconstructToVault` 截断逻辑 |
| `enable_semantic_dedup` | true | 是否启用语义去重（SearchContent 步骤） | Phase C (Tier 2) | `reconstructToVault` 查重步骤 |
| `map_update_mode` | refresh | 体系主文件更新模式：refresh=刷新已有 / snapshot=保留历史快照 | Phase D (Tier 4) | `RunDigest` |
| `map_snapshot_on_refresh` | true | refresh 模式下是否在刷新前保存快照 | Phase D (Tier 4) | `RunDigest` |
| `topic_moc_enabled` | true | 是否生成主题 MOC（`topics/<主题>.md` 中间层） | Phase D (Tier 4) | `RunDigest` |
| `topic_min_cards` | 5 | 一个主题至少含几张卡才单独生成主题 MOC（防碎片化） | Phase D (Tier 4) | `RunDigest` |
| `audit_on_run` | true | 每轮 pkb-curate 结束附带网健康度摘要（孤儿/断链/重复计数） | Phase E (Tier 6) | `Run()` 末尾 |

> 每个新增配置项在落地 Phase 中同步完成：yaml 字段 + Go 结构体字段 + `LoadDomains` 默认值 + 消费方代码引用。`internal/pkb/domains.go` 的 `Defaults` 结构体需在对应 Phase 的改动范围内补字段与默认值（§1.2 护栏：新增 yaml 字段必有 Go 默认值）。

---

## 7. Tier 6 — 网编织与可观测（P2）

> 回答 P8「网会退化但无可观测」。两条腿：① Obsidian Dataview 模板（零代码，§1.2 护栏 10）；② `pkb-curate audit` 只读审计子命令（§1.2 护栏 9）。本 Tier **不做**主动回填（§1.3.4 第 3 档，路线图触发项）。

### 7.1 Obsidian Dataview 模板（零代码，优先）

把"网视图"交给 Obsidian Dataview，作为 vault 资产维护（不写 Go 代码）。三张核心查询，落在领域根索引或一张全局 `_dashboard.md`：

**① 孤儿卡（无任何出入链的原子卡）**：
```dataview
TABLE file.folder AS 领域
FROM "vault"
WHERE type = "pkb_card"
  AND length(file.inlinks) = 0 AND length(file.outlinks) = 0
SORT file.mtime DESC
```

**② 反向关系（补回 P5 丢失的关系类型）**：在每张卡模板底部放一段，按逆关系表（§1.3.2）展示"谁把我当作前置/特例/对比"——读取入链卡片在其关系章节里对本卡的标注并显式列出。这正是 §1.3.4 第 2 档的落地：把单向写入的关系在被指向端也呈现出来。

**③ 重复概念候选（同 concept 多卡 / supplement 簇）**：
```dataview
TABLE rows.file.link AS 卡片簇
FROM "vault"
WHERE atomic_concept
GROUP BY atomic_concept
WHERE length(rows) > 1
```

> Dataview 模板随 vault 走（Obsidian 侧资产），仓库以 `config/pkb/obsidian/` 存一份样板供同步参考；它们查的是 Obsidian 维护的链接图，零 Go 代码。

### 7.2 `pkb-curate audit` 只读审计子命令

Dataview 是"在 Obsidian 里看"，audit 是"在 CLI/cron 里量化并落日志"，用于无人值守时的网健康度告警。**只读、绝不改盘**（§1.2 护栏 9）。

**做什么**：扫描 `vault/`，构建链接图，输出网健康度报告：

| 指标 | 含义 | 调方向信号 |
|------|------|-----------|
| 孤儿率 | 0 链接卡片占比 | 偏高 → 候选检索太弱 / 概念太散（调 Tier 2 limit、提示词）|
| 断链数 | `[[X]]` 指向不存在的 concept/alias | >0 → slug 漂移（评估是否引入硬 ID，§1.3.3）|
| 平均度 / 度分布 | 每卡平均出入链、Top-N 超级 hub | hub 过载 → 该概念应拆分或升为主题 MOC |
| 重复概念 | 同名 / 近名 concept 簇 | 偏多 → 应走概念归并（Tier 2 supplement）|
| 覆盖率 | 有主题 MOC 归属的卡占比 | 偏低 → digest 未覆盖（补跑 Tier 4）|

**实现要点**：
- 新增 `internal/pkb/audit.go`：`WalkDir(vault)` → 解析每卡 frontmatter（concept/aliases/type）+ 正文 `[[]]` 边 → 建图 → 统计 → 打印（可选 `--json` 出机读格式）。
- 复用现成：frontmatter 解析（`parseFrontmatterMap`）、wikilink 正则（`wikilinkRe`）、目录遍历（`collectDigestCards` 的 WalkDir 模式）都已存在，audit 是组合复用，不新建解析器。
- 挂 CLI：`cmd/bellkeeper/main.go` 给 `pkb-curate` 加 `audit` 子命令或 `--audit` flag。
- `audit_on_run=true` 时，`Run()` 结束追加一行精简摘要（孤儿 N / 断链 M / 重复 K），完整报告走 `pkb-curate audit`。

### 7.3 只读护栏

- audit **只 `os.ReadFile` + 统计 + 打印**，不 `Rename` / `WriteFile` / 不删链——可观测与维护写操作严格分离（§1.2 护栏 9）。
- 断链 / 孤儿只**报告**，是否处理由人调方向（§0.3）——audit 不自动"修复"，避免 AI 误删独特知识。

---

## 8. 实施路线

### 8.1 依赖关系

```
Tier 3 (一文多卡) ──→ Tier 1 (提示词)
Tier 2 (语义去重) ──→ Tier 3 (一文多卡)
Tier 4 (体系主文件) ──→ Tier 2 (语义去重)
Tier 5 (配置)       ──→ Tier 1 (提示词)
```

> 箭头方向统一：A → B 表示"A 依赖 B"（A 实施前 B 必须就绪）。

- Tier 1 和 Tier 5 无代码依赖，可同时实施。
- Tier 3 依赖 Tier 1 的新提示词（`---CARD---` 分隔符 + 新 frontmatter 字段）。
- Tier 2 依赖 Tier 1 的新 frontmatter 字段（`atomic_concept`）和 Tier 3 的多卡产出。
- Tier 4 依赖 Tier 1 的新 digest 提示词 + Tier 3 的原子卡片作为体系树叶子节点。

### 8.2 分阶段实施

#### Phase A：Tier 1 + Tier 5（预计 0.5 天）

**改动范围**：
- 新增 `config/pkb/prompts/score.v2.md`
- 新增 `config/pkb/prompts/reconstruct.v2.md`
- 新增 `config/pkb/prompts/digest.v2.md`
- 修改 `config/pkb/prompts/registry.yaml`（切换到 v2）
- 修改 `config/pkb/domains.yaml`（权重 + 截断长度）
- 修改 `internal/pkb/reconstruct.go`：`validateCard` 中 `requiredSections` 改为 v1/v2 双栈（最小代码改动，仅章节名校验字符串）

**微代码改动**（仅 `validateCard` 双栈，其余 Tier 3 代码留 Phase B）：
- `reconstructCard()` 的 `validateCard()` 仍然校验旧版 4 个章节名，新提示词生成的 4 个新章节名会验证失败。
- Phase A 先**仅改 `validateCard` 的 `requiredSections`**（同时接受 v1/v2 章节名，是最小改动），其余 Tier 3 代码（多卡返回值等）留到 Phase B。

**验收**：
- `bellkeeper pkb-curate --dry-run` 正常运行（提示词加载不报错）
- `--dry-run` 输出显示新章节结构（定义与本质/关键细节/适用场景与边界/与其他知识的关系）而非旧版四章节
- novelty 权重生效：`--dry-run` 输出中重复性文章得分明显降低
- `validateCard` 双栈校验通过（v1/v2 章节名均被接受）

> **⚠️ Phase A 不实跑**：reconstruct.v2 会产出含 `---CARD---` 分隔符的多卡输出，但 Phase A 尚未实现多卡解析（Phase B 才做），实跑会产出含分隔符残留的"合体卡"写进单个文件。Phase A 验收止于 `--dry-run`；去掉 `--dry-run` 的实跑验收挪到 Phase B（多卡解析 + 逐卡写入就绪后）。

#### Phase B：Tier 3（预计 1.0 天）

**改动范围**：
- `internal/pkb/reconstruct.go`：`reconstructCard` 返回 `[]string` + 多卡解析 + `pruneWikilinks` 按 concept（有效候选 = 已有 vault concept ∪ 本批次同源 concept）
- `internal/pkb/curator.go`：`reconstructToVault` 多卡写入 + 文件名 `sanitize(atomic_concept)`（去日期前缀；重名加 `-N` 后缀并回写 aliases）
- `internal/pkb/reconstruct.go`：`validateCard` 新增 `atomic_concept`/`aliases`/`card_type`（含 supplement→supplement_to）校验 + v1/v2 章节双栈 + 最小长度降到 100

**验收**：
- 一篇含 3 个独立知识点的文章产出 3 张原子卡片；单知识点文章仍只 1 张
- 每张卡片有独立的 `atomic_concept`、`aliases`、`card_type`
- 同源卡之间**成对双向**链接闭合（A→`[[B]]`:前置 ⟺ B→`[[A]]`:后续），且未被 prune 误删
- 文件名 = concept slug，Obsidian 里 `[[concept]]` 直接命中
- **护栏 3**：vault 目录中无 `.tmp.md` 残留（临时文件全部 rename 成功或已清理）
- **护栏 4**：DB 记录含 `pkb_cards_written` / `pkb_cards_skipped` / `pkb_reconstruct_errors`；部分卡失败时其余卡正常入库、错误被记录
- 去掉 `--dry-run` 实跑，vault 卡片使用新章节结构（Phase A 验收的实跑项挪至此）

#### Phase C：Tier 2（预计 1.0 天）

**改动范围**：
- `internal/pkb/client.go`：新增 `SearchContent()` 方法（返回 `atomic_concept` + 摘要）
- `internal/pkb/curator.go`：`reconstructToVault` 中增加语义查重步骤（标题级 + 内容级候选合并）
- `config/pkb/domains.yaml`：新增 `enable_semantic_dedup` / `max_cards_per_article`
- `internal/pkb/domains.go`：`Defaults` 结构体新增 `EnableSemanticDedup` / `MaxCardsPerArticle` 字段 + `LoadDomains` 默认值

**验收**：
- 高度重复的文章不再生成重叠卡片（LLM 判断重叠后跳过）
- 相近知识点生成 `card_type: supplement` 卡片，`supplement_to` 指向同一 `atomic_concept`（概念归并，§3.2）
- SearchContent 返回的概念 + 摘要正确传入重构提示词的 `{{candidates}}`

#### Phase D：Tier 4（预计 1.5 天）

**改动范围**：
- `internal/pkb/digest.go`：`RunDigest` 改为刷新根索引 `_index.md`（原子替换）+ 按顶层主题分组生成主题 MOC `topics/<主题>.md` + 可选快照
- `internal/pkb/digest.go`：`renderDigestCards` 附带各卡已声明的关系边（供 LLM 还原层级，§5.3）
- `internal/pkb/digest.go`：`writeDigest` 传入已有体系结构上下文 + 支持 `type=pkb_map`/`pkb_topic`（mode 参数切换章节与 frontmatter，§5.2 主题 MOC 提示词适配）
- `config/pkb/domains.yaml`：新增 `map_update_mode`/`map_snapshot_on_refresh`/`topic_moc_enabled`/`topic_min_cards`
- `internal/pkb/domains.go`：`Defaults` 结构体新增 `MapUpdateMode` / `MapSnapshotOnRefresh` / `TopicMocEnabled` / `TopicMinCards` 字段 + `LoadDomains` 默认值
- `internal/pkb/digest.go`：`validateDigest` 适配 v2 五章结构（root 模式五章 / topic 模式两章，§5.2）

**验收**：
- `_index.md` 含知识树（依据已声明关系还原层级）+ 核心脉络（沿前置→后续）+ 缺口
- 每个达标主题生成 `topics/<主题>.md`，三层（领域→主题→卡片）wikilink 贯通可下钻
- 再次运行 digest 时 `_index.md` 增量更新而非从零重写；快照保存到 digest/

#### Phase E：Tier 6（预计 0.75 天）

**改动范围**：
- 新增 `internal/pkb/audit.go`：扫 vault 建链接图 → 孤儿/断链/度分布/重复/覆盖率统计 + `--json` 输出
- `cmd/bellkeeper/main.go`：`pkb-curate` 加 `audit` 子命令（或 `--audit` flag）
- `internal/pkb/curator.go`：`audit_on_run=true` 时 `Run()` 末尾追加精简网健康度摘要
- `config/pkb/domains.yaml`：新增 `audit_on_run`
- `internal/pkb/domains.go`：`Defaults` 结构体新增 `AuditOnRun` 字段 + `LoadDomains` 默认值
- 新增 `config/pkb/obsidian/`：Dataview 模板样板（孤儿 / 反向关系 / 重复概念）

**验收**：
- `pkb-curate audit` 输出孤儿率/断链数/度分布/重复概念/覆盖率；`--json` 可机读
- audit 全程只读：不 `Rename`/`WriteFile`/不删链（grep 验证 audit.go 无写操作）
- Dataview 模板在本地 Obsidian 实测可用（孤儿、反向关系、重复簇能查出）

---

## 9. 验收标准

### 全局验收（所有 Tier 完成后）

| # | 验收项 | 由哪步保证 |
|---|--------|-----------|
| 1 | vault 卡片是原子的：每张卡片只含一个独立知识点，标题即知识点本身 | Tier 1 + Tier 3 |
| 2 | 重复内容不建卡：语义高度重叠的知识只保留一张 | Tier 2 |
| 3 | 相近知识可补充：`supplement` 类型卡片通过 backlink 补充已有卡片 | Tier 2 |
| 4 | 体系主文件可导航：看 `_index.md` 即知领域全貌、知识树结构、核心脉络 | Tier 4 |
| 5 | 卡片间有结构化关系：wikilink 标注关系类型（前置/互补/对比等） | Tier 1 |
| 6 | novelty 权重生效：重复性文章得分显著降低 | Tier 5 |
| 7 | 调方向不改代码：增删提示词/调权重只编辑 `config/pkb/` 即生效 | 全 Tier 保持 |
| 8 | 链接双向可见：同源卡成对、跨卡反向关系经 Obsidian 反链/Dataview 呈现（含关系类型） | Tier 1 + Tier 3 + Tier 6 |
| 9 | 身份稳定：链接锚定 `atomic_concept` + `aliases`，标题措辞漂移不断链 | Tier 1 + Tier 3 |
| 10 | 分层可导航：领域 `_index.md` → 主题 `topics/` → 原子卡，三层 wikilink 可下钻 | Tier 4 |
| 11 | 网可观测：`pkb-curate audit` 量化孤儿/断链/度分布/重复/覆盖率 | Tier 6 |

### 自检清单（CLAUDE.md §3 扩展）

```
□ go build ./... / go vet ./... 绿
□ 新文件/包 grep 验证有真实调用方（导入数 > 0）
□ 提示词文件在 registry.yaml 中正确注册
□ validateCard / validateDigest 与新章节结构一致
□ 数据链路完整：提取→去重→多卡写入→索引→体系主文件
□ 无 not implemented / 暂未实现 / 硬编码占位残留
□ 旧版提示词文件保留、可随时切回
□ domains.yaml 新增字段有对应 Go 结构体字段和默认值
□ 链接锚定 concept（非 title）：pruneWikilinks / 候选检索 / 文件名三处一致
□ pruneWikilinks 有效集 = concept ∪ aliases（alias 链接不被误删）
□ 同源多卡成对双向链接闭合，未被 prune 误删
□ 多卡写入：临时文件 + rename，无 .tmp.md 残留；pkb_cards_written/skipped/errors 已记录
□ 原子卡 frontmatter 含 type: pkb_card（三层节点 type 齐备）
□ audit.go 只读：无 Rename / WriteFile / 删链（grep 验证）
□ 分层 MOC：topics/ 与 _index.md 已被 collectDigestCards 排除（不污染候选）
□ 快照在替换前执行（先快照再原子替换 _index.md）
□ 新增配置项随消费方 Phase 落地（无死配置）
```

---

## 10. 风险与回滚

| 风险 | 影响 | 缓解 |
|------|------|------|
| Phase A 实跑产出含 `---CARD---` 残留的合体卡 | 多卡输出未解析，整坨内容写进单个文件 | Phase A 验收止于 `--dry-run`，实跑挪至 Phase B（多卡解析就绪后）|
| 新提示词导致 LLM 输出格式不符（`---CARD---` 分隔符、新章节名） | `validateCard` 失败，卡片无法入库 | Phase A 同步改 `validateCard`；旧版提示词保留可切回 |
| 一文多卡导致卡片数量膨胀 | vault 文件过多、digest 候选过载 | `max_cards_per_article=5` 上限；digest 已有 `maxCards=50` 限制 |
| 语义查重误判（相似但不同知识点被跳过） | 丢失独特知识 | LLM 判断为主、SearchContent 为辅；supplement 类型兜底（宁可补充不可丢弃） |
| 体系主文件刷新丢失历史结构 | 无法回溯知识体系演变 | `map_snapshot_on_refresh=true` 保存快照 |
| novelty 权重过高导致偏门文章进 vault | 质量下降 | 0.15 是温和调整，可运行后根据实际打分分布微调 |
| `content_truncate` 增大导致 LLM 成本上升 | 单次调用 token 增加 | 12000 rune ≈ 6000-8000 token，pool-pkb（Kimi K2.6）长上下文无压力；且 per_run=5 限制调用量 |
| concept slug 漂移导致断链（P6） | wikilink 指向失效 | aliases 容错 + 重名加后缀；Tier 6 audit 实测断链率，过高再评估引入硬 ID |
| 同源 concept 未并入候选 → 双向链接被 prune 误删 | 同源卡双向断裂 | Phase B 明确"有效候选 = 已有 ∪ 本批次"，并入测试用例 |
| pruneWikilinks 不认 aliases → 合法 alias 链接被误删 | LLM 用 alias 写的 `[[同义说法]]` 被当死链截掉 | 有效集 = concept ∪ aliases（§4.4）；Phase B 代码实现 |
| 分层 MOC 过期（卡片更新后 MOC 未刷新） | `_index`/`topics` 与卡片不一致 | digest 刷新即重建 MOC + 快照可回溯；audit 覆盖率指标暴露失配 |
| Dataview 反向视图依赖 Obsidian 插件 | 不装插件则反向关系类型看不全 | 核心双向（同源）已写入文件、不依赖插件；Dataview 仅增强跨卡反向展示，audit 作 CLI 侧兜底 |

**回滚策略**：
- Phase A：改 `registry.yaml` 切回旧提示词 + 改 `domains.yaml` 权重回退 → 即时回滚，无需重编。
- Phase B-D：Git revert 对应提交 → 代码回退，已生成的卡片文件不受影响（可手动清理）。
- Phase E：audit 只读，停用只需移除子命令注册 / 置 `audit_on_run=false`；Dataview 模板删除即可，不影响卡片。

---

## 附录：关键文件索引

| 关注点 | 文件 |
|--------|------|
| 重构提示词（当前） | `config/pkb/prompts/reconstruct.md` |
| 综述提示词（当前） | `config/pkb/prompts/digest.md` |
| 打分提示词（当前） | `config/pkb/prompts/score.md` |
| 提示词注册表 | `config/pkb/prompts/registry.yaml` |
| 领域配置 + 权重 | `config/pkb/domains.yaml` |
| 卡片重构逻辑 | `internal/pkb/reconstruct.go` |
| 综述生成逻辑 | `internal/pkb/digest.go` |
| 编排主流程 | `internal/pkb/curator.go` |
| 打分逻辑（+atomic_potential） | `internal/pkb/score.go` |
| HTTP 客户端（SearchTitles / +SearchContent） | `internal/pkb/client.go` |
| 领域配置加载 | `internal/pkb/domains.go` |
| 网健康度审计（Tier 6 新增） | `internal/pkb/audit.go` |
| Dataview 模板样板（Tier 6 新增） | `config/pkb/obsidian/` |
| v2 提示词（Tier 1 新增） | `config/pkb/prompts/{score,reconstruct,digest}.v2.md` |
| MVP 实施文档（上级文档） | `doc/PKB-IMPLEMENTATION.md` |

---

*本文件随实施推进更新；每完成一 Phase，回写验收勾选并在 [STATUS.md](STATUS.md) 追加主线动作。需求层面变更回 [ROADMAP.md](ROADMAP.md) §10。*
