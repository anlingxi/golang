package helper

import (
	"context"
	"fmt"
	"pai-smart-go/internal/ai/history"
	einocallbacks "pai-smart-go/internal/eino/callbacks"
	"pai-smart-go/internal/model"
	"pai-smart-go/internal/service"
	"pai-smart-go/pkg/log"
	"sync"
	"time"
)

// AIHelper 是会话级运行时对象。
// 一个 conversationID 对应一个 AIHelper。
// 它负责维护该会话的内存消息、首次历史加载，以及调用底层 ChatService 完成响应生成。
type AIHelper struct {
	userID    uint
	sessionID string
	// 为什么用RWMutex？因为我们读的频率远高于写，尤其是在生成回复的过程中，历史消息基本上是只读的，只有在追加新消息时才需要写锁。
	// 使用 RWMutex 可以提高并发性能。
	// RWMutex 允许多个读锁同时存在，但写锁是独占的。当有写锁时，所有读锁和其他写锁都会被阻塞。不同于 Mutex，RWMutex 适用于读多写少的场景，
	// 可以提高并发性能。
	// 为什么是读多写少？因为在生成回复的过程中，我们会频繁地读取历史消息来构建模型输入，但只有在用户发送新消息或模型生成新回复时才会修改历史消息。
	mu     sync.RWMutex
	loaded bool
	// 这里直接把历史消息放在内存里，避免每次生成回复都要访问数据库加载历史，提升性能。
	// 如果单次会话消息过多，可能会导致内存占用过高，后续可以考虑只保留最近的 N 条消息在内存，或者使用其他数据结构来管理历史消息。
	// 使用其他什么数据结构？比如环形缓冲区（circular buffer）可以在达到一定容量后覆盖旧消息，或者使用链表来动态管理消息列表，
	// 避免切片扩容带来的性能问题。
	// 哪个方法更好？切片实现简单，适合消息数量不大的场景；环形缓冲区适合需要限制内存使用的场景；链表适合频繁插入删除的场景。
	// 需要根据实际需求选择。
	messages []model.ChatMessage

	chatService    service.ChatService
	historyManager history.Manager
}

// NewAIHelper 创建一个新的 AIHelper。
// 注意：这里只是创建运行时对象，不会立即加载历史。
func NewAIHelper(
	userID uint,
	conversationID string,
	chatService service.ChatService,
	historyManager history.Manager,
) *AIHelper {
	return &AIHelper{
		userID:         userID,
		sessionID:      conversationID,
		chatService:    chatService,
		historyManager: historyManager,
		messages:       make([]model.ChatMessage, 0),
	}
}

// UserID 返回当前 helper 绑定的用户 ID。
func (h *AIHelper) UserID() uint {
	return h.userID
}

// SessionID 返回当前 helper 绑定的会话 ID。
func (h *AIHelper) SessionID() string {
	return h.sessionID
}

// EnsureLoaded 确保会话历史只被加载一次。
// 防止止重复加载历史导致性能问题。 这里使用双重检查锁定（double-checked locking）模式。
// 双重锁检查锁定原理：第一次检查 loaded 标志，如果已经加载则直接返回，避免获取写锁；如果没有加载，则获取写锁后再次检查 loaded 标志，
// 确保只有一个线程进行加载。
// 为什么要双重检查锁定？因为我们希望在多数情况下（历史已经加载的情况下）避免获取写锁，提高性能。
// 只有在第一次加载历史时才需要获取写锁，之后的调用都可以直接读取 loaded 标志而不需要锁。
func (h *AIHelper) EnsureLoaded(ctx context.Context) error {
	h.mu.RLock()
	if h.loaded {
		h.mu.RUnlock()
		return nil
	}
	h.mu.RUnlock()

	h.mu.Lock()
	defer h.mu.Unlock()

	// double check，避免并发重复加载
	// 为什么加载历史的时候要用写锁？因为加载历史会修改 AIHelper 内部的状态（messages 和 loaded），需要保证线程安全。
	if h.loaded {
		return nil
	}

	if h.historyManager == nil {
		return fmt.Errorf("history manager is nil")
	}

	historyMessages, err := h.historyManager.LoadHistory(ctx, history.LoadHistoryOptions{
		SessionID:         h.sessionID,
		UserID:            h.userID,
		Limit:             20,
		PreferRecentCache: true,
		IncludeSystem:     true,
	})
	if err != nil {
		return err
	}

	h.messages = historyMessages
	h.loaded = true

	log.Infof("[AIHelper] 会话历史加载完成, user_id=%d, session_id=%s, history_len=%d",
		h.userID, h.sessionID, len(h.messages))

	return nil
}

// GetMessages 返回当前内存中的会话消息副本。
// 为什么要返回副本，因为我们不希望外部直接修改 AIHelper 内部的消息切片，导致并发安全问题。
func (h *AIHelper) GetMessages() []model.ChatMessage {
	h.mu.RLock()
	defer h.mu.RUnlock()

	copied := make([]model.ChatMessage, len(h.messages))
	copy(copied, h.messages)
	return copied
}

// appendMessage 内部使用，要求调用者自己保证加锁策略一致。
func (h *AIHelper) appendMessage(msg model.ChatMessage) {
	h.messages = append(h.messages, msg)
}

// AppendMessage 向内存会话中追加一条消息。
func (h *AIHelper) AppendMessage(msg model.ChatMessage) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.appendMessage(msg)
}

// AppendUserMessage 追加用户消息。
func (h *AIHelper) AppendUserMessage(msg model.ChatMessage) model.ChatMessage {
	h.AppendMessage(msg)
	return msg
}

// AppendAssistantMessage 追加助手消息。
func (h *AIHelper) AppendAssistantMessage(msg model.ChatMessage) model.ChatMessage {
	h.AppendMessage(msg)
	return msg
}

// StreamResponse 是 AIHelper 对外暴露的核心入口。
func (h *AIHelper) StreamResponse(
	ctx context.Context,
	user *model.User,
	userMessage string,
	writer service.StreamWriter,
	shouldStop func() bool,
) error {
	if h.chatService == nil {
		return fmt.Errorf("chat service is nil")
	}
	if h.historyManager == nil {
		return fmt.Errorf("history manager is nil")
	}

	// 1. 首次加载历史
	if err := h.EnsureLoaded(ctx); err != nil {
		return err
	}

	// 2. 取当前历史快照，作为本轮模型输入历史
	// 注意：这里仍然保持“旧 history + 本轮 userMessage”的模式
	// 先取旧历史快照，作为本轮模型输入
	historySnapshot := h.GetMessages()

	// 先把 user 消息写入内存
	userMsg := h.AppendUserMessage(model.ChatMessage{
		Role:      "user",
		Content:   userMessage,
		Timestamp: time.Now(),
	})

	genResult, err := h.chatService.GenerateStream(
		ctx,
		user,
		h.sessionID,
		historySnapshot,
		userMessage,
		writer,
		shouldStop,
	)
	if err != nil && genResult.Answer == "" {
		log.Errorf("[AIHelper] 生成失败, session_id=%s, err=%v", h.sessionID, err)
		return err
	}

	assistantMsg := h.AppendAssistantMessage(model.ChatMessage{
		Role:      "assistant",
		Content:   genResult.Answer,
		Provider:  genResult.Provider,
		Model:     genResult.Model,
		Timestamp: time.Now(),
	})
	fullMessages := h.GetMessages()

	// 这里先临时生成 turn/request，下一步再从请求链路透传
	turnID := fmt.Sprintf("turn_%d", time.Now().UnixNano())

	// 如果你现在 callback 里已经有更完整的 RunMeta，可以在这里做一个转换层
	_ = einocallbacks.RunMeta{} // 临时保留 import，防止你下一步接 callback 时忘了

	req := history.PersistTurnRequest{
		UserID:           h.userID,
		SessionID:        h.sessionID,
		TurnID:           turnID,
		RequestID:        genResult.TraceID,
		Mode:             string(model.SessionModeNormal),
		Provider:         genResult.Provider,
		Model:            genResult.Model,
		UserMessage:      userMsg,
		AssistantMessage: assistantMsg,
		FullMessages:     fullMessages,
		RunMeta: history.RunMeta{
			InputTokens:   genResult.InputTokens,
			OutputTokens:  genResult.OutputTokens,
			LatencyMS:     genResult.LatencyMS,
			StopReason:    genResult.StopReason,
			IsInterrupted: genResult.IsInterrupted,
			TraceID:       genResult.TraceID,
			ErrorMessage:  errString(err),
		},
		OccurredAt: time.Now(),
	}

	if err := h.historyManager.PersistTurn(ctx, req); err != nil {
		log.Errorf("[AIHelper] 历史持久化失败, session_id=%s, err=%v", h.sessionID, err)
	}

	return nil
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
package helper

import (
	"context"
	"fmt"
	"pai-smart-go/internal/ai/history"
	einocallbacks "pai-smart-go/internal/eino/callbacks"
	"pai-smart-go/internal/model"
	"pai-smart-go/internal/service"
	"pai-smart-go/pkg/log"
	"sync"
	"time"
)

// AIHelper 是会话级运行时对象。
// 一个 conversationID 对应一个 AIHelper。
// 它负责维护该会话的内存消息、首次历史加载，以及调用底层 ChatService 完成响应生成。
type AIHelper struct {
	userID    uint
	sessionID string
	// 为什么用RWMutex？因为我们读的频率远高于写，尤其是在生成回复的过程中，历史消息基本上是只读的，只有在追加新消息时才需要写锁。
	// 使用 RWMutex 可以提高并发性能。
	// RWMutex 允许多个读锁同时存在，但写锁是独占的。当有写锁时，所有读锁和其他写锁都会被阻塞。不同于 Mutex，RWMutex 适用于读多写少的场景，
	// 可以提高并发性能。
	// 为什么是读多写少？因为在生成回复的过程中，我们会频繁地读取历史消息来构建模型输入，但只有在用户发送新消息或模型生成新回复时才会修改历史消息。
	mu     sync.RWMutex
	loaded bool
	// 这里直接把历史消息放在内存里，避免每次生成回复都要访问数据库加载历史，提升性能。
	// 如果单次会话消息过多，可能会导致内存占用过高，后续可以考虑只保留最近的 N 条消息在内存，或者使用其他数据结构来管理历史消息。
	// 使用其他什么数据结构？比如环形缓冲区（circular buffer）可以在达到一定容量后覆盖旧消息，或者使用链表来动态管理消息列表，
	// 避免切片扩容带来的性能问题。
	// 哪个方法更好？切片实现简单，适合消息数量不大的场景；环形缓冲区适合需要限制内存使用的场景；链表适合频繁插入删除的场景。
	// 需要根据实际需求选择。
	messages []model.ChatMessage

	chatService    service.ChatService
	historyManager history.Manager
}

// NewAIHelper 创建一个新的 AIHelper。
// 注意：这里只是创建运行时对象，不会立即加载历史。
func NewAIHelper(
	userID uint,
	conversationID string,
	chatService service.ChatService,
	historyManager history.Manager,
) *AIHelper {
	return &AIHelper{
		userID:         userID,
		sessionID:      conversationID,
		chatService:    chatService,
		historyManager: historyManager,
		messages:       make([]model.ChatMessage, 0),
	}
}

// UserID 返回当前 helper 绑定的用户 ID。
func (h *AIHelper) UserID() uint {
	return h.userID
}

// SessionID 返回当前 helper 绑定的会话 ID。
func (h *AIHelper) SessionID() string {
	return h.sessionID
}

// EnsureLoaded 确保会话历史只被加载一次。
// 防止止重复加载历史导致性能问题。 这里使用双重检查锁定（double-checked locking）模式。
// 双重锁检查锁定原理：第一次检查 loaded 标志，如果已经加载则直接返回，避免获取写锁；如果没有加载，则获取写锁后再次检查 loaded 标志，
// 确保只有一个线程进行加载。
// 为什么要双重检查锁定？因为我们希望在多数情况下（历史已经加载的情况下）避免获取写锁，提高性能。
// 只有在第一次加载历史时才需要获取写锁，之后的调用都可以直接读取 loaded 标志而不需要锁。
func (h *AIHelper) EnsureLoaded(ctx context.Context) error {
	h.mu.RLock()
	if h.loaded {
		h.mu.RUnlock()
		return nil
	}
	h.mu.RUnlock()

	h.mu.Lock()
	defer h.mu.Unlock()

	// double check，避免并发重复加载
	// 为什么加载历史的时候要用写锁？因为加载历史会修改 AIHelper 内部的状态（messages 和 loaded），需要保证线程安全。
	if h.loaded {
		return nil
	}

	if h.historyManager == nil {
		return fmt.Errorf("history manager is nil")
	}

	historyMessages, err := h.historyManager.LoadHistory(ctx, history.LoadHistoryOptions{
		SessionID:         h.sessionID,
		UserID:            h.userID,
		Limit:             20,
		PreferRecentCache: true,
		IncludeSystem:     true,
	})
	if err != nil {
		return err
	}

	h.messages = historyMessages
	h.loaded = true

	log.Infof("[AIHelper] 会话历史加载完成, user_id=%d, session_id=%s, history_len=%d",
		h.userID, h.sessionID, len(h.messages))

	return nil
}

// GetMessages 返回当前内存中的会话消息副本。
// 为什么要返回副本，因为我们不希望外部直接修改 AIHelper 内部的消息切片，导致并发安全问题。
func (h *AIHelper) GetMessages() []model.ChatMessage {
	h.mu.RLock()
	defer h.mu.RUnlock()

	copied := make([]model.ChatMessage, len(h.messages))
	copy(copied, h.messages)
	return copied
}

// appendMessage 内部使用，要求调用者自己保证加锁策略一致。
func (h *AIHelper) appendMessage(msg model.ChatMessage) {
	h.messages = append(h.messages, msg)
}

// AppendMessage 向内存会话中追加一条消息。
func (h *AIHelper) AppendMessage(msg model.ChatMessage) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.appendMessage(msg)
}

// AppendUserMessage 追加用户消息。
func (h *AIHelper) AppendUserMessage(msg model.ChatMessage) model.ChatMessage {
	h.AppendMessage(msg)
	return msg
}

// AppendAssistantMessage 追加助手消息。
func (h *AIHelper) AppendAssistantMessage(msg model.ChatMessage) model.ChatMessage {
	h.AppendMessage(msg)
	return msg
}

// StreamResponse 是 AIHelper 对外暴露的核心入口。
func (h *AIHelper) StreamResponse(
	ctx context.Context,
	user *model.User,
	userMessage string,
	writer service.StreamWriter,
	shouldStop func() bool,
) error {
	if h.chatService == nil {
		return fmt.Errorf("chat service is nil")
	}
	if h.historyManager == nil {
		return fmt.Errorf("history manager is nil")
	}

	// 1. 首次加载历史
	if err := h.EnsureLoaded(ctx); err != nil {
		return err
	}

	// 2. 取当前历史快照，作为本轮模型输入历史
	// 注意：这里仍然保持“旧 history + 本轮 userMessage”的模式
	// 先取旧历史快照，作为本轮模型输入
	historySnapshot := h.GetMessages()

	// 先把 user 消息写入内存
	userMsg := h.AppendUserMessage(model.ChatMessage{
		Role:      "user",
		Content:   userMessage,
		Timestamp: time.Now(),
	})

	genResult, err := h.chatService.GenerateStream(
		ctx,
		user,
		h.sessionID,
		historySnapshot,
		userMessage,
		writer,
		shouldStop,
	)
	if err != nil && genResult.Answer == "" {
		log.Errorf("[AIHelper] 生成失败, session_id=%s, err=%v", h.sessionID, err)
		return err
	}

	assistantMsg := h.AppendAssistantMessage(model.ChatMessage{
		Role:      "assistant",
		Content:   genResult.Answer,
		Provider:  genResult.Provider,
		Model:     genResult.Model,
		Timestamp: time.Now(),
	})
	fullMessages := h.GetMessages()

	// 这里先临时生成 turn/request，下一步再从请求链路透传
	turnID := fmt.Sprintf("turn_%d", time.Now().UnixNano())

	// 如果你现在 callback 里已经有更完整的 RunMeta，可以在这里做一个转换层
	_ = einocallbacks.RunMeta{} // 临时保留 import，防止你下一步接 callback 时忘了

	req := history.PersistTurnRequest{
		UserID:           h.userID,
		SessionID:        h.sessionID,
		TurnID:           turnID,
		RequestID:        genResult.TraceID,
		Mode:             "normal_chat",
		Provider:         genResult.Provider,
		Model:            genResult.Model,
		UserMessage:      userMsg,
		AssistantMessage: assistantMsg,
		FullMessages:     fullMessages,
		RunMeta: history.RunMeta{
			InputTokens:   genResult.InputTokens,
			OutputTokens:  genResult.OutputTokens,
			LatencyMS:     genResult.LatencyMS,
			StopReason:    genResult.StopReason,
			IsInterrupted: genResult.IsInterrupted,
			TraceID:       genResult.TraceID,
			ErrorMessage:  errString(err),
		},
		OccurredAt: time.Now(),
	}

	if err := h.historyManager.PersistTurn(ctx, req); err != nil {
		log.Errorf("[AIHelper] 历史持久化失败, session_id=%s, err=%v", h.sessionID, err)
	}

	return nil
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
