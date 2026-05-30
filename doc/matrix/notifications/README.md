# Matrix 通知系统总览

> 本目录定义 Bellkeeper Matrix Bot Platform 中的通知平面（Notification Plane）。

---

## 定位

通知系统不是“发消息函数”的集合，而是平台化的 **Notification Gateway**：

- 接收多系统通知请求
- 按逻辑频道路由到 Matrix 房间
- 进行模板渲染、频控、去重、聚合、重试
- 记录完整发送审计

它是 SilkSpool 各系统对接 Matrix 的统一出口。

---

## 文档

1. [NOTIFICATION-MODEL.md](NOTIFICATION-MODEL.md)
   - 通知事件模型、逻辑频道模型、模板与路由语义
2. [NOTIFICATION-OPERATIONS.md](NOTIFICATION-OPERATIONS.md)
   - 通知系统运维、排障、重试、降噪与审计口径

---

## 使用原则

- 外部系统不直接调用 Matrix Send API
- 外部系统不直接持有 room id 作为主路由依据
- 统一通过 `POST /api/matrix/notify`
- 使用逻辑频道，如：`alerts` / `daily` / `todo` / `qa` / `ops`

---

## 一句话定义

**通知系统是 Bellkeeper Matrix Platform 的外向平面：它把“向 Matrix 发消息”升级为“可治理、可观测、可重试、可演进的基础设施能力”。**
