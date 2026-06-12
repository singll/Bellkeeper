# Matrix 命令模型

> 本文档定义 Bellkeeper Matrix 平台的命令语义、解析规则、权限边界与 handler 模型。

---

## 1. 命令平面目标

命令系统必须满足：
- 前缀命令仍为标准形态：`!` / `！`
- 可扩展别名
- 可做房间范围控制
- 可做角色权限控制
- 可路由到不同 handler / connector
- 可审计、可回放、可调试

---

## 2. 命令标准格式

```text
!命令 参数
!命令 子命令 参数
```

示例：
- `!帮助`
- `!列表`
- `!新增 买牛奶`
- `!问 RAGFlow 解析状态`
- `!notify test alerts`
- `!room bind alerts`

### 2.1 提及触发

提及触发只作为补充：
- `@bot:matrix.singll.net 帮助`

进入平台内部后应被标准化为：
- `!帮助`

### 2.2 不支持的形式

- 不使用 slash 命令
- 不依赖客户端特性实现指令

---

## 3. 命令拆解模型

建议解析后统一为：

```json
{
  "prefix": "!",
  "name": "问",
  "alias": "ask",
  "subcommand": "",
  "args": "RAGFlow 解析状态",
  "tokens": ["问", "RAGFlow", "解析状态"]
}
```

---

## 4. 命令分类

### 4.1 用户业务命令
- `帮助`
- `列表`
- `新增`
- `完成`
- `删除`
- `修改`
- `截止`
- `问`
- `搜`

### 4.2 管理命令
- `status`
- `health`
- `rooms`
- `commands`
- `routes`
- `audit`
- `whoami`

### 4.3 平台治理命令
- `notify test <channel>`
- `room bind <channel>`
- `policy check <command>`
- `connector list`

---

## 5. Handler 模型

每个命令注册项至少包括：
- `name`
- `aliases`
- `handler`
- `category`
- `allowed_rooms`
- `allowed_roles`
- `timeout_seconds`

handler 接口建议：

```text
Handle(ctx, commandContext) -> commandResult
```

`commandContext` 应包含：
- sender
- room
- raw message
- parsed command
- trace id
- permission snapshot

`commandResult` 应包含：
- reply.text
- reply.html
- side_effects
- metadata

---

## 6. 权限模型

### 6.1 基础角色
- `admin`
- `operator`
- `member`
- `readonly`

### 6.2 作用域
- 全局作用域
- 房间作用域
- 命令作用域
- 连接器作用域

### 6.3 示例规则
- `!问`：允许 `member+`，仅限 `qa` 房间
- `!列表`：允许 `member+`，仅限 `todo` 房间
- `!notify test`：允许 `operator+`，仅限 `admin` 房间
- `!policy check`：允许 `admin`

---

## 7. 房间语义

建议房间按用途分型：
- `admin`
- `qa`
- `todo`
- `alerts`
- `daily`
- `infra`

命令应默认绑定到房间语义，而不是直接依赖具体 room id。

---

## 8. 回复模型

### 8.1 同步短回复
适用于：
- 帮助
- 列表
- 简单查询

### 8.2 异步任务回复
适用于：
- 耗时问答
- workflow 触发
- 复杂运维操作

模式：
1. 先回复“已接收，处理中”
2. 完成后再由 notify gateway 发最终结果

---

## 9. 命令治理原则

- 命令名要稳定
- 别名可以增加，但不要频繁变更主命令
- 不同 handler 不要直接写 Matrix 发送逻辑
- 所有回复统一回流到 notify gateway
- 所有命令执行必须有审计记录

---

## 10. 推荐首批命令集

### 基础命令
- `!帮助`
- `!whoami`
- `!status`
- `!commands`

### Todo
- `!列表`
- `!新增`
- `!完成`
- `!删除`

### QA
- `!问`
- `!搜`

### 管理
- `!rooms`
- `!routes`
- `!notify test`
- `!audit recent`

这套命令足以构成首个正式平台版本的控制面与日常入口。
