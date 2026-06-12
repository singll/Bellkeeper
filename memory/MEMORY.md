# Bellkeeper MEMORY

> 最后更新: 2026-06-12

## 当前进度

- **Phase 0-6**: 全部完成 ✅
- **Phase 7 (测试)**: Repository 全覆盖完成 ✅，其余子项进行中
- **Phase 8-10**: 待执行

## 关键决策

- 测试基础设施从 SQLite 切换到 **Docker PostgreSQL** (`bellkeeper-test-postgres`, `localhost:15432`, user=`bellkeeper`, password=`testpass`, db=`bellkeeper_test`)
- 共享 schema `repo_test`，每个测试前后 TRUNCATE 隔离
- 仅 3 个 LLMChannelCredential 方法因依赖 crypto 包而 skip
- `go build ./...` + `go vet ./...` 绿色
- `go test ./...` 全部通过（Repository 测试较慢，约 4-5 分钟）

## 下一步

1. Phase 7 剩余：核心链路测试质量审查 + LLM 协议转换测试 + 日报跳过测试补回
2. Phase 8: golangci-lint 安装 + error 清零
3. Phase 9-10: Matrix T5-T9 + 文档 + v1.0.0 tag

## 计划文件

- `/home/ubuntu/Bellkeeper/1.0-PROGRESS.md`（主要进度跟踪）

## 环境备忘

- Docker 已可用（ubuntu 用户已加入 docker 组）
- 测试 PG 容器：`docker ps` 应显示 `bellkeeper-test-postgres`
- 启动命令：`docker run -d --name bellkeeper-test-postgres -e POSTGRES_DB=bellkeeper_test -e POSTGRES_USER=bellkeeper -e POSTGRES_PASSWORD=testpass -p 15432:5432 postgres:16-alpine`