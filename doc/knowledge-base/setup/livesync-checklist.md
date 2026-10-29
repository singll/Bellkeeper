# 已有 Obsidian 库接入 LiveSync

2026-10-30：用户已完成两台电脑的接入与内容同步。当前使用 `https://couchdb.singll.net` → prod `192.168.7.128:5984`，数据库 `obsidian-notes` 已有笔记且开启内容加密。**不要再次执行首次上传/覆盖远端流程。** 新终端按下文“加入已有同步”接入。

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

服务端认证、CORS、TLS、批量写入和变更流已验证。初始化时客户端为 Self-hosted LiveSync 0.25.57，内容加密 v2、路径未混淆；新增服务器客户端沿用导出的参数，不更改现有库的分块/加密方案。

## 已完成的首次初始化

第一台已有 Vault 已上传、第二台已取回并同步。此前“空数据库”的描述只适用于初始部署验收，现已失效。以后恢复/重建必须先独立备份并明确数据方向；本次服务器加入仅重建自己的空本地缓存，不删除远端数据库。

## 第二台及后续设备

1. 第一台同步正常后，在命令面板运行 **Self-hosted LiveSync: Copy settings as a new Setup URI**，设置独立的 URI 保护口令。
2. 新设备创建空 Vault，安装插件，选择“加入已有同步”，导入刚生成的 Setup URI。
3. 对新建空 Vault 选择从远端取回；额外本地文件选择保留。已有本地工作的设备先备份并合并，不能直接按空库处理。
4. URI 含加密后的账号和同步配置，和其保护口令分开保存；同一 Vault 的后续设备使用第一台导出的设置，不要分别生成不同的加密配置。

## Knowledge 自动进入固定目录

```text
Bellkeeper → TrueNAS knowledge/vault（整理后的 Markdown）
  → prod 的 knowledge-sync 发布器
  → TrueNAS knowledge/obsidian/20-PKB/Bellkeeper
  ↔ 官方 LiveSync CLI ↔ CouchDB obsidian-notes
  ↔ 各终端现有 Vault 的 20-PKB/Bellkeeper/
```

服务器已作为新增客户端加入，只落盘这个固定目录。按原领域保留子目录；新增和更新自动发布，内容相同不重复导入。终端编辑/删除受保护，源更新冲突进入清单；NAS 原件保留。同步落后/冲突通过 Bellkeeper 健康探针进入现有 n8n O02/Matrix 告警。raw、archive、录像、工作流日志不在自动发布范围内。

现有两台 Obsidian **无需重新初始化，也无需运行 Win10/Win11 拉取脚本**。打开已同步的 Vault 后查看 `20-PKB/Bellkeeper/`。旧 [Windows 脚本](windows-knowledge-sync.md) 仅用于人工补导其他资产，避免再次导入同一批整理稿。

服务部署、状态检查和恢复见 [SilkSpool knowledge-sync](../../../../SilkSpool/bundles/knowledge-sync/README.md)。私有 Setup URI 文件为控制机 `/opt/SilkSpool/hosts/prod/couchdb/knowledge-livesync-setup.env`；服务器设置在 `/opt/silkspool/knowledge-sync/state/.livesync/settings.json`，均不进版本库或 Vault。

当前没有 K07 工作流；分块和加解密交给 [官方 CLI](https://github.com/vrtmrz/obsidian-livesync/tree/main/src/apps/cli)，不能用 n8n 将普通 JSON 直接写入 LiveSync 数据库。

## 故障定位与备份

- `https://couchdb.singll.net/_up` 应返回 `status: ok`；若域名不可达，先检查内网/VPN 和 istoreos DNS。不要回退到旧 keeper 地址。
- `401`：核对专用用户名、密码；数据库创建/初始化 `403`：核对账号是否有首次初始化所需权限。
- 服务端配置备份在 `/mnt/NAS/data/prod/store/backups/couchdb-config/20261030/local.d.tar.gz`，包含管理员密码哈希，按 secret 保护。控制机原始备份在 `/opt/SilkSpool/backups/nfs-livesync-20261029/`。
- 定期独立备份本地 Vault 和 NAS 数据；同步不是删除/冲突恢复的备份替代品。

当前向导以 [LiveSync 官方初始化指南](https://github.com/vrtmrz/obsidian-livesync/blob/main/docs/quick_setup.md) 为准；服务端参数见 [官方 CouchDB 部署说明](https://github.com/vrtmrz/obsidian-livesync/blob/main/docs/setup_own_server.md)。不同插件版本的按钮翻译可能不同，但首台“本地上传”、后续“远端取回”的方向不变。
