# 文件入库模型

> 本文档定义 Bellkeeper 如何把外部 URL / 内容转化为 `knowledge/raw|working` 文件资产。

---

## 主链路

```text
URL / 文本输入
  ↓
Bellkeeper `/api/files/ingest/*`
  ↓
URL 去重
  ↓
正文提取
  ├─ Trafilatura 主力
  └─ Firecrawl 兜底
  ↓
分类 / 标签匹配
  ↓
frontmatter / 文件命名 / 路径决策
  ↓
落地到 `knowledge/raw|working`
  ↓
提交索引任务
```

---

## Bellkeeper 负责的步骤

### 1. 去重

入库前必须先做：

- 精确 URL 匹配
- 归一化 URL 匹配
- 内容 hash 匹配（后续建议补）

目标是避免：

- 重复落地同一文章
- 重复触发索引
- 重复污染搜索结果

### 2. 提取器编排

默认策略：

1. 优先用 Trafilatura 提取正文
2. 若提取失败或质量过差，再切 Firecrawl
3. 把最终使用的 extractor 记录到文件元数据中

### 3. 文件治理

Bellkeeper 负责统一生成：

- 文件名
- frontmatter
- 来源 URL
- 分类与标签
- 抓取时间
- 落地层级（`raw` 或 `working`）

### 4. 索引排队

文件成功落地后，Bellkeeper 负责：

- 标记索引待处理状态
- 异步触发全文索引与后续问答派生流程

---

## 目标目录语义

### `raw`

用于保存：

- 抓取后的原始 Markdown 正文
- 来源元数据完整的采集文件
- 尚未经过提炼的材料

### `working`

用于保存：

- AI 摘要
- 阶段性整理稿
- 结构化中间产物
- 待提升到 PKB 的整理结果

---

## 推荐 API 语义

### `POST /api/files/ingest/url`

输入 URL，由 Bellkeeper 完整执行：

- 去重
- 提取
- 分类
- 标签匹配
- frontmatter 生成
- 文件落地
- 索引排队

### `POST /api/files/ingest/content`

用于直接提交外部已提取内容，由 Bellkeeper 只负责：

- 去重
- 元数据治理
- 文件落地
- 索引排队

---

## 设计原则

1. **文件是主产物**：索引只是派生物。
2. **提取器可替换**：Trafilatura / Firecrawl 只是策略实现，不应绑定主语义。
3. **命名与 frontmatter 一次定稳**：避免后续大规模迁移。
4. **入库结果要可追踪**：必须能知道文件来自哪个 URL、哪个 extractor、何时落地、是否已索引。
