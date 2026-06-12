# LLM Proxy 前端界面重构与统一凭证模型 — 实施文档

> 本文档是 **实施级蓝图**，给后续开发会话（含 AI 助手）逐步落地用。
> 一切以代码为准：文中 `file:line` 锚点是落笔时（2026-06-07）的真值，开发过程中会漂移，**改前先 grep 校准**。
> 关联：`doc/ROADMAP.md` §（llm_channel_credentials 原始设计 271 / 736 行）、`doc/LLM_PROXY_GUIDE.md`（环境变量配置）、`CLAUDE.md` §2.7（"完成"的定义、死代码红线）。

---

## 0. 一句话目标

把 LLM 模块 **10 个割裂页面**收敛为 **5 个「展示为主体、配置就地内融」的页面**，并用 **统一凭证模型**消灭"渠道 API Key 与加密凭证表二元割裂"的混乱——让现存的加密凭证表（当前是**死代码**）成为密钥/凭证的**单一真相源**，同时偿还 `CLAUDE.md §2.7` 点名的技术债。

**两条主线，后端先行：**
- **主线 A（数据/凭证）**：统一凭证模型 → 接通转发与余额查询的解密路径 → 数据迁移。
- **主线 B（界面/收敛）**：展示页吸收配置职责 → 渠道页就地编辑 + 统一凭证区 → 导航 10→5，删除"配置"大杂烩页。

---

## 1. 现状盘点（带证据）

### 1.1 前端：10 个页面 / 4 节导航（SolidJS + Tailwind 自建工具类，深色主题）

导航定义在 [Layout.tsx:243-260](web/src/components/Layout.tsx#L243-L260)，LLM 模块 10 项分 4 节：

| 节 | 菜单项 | 文件 | 行数 | 性质 | 调用的 API |
|----|--------|------|------|------|-----------|
| 运行时 | 总览 | [LLMOverview.tsx](web/src/pages/llm/LLMOverview.tsx) | 334 | 展示 | channelsStatus / groupsStatus / getUsage / allBalances / listAlerts |
| 运行时 | 渠道 | [LLMChannels.tsx](web/src/pages/llm/LLMChannels.tsx) | 219 | **纯展示** | channelsStatus（+ 重置熔断器） |
| 运行时 | 模型组 | [LLMGroups.tsx](web/src/pages/llm/LLMGroups.tsx) | 134 | 展示 | groupsStatus / clearGroupSticky |
| 运行时 | 调用日志 | [LLMLogs.tsx](web/src/pages/llm/LLMLogs.tsx) | 145 | 展示 | listChannels / logs |
| 配置 | 配置 | [LLMConfig.tsx](web/src/pages/llm/LLMConfig.tsx) | **470** | **配置大杂烩** | 渠道 CRUD + 凭证 CRUD + 模型组 CRUD |
| 配置 | 池子路由 | [LLMPools.tsx](web/src/pages/llm/LLMPools.tsx) | 416 | 展示+配置 | listChannels / groupsStatus / get·setCodingStrategy / listRateLimits / list·deleteConversation |
| 配置 | Token | [LLMTokens.tsx](web/src/pages/llm/LLMTokens.tsx) | 239 | 配置 | listTokens / create·update·deleteToken / regenerateTokenKey |
| 配置 | 定价 | [LLMPricing.tsx](web/src/pages/llm/LLMPricing.tsx) | 169 | 配置 | listPricing / create·update·deletePricing |
| 计费 | 计费 | [LLMBilling.tsx](web/src/pages/llm/LLMBilling.tsx) | 114 | 展示 | getUsage |
| 告警 | 告警 | [LLMAlerts.tsx](web/src/pages/llm/LLMAlerts.tsx) | 175 | **只读展示** | listChannels / listAlerts |

设计系统：自建 Tailwind 工具类 `card / badge / btn / input / table` + 共享 `Modal`，共享逻辑在 [shared.ts](web/src/pages/llm/shared.ts)。**卡片风格集中在总览 / 渠道 / 模型组**，这是要保留并强化的视觉资产。

### 1.2 四个核心问题

**问题 A — 密钥入口二元割裂（用户当面踩坑的点）**
在 [LLMConfig.tsx](web/src/pages/llm/LLMConfig.tsx) 的渠道行里，同时有「编辑」和「凭证」两个按钮：
- 「编辑」表单里填 `api_key_env`（**转发上游时真正生效**的字段）；
- 「凭证」另弹一个 Modal，往加密凭证表写 AES 密文（见问题 B：**转发根本不读它**）。

操作者面对"我到底该在哪填 key"无所适从——这正是用户要求合并的根因。

**问题 B — 加密凭证表是死代码（违反 `CLAUDE.md §2.7`「禁止死代码」）**
凭证表有完整链路：model [llm_channel_credential.go:8-18](internal/model/llm_channel_credential.go#L8-L18) → repo（Create/Update/Get/ListByChannel/**GetDecrypted**/Delete）→ service CRUD [llm_credential.go](internal/service/llm_credential.go) → 4 个 REST 端点 [router.go:166-169](internal/router/router.go#L166-L169) → 前端凭证 Modal。
**但取明文的唯一出口 `GetDecryptedCredential` 零调用方**（全仓库 grep 仅命中定义 [service:133](internal/service/llm_credential.go#L133) + [repo:60](internal/repository/llm_channel_credential.go#L60)）。即：密文存得进、读得出预览，却从未被任何转发/余额逻辑消费——**写了一半的功能**。

**问题 C — 渠道表单缺字段（后端有、前端无编辑入口）**
后端 `model.LLMChannel`（[llm_channel.go](internal/model/llm_channel.go)）已含 `Tier / TaskTypes / BalanceProviderType / BalanceConfigJSON / ModelRPMOverrides`，且 handler 直接 `c.ShouldBindJSON(&model.LLMChannel)`（[handler:286-316](internal/handler/llm_proxy.go#L286-L316)）——**后端早已能接收全字段**。前端类型 `LLMChannelConfig`（[types/index.ts:141-160](web/src/types/index.ts#L141-L160)）也已声明这些字段，但 [LLMConfig.tsx](web/src/pages/llm/LLMConfig.tsx) 的 `chForm` 只编辑 `name/base_url/api_key_env/provider_type/rpm/rpd/priority/is_free/is_enabled/models`。**补这些字段是纯前端工作，零后端改动。**

**问题 D — 展示与配置物理割裂**
运行态在「渠道」展示页（[LLMChannels.tsx](web/src/pages/llm/LLMChannels.tsx) 纯展示，无任何 CRUD），配置却在另一个「配置」页（[LLMConfig.tsx](web/src/pages/llm/LLMConfig.tsx) 把渠道+凭证+模型组三件事塞一起）。操作者看到一个渠道熔断了，要跳到另一个页面才能改它——这是用户要消除的割裂。

### 1.3 后端：转发 / 余额凭证的真实链路

**转发上游用的 key** 只来自 `APIKeyEnv`，逻辑在 `dbChannelToConfig`（[service/llm_proxy.go:766-806](internal/service/llm_proxy.go#L766-L806)），核心 767-792：

```go
apiKey := ""
if ch.APIKeyEnv != "" {
    apiKey = os.Getenv(ch.APIKeyEnv)        // ① 先当"环境变量名"解析
    if apiKey == "" {
        if looksLikeEnvVar(ch.APIKeyEnv) {  // ② 像变量名却取不到 → 告警
            ...
        } else {
            apiKey = ch.APIKeyEnv           // ③ 不像变量名 → 当明文直填(明文落库!)
        }
    }
}
// → config.ChannelConfig{ APIKey: apiKey, ... }
```

`APIKeyEnv` 字段一身二职（环境变量名 **或** 明文 key），③ 分支意味着**直填的 key 以明文存进 DB**——而专门的加密凭证表却没人用，**安全倒挂**。

**余额查询凭证**：`registerBalanceProviders`（[service:285-297](internal/service/llm_proxy.go#L285-L297)）调 `balanceMgr.Register(name, BalanceProviderType, BaseURL, apiKey, BalanceConfigJSON)`，其中 `apiKey = ch.Config.APIKey`（同上解析值），`extraConfig = BalanceConfigJSON`（**明文** JSON）。签名见 [balance/manager.go:34](internal/llm/balance/manager.go#L34)。即余额的"额外凭证"（session / ak-secret）现在散落在明文 `BalanceConfigJSON`，而非加密表。

**这与 `ROADMAP.md` 的原始设计正好相反**：
- [ROADMAP.md:271](doc/ROADMAP.md) — `llm_channel_credentials` 本是为存"**非 API 调用用**"的额外凭证（例 `{"session":"xxx","new_api_user":"12345"}` / `{"ak_id":"","ak_secret":""}`）；
- [ROADMAP.md:313](doc/ROADMAP.md) — 用 `BELLKEEPER_CREDENTIAL_KEY` 的 AES-GCM 加密落库；
- [ROADMAP.md:736](doc/ROADMAP.md) — 早已规划「API Key 改为『环境变量名 / 直接粘贴』双模 + 『绑定余额来源』分页」。

**统一凭证模型就是把这条本应如此的设计真正接通。**

**加密前提**：`crypto.Encrypt/Decrypt`（[aesgcm.go:71/86](internal/pkg/crypto/aesgcm.go#L71-L86)）的 key 来自环境变量 `BELLKEEPER_CREDENTIAL_KEY`（[aesgcm.go:29](internal/pkg/crypto/aesgcm.go#L29)）；**未设置则加密被禁用**（明文存储，[aesgcm.go:3-4](internal/pkg/crypto/aesgcm.go#L3-L4)）。落地前必须先在 keeper 配置它（见 §3.7）。

**迁移基础设施已就位**：`AutoMigrate`（[db.go:39-56](internal/model/db.go#L39-L56)）已注册 `LLMChannel` + `LLMChannelCredential` 两表，GORM 加列安全；`AutoMigrateWithLLMSeed`（[db.go:96](internal/model/db.go#L96)）从 YAML seed 渠道。

---

## 2. 设计目标与原则

1. **展示为主体**：以运行态展示页为入口，配置「就地」内融（在你看到问题的同一张卡上直接改），不再"看一个页、改另一个页"。
2. **独立配置页最小化**：只有实在融不进展示语境的全局设置（Token、定价、全局开关）才保留独立配置区；删除「配置」大杂烩页。
3. **保留并强化卡片**：总览/渠道/模型组的卡片是既有视觉资产，重构在其上叠加"就地编辑"，不推倒重来。
4. **统一凭证 = 单一真相源**：一个渠道的所有密钥/凭证（转发用、余额用）统一在一处管理；消除 `APIKeyEnv` 的双义与 `BalanceConfigJSON` 的明文存储。
5. **偿还死代码债**：让 `GetDecrypted` 真正被转发/余额消费（呼应 `CLAUDE.md §2.7`「禁止死代码 / 禁止只读不写」）。
6. **后端先行的理由**：统一凭证改变了数据形状（凭证新增"用途/来源"维度）和解析入口（转发/余额改从凭证表取）。前端的统一凭证 UI 依赖后端先提供 `purpose/source/env_var_name` 字段与端点语义；后端不先稳定，前端只能照着会变的契约空转。故 **Phase B（后端）必须先于 Phase F（前端）落地并自验**。

---

## 3. 统一凭证模型（后端核心 = 主线 A）

### 3.1 概念：凭证 = 用途 × 来源

把"密钥/凭证"统一抽象成挂在渠道下的**凭证条目**，两个正交维度：

```
                       来源 Source
                ┌──────────────┬──────────────────┐
                │  env         │  direct          │
                │ (环境变量名)  │ (直填,AES加密)    │
   ┌────────────┼──────────────┼──────────────────┤
用 │ api        │ EnvVarName=  │ CredentialJSON=  │
途 │ (转发上游)  │ LLM_KIMI_KEY │ enc(sk-xxx)      │
   ├────────────┼──────────────┼──────────────────┤
P  │ balance    │ EnvVarName=  │ CredentialJSON=  │
   │ (余额查询)  │ LLM_X_BAL    │ enc({"session"…})│
   └────────────┴──────────────┴──────────────────┘
```

- **Purpose（用途）**：`api`（转发上游）/ `balance`（余额查询）。一个渠道可挂多条（如主+备 api key、外加一条 balance）。
- **Source（来源）**：`env`（存环境变量名，运行时 `os.Getenv` 解析，变量名非秘密、不加密）/ `direct`（直填，AES 加密存 `CredentialJSON`）。
- 这统一了今天三处割裂：`APIKeyEnv`（=api/env 或 api/direct）、加密凭证表（应是 balance/direct）、`BalanceConfigJSON`（balance 的非密参数）。

### 3.2 `LLMChannelCredential` 表扩展（GORM 加列，安全）

在现有结构（[llm_channel_credential.go:8-18](internal/model/llm_channel_credential.go#L8-L18)）上**新增 4 字段**，旧字段全部保留：

```go
type LLMChannelCredential struct {
    ID              uint       `gorm:"primaryKey" json:"id"`
    ChannelID       uint       `gorm:"index;not null" json:"channel_id"`
    // --- 新增：统一凭证维度 ---
    Purpose         string     `gorm:"size:20;default:'api';index" json:"purpose"`   // api | balance
    Source          string     `gorm:"size:20;default:'direct'" json:"source"`        // env | direct
    EnvVarName      string     `gorm:"size:100" json:"env_var_name"`                  // source=env 时的变量名(非密)
    IsPreset        bool       `gorm:"default:false" json:"is_preset"`                // 由 YAML seed 迁移而来
    Label           string     `gorm:"size:100" json:"label"`                         // 人类可读标签(可选)
    // --- 旧字段保留 ---
    ProviderType    string     `gorm:"size:50" json:"provider_type"`
    CredentialJSON  string     `gorm:"type:text" json:"-"`                            // source=direct 的 AES 密文
    Status          string     `gorm:"size:20;default:'active'" json:"status"`        // active|error|expired
    ErrorMessage    string     `gorm:"type:text" json:"error_message,omitempty"`
    LastRefreshedAt *time.Time `json:"last_refreshed_at"`
    CreatedAt       time.Time  `json:"created_at"`
    UpdatedAt       time.Time  `json:"updated_at"`
}
```

约束：`CredentialJSON` 维持 `json:"-"`（永不出 API，[现状注释:6-7](internal/model/llm_channel_credential.go#L6-L7)）；`EnvVarName` 可出 API（变量名非秘密，且 UI 要展示"引用了哪个变量"）。`ChannelCredentialView`（[service:14-17](internal/service/llm_credential.go#L14-L17)）随之带出新字段，`CredentialPreview` 逻辑保留——`source=env` 时预览可显示 `$LLM_KIMI_KEY`（解析成功/失败标记），`source=direct` 时仍显示 `abcd...wxyz`。

### 3.3 统一解析函数 `ResolveCredential`（修死代码的关键）

新增 service 方法，作为**密钥获取的唯一入口**，替代 `APIKeyEnv` 的内联解析与无人调用的 `GetDecryptedCredential`：

```go
// ResolveCredential 返回渠道在指定用途下应使用的明文密钥，是统一凭证模型的单一出口。
// 取 active 状态、优先级最高的一条；env 走 os.Getenv，direct 走 crypto.Decrypt。
func (s *LLMProxyService) ResolveCredential(channelID uint, purpose string) (string, error) {
    creds, err := s.credentialRepo.ListByChannel(channelID) // 已存在
    if err != nil { return "", fmt.Errorf("list credentials: %w", err) }
    for _, c := range creds {
        if c.Purpose != purpose || c.Status != "active" { continue }
        switch c.Source {
        case "env":
            if v := os.Getenv(c.EnvVarName); v != "" { return v, nil }
            return "", fmt.Errorf("env var %q empty for channel %d", c.EnvVarName, channelID)
        case "direct":
            return s.credentialRepo.GetDecrypted(c.ID) // ← 死代码在此被接通
        }
    }
    return "", nil // 无凭证：调用方决定是否回退/告警
}
```

> 多条 active 的取舍（主备/轮换）先做"取首条"，排序/轮换列为开放问题 §7-Q3。

### 3.4 接通转发路径

改 `dbChannelToConfig`（[service:766-806](internal/service/llm_proxy.go#L766-L806)）：把 767-792 的 `APIKeyEnv` 内联解析换成 `ResolveCredential(ch.ID, "api")`。

- **兼容回退**：`ResolveCredential` 返回空 + 该渠道仍有旧 `APIKeyEnv` 时，回退到旧逻辑（迁移期保命，打 deprecation 日志）。迁移（§3.6）完成且观察期过后，由 Phase B5 删除回退。
- 此函数在 `Reload()`（[service:733](internal/service/llm_proxy.go#L733)）路径上被调用，凭证改动经 `POST /api/llm/reload`（[router:176](internal/router/router.go#L176)）即时生效，无需重启。

### 3.5 接通余额路径

改 `registerBalanceProviders`（[service:285-297](internal/service/llm_proxy.go#L285-L297)）：

- `apiKey` 改用 `ResolveCredential(ch.ID, "balance")`，取不到再回退 `ResolveCredential(ch.ID, "api")`（很多 provider 余额查询复用 api key），仍空才回退旧 `ch.Config.APIKey`。
- `extraConfig` 维持 `BalanceConfigJSON`，但**仅承载非密参数**（如 `new_api_user` ID、region）；密文部分（session/ak-secret）迁入 `balance/direct` 凭证。`Register` 签名不变（[manager.go:34](internal/llm/balance/manager.go#L34)）。

### 3.6 数据迁移（幂等，随 AutoMigrate 后跑一次）

新增迁移函数（在 [db.go](internal/model/db.go) AutoMigrate 之后调用，或独立 `migrateChannelCredentials`），对每个现存渠道：

```
若该渠道无 purpose=api 的凭证：
  读 channel.APIKeyEnv：
    looksLikeEnvVar(APIKeyEnv)  → 建 {purpose:api, source:env,    EnvVarName:APIKeyEnv,        IsPreset:true}
    否则(明文)                   → 建 {purpose:api, source:direct, CredentialJSON:enc(APIKeyEnv), IsPreset:true}
  解析 BalanceConfigJSON 中的密文字段(session/ak-secret/...)：
    若存在 → 建 {purpose:balance, source:direct, ...}，并从 BalanceConfigJSON 移除密文键(只留非密参数)
```

- **幂等键**：以"该渠道已有 `purpose=api` 凭证"为跳过条件，可重复执行（呼应 `CLAUDE.md` 对幂等的偏好）。
- `looksLikeEnvVar` 复用现有 [service:899](internal/service/llm_proxy.go#L899)。
- 迁移后**不立即删** `APIKeyEnv` 列（保命回退），由 Phase B5 在观察期后清理。

### 3.7 加密前提（落地前置，勿跳过）

- 确认 keeper 的 `.env` 配置了 `BELLKEEPER_CREDENTIAL_KEY`（否则 `direct` 凭证明文落库，[aesgcm.go:3-4](internal/pkg/crypto/aesgcm.go#L3-L4)）。
- 新增/确认该环境变量需**同步到 `bellkeeper-init.sh` 的 `export`**（`CLAUDE.md §2.6` 硬性要求）与 SilkSpool 的 compose 模板，改完 `spool sync push keeper`。
- 变更后端配置/字段经 `spool` 部署（`CLAUDE.md §4.2`）：改 .env 走 `sync push + restart bellkeeper`；改代码走 `spool bundle keeper up keeper`。

---

## 4. 页面架构收敛（10 → 5 = 主线 B）

### 4.1 收敛映射

| 新页面（5） | 吸收的旧页面（10） | 展示 / 配置职责 |
|------------|-------------------|----------------|
| **① 总览** | LLMOverview | 入口仪表盘：渠道健康 / 成本余额 / 模型组 / 告警 四类卡片，**每张可下钻**到对应页 |
| **② 渠道** | LLMChannels（运行态）+ LLMConfig 的渠道&凭证部分 | 展示渠道卡（健康/令牌桶/熔断/成功率）**＋ 卡上就地编辑** ＋ **统一凭证区** ＋ 补齐 tier/task_types/balance_* |
| **③ 模型组与路由** | LLMGroups + LLMPools + LLMConfig 的模型组部分 | 组状态/CRUD ＋ 任务分层路由 ＋ 编码策略 ＋ 会话粘性 |
| **④ 用量与计费** | LLMBilling + LLMPricing + LLMTokens | 用量统计展示 ＋ 定价配置就地 ＋ 下游 Token 管理 |
| **⑤ 日志与告警** | LLMLogs + LLMAlerts | 调用日志 ＋ 告警事件（均为可观测，只读） |

**「配置」大杂烩页和「池子路由」页被消解内融**——这正是"以展示为主体、配置就地"。导航从 10 项降到 5 项。

> ⚠️ 注意边界：[web/src/pages/MatrixChannels.tsx](web/src/pages/MatrixChannels.tsx) 是 **Matrix 通知渠道**，与 LLM 无关，本次重构不涉及，勿误并。

### 4.2 导航收敛（[Layout.tsx:243-260](web/src/components/Layout.tsx#L243-L260)）

```
重构前 (10 项 / 4 节)              重构后 (5 项 / 1 节)
├─ 运行时                          LLM Proxy
│   ├─ 总览                         ├─ 总览
│   ├─ 渠道                         ├─ 渠道          (运行态+配置+凭证)
│   ├─ 模型组                       ├─ 模型组与路由   (组+池+策略+粘性)
│   └─ 调用日志                     ├─ 用量与计费     (用量+定价+Token)
├─ 配置                            └─ 日志与告警     (日志+告警)
│   ├─ 配置  ← 删除(职责拆分内融)
│   ├─ 池子路由 ← 并入"模型组与路由"
│   ├─ Token  ← 并入"用量与计费"
│   └─ 定价   ← 并入"用量与计费"
├─ 计费   ← 并入"用量与计费"
└─ 告警   ← 并入"日志与告警"
```

### 4.3 线框：渠道页（重构核心 —— 展示 + 就地配置 + 统一凭证）

```
┌─ 渠道 ─────────────────────────────────────────────────[ + 新建渠道 ]┐
│                                                                       │
│ ┌─ kimi-code ───────────────────────[● 健康]──[编辑]──[凭证 2]──[⋮]┐ │
│ │ base_url  https://api.moonshot.cn/v1        provider  anthropic   │ │
│ │ tier  premium   tasks [coding][analysis]    优先级 10   免费 ✗     │ │  ← tier/tasks 现在可见可改(问题C修复)
│ │ ───────────────────────────────────────────────────────────────  │ │
│ │ 令牌桶 ▓▓▓▓▓▓▓░░ 70/100   今日 1.2k/5k   RPM 60   成功率 99.2%    │ │  ← 运行态(原展示页)
│ │ 熔断器 [关闭]  最近错误 —                          [重置熔断器]    │ │
│ │ ───────────────────────────────────────────────────────────────  │ │
│ │ 凭证                                                              │ │  ← 统一凭证区(替代独立Modal)
│ │  • [API]   env  $LLM_KIMI_CODE_API_KEY   ✓已解析  [预置]  [改][×] │ │
│ │  • [余额]  直填 abcd…wxyz                 active            [改][×] │ │
│ │                                              [ + 添加凭证 ]       │ │
│ └───────────────────────────────────────────────────────────────────┘ │
│ ┌─ deepseek ────────────────────────[● 健康]──[编辑]──[凭证 1]──[⋮]┐ │
│ │ …                                                                  │ │
└───────────────────────────────────────────────────────────────────────┘

「+ 添加凭证 / 改」弹出统一凭证表单(替代今天的二选一困惑):
┌─ 凭证 ───────────────────────────────┐
│ 用途   ( ) 转发(api)   ( ) 余额(balance) │
│ 来源   ( ) 环境变量名   ( ) 直接粘贴      │
│ ┌─ env  ─────────────────────────────┐ │
│ │ 变量名  LLM_KIMI_CODE_API_KEY        │ │   ← source=env：填变量名,后端 os.Getenv
│ └────────────────────────────────────┘ │
│ ┌─ direct ───────────────────────────┐ │
│ │ 密钥/JSON  ************************   │ │   ← source=direct：AES 加密落库
│ └────────────────────────────────────┘ │
│                         [取消]  [保存]   │
└──────────────────────────────────────┘
```

**关键**：渠道行不再有"编辑填 api_key_env / 凭证按钮另填密文"的二元割裂（问题 A 消除）；密钥统一进凭证区，用途+来源一目了然。

### 4.4 线框：其余四页（保留卡片基调，下钻互链）

```
① 总览                            ③ 模型组与路由                  ④ 用量与计费              ⑤ 日志与告警
┌──────┬──────┐                  ┌─ 组: pool-chat ──────┐        [日][周][月]          [全部][熔断][配额][余额]
│渠道  │成本  │ ←卡片下钻         │ 成员 A→B→C  粘性 5m   │        ┌────┬────┬────┐      ┌──────────────────┐
│健康  │余额  │                  │ [编辑成员][清粘性]    │        │请求│Token│成本│      │ time chan model … │
├──────┼──────┤                  ├─ 任务分层路由 ───────┤        └────┴────┴────┘      │ …(虚拟滚动)       │
│模型组│告警  │                  │ coding→premium 兜free │        定价表(就地编辑)        └──────────────────┘
└──────┴──────┘                  │ 编码策略 [balanced▾]  │        Token CRUD
                                  └──────────────────────┘
```

---

## 5. 实施计划（后端先行，分阶段，小步提交）

> 每个子阶段 = 一次可验收提交，`go build ./... && go vet ./...` 全绿（动前端则 `cd web && pnpm build` 绿）；遵守 `CLAUDE.md §2.8` 提交纪律与 §5 检查点。

### Phase B — 后端（必须先行）

| 步 | 内容 | 关键文件 | 自验 |
|----|------|----------|------|
| **B0** | 配置 `BELLKEEPER_CREDENTIAL_KEY`，同步 `bellkeeper-init.sh` + compose 模板，`spool sync push keeper` | `.env` / `bellkeeper-init.sh` / SilkSpool 模板 | `spool exec keeper "env \| grep CREDENTIAL_KEY"` 非空 |
| **B1** | 扩展 `LLMChannelCredential` 4 字段；`ChannelCredentialView` 带出新字段 | [model/llm_channel_credential.go](internal/model/llm_channel_credential.go)、[service/llm_credential.go:14](internal/service/llm_credential.go#L14) | AutoMigrate 加列成功；GET 凭证返回新字段 |
| **B2** | 新增 `ResolveCredential`；接通转发 `dbChannelToConfig`（带 `APIKeyEnv` 回退） | [service/llm_proxy.go:766](internal/service/llm_proxy.go#L766) | 单测：env/direct 各解析正确；回退生效 |
| **B3** | 接通余额 `registerBalanceProviders`（balance→api→旧值 三级回退） | [service/llm_proxy.go:285](internal/service/llm_proxy.go#L285) | 余额查询用新凭证仍成功 |
| **B4** | 凭证 CRUD 端点请求体扩展 `purpose/source/env_var_name/label`；handler 透传 | [handler/llm_proxy.go:349-410](internal/handler/llm_proxy.go#L349-L410) | POST/PUT 各 source 落库正确 |
| **B5** | 幂等迁移 `migrateChannelCredentials`（§3.6）；观察期后删 `APIKeyEnv` 内联回退 | [model/db.go](internal/model/db.go) | 重复跑不产生重复行；老渠道转发不变 |

**死代码验收（呼应问题 B）**：B2/B3 后，全仓库 `grep -rn 'ResolveCredential\|GetDecrypted'` 必须显示 `GetDecrypted` 有了真实调用方（经 `ResolveCredential`），即死代码被消灭。

### Phase F — 前端（B 稳定后）

| 步 | 内容 | 关键文件 |
|----|------|----------|
| **F1** | 渠道表单补齐 `tier/task_types/balance_provider_type/balance_config_json/model_rpm_overrides` 编辑控件（问题 C） | [LLMConfig.tsx](web/src/pages/llm/LLMConfig.tsx) → 迁入新「渠道」页 |
| **F2** | 统一凭证区组件：用途×来源表单，列表展示 `purpose/source/env_var_name/preview/is_preset`；调扩展后的凭证端点 | 新组件 + [api/index.ts:328-334](web/src/api/index.ts#L328-L334)、[types/index.ts:460](web/src/types/index.ts#L460) |
| **F3** | 「渠道」页合并：LLMChannels 运行态 + 就地编辑 + F2 凭证区（问题 A、D） | [LLMChannels.tsx](web/src/pages/llm/LLMChannels.tsx) ← [LLMConfig.tsx](web/src/pages/llm/LLMConfig.tsx) 渠道部分 |
| **F4** | 「模型组与路由」合并（Groups+Pools+Config 组部分）；「用量与计费」合并（Billing+Pricing+Tokens）；「日志与告警」合并 | 对应页面 |
| **F5** | 导航 10→5（§4.2）；删除 LLMConfig 大杂烩页与「池子路由」入口；总览卡片下钻互链 | [Layout.tsx:243-260](web/src/components/Layout.tsx#L243-L260)、[LLMOverview.tsx](web/src/pages/llm/LLMOverview.tsx) |
| **F6** | 清理 `pages/llm/index.ts` 导出与路由；`pnpm build` 绿，grep 验证无死组件 | [pages/llm/index.ts](web/src/pages/llm/index.ts) |

---

## 6. 验收标准（逐项过，呼应 `CLAUDE.md §2.7 / §3`）

```
□ go build ./...  /  go vet ./...  /  cd web && pnpm build  全绿
□ 死代码已消灭：GetDecrypted 经 ResolveCredential 被转发+余额真实调用(grep 证明调用方>0)
□ 数据链路完整(写+存+读+用)：UI 存凭证 → AES 落库 → ResolveCredential 解出 → 转发上游真的带上它
□ 统一入口：渠道页不再存在"api_key_env 单填" 与 "独立凭证 Modal" 二选一(问题A 消除)
□ 渠道表单可编辑 tier/task_types/balance_*/model_rpm_overrides(问题C 消除)
□ 展示配置就地：在渠道卡上既看运行态又能改配置,无需跳页(问题D 消除)
□ 导航 10→5,LLMConfig 大杂烩页与池子路由入口已删,无残留死组件/死路由
□ 迁移幂等：重复执行不产生重复凭证行；老渠道(仅有 APIKeyEnv)转发行为不回归
□ 安全：BELLKEEPER_CREDENTIAL_KEY 已配置且同步 init 脚本；direct 凭证密文落库;CredentialJSON 不出 API
□ 卡片风格在总览/渠道/模型组延续强化,非推倒重来
□ 无 not implemented / 暂未实现 / 硬编码占位;无裸 SQL;无跨层调用
```

---

## 7. 待确认的开放问题（实施前请用户拍板）

- **Q1 页面分组**：§4.1 把 Token+定价+计费合为「用量与计费」、日志+告警合为「日志与告警」。是否认可？或偏好把"告警"独立保留为第 6 页（告警是运维高频入口）？
- **Q2 `APIKeyEnv` 列去留**：迁移后是保留该列作长期兼容（YAML seed 仍写它），还是 Phase B5 彻底删除、seed 也改为直接写凭证表？倾向**保留列、seed 经迁移函数转凭证**，兼顾平滑。
- **Q3 多凭证策略**：一个渠道多条 `purpose=api` active 凭证时，`ResolveCredential` 取首条即可，还是需要优先级/轮换/主备故障转移？MVP 建议取首条，轮换列为后续增强。
- **Q4 `BalanceConfigJSON` 拆分粒度**：余额密文迁入凭证表后，`BalanceConfigJSON` 只留非密参数。是否需要为每个 balance provider 明确"哪些键是密、哪些非密"的 schema？还是迁移时按已知敏感键名（session/ak_secret/...）启发式拆分？
- **Q5 落地范围**：本次是否两条主线（A 凭证 + B 收敛）一起做，还是先 A（后端凭证统一，前端仅最小适配）、B（页面收敛）分两个里程碑提交，以契合 §5 检查点防溢出？

---

## 8. 受影响文件速查

**后端**
- [internal/model/llm_channel_credential.go](internal/model/llm_channel_credential.go) — 扩展 4 字段（B1）
- [internal/model/db.go](internal/model/db.go) — 迁移函数（B5）
- [internal/service/llm_credential.go](internal/service/llm_credential.go) — View 带出新字段（B1）
- [internal/service/llm_proxy.go](internal/service/llm_proxy.go) — `ResolveCredential` 新增 + 接通转发 766 / 余额 285（B2/B3）
- [internal/handler/llm_proxy.go](internal/handler/llm_proxy.go) — 凭证 CRUD 请求体扩展 349-410（B4）
- [internal/repository/llm_channel_credential.go](internal/repository/llm_channel_credential.go) — 复用现有，`GetDecrypted` 被接通
- [internal/pkg/crypto/aesgcm.go](internal/pkg/crypto/aesgcm.go) — 不改，但依赖 `BELLKEEPER_CREDENTIAL_KEY`（B0）
- `bellkeeper-init.sh` + SilkSpool compose 模板 — 同步环境变量（B0）

**前端**
- [web/src/components/Layout.tsx](web/src/components/Layout.tsx) — 导航 10→5（F5）
- [web/src/pages/llm/LLMChannels.tsx](web/src/pages/llm/LLMChannels.tsx) — 升级为渠道主页（F3）
- [web/src/pages/llm/LLMConfig.tsx](web/src/pages/llm/LLMConfig.tsx) — 拆分内融后删除（F1/F3/F4）
- LLMGroups / LLMPools / LLMBilling / LLMPricing / LLMTokens / LLMLogs / LLMAlerts — 按 §4.1 合并（F4）
- [web/src/pages/llm/LLMOverview.tsx](web/src/pages/llm/LLMOverview.tsx) — 卡片下钻互链（F5）
- [web/src/types/index.ts](web/src/types/index.ts) — `LLMChannelCredentialView` 加 `purpose/source/env_var_name/is_preset/label`（F2）
- [web/src/api/index.ts](web/src/api/index.ts) — 凭证 create/update 请求体加新字段（F2）
- [web/src/pages/llm/index.ts](web/src/pages/llm/index.ts) — 清理导出（F6）

---

*落笔 2026-06-07。锚点会漂移，改前先 grep 校准；以代码为准。*
