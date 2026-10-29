# Bellkeeper 部署与配置（现行）

> 最后更新：2026-10-29 · 适用 SilkSpool `spool` 1.2+
>
> 当前生产：`prod`（192.168.7.128），应用栈 `prod-stack`，数据依赖 `proddata`，Matrix 在 `txhk` 的 tuwunel。
> `knowledge-stack` / `silkdata` 是通用模板名，旧 keeper(.230)/silkdata(.231) 已不属于当前生产目标。所有远程操作走 PATH 中的 `spool`。

---

## 1. 交付形态

```text
Bellkeeper/
└── deploy/
    ├── bundle.yaml       # product 契约：镜像/digest、outputs、health、backup/restore
    ├── compose.yaml      # 单服务 bellkeeper
    └── scripts/
        ├── backup.sh
        └── restore.sh

SilkSpool/
├── bundles/knowledge-stack/   # composition：bellkeeper + n8n + memos + rsshub
└── bundles/silkdata/          # capability：Postgres/Redis/Meili/NATS/CouchDB
```

职责边界：

- **Bellkeeper 仓库**：产品镜像、`deploy/bundle.yaml`、产品备份/恢复 hook。
- **SilkSpool**：解析 composition/wiring、生成 compose override、执行 plan/apply/rollback。
- **silkdata**：数据层独立部署，`knowledge-stack` 只读 outputs，绝不 apply/升级/回滚它。

> **认证模式（排障）**：Caddy 未启用 Forward Auth / Authelia 时，composition wiring 必须显式
> `BELLKEEPER_SERVER_MODE: noauth`。若沿用 `release`，`AutheliaAuth` 会要求 `Remote-User` 头，
> 前端所有 `/api/*` 请求直接 401（表现为 LLM 管理页渠道/模型组/用量/余额/告警全部加载失败）。
> 修改 wiring 后 `spool plan <stack> prod --offline` → `spool apply <stack> prod --yes`。

---

## 2. 通用模板配置示例（非本机生产配置）

### 2.1 `silkspool.yaml`

```yaml
global:
  ssh_key_path: "./keys/id_silkspool"
  default_domain: "example.com"
  defaults:
    deploy_path: "/opt/silkspool"

hosts:
  keeper:
    address: "silkspool@192.168.7.230"
    magicdns: "keeper"
    app_prefix: "sp-"
    bundles: ["knowledge-stack"]
    inputs:
      public_url: "https://bellkeeper.example.com"

  silkdata:
    address: "silkspool@192.168.7.231"
    magicdns: "silkdata"
    app_prefix: "sp-"
    bundles: ["silkdata"]

bundle_sources:
  - name: bellkeeper
    type: git
    repo: https://github.com/singll/Bellkeeper.git
    path: deploy
    commit: "<40-hex-pinned-commit>"
  - name: local
    type: dir
    path: ./bundles
```

可选 OCI 发布方式（推荐正式发布后使用）：

```yaml
bundle_sources:
  - name: bellkeeper
    type: oci
    ref: ghcr.io/singll/bellkeeper-bundle
    digest: "sha256:<64-hex-manifest-digest>"
    path: deploy
    cosign:
      key: "./cosign.pub"   # 或 identity + issuer
```

> OCI source 必须固定 digest；声明 `cosign` 后验签失败会直接报错，不会降级。

### 2.2 `hosts/keeper/.env` secrets

`knowledge-stack` 的 wiring 从以下键解析（大小写不敏感）：

```dotenv
# --- Bellkeeper ---
BELLKEEPER_DB_PASSWORD=<random-password>
BELLKEEPER_API_KEY=<random-api-key>
BELLKEEPER_CREDENTIAL_KEY=<32-byte-hex>

# --- n8n ---
N8N_ENCRYPTION_KEY=<random-string>
N8N_API_KEY=<n8n-api-key>
N8N_OWNER_EMAIL=owner@example.com
N8N_OWNER_FIRSTNAME=Owner
N8N_PASSWORD=<random-password>

# --- silkdata wiring ---
REDIS_PASSWORD=<random-password>
MEILI_MASTER_KEY=<random-master-key>

# --- Memos ---
MEMOS_API_TOKEN=<random-token>
```

> LLM 渠道/模型组通过数据库和热重载管理。**Matrix 登录凭证、Agent 开关来自启动 YAML / 环境变量，修改后需重启 Bellkeeper**；`/api/config/reload` 目前只重载 LLM。Matrix 房间/频道写数据库，频道变更后调用 `/api/matrix/notify/channels/reload`。

> 不要把真实值写入 `silkspool.yaml` 或 `deploy/bundle.yaml`；plan/lock 只保存 `secret://...` 占位。

### 2.3 `deploy/bundle.yaml`（产品契约）

```yaml
apiVersion: spool/v1alpha1
kind: Bundle
metadata:
  name: bellkeeper
  class: product
  version: 2.4.0
  owner: singll/Bellkeeper
name: bellkeeper
type: compose
templates: [compose.yaml]
spec:
  delivery:
    driver: compose
    templates: [compose.yaml]
    artifact:
      type: image
      image: ghcr.io/singll/bellkeeper:2.4.0
      digest: "sha256:<real-manifest-digest>"
  outputs:
    url: "${input.public_url}"
    health: "http://127.0.0.1:8080/api/health"
    internal:
      kind: endpoint
      service: bellkeeper
      port: 8080
      remote: "${host.magicdns}"
  lifecycle:
    health: {http: "${outputs.health}", from: container, timeout: 120s}
    upgrade: {strategy: backup-then-recreate, irreversible: false}
    restore: {hook: scripts/restore.sh}
  backup:
    hook: scripts/backup.sh
    assets:
      - {type: volume, source: kp-bellkeeper-data}
```

> 独立 `spool apply bellkeeper keeper` 时必须在 `hosts.keeper.inputs.public_url` 提供 URL；
> 通过 `knowledge-stack` 部署时由 composition 的 component outputs 覆盖，不依赖该输入。

---

## 3. 当前生产运维

```bash
spool plan prod-stack prod --offline
spool plan prod-stack prod
spool restart prod bellkeeper
spool service prod status bellkeeper
spool logs prod bellkeeper 200
spool service txhk status tuwunel
spool n8n list
spool n8n update --dry-run
spool n8n update --yes
```

生产运行目录是 `/opt/SilkSpool/`；`hosts/prod/.env` 存放 secret。
`prod-stack` 只管理 Bellkeeper/n8n；`proddata` 是只读 dependency；RSSHub 是独立 `rsshub` bundle。
需要部署时先审阅 plan，再按现有容器重建授权执行 `spool apply prod-stack prod --locked --yes`。
本次 Matrix 接入使用配置持久化和单容器重启，没有重建 Bellkeeper/n8n。

- Bellkeeper 启动配置：`hosts/prod/bellkeeper/bellkeeper.yaml` → TrueNAS `/mnt/NAS/data/prod/store/bellkeeper/bellkeeper.yaml` → 容器 `/app/data/bellkeeper.yaml`。
- 当前容器 `/app/config/bellkeeper.yaml` 指向持久配置；运行时 bundle 已声明 `--config /app/data/bellkeeper.yaml serve`，后续部署继续使用同一文件。
- 当前运行二进制为 `2.4.3-matrix.20261029`（基础镜像仍为 `bellkeeper:2.4.3`），修复房间发现覆盖配置、停用命令、重复 webhook 路径和重复入队。`/app/bellkeeper` 指向持久卷中的 `bellkeeper-matrix`，bundle 的 entrypoint 同样指向该文件；仅重启现有容器，没有重建。已保留 `bellkeeper-before-matrix` 供回滚，artifact 和校验值在私有备份目录。下次镜像发布含这些修复后，明确移除该 entrypoint 覆盖，避免一直运行旧补丁。
- 修复后的正文提取脚本保存在同一持久卷，运行时 bundle 显式挂载该脚本；源码在 `scripts/trafilatura_extract.py`。
- 独立 RSSHub 使用 GHCR 固定 digest，经 `spool apply rsshub prod --yes` 管理；prod-net 内地址 `http://rsshub:1200`，内网站点 `https://rsshub.singll.net`。Hugging Face Papers、Anthropic News、安全客知识、36氪快讯路由已逐条验证，勿恢复 `.230` 后端。
- n8n 启动脚本读取绑定的 `/home/node/config/.env`；环境变量变化仅重启 `sp-n8n`，保留 `prod-n8n` 卷。
- Matrix secret：`MATRIX_HOMESERVER_URL`、`MATRIX_BOT_USER_ID`、`MATRIX_BOT_ACCESS_TOKEN`、`MATRIX_BOT_DEVICE_ID`、`MATRIX_ADMIN_USER`、`MATRIX_ROOM_ALERTS`。不写入工作流 JSON 或 Git。
- 正常通知经 Bellkeeper → NATS → Matrix；B02 错误告警直接使用 Matrix API，保证 Bellkeeper 失败时仍有告警出口。
- 工作流错误目标 ID 属于当前 n8n 实例，迁移时先导入 B02，再把其它工作流的 `settings.errorWorkflow` 解析到新 ID；B02 自身没有 errorWorkflow。
- 配置和工作流改动前备份位于 `/opt/SilkSpool/backups/matrix-integration-20261029/`；旧工作流完整 API 导出位于 `/home/dev/silkspool_backups/n8n/20261029-145629/`。

## 4. 通过本地 API 触发

```bash
spool serve --socket /run/spool.sock --token "$SPOOL_TOKEN"
curl --unix-socket /run/spool.sock \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $SPOOL_TOKEN" \
  -d '{"action":"bundle.apply","target":{"host":"prod","bundle":"prod-stack"},"options":{"locked":true,"yes":true},"idempotency_key":"bk-'$(date +%s)'"}' \
  http://localhost/v1/actions
```

随后轮询 `GET /v1/jobs/<job_id>`。n8n/Bellkeeper 不需要持有 SSH 私钥。

---

## 5. 禁止事项

- 不要直接 `ssh` / `docker` / `docker compose up` 操作 prod。
- 不要 `docker compose down -v` / `--volumes`：会删除 n8n 等有状态卷（当前为 `prod-n8n`）。
- 不要用旧命令 `spool bundle keeper ...`（兼容 alias，已弃用）；当前生产用 `prod-stack` / `prod`。
- 不要手写跨机 IP / `extra_hosts`；由 `dependencies.data` 的 endpoint 拓扑和 MagicDNS 解析。
- 不要把数据层（silkdata）加进 `knowledge-stack` 的 components；它必须是 dependencies。

n8n 更新 settings 会保留遗漏字段：B02 的 `errorWorkflow` 必须显式设为空字符串，不能靠删除 JSON 字段清除旧 ID；其它工作流指向当前 B02 ID。

## NFS 与 LiveSync 持久配置

prod 的 knowledge 已通过 `prod-knowledge` NFS volume 接入 Bellkeeper；录像已增加宿主挂载 `/mnt/NAS/data/media/recordings`。NFS 配置、写入验收和容器接入示例见 [NFS-SETUP](ops/NFS-SETUP.md)。不要把旧 `NAS/data/prod/couchdb` dataset 当作正在使用的 `prod/store/couchdb`。

CouchDB `prod-couchdb-config` 保留认证/运行配置；本次通过管理 API 和现有 volume 写入 `10-livesync.ini`，仅重启现有容器。未来 proddata 模板也挂载这一基线文件，避免重建后丢失 CORS 配置。API 生成的管理员哈希在 `docker.ini`，随 config volume 保存；新建容器仍需保留此 volume。客户端配置与已有 Vault 首次上传步骤见 [LiveSync 清单](knowledge-base/setup/livesync-checklist.md)。
