# Bellkeeper

Bellkeeper (钟守者) 是一个知识管理系统，用于采集、组织和检索信息。

## 特性

- 标签系统 - 统一管理知识分类
- 数据源管理 - 管理各类信息来源
- RSS 订阅 - 自动采集 RSS 内容
- Webhook 集成 - 触发和管理工作流
- RagFlow 集成 - 向量化知识库存储
- 智能路由 - 根据标签自动分配知识库

## 技术栈

- **后端**: Go 1.22+, Gin, GORM
- **数据库**: PostgreSQL 16
- **认证**: Authelia (Forward Auth)

## 快速开始

### 本地开发

```bash
# 安装依赖
make deps

# 运行数据库迁移
make migrate

# 启动服务
make run
```

### Docker 部署

```bash
# 创建 .env 文件
cp docker/.env.example docker/.env
# 编辑 .env 设置密码

# 启动服务
make docker-up

# 查看日志
make docker-logs
```

## API 文档

启动服务后访问: `http://localhost:8080/swagger/`

## 配置

配置文件: `config/bellkeeper.yaml`

支持环境变量覆盖，前缀 `BELLKEEPER_`，例如:
- `BELLKEEPER_SERVER_PORT=8080`
- `BELLKEEPER_DATABASE_PASSWORD=secret`

## 项目结构

```
bellkeeper/
├── cmd/bellkeeper/     # 入口
├── internal/
│   ├── config/         # 配置管理
│   ├── handler/        # HTTP 处理器
│   ├── service/        # 业务逻辑
│   ├── repository/     # 数据访问
│   ├── model/          # 数据模型
│   └── middleware/     # 中间件
├── config/             # 配置文件
├── docker/             # Docker 配置
└── migrations/         # 数据库迁移
```

## License

MIT
