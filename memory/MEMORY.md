# Bellkeeper MEMORY

> 最后更新: 2026-06-12

## 当前进度

- **v1.0.0**: 已打 tag ✅
- **提示词 P0 修复**: ✅ 全部完成(6项)
  - P0-1: estimateTokens 中文低估修复(rune/2→rune*2)
  - P0-2: knowledge_ask 已是 rune 截断(前序修复确认)
  - P0-3: digest 阈值已通过 YAML 配置可调(前序修复确认)
  - P0-4: rule_optimizer 默认改 pool-summary + 注册 TaskRuleGeneration
  - P0-5: Anthropic tool_choice 仅在 tools 非空时设置
  - P0-6: max_tokens 注释修正 + stripJSONFence/stripCardFence 统一为 textutil.StripFence

## 关键决策

- Agent 通过 llmclient.ChatCompletionFull 支持 function calling（OpenAI schema 直通 LLM Proxy）
- 工具权限分级：readonly（所有人）/ write（admin）/ danger（需确认，T2 实现）
- 会话存 Redis（matrix:agent:session:<roomID>），TTL 30min，上限 20 条
- 限速每房间 30 回合/小时（Redis INCR + 1h 窗口）
- 命令消息（!开头）走 Router，普通消息走 Agent
- Agent 工具调用写 matrix_command_logs（handler_type=agent_tool）
- Memos todo 工具直接走 HTTP API（不走 CommandService）
- estimateTokens: CJK(avgBytesPerRune≥3)→rune*2, 否则 byteLen/4

## 下一步

1. ROADMAP P0: PKB 存量批跑 + cron 固化
2. ROADMAP P1: 提示词基础设施(response_format/golden set/system-user分离)
3. ROADMAP P1: PKB 原子知识网 Phase A–E
4. 可选：前端 7→3 页重构

## 计划文件

- `/home/ubuntu/Bellkeeper/1.0-PROGRESS.md`（主要进度跟踪）

## 环境备忘

- Docker 已可用（ubuntu 用户已加入 docker 组）
- 测试 PG 容器：`docker ps` 应显示 `bellkeeper-test-postgres`
- 启动命令：`docker run -d --name bellkeeper-test-postgres -e POSTGRES_DB=bellkeeper_test -e POSTGRES_USER=bellkeeper -e POSTGRES_PASSWORD=testpass -p 15432:5432 postgres:16-alpine`
- Agent 配置项：`matrix.agent.enabled/model/max_turns_per_hour/session_ttl/max_tool_iterations/system_prompt`
- bellkeeper-init.sh 已新增 Agent 环境变量导出
