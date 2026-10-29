# 同步边界与忽略规则

> 目标：明确哪些目录参与 LiveSync 同步，哪些目录可进入检索增强消费范围，哪些内容不应出现在 Vault 中。

---

## LiveSync 同步边界

各个人终端同步同一个已有 Vault，沿用现有客户端的忽略规则，不按旧文档重新创建目录。服务器的官方 CLI 是例外：只将 `20-PKB/Bellkeeper/` 落盘到 NAS，防止混入个人其他目录。

- 整理稿：`TrueNAS knowledge/vault → knowledge-sync → Vault 20-PKB/Bellkeeper/`，自动发布。
- 原始资产：raw、archive、working、附件和录像保留在 NAS，不全量送入 LiveSync。
- Windows 导入缓冲区不属于 Vault；[SilkFiles](../setup/windows-knowledge-sync.md) 只做其他资产的手动补导。
- CouchDB 是 LiveSync 私有后端；官方兼容客户端负责分块/加解密，外部脚本不得自造同步文档。


---

## 检索增强消费边界

### 推荐消费（高质量内容）

```text
KNOWLEDGE/Evergreen/
KNOWLEDGE/Labs/
KNOWLEDGE/Playbooks/
KNOWLEDGE/Programming/
```

### 不建议消费

```text
00-Inbox/                  # 未处理，质量不稳定
RAW/                       # 原料，半成品
WORKING/                   # 工作稿，中间态内容
KNOWLEDGE/Templates/       # 模板文件，无实质内容
KNOWLEDGE/Attachments/     # 附件，非文本
KNOWLEDGE/Projects/        # 项目决策记录，通常含敏感内容
Daily/                     # 日记，个人流水，不适合检索
```

---

## Obsidian 忽略建议

以下内容通过 `.obsidian/app.json` 的 `userIgnoreFilters` 或 LiveSync 的排除规则配置：

```text
.obsidian/workspace.json   # 窗口状态，不应同步
.obsidian/cache            # 本地缓存
D:\SilkSpoolImport\raw    # 本地导入缓冲区，不属于 Vault
D:\SilkSpoolImport\working
```

LiveSync 默认已排除 `.obsidian/workspace` 状态文件，其余 `.obsidian/` 配置建议参与同步以保持多设备一致。

---

## 不要放进 Vault 的内容

| 类型 | 推荐位置 |
|------|----------|
| PCAP 文件 | TrueNAS |
| 内存转储 | TrueNAS |
| 恶意样本 | 隔离存储 / 加密目录 |
| 大日志（>10MB） | TrueNAS / 本地归档 |
| 大型压缩包 | TrueNAS |
| 虚拟机镜像 | TrueNAS |
| 大型代码仓库 | Gitea / GitHub |
| 凭据和敏感明文 | 密码管理器 / 加密目录 |

---

## Sensitivity 字段与检索增强的关系

| sensitivity 值 | 是否进入检索增强消费 |
|---|---|
| `public` | 可以 |
| `normal` | 可以 |
| `internal` | 谨慎，建议不回流 |
| `restricted` | 不应回流 |

如果一篇笔记包含敏感内容，建议在 frontmatter 中标记 `sensitivity: restricted`，并在实际检索消费链路中过滤该值；当前没有 K07 工作流。

---

## 一句话原则

- 个人终端同步现有 Vault；服务器只接固定发布目录
- 检索增强只消费 `KNOWLEDGE/` 下高质量目录
- 敏感内容不进 Vault，极敏感内容标记 `restricted` 并在消费链路过滤
- Windows 导入缓冲区只做短期事务中转；成功导入后默认应被清理，避免形成重复实体文件
