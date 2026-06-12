DELETE FROM settings WHERE key LIKE 'ragflow.%';
DELETE FROM matrix_commands WHERE handler_type IN ('ragflow_qa', 'ragflow_search', 'memos_todo_list', 'memos_todo_create');
