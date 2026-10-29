# Bellkeeper 运行状态

> 2026-10-29 生产实测。历史里程碑见 [TIMELINE.md](TIMELINE.md)，部署操作见 [DEPLOYMENT.md](DEPLOYMENT.md)。

## 当前拓扑

| 主机 | 地址 | 当前职责 |
|---|---|---|
| prod | 192.168.7.128 | `prod-stack`：Bellkeeper 2.4.3 + Matrix 修复补丁、n8n 2.44.6；`proddata`：PostgreSQL、Redis、NATS、Meilisearch、CouchDB；独立 RSSHub |
| txhk | 腾讯云 HK | tuwunel 1.8.0、Caddy、Headscale、Authelia、RDP 网关 |
| TrueNAS | 192.168.7.121 | NFS `/mnt/NAS/data/prod/store` 及 `/mnt/NAS/data/knowledge` 持久存储 |
| istoreos | 192.168.7.1 | DNS、Caddy、代理、子网路由 |

旧 keeper(.230)、silkdata(.231)、knowledge(.220) 不是当前 Spool 生产目标。旧 Prometheus/Loki/Grafana 的历史验收不能视为本机当前运行状态。当前告警使用 n8n、Bellkeeper 日志中心和 Matrix。
Memos 当前未部署；待办频道和命令已停用，禁用时不再注册 n8n Memos 回退。Firecrawl 没有有效服务地址，已关闭无效回退，正文提取使用已实测的 Trafilatura 2.0。

## Matrix 房间

账号：`@bellkeeper:matrix.singll.net`；管理员：`@singll:matrix.singll.net`，已邀请到全部房间。
所有房间采用邀请加入，访客禁止加入；机器人接入的房间未启用端到端加密。

| 房间 | 类型 / 频道 | 入口 |
|---|---|---|
| 机器人命令 | command | [打开](https://matrix.to/#/!sUhDtrkWVUidmRmKln:matrix.singll.net) |
| 知识聊天室 | chat / qa | [打开](https://matrix.to/#/!pt86Tgws4HhHgWBuUi:matrix.singll.net) |
| 自由聊天 | direct | [打开](https://matrix.to/#/!Ov9F0IzKcd5dJL7UK0:matrix.singll.net) |
| 技术资讯 | notification / news | [打开](https://matrix.to/#/!13hO0X4QDPEctQ2oFg:matrix.singll.net) |
| 系统告警 | notification / alerts | [打开](https://matrix.to/#/!BC6V5eM69Brfq3u1O6:matrix.singll.net) |
| 运行日报 | notification / daily | [打开](https://matrix.to/#/!L4JaaVlNJmVTJioCvZ:matrix.singll.net) |

`!ping`、自由聊天、知识库工具检索均已通过真实 Matrix 收发验证。Agent 默认使用 `pool-chat-balanced`，`!model` 可切换；每房间 120 回合/小时，会话 30 分钟。
Matrix 凭证与 Agent 开关只在启动时加载；房间和频道存数据库，频道支持 API 重载。

## 知识与通知链路

```text
77 个既有技术资讯源 → RSSFetcher → 持久化 CrawlQueue → Trafilatura 正文提取
  → NATS crawl/extract/index → Markdown + PostgreSQL + Meilisearch
  → 分类与 PKB 治理 → 资讯重要性评分 → news 房间
用户 → Matrix Gateway → !命令 / chat 知识工具 / direct 模型对话
n8n / Bellkeeper 日志 / LLM 聚合告警 → alerts 房间
```

URL 与内容去重、域名冷却和失败重试由 Bellkeeper 负责。资讯沿用编程、人工智能、网络安全和技术资讯范围，重要性评分阈值 6/10，每领域最多 15 条、打分候选最多 60 条；噪声社交/视频域名由现有规则过滤。评分失败时现有实现会记录降级并按时间取前 N，不宣称此时已完成评分过滤。
首次恢复订阅会读取源内历史条目，积压由队列限速处理；7 天成功率属于后续观察。

## n8n 工作流

源文件：`internal/n8n_workflows/`；生产副本：`/opt/SilkSpool/hosts/prod/n8n-workflows/`。9 个工作流启用。

| 工作流 | 职责 |
|---|---|
| B02 | 工作流失败直达 Matrix；不依赖 Bellkeeper 可用性 |
| K01 | 单 URL 校验、提交持久队列、返回入队/去重结果 |
| K02 | 每日 06/12/18 点触发到期 RSS 采集；后端仍按源间隔调度 |
| K03 | 批量 URL 入队，最多 100 个 |
| K04 | 每 15 分钟检查持久队列终止任务；自动重试由后端负责 |
| K08 | 08:00 生成资讯早报并发往 news；取材周期前日 06:00 至当日 06:00 |
| O01 | 21:00 运行日报；21:20 后端检查日报缺失 |
| O02 | 每 5 分钟检查服务状态，仅告警变化，忽略 disabled 服务 |
| O03 | 每 5 分钟评估日志告警规则，经通知服务去重推送 |

以上时间均为 Asia/Shanghai。旧 M01–M03、B01 属归档，不参与 Matrix 轮询或当前业务处理。
日志规则覆盖系统 error、RSS 采集失败、抓取处理失败和正文提取失败。LLM 告警沿用内置聚合器，经 alerts 发送。

## 验证与后续观察

- Matrix 通知状态为 sent，机器人命令返回 Pong，自由聊天返回模型回答，知识聊天室返回检索结果及来源。
- K01 重复提交已入队/入库 URL 返回 duplicate；受控无效 URL 触发 B02，错误告警执行成功。
- 当日运行日报已生成、归档并通知 daily。
- O02/O03 已有成功执行，日志规则实际触发并送达；正文提取修复后已产出成功入库记录。
- 四个 RSSHub 路由均获取成功；实际文章经评分选出 20 条预览并推送到资讯房间。初次恢复包含历史资料，预览明确标注为当前入库内容。
- 重启后 6 个房间的名称、类型、停用状态保留；未启用的待办命令不可执行或出现在帮助中。
- `go test ./...`、`go vet ./...`、`go build ./...` 通过；额外覆盖真实 HTTP 提取、工作流脚本、命令禁用和重新启用。
- 首轮恢复后的 RSS 成功率、积压消化速度继续观察；不把瞬时成功等同长期稳定。
- 当前未部署旧观测中心栈；后续按需求在 prod 建立独立指标/日志后端。

2026-10-29 已移除不可达的旧 Memos、RAGFlow、Firecrawl、MinIO、Grafana 首页/DNS/反代入口；原配置保存在私有接入备份目录，不能把旧主机地址当作当前服务。

订阅验收：76/77 至少成功获取一次。36氪旧 `/feed` 已失效，已改为本地 RSSHub `/36kr/newsflashes`；酷壳上游返回 403/超时，保留原订阅由健康探测恢复，不绕过访问限制。

## NFS 与 Obsidian（2026-10-30）

prod、knowledge、recordings 三个 NFS 挂载都已通过写入/fsync/读回/重命名/删除测试，knowledge 的容器与宿主双向读写也通过。录像路径为 `/mnt/NAS/data/media/recordings`，已持久化到 fstab，未迁移录像程序。

CouchDB 3.3.3 复用 `https://couchdb.singll.net`，已配置认证、LiveSync CORS、大小限制和持久配置；`/_up` 保留健康探测。用户已完成两台电脑的加密同步，`obsidian-notes` 已有数据，不再按空库初始化。专用凭证在私有 `hosts/prod/couchdb/obsidian-livesync.env`。

Knowledge 整理稿通过 prod 独立 `knowledge-sync` 服务发布，入口 `knowledge/vault`，NAS 局部 Vault `knowledge/obsidian`，各终端固定目录 `20-PKB/Bellkeeper/`。官方 LiveSync CLI 处理加密协议，发布器保护终端编辑和删除；raw、archive、工作流日志和录像不进入自动同步。操作见 [LiveSync 接入](knowledge-base/setup/livesync-checklist.md)。
