# CLAUDE.md — Bellkeeper 项目 Claude Code 协作约定

> 本文件位于仓库根目录，**每次会话由 Claude Code 自动加载**，是优先级最高的协作约定。
> 详细规范见 `doc/`，本文件只收录「必须时刻遵守」的条目 + 指针。**当本文件与 `doc/` 冲突时，以本文件为准**，并主动向用户指出冲突。

---

## 0. 先读这些（权威来源）

| 文档 | 内容 |
|------|------|
| `README.md` 顶部「Mandatory AI Execution Rules」 | SilkSpool 运维强制指令（见第 4 节）|
| `doc/DEVELOPMENT-GUIDE.md` | 完整编码规范、分层职责、禁止事项、验收标准 |
| `doc/ASSISTANT-GUIDELINES.md` | AI 助手开发守则（「完成」的定义、防死代码/假测试）|

> ⚠️ `doc/` 被 `.gitignore` 忽略，仅本地存在、不进版本库、也不会自动进入上下文。因此本文件已把最高频、最易违反的规则**复述**于下，确保即使不读 `doc/` 也能正确工作。需要细节时再去读对应文档。

**跨会话续作**：开工前先读 `memory/MEMORY.md` 指向的记忆文件，以及其中记录的计划文件路径（如 `/home/ubuntu/.claude/plans/*.md`）——**以计划文件为准，不要只相信对话上下文**（已发生过会话中途溢出导致进度失真）。

---

## 1. 语言约定（强制中文）

- **扩展思考（thinking / 推理过程）本身必须全程用中文**：从思考的第一个字起就用中文——规划、分析、推理、读代码后的判断、自我纠错都包含在内，**严禁"先用英文思考再翻译成中文"**。这是最易被忽略、最常被违反的一条（曾多次以英文思考），单列于此强制死守；衡量标准是「思考块逐字检查应见不到成段英文句子」（英文仅限标识符/类型名/命令等下方例外项）。
- **所有面向用户的输出一律用中文**：方案、解释、进度汇报、提交说明里的描述性文字、最终总结。
- **保持英文 / 跟随既有约定的例外**（不要强行中文化）：
  - 代码标识符、类型名、API 字段、配置 key、日志关键字；
  - `go` / `git` / `make` 等工具的原始输出；
  - Git commit 的前缀与既有风格（如 `fix(llm-proxy): Tier N — ...`，正文描述可中文）；
  - 代码注释**跟随所在文件的既有语言风格**，不混入新风格。
- 一句话原则：**讲给人听的用中文，写进代码/喂给机器的跟随代码库。**

---

## 2. 关键编码规则（违反将导致审查不通过）

### 2.1 架构分层（单向调用）
- 调用链严格单向：`Router → Handler → Service → Repository → Model`。
- **禁止跨层**：Handler 不得直接调 Repository；Service 不得直接操作 HTTP 响应；Repository 不含业务逻辑。
- 已知分层例外必须登记在 `doc/ARCHITECTURE-EXCEPTIONS.md`，新增例外需要写明范围、原因、护栏和退出计划。
- **手动依赖注入**：构造函数注入（`NewXxx(...)`），无 DI 容器、**无全局可变状态**、无 `init()` 副作用。
- 新增模块按 `model → db.go(AutoMigrate) → repository → repository.go → service → service.go → handler → handler.go → router` 全链路接好（详见开发指南「开发清单」）。

### 2.2 统一响应 / 常量
- 所有 Handler 必须用 `internal/pkg/response`（`Success` / `SuccessList` / `BadRequest` / `NotFound` / `InternalError`）。
- **禁止直接 `c.JSON`**，唯一例外：LLM 代理端点透传上游响应。
- 硬编码值集中到 `internal/pkg/defaults`，不要散落字面量。

### 2.3 错误处理
- **永不忽略 error**：禁止 `_ = f()` 或丢弃返回值；用 `fmt.Errorf("...: %w", err)` 包装（`%w` 而非 `%v`）。
- **禁止 `panic`**（除真正不可恢复的启动期）；Service 返回 error，由 Handler 决定 HTTP 状态码。
- 异步 goroutine 内的 error 必须被记录，不得静默吞掉。

### 2.4 并发安全
- 共享状态必须有锁（读写分离用 `sync.RWMutex`）。
- **goroutine 必须可停止**：`stopCh chan struct{}` + `sync.WaitGroup`，`Stop()` 关闭后 `wg.Wait()`；禁止 `for { time.Sleep }` 式无法停止的后台任务。
- **禁止无限制创建 goroutine**：用信号量 `chan struct{}` 限制并发数。

### 2.5 数据库（GORM）
- 用查询构建器，**禁止手工拼接 SQL**（注入风险）。
- 多步写操作用 `db.Transaction` 保原子性；关联查询用 `Preload` 避免 N+1。
- **禁止在循环里查库**，改用 `Where("id IN ?", ids)` 批量。
- 并发累加用 `clause.OnConflict` 原子 upsert，不要 read-modify-write。

### 2.6 安全
- **禁止硬编码密钥/密码**，一律从环境变量或配置读取（`${ENV_VAR}` 语法）。
- 输入需校验；HTML 输出需清洗（`bluemonday`）防 XSS；API Key 用常量时间比较。
- 新增环境变量必须同步到 `bellkeeper-init.sh` 的 `export`。

### 2.7 「完成」的定义（本项目最痛的教训，务必遵守）
> **完成 ≠ 文件存在；完成 = 功能可用且被接入。**
- **禁止死代码**：新建包/组件/Hook 后，**必须在同一次提交内接入至少一个真实调用方**；提交前 `grep` 验证导入数 > 0。
- **禁止只读不写**：做了展示功能，必须同时实现写入路径（写→存→读→展示，缺一不可）。
- **禁止假测试**：测试必须实例化被测类型并调用真实方法、断言其返回值；不得只断言测试内部构造的字面量。
- **禁止占位残留**：不留 `not implemented`、`暂未实现`、`Promise.resolve(硬编码)`。
- **禁止文档自相矛盾 / 静默降级**：进度状态单一事实源、不重复章节、改动时全文同步；降级要显式标注（`🔶 部分完成` / `⏭️ 跳过` + 原因），不得事后偷偷改标为「可选」规避验收。

### 2.8 提交纪律与绿色构建
- **每次提交前保持绿色**：`go build ./...`、`go vet ./...`、（动前端时）`cd web && pnpm build` 全部通过。
- **小步提交**：一个 tier / 一个可验收子任务一次提交，便于回滚、也防止会话溢出时丢失进度。
- 提交/推送/建 PR 等仅在用户要求时进行；在默认分支上先开分支。
- Git commit 结尾追加：
  `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`

---

## 3. 自检清单（标记完成 / 提交前逐项过）

```
□ go build ./...  /  go vet ./...  /  (前端) pnpm build  全绿
□ 新文件/包/组件 grep 验证有真实调用方（导入数 > 0）
□ 数据链路完整：写 + 存 + 读 + 展示都存在
□ 无 not implemented / 暂未实现 / 硬编码占位残留
□ 测试调用了真实方法，不是自说自话
□ 进度/文档无矛盾状态，必选项未被静默降级
□ 无硬编码密钥、无裸 SQL 拼接、无跨层调用
```

---

## 4. 运维：只用 SilkSpool（强制）

> 来自 `README.md` 的 AI 强制指令，违反可能误操作生产环境。

- **禁止**裸 `ssh` / `docker` / `docker-compose` / `rsync`；**所有**远程/服务操作走 `spool` CLI（`$PATH` 已全局可用）。
- 改任何配置（`.env` / Caddyfile / YAML）后立即 `spool sync push <host>`。
- 服务管理：`spool service <host> restart|status|logs <alias>`；服务别名先查 `silkspool.yaml` 的 `services` 块。
- **禁止 `docker compose down`**（会停全部服务）；只重启指定服务。
- 本地依赖容器（Postgres 等）可用 `make docker-up/down`——这是本机开发环境，不是生产。

### 4.1 spool 命令优先级（禁止滥用 exec）

**核心原则：专用命令优先，`exec` 仅作为最后手段。** `spool exec` 等同于裸 SSH，绕过了 spool 的日志、错误处理、安全审计。以下场景**必须**使用专用命令，**禁止**用 `exec` 替代：

| 场景 | 正确命令 | 禁止 |
|------|---------|------|
| 部署/更新整个 bundle | `spool bundle <name> up <host>` | ~~`spool exec <host> "docker compose up -d"`~~ |
| 查看 bundle 状态 | `spool bundle <name> status <host>` | ~~`spool exec <host> "docker compose ps"`~~ |
| 重启单个服务 | `spool restart <host> <alias>` 或 `spool service <host> restart <alias>` | ~~`spool exec <host> "docker restart sp-xxx"`~~ |
| 查看服务状态 | `spool service <host> status [alias]` | ~~`spool exec <host> "docker ps"`~~ |
| 查看服务日志 | `spool logs <host> <alias> [lines]` | ~~`spool exec <host> "docker logs sp-xxx"`~~ |
| 同步配置到远程 | `spool sync push <host>` 或 `spool push <host>` | ~~`spool exec <host> "rsync ..."`~~ |
| 拉取远程配置 | `spool sync pull <host>` 或 `spool pull <host>` | ~~`spool exec <host> "cat /path/config"`~~ |
| 备份主机数据 | `spool backup <host>` | ~~`spool exec <host> "tar ..."`~~ |

`spool exec` 仅在**专用命令无法覆盖**的场景使用（如检查磁盘空间 `df -h`、查看进程 `ps aux` 等）。

### 4.2 Bellkeeper 部署标准流程

Bellkeeper 部署在 **keeper** 主机（`192.168.7.230`），bundle 名为 `keeper`，服务别名 `bellkeeper`。

**完整部署（代码更新 + 构建镜像 + 启动）：**
```bash
spool bundle keeper up keeper
```
此命令自动完成：git pull → sync push（推送 .env / n8n-workflows / couchdb 配置）→ docker build → docker compose up -d。

**Bellkeeper 单服务代码部署（优先使用，禁止用 exec 手搓 docker compose）：**
```bash
spool bundle keeper service keeper bellkeeper up
```
此命令用于仅更新 Bellkeeper 服务镜像/容器；有专用 spool 命令可用时，必须优先使用专用命令，`spool exec` 仅用于专用命令覆盖不到的临时诊断。

**仅重启服务（代码未变，仅改了 .env 或配置）：**
```bash
spool sync push keeper          # 先推送配置
spool restart keeper bellkeeper # 再重启 bellkeeper 容器
```

**仅重启 bellkeeper（不改配置）：**
```bash
spool restart keeper bellkeeper
```

**查看状态 / 日志：**
```bash
spool service keeper status bellkeeper   # 查看运行状态
spool logs keeper bellkeeper 100         # 查看最近 100 行日志
```

**keeper 主机上的其他服务别名**（来自 silkspool.yaml）：
`redis` / `n8n` / `bellkeeper` / `bellkeeper-db` / `memos` / `rsshub` / `couchdb` / `nats` / `meilisearch`

**注意事项：**
- `spool bundle keeper up keeper` 会重建所有服务镜像，耗时较长（~5 分钟），仅在代码变更时使用。
- 仅修改 `.env` 时不需要 `bundle up`，`sync push` + `restart bellkeeper` 即可（init 脚本会从挂载的 .env 重新加载变量）。
- **禁止** `spool bundle keeper down keeper`——会停止 keeper 上所有服务。

---

## 5. 会话体量与检查点（防上下文溢出）

**背景**：本仓库的 LLM Proxy 审计曾在一个 tier 中途上下文溢出、触发自动压缩，导致进度失真。本节目标是**在我们自己选定的干净边界（提交点）主动存档收尾**，而不是被动等自动压缩在任务中途打断。

### 5.1 阈值（按上下文窗口百分比，稳健于具体窗口大小）

当前模型 `claude-opus-4-8[1m]`，窗口 **W = 1,000,000 token**。预留安全裕量 **R ≈ 150k（15%）**（基线注入 ~30k + 存档动作 ~11k + 一次意外大工具结果 ~40k + 最终响应 ~16k + 兜底 ~53k）。

| 阈值 | 百分比 | 1M 窗口绝对值 | 动作 |
|------|--------|--------------|------|
| **软检查点** | 65% | ~650k | 不再开启新的大子任务/新 tier；收尾当前改动、提交。理由：单个 tier 约耗 80–150k，从此点出发仍能在硬上限前干净完成。|
| **硬上限** | 85% | ~850k | **立即**执行检查点流程并结束会话，即使当前任务未完。|

> 若实际窗口不是 1M（例如未启用 1M、按 200k 计），按同百分比换算：软 ~130k / 硬 ~170k。
> **判断依据**：以 Claude Code 状态栏 / `/context` 实际显示的用量为准，并留意 Claude Code 自动注入的「上下文不足」提醒——出现该提醒即视为已达硬上限。

### 5.2 检查点流程（到阈值时按序执行）

1. **收尾到原子边界**：把当前改动收到一个可验收单元，确保 `go build ./...` + `go vet` 绿（动前端则 `pnpm build` 绿）。若实在收不齐，至少让代码可编译。
2. **提交**：`git commit`（每 tier / 子任务一提交，遵守第 2.8 节）。
3. **存档进度**：调用 `/context-manager` 技能归档对话要点到记忆；更新计划文件与 `memory/MEMORY.md` 中的「下一步 / 当前进度」。
4. **输出交接摘要**（中文）：已完成、进行中（停在哪）、下一步第一个动作、涉及的关键文件与命令。
5. **结束会话**：明确告诉用户「请新开一个会话（或 `/clear`）继续，开工先读计划文件 + 记忆」，然后**停止产出新工作**。

### 5.3 诚实边界（我能做与不能做）
- 我**无法**逐 token 精确自测用量、也**无法**自行杀掉会话进程；我能做的是：监控 Claude Code 的用量提示、在达阈值时执行上述检查点、并停止接新活、提示用户另起会话。
- 因此**小步提交 + 每会话先读计划文件**是防溢出的根本手段，阈值检查点是第二道防线。

---

## Agent skills

### Issue tracker

Issues tracked in GitHub (singll/Bellkeeper) via `gh` CLI. See `docs/agents/issue-tracker.md`.

### Triage labels

Using default triage label vocabulary (needs-triage, needs-info, ready-for-agent, ready-for-human, wontfix). See `docs/agents/triage-labels.md`.

### Domain docs

Single-context layout. See `docs/agents/domain.md`.
