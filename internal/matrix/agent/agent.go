package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/singll/bellkeeper/internal/config"
	"github.com/singll/bellkeeper/internal/llmclient"
	"github.com/singll/bellkeeper/internal/matrix/gateway"
	"github.com/singll/bellkeeper/internal/middleware"
	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/repository"
	"go.uber.org/zap"
)

type AgentService struct {
	llm         *llmclient.Client
	tools       *ToolRegistry
	sessions    *SessionStore
	rateLimiter *RateLimiter
	gateway     *gateway.Client
	repos       *repository.Repositories
	cfg         config.MatrixAgentConfig
	policy      PolicyChecker
}

type PolicyChecker interface {
	IsAdmin(userID string) bool
}

func NewAgentService(
	cfg config.MatrixAgentConfig,
	llmProxyURL string,
	apiKey string,
	redisClient *redis.Client,
	gatewayClient *gateway.Client,
	repos *repository.Repositories,
	policy PolicyChecker,
	tools *ToolRegistry,
) *AgentService {
	if !cfg.Enabled {
		return nil
	}

	sessionTTL := 30 * time.Minute
	if cfg.SessionTTL != "" {
		if d, err := time.ParseDuration(cfg.SessionTTL); err == nil {
			sessionTTL = d
		}
	}

	maxTurns := int64(30)
	if cfg.MaxTurnsPerHour > 0 {
		maxTurns = int64(cfg.MaxTurnsPerHour)
	}

	maxIter := 5
	if cfg.MaxToolIterations > 0 {
		maxIter = cfg.MaxToolIterations
	}

	llm := llmclient.New(llmclient.Options{
		BaseURL: llmProxyURL,
		APIKey:  apiKey,
		Timeout: 120 * time.Second,
	})

	svc := &AgentService{
		llm:         llm,
		tools:       tools,
		sessions:    NewSessionStore(redisClient, sessionTTL, 20),
		rateLimiter: NewRateLimiter(redisClient, maxTurns),
		gateway:     gatewayClient,
		repos:       repos,
		cfg:         cfg,
		policy:      policy,
	}

	_ = maxIter
	middleware.GetLogger().Info("agent service initialized",
		zap.String("model", cfg.Model),
		zap.Int64("max_turns_per_hour", maxTurns),
		zap.Duration("session_ttl", sessionTTL))

	return svc
}

type TurnResult struct {
	Reply string
	UsedTools bool
}

func (s *AgentService) HandleMessage(ctx context.Context, roomID, sender, content string) (*TurnResult, error) {
	allowed, _, err := s.rateLimiter.Allow(ctx, roomID)
	if err != nil {
		return nil, fmt.Errorf("rate limit check: %w", err)
	}
	if !allowed {
		return &TurnResult{
			Reply: fmt.Sprintf("⏳ 此房间 Agent 回合已达上限（%d/小时），请稍后再试。", s.rateLimiter.max),
		}, nil
	}

	isAdmin := s.policy.IsAdmin(sender)

	systemPrompt := s.buildSystemPrompt(isAdmin)

	session, err := s.sessions.Get(ctx, roomID)
	if err != nil {
		middleware.GetLogger().Warn("failed to get session, starting fresh", zap.Error(err))
		session = nil
	}

	if session == nil {
		session = []llmclient.ChatMessage{
			{Role: "system", Content: systemPrompt},
		}
	} else if len(session) > 0 && session[0].Role == "system" {
		session[0].Content = systemPrompt
	}

	session = append(session, llmclient.ChatMessage{
		Role:    "user",
		Content: content,
	})

	maxIter := 5
	if s.cfg.MaxToolIterations > 0 {
		maxIter = s.cfg.MaxToolIterations
	}

	// 选用模型：优先该发言用户持久化的模型组覆盖（!model 设置），无则房间默认。
	model := s.cfg.Model
	if override, err := s.sessions.GetUserModel(ctx, sender); err == nil && override != "" {
		model = override
	}

	var reply string
	var usedTools bool
	current := session

	for i := 0; i < maxIter; i++ {
		req := llmclient.ChatRequest{
			Model:       model,
			Messages:    current,
			Temperature: 0.3,
			Tools:       s.tools.List(),
		}

		opts := llmclient.ChatOptions{
			CallerID:       "matrix-agent",
			TaskType:       "agent",
			ConversationID: roomID,
		}

		resp, err := s.llm.ChatCompletionFull(ctx, req, opts)
		if err != nil {
			return nil, fmt.Errorf("llm call (iter %d): %w", i, err)
		}

		assistantMsg := llmclient.ChatMessage{
			Role:      "assistant",
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		}
		current = append(current, assistantMsg)

		if len(resp.ToolCalls) == 0 {
			reply = resp.Content
			break
		}

		usedTools = true
		for _, tc := range resp.ToolCalls {
			if err := s.tools.CheckPermission(tc.Function.Name, isAdmin); err != nil {
				resultMsg := llmclient.ChatMessage{
					Role:       "tool",
					Content:    fmt.Sprintf("权限不足: %s", err.Error()),
					ToolCallID: tc.ID,
				}
				current = append(current, resultMsg)
				continue
			}

			result, err := s.tools.Execute(ctx, tc.Function.Name, json.RawMessage(tc.Function.Arguments))
			if err != nil {
				result = fmt.Sprintf("工具执行错误: %s", err.Error())
			}

			s.logToolCall(ctx, roomID, sender, tc.Function.Name, tc.Function.Arguments, result, err)

			resultMsg := llmclient.ChatMessage{
				Role:       "tool",
				Content:    result,
				ToolCallID: tc.ID,
			}
			current = append(current, resultMsg)
		}
	}

	if reply == "" {
		reply = "（Agent 完成工具调用但未生成文本回复）"
	}

	if err := s.sessions.Append(ctx, roomID, current...); err != nil {
		middleware.GetLogger().Warn("failed to save session", zap.Error(err))
	}

	return &TurnResult{
		Reply:     reply,
		UsedTools: usedTools,
	}, nil
}

func (s *AgentService) ResetSession(ctx context.Context, roomID string) error {
	if err := s.sessions.Clear(ctx, roomID); err != nil {
		return fmt.Errorf("clear session: %w", err)
	}
	if err := s.rateLimiter.Reset(ctx, roomID); err != nil {
		middleware.GetLogger().Warn("failed to reset rate limiter", zap.Error(err))
	}
	return nil
}

// SetUserModel 持久化某用户的对话模型组覆盖（按用户，跨房间生效；group 为空清除）。
func (s *AgentService) SetUserModel(ctx context.Context, userID, group string) error {
	return s.sessions.SetUserModel(ctx, userID, group)
}

// CurrentUserModel 返回某用户当前生效的模型组（无覆盖则返回房间默认 cfg.Model）。
func (s *AgentService) CurrentUserModel(ctx context.Context, userID string) (string, error) {
	override, err := s.sessions.GetUserModel(ctx, userID)
	if err != nil {
		return "", err
	}
	if override == "" {
		return s.cfg.Model, nil
	}
	return override, nil
}

func (s *AgentService) buildSystemPrompt(isAdmin bool) string {
	base := `你是 Bellkeeper 的 AI 助手，运行在 Matrix 通讯平台上。你可以通过工具调用来查询系统状态、搜索知识库、查看统计数据等。

行为准则：
1. 优先使用工具获取实时数据，不要编造信息
2. 回答简洁实用，避免冗长
3. 如果工具返回错误，如实告知用户
4. 使用中文回答`

	if s.cfg.SystemPrompt != "" {
		base = s.cfg.SystemPrompt
	}

	if isAdmin {
		base += "\n\n你是管理员，可以使用所有工具（包括写操作）。"
	} else {
		base += "\n\n你只有只读权限，不能执行写操作。"
	}

	var toolDescs []string
	for _, t := range s.tools.List() {
		toolDescs = append(toolDescs, fmt.Sprintf("- %s: %s", t.Function.Name, t.Function.Description))
	}
	if len(toolDescs) > 0 {
		base += "\n\n可用工具:\n" + strings.Join(toolDescs, "\n")
	}

	return base
}

func (s *AgentService) logToolCall(ctx context.Context, roomID, sender, toolName, args, result string, execErr error) {
	if s.repos == nil {
		return
	}
	status := "success"
	errMsg := ""
	if execErr != nil {
		status = "failed"
		errMsg = execErr.Error()
	}

	logEntry := &model.MatrixCommandLog{
		RoomID:          roomID,
		Sender:          sender,
		CommandName:     toolName,
		CommandArgs:     args,
		HandlerType:     "agent_tool",
		ExecutionStatus: status,
		CreatedAt:       time.Now(),
	}
	if errMsg != "" {
		logEntry.ErrorMessage = errMsg
	}
	if err := s.repos.MatrixCommandLog.Create(logEntry); err != nil {
		middleware.GetLogger().Warn("failed to log agent tool call", zap.Error(err))
	}
}
