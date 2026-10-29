# Windows 从零落地单 Vault 指南

> 2026-10-30：现有两台电脑已经加入同一个 Notes 库，不执行本篇从零建库流程。现有目录有 `10-RAW`、`20-PKB` 等，本文的 `RAW/WORKING/KNOWLEDGE` 是旧新建库示例，不要求迁移现有目录。Knowledge 自动入口为 `20-PKB/Bellkeeper/`，见 [LiveSync 清单](livesync-checklist.md)。
>
> 新建空库时的目标：在 Windows 上直接创建一个可用于 Obsidian + LiveSync 的真实 Vault，并让它成为可独立运转的 PKB / Working 前端。

---

## 最终形态

你只需要一个真实 Vault：

```text
D:\Notes\obsidian-note
```

内部分五个顶层分区：
- `00-Inbox/`：所有新笔记先落这里
- `RAW/`：导入后的原料笔记、来源摘录、原始实验记录
- `WORKING/`：AI 整理稿、阶段总结、待沉淀内容
- `KNOWLEDGE/`：沉淀区，整理后的稳定知识
- `Daily/`：日记/周记/月记

此外还应准备一个**不属于 Vault** 的本地导入缓冲区：

```text
D:\SilkSpoolImport\
├── raw\
└── working\
```

它只负责承接从 TrueNAS 同步下来的文件资产，随后由统一 PowerShell 脚本把内容更新到 Vault `RAW/` / `WORKING/`。

---

## Step 1：运行目录创建脚本

Windows 侧文件编排现在已独立为 `SilkFiles` 项目。创建 Vault 目录时，应从 `SilkFiles/files.ps1` 进入，而不是继续使用 `doc/knowledge-base/setup/` 下的脚本副本。

在 `SilkFiles` 项目根目录执行：

```powershell
Set-ExecutionPolicy -Scope Process -ExecutionPolicy Bypass
.\files.ps1 create-vault-dirs
```

也可以按需要修改 `SilkFiles/bin/create-vault-dirs.ps1` 顶部的 `$vault` 变量指定路径。

然后手动创建本地导入缓冲区：

```powershell
New-Item -ItemType Directory -Force -Path "D:\SilkSpoolImport\raw" | Out-Null
New-Item -ItemType Directory -Force -Path "D:\SilkSpoolImport\working" | Out-Null
```

如果你准备把 `SilkFiles` 固定在某个 Windows 目录，也建议同时创建例如：

```text
D:\Scripts\SilkFiles\
```

---

## Step 2：用 Obsidian 打开

1. 安装并打开 Obsidian
2. 在欢迎页选择 **Open folder as vault**
3. 选择 `D:\Notes\obsidian-note`
4. 之后只使用这一个 Vault

---

## Step 3：基础设置

### 附件目录

设置 → Files and links → Default location for new attachments
- 选择：**In the folder specified below**
- 填：`KNOWLEDGE/Attachments`

### 模板目录（Templater 插件）

1. 安装社区插件 **Templater**
2. Template folder location 填：`KNOWLEDGE/Templates`

### 新笔记默认位置

建议设置为 `00-Inbox`，或直接手动放到对应目录。

---

## Step 4：复制模板

把以下模板复制到 `D:\Notes\obsidian-note\KNOWLEDGE\Templates\`：

- `source-note.md`
- `evergreen-note.md`
- `lab-note.md`
- `programming-note.md`
- `weekly-review.md`

模板用途见 [templates/README.md](../templates/README.md)。

---

## Step 5：配置 LiveSync

见 [setup/livesync-checklist.md](livesync-checklist.md)。

---

## Step 6：接收服务器整理稿

现有 Vault 的 `20-PKB/Bellkeeper/` 由 prod 常驻服务经 LiveSync 自动接收。新电脑只需加入已有同步，不需要部署 Windows 导入脚本。

需要人工补导 NAS raw/working 中的其他资产时，再使用 [SilkFiles 补导说明](windows-knowledge-sync.md)，先 plan/dry-run。不要把补导任务配置成整理稿的第二条自动写入链路。

---

## Step 7：日常使用分流原则

### 放进 `00-Inbox/`
- 任何临时想法、快速记录
- 不确定放哪里的内容

### 放进 `RAW/`
- 文章摘录 → `RAW/Articles/`
- 漏洞通告 → `RAW/Advisories/`
- 实验原始记录 → `RAW/Lab-Notes/`
- 编程草稿 → `RAW/Programming-Drafts/`
- AI 生成内容 → `RAW/AI-Drafts/`

### 放进 `WORKING/`
- AI 整理稿 → `WORKING/Summaries/`
- 待提炼中间稿 → `WORKING/Distill/`
- 周整理或主题整理工作稿 → `WORKING/Reviews/`

### 放进 `KNOWLEDGE/`
- 已整理的永久笔记 → `KNOWLEDGE/Evergreen/`
- 最终版复现记录 → `KNOWLEDGE/Labs/`
- 操作手册 → `KNOWLEDGE/Playbooks/`
- 稳定编程知识 → `KNOWLEDGE/Programming/`

### 放进 `Daily/`
- 日记 → `Daily/Daily/`
- 周整理 → `Daily/Weekly/`
- 月复盘 → `Daily/Monthly/`

---

## 目录结构参考

完整设计见 [vaults/single-vault.md](../vaults/single-vault.md)。
