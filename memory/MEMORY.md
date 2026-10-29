# Bellkeeper MEMORY

> 更新：2026-10-29。当前权威状态见 [STATUS](../doc/STATUS.md)，操作见 [DEPLOYMENT](../doc/DEPLOYMENT.md)。

- 当前主机 `prod` (.128)，应用 `prod-stack`，数据 `proddata`，存储 TrueNAS NFS；旧 keeper/silkdata/knowledge 不再是生产目标。
- Matrix 在 txhk 的 tuwunel 1.8.0；Spool 已修正服务名、同步路径和安装源，禁止恢复 Conduit 配置。
- Bellkeeper 已接入 6 个 Matrix 房间，9 个 n8n 工作流、77 个技术资讯源；Matrix 命令/两类聊天/通知已实测。
- Matrix 凭证和 Agent 配置需重启；只有通知频道、LLM 配置支持对应热重载。
- 修复 Trafilatura 2.0 `headers` 参数不兼容；当前无 Firecrawl 地址，已关闭无效回退。
- 自动重试使用持久 CrawlQueue；旧 n8n staticData 重试队列及 M01–M03 不再使用。
- secret 在 `/opt/SilkSpool/hosts/prod/.env` 和受限 YAML；不提交 Git。

- 生产补丁二进制在 `/app/data/bellkeeper-matrix`，现有 `/app/bellkeeper` 为软链，bundle entrypoint 已持久化；下次镜像升级含修复后移除覆盖，详见 DEPLOYMENT。
- RSSHub 独立 bundle 已上线，四个相对路由抓取成功（含已替换旧 URL 的 36氪）；酷壳上游 403/超时待自动恢复。初次恢复会收录历史文章，资讯预览为当前入库内容。
