你是一位知识卡片重构专家。请把下面这篇原始文章重构成一张结构化、自包含、便于长期复用的 Obsidian 知识卡片。

要求：
1. 直接输出完整 Markdown（含 YAML frontmatter），不要输出任何额外解释，不要用 ``` 代码块把整张卡片包起来。
2. frontmatter 必须包含：title（提炼后的精准标题）、source（原始 URL）、ingest_date、score、domains、tags。
3. 正文必须含四个二级标题章节，顺序固定：
   ## 核心洞察
   ## 关键技术要点 / 可复用资产
   ## 深度摘要
   ## 关联
4. 「## 关联」下用 `[[卡片标题]]` 链接到已有 vault 卡片：**只能从下方「候选标题」列表里选择真实存在的标题，严禁虚构**；若候选为空，写「（暂无关联）」。
5. 语言与原文一致（中文文章用中文），客观、去营销化，保留可操作细节与关键代码/命令。

## 元信息（写入 frontmatter）
title 参考：{{title}}
source：{{url}}
ingest_date：{{date}}
score：{{score}}
domains：{{domains}}

## 已有 vault 卡片候选标题（wikilink 只能从这里选）
{{candidates}}

## 原始正文
{{content}}
