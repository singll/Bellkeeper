INSERT INTO settings (key, value, value_type, category, description, is_secret, created_at, updated_at) VALUES
('ragflow.base_url', 'http://ragflow:9380', 'string', 'api', 'RagFlow Base URL', false, NOW(), NOW()),
('ragflow.api_key', '', 'string', 'api', 'RagFlow API Key', true, NOW(), NOW())
ON CONFLICT (key) DO NOTHING;

INSERT INTO matrix_commands (command_name, handler_type, permission_level, room_scope, is_active, handler_config, description, usage_example, created_at, updated_at) VALUES
('列表', 'memos_todo_list', 'user', 'any', true, '{}', '查看 Memos 待办列表', '!列表', NOW(), NOW()),
('新增', 'memos_todo_create', 'user', 'any', true, '{}', '创建 Memos 待办事项', '!新增 完成项目文档', NOW(), NOW())
ON CONFLICT (command_name) DO NOTHING;
