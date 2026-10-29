# 已有 Obsidian 库接入 LiveSync

2026-10-29 服务端已配置，复用 istoreos 上的 `https://couchdb.singll.net`，反代到 prod `192.168.7.128:5984`。用户确认接入已有 Obsidian 库，因此首次方向为 **已有本地 Vault → 新远端数据库**。

## 已完成的服务端配置

| 项目 | 值 |
|---|---|
| CouchDB URL | `https://couchdb.singll.net`（不要附加数据库名） |
| Database name | `obsidian-notes` |
| Username | `obsidian-sync` |
| Password | 控制机 `/opt/SilkSpool/hosts/prod/couchdb/obsidian-livesync.env` 的 `LIVESYNC_PASSWORD` |
| 数据位置 | TrueNAS `/mnt/NAS/data/prod/store/couchdb` → 容器 `/opt/couchdb/data` |
| 配置位置 | 持久 volume `prod-couchdb-config`；基线 `hosts/prod/couchdb/10-livesync.ini` |
| 访问网络 | 能访问 istoreos 的内网或已有 VPN/子网路由 |

专用凭证在 prod 也保存于 `/opt/silkspool/proddata/couchdb/obsidian-livesync.env`（0600）。不要使用共享管理员密码，也不要把这个文件写进 Vault、聊天或版本库。

LiveSync 的首次初始化/完整重建会删除并重建目标数据库，因此该专用账号是此私有 CouchDB 实例的管理员。只给数据库 member/admin 权限不能完成这一流程；不要把凭证发给不应管理该实例的人。

已配置 Obsidian 的三个 CORS origin：`app://obsidian.md`、`capacitor://localhost`、`http://localhost`，允许带认证跨域。普通 API 要求登录；`/_up` 保留无凭证探活，保证 Docker/Bellkeeper 监控正常。请求/文档大小遵循官方 LiveSync 配置。Caddy 保留长轮询转发，不经过 Authelia 登录跳转。

服务端已通过 HTTPS、TLS 证书、登录、CORS 预检、匿名拒绝、数据库创建、批量写入、读回、`_changes`、更新、删除测试，且测试库已删除。`obsidian-notes` 当前为 0 篇文档，客户端的协议元数据和加密信息由第一台 Obsidian 初始化。

## 第一台设备：保留已有笔记，上传到新远端

1. 将现有 Vault 完整复制一份到同步目录以外，包含 `.obsidian`。打开原来的已有 Vault，不必新建空库，也不必改目录结构。若同一个 Vault 已被另一种同步工具管理，先暂停它，避免两个工具同时处理文件。
2. 安装并启用社区插件 **Self-hosted LiveSync**。如果此前用过它，先保存原连接信息，再把这次连接作为新的同步目标配置。
3. 在欢迎向导选择“首次设置”，再选择手动配置远端。这里“首次”是指新的服务端同步关系，不是要求 Vault 为空。
4. 启用端到端加密，设置并妥善保存独立的 Vault 加密口令，可启用属性混淆。该口令由你在客户端设置，与 CouchDB 密码、之后 Setup URI 的保护口令不同。
5. 选择 CouchDB，填写上表 URL、用户名、密码、数据库名，执行连接检查，然后继续。服务端参数已配置，无需再自动修复服务端。
6. 首次初始化选择以当前本地内容为准重建/上传服务端。当前界面会要求重启并初始化，以及确认覆盖服务端：只有确认 URL 为 `couchdb.singll.net`、数据库为本次新建的 `obsidian-notes`，且已备份本地库时才继续。**不要选择从空远端覆盖本地。**
7. 若提示远端没有同步设置，对本次已确认的新库选择使用当前设备设置。等待初始化完成；保持 Obsidian 打开，先不启用配置文件/插件等可选同步功能。
8. 新建普通测试笔记，修改两次，确认插件队列清空，再在第二台设备验证内容。

如果连接时发现远端已经有笔记或预期之外的设置，先停止覆盖操作。本文的“空库”是部署验收时的状态，不是永久允许清空数据库。

## 第二台及后续设备

1. 第一台同步正常后，在命令面板运行 **Self-hosted LiveSync: Copy settings as a new Setup URI**，设置独立的 URI 保护口令。
2. 新设备创建空 Vault，安装插件，选择“加入已有同步”，导入刚生成的 Setup URI。
3. 对新建空 Vault 选择从远端取回；额外本地文件选择保留。已有本地工作的设备先备份并合并，不能直接按空库处理。
4. URI 含加密后的账号和同步配置，和其保护口令分开保存；同一 Vault 的后续设备使用第一台导出的设置，不要分别生成不同的加密配置。

## knowledge 文件如何进入已有 Vault

```text
已有本地 Obsidian Vault ←→ Self-hosted LiveSync ←→ CouchDB obsidian-notes
           ↑ 选取 Markdown 导入
TrueNAS knowledge ←→ prod NFS ←→ Bellkeeper
TrueNAS recordings ←→ prod NFS（录像，不进入 LiveSync）
```

CouchDB 是插件的同步数据库，不会自动将 NFS 上的 `knowledge/vault` 镜像进已有 Obsidian 库。本次已完成两条服务端接入，不会自动混合已有笔记与抓取历史文件。导入选用 [SilkFiles knowledge 流程](windows-knowledge-sync.md)，先 plan/dry-run，再选择需要的 Markdown；大附件保留在 NAS，只在笔记中存引用。

旧说明中的 K07 只是历史可选构想，当前 9 个 n8n 工作流没有 K07。LiveSync 数据有分块、协议元数据和可选加密，不能直接把 `_changes` 当普通 Markdown 文本消费。

## 故障定位与备份

- `https://couchdb.singll.net/_up` 应返回 `status: ok`；若域名不可达，先检查内网/VPN 和 istoreos DNS。不要回退到旧 keeper 地址。
- `401`：核对专用用户名、密码；数据库创建/初始化 `403`：核对账号是否有首次初始化所需权限。
- 服务端配置备份在 `/mnt/NAS/data/prod/store/backups/couchdb-config/20261030/local.d.tar.gz`，包含管理员密码哈希，按 secret 保护。控制机原始备份在 `/opt/SilkSpool/backups/nfs-livesync-20261029/`。
- 定期独立备份本地 Vault 和 NAS 数据；同步不是删除/冲突恢复的备份替代品。

当前向导以 [LiveSync 官方初始化指南](https://github.com/vrtmrz/obsidian-livesync/blob/main/docs/quick_setup.md) 为准；服务端参数见 [官方 CouchDB 部署说明](https://github.com/vrtmrz/obsidian-livesync/blob/main/docs/setup_own_server.md)。不同插件版本的按钮翻译可能不同，但首台“本地上传”、后续“远端取回”的方向不变。
