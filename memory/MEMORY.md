# Bellkeeper MEMORY

> 最后更新: 2026-06-12

## 当前进度

- **Phase 0-8**: 全部完成 ✅
- **Phase 9-10 (T5-T8)**: 全部完成 ✅
  - T5: Agent MVP — llmclient tools 扩展 + AgentService + 6只读工具 + 会话 + 限速
  - T6: Agent 扩展 — todo写工具 + workflow触发 + 权限护栏
  - T7: 后端API补齐 — PUT rooms, POST/DELETE channels, PUT commands, POST notifications/retry
  - T8: 前端API对齐（pnpm build 绿）
- **Phase 9-10 (T9+收尾)**: 待执行
  - T9: 文档与收尾
  - doc/ 进版本库
  - CLAUDE.md 瘦身
  - v1.0.0 tag

## 关键决策

- Agent 通过 llmclient.ChatCompletionFull 支持 function calling（OpenAI schema 直通 LLM Proxy）
- 工具权限分级：readonly（所有人）/ write（admin）/ danger（需确认，T2 实现）
- 会话存 Redis（matrix:agent:session:<roomID>），TTL 30min，上限 20 条
- 限速每房间 30 回合/小时（Redis INCR + 1h 窗口）
- 命令消息（!开头）走 Router，普通消息走 Agent
- Agent 工具调用写 matrix_command_logs（handler_type=agent_tool）
- Memos todo 工具直接走 HTTP API（不走 CommandService）

## 下一步

1. T9 文档与收尾 + v1.0.0 tag
2. 可选：前端 7→3 页重构

## 计划文件

- `/home/ubuntu/Bellkeeper/1.0-PROGRESS.md`（主要进度跟踪）

## 环境备忘

- Docker 已可用（ubuntu 用户已加入 docker 组）
- 测试 PG 容器：`docker ps` 应显示 `bellkeeper-test-postgres`
- 启动命令：`docker run -d --name bellkeeper-test-postgres -e POSTGRES_DB=bellkeeper_test -e POSTGRES_USER=bellkeeper -e POSTGRES_PASSWORD=testpass -p 15432:5432 postgres:16-alpine`
- Agent 配置项：`matrix.agent.enabled/model/max_turns_per_hour/session_ttl/max_tool_iterations/system_prompt`
- bellkeeper-init.sh 已新增 Agent 环境变量导出