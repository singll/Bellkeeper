# TrueNAS NFS 共享配置指南

> **目标**: 将 TrueNAS 的 `NAS/data/knowledge` 数据集通过 NFS 共享给 keeper 宿主机

---

## 环境信息

- **TrueNAS**: 192.168.7.121 (truenas.singll.net)
- **数据集**: NAS/data/knowledge
- **TrueNAS 挂载点**: /mnt/NAS/data/knowledge
- **宿主机挂载点**: /mnt/NAS/data/knowledge
- **keeper 宿主机**: 通过 `spool exec keeper` 访问

---

## 步骤 1: 创建 NFS 共享（TrueNAS 端）

### 1.1 检查现有 NFS 共享

```bash
# 在本地执行
cd /home/ubuntu/SilkSpool
python3 lib/core/truenas_rpc.py sharing.nfs.query '[]'
```

### 1.2 创建 NFS 共享

```bash
# 创建 NFS 共享配置
python3 lib/core/truenas_rpc.py sharing.nfs.create '{
  "path": "/mnt/NAS/data/knowledge",
  "comment": "Knowledge base files for Bellkeeper",
  "networks": ["192.168.0.0/16"],
  "hosts": [],
  "ro": false,
  "maproot_user": "apps",
  "maproot_group": "apps",
  "mapall_user": "",
  "mapall_group": "",
  "security": ["SYS"]
}'
```

**参数说明**:
- `path`: TrueNAS 上的数据集路径
- `networks`: 允许访问的网络段（192.168.0.0/16 覆盖整个内网）
- `ro`: false 表示读写权限
- `maproot_user/group`: 映射 root 用户到 apps 用户（UID 568）
- `security`: SYS 表示使用 Unix 权限

### 1.3 启用 NFS 服务

```bash
# 检查 NFS 服务状态
python3 lib/core/truenas_rpc.py service.query '[["service", "=", "nfs"]]'

# 如果未启动，启用并启动 NFS 服务
python3 lib/core/truenas_rpc.py service.update 'nfs' '{"enable": true}'
python3 lib/core/truenas_rpc.py service.start '"nfs"'
```

### 1.4 验证 NFS 导出

```bash
# 在 keeper 宿主机上检查
spool exec keeper "showmount -e 192.168.7.121"
```

预期输出应包含：
```
/mnt/NAS/data/knowledge 192.168.0.0/16
```

---

## 步骤 2: 配置宿主机挂载（keeper 端）

### 2.1 安装 NFS 客户端

```bash
spool exec keeper "sudo apt-get update && sudo apt-get install -y nfs-common"
```

### 2.2 创建挂载点

```bash
spool exec keeper "sudo mkdir -p /mnt/NAS/data/knowledge"
```

### 2.3 测试手动挂载

```bash
spool exec keeper "sudo mount -t nfs -o vers=4,rw 192.168.7.121:/mnt/NAS/data/knowledge /mnt/NAS/data/knowledge"

# 验证挂载
spool exec keeper "df -h | grep knowledge"
spool exec keeper "ls -la /mnt/NAS/data/knowledge"
```

### 2.4 配置自动挂载

```bash
# 添加到 /etc/fstab
spool exec keeper "echo '192.168.7.121:/mnt/NAS/data/knowledge /mnt/NAS/data/knowledge nfs vers=4,rw,hard,intr,timeo=600,retrans=2,_netdev 0 0' | sudo tee -a /etc/fstab"

# 测试 fstab 配置
spool exec keeper "sudo umount /mnt/NAS/data/knowledge"
spool exec keeper "sudo mount -a"
spool exec keeper "df -h | grep knowledge"
```

**挂载选项说明**:
- `vers=4`: 使用 NFSv4 协议
- `rw`: 读写模式
- `hard`: 硬挂载（NFS 不可用时会重试）
- `intr`: 允许中断挂载操作
- `timeo=600`: 超时时间 60 秒
- `retrans=2`: 重传次数
- `_netdev`: 等待网络就绪后再挂载

### 2.5 设置权限

```bash
# 检查当前权限
spool exec keeper "ls -ld /mnt/NAS/data/knowledge"

# 如果需要，调整权限（通常 TrueNAS 已设置好）
# spool exec keeper "sudo chown -R 568:568 /mnt/NAS/data/knowledge"
```

---

## 步骤 3: 修改 product compose

### 3.1 修改 Bellkeeper 仓库

在 `deploy/compose.yaml` 的 `bellkeeper` 服务 `volumes` 中保留：

```yaml
volumes:
  - kp-bellkeeper-data:/app/data
  - ${BELLKEEPER_KNOWLEDGE_PATH:-/mnt/knowledge}:/mnt/knowledge:rw
```

`BELLKEEPER_KNOWLEDGE_PATH` 由 `knowledge-stack` 的 `defaults` / `hosts/keeper/.env` 提供，
不要把宿主 IP 或 `.env` 路径写进 compose。

### 3.2 发布并应用

1. 提交 Bellkeeper 仓库的 `deploy/` 变更并发布新版本；
2. 更新 `silkspool.yaml` 中 `bundle_sources.bellkeeper.commit`（或 OCI digest）；
3. 在 SilkSpool 控制机执行：

```bash
spool plan knowledge-stack keeper --offline
spool plan knowledge-stack keeper
spool apply knowledge-stack keeper --locked --yes

# 验证挂载
spool exec keeper "docker exec sp-bellkeeper ls -la /mnt/knowledge"
```

---

## 步骤 4: 验证完整链路

### 4.1 在 TrueNAS 上创建测试文件

```bash
python3 lib/core/truenas_rpc.py filesystem.file_receive '"/mnt/NAS/data/knowledge/test.txt"' <<EOF
This is a test file from TrueNAS
EOF
```

### 4.2 在宿主机上验证

```bash
spool exec keeper "cat /mnt/NAS/data/knowledge/test.txt"
```

### 4.3 在容器内验证

```bash
spool exec keeper "docker exec sp-bellkeeper cat /mnt/knowledge/test.txt"
```

### 4.4 测试写入权限

```bash
spool exec keeper "docker exec sp-bellkeeper sh -c 'echo \"Test from container\" > /mnt/knowledge/container-test.txt'"
spool exec keeper "cat /mnt/NAS/data/knowledge/container-test.txt"
```

### 4.5 清理测试文件

```bash
spool exec keeper "docker exec sp-bellkeeper rm /mnt/knowledge/test.txt /mnt/knowledge/container-test.txt"
```

---

## 故障排查

### 问题 1: showmount 显示 "No exports"

**原因**: NFS 服务未启动或共享未创建

**解决**:
```bash
python3 lib/core/truenas_rpc.py service.start '"nfs"'
python3 lib/core/truenas_rpc.py sharing.nfs.query '[]'
```

### 问题 2: mount 失败 "access denied"

**原因**: 网络段不匹配或权限配置错误

**解决**:
1. 检查 keeper 宿主机 IP 是否在 `192.168.0.0/16` 范围内
2. 检查 NFS 共享的 `networks` 配置
3. 尝试添加宿主机 IP 到 `hosts` 列表

### 问题 3: 容器内无法写入

**原因**: UID/GID 映射问题

**解决**:
```bash
# 检查容器内用户
spool exec keeper "docker exec sp-bellkeeper id"

# 检查 TrueNAS 文件权限
python3 lib/core/truenas_rpc.py filesystem.stat '"/mnt/NAS/data/knowledge"'

# 调整 maproot_user 为容器内的用户（通常是 bellkeeper 或 apps）
```

### 问题 4: 重启后挂载丢失

**原因**: fstab 配置错误或网络未就绪

**解决**:
1. 检查 fstab 是否包含 `_netdev` 选项
2. 手动重新挂载：`sudo mount -a`
3. 检查系统日志：`journalctl -u remote-fs.target`

---

## 安全建议

1. **限制网络访问**: 生产环境建议将 `networks` 改为具体的 IP 段或使用 `hosts` 指定特定主机
2. **只读挂载**: 如果 Bellkeeper 只需读取，可以设置 `ro: true`
3. **防火墙规则**: 确保 TrueNAS 防火墙允许 NFS 端口（2049）
4. **定期备份**: NFS 共享的数据应定期快照备份

---

## 下一步

完成 NFS 配置后，继续执行 IMPLEMENTATION.md 的 Phase 1.2（配置文件扩展）。
