package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/singll/bellkeeper/internal/llmclient"
)

type TodoManager interface {
	ListTodos(ctx context.Context) (interface{}, error)
	AddTodo(ctx context.Context, content string) (interface{}, error)
	CompleteTodo(ctx context.Context, id int) error
}

type WorkflowTriggerer interface {
	TriggerWorkflow(ctx context.Context, name string, payload map[string]interface{}) (interface{}, error)
}

type WriteToolDependencies struct {
	TodoMgr    TodoManager
	Workflow   WorkflowTriggerer
}

func RegisterWriteTools(registry *ToolRegistry, deps WriteToolDependencies) {
	if deps.TodoMgr != nil {
		registerTodoTools(registry, deps)
	}
	if deps.Workflow != nil {
		registerWorkflowTools(registry, deps)
	}
}

func registerTodoTools(registry *ToolRegistry, deps WriteToolDependencies) {
	registry.Register(&ToolDefinition{
		Level: LevelReadonly,
		Tool: llmclient.Tool{
			Type: "function",
			Function: llmclient.Function{
				Name:        "todo_list",
				Description: "查看待办事项列表",
				Parameters:  jsonSchema(map[string]interface{}{}, nil),
			},
		},
		Handler: func(ctx context.Context, _ json.RawMessage) (string, error) {
			result, err := deps.TodoMgr.ListTodos(ctx)
			if err != nil {
				return "", fmt.Errorf("list todos: %w", err)
			}
			data, err := json.Marshal(result)
			if err != nil {
				return "", fmt.Errorf("marshal result: %w", err)
			}
			return string(data), nil
		},
	})

	registry.Register(&ToolDefinition{
		Level: LevelWrite,
		Tool: llmclient.Tool{
			Type: "function",
			Function: llmclient.Function{
				Name:        "todo_add",
				Description: "添加一条待办事项",
				Parameters: jsonSchema(map[string]interface{}{
					"content": map[string]interface{}{"type": "string", "description": "待办内容，支持 todo.txt 格式如 P1 D4/20 内容 +项目 @上下文"},
				}, []string{"content"}),
			},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (string, error) {
			var params struct {
				Content string `json:"content"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return "", fmt.Errorf("parse args: %w", err)
			}
			result, err := deps.TodoMgr.AddTodo(ctx, params.Content)
			if err != nil {
				return "", fmt.Errorf("add todo: %w", err)
			}
			data, err := json.Marshal(result)
			if err != nil {
				return "", fmt.Errorf("marshal result: %w", err)
			}
			return string(data), nil
		},
	})

	registry.Register(&ToolDefinition{
		Level: LevelWrite,
		Tool: llmclient.Tool{
			Type: "function",
			Function: llmclient.Function{
				Name:        "todo_done",
				Description: "标记待办事项为已完成",
				Parameters: jsonSchema(map[string]interface{}{
					"id": map[string]interface{}{"type": "integer", "description": "待办事项ID"},
				}, []string{"id"}),
			},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (string, error) {
			var params struct {
				ID int `json:"id"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return "", fmt.Errorf("parse args: %w", err)
			}
			if err := deps.TodoMgr.CompleteTodo(ctx, params.ID); err != nil {
				return "", fmt.Errorf("complete todo: %w", err)
			}
			return fmt.Sprintf(`{"success":true,"message":"待办 #%d 已完成"}`, params.ID), nil
		},
	})
}

func registerWorkflowTools(registry *ToolRegistry, deps WriteToolDependencies) {
	registry.Register(&ToolDefinition{
		Level: LevelWrite,
		Tool: llmclient.Tool{
			Type: "function",
			Function: llmclient.Function{
				Name:        "trigger_workflow",
				Description: "触发 n8n 工作流",
				Parameters: jsonSchema(map[string]interface{}{
					"name":    map[string]interface{}{"type": "string", "description": "工作流名称"},
					"payload": map[string]interface{}{"type": "object", "description": "传给工作流的参数（可选）"},
				}, []string{"name"}),
			},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (string, error) {
			var params struct {
				Name    string                 `json:"name"`
				Payload map[string]interface{} `json:"payload"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return "", fmt.Errorf("parse args: %w", err)
			}
			result, err := deps.Workflow.TriggerWorkflow(ctx, params.Name, params.Payload)
			if err != nil {
				return "", fmt.Errorf("trigger workflow: %w", err)
			}
			data, err := json.Marshal(result)
			if err != nil {
				return "", fmt.Errorf("marshal result: %w", err)
			}
			return string(data), nil
		},
	})
}
