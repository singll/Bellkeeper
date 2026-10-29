# prod 的 NFS 挂载与写入

2026-10-29 核验；录像挂载在北京时间 2026-10-30 凌晨完成。所有远程操作使用 PATH 中的 `spool`。旧 keeper `.230` 和旧 Python TrueNAS RPC 脚本不是当前入口。

## 实际路径

| 数据 | TrueNAS 导出 | prod 挂载点 | 使用方 |
|---|---|---|---|
| 生产服务数据 | `192.168.7.121:/mnt/NAS/data/prod` | `/mnt/NAS/data/prod` | `store/` 下各服务持久数据与备份 |
| knowledge | `192.168.7.121:/mnt/NAS/data/knowledge` | `/mnt/NAS/data/knowledge` | Bellkeeper 的 `prod-knowledge` NFS volume → `/mnt/knowledge:rw` |
| 录像 | `192.168.7.121:/mnt/NAS/data/media/recordings` | `/mnt/NAS/data/media/recordings` | prod 的录像读写入口；复用 NAS 原有数据集 |

三个挂载均为 `nfs4`、`rw`、`vers=4.1`、`hard`，已写入 prod `/etc/fstab`。knowledge 原来已挂载且已接入 Bellkeeper；本次增加录像的 NFS 导出（ID 19）和宿主挂载，没有搬动历史录像。

TrueNAS 的录像导出仅允许 prod `192.168.7.128/32`，与现有 prod 数据共享一样使用 `mapall=root`。knowledge 沿用既有 `192.168.7.0/24` 和身份映射。NFS 文件显示的 UID 可能与调用进程不同，不能凭 UID 显示判断写入失败，也不要对既有内容递归 chown/chmod。

CouchDB 实际数据在 `/mnt/NAS/data/prod/store/couchdb`，经 `prod-couchdb` NFS volume 映射到 `/opt/couchdb/data`。`NAS/data/prod/couchdb` 是另一个既有 dataset，不能把它当作当前数据库路径。

## 挂载声明

```fstab
192.168.7.121:/mnt/NAS/data/prod /mnt/NAS/data/prod nfs4 rw,vers=4.1,hard,noatime,_netdev,timeo=600,retrans=2 0 0
192.168.7.121:/mnt/NAS/data/knowledge /mnt/NAS/data/knowledge nfs4 rw,vers=4.1,hard,noatime,_netdev,timeo=600,retrans=2 0 0
192.168.7.121:/mnt/NAS/data/media/recordings /mnt/NAS/data/media/recordings nfs4 rw,vers=4.1,hard,noatime,_netdev,timeo=600,retrans=2 0 0
```

prod/knowledge 原条目未显式写 `rw`，实际挂载为默认读写，已验证。不要重复追加 fstab，不要卸载业务正在使用的挂载来测试。

```bash
spool exec prod 'findmnt -t nfs,nfs4 -o TARGET,SOURCE,FSTYPE,OPTIONS'
spool exec prod 'sudo findmnt --verify --tab-file /etc/fstab'
spool exec prod 'systemctl status mnt-NAS-data-media-recordings.mount --no-pager'
spool nas rpc sharing.nfs.query '[]'
```

本次验证结果：

- 以 prod 日常用户 UID 1001 在三个宿主挂载中分别创建唯一临时文件、写入 256 KiB、fsync、读回校验、重命名、删除，全部通过。
- Bellkeeper 容器写入后宿主读到；宿主修改后容器读到，双向检查通过。
- fstab 无解析或挂载错误；唯一警告为既有光驱文件系统列表，与 NFS 无关。
- 内核日志未发现最近 24 小时 NFS stale、服务器无响应或 I/O 错误。
- 未重启 prod 主机；开机挂载持久性通过 fstab 和 systemd 生成的 mount unit 核验。

## 容器如何接入

Bellkeeper 已用直接 NFS volume 连接 knowledge，重启不依赖宿主 bind mount 的启动时序。新录像服务可采用同一方式：

```yaml
volumes:
  prod-recordings:
    name: prod-recordings
    driver: local
    driver_opts:
      type: nfs
      o: "addr=192.168.7.121,vers=4.1,rw,hard,noatime,timeo=600,retrans=2"
      device: ":/mnt/NAS/data/media/recordings"
# 在实际录像服务 volumes 中添加 prod-recordings:/recordings:rw，
# 再把应用的录像输出目录设为 /recordings 下的对应子目录。
```

如果用宿主 bind mount，应先验证该目录确实是 NFS，并给启动服务配置 `RequiresMountsFor=/mnt/NAS/data/media/recordings`，防止 NAS 未挂载时写入 prod 系统盘。

本次没有在 prod 新建或迁移录像程序。istoreos 的 `bilirecorder.singll.net` 仍指向现有 `.108:2356`；给 prod 增加存储挂载不代表录像 Web 服务已迁到 prod。

## knowledge 与 Obsidian

NAS `knowledge/vault` 中的整理稿通过 prod 独立 `knowledge-sync` 服务自动发布到 `knowledge/obsidian/20-PKB/Bellkeeper/`，再由官方 LiveSync CLI 加密同步至各终端。源目录只读挂入发布服务，目标使用直接 NFS volume；CLI 的 PouchDB 与账本保存在 prod 本地磁盘。

用户的两台 Obsidian 已连接并同步，现有库不重新初始化。raw、archive、工作流日志和录像不进入这条链路；Windows 脚本只保留手动补导用途。见 [LiveSync 接入和固定目录](../knowledge-base/setup/livesync-checklist.md)。

## 备份与恢复

本次私有备份：`/opt/SilkSpool/backups/nfs-livesync-20261029/`。包含 fstab 原件、NFS 导出、CouchDB 原配置及验证结果；prod 的原 fstab 另存 `/etc/fstab.spool-before-recordings-20261029`。当前 fstab 快照位于控制机 `hosts/prod/nfs/fstab`，不要自动把含本机磁盘 UUID 的整份 fstab 覆盖到另一主机。
