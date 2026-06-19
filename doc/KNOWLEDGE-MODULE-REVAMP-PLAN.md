# 知识库模块重做计划（KNOWLEDGE MODULE REVAMP）

> 状态：**阶段 1（Matrix agent 通电）已实施**（2026-06-19）；阶段 2/3 待做。详见 §6.5 实施纪要。
> 本计划只产出文档与决策，**尚无任何代码改动**。落档：ADR-0006 + CONTEXT.md（调方向前端/对话问答/对话房间）+ 本文件。
> 权威边界见 `docs/adr/0006-web-frontend-boundary.md`；术语见 `CONTEXT.md`；上游体系见 ADR-0004/0005 与 `doc/PKB-KNOWLEDGE-SKELETON-PLAN.md`。

## 0. 背景

PKB 已从早期「RAGFlow 知识库问答 + 切分」彻底转向「自维护的 Obsidian vault + AI 驱动」体系（ADR-0004 骨架、ADR-0005 双库）。后端能力换代了，但前端「知识库」模块仍是旧形态：6 项混装、知识问答是 RAG-only 拒答式、数据集是 RAGFlow 遗留。本计划按新体系重做配套界面与问答。

## 1. 总边界（ADR-0006）

**Web 管「机制与观测」，Obsidian/文件系统管「产物与浏览」。** 判据看操作的作用对象：

- 机制/规则/方向/配置（产物随后由机器据此生成）→ ✅ 允许（哪怕直接改卡片生成规则）
- 只读观测产物（统计/状态/结构投影）→ ✅ 允许
- 产物本身（卡片正文、文件、目录树的增删改）→ ❌ 否决，归 Obsidian
- **唯一例外**：资讯库每日存档（`pkb_feed` 报告类产物）允许 Web 时间线只读渲染（见 ADR-0006 例外条款）

## 2. 决策清单（13 项，grill 对齐）

| # | 主题 | 结论 |
|---|---|---|
| 0 | 前端边界 | ADR-0006，判据=作用对象 |
| 1 | 模块结构 | 单一「知识库」组 + 底部「采集」子分区，不拆顶层 |
| 2 | 知识搜索 | 保留，重定位 archive+vault；结果=片段+原文外链+路径，不渲染正文 |
| 3 | 数据集 | 前端页退役；`DatasetMapping`/`dataset_id` 后端冻结；`ArticleTag` 账本保留 |
| 4 | 标签 | 保留现状；采集侧活、知识库侧不用；RSS 标签只影响采集层检索质量 |
| 5 | 资讯库 | 并入总览，ADR-0006 有界例外：Web 时间线只读渲染当日综述 |
| 6 | 总览 | 5 块全做；数据基本现成；仪表盘卡改链总览 |
| 7 | 问答语义 | RAG-only 拒答 → 通用对话 + 命中引用；引用基本免费；真多轮 |
| 8 | 问答重点 | **Matrix agent**（Web 顺带）；agent 已建好未通电，已含 knowledge 工具 |
| 9 | 触发 | (a) 对话房间(room_type=chat)非命令消息触发；`!kb` 保留作强制 RAG |
| 10 | 命令 | 全字母化(待办→todo, 问→kb)；`!` 前缀（`/` 被客户端吞） |
| 11 | 模型 | 房间默认模型组(agent.model) + `!model` 切换(模型组, **按用户持久**，见 §6.5) |

## 3. 目标模块结构

当前导航：`web/src/components/Layout.tsx:218-242`（knowledge 组 6 项混装）；路由 `web/src/index.tsx:40-46`。

```
仪表盘 ── 知识库入口卡 → 点进总览（现链 /knowledge/search，改链总览）
知识库（单一组）
   总览        ★新增 /knowledge/overview
   知识骨架    保留 /knowledge/skeleton（调方向，已实现）
   知识问答    重构（Web 顺带）/knowledge/ask
   知识搜索    保留+重定位 /knowledge/search
   ─── 采集 ───（子分区：分隔线+小标题，贴着放）
   订阅源(RSS) 保留 /rss
   标签        保留现状 /tags
   〔数据集 → 退役前端页（删路由+导航项），后端冻结〕
   〔资讯库 → 并入总览，不单独成页〕
```

参照范式：LLM 组（`Layout.tsx:243`）、Matrix 组（`:270`）都是「总览+子功能」结构。

## 4. 各功能定性（含关键代码位置）

### 4.1 知识搜索（保留 + 重定位）
- 后端 `internal/service/knowledge_search.go` → Meili `knowledge_chunks`（索引 vault+archive 分块）。
- **独占价值**：archive 层(4–7分)进 Meili 但**不下行 Obsidian**（Obsidian 只挂 vault）→ archive 是 Obsidian 盲区，**只能靠 Web 搜索**。砍了 archive 就成黑洞。
- **轻形态查看**（守 ADR-0006）：结果项=标题/域/layer/分数/时间 + snippet 片段 + `source_url` 原文外链 + 路径文本；vault 卡可加 `obsidian://` 深链。**不**渲染 markdown 正文，**不**接回 `GET /api/knowledge/files/read` 与孤儿页 `KnowledgeFiles.tsx`。

### 4.2 数据集（退役前端，冻结后端）
- `dataset_id` 是 RAGFlow 遗留的**无下游死字段**：入库时 `internal/service/file_ingestion.go:294 resolveDatasetID`（→`dataset.go RecommendByTags`）或 `:171` 取 classify 结果写入 `ArticleTag.DatasetID`，但 `internal/pkb/` **零引用**；PKB 走 `pkb_domain`（`internal/pkb/curator.go:498`，打分时定）；落盘 layer 由打分定；Meili 无 dataset_id 字段。
- 处置：删 `Datasets.tsx` 路由(`index.tsx`)+导航项(`Layout.tsx`)。`DatasetMapping` 表 + `resolveDatasetID` 链路冻结不动（留着无害）。**`ArticleTag` 表是 PKB 账本命脉（pkb_status/去重/file_path），绝不动 schema**。
- ⚠ n8n `K01-article-ingest.json:37` 仍调 `/api/datasets/check-url`（走 ArticleTag 全量 URL 去重），该端点**保留**。

### 4.3 标签（保留现状）
- 知识库体系侧**不用** DB Tag：`internal/pkb/` 只读卡片 frontmatter 的 tags（LLM 自由文本，`digest.go:268/422`）和 pkb_domain。
- 标签自动产生：classify(LLM) 产 `tags`（`classify.go:83`），入库 `ensureTags` FindOrCreate。
- 采集侧活用途：RSS 源关联标签(`rss.go:25`)、跨实体全局搜索 `/api/search`(`search.go` + `GlobalSearch.tsx`，挂 Layout 顶栏)。
- **RSS 标签影响入库**（实锤）：`K02-rss-fetch.json:116/:174` 把源标签附到每篇文章 → `K01:255 merge-tags` 与 LLM/match 标签合并 → 账本 + raw/archive frontmatter → 出现在全局/知识搜索过滤。**但污染不到骨架/vault**（vault 卡重构时标签换成领域，`reconstruct.go:41`）。**平时配源注意**：源标签配准点，archive/全局搜索才干净。

### 4.4 资讯库（并入总览，有界例外）
- ADR-0005 时效流，存档 `vault/资讯/<领域>/<日期>.md`（`pkb_feed`，LLM 每日综述）。
- 不单独成页；总览做「资讯时间线」（最近 14 天 + 可往前翻全部），点开**只读渲染**当日综述（ADR-0006 唯一例外）。
- 后端已有 `PKBReportService.FeedArchivesByDate`（日报联动扫当日各领域存档）→ 扩成「列最近 N 天 + 读单篇」轻端点。

## 5. 总览设计（/knowledge/overview）

数据基本现成：`GET /api/pkb/domains/stats`（各领域 卡片数/今日·7天/缺口/待归位/低置信/最近digest/has_skeleton）、`knowledgeFilesApi.getStats()`（分层文件数）、`GET /api/pkb/proposals`（待批提议）。**只有资讯时间线要新增轻端点**。

5 块（从上到下，全做）：
1. **顶部 KPI**：卡片总数·今日新增·领域数·缺口·待归位·低置信·待批提议
2. **各领域一览**：每领域一卡，点击→跳知识骨架页调方向
3. **资讯时间线**：最近 14 天×各领域，点开只读渲染（可往前翻全部历史）
4. **需要关注**：待批提议 + 待归位/低置信偏高领域（阈值配置化），点击直达对应页
5. **采集动态**：分层文件数 raw/archive/vault + 今日入库量

仪表盘 PKB 卡（`Dashboard.tsx:213`/`:332-376`）改链 `/knowledge/overview`（现链 `/knowledge/search`）。

## 6. 问答重构（重点：Matrix；Web 顺带）

### 6.1 现状
- Web/Matrix 共用 `internal/service/knowledge_ask.go`：纯 RAG，搜不到回「知识库中未找到相关信息」**不调 LLM**(`:89`)，prompt 写死「只基于知识库」(`:156`)；**不传对话历史**（伪多轮）。
- Matrix `!问` = `handler_qa.go` → AskService。
- **Matrix agent 已建好但没通电**：`internal/matrix/agent/agent.go AgentService`（LLM + ToolRegistry + Redis SessionStore 按房间多轮 + 限流 + 权限 + `HandleMessage:97` + `ResetSession`），已注册 `knowledge_search`+`knowledge_ask` 工具（`tools_readonly.go:59-60`）。但 `matrix.agent.enabled=false`(`config.go:517`)，且 `HandleMessage` **无调用方**——`gateway/sync.go:61 handleRoomMessage` 只走命令路由，非命令消息被忽略。
> ✏️ **阶段1实施修正**：此判断有误——分流脚手架早已存在于 `service/command.go ExecuteMessage`（非命令消息→`agent.HandleMessage` 并回帖）。真实缺口是「分流不判 room_type」+ enabled 默认关，见 §6.5。

### 6.2 阶段 1（重点 · Matrix agent 通电）
1. 开 `matrix.agent.enabled`（配 model/限流/权限）。
2. `RoomType` 加 `chat` 类型（现 `model/matrix.go:16`：command/notification/admin）。
3. `handleRoomMessage`：对话房间(room_type=chat)的**非命令消息** → `agent.HandleMessage`（= 去掉 `!问` 前缀，直接说话即对话）。
4. 命令字母化：`待办`(`handler_memos.go:28`)→`todo`、`问`(`handler_qa.go`)→`kb`；`!kb` 保留 QAHandler 作**强制纯 RAG**（= `/kb` 扩展口子，零成本）。前缀仍 `!,！`（`config.go:514`）——`/` 被 Matrix 客户端当 slash command 吞掉，不可用。
5. 新增 `!model` 命令：列出/切换 LLM 模型组（`pool-chat-balanced`/`pool-pkb`…），**按用户持久**覆盖 agent 模型（存独立 Redis key `matrix:agent:usermodel:<user>`，与会话 key 分离 → `!reset` 只清对话不清模型）。
6. 会话管理**全现成**：房间=会话、切房间=切会话、新房间=新会话、`!reset`=清当前会话；多轮/引用由 agent + knowledge 工具自带。

### 6.3 阶段 2（Web 顺带）
- `AskService` 升级：搜不到**不拒答**走通用回答；prompt 改通用助手（优先知识库、没有就正常答、用到卡片标引用）；去 layer 复选框（`KnowledgeAsk.tsx:190-212`）；保留 references 引用（已有，符合 ADR-0006 只读指针）。
- 真多轮：前端 history 传后端、后端拼进 messages（现 `knowledge_ask.go callLLM` 不传历史）。
- 知识搜索页重定位 + 轻形态查看（见 4.1）。
- Web 多会话（自建会话表那套）排最后、可不做——重点在 Matrix（房间即会话）。

### 6.4 阶段 3（留口不做）
- `/kb` 模式参数、限定领域/layer 等：对话端点/agent 预留「模式」入参即可，先不实现。

### 6.5 阶段 1 实施纪要（2026-06-19 完成）

提交 `1c782bf`(门禁) / `a3180fb`(别名) / `11acd3e`(!model)。**纯后端**，本地止于 `go build`+`go vet`（无真实 Matrix）。

- **现状修正**：grill 时判断「`HandleMessage` 无调用方」**有误**——分流脚手架早已在 `service/command.go ExecuteMessage`（非命令消息→`agent.HandleMessage` 并回帖），`SetAgent`/`ResetHandler`/`agentServiceAdapter` 也都接好。真实缺口仅：①分流**不判 room_type**（任何房间都触发 agent）；②无 todo/kb 别名；③无 `!model`；④enabled 默认关。
- **分流点偏离**：计划写「`handleRoomMessage` 分流」，实改在 `service.CommandService.ExecuteMessage` 加 `isChatRoom` 门禁。原因：`sync.go`(gateway 包) 反向 import agent 会**循环依赖**（agent 已 import gateway 发消息）；现状用 service 层 + `AgentHandler` interface 解耦，语义与「非命令→agent」一致。
- **模型粒度偏离**：决策11/§6.2.5 原写「房间持久」，按用户最新指示改为**按发言用户(sender)持久**——每人切自己的模型组（权限 user），同房间多人各用各的；会话仍按房间(房间=会话)，仅「本轮用哪个模型」按发言人。
- **命令字母化**：中文+字母**并存**（不删 待办/问/搜），新增 `todo`/`kb`；`!kb`=`QAHandler`（纯 RAG，不经 agent）。
- **部署待办**（未做，本地难跑）：开 `matrix.agent.enabled` + 标一个房间 `room_type=chat`（`PUT /api/matrix/admin/rooms/:id` body `{"room_type":"chat"}`）后在 keeper 冒烟。

## 7. 关键非显然要点（避免重复踩坑）

- **`/` 前缀在 Matrix 不可用**：Element 等客户端把 `/开头` 当 slash command 拦截，消息发不到机器人。机器人命令必须用 `!`。
- **archive 是 Obsidian 盲区**：archive 进 Meili 不下行 vault → 只能 Web 搜索；这是知识搜索保留的根本理由。
- **引用基本免费**：`AskService` 已返回 references（标题/路径/片段/源URL），前端已展示，正是 ADR-0006 要的只读指针形态。别误以为要做卡片内逐句定位（那个不做）。
- **`ArticleTag` 账本碰不得**：PKB 幂等账本（pkb_status/pkb_decision/pkb_score/file_path/去重），AutoMigrate 加过列。退役数据集只删前端，不动表 schema。
- **agent 是「建好未通电」**：重点工作量是接线（开关 + handleRoomMessage 路由 + RoomType），不是从零造对话系统。
- **不渲染正文红线**：知识搜索/问答的「查看」都只给指针+外链；唯一例外是资讯库每日存档（ADR-0006 例外条款）。
- **前端勿跑 biome format**：仓库既有前端非 biome 格式，`pnpm format` 会重排数十个无关文件。只 `pnpm lint`+`build`。
- 前端 stack：SolidJS + @solidjs/router + Tailwind + Vite + Biome + TS；CSS 类 `card`/`input`/`btn-*`/`badge-*`/`loading-spinner`；错误体 `{"error":msg}`、成功 `{"data":...}`。

## 8. 关联

- `docs/adr/0006-web-frontend-boundary.md`（前端边界 + 资讯例外）
- `docs/adr/0004-*`（骨架 W1）、`docs/adr/0005-*`（双库）
- `CONTEXT.md`：调方向前端 / 对话问答 / 对话房间 / 知识库 / 资讯库
- `doc/PKB-KNOWLEDGE-SKELETON-PLAN.md`（上游体系与调方向前端）
