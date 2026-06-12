# CLAUDE.md — Bellkeeper 项目协作约定

> 本文件由 Claude Code 自动加载，优先级最高。详细规范见 `doc/`（现已进版本库）。
> 当本文件与 `doc/` 冲突时，以本文件为准。

## 0. 必读

| 文档 | 内容 |
|------|------|
| `doc/DEVELOPMENT-GUIDE.md` | 完整编码规范、分层职责、禁止事项、验收标准 |
| `doc/ASSISTANT-GUIDELINES.md` | AI 助手守则（「完成」定义、防死代码/假测试） |
| `doc/ROADMAP.md` | 演进规划与优先级 |
| `doc/ARCHITECTURE.md` | 架构与数据模型 |
| `memory/MEMORY.md` | 跨会话记忆（开工前先读） |

## 1. 语言（强制中文）

- **思考过程全程中文**，严禁先用英文再翻译；例外：代码标识符/类型名/命令/工具输出。
- **面向用户的输出中文**；代码注释跟随文件既有风格。
- 一句话：讲给人听的用中文，写进代码的跟随代码库。

## 2. 红线（违反即审查不通过）

### 2.1 架构分层
- `Router → Handler → Service → Repository → Model` 严格单向，禁止跨层。
- 例外必须登记在 `doc/ARCHITECTURE-EXCEPTIONS.md`（范围、原因、护栏、退出计划）。
- 手动 DI 构造函数注入，无全局可变状态、无 `init()` 副作用。

### 2.2 「完成」= 功能可用且被接入
- **禁止死代码**：新包/组件必须同次提交内接入真实调用方（`grep` 验证导入 > 0）。
- **禁止假测试**：必须调真实方法断言返回值，不得只断言内部构造。
- **禁止占位残留**：不留 `not implemented` / `暂未实现`。
- **禁止静默降级**：降级要显式标注（`🔶` / `⏭️` + 原因）。

### 2.3 错误处理
- **永不忽略 error**：禁止 `_ = f()`；用 `fmt.Errorf("...: %w", err)` 包装。
- **禁止 panic**（除启动期不可恢复）；Service 返回 error，Handler 决定状态码。
- 异步 goroutine 内 error 必须记录。

### 2.4 安全
- **禁止硬编码密钥**，一律从环境变量/配置读取；新增环境变量同步到 `bellkeeper-init.sh`。
- 输入校验 + HTML 清洗（`bluemonday`）；API Key 常量时间比较。

### 2.5 提交纪律
- 每次提交前 `go build ./...` + `go vet ./...` + （动前端时）`pnpm build` 全绿。
- 小步提交，一个 tier/子任务一次。
- commit 结尾追加：`Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`

## 3. 运维：只用 SilkSpool（强制）

- **禁止**裸 `ssh` / `docker` / `rsync`；**所有**远程操作走 `spool` CLI。
- 专用命令优先，`exec` 仅作最后手段。

| 场景 | 命令 |
|------|------|
| 单服务部署 | `spool bundle keeper service keeper bellkeeper up` |
| 仅重启 | `spool restart keeper bellkeeper` |
| 改配置后重启 | `spool sync push keeper && spool restart keeper bellkeeper` |
| 状态/日志 | `spool service keeper status bellkeeper` / `spool logs keeper bellkeeper 100` |
| 完整部署 | `spool bundle keeper up keeper`（耗时~5min，仅代码变更时用） |

- Bellkeeper 部署在 keeper 主机(`192.168.7.230`)，bundle `keeper`，别名 `bellkeeper`。
- **禁止** `spool bundle keeper down keeper`——会停全部服务。

## 4. 上下文溢出防护

- **软检查点 ~65% / 硬上限 ~85%** 上下文窗口用量时主动收尾提交、存档记忆、结束会话。
- 判断依据：Claude Code 状态栏用量或「上下文不足」提醒。
- 小步提交 + 每会话先读计划文件是根本手段。

## Agent skills

### Issue tracker
Issues tracked in GitHub (singll/Bellkeeper) via `gh` CLI. See `docs/agents/issue-tracker.md`.

### Triage labels
Using default triage label vocabulary. See `docs/agents/triage-labels.md`.

### Domain docs
Single-context layout. See `docs/agents/domain.md`.
