# Bellkeeper

SilkSpool 的知识治理中台 + LLM 代理网关 + Matrix 控制平面。做 n8n 做不好的有状态工作：长连接、持久化队列、分类去重、检索、文件治理。

## Language

**知识管线**:
从 URL 入库到 Meilisearch 可检索的全链路，含爬取、提取、分类、入库、索引
_Avoid_: 知识流程、入库管线

**入库**:
URL/内容经去重、提取、分类后落盘为 Markdown 并写入 DB 元数据的动作
_Avoid_: 导入、导入库

**知识层**:
raw / archive / vault 三级存储分层，决定文件是否进 Meili 索引和 Obsidian 同步
_Avoid_: 存储层、文件层

**策展**:
PKB 的打分→分流→重构→摘要批处理流程
_Avoid_: 整理、治理

**渠道**:
LLM API 的一个具体提供商端点（如 DashScope、DeepSeek），含凭证和模型列表
_Avoid_: provider、供应商

**模型组**:
虚拟模型名（如 pool-chat-free），按策略解析到具体渠道的模型
_Avoid_: 虚拟模型、模型池

**会话粘性**:
同一 ConversationID 的请求绑定到同一渠道以利用 prompt cache
_Avoid_: 粘性路由、会话绑定

**限流学习**:
基于 429 响应自适应调整渠道 RPM 的机制
_Avoid_: 自适应限流、限流适配

**命令路由**:
Matrix 消息的 `!command` 前缀解析与权限检查后的分发
_Avoid_: 命令分发、命令处理

**通知网关**:
NATS 队列 → Worker → Matrix 消息投递的管道
_Avoid_: 通知管道、消息网关

**工作流漂移**:
本地 `internal/n8n_workflows/*.json` 与 n8n 运行态的工作流定义不一致
_Avoid_: 工作流不同步、定义偏差

**心跳**:
Go 后台服务定期写 activity_logs 的存活信号（module=heartbeat），超过 15 分钟无心跳视为异常
_Avoid_: 存活检测、健康探针

**错误传播**:
n8n 节点失败后错误是否向上传播到工作流整体 error 状态；加 onError 则吞错误（不传播），不加则传播（触发 B02）
_Avoid_: 错误冒泡、失败透传
