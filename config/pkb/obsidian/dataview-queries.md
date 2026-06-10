# PKB Dataview 查询模板

> 将以下查询复制到 Obsidian 笔记中使用（需安装 Dataview 插件）。

## 1. 孤儿卡（无任何出入链的原子卡）

```dataview
TABLE file.folder AS 领域
FROM "vault"
WHERE type = "pkb_card"
  AND length(file.inlinks) = 0 AND length(file.outlinks) = 0
SORT file.mtime DESC
```

## 2. 重复概念候选（同 concept 多卡 / supplement 簇）

```dataview
TABLE rows.file.link AS 卡片簇
FROM "vault"
WHERE atomic_concept
GROUP BY atomic_concept
WHERE length(rows) > 1
```

## 3. 所有 supplement 卡片（按目标概念分组）

```dataview
TABLE supplement_to AS 补充目标
FROM "vault"
WHERE card_type = "supplement"
SORT supplement_to ASC
```

## 4. 超级 Hub（被链接最多的卡片）

```dataview
TABLE length(file.inlinks) AS 被链接数, file.folder AS 领域
FROM "vault"
WHERE type = "pkb_card"
SORT length(file.inlinks) DESC
LIMIT 20
```

## 5. 领域知识体系索引

```dataview
LIST
FROM "vault"
WHERE type = "pkb_map"
SORT domain ASC
```

## 6. 主题 MOC 列表

```dataview
TABLE parent AS 上级, length(file.outlinks) AS 包含卡片数
FROM "vault"
WHERE type = "pkb_topic"
SORT domain ASC
```
