# 搜索与问答模型

> 本文档定义 Bellkeeper 基于文件优先架构提供的 Search / Ask 能力。

---

## 当前范围

当前阶段只覆盖：

- `knowledge/raw`
- `knowledge/working`

不默认覆盖：

- 完整 PKB
- LiveSync CouchDB
- RAGFlow 数据集全量内容

---

## 对外入口

### Matrix

由 n8n `M03-知识问答处理器` 调用 Bellkeeper：

- `!搜` → Search API
- `!问` → Ask API

### Web

Bellkeeper Web 提供：

- 文件搜索页
- 文件问答页
- 来源、标签、目录过滤
- 命中片段与文件路径展示

---

## 推荐 API

### `POST /api/files/search`

输入：

- keyword / query
- layer 过滤
- category 过滤
- tag 过滤
- source_domain 过滤

输出：

- 命中文件
- 命中文段
- 文件路径
- 来源 URL
- 标签 / 分类

### `POST /api/files/ask`

输入：

- question
- 可选过滤条件

输出：

- 基于检索内容生成的答案
- 引用文件路径
- 引用来源 URL
- 相关命中文段摘要

---

## 答案质量原则

问答系统默认应做到：

1. 先检索，再回答
2. 明确返回引用
3. 不把模型幻觉当作事实来源
4. 在证据不足时返回“不足以回答”而不是强编

---

## 返回语义建议

### Search

重点是“找得到文件和片段”。

### Ask

重点是“基于文件内容回答，并给出引用”。

因此 UI 和 API 都应优先展示：

- 命中的文件
- 命中的片段
- 文件路径
- 来源 URL
- 标签 / 分类

而不是只返回一个黑盒答案。
