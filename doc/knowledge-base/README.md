# Obsidian / Markdown 知识主库

> 这里定义 SilkSpool 的**知识真相源**与个人整理工作界面。
> 当前口径不再把笔记系统理解为“RAGFlow 的上游”，而是理解为：**Markdown 文件为王，Obsidian 是主要前端之一，PKB 优先，自动化回流是可选增强。**
> Windows 手工补导入口在 `SilkFiles` 项目，整理稿自动发布由 prod 承担；本目录只负责知识结构、规则与操作边界，不再承载当前脚本真相源。

---

## 角色定位

在新的知识模型下，笔记系统主要承担：

- **Layer 3 / PKB** 的长期沉淀
- 必要时承接部分 **Layer 2 / Working Layer** 草稿整理
- 作为用户进行写作、复盘、提炼、归档的核心界面

它**不是**：

- 整个知识系统唯一入口
- 自动采集层的唯一落点
- RAG 平台的从属数据区

---

## 新的总体原则

### 1. 文件为王

真正稳定的知识主库是：

- Markdown / 文本文件
- 附件目录
- 可迁移、可备份、可被多种工具读取的目录结构

Obsidian 是主要编辑器之一，但不是唯一前端，也不是知识真相源本身。

### 2. 三层知识模型里，笔记主库更偏 Layer 3

```text
Layer 1  Raw Capture   原始采集层
Layer 2  Working Layer AI / 工作层
Layer 3  PKB           最终个人知识库
```

Obsidian / Markdown 在这里最核心的职责是：

- 承接最终保留的 Evergreen Notes
- 沉淀方案、经验、踩坑、实验结论、主题页
- 为长期复用服务，而不是承接所有自动采集原文

### 3. MVP-first

即使没有 LiveSync、CouchDB、RAGFlow、n8n，知识主库也应独立成立：

- 能写
- 能看
- 能整理
- 能人工归档

同步、检索、自动回流都属于后加能力，不应反向定义主库结构。

---

## 当前生产链路（2026-10-30）

两台电脑的现有 Obsidian 库已经通过加密 LiveSync 同步，CouchDB 使用 `https://couchdb.singll.net/obsidian-notes`。保留用户现有 `10-RAW`、`20-PKB`、`20-WORKING` 等目录，不按旧文档重新建库。

```text
资讯源 → Bellkeeper 抓取/去重/分类 → TrueNAS knowledge/raw
       → PKB 筛选和重构 → TrueNAS knowledge/vault
       → prod knowledge-sync → 20-PKB/Bellkeeper/
       → 官方 LiveSync CLI ↔ CouchDB ↔ 各终端 Obsidian
```

整理后的 Markdown 自动进入 `20-PKB/Bellkeeper/`，按原领域保留子目录。TrueNAS 的 `knowledge/obsidian` 保存服务器这一固定目录的局部 Vault；常驻 CLI 沿用现有内容加密设置，处理分块与同步协议。

新增和源更新自动发布，相同哈希不重写。终端已编辑或删除的笔记不会被发布器强制恢复/覆盖；遇到源更新冲突记录清单，原件继续保留。删除 Knowledge 原件也不会删除个人 Vault 中的笔记。

当前自动范围是 `knowledge/vault/**/*.md`。约 5.1 万篇 raw、archive、working 渠道日志、大附件、录像均留在 NAS。个人其他目录不会被这个服务器客户端落盘，也不会自动回流到 Bellkeeper 检索。

## 使用方式

日常只需打开现有 Obsidian，等待 LiveSync 同步，查看 `20-PKB/Bellkeeper/`。两台电脑无需重新初始化，也不需要 Win10/Win11 运行拉取脚本。新增终端使用已同步设备导出的 Setup URI 加入同一个库。

- [LiveSync 配置与现有库接入](setup/livesync-checklist.md)
- [服务器发布服务运维](../../../SilkSpool/bundles/knowledge-sync/README.md)
- [NFS 存储和录像挂载](../ops/NFS-SETUP.md)
- [Windows 手工补导其他资产](setup/windows-knowledge-sync.md)：旧 SilkFiles 路径仅作补导，不再是整理稿的日常自动入口。

CouchDB 是 LiveSync 私有同步后端。官方兼容客户端可以写入，普通文件脚本不能自造 LiveSync JSON。当前没有 K07 工作流，不按旧 `_changes` 直接读 Markdown 的设想接线。

## 文件与编辑边界

Markdown 和附件仍是可独立备份、可迁移的知识内容。个人 Vault 负责编辑和沉淀；TrueNAS `data/knowledge` 承载抓取原件、整理产出和文件资产；发布器把机器产出送进固定区域，不改变个人整理习惯。

`vaults/single-vault.md` 和从零 bootstrap 文档的 `D:\Notes\obsidian-note`、`RAW/WORKING/KNOWLEDGE` 为旧目录示例，不要求迁移当前 Notes 库。Windows 工具和环境仍放 `data/sync`，不要混入笔记库。

---

## 文件树

```text
doc/knowledge-base/
├── README.md                              # 本文件
├── setup/
│   ├── windows-vault-bootstrap.md         # Windows 从零落地指南（基于 SilkFiles）
│   ├── windows-knowledge-sync.md          # SilkFiles knowledge 入口规则
│   └── livesync-checklist.md              # LiveSync 配置清单
├── vaults/
│   └── single-vault.md                    # 单 Vault 目录设计与职责说明
├── standards/
│   ├── frontmatter.md                     # Frontmatter 规范（5 个必填字段）
│   ├── naming.md                          # 命名规范
│   └── sync-and-ignore-rules.md           # 同步边界与回流规则
├── templates/
│   ├── README.md                          # 模板使用说明
│   ├── source-note.md
│   ├── evergreen-note.md
│   ├── lab-note.md
│   ├── programming-note.md
│   ├── playbook-note.md
│   ├── project-note.md
│   ├── tool-card.md
│   ├── daily-note.md
│   ├── weekly-review.md
│   └── monthly-review.md
└── workflows/
    └── weekly-distill-sop.md              # 每周整理 SOP
```

---

## 阅读顺序

1. [vaults/single-vault.md](vaults/single-vault.md) — 理解单 Vault 目录结构和分区职责
2. [setup/windows-vault-bootstrap.md](setup/windows-vault-bootstrap.md) — Windows 首次部署
3. [setup/windows-knowledge-sync.md](setup/windows-knowledge-sync.md) — Windows 导入脚本规则与命令动作
4. [setup/livesync-checklist.md](setup/livesync-checklist.md) — 配置 LiveSync 同步
5. [standards/frontmatter.md](standards/frontmatter.md) — 笔记元数据规范
6. [standards/naming.md](standards/naming.md) — 命名规范
7. [standards/sync-and-ignore-rules.md](standards/sync-and-ignore-rules.md) — 同步边界与回流规则
8. [templates/README.md](templates/README.md) — 模板使用说明
9. [workflows/weekly-distill-sop.md](workflows/weekly-distill-sop.md) — 每周整理流程

---

## 当前原则

- 各终端加入同一个现有 Notes Vault；本地盘符/路径可以不同
- Vault 首先服务于**个人整理与长期沉淀**，而不是承接所有机器采集原文
- `00-Inbox` 可以继续作为人工写入入口，但不再把它等同于整个知识系统唯一入口
- LiveSync 只同步这一个 Vault，对应一个 CouchDB 数据库
- 是否把笔记再回流到检索系统，应视为可选增强，不应视为主库定义条件

---

## 与其他模块的边界

- [../ARCHITECTURE.md](../ARCHITECTURE.md) — 知识入库 / 检索 / 问答 / PKB 在 Bellkeeper 中的实现与数据流
- [../ops/NFS-SETUP.md](../ops/NFS-SETUP.md) — TrueNAS 经 NFS 承载附件 / raw 资产（不定义笔记结构）
