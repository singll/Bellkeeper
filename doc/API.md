# Bellkeeper API 参考

> 基础路径: `/api`
> 认证: `X-API-Key: <key>` 或 Authelia SSO（`Remote-User` 头）
> 健康检查端点无需认证

---

## 文件入库 API

### `POST /api/files/ingest/url`
从 URL 提取内容并保存为本地文件。

**请求体**
```json
{
  "url": "https://example.com/article",
  "title": "文章标题（可选，留空自动提取）",
  "tags": ["security", "web"],
  "category": "security",
  "layer": "raw"
}
```

**响应**
```json
{
  "data": {
    "success": true,
    "status": "success",
    "file_path": "/mnt/knowledge/raw/20260408_article-title.md",
    "title": "文章标题",
    "tags": ["security", "web"],
    "extractor": "trafilatura"
  }
}
```

**状态码**
- `success`: 入库成功
- `duplicate`: URL 已存在
- `extract_failed`: 内容提取失败

### `GET /api/files/metadata/:id`
获取文件元数据。按 ID 查询已入库文件的标题、标签、分类、层级等元信息。

### `GET /api/files/list`
列出已入库文件，支持分页和过滤。

**查询参数**
- `layer`: 过滤层级（raw/working）
- `status`: 过滤状态（ingested/indexed）
- `keyword`: 关键词搜索（标题/URL）
- `page`: 页码（默认 1）
- `per_page`: 每页数量（默认 20，最大 1000）

---

## 健康检查

### `GET /api/health`
基础健康检查，无需认证。

**响应**
```json
{ "status": "healthy", "version": "1.0.0" }
```

### `GET /api/health/detailed`
详细状态，包含依赖服务连通性和数据统计。

**响应**
```json
{
  "status": "healthy",
  "version": "1.0.0",
  "services": {
    "ragflow": { "status": "up", "latency_ms": 12 },
    "n8n":     { "status": "up", "latency_ms": 8 }
  },
  "metrics": {
    "timestamp": "2026-03-05T10:00:00Z",
    "tags_count": 15,
    "rss_feeds_count": 42,
    "datasets_count": 5
  }
}
```

---

## LLM Proxy

OpenAI 兼容代理，所有 OpenAI SDK 可直接对接。

### `ANY /api/llm/v1/*path`
透传到上游 LLM API，自动限速、重试、日志。

**请求头**
- `X-Caller-ID: <string>` — 可选，标识调用方（记录在日志中）

**示例**
```bash
curl -X POST http://bellkeeper/api/llm/v1/chat/completions \
  -H "X-API-Key: <key>" \
  -H "Content-Type: application/json" \
  -d '{"model": "deepseek-chat", "messages": [{"role": "user", "content": "hello"}]}'
```

### `GET /api/llm/channels/status`
查看所有渠道的令牌桶状态。

**响应**
```json
[
  {
    "name": "deepseek",
    "base_url": "https://api.deepseek.com",
    "models": ["deepseek-chat"],
    "priority": 1,
    "rpm_limit": 60,
    "rpd_limit": 1000,
    "available_tokens": 58,
    "max_tokens": 60,
    "daily_used": 42,
    "daily_limit": 1000,
    "refill_rate_per_s": "1.00"
  }
]
```

### `GET /api/llm/stats?hours=24`
聚合统计（按渠道/模型）。

### `GET /api/llm/logs?channel=<name>&limit=50`
最近的代理请求日志。

### `GET /api/llm/rate-limit-events?hours=24&channel=<name>`
最近的限速事件。

---

## RAGFlow 集成

### `POST /api/ragflow/upload`
上传文档到指定数据集。

**请求体**
```json
{
  "content": "文档内容",
  "filename": "article.txt",
  "dataset_id": "abc123"
}
```

### `POST /api/ragflow/upload/with-routing`
智能路由上传：根据标签/分类自动选择目标数据集，并记录文章-标签关联。

**请求体**
```json
{
  "content": "文档内容",
  "filename": "article.txt",
  "title": "文章标题",
  "url": "https://example.com/article",
  "tags": ["AI", "技术"],
  "category": "tech",
  "auto_create_tags": true
}
```

**路由优先级**: 标签匹配 → 分类名匹配 → 默认数据集

**响应**
```json
{
  "data": { "code": 0, "message": "success", "data": {...} },
  "dataset_id": "abc123"
}
```

### `GET /api/ragflow/check-url?url=<url>&normalize=true&fuzzy=false`
检查 URL 是否已入库。

**查询参数**
- `url` — 要检查的 URL（必填）
- `normalize` — 是否启用归一化匹配（默认 false）
- `fuzzy` — 是否启用模糊匹配（默认 false）

**响应**
```json
{
  "exists": true,
  "document_id": "doc123",
  "dataset_id": "ds456",
  "title": "文章标题",
  "stored_url": "https://example.com/article",
  "match_type": "normalized"
}
```

### `GET /api/ragflow/documents?dataset_id=<id>&page=1&limit=20`
列出数据集中的文档。

### `DELETE /api/ragflow/documents/:id?dataset_id=<id>`
删除文档（同时清理本地 article_tags 记录）。

### `POST /api/ragflow/documents/parse`
触发文档解析。

**请求体**
```json
{ "dataset_id": "abc123", "document_ids": ["doc1", "doc2"] }
```

### `POST /api/ragflow/documents/parse/throttled`
节流解析：分批提交，避免 Embedding 限速。后台异步执行。

**请求体**
```json
{
  "dataset_id": "abc123",
  "document_ids": ["doc1", "doc2", "doc3"],
  "batch_size": 3,
  "interval_seconds": 30
}
```

### `POST /api/ragflow/documents/parse/stop`
停止解析。

### `GET /api/ragflow/documents/parse/status?dataset_id=<id>&document_id=<id>`
查询解析状态。

### `POST /api/ragflow/upload/batch`
批量上传文档。

### `POST /api/ragflow/documents/batch-delete`
批量删除文档。

### `POST /api/ragflow/documents/transfer`
将文档从一个数据集转移到另一个。

**请求体**
```json
{
  "source_dataset_id": "src123",
  "target_dataset_id": "tgt456",
  "document_id": "doc789"
}
```

### `POST /api/ragflow/documents/batch-transfer`
批量转移文档。

### `PUT /api/ragflow/documents/metadata`
更新文档元数据。

### `GET /api/ragflow/chunks?dataset_id=<id>&document_id=<id>&page=1&limit=20`
列出文档分块。

### `DELETE /api/ragflow/chunks`
删除指定分块。

### RAGFlow 数据集管理

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/ragflow/datasets` | 列出所有数据集 |
| GET | `/api/ragflow/datasets/:id` | 获取数据集详情 |
| POST | `/api/ragflow/datasets` | 创建数据集 |
| PUT | `/api/ragflow/datasets/:id` | 更新数据集 |
| DELETE | `/api/ragflow/datasets/:id` | 删除数据集 |

---

## 标签管理

### `GET /api/tags?page=1&per_page=20&keyword=`
分页列出标签。

### `POST /api/tags`
创建标签。

**请求体**
```json
{ "name": "AI", "description": "人工智能相关", "color": "#409EFF" }
```

### `GET /api/tags/:id` / `PUT /api/tags/:id` / `DELETE /api/tags/:id`
获取/更新/删除标签。

### `GET /api/tags/all`
获取所有标签（不分页）。

### `POST /api/tags/batch`
批量获取或创建标签。

**请求体**
```json
{ "names": ["AI", "技术", "新标签"] }
```

### `POST /api/tags/match`
模糊匹配标签。

### `POST /api/tags/by-names`
按名称批量获取标签。

---

## RSS 订阅管理

### `GET /api/rss?page=1&per_page=20&category=&keyword=`
分页列出 RSS 订阅源。

### `POST /api/rss`
创建订阅源。

**请求体**
```json
{
  "name": "Hacker News",
  "url": "https://news.ycombinator.com/rss",
  "category": "tech",
  "description": "技术新闻",
  "is_active": true,
  "tag_ids": [1, 2]
}
```

### `GET /api/rss/:id` / `PUT /api/rss/:id` / `DELETE /api/rss/:id`
获取/更新/删除订阅源。

---

## 知识库映射（Dataset Mappings）

### `GET /api/datasets?page=1&per_page=20`
分页列出映射。

### `POST /api/datasets`
创建映射。

**请求体**
```json
{
  "name": "tech",
  "display_name": "技术知识库",
  "dataset_id": "ragflow-dataset-id",
  "description": "技术文章",
  "is_default": false,
  "is_active": true,
  "parser_id": "naive",
  "tag_ids": [1, 2]
}
```

### `GET /api/datasets/all`
获取所有映射（不分页）。

### `GET /api/datasets/by-name/:name`
按名称获取映射。

### `POST /api/datasets/by-tag`
根据标签推荐数据集。

**请求体**
```json
{ "tags": ["AI", "技术"], "category": "tech" }
```

**响应**
```json
{
  "data": { "id": 1, "name": "tech", ... },
  "match_type": "tag"
}
```

### `POST /api/datasets/check-url`
检查 URL 是否已入库（支持批量）。

**请求体**
```json
{
  "url": "https://example.com/article",
  "normalize": true,
  "fuzzy": false
}
```

或批量：
```json
{
  "urls": ["https://a.com", "https://b.com"],
  "normalize": true
}
```

### `POST /api/datasets/article-tags`
手动添加文章-标签关联。

### `GET /api/datasets/article-tags/:document_id`
获取文档的标签关联。

### `GET /api/datasets/articles-by-tag/:tag_id?page=1&per_page=20`
按标签查询文章列表。

---

## n8n 工作流

### `GET /api/workflows/status`
列出所有工作流及激活状态。

### `GET /api/workflows/:id`
获取工作流详情。

### `POST /api/workflows/:id/activate`
激活工作流。

### `POST /api/workflows/:id/deactivate`
停用工作流。

### `GET /api/workflows/executions?workflow_id=<id>&limit=20`
查询执行记录。

### `POST /api/workflows/trigger/:name`
通过 Webhook 名称触发工作流。

**请求体**（可选）
```json
{ "key": "value" }
```

---

## 系统配置

### `GET /api/settings?category=`
列出配置项（可按分类过滤：`api` / `feature` / `ui`）。

### `GET /api/settings/:key`
获取单个配置。

### `PUT /api/settings/:key`
更新配置。

**请求体**
```json
{ "value": "new-value" }
```

---

## 系统管理

### `POST /api/system/restart`
优雅重启 Bellkeeper 进程（容器会自动重启）。

---

## 通用响应格式

**成功（列表）**
```json
{
  "data": [...],
  "total": 100,
  "page": 1,
  "per_page": 20
}
```

**成功（单项）**
```json
{ "data": { ... } }
```

**成功（操作）**
```json
{ "message": "操作成功" }
```

**错误**
```json
{ "error": "错误描述" }
```

HTTP 状态码：`200` 成功，`400` 参数错误，`401` 未认证，`404` 不存在，`500` 服务器错误。
