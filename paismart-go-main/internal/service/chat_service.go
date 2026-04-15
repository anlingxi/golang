package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"pai-smart-go/internal/config"
	einocallbacks "pai-smart-go/internal/eino/callbacks"
	"pai-smart-go/internal/model"
	"pai-smart-go/pkg/log"
)

// ChatModel 是聊天模型抽象。
// 第一批先不要绑定具体 Eino 类型，后面接 factory 时再落具体实现。
type ChatModel interface {
	Stream(ctx context.Context, messages []ChatMessage, onChunk func(delta string) error) (string, error)
}

// ChatMessage 是 service 层内部统一消息结构。
// 后续由 internal/eino/types 负责和 Eino schema.Message 互转。
type ChatMessage struct {
	Role      string
	Content   string
	Provider  string
	Model     string
	Timestamp time.Time
}

type GenerateResult struct {
	Answer        string
	Provider      string
	Model         string
	InputTokens   int
	OutputTokens  int
	StopReason    string
	IsInterrupted bool
	TraceID       string
	LatencyMS     int64
}

// ChatService 现在是无状态能力服务，不持有会话状态。
type ChatService interface {
	GenerateStream(
		ctx context.Context,
		user *model.User,
		conversationID string,
		history []model.ChatMessage,
		userMessage string,
		writer StreamWriter,
	) (GenerateResult, error)
}

type chatService struct {
	searchService   SearchService
	chatModel       ChatModel
	callbackManager *einocallbacks.Manager
	einoCfg         config.EinoConfig
}

func NewChatService(
	searchService SearchService,
	chatModel ChatModel,
	callbackManager *einocallbacks.Manager,
	einoCfg config.EinoConfig,
) ChatService {
	return &chatService{
		searchService:   searchService,
		chatModel:       chatModel,
		callbackManager: callbackManager,
		einoCfg:         einoCfg,
	}
}

// GenerateStream 是 ChatService 的核心方法，负责处理一次完整的问答交互：
// 1. 接收用户输入和历史消息
// 2. 调用 SearchService 做检索，获取相关文档
// 3. 构造模型输入的消息列表，包含系统提示、历史消息、用户新消息
func (s *chatService) GenerateStream(
	ctx context.Context,
	user *model.User,
	conversationID string,
	history []model.ChatMessage,
	userMessage string,
	writer StreamWriter,
) (GenerateResult, error) {
	if s.chatModel == nil {
		return GenerateResult{}, fmt.Errorf("chat model is nil")
	}
	// 构建 Eino 回调元信息，包含场景、阶段、用户ID、模型提供商和模型名称等
	meta := einocallbacks.RunMeta{
		Scene:          "chat",
		Stage:          einocallbacks.StageChatGenerate,
		UserID:         user.ID,
		ConversationID: conversationID,
		Provider:       s.einoCfg.ChatModel.Provider,
		Model:          s.einoCfg.ChatModel.Model,
	}
	// 初始化回调上下文
	runCtx := ctx
	if s.callbackManager != nil && s.callbackManager.Enabled() {
		runCtx = s.callbackManager.InitCallbacks(ctx, meta)
	}
	// 检索阶段可以单独复用回调上下文，区分 generate 和 retrieve 两个阶段
	retrieveCtx := runCtx
	if s.callbackManager != nil && s.callbackManager.Enabled() {
		retrieveCtx = s.callbackManager.ReuseForStage(runCtx, meta.WithStage(einocallbacks.StageChatRetrieve))
	}
	// 调用 SearchService 做检索，获取相关文档；如果检索失败，记录日志但继续进行生成，使用无检索上下文模式
	searchResults, err := s.searchService.HybridSearch(retrieveCtx, userMessage, 5, user)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return GenerateResult{
				Provider:      s.einoCfg.ChatModel.Provider,
				Model:         s.einoCfg.ChatModel.Model,
				IsInterrupted: true,
				StopReason:    "context_canceled",
			}, err
		}
		log.Warnf("[ChatService] 检索失败，继续使用无检索上下文模式, err=%v", err)
	}
	// 构造模型输入的消息列表，包含系统提示、历史消息、用户新消息，以及检索到的相关文档作为上下文
	contextText := s.buildContextText(searchResults)
	msgs := s.buildMessages(history, contextText, userMessage)
	if err := modelCtxErr(runCtx); err != nil {
		return GenerateResult{
			Provider:      s.einoCfg.ChatModel.Provider,
			Model:         s.einoCfg.ChatModel.Model,
			IsInterrupted: true,
			StopReason:    "context_canceled",
		}, err
	}
	modelCtx := runCtx
	if s.callbackManager != nil && s.callbackManager.Enabled() {
		modelCtx = s.callbackManager.ReuseForStage(runCtx, meta.WithStage(einocallbacks.StageChatModel))
	}

	var fullAnswer strings.Builder
	finalText, err := s.chatModel.Stream(modelCtx, msgs, func(delta string) error {
		if delta == "" {
			return nil
		}
		fullAnswer.WriteString(delta)
		if writer != nil {
			return writer.WriteChunk(delta)
		}
		return nil
	})
	if err != nil {
		result := GenerateResult{
			Answer:        fullAnswer.String(),
			Provider:      s.einoCfg.ChatModel.Provider,
			Model:         s.einoCfg.ChatModel.Model,
			IsInterrupted: errors.Is(err, context.Canceled),
			StopReason:    "",
		}

		if errors.Is(err, context.Canceled) {
			result.StopReason = "context_canceled"
		}

		return result, err
	}

	answer := fullAnswer.String()
	if finalText != "" {
		answer = finalText
	}

	return GenerateResult{
		Answer:        answer,
		Provider:      s.einoCfg.ChatModel.Provider,
		Model:         s.einoCfg.ChatModel.Model,
		InputTokens:   0,
		OutputTokens:  0,
		StopReason:    "",
		IsInterrupted: false,
		TraceID:       "",
		LatencyMS:     0,
	}, nil
}

func modelCtxErr(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}

// buildContextText 将检索结果格式化为字符串，作为系统提示的一部分提供给模型。
func (s *chatService) buildContextText(results []model.SearchResponseDTO) string {
	if len(results) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("以下是与用户问题相关的参考资料：\n\n")
	for i, doc := range results {
		b.WriteString(fmt.Sprintf("[%d] %s\n", i+1, doc.TextContent))
	}
	return b.String()
}

func (s *chatService) buildMessages(
	history []model.ChatMessage,
	contextText string,
	userMessage string,
) []ChatMessage {
	msgs := make([]ChatMessage, 0, len(history)+2)

	systemPrompt := "你是一个严谨的智能问答助手。请优先基于给定资料回答。"
	if contextText != "" {
		systemPrompt += "\n\n" + contextText
	}

	msgs = append(msgs, ChatMessage{
		Role:    "system",
		Content: systemPrompt,
	})

	for _, m := range history {
		if m.Role == "" || m.Content == "" {
			continue
		}
		msgs = append(msgs, ChatMessage{
			Role:    m.Role,
			Content: m.Content,
		})
	}

	msgs = append(msgs, ChatMessage{
		Role:    "user",
		Content: userMessage,
	})

	return msgs
}
