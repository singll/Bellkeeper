# 文件索引模型

> 本文档定义 Bellkeeper 如何把 `knowledge/raw|working` 中的文件转化为搜索与问答可消费的派生索引。

---

## 核心原则

### 文件为真相源

`raw|working` 中的 Markdown / 文本文件才是主资产。

索引层：

- 可以重建
- 可以替换
- 可以删后重跑
- 不应反向定义文件主库结构

### 索引是派生物

全文索引、chunk、embedding、问答缓存都应理解为派生物，而不是主库。

---

## 推荐索引层次

### 第一阶段

优先建设：

- 文件元数据索引
- 全文索引
- 命中片段高亮
- 分类 / 标签 / 层级过滤

推荐后端：

- **Meilisearch** 负责全文检索、过滤、排序、高亮

### 后续阶段

按需增加：

- 向量检索
- rerank
- chunk embedding
- 多段上下文问答优化

这些能力不应阻塞第一阶段落地。

---

## 索引对象

每个文件至少需要派生出：

- 文件元数据记录
- 全文检索文档
- 可选的 chunk 记录
- 索引状态

建议元数据字段：

- source_url
- canonical_url
- content_hash
- title
- tags
- category
- layer (`raw` / `working`)
- file_path
- extractor
- ingest_status
- index_status
- updated_at

---

## 增量更新语义

文件索引必须支持：

- 新文件创建 → 新增索引
- 文件更新 → 增量重建
- 文件删除 → 删除索引
- 重建索引 → 从文件系统全量恢复

因此索引系统的正确依赖方向应是：

```text
file system → index
```

而不是：

```text
index → file system
```

---

## 与问答层的关系

问答默认基于：

1. 全文检索命中文件 / 片段
2. 可选 rerank / chunk 选择
3. LLM 生成回答
4. 返回引用文件与来源

这意味着问答依赖索引，但索引仍依赖文件。

---

## 运维结论

- 文件丢了，索引不算主资产
- 索引坏了，可以重建
- 迁移检索后端时，只迁移索引派生逻辑，不迁移主文件语义
