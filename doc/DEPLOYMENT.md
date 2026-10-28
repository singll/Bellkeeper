# Bellkeeper 部署与配置（现行）

> 最后更新：2026-10-28 · 适用 SilkSpool `spool` 1.2+
>
> Bellkeeper 以 **product bundle** 发布；生产由 SilkSpool 的 `knowledge-stack` composition 引用，
> `silkdata` 作为只读数据依赖。不要直接 SSH/Docker 操作远程主机。

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

## 2. 生产配置示例

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

> Matrix 凭证、LLM 渠道/模型组等运行时配置通过 Web UI 写入数据库并热重载；只有需要环境变量覆盖时
> 才在 `.env` 增加 `MATRIX_*` / `LLM_*`。`knowledge-stack` 的 wiring 已覆盖数据库/Redis/NATS/
> Meili/n8n 等连接参数。

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

## 3. 日常操作

```bash
# 静态校验（不连远端；合法 0，非法 2）
spool plan knowledge-stack keeper --offline

# online diff（有 drift 退出 3）
spool plan knowledge-stack keeper

# 可复现部署（健康检查失败自动回滚）
spool apply knowledge-stack keeper --locked --yes

# 回滚 / 恢复
spool rollback knowledge-stack keeper
spool rollback knowledge-stack keeper 3
spool restore knowledge-stack keeper

# 状态 / 日志 / 单服务重启
spool inventory --json
spool service keeper status bellkeeper
spool restart keeper bellkeeper
spool logs keeper bellkeeper 200
```

- `knowledge-stack` 只 apply keeper 上的 components；`dependencies.data` 只读。
- 升级 Bellkeeper：发布新镜像 + 真实 digest → 更新 `bundle_sources` 固定 commit / OCI digest →
  `plan --offline` → online plan → `apply --locked --yes`。
- `backup-then-recreate` 升级前会调用 `deploy/scripts/backup.sh`，产物在
  `/opt/silkspool/keeper/backups/`。
- 不可逆迁移（`irreversible: true`）不会自动回滚，需显式 `--allow-irreversible` 并人工处理。

---

## 4. 通过本地 API 触发

```bash
spool serve --socket /run/spool.sock --token "$SPOOL_TOKEN"
curl --unix-socket /run/spool.sock \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $SPOOL_TOKEN" \
  -d '{"action":"bundle.apply","target":{"host":"keeper","bundle":"knowledge-stack"},"options":{"locked":true,"yes":true},"idempotency_key":"bk-'$(date +%s)'"}' \
  http://localhost/v1/actions
```

随后轮询 `GET /v1/jobs/<job_id>`。n8n/Bellkeeper 不需要持有 SSH 私钥。

---

## 5. 禁止事项

- 不要直接 `ssh` / `docker` / `docker compose up` 操作 keeper。
- 不要 `docker compose down -v` / `--volumes`：会删除 `kp-n8n-data` 等有状态卷。
- 不要用旧命令 `spool bundle keeper ...`（兼容 alias，已弃用）；统一用 `knowledge-stack`。
- 不要手写跨机 IP / `extra_hosts`；由 `dependencies.data` 的 endpoint 拓扑和 MagicDNS 解析。
- 不要把数据层（silkdata）加进 `knowledge-stack` 的 components；它必须是 dependencies。
