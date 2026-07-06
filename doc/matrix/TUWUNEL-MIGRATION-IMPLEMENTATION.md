# txhk Conduit → tuwunel 替换实施文档

> ℹ️ **2026-07-02 复核定案**：命门②（迁移）经复核翻案——tuwunel **现官方支持 Conduit→tuwunel in-place 保数据迁移**（详见 `HOMESERVER-MIGRATION-EVAL.md` §5/§7 方案 B）。即 fresh start 不再是被迫路径。**用户在知晓可保数据的前提下，仍确认主动走本手册的方案 A（fresh start，弃旧库清白重来）**——故本手册即为最终执行方案，无需再改。
>
> 决策依据见同目录 `HOMESERVER-MIGRATION-EVAL.md`（选 tuwunel、fresh start、弃旧数据）。
> 本文件是**可执行的实施手册（fresh start 路径）**，所有远程操作走 `spool`。日期：2026-06-19（顶注 2026-07-02）。
> ⚠️ 本操作**不可逆**（清空整个 Matrix homeserver），需维护窗口。用户已确认：暂不执行、只出方案；执行时按本文步骤。

---

## 0. 用户已确认前提（来自评估对话）

- 选 **tuwunel**（conduwuit 官方继承者，Rust/RocksDB，v1.7.1，维护最活跃）。
- **fresh start**：不保留旧聊天记录/房间/账号，用全新空库。
- `server_name` **保持 `matrix.singll.net`**（永久不可改，且 caddy/Bellkeeper 都依赖它）。
- 注册策略：**保持 `allow_registration=true` + token 开放注册**（不收紧）。
- 旧库执行前**留档到 NAS**（异地容灾，库仅 414M）。

---

## 1. 可行性结论（直接回答用户两个问题）

### Q1：能否用「二进制 + 系统服务」方式安装 tuwunel？

**✅ 能，且本方案采用二进制安装**（与现有 conduit/caddy/headscale/ntfy/authelia 全裸二进制 stack 保持一致，最纯净、最好管）。tuwunel 官方发布**全静态预编译单文件二进制**（另有 `.deb`/`.rpm`，本方案不用）。

- txhk = **Ubuntu 22.04 / x86_64 / 2 核**，CPU 支持 `avx2` → 用 **`-v3-` 优化变体**（官方检测脚本 `s/avx2/v3/` 印证）。
- 二进制资产以 **`.zst` 压缩单文件**分发，解压后是无依赖的全静态可执行文件，放 `/usr/local/bin/tuwunel`（与现 conduit 同位置）。
- 资产文件：`v1.7.1-release-all-x86_64-v3-linux-gnu-tuwunel.zst`
  下载 URL：`https://github.com/matrix-construct/tuwunel/releases/download/v1.7.1/v1.7.1-release-all-x86_64-v3-linux-gnu-tuwunel.zst`
- systemd unit 由本方案自备（§4.3 官方加固版），经 spool `sync_rules` 下发——不依赖 `.deb` 自带 unit。

> **为何不用 `.deb`**：`.deb` 会把二进制塞进 `/usr/sbin`、登记进 dpkg 数据库、自带一套 unit，成为这台「全裸二进制 stack」里的异类，破坏统一性且与 spool 的 stack 纳管模型不贴合。裸二进制无包管理器状态、unit 由 spool 完全掌控，是 conduit 的同款形态。

### Q2：能否用 spool 编排管理 tuwunel？

**部分原生、关键部分原生。** 分两层看（证据见 §7）：

| 能力 | 是否 spool 原生 | 怎么做 |
|---|---|---|
| **生命周期管理**（状态/启停/重启/日志） | ✅ **完全原生** | `services:` 注册 `type: systemd`，走 `systemctl`/`journalctl`，与现在管 conduit **完全相同** |
| **配置管理**（tuwunel.toml 下发 + 改后重启） | ✅ **完全原生** | `sync_rules:` + `post_push_hooks:`（`spool sync push` 自动触发重启） |
| **二进制初装** | 取决于路线 | **方案 C（推荐）**：给 spool 加 `.zst` 支持(~15行)→进 `install_sources`，`spool` 一键装/升级，全原生。**方案 B**：不改 spool，`spool exec` 跑一次性脚本（下载.zst→`zstd -d`→落位）。 |

**一句话**：tuwunel 的**日常运维 100% 由 spool 原生纳管**（和 conduit 现状一致）。唯一分叉在「二进制初装」：spool 安装器现只认 `.tar.gz`/裸二进制、不认 `.zst`——**要么给 spool 加 15 行 `.zst` 支持做到完全原生（方案 C，推荐），要么用 `spool exec` 兜底初装一次（方案 B）**。两条路全程都只用 spool，不裸 ssh。

---

## 2. txhk 现状事实（spool 实地确认）

| 项 | 值 | 来源 |
|---|---|---|
| 主机 | `silkspool@43.129.195.4`，Ubuntu 22.04 x86_64 2核/3.6G/无swap | `silkspool.yaml:170-171` / `spool exec` |
| 现 homeserver | Conduit 0.10.11，systemd `conduit.service` | `systemctl cat conduit.service` |
| 二进制 | `/usr/local/bin/conduit`（65MB），用户 `conduit`(uid 997) | `spool exec` |
| 配置 | `/etc/conduit/conduit.toml`（spool 副本 `hosts/txhk/conduit/conduit.toml`） | `silkspool.yaml:180-181` |
| 现配置关键项 | rocksdb / server_name=matrix.singll.net / **port=6167** / db=`/var/lib/conduit/conduit_db` / allow_registration=true / token=`llDHgate9525.` | `conduit.toml` |
| caddy 反代 | `matrix.singll.net { reverse_proxy 127.0.0.1:6167 }`（注释误写「Dendrite」，应订正为 tuwunel） | `hosts/txhk/caddy/Caddyfile:19-22` |
| spool 纳管 | bundle `server`(type:stack)；service `conduit`(systemd)；sync conduit.toml；stack 列表含 conduit | `silkspool.yaml:174,180,212,224` |
| 同机其它 systemd | caddy / headscale / ntfy / authelia（均 spool 纳管） | `silkspool.yaml:205-220` |

**Bellkeeper 侧耦合**（keeper `.env`，fresh start 后需改）：
```
MATRIX_HOMESERVER=https://matrix.singll.net   # 不变（caddy 公网入口）
MATRIX_BOT_USER_ID=@kb-bot:matrix.singll.net   # 不变（重注册同名）
MATRIX_BOT_TOKEN=<旧token>                      # ★变：重注册后取新 access token
MATRIX_BOT_DEVICE_ID=BELLKEEPER_KEEPER          # ★可能变
MATRIX_ROOM_ALERTS / DAILY / TODO / QA = !xxx   # ★全变：4 房间重建后新 room_id
```
> n8n 工作流引用**逻辑房名**（`room:'alerts'`），**不受影响**，无需改。

---

## 3. 目标架构

- tuwunel **静态裸二进制**（v3，`.zst` 解压）→ `/usr/local/bin/tuwunel`（与现 conduit 同位置），专用用户 `tuwunel`。
- 配置 `/etc/tuwunel/tuwunel.toml`（spool sync 下发），**新空库 `/var/lib/tuwunel`**（与旧 `/var/lib/conduit` 完全隔离，绝不复用旧库 → 规避跨实现损库）。
- **端口复用 6167** → caddy 零改动（tuwunel `port = 6167`）。
- systemd `tuwunel.service`：用**官方加固版 unit**（§4.3，`Type=notify`/`WatchdogSec`/`ManagedOOMPreference=avoid`/沙箱化，比 conduit 裸 unit 更稳更安全），经 spool 下发，`ExecStart` 指向 `/usr/local/bin/tuwunel`。
- spool 纳管：`silkspool.yaml` 增 service/sync_rule/hook。
  - **走方案 C**（推荐，已给 spool 加 `.zst` 支持）：tuwunel 入 `install_sources` + `stack:`，`spool` 一键装/升级。
  - **走方案 B**（不改 spool）：**不入 `stack:`**（无 install_source 会报错），二进制由 `spool exec` 一次性装。

---

## 4. 配置产物

### 4.1 `hosts/txhk/tuwunel/tuwunel.toml`（新建，下发到 `/etc/tuwunel/tuwunel.toml`）

```toml
[global]
server_name = "matrix.singll.net"
database_backend = "rocksdb"
database_path = "/var/lib/tuwunel"      # 全新空库，勿指向旧 conduit_db
port = 6167                              # 复用旧端口 → caddy 不用改
address = ["127.0.0.1"]                  # 仅本地，caddy 反代入口
max_request_size = 20971520             # 20 MiB，沿用现状

# 注册：用户确认保持开放（谁有 token 都能注册的现状不收紧）
allow_registration = true
registration_token = "llDHgate9525."     # 可沿用或借机轮换

# 单用户+bot 私有部署，联邦可留默认开启（4 房间均本地成员，联邦影响为零）
allow_federation = true
trusted_servers = ["matrix.org"]
```
> 以官方 `tuwunel-example.toml`（`[global]` 段，117KB 全注释模板）为准微调；上为最小可用集。

### 4.2 `silkspool.yaml` txhk 段增量（切换后再删 conduit 条目）

```yaml
    sync_rules:
      # ... 保留现有 ...
      - local: "tuwunel/tuwunel.toml"            # 新增：配置 → /etc/tuwunel/
        remote: "/etc/tuwunel/tuwunel.toml"
      - local: "systemd/tuwunel.service"          # 新增：unit 先推 /opt 暂存（避免 chown /etc/systemd 坑）
        remote: "/opt/tuwunel-stage/tuwunel.service"
    services:
      # ... 保留现有 ...
      - alias: "tuwunel"                          # 新增
        type: "systemd"
        name: "tuwunel"
    post_push_hooks:
      # ... 保留现有 ...
      - pattern: "tuwunel/tuwunel.toml"           # 下发配置后修属主并重启
        command: "sudo chown -R tuwunel:tuwunel /etc/tuwunel && sudo systemctl restart tuwunel"
    # 【仅方案 C：已给 spool 加 .zst 支持时才加下面这行】
    stack:
      # ... 保留现有 ...
      - "tuwunel"
```

**【仅方案 C】`install_sources:` 增一条**（github + 精确文件名模板，spool 按 `baseURL/<填充后文件名>` 直取）：
```yaml
  - alias: "tuwunel"
    repo: "matrix-construct/tuwunel"
    pattern: "{VERSION}-release-all-x86_64-v3-linux-gnu-tuwunel.zst"   # 注意：tuwunel 资产用 x86_64 字样，故不用 {ARCH} 占位
    service_name: "tuwunel"
    default_version: "v1.7.1"
```
> ⚠️ **方案 B（不改 spool）不要加 `stack:` 和 `install_sources:` 条目** —— spool 安装器不认 `.zst`，会报错；二进制初装改走 §5.2-B 的 `spool exec`。
> ⚠️ `tuwunel.toml` 下发到 `/etc/tuwunel/` 会让 spool 把该目录属主 chown 给 silkspool（与 conduit 现状同），故 post-push hook 把属主改回 `tuwunel:tuwunel`，保证服务（`User=tuwunel`）能读含 token 的配置。
> ⚠️ unit 文件**先推 `/opt/tuwunel-stage/` 暂存**再由 `spool exec sudo install` 落位到 `/etc/systemd/system/`（沿用 rdp-gateway 同款安全模式，规避 `ensureRemoteDir` 误 chown `/etc/systemd`）。

### 4.3 `hosts/txhk/systemd/tuwunel.service`（官方加固版，仅改 `ExecStart` 路径）

```ini
[Unit]
Description=Tuwunel Matrix homeserver
Wants=network-online.target
After=network-online.target
Documentation=https://tuwunel.chat/

[Service]
User=tuwunel
Group=tuwunel
Type=notify
WatchdogSec=30

Environment="TUWUNEL_CONFIG=/etc/tuwunel/tuwunel.toml"
Environment="MALLOC_CONF=background_thread:false"

ExecStart=/usr/local/bin/tuwunel          # 官方默认 /usr/sbin/tuwunel；本方案放 /usr/local/bin 与 conduit 一致

ReadWritePaths=/var/lib/tuwunel /etc/tuwunel

AmbientCapabilities=
CapabilityBoundingSet=
ManagedOOMPreference=avoid
DevicePolicy=closed
LockPersonality=yes
MemoryDenyWriteExecute=yes
NoNewPrivileges=yes
ProtectClock=yes
ProtectControlGroups=yes
ProtectHome=yes
ProtectHostname=yes
ProtectKernelLogs=yes
ProtectKernelModules=yes
ProtectKernelTunables=yes
ProtectProc=invisible
ProtectSystem=strict
PrivateDevices=yes
PrivateMounts=yes
PrivateTmp=yes
PrivateUsers=yes
PrivateIPC=yes
RemoveIPC=yes
RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX
RestrictNamespaces=yes
RestrictRealtime=yes
RestrictSUIDSGID=yes
SystemCallArchitectures=native
SystemCallFilter=@system-service @resources
SystemCallFilter=~@clock @debug @module @mount @reboot @swap @cpu-emulation @obsolete @timer @chown @setuid @privileged @keyring @ipc
SystemCallErrorNumber=EPERM
RuntimeDirectory=tuwunel
RuntimeDirectoryMode=0750
Restart=on-failure
RestartSec=5
TimeoutStopSec=2m
TimeoutStartSec=2m
StartLimitInterval=1m
StartLimitBurst=5

[Install]
WantedBy=multi-user.target
Alias=matrix-tuwunel.service
```
> 注：`SystemCallFilter` 屏蔽了 `@chown` 等，是**服务运行时**的沙箱；我们的属主修正在 post-push hook 里以 root 执行，不受影响。`MemoryDenyWriteExecute=yes` 对 RocksDB/jemalloc 无碍（无 JIT）。

---

## 5. 实施步骤（按序，全程 spool）

### 5.0 维护窗口前置
- 选低峰窗口（@kb-bot 停摆期间日报/告警/待办中断）。
- 通知：本次将清空 Matrix，所有客户端需重登、E2EE 密钥重置。

### 5.1 留档旧库（可逆保险）
```bash
spool backup txhk                                  # spool 原生备份
# 停服后做文件级一致快照并留档（RocksDB 需停写保证一致）
spool exec txhk "sudo systemctl stop conduit && sudo tar -C /var/lib/conduit -czf /tmp/conduit-db-backup-$(date +%Y%m%d).tgz conduit_db"
# 将快照拉到本地/推 NAS（按 NAS 留档惯例）
```

### 5.2 安装 tuwunel 二进制

**公共前置**（建专用用户 + 数据/配置目录）：
```bash
spool exec txhk "id tuwunel >/dev/null 2>&1 || sudo useradd --system --no-create-home --shell /usr/sbin/nologin tuwunel; sudo mkdir -p /var/lib/tuwunel /etc/tuwunel && sudo chown tuwunel:tuwunel /var/lib/tuwunel /etc/tuwunel"
```

**方案 B —— 不改 spool，`spool exec` 一次性装二进制**：
```bash
spool exec txhk "command -v zstd >/dev/null || sudo apt-get install -y zstd; \
  curl -fSL -o /tmp/tuwunel.zst https://github.com/matrix-construct/tuwunel/releases/download/v1.7.1/v1.7.1-release-all-x86_64-v3-linux-gnu-tuwunel.zst && \
  zstd -d -f /tmp/tuwunel.zst -o /tmp/tuwunel && \
  sudo install -m 0755 /tmp/tuwunel /usr/local/bin/tuwunel && \
  /usr/local/bin/tuwunel --version"
```

**方案 C —— 已给 spool 加 `.zst` 支持（见 §9），走原生 stack 安装**：
```bash
spool exec txhk "command -v zstd >/dev/null || sudo apt-get install -y zstd"   # 目标机需 zstd
# silkspool.yaml 已把 tuwunel 加进 install_sources + server bundle 的 stack（§4.2）
spool bundle server init txhk        # stack driver 幂等安装/补齐 stack 二进制（含 tuwunel）；不会重装未变的
spool exec txhk "/usr/local/bin/tuwunel --version"
```
> 两方案都把二进制落到 `/usr/local/bin/tuwunel`（与 conduit 同位）。
> 若启动报 `Illegal Instruction` → CPU 不支持 v3，把 URL/pattern 换成 `-v2-` 变体重装。
> 方案 C 的 `bundle init` 是**整 bundle 级**幂等操作（server bundle 含 caddy/headscale/ntfy/authelia/tuwunel），只补缺失/版本变更的，不动其它已装项。

### 5.3 下发配置 + systemd unit（spool 原生）
```bash
# 本地写好 hosts/txhk/tuwunel/tuwunel.toml（§4.1）和 hosts/txhk/systemd/tuwunel.service（§4.3）
spool sync push txhk
#   → tuwunel.toml 落 /etc/tuwunel/（post-push hook 自动 chown+restart；首次 restart 因 unit 尚未装好会无害失败，5.4 显式启动）
#   → tuwunel.service 落 /opt/tuwunel-stage/ 暂存
# 把 unit 落位到 systemd 并 reload（暂存→/etc/systemd，规避 ensureRemoteDir 误 chown /etc/systemd）
spool exec txhk "sudo install -m 0644 /opt/tuwunel-stage/tuwunel.service /etc/systemd/system/tuwunel.service && sudo systemctl daemon-reload"
```

### 5.4 纳管 + 切换
```bash
# 编辑 silkspool.yaml 加入 §4.2 的 service/sync_rule/hook（先不删 conduit）
spool exec txhk "sudo systemctl disable --now conduit"     # 停旧（端口 6167 让出）
spool exec txhk "sudo systemctl enable --now tuwunel"      # 启新
spool service txhk status tuwunel                          # 确认 active
spool logs txhk tuwunel 80                                 # 看启动日志
```

### 5.5 重建账号与房间
```bash
# @singll 首注册自动成 admin；再注册 @kb-bot
# 注册走客户端或 register API（带 registration_token）；admin room 由首个 admin 自动建
# 登录 @kb-bot 取新 access token + device_id（Element/curl /login）
# @kb-bot 创建 4 房间 alerts/daily/todo/qa（建议同时设稳定 alias #alerts:matrix.singll.net）
# 记下 4 个新 room_id
```

### 5.6 改 Bellkeeper（keeper）配置并重新部署
```bash
# 编辑 keeper .env：MATRIX_BOT_TOKEN / MATRIX_BOT_DEVICE_ID / MATRIX_ROOM_{ALERTS,DAILY,TODO,QA}
spool sync push keeper && spool restart keeper bellkeeper
```

### 5.7 验收
```bash
spool service txhk status tuwunel                          # active (running)
spool logs keeper bellkeeper 100                           # @kb-bot 已连上 matrix
# 在 4 房间各发一条命令，确认 @kb-bot 响应
# 触发一次日报/告警链路（端到端）
# 验证核心诉求：admin room 执行 !admin rooms delete <测试房> → 确认物理删除生效
```

### 5.8 收尾（确认稳定运行 1-2 天后）
```bash
# 从 silkspool.yaml 删除 conduit 的 service/sync_rule/stack 条目
spool exec txhk "sudo rm -f /etc/systemd/system/conduit.service && sudo systemctl daemon-reload"
# 保留 /var/lib/conduit 旧库留档一段时间再删
# 订正 Caddyfile 注释「Matrix (Dendrite)」→「Matrix (tuwunel)」，spool sync push txhk
```

---

## 6. 回滚预案

切换后若 tuwunel 不可用且短期修不好：
```bash
spool exec txhk "sudo systemctl disable --now tuwunel && sudo systemctl enable --now conduit"
spool service txhk status conduit
```
- 端口同为 6167，caddy 无需动。
- 旧库 `/var/lib/conduit/conduit_db` 未删（§5.8 才删），conduit 直接恢复原状。
- 代价：tuwunel 上已建的账号/房间作废（fresh start 本就无数据，损失仅为重建工作量）。

---

## 7. spool 编排可行性详评（源码证据）

**结论：spool 原生支持 systemd 服务编排，非 Docker-only。** tuwunel 的生命周期/配置管理可像 conduit 一样完全被 spool 纳管；唯「二进制初装」因 spool 安装器不认 `.zst`，需补 ~15 行（方案 C）或 `spool exec` 兜底（方案 B）。

| 能力 | 支持 | 源码证据 |
|---|---|---|
| bundle 驱动含 `stack`（二进制+systemd） | ✅ | `internal/engine/bundle_driver.go`（compose/stack 双驱动）；`bundles/server/manifest.yaml` type:stack |
| service 走 systemctl | ✅ | `internal/engine/service.go:223-288`：systemd 分支 `systemctl is-active/start/stop/restart`、`journalctl -u` |
| host 声明 systemd 服务 | ✅ | `internal/config/types.go` ServiceEntry{Type:"systemd"}；`silkspool.yaml:212-214` conduit 实例 |
| 配置同步 + 推送后钩子 | ✅ | `internal/engine/sync.go`；`silkspool.yaml:227-231` caddy reload hook |
| 二进制自动安装认 `.zst` | ❌（方案 C 补丁可解） | `internal/engine/install.go:189` 仅 `.tar.gz/.tgz` 解包，其余当**裸二进制** cp+chmod；`.zst` 拷过去 chmod 不可执行（§9 补 15 行 `.zst` 分支即可原生） |
| 自动生成的 unit 够用 | ❌（太简陋） | `install.go:216-233` 自动 unit 无 `User=`/`Environment=` → 自备完整 unit（§4.3）经 sync 覆盖 |
| `{ARCH}` 映射 | x86_64→amd64 | `install.go:128-129`；tuwunel 资产用 `x86_64` 字样，故 pattern 不用 `{ARCH}` 占位 |

**判断**：
- 「管理」层（运维高频操作）**完全原生**，体验与 Docker bundle 对等，与 txhk 现有 conduit/caddy/headscale 一致。
- 「初装」层因 tuwunel 只发 `.zst`（裸二进制压缩）而非裸二进制/`.tar.gz`：**方案 C** 给 spool 加 `.zst` 支持后即可纳入 `install_sources` 原生安装（推荐，与 conduit 同模式）；**方案 B** 用 `spool exec` 解压落位，零 spool 改动。

---

## 8. 风险与注意

1. **无 swap 内存盯紧**：2核/3.6G/无swap，OOM 不留情。tuwunel(Rust/RocksDB) 与 conduit 同量级（预计 RSS 200-400M），但首次同步/大房间可能瞬时升高。unit 自带 `ManagedOOMPreference=avoid` 有缓解。切换后用 `spool exec txhk "systemctl status tuwunel; free -m"` 观察。
2. **v3 兼容**：若 `Illegal Instruction` 退 v2 包（§5.2）。RocksDB 在 v2+ 有硬件 CRC32 加速，v2 起步即可。
3. **`Type=notify`**：官方 unit 用 notify（tuwunel 支持 sd_notify）；若异常未就绪可临时改 `Type=simple` 排查。
4. **ensureRemoteDir 碰 /etc 的坑**（`silkspool.yaml:189` 记载）：tuwunel.toml 同步到 `/etc/tuwunel/`（独立子目录，同 conduit 现状，安全）；勿把 sync remote 指向 `/etc` 根或 `/etc/sudoers.d`。
5. **联邦签名密钥**：fresh start 生成新服务器签名密钥。4 房间均本地成员，联邦影响为零；若将来与外部联邦，对端会按 Matrix 密钥轮换处理。
6. **客户端全部重登 + E2EE 重置**：单用户+bot 场景代价小，但需事先知晓。
7. **token 开放注册风险延续**：用户选择保持，注意 `registration_token` 勿外泄；可借切换轮换 token。

---

## 9. 方案 C：给 spool 加 `.zst` 支持（推荐——实现 spool 原生二进制安装/升级）

让 tuwunel 像 conduit 一样进 `install_sources`+`stack`，由 `spool` 负责下载+解压+落位+版本钉死，是最统一、最好管的长期形态。改动很小（约 15 行），集中在 `internal/engine/install.go` 的 `buildInstallScript`：

```go
// install.go:189 附近，与现有 isTarball 并列
isTarball := strings.HasSuffix(downloadURL, ".tar.gz") || strings.HasSuffix(downloadURL, ".tgz")
isZst := strings.HasSuffix(downloadURL, ".zst") && !strings.HasSuffix(downloadURL, ".tar.zst") // 单文件 zstd（排除 .tar.zst 容器包）

// 下载段后，分支里新增：
} else if isZst {
    script += fmt.Sprintf(`
command -v zstd >/dev/null || sudo apt-get install -y zstd
zstd -d -f %s -o %s.bin
sudo install -m 0755 %s.bin /usr/local/bin/%s
`, app, app, app, app)
}
```
- 目标主机需 `zstd`（脚本里已兜底 `apt-get install`）。
- `{ARCH}` 占位仍映射 `x86_64→amd64`（install.go:128），而 tuwunel 资产用 `x86_64` 字样 → §4.2 的 pattern 直接写死 `x86_64-v3` 不用占位。
- spool 自动生成的最小 unit（install.go:216，无 `User=`/`Environment=`）对 tuwunel 不够用 → 仍由 §4.3 的完整 unit 经 sync 落位覆盖（§5.3）。

**改完按全局规则只覆盖二进制**（保留运行时 hosts/keys/yaml/backups）：
```bash
make -C /home/ubuntu/SilkSpool build
cp /home/ubuntu/SilkSpool/out/spool /opt/SilkSpool/spool && chmod 755 /opt/SilkSpool/spool
```

**B vs C 取舍**：
- **C（推荐）**：一次性 ~15 行补丁，换来 tuwunel 安装/升级全走 `spool`（版本钉死、与 conduit 同模式）。日后升级 = 改 `default_version` + `spool bundle server init txhk`。
- **B**：零 spool 改动，二进制初装/升级走 `spool exec` 脚本（§5.2-B）。生命周期/配置照样 spool 原生。适合「暂不想动 spool 源码」。
> 两者最终运维体验一致（service/restart/logs/sync 全原生），差别只在「二进制装/升级」是 `spool` 原生命令还是 `spool exec` 脚本。
