# txhk Matrix Homeserver 替换评估（进行中）

> 起因：Conduit 0.10「只能禁用不能删」太憋屈，评估换一个**能真正删 + 不吃性能**的实现。
> 本文档记录**已联网/实地确认**的事实与评估框架；标 ⚠️ 的为待联网核实项（评估时 WebSearch/WebFetch 被 429 限流、本机无 gh）。
> 日期：2026-06-19（初评）。**2026-07-02 三命门复核翻案：命门②（迁移）由「硬阻断/损库」订正为「官方支持 in-place 保数据迁移」——详见 §5/§7 方案 B/§8。**

## 1. txhk 现状（spool 实地确认）

| 项 | 事实 |
|---|---|
| Homeserver | **Conduit 0.10.11** 官方分支（conduit.rs），`/usr/local/bin/conduit`，**systemd 原生服务** `conduit.service` |
| 存储后端 | **RocksDB**，`/var/lib/conduit/conduit_db`，当前 **414M** |
| server_name | `matrix.singll.net`，监听 `localhost:6167`（caddy 反代，端口入口） |
| 注册 | `allow_registration = true` + 固定 `registration_token`（**风险：谁有 token 都能注册**） |
| 配置 | `/etc/conduit/conduit.toml`（spool 侧副本 `hosts/txhk/conduit/conduit.toml`） |

## 2. 硬件约束（决定「吃不吃性能」）

- **2 核** / 内存 **3.6G 总、可用 ~2.8G** / **无 Swap**（内存打满直接 OOM）/ x86_64（腾讯云香港 VPS）
- Conduit 当前 RSS **155MB**（极轻）
- 同机常驻：authelia(66M)、headscale(48M)、caddy(45M)、tailscaled(45M)、腾讯云镜 YDService(88M)、ntfy、rdp-gateway、journald(82M)
- **推论：Synapse 基本出局**——单进程基线 500MB–1GB+ 且强制 Postgres，在 2 核 3.6G 无 swap 上与现有服务抢内存，极易 OOM。

## 3. 账号现状（待 admin room 精确确认）

strings 粗扫 RocksDB（**不可靠，活跃账号可能漏**）得：

| 账号 | 性质 | 处置倾向 |
|---|---|---|
| `@kb-bot:matrix.singll.net` | **Bellkeeper 平台 bot（命脉）** | 绝不动 |
| `@singll:matrix.singll.net` | 本人 / 首注册 admin | 保留 |
| `@conduit:matrix.singll.net` | Conduit 系统账号 | 保留 |
| `@aaa` / `@test` | 疑似测试账号 | 无用候选 |

> ⚠️ 全集必须用 admin room `list-local-users` / `list-rooms` 拿（需 @singll 凭证或 @kb-bot admin 权）。

## 4. 平台在用房间（keeper 的 Postgres `matrix_rooms` 确认，is_active=t）

| 频道 | room_id | 类型 |
|---|---|---|
| alerts | `!G3LKKFRkdW12v6BPU-kUIrarL9ZYVFoUhNKuTvtiaO0` | command |
| daily | `!OXnN7pnE5w7fi76jPiszgrUS8xwmUrFPTV_pT7RnhSc` | command |
| todo | `!auqtgwzTasSebguRECeVIb4oXkVB7_46iosZU2gukCk` | command |
| qa | `!E0EaIIy_69P1mQSa06TUUwpB-AzhpeLWy6T2iUmln54` | direct |

> Conduit 端实际房间数未知（含测试/废弃遗留），是「无用房间」来源；需 `list-rooms` 取全集。

## 5. 候选对比框架

| 候选 | 语言/后端 | 资源 | 删房间/用户 | 清历史(retention) | 从 Conduit 0.10 迁移 |
|---|---|---|---|---|---|
| **Conduit 0.10**（现状） | Rust / RocksDB | 极轻 155M | 仅 disable，不回收 | ❌ 无 | — |
| **tuwunel** | Rust / RocksDB | 与 Conduit 同量级（轻） | ✅ **`!admin rooms delete` 物理删库**（已核实） | 🔶 无 m.room.retention 自动过期(MSC1763 ❌)，但有手动删房/删媒体/redaction_retention | ✅ **官方支持 in-place 迁移**（2026-07-02 复核翻案，见下） |
| **continuwuity** | Rust / RocksDB | 轻 | ✅ 同 tuwunel（同源 conduwuit fork） | 🔶 同上 | 🔶 未复核（tuwunel 已支持 Conduit 迁入，此列不再是选型关键） |
| **Dendrite** | Go / SQLite·Postgres | 中（~100-300M） | 中等 admin API | ⚠️ 部分支持 | ❌ 无直接迁移，数据格式不同 |
| **Synapse** | Python / Postgres | **重（出局）** | ✅ 完整（删用户/房间/purge/retention/媒体） | ✅ 完整 | ❌ 无官方 Conduit→Synapse 迁移 |

**已确认（一手文档/API）**：
- tuwunel = conduwuit（girlbossceo 原项目，仓库已归档）的官方继承者，Rust + RocksDB，与 conduwuit 二进制平替。文档站 https://matrix-construct.github.io/tuwunel/ 。
- **命门②（迁移）= 翻案：现已官方支持 Conduit→tuwunel in-place 迁移**（2026-07-02 复核，一手 raw 源 `docs/deploying.md`）：
  > 「A RocksDB database **from Conduit** or a fork of conduwuit migrates in place on first boot: stop the source server, back up its data directory and media, then start Tuwunel against it. Tuwunel reconciles the schema version, room history, account data, and media automatically; no flags are required.」
  - 且**点名 Conduit 0.10 的媒体迁移细节**：0.10.0 flat 存储 → `conduit_media_directory_depth = 0`；0.10.1+ 分片（默认）。→ 正对应 txhk 的 **0.10.11**。
  - issue #41『Feasible conduit db > 13 migration path』**已 closed**（此前解读为"证实无路"，实为该迁移已实现而关闭）。
  - ⚠️ **纠错**：2026-06-19 旧结论「命门②=硬阻断、官方明示 Conduit ❌ Not right now、强换损库」**已作废**——该判断基于当时 429 限流下的二手/过期缓存（WebSearch 至今仍返回旧缓存"❌ Not right now"，勿采信）。「Never switch between forks corrupt」的警告**只针对 conduwuit 系 fork 之间横跳**（tuwunel↔continuwuity），**不适用 Conduit→tuwunel**（后者是官方支持的 in-place 迁移路径）。
  - **推论**：fresh start **不再是唯一路径**——现可选「保数据 in-place 迁移」（保留 4 房间/历史/@kb-bot 账号与 E2EE，room_id 与 token 不变，keeper `.env` 或可不改）。是否仍走 fresh start 取决于用户是否想借机弃旧库清白重来（用户 2026-06-19 曾表态"无迁移需求、可删全部历史"，但那是在"迁移=损库"的错误前提下做的取舍，**建议复述此翻案让用户重新定夺**）。
- **命门①（删/清）= tuwunel 完胜 Conduit 0.10**（2026-07-02 一手 raw 源 `docs/moderation.md` + `docs/media/management.md` 逐条核实）：
  - `!admin rooms delete <room>`：「harder than ban; **removes the room from the database** after evicting users」← 正是 Conduit 0.10 缺的真删
  - `!admin rooms moderation ban-room` / `ban-list-of-rooms`（批量代码块）/ `unban-room` / `list-banned-rooms`
  - `!admin users deactivate <user>`（默认同时退出所有房间）/ `deactivate-all`（批量）/ `reject-invites` / `redact-event` / `force-demote`
  - 媒体：`!admin media delete --mxc`、`delete-by-event --event-id`、`delete-list`（代码块批量）、`delete-range <duration> --older-than/--newer-than`（整套确认存在）
  - 🔶 **不支持** per-room 自动 retention（MSC1763 ❌）——诉求是「能删」而非「自动过期」，手动 admin 删房已满足。
  - 🔶 **未复核**：旧稿提及的 `delete_rooms_after_leave`（实验性自动删房）与 `delete-all-from-user/server` 媒体命令，本轮 raw 源未见对应条目，**存疑，勿作承诺**（可能已改名或移除）。
- **命门③（重连）在 fresh-start 下 = 配置工作**（keeper `.env` 实地确认）：Bellkeeper 靠 **room_id** 找房（`MATRIX_ROOM_*=!...`），fresh start 后 token+4 room_id 全变需更新；但 n8n 工作流引用**逻辑房名**（`room:'alerts'`），不受影响。tuwunel 默认端口 **8008**（Conduit 6167）→ caddy 上游改一行。部署同为静态二进制+systemd（非容器），干净替换。

## 6. 定稿结论（2026-06-19，用户确认「无迁移需求、可删全部历史、重新开始」后）

**推荐：tuwunel 全新部署（fresh start），server_name 保持 `matrix.singll.net`，弃旧库。**

理由链：
1. 用户核心诉求是「能真删房间、功能好用」，tuwunel 的 `!admin rooms delete` 物理删库 + 整套用户/媒体清理命令**直接满足**，相对 Conduit 0.10 是确定升级。
2. ~~用户明确放弃历史数据 → 命门②（迁移损库）这个唯一硬阻断自动消解~~ ⚠️ **2026-07-02 翻案作废本条**：命门②已不成立（官方支持 Conduit→tuwunel in-place 迁移，见 §5）。fresh start 仍是**可选**方案（若用户想清白重来），但**不再是被迫的唯一路径**——现可保数据平滑迁入，保留 4 房间/历史/账号/room_id/token。**选型结论（tuwunel）不受影响**，但**迁移方式需用户重新定夺**（fresh start vs in-place）。
3. 硬件轻约束下 tuwunel 与 Conduit 同量级（Rust/RocksDB），可在 2 核/3.6G/无 swap 上运行（Synapse 仍出局）。
4. tuwunel vs continuwuity 决选（2026-06-19 实时 API 数据）→ **选 tuwunel**：

| 维度（用户诉求） | tuwunel | continuwuity | Dendrite |
|---|---|---|---|
| 开源/许可 | ✅ Apache-2.0 | ✅ (conduwuit 系) | ✅ Apache-2.0 |
| 资源(性能) | ✅ Rust/RocksDB，与 Conduit 同量级 | ✅ 同 | 🔶 Go，中等 |
| **维护活跃(2026-06-19)** | ✅ **今日仍推代码**，2210★，200+ 贡献者 | ✅ 今日活跃，~868★ | ❌ **archived，2024-11 起停更** |
| **成熟度/采用度**（越高越好，非硬门槛） | ✅ **v1.7.1**，月度稳定节奏+降级守卫，采用最广 | 🔶 v0.5.9（沿用 conduwuit 0.x 编号，刚切 CalVer），采用较少 | ❌ 死 |
| 真删/管理 | ✅ `!admin rooms delete` 等整套 | ✅ 同源同能力 | 🔶 中等 admin API |
| 扩展性/不换栈 | ✅ 在做 MAS 鉴权/MSC4190/MSC4284/sliding sync | ✅ 跟进但节奏慢 | ❌ 死 |
| 贡献者构成(抗 bus-factor) | ✅ 含原 Conduit 作者 timokoesters + conduwuit 核心 + construct 作者 jevolk 领衔 | 🔶 社区集体治理 | — |

> 注：用户「1.0 后要稳」指的是 **Bellkeeper 系统自身的 1.0**，非 Matrix 服务端版本号；成熟度越高越好但**不卡 Matrix 端的 1.0**。故 continuwuity 的 v0.5.x 编号**不构成出局理由**（它与 tuwunel 同出 conduwuit 0.x 代码基，版本号差异部分是命名选择）。

→ **Dendrite 已 archived 直接出局**；**tuwunel 与 continuwuity 同源同能力、都活跃、都可选**，决选看成熟度与抗风险：tuwunel **采用最广（~2.5× stars）、含原 Conduit/conduwuit/construct 三代作者、月度发布节奏、技术积累最厚 → 决选 tuwunel**（「越成熟越好」恰好指向它）。continuwuity 是合理的同源备选（若更看重纯社区治理而非单一领衔者）。

🔶 **唯一功能缺口**：无 per-room 自动 retention（MSC1763 未实现）。对本场景无影响——诉求是「手动能删」，已满足。

⚠️ **关于「账号数据迁移」(用户第10点) 的硬事实**：~~Matrix 生态没有任何 homeserver 提供跨实现的干净账号/数据迁移——Conduit→任何 fork 损库~~ ⚠️ **2026-07-02 部分翻案**：Conduit→tuwunel 现为官方支持的 in-place 迁移（见 §5），**并非损库**。仍成立的部分：Synapse↔其它无标准通道（tuwunel Synapse 导入 #2 仍 open）；同源的 tuwunel↔continuwuity 互换仍会损库（跨 fork），故选定后不轻易横跳。**结论：「不被锁死」的真正保险是选最不会死的项目** → 各项实时指标均指向 tuwunel（200+ 贡献者含三代原作者、月度发布、已过 1.0）。

⚠️ **代价（仅 fresh start 方案）**：所有客户端重新登录、E2EE 密钥重置、@kb-bot 重注册、4 房间重建。因均为本地单用户+bot 场景，代价可控。**若改走 in-place 迁移方案（§7-bis），这些代价基本消失**（账号/房间/token/E2EE 均保留）。

## 7. 两条迁移路径（择一，均待用户批准后执行，**需维护窗口**）

> @kb-bot 驱动日报/告警/待办，停服期间这些功能中断。两方案均建议选低峰窗口，且**先备份旧 Conduit 库到 NAS 留档**（原需求二：异地容灾）。
>
> **⚠️ 2026-07-02 复核后新增「方案 B（in-place 保数据迁移）」** —— 原命门②翻案后现已可行，代价远低于 fresh start，通常应优先。方案 A（fresh start）仅在用户想主动弃旧库清白重来时选用。

### 方案 A — Fresh start（弃旧库，空库重建）— **不可逆**

1. **留档**：`spool backup txhk` + 停 conduit 后对 `/var/lib/conduit/conduit_db` 文件级快照推 NAS（停服保证 RocksDB 一致性）。
2. **部署 tuwunel**：下载 x86_64 静态二进制 → `/usr/local/bin/tuwunel`；写 `tuwunel.toml`（server_name=matrix.singll.net、port=8008、allow_registration + registration_token、RocksDB 路径用新目录如 `/var/lib/tuwunel`）；写 systemd unit；停 `conduit.service`，启 `tuwunel.service`（空库）。
3. **caddy**：反代上游 `localhost:6167` → `localhost:8008`，reload。
4. **重建账号**：注册 `@singll`（首注册自动 admin）、`@kb-bot`；登录 @kb-bot 取新 access token 与 device_id。
5. **重建 4 房间**：@kb-bot 创建 alerts/daily/todo/qa（建议同时设稳定 alias 如 `#alerts:matrix.singll.net` 便于将来）；记 4 个新 room_id。
6. **改 keeper 配置**：`.env` 更新 `MATRIX_BOT_TOKEN`、`MATRIX_BOT_DEVICE_ID`、`MATRIX_ROOM_ALERTS/DAILY/TODO/QA`（4 新 room_id）→ `spool sync push keeper && spool restart keeper bellkeeper`。**n8n 工作流无需改**（引用逻辑房名）。
7. **验收**：@kb-bot 在 4 房间响应命令；触发一次日报/告警链路；`!admin rooms delete` 删一个测试房验证真删。

### 方案 B — In-place 保数据迁移（推荐，代价低）— **2026-07-02 新增**

> 依据 §5 命门②翻案：tuwunel 官方支持读旧 Conduit RocksDB 库 in-place 迁移。**保留全部房间/历史/账号/room_id/access token/E2EE**，keeper `.env` 通常**无需改** room_id/token。仍需停服窗口 + 先备份（迁移不可逆，且旧库被 tuwunel 就地升级 schema 后无法回 Conduit）。
>
> ⚠️ **执行前必做**：先对旧库做**只读文件级快照留档**再迁移（tuwunel 会就地改写 schema version，一旦迁移旧 Conduit 无法再读同一目录）。建议迁移前先对快照副本做一次 tuwunel 试启动验证成功，再对生产库操作，或直接迁移副本、验证无误后切换。

1. **留档（强制）**：`spool backup txhk` + 停 `conduit.service` 后对 `/var/lib/conduit/conduit_db` 文件级快照推 NAS（停服保证 RocksDB 一致性）。**此快照是唯一回退保险**。
2. **部署 tuwunel 二进制**：下载 x86_64 静态二进制（按 §5「Pick the right binary」选 `-v2-`/`-v3-` 变体，跑 CPU 检测命令确认，选错会 `Illegal Instruction`）→ `/usr/local/bin/tuwunel`；写 systemd unit（先 `disabled`，不自启）。
3. **写 `tuwunel.toml`（指向旧库）**：`server_name=matrix.singll.net`、监听端口（tuwunel 默认 8008，可显式设 6167 以免动 caddy）、`allow_registration=true` + 原 `registration_token`、**`database_path` 指向旧 Conduit 库目录（或其副本）**；按 §5 设 `conduit_media_directory_depth`——txhk 是 **0.10.11（≥0.10.1）用分片默认**，若旧库媒体在非默认路径再设 `conduit_source_media_path`。
4. **首启迁移**：停 `conduit.service`，启 `tuwunel.service`。tuwunel 首启识别 foreign database、自动 reconcile schema/room history/account data/media（无需 flag）。**盯日志确认迁移成功**（`spool logs`/`journalctl`），失败则回滚到步骤 1 快照。
5. **caddy**：若步骤 3 已让 tuwunel 监听 6167，则 caddy 上游**无需改**；若用默认 8008，则上游 `localhost:6167`→`localhost:8008` 后 reload。
6. **keeper 配置**：room_id/token/device_id **均不变** → **`.env` 通常无需改**。仅当迁移后 @kb-bot 旧 access token 失效（观察验收阶段）才需重登取新 token。**n8n 工作流无需改**。
7. **验收**：@kb-bot 在原 4 房间响应命令（历史消息应仍在）；触发一次日报/告警链路；`!admin rooms delete` 删一个**测试**房验证真删（勿删业务房）。

## 8. 用户决定（2026-06-19 确认；2026-07-02 复核后需补一项）

- **执行时机**：⏸️ **暂不执行，只要结论**。本方案存档备查；将来用户让动手时按 §7 执行。
- **迁移方式**：✅ **2026-07-02 用户复核翻案后仍确认走方案 A（fresh start，弃旧库清白重来）**。即：已知 in-place 保数据（方案 B）可行且代价更低，用户仍**主动选择弃旧库全新部署**（可清空历史/测试房/废账号，干净开局）。→ 按 §7 **方案 A** 七步执行；方案 B 存档备查不用。选型 tuwunel 不变。
- **注册策略**：✅ **保持 `allow_registration=true` + 固定 registration_token 开放注册**（用户明确不借机收紧，两方案均一致）。
- 旧库留档：两方案均先 `spool backup txhk` + 停服文件级快照推 NAS（库仅 ~431M，成本极低）；方案 B 中此快照更是唯一回退保险。

## 9. 环境注意

- 本机 **无 gh**（已复核 2026-07-02）；本机 **curl 可联网**（github.io / raw.githubusercontent 均 200）。**2026-07-02 复核时 WebSearch/WebFetch 已可用**（评估当日 2026-06-19 被 429 限流，故当时靠 curl 二手核实 → 是命门②误判的根因；本轮已用一手 raw 源订正）。
- ⚠️ **WebSearch 缓存滞后**：2026-07-02 WebSearch 仍返回旧「Conduit ❌ Not right now」，与现行官方 `docs/deploying.md` 矛盾；**以 raw.githubusercontent 一手源为准**。
- 运维一律走 `spool`（txhk / keeper）。txhk Conduit 是 systemd 服务（非容器），重启走 `systemctl`/spool service。
