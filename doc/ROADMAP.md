# Bellkeeper 演进规划

> 更新日期: 2026-07-26
>
> 反馈环：[STATUS.md](STATUS.md)（现状）→ ROADMAP（待办）→ git commit（实施）→ STATUS（回写）。
> 历史里程碑见 [TIMELINE.md](TIMELINE.md)；已完成的一次性计划原文已删除（可查 git 历史）。

---

## 0. 现状

**1.0 已 GA，里程碑 M1–M6 全部完成，系统进入稳定运行**（详见 [TIMELINE.md](TIMELINE.md)）。以下为剩余待办，多数为数据运营与 P2/P3 增强，无 P0 阻塞。

| 优先级 | 类别 | 摘要 |
|--------|------|------|
| **P1** | 运维收尾 | silkdata 割接收尾（删旧卷/停用 LXC/降内存）(§1) |
| **P1** | PKB 运营 | 存量 raw 分批跑 + cron 固化 + 线上验收（数据运营）(§2) |
| **P1** | 爬虫运营 | 新源批量导入 + 7 天成功率验收 + 周健康报告 (§4) |
| **P2** | LLM | cache 命中率监控（new-api 已退役，§3） |
| **P2** | 前端/问答/工程 | Vault 预览增强、问答优化、API 契约测试 (§5–§7) |
| **P3** | 远期 | 按需启动 (§8) |

---

## 1. 运维收尾（P1）

- [ ] silkdata 稳定 ≥7 天后删 keeper 旧数据卷（~15G）
- [ ] `pct destroy` 停用数据 LXC 107（mongodb）/108（redis）/109（mysql）
- [ ] knowledge guest 32→16G 降内存（内部实占 ~5.3G）
- [ ] keeper 单容器观测指标缺口（Docker 29 + cAdvisor）：试更新 cadvisor 版本或改回 overlay2 存储驱动

## 2. PKB 运营（P1，数据运营为主）

- [ ] 存量 raw 按预算分批跑完（`pkb-curate`，经 llm_jobs 队列）
- [ ] cron / 调度参数固化；按需开启 `feature_pkb_auto_fill/feed/propose`（默认关）
- [ ] 线上验收：打分合理性、LiveSync 同步范围（raw 不进）、Meili rebuild 结果
- [ ] Phase E 遗留：独立 audit API 端点（`/api/pkb/audit`）
- [ ] `spool n8n export --to-source` 冷备进 Git + 线上漂移检测

## 3. LLM Proxy（P2）

- [ ] prompt cache 命中率统计/监控（当前仅提取 cache token 用于计费，未暴露命中率）
- [x] 停 new-api 容器 → **已完成**：new-api 已退役下线（调用方 base_url 迁移随 M2 完成）

## 4. 爬虫运营（P1）

- [ ] 批量验证候选源，导入 20–40 个高成功率源
- [ ] 7 天观察：RSS 拉取成功率 > 90%
- [ ] 周源健康报告（自动暂停已实现：`ConsecutiveFailures≥5` 暂停 / `HealthScore≥30` 恢复）

## 5. 前端增强（P2）

- [ ] Vault 预览增强：Markdown 渲染 + frontmatter 折叠 + `[[wikilink]]` 跳转（当前 `<pre>` 纯文本）
- [ ] Dashboard 时间序列图表（当前仅指标卡，无图表库）

## 6. 工程质量（P2）

- [ ] API 契约测试或 OpenAPI/类型生成
- [ ] 配置热重载推广（LLM Proxy/通知渠道已实现；PKB/RSS 等未推广）

## 7. 知识问答优化（P2）

- [ ] 上下文压缩：片段独立摘要后拼接（当前 rerank 后直接拼接 + 截断）
- [ ] 引用结构化：补 `line_range/score` + 前端跳转
- [ ] 多源检索：可选包含 vault / todos
- [ ] 历史会话持久化：`qa_sessions/qa_messages`（当前纯前端传递）

## 8. 远期（P3）

- [ ] K07 Obsidian 回流端到端验证
- [ ] 文件级权限标签 + 检索过滤
- [ ] 存量知识批量导入 / Vault 在线编辑 / 元数据批量操作

**已取消项**：
- ~~智能归档建议~~ — 已被 PKB `pkb-curate` 漏斗取代
- ~~Embedding 端点~~ — 无消费方，有真实需求再立项
- ~~n8n 通知链路降层~~ — 维持 B01 模板渲染 + NATS 现状

---

## 维护规则

1. 完成一项：本文打勾或删除 → STATUS.md 回写现状 → 大架构变化同步 ARCHITECTURE.md → 里程碑级成果追加 TIMELINE.md。
2. 新增任务：按 P1–P3 评估加入对应章节，不另开新文档；大型计划才单独立文档并在此索引。
3. 计划类文档（\*-PLAN/\*-REVIEW）完成后从 doc/ 删除（git 历史留痕），残留待办转本文；仍生效的运维/知识库文档归 `doc/ops/`、`doc/knowledge-base/`。
