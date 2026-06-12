# golang-migrate 替代纯 AutoMigrate 管理数据库 schema

GORM AutoMigrate 只能加列/加表不能删，导致死字段（ParserID）和死表（crawl_sources）无法通过代码清理。引入 golang-migrate 做版本化迁移，AutoMigrate 降级为仅建新表。1.0 迁移文件覆盖：删 ParserID 列、删 crawl_sources 表、清 RAGFlow setting seed。
