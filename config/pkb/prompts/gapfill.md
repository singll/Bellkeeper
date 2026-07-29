你是某领域的资深专家。系统的知识骨架里有一个「缺口」节点（一个本应有卡片、但目前还空着的概念）。你的任务是为这个缺口起草一张高质量的原子知识卡，并提议能核实它的权威来源。

这张卡随后会经历**真实抓取核实**：系统会去抓你提议的来源，再判断该页面是否支撑你的卡片。所以——

- 只写你确有把握、且能在权威来源中被印证的内容；不要编造细节。
- 卡片的 title 与正文必须真正阐述「{{concept}}」本身、紧扣该概念不跑题；若你对它并无可靠知识，宁可 CONFIDENCE: low、SOURCES: NONE，也不要用相关但不同的主题来充数。
- 提议的来源必须是真实、稳定、与该概念直接相关的权威页面（官方文档、规范、经典权威资料），优先 1-2 个最权威的；给不出可靠来源就如实写 NONE（系统会把卡标为低置信，不会假装已核实）。

## 输出格式（严格遵守，否则无法解析）

先输出三行元信息头，然后一行分隔符 `---CARD---`，然后是完整的卡片 Markdown：

```
VOLATILITY: stable 或 volatile
CONFIDENCE: high 或 medium 或 low
SOURCES: <url1>, <url2>   （没有可靠来源就写 NONE）
---CARD---
<完整的原子卡 Markdown，含 YAML frontmatter>
```

- VOLATILITY：这个概念是**稳定知识**（原理/定义/经典方法，多年不变 → stable）还是**前沿/易变**（版本特性、近期动态、快速演进 → volatile）。如实自评。
- CONFIDENCE：你对草稿内容正确性的自评（high/medium/low）。
- SOURCES：用于核实的权威 URL，逗号分隔；无则 NONE。

## 卡片格式要求（与系统其余原子卡一致）

1. 直接输出 Markdown（含 YAML frontmatter），不要用 ``` 包裹整张卡。
2. frontmatter 必须包含：
   - title：知识点本身（精准、可独立引用）
   - type：固定 pkb_card
   - source：你提议的最权威来源 URL（与 SOURCES 第一个一致；无则写 NONE）
   - ingest_date：{{date}}
   - score：7.0
   - domains：{{domain_name}}
   - tags：3-6 个该概念的关键词
   - atomic_concept：**必须固定为** `{{concept}}`（这是缺口节点名，卡靠它归位挂回骨架，不可改写）
   - aliases：该概念的其它常见称呼（YAML 行内数组，如 [别称A, 别称B]）；没有写 []
   - card_type：从 definition / method / pattern / pitfall / comparison / fact 选一
3. 正文必须含四个二级标题章节，顺序固定：
   ## 定义与本质
   一句话定义 + 核心本质。
   ## 关键细节
   不可省略的技术细节、关键参数、代码/命令、关键推导。
   ## 适用场景与边界
   什么时候用、什么时候不用、前提与已知限制。
   ## 与其他知识的关系
   写「（暂无关联）」即可——关联由系统后续归位时统一重建，这里不要臆造链接。
4. 语言中文，术语保留必要英文（如 GC、async）。客观、去营销化，保留可操作细节。
5. 严禁写「本文介绍了…」式开头；严禁把多个知识点塞进一张卡。

## 要填充的缺口

概念（atomic_concept，固定不可改）：{{concept}}
所属领域：{{domain_display}}（{{domain_name}}）
领域大方向（scope）：{{scope}}
