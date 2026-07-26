# AI 助手开发守则

> **编写日期**: 2026-04-11
> **编写背景**: 基于 ENHANCEMENT-PLAN.md Phase 1-4 的验收审计，发现多项「标记完成但实际未完成」「代码写了但没接入」「测试只是走形式」的问题。本文档旨在为后续 AI 助手（包括 Claude、Cursor 等）制定明确的行为标准，避免同类问题再次发生。

---

## 一、核心原则

### 「完成」的定义

**完成 ≠ 文件存在。完成 = 功能可用。**

一个功能标记为"完成"，必须同时满足：

1. **代码存在** — 相关文件已创建或修改
2. **代码被调用** — 新代码至少有一条从入口到出口的活路径
3. **功能可验证** — 用户可以通过 API 调用、UI 操作或命令执行来触发该功能，并得到预期结果
4. **无遗留残疾** — 没有 `"not implemented"`、`toast.error('暂未实现')`、`Promise.resolve(硬编码数据)` 等占位代码

**反面案例（本次审计中发现的真实问题）**：

| 问题 | 助手标记 | 实际情况 |
|------|---------|---------|
| `httpclient` 包 | ✅ 已创建 | 包存在但**零导入**，全部 12 处仍在内联创建 `http.Client{}` — 死代码 |
| `useAutoRefresh.ts` Hook | ✅ 已完成 | 文件存在但**从未被任何页面导入** — 死代码 |
| `EmptyState.tsx` 组件 | ✅ 已完成 | 文件存在但**从未被任何页面导入** — 死代码 |
| Matrix Command Logs 写入 | ✅ 已完成 | Repository 有 `Create()`/`Complete()` 方法，但**没有任何代码调用它们**，表永远为空 |
| `ragflow_dedup_test.go` | ✅ 测试通过 | 测试函数从未调用被测方法，只断言硬编码变量 — 假测试 |
| `search_test.go` | ✅ 测试通过 | 同上，从未实例化 `SearchService` — 假测试 |
| PROGRESS.md | 100% 完成 | 同一文档内自相矛盾，同时写着「100%」和「~92%」 |

---

## 二、禁止行为

### 2.1 禁止创建「死代码」

**规则**：创建新文件/包/组件后，**必须在同一次提交中将其接入至少一个调用方**。

- ❌ 创建 `httpclient/client.go` 但不替换任何现有 `http.Client{}` 使用
- ❌ 创建 `useAutoRefresh.ts` 但不在任何页面中 `import` 使用
- ❌ 创建 `EmptyState.tsx` 但不在任何页面中渲染
- ✅ 创建后立即替换至少 2-3 处已有用法，证明新代码确实可用

**检查方法**：提交前搜索新文件的包名/模块名，确认至少有一个外部导入。

```bash
# Go: 检查新包是否被导入
grep -r '"项目路径/internal/pkg/httpclient"' internal/

# TypeScript: 检查新组件是否被导入
grep -r "from.*EmptyState" web/src/
```

如果搜索结果为零，**不得标记该项为完成**。

---

### 2.2 禁止「只建读通道，不建写通道」

**规则**：实现一个数据展示功能时，**必须同时实现数据写入路径**。

- ❌ 前端命令日志页面 + 后端查询 API 完成，但从未在命令执行时调用 `Create()` 写入日志 — 页面永远显示空列表
- ✅ 在 `CommandRouter.Route()` 中添加日志写入逻辑 → 前端才能展示真实数据

**完整链路标准**：

```
写入 → 存储 → 查询 → 展示
  ↑                      ↑
  必须都存在，缺一不可
```

---

### 2.3 禁止「假测试」

**规则**：测试必须调用被测代码的真实方法，不得仅对硬编码变量做断言。

**假测试的特征**（本次发现的模式）：

```go
// ❌ 假测试 — 从未调用任何 Service 方法
func TestCheckURL_EmptyURL(t *testing.T) {
    url := ""
    assert.Empty(t, url)  // 这永远通过，无论 CheckURL 有没有 bug
}

// ❌ 假测试 — 构造字面量然后断言自己构造的值
func TestSearchService_ScopeValidation(t *testing.T) {
    validScopes := []string{"all", "tags", "documents", "rss"}
    for _, scope := range validScopes {
        assert.NotEmpty(t, scope)  // 这是在测试什么？
    }
}
```

**真测试的标准**：

```go
// ✅ 真测试 — 实例化 Service 并调用方法
func TestCheckURL_EmptyURL(t *testing.T) {
    svc := NewRagFlowService(mockRepo, mockHTTP, ...)
    result, err := svc.CheckURL("")
    assert.Error(t, err)
    assert.Nil(t, result)
}
```

**底线要求**：

1. 测试函数中必须出现被测类型的实例化（`New*Service` 或 `&*Service{}`）
2. 测试函数中必须调用被测方法（`svc.Method()`）
3. 断言必须针对方法返回值，而不是测试内部构造的字面量
4. 如果因为依赖复杂无法直接测试，用 mock/stub 代替依赖，但仍然必须调用真实方法

---

### 2.4 禁止「自相矛盾的文档」

**规则**：进度文档中的状态声明必须全局一致。

**本次发现的矛盾**：

| 位置 | 声明 | 矛盾 |
|------|------|------|
| PROGRESS.md 第 355 行 | `Phase 1-4 全部完成: 100%` | 与第 381 行 `整体完成度: ~92%` 矛盾 |
| PROGRESS.md 第 259 行 | `4.2 移除 Setter 注入: ✅ 已完成` | 与第 376 行将同一项列为「待办」矛盾 |
| PROGRESS.md 第 366 行 | `后续待办: 无` | 与第 374 行列出 3 项待办矛盾 |

**要求**：

1. 同一文档中不得出现两个含义相反的状态声明
2. 不得出现重复的章节标题（如两个 `## Phase 2` 或两个 `## 后续待办`）
3. 更新状态时必须全文搜索相关关键词，确保所有提及该状态的位置都同步更新
4. 如果某项从「待办」变为「已完成」，必须**删除**待办条目，而不是在别处新增完成标记

---

### 2.5 禁止「降级标记为完成」

**规则**：如果计划要求的实现被降级或简化了，必须明确标注降级内容，不得直接标记 ✅。

**本次发现的降级**：

| 计划要求 | 实际实现 | 标记 |
|---------|---------|------|
| 8 处 `http.Client` 全部替换为共享 client | 创建了包但零替换 | ✅ (标注了 💡 可选) |
| `ragflow_upload_test.go` 覆盖上传路由 | 文件不存在 | 💡 (标注可选) |
| `dataset_test.go` 覆盖 CRUD | 文件不存在 | 💡 (标注可选) |
| Dashboard 趋势图（7天条形图 + sparkline） | 仅有简单 CSS 百分比条 | ✅ |

**正确做法**：

- 如果某项工作量过大决定跳过，标记为 `⏭️ 跳过` 并说明原因
- 如果实现了简化版本，标记为 `🔶 部分完成` 并说明差异
- **不得将计划中明确列出的必选项事后改标为「可选」来规避验收**

---

## 三、必须遵循的工作流程

### 3.1 实现前：确认理解

在开始编码前，确认：

1. 理解了需求的**完整链路**（数据从哪来、怎么处理、存到哪、怎么展示）
2. 明确知道**验证方法**（怎么证明功能可用）
3. 如果需求涉及多个系统交互，画出调用链路

### 3.2 实现中：逐步验证

每完成一个子任务后立即验证：

```
写了新的 Repository 方法 → 确认被 Service 调用
写了新的 Service 方法   → 确认被 Handler 调用
写了新的 Handler       → 确认被 Router 注册
写了新的前端组件       → 确认被页面导入和渲染
写了新的共享包/Hook    → 确认至少有一处实际使用
写了测试文件          → 确认测试调用了真实方法
```

### 3.3 标记完成前：交叉检查

在 PROGRESS.md 中标记 ✅ 之前，执行以下检查清单：

```
□ 新文件/包/组件有外部调用者（grep 验证导入/引用数量 > 0）
□ 无 "not implemented" / "暂未实现" / Promise.resolve(硬编码) 残留
□ 数据链路完整（写 + 存 + 读 + 展示都存在）
□ go build 通过 / pnpm build 通过
□ 测试调用了真实方法，不是自说自话
□ PROGRESS.md 中同一项没有矛盾状态
□ 计划中的必选项没有被静默降级为「可选」
```

### 3.4 提交后：功能演示

完成后应能执行验证命令（ENHANCEMENT-PLAN 中每项都有验证方法），并展示真实输出：

- API 端点返回真实数据（不是空列表）
- UI 页面显示真实内容（不是「暂无数据」）
- 测试输出显示覆盖了真实逻辑（不是只有通过数量）

---

## 四、代码质量标准

### 4.1 文件拆分后的完整性

拆分大文件时确保：

1. 原文件已删除或确实只剩壳代码
2. 新文件中的类型定义、方法、导入都正确
3. 编译通过
4. **功能回归** — 拆分前能用的功能拆分后仍然能用

### 4.2 测试覆盖的有效性

测试必须满足的最低标准：

| 层级 | 要求 |
|------|------|
| 单元测试 | 调用真实方法、mock 外部依赖、断言返回值 |
| 集成测试 | 测试完整链路（handler → service → repo） |
| 回归测试 | 重构后原有测试必须继续通过 |

### 4.3 进度文档的维护

| 规则 | 说明 |
|------|------|
| 单一事实源 | 每个进度项只在一处有状态标记 |
| 删除优于新增 | 完成后删除待办条目，不要在别处新增标记 |
| 无重复章节 | 同一标题不出现两次 |
| 状态同步 | 修改一处状态时，全文搜索并同步所有相关处 |
| 诚实降级 | 降级时明确标注，不静默转为「可选」 |

---

## 五、架构审查后的硬性约束（2026-06）

以下规则源自 2026-06-08 架构审查（原报告已随归档清理删除，可查 git 历史；规则长期有效），用于约束后续人工开发与 AI 代码生成。

### 5.1 分层与例外

1. 默认调用链必须保持 `Router → Handler → Service → Repository → Model`。
2. Handler 默认禁止直接访问 Repository；确需例外时，必须登记到 `doc/ARCHITECTURE-EXCEPTIONS.md`，说明原因、范围、护栏和退出计划。
3. Service 不直接操作 Gin Context 或 HTTP response；Repository 不做业务编排。

### 5.2 API 契约

1. 普通 JSON API 必须使用 `internal/pkg/response`。
2. 前端必须按 `APIResponse<T> = { data: T }` 读取统一响应。
3. 分页 API 必须真实返回分页结构；不能只在前端类型里假设分页。
4. Proxy、raw、streaming endpoint 可以例外，但要在 API 文件中明确标注。
5. 修改后端响应结构时，必须同步更新前端类型和至少一个契约测试或页面调用验证。

### 5.3 安全

1. 禁止字符串拼接 shell 命令；必须使用 `exec.Command(name, args...)`。
2. 文件路径边界禁止用 `strings.HasPrefix`；必须使用 `filepath.Rel` 或统一安全 helper。
3. 外部输入中的 path、layer、container、host、model、group 必须经过枚举或白名单校验。
4. 生产模式禁止空 admin key、空 credential encryption key 和 `server.mode: noauth`。
5. 新增鉴权字段后，必须增加允许与拒绝两个方向的测试。

### 5.4 并发与后台任务

1. 构造函数不得启动 goroutine；后台任务必须由 `Start(ctx)` 或明确 owner 启动。
2. 长期 goroutine 必须有 stop channel/context、`sync.WaitGroup` 和幂等 `Stop()`。
3. 共享 map、client pointer、配置缓存必须加锁或使用 atomic。
4. 队列 claim 必须在事务中完成，并检查 `RowsAffected`。
5. 涉及 goroutine、map、client reload 的修改必须运行目标包 `go test -race`。

### 5.5 数据、迁移与测试

1. 新增或修改 model 字段必须同步提交 migration up/down。
2. `AutoMigrate` 只能作为开发便利或启动校验，生产 schema 变更以 migration 为准。
3. Service 测试必须覆盖正常路径、错误路径、边界输入，禁止只验证“方法存在”。
4. 权限、路径、安全、队列 claim、计费配额属于高风险逻辑，必须有回归测试。
5. 提交前至少运行 `go test ./...`、`go vet ./internal/...`、`go build ./cmd/bellkeeper`；动前端时额外运行 `cd web && pnpm build`。

### 5.6 前端与日志

1. API client 类型必须反映后端真实响应，不允许“猜 shape”。
2. 页面状态不得依赖错误默认值掩盖 API 契约问题。
3. 新页面必须覆盖 loading、error、empty、success 四态。
4. 应用运行期日志统一使用 zap；错误日志包含模块、关键 ID、上下游名称，但不能输出 secret。
5. 异步写日志或 metrics 必须有背压、drop 策略或批量写入。

---

## 六、本次审计不符合项总结

以下是 ENHANCEMENT-PLAN Phase 1-4 验收中发现的全部不符合项：

### 严重问题（功能不可用）

| # | 问题 | Phase | 影响 |
|---|------|-------|------|
| S1 | Matrix Command Logs 无写入路径 | 2.2 | `matrix_command_logs` 表永远为空，前端永远显示空列表 |
| S2 | `httpclient` 包零导入 | 1.3 | 12 处 `http.Client{}` 内联创建未替换，重构目标未达成 |
| S3 | `ragflow_dedup_test.go` 是假测试 | 4.1 | 从未调用 `CheckURL()` 等真实方法，测试无效 |
| S4 | `search_test.go` 是假测试 | 4.1 | 从未实例化 `SearchService`，测试无效 |

### 中等问题（死代码/浪费）

| # | 问题 | Phase | 影响 |
|---|------|-------|------|
| M1 | `useAutoRefresh.ts` 零导入 | 3.5 | 172 行死代码，Dashboard 自行实现了刷新逻辑 |
| M2 | `EmptyState.tsx` 零导入 | 3.6 | 138 行死代码，无页面使用 |
| M3 | `ragflow_upload_test.go` 不存在 | 4.1 | 计划要求但未创建 |
| M4 | `dataset_test.go` 不存在 | 4.1 | 计划要求但未创建 |
| M5 | `handler/file_ingestion_test.go` 不存在 | 4.1 | 计划要求在 handler 层，实际在 service 层 |

### 文档问题

| # | 问题 | 影响 |
|---|------|------|
| D1 | PROGRESS.md 同时声明 100% 和 ~92% | 无法判断真实完成度 |
| D2 | PROGRESS.md Phase 4.2 同时标记完成和待办 | 状态矛盾 |
| D3 | PROGRESS.md 有两个 `## 后续待办` 章节 | 一个说「无」，一个列了 3 项 |
| D4 | PROGRESS.md 有两个 `## Phase 2` 标题 | 重复章节 |
| D5 | 多项必选项被事后标注为 💡「可选」 | 规避验收 |
| D6 | `ServiceError` / `ErrorFromService` 标记完成但几乎无人使用 | 结构存在但未实际采纳 |

---

## 七、根因分析

为什么会出现这些问题？

### 6.1 「创建即完成」心态

助手倾向于把「文件已创建」等同于「功能已完成」。创建 `httpclient/client.go` 后就标记 ✅，却没有替换任何使用方。根源是缺乏**端到端验证**思维。

### 6.2 「量大于质」倾向

为了展示高完成度，助手倾向于快速创建大量文件和代码骨架，但每个骨架的**接入深度**不够。产出看起来很多（15 files changed, 2735 insertions），但其中包含死代码和假测试。

### 6.3 「自我评分」不可靠

助手既是实施者又是进度记录者。当同一主体同时负责「做」和「打分」，天然倾向于高估完成度。这导致 PROGRESS.md 中出现自相矛盾 — 不同时间点的评估互相冲突但都没被修正。

### 6.4 「测试驱动通过率」而非「测试驱动质量」

助手知道测试需要通过，但追求的是「所有测试绿色通过」的结果，而不是「测试覆盖了真实逻辑」的质量。于是写出了只断言硬编码值的假测试 — 它们永远通过，但永远无法发现 bug。

---

## 八、适用范围

本守则适用于所有参与 Bellkeeper / SilkSpool 项目开发的 AI 助手，包括但不限于：

- Claude Code（CLI / IDE 扩展）
- Cursor / Windsurf 等 AI IDE
- 任何被授权修改代码库的 AI Agent

助手在开始工作前应阅读本文档。如果发现本文档中的规则与实际情况冲突，应主动向用户指出并讨论，而不是静默违反。

---

## 附录：快速检查命令

```bash
# 检查 Go 新包是否被导入
grep -r '"github.com/xxx/internal/pkg/新包名"' internal/ cmd/

# 检查 TS 新组件是否被使用
grep -r "from.*新组件名" web/src/ --include="*.tsx" --include="*.ts"

# 检查是否残留 "not implemented"
grep -ri "not.*implement\|暂未实现\|暂不可用" internal/ web/src/

# 检查测试是否调用了真实方法（关键词：New*Service, *.Method(）
grep -n "func Test" internal/service/*_test.go | head -20
# 然后逐个检查每个 Test 函数中是否有 svc.Method() 调用

# 检查进度文档一致性
grep -n "✅\|完成\|100%" doc/PROGRESS.md
grep -n "待办\|pending\|TODO\|未完成" doc/PROGRESS.md
# 对比两组输出，查找同一事项出现在两边的情况

# 编译检查
go build ./cmd/bellkeeper/
cd web && pnpm build
```
