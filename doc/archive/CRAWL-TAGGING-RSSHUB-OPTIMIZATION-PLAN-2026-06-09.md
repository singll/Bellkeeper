# 爬取、标签与 RSSHub 优化计划

编写日期：2026-06-09

## 1. 审查范围

本计划基于当前仓库代码、配置、n8n 工作流 JSON、已有文档与可用的外部 RSSHub 文档。重点覆盖：

- RSS 拉取、正文抽取、持久爬取队列、文件入库、自动分类和标签写入链路。
- `K01-article-ingest`、`K02-rss-fetch`、`K04-error-retry`、`K05-ai-summarize`、`K06-parse-fallback` 等 n8n 工作流。
- RSSHub 利用方式、可新增源的发现和验证流程。

运行日志限制：本机没有 `docker`、没有 `psql`、没有数据库连接环境变量，且 `localhost:8080` 未启动，因此本次未能读取生产容器 stdout 或真实 activity/log_center 数据。后续实施前必须先拉一轮真实日志和队列样本，作为优化基线。

## 2. 当前链路判断

当前后端主链路是：

1. `internal/service/rss_fetcher.go` 定时读取 active 且未 paused 的 RSS 源。
2. RSS 条目进入 `internal/service/crawl_queue.go` 的持久队列。
3. worker 调用 `internal/service/extractor.go`，默认 Trafilatura，失败后 Firecrawl。
4. 提取成功后传入 `internal/service/file_ingestion.go`，写 raw Markdown。
5. 如果请求未显式传 tags，则 `internal/service/classify.go` 调 LLM 分类。
6. `internal/service/knowledge_index.go` 扫描 archive/vault 并把 frontmatter tags 写入 Meilisearch。

n8n 仍保留另一条链路：`K02-rss-fetch` 直接拉 RSS、解析 XML、调用 `K01-article-ingest`；`K01` 先 Firecrawl scrape，再调用分类和 `/api/files/ingest/url`。这和后端 RSSFetcher/CrawlQueue 存在职责重叠，失败统计和重试口径也不一致。

## 3. 主要发现

### 3.1 爬取与抽取

- 目前没有站点级抽取规则。Trafilatura 只通过 `scripts/trafilatura_extract.py` 调 `trafilatura.fetch_url(url)`；Firecrawl 只传 `url/formats/timeout`。
- 没有规则优化闭环。失败后只按错误类型重试、blocked、dead，不会根据域名总结失败样本并生成新规则。
- RSSFetcher 有 `fetch_interval_minutes` 字段和 UI，但实际按全局 `rss_fetcher.check_interval` 拉全部 active feeds，单源频率没有生效。
- RSS 列表接口不处理 `is_active=true` 查询参数；n8n `K02` 传了该参数，但后端 `RSSHandler.List` 只接收 `category/keyword`。
- 429 当前被 `classifyCrawlError` 归为 `4xx` 并直接 `dead`，不读取 `Retry-After`，也不会作为域名限速信号。
- crawl queue 有基础重试、指数退避、jitter、channel circuit breaker、blocked domain set，但缺少按域名的自适应 token bucket、成功率反馈和 next_allowed_at。
- `autoLearnDomain` 只写内存，重启后通过最近 blocked jobs 恢复；没有持久化 domain profile，也没有区分 paywall、rate limit、临时阻断、结构变更。
- RSSHub 相对路径支持存在，但 RSSHub 能力只用到了 base URL 拼接和补录时 `limit` 参数。

### 3.2 n8n 工作流

- `K02-rss-fetch` 自己拉 RSS 并手写 XML 解析，和后端 gofeed 解析重复。
- `K02` 后续仍有“智能解析文档”等旧 RAGFlow 语义节点，本地定义需要和当前文件优先链路重新对齐。
- `K01-article-ingest` 的 `LLM Classify` timeout 是 5 秒，而后端分类可通过 `llm_jobs` 持久队列等待；n8n 这里更容易产生空标签或误判失败。
- `K04-error-retry` 使用 n8n static data 做失败队列，和后端 `crawl_jobs` 的 retry/dead/blocked 分离，建议退役或改成只触发后端 retry API。

### 3.3 标签

- 默认分类 prompt 明确写着“1-3 个为宜”，标签数量偏保守。
- `classify.max_content_len` 当前为 1000，复杂技术文章可能只看到很短片段，影响标签丰富度。
- tags 写入 Markdown frontmatter 后会被 Meilisearch 使用，但 `FileIngestionService` 入库时没有为每个 tag 创建 `article_tags` 关联行。
- frontmatter 生成格式是 `tags: [a, b]`，而 `getTagsFromMap` 只按逗号切分，没有去掉 `[` 和 `]`，可能索引成 `[a`、`b]` 这样的脏标签。

## 4. 目标

### 4.1 爬取成功率目标

- 单域名默认低速、低并发、可恢复，减少被限速或临时阻断。
- 失败不直接丢弃：先进入可解释的错误分类、规则优化、限速调整或人工复核。
- 所有任务最终进入 `success/skipped/blocked/dead` 中的明确终态，并能解释原因。

### 4.2 标签目标

- 自动标签数量从 1-3 个放大到 3-8 个，最多 10 个。
- 标签要可检索、低噪声、可归一化、可追踪来源。
- 同步写入 frontmatter、Meilisearch 和数据库标签关系。

### 4.3 源目标

- 优先使用官方 RSS、Atom、GitHub Atom、RSSHub 路由。
- 对需要页面抽取的源先做小样本验证，达标后再纳入自动队列。
- 充分使用 RSSHub 的 `limit/filter/filterout/sorted/mode=fulltext/format=json` 等参数。

## 5. 方案一：LLM 爬虫规则优化闭环

### 5.1 新增规则对象

新增 `crawl_domain_profiles`：

- `domain`
- `default_delay_seconds`
- `max_concurrency`
- `success_rate_24h`
- `block_rate_24h`
- `last_status`
- `next_allowed_at`
- `robots_checked_at`
- `notes`

新增 `crawl_extraction_rules`：

- `domain`
- `match_pattern`
- `strategy`: `rsshub | trafilatura | firecrawl | readability | playwright`
- `rsshub_route`
- `css_title_selector`
- `css_content_selector`
- `css_remove_selectors`
- `firecrawl_options`
- `trafilatura_options`
- `quality_min_chars`
- `version`
- `status`: `candidate | active | rejected | rollback`
- `created_by`: `llm | human | seed`

新增 `crawl_rule_trials`：

- `rule_id`
- `sample_urls`
- `attempt`
- `before_error`
- `after_status`
- `content_length`
- `quality_score`
- `diff_summary`

### 5.2 优化流程

1. CrawlQueue 遇到 `extract_failed`、`empty_content`、`timeout`、`403`、`429` 或同域名连续失败时，写入 `rule_optimization_jobs`。
2. Rule Optimizer 收集该域名最近 3-10 个失败样本：URL、HTTP status、提取器、错误、已有 HTML 片段或 Firecrawl/Trafilatura 输出摘要。
3. LLM 只输出受约束 JSON，不允许直接生成任意代码。
4. 系统在沙箱里应用 candidate rule，对样本 URL 进行小批量验证。
5. 达标条件：正文长度、标题存在、重复率、paywall/captcha 关键词、正文噪声比例均通过。
6. 成功则把规则标记 active，并重试该域名 blocked/dead 任务；失败则最多迭代 3 轮。
7. 仍失败则保留 trial 记录，进入人工复核列表。

### 5.3 边界

- 不绕过登录、付费墙。
- 优先降速、使用官方 RSS/API/RSSHub、条件请求和缓存。
- 对高价值但易阻断网站，默认改为 RSSHub/官方 RSS 或人工订阅，不强行抓全文。

## 6. 方案二：智能爬取队列

### 6.1 调度策略

把队列从“固定 worker 轮询”升级为“全局公平队列 + 域名节流器”：

- 每个 domain 一个 token bucket，默认并发 1，默认延迟 60-180 秒。
- 根据 EWMA 成功率、429/403、超时、内容过短动态调低或恢复速率。
- 读取 `Retry-After`，把 429 记为 `rate_limited`，进入延后重试，不直接 dead。
- 尊重 `fetch_interval_minutes`，RSS 源不到时间不拉。
- 支持 `ETag`、`Last-Modified`、`If-None-Match`、`If-Modified-Since`，减少无效请求。
- 用 `next_allowed_at` 控制同域名下一次抓取时间。

### 6.2 错误分类调整

建议拆分现有 `classifyCrawlError`：

- `rate_limited`: 429 或明确限流提示，可重试，强降速。
- `forbidden`: 403，先低频重试一次，再进入规则优化或 blocked。
- `not_found`: 404/410，直接 dead。
- `server_error`: 5xx，可重试。
- `timeout/network`: 可重试，并记录域名健康。
- `empty_content`: 进入规则优化，不直接认定 paywall。
- `paywall/captcha/login_required`: blocked，不做绕过。

### 6.3 RSS 与 n8n 入口统一

- 后端 CrawlQueue 作为唯一文章抓取入口。
- `K02-rss-fetch` 改为只触发 `/api/rss/fetch-all` 或 `/api/crawl/fetch/:sourceId`，并发送报告。
- `K01-article-ingest` 保留为手动 URL 入库入口，但默认调用后端 `/api/crawl/queue/enqueue`，不再自己维护主要重试逻辑。
- `K04-error-retry` 改为调用 `/api/crawl/queue/jobs/:id/retry` 或退役。

## 7. 方案三：标签放大与去噪

### 7.1 Prompt 调整

分类 prompt 改为：

- 返回 `primary_domain`、`tags`、`tag_confidences`、`dataset`、`reasoning`。
- 标签数量建议 3-8 个，最多 10 个。
- 标签分三类：领域标签、技术实体标签、内容形态标签。
- 要求标签稳定、短、可复用，不返回一次性长短语。

### 7.2 规范化

新增标签清洗规则：

- lowercase、trim、空白转 `-`。
- 去掉 `#`、方括号、引号、尾随标点。
- 限制长度 2-48 字符。
- 合并同义词，例如 `llm`/`large-language-models`、`genai`/`generative-ai`。
- 过滤噪声词，例如 `news`、`article`、`update`，除非来自源级标签。

### 7.3 持久化

- 修复 `getTagsFromMap` 对 `tags: [a, b]` 的解析。
- `FileIngestionService` 成功后调用 TagService/TagRepository 自动 get-or-create。
- 新建文章与标签的多对多关系，或重构 `article_tags`：当前它同时承担“文章账本”和“文章-标签关系”，职责混在一起。
- 在 Article metadata 中记录 `tag_source`: `llm | rss_source | user | rule`。

### 7.4 配置

- `classify.max_content_len` 从 1000 调到 2500-4000。
- n8n 不再用 5 秒同步分类作为主路径，统一走后端 `llm_jobs`。
- 对标签数量、置信度阈值、自动创建开关提供配置。

## 8. 方案四：更多可爬取源与 RSSHub 利用

### 8.1 源发现流程

1. 导出现有 RSS 源和健康分数，按领域找缺口。
2. 优先添加官方 RSS/Atom 和 GitHub Atom。
3. 对没有官方 RSS 的站点，优先查 RSSHub route。
4. 用 RSSHub Radar 或 RSSHub route docs 扫描候选站点。
5. 每个候选源先跑 10 条样本验证：RSS 可解析率、正文提取成功率、重复率、标签质量。
6. 通过后写入 RSSFeed，并设置初始 `fetch_interval_minutes` 和 `max_concurrency`。

### 8.2 RSSHub 参数策略

- 补录：`?limit=200`，只对 RSSHub 路由启用。
- 降噪：`filter` / `filterout`。
- 排序：必要时 `sorted=false`。
- 全文：部分路由可试 `mode=fulltext`，成功则减少后续正文抓取。
- 调试：自建 RSSHub 开 `debugInfo` 时，用 `format=debug.json` 和 `format=0.debug.html` 辅助规则验证。
- 输出格式：优先支持 `format=json`，减少 XML 手写解析。

### 8.3 候选源池

优先验证以下源，验证通过后再批量导入：

| 领域 | 源 | 建议入口 |
| --- | --- | --- |
| AI 产品 | OpenAI News | `https://openai.com/news/rss.xml` |
| AI 产品 | OpenAI Developers | `https://developers.openai.com/rss.xml`，需验证 |
| AI 产品 | Anthropic News/Research/Engineering | RSSHub `/anthropic/news`、`/anthropic/research`、`/anthropic/engineering` |
| AI 产品 | DeepSeek News | RSSHub `/deepseek/news` |
| AI 开源 | Hugging Face Blog / Daily Papers | RSSHub `/huggingface/blog`、`/huggingface/blog-zh`、`/huggingface/daily-papers` |
| AI 工程 | web.dev Articles / Blog | RSSHub `/web/articles`、`/web/blog` |
| 研究 | arXiv AI/ML/NLP/CV/Security | `https://rss.arxiv.org/rss/cs.AI`、`cs.LG`、`cs.CL`、`cs.CV`、`cs.CR` |
| 开发 | Hacker News | RSSHub `/hackernews/:section?/:type?/:user?` 或 hnrss |
| 开发 | GitHub Releases/Commits | `https://github.com/{owner}/{repo}/releases.atom`、`/commits/{branch}.atom` |
| 开发 | GoCN | RSSHub `/gocn/topics` |
| 安全 | SecWiki Weekly | RSSHub `/sec-wiki/weekly` |
| 安全 | Hacking8 | RSSHub `/hacking8/index`，搜索路由标记 strict anti-crawling，慎用 |
| 中文科技 | 少数派 | RSSHub `/sspai/index` |
| 中文科技 | 阮一峰博客 | `https://www.ruanyifeng.com/blog/atom.xml` |
| 基础设施 | Cloudflare Status | RSSHub `/cloudflarestatus/` |
| AI 硬件 | NVIDIA Blog / Developer Blog | NVIDIA 官方 RSS 页面选择 Blog/Developer Blog |

## 9. 实施阶段

### P0：基线和明显缺陷修复

预计 1-2 天。

- 增加日志审计脚本或管理 API：最近 24h/7d 按 domain、extractor、error_type 汇总。
- 修复 RSS 列表 `is_active` 查询参数。
- 让 RSSFetcher 尊重 `fetch_interval_minutes`。
- 修复 429 分类，不再直接 dead。
- 修复 frontmatter tags 解析的方括号问题。
- 明确后端 CrawlQueue 是主链路，n8n 只触发和汇报。

验收：

- 能输出失败 Top domains、Top error types、extractor 成功率。
- 单源 `fetch_interval_minutes` 生效。
- 新入库文件 tags 在 Meilisearch 中不带方括号脏字符。

### P1：智能队列

预计 3-5 天。

- 新增 domain profile 和 per-domain scheduler。
- 实现 token bucket、EWMA 成功率、next_allowed_at。
- 429/403/timeout/empty_content 分类进入不同退避策略。
- 增加条件请求缓存字段。
- 前端 crawl queue 页面展示域名健康、限速状态、重试原因。

验收：

- 同域名默认并发不超过配置值。
- 429 后遵循 `Retry-After` 或指数降速。
- blocked/dead 任务有可解释原因。

### P2：LLM 规则优化闭环

预计 5-8 天。

- 新增规则表、trial 表和 Rule Optimizer worker。
- 设计 LLM JSON schema 与 prompt。
- 支持 trafilatura/readability/css selector/firecrawl options 四类规则。
- 对 candidate rule 做样本验证、质量打分、自动启用和回滚。
- 把 repeated empty/extract_failed 自动接入规则优化。

验收：

- 至少 3 个当前失败域名能自动生成 candidate rule。
- 自动启用前必须有样本 trial 记录。
- 失败迭代不超过配置次数，避免无限消耗 LLM。

### P3：标签扩容与持久化

预计 2-4 天。

- 调整分类 prompt 和解析结构。
- 增加标签规范化、置信度、噪声过滤。
- 标签同时写 frontmatter、Meilisearch、DB 关系。
- 把源级标签、用户标签、LLM 标签合并并记录来源。

验收：

- 抽样 50 篇文章，平均标签数 3-8。
- 噪声标签人工判定低于 10%。
- 按 tag 查询文章能查到新入库文章。

### P4：RSSHub 与源扩展

预计 2-3 天持续迭代。

- 建立候选源验证命令或 API。
- 批量验证候选源池。
- 导入第一批高成功率源。
- RSSHub 路由支持 `limit/filter/filterout/mode/format` 配置化。
- 形成每周源健康报告，自动暂停低质量源。

验收：

- 第一批新增 20-40 个源，7 天 RSS 拉取成功率高于 90%。
- 正文提取成功率按域名可见。
- 低质量源自动进入观察或暂停。

## 10. 指标

- RSS feed fetch success rate。
- Article extraction success rate。
- Per-domain blocked/dead/rate_limited count。
- Median/95p crawl latency。
- Retry-to-success ratio。
- Average tags per article。
- Tag noise rate by manual sample。
- New source survival rate after 7 days。

## 11. 参考资料

- RSSHub 路由模型：<https://rsshub.netlify.app/routes>
- RSSHub 通用参数：<https://rsshub.netlify.app/zh/parameter>
- RSSHub 文档入口：<https://rsshub.netlify.app/zh/>
- RSSHub Programming 路由示例：<https://rsshub.netlify.app/routes/programming>
- RSSHub Radar：<https://github.com/DIYgod/RSSHub-Radar>
- NVIDIA 官方 RSS 页面：<https://www.nvidia.com/en-us/about-nvidia/rss/>
