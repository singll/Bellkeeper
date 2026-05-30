# Bellkeeper 文档与文件检索增强

> 本目录定义 Bellkeeper 在 **文件治理、文件索引、搜索与问答增强** 中的职责。

---

## 定位

Bellkeeper 在新的知识链路中不再只是 RAGFlow 辅助层，而是面向多项核心设施的**模块化中心平台**之一。

在文档与检索场景下，它负责：

- 接收 URL / 文本入库请求
- 做去重、分类、标签匹配与 frontmatter 生成
- 编排正文提取策略（Trafilatura 主力，Firecrawl 兜底）
- 把内容落地到 `knowledge/raw|working` 文件资产区
- 维护文件元数据与索引状态
- 对外提供 Search / Ask API
- 承载 Web 搜索与问答入口

它不负责替代 Markdown / Obsidian 主库，也不应再把 RAGFlow 作为默认第一落点。

---

## 文档

1. [FILE-INGESTION.md](FILE-INGESTION.md)
   - URL / 文本如何进入 Bellkeeper，并被落地为文件
2. [FILE-INDEXING.md](FILE-INDEXING.md)
   - 文件索引、增量更新、状态管理与派生物边界
3. [SEARCH-AND-QA.md](SEARCH-AND-QA.md)
   - 搜索与问答 API、Matrix/Web 入口语义
4. [MIGRATION-FROM-RAGFLOW.md](MIGRATION-FROM-RAGFLOW.md)
   - 从 RAGFlow 主链迁移到文件优先架构的边界与步骤

---

## 一句话定义

**Bellkeeper 的 documents 模块是文件优先知识链路的治理平面：它把“抓取后直接灌搜索后端”升级为“先落地文件，再派生索引与问答能力”。**
