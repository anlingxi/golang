package history

import (
	"context"
	"pai-smart-go/internal/infra/cache"
	"pai-smart-go/internal/model"
	"pai-smart-go/internal/repository"
	"pai-smart-go/pkg/log"
	"time"
)

// 这是一个同步的DB持久化实现，适用于对数据一致性要求较高的场景。每次持久化都会直接写入数据库，并更新缓存中的最近消息。
type syncDBPersister struct {
	historyCache cache.HistoryCache
	sessionRepo  repository.SessionRepository
	msgRepo      repository.ConversationMsgRepository
	turnRepo     repository.ConversationTurnRepository
	recentTTL    time.Duration
}

func NewSyncDBPersister(
	historyCache cache.HistoryCache,
	sessionRepo repository.SessionRepository,
	msgRepo repository.ConversationMsgRepository,
	turnRepo repository.ConversationTurnRepository,
	recentTTL time.Duration,
) Persister {
	return &syncDBPersister{
		historyCache: historyCache,
		sessionRepo:  sessionRepo,
		msgRepo:      msgRepo,
		turnRepo:     turnRepo,
		recentTTL:    recentTTL,
	}
}

// 1. 持久化时直接写入数据库，确保数据的一致性和可靠性。
// 2. 每次持久化后更新缓存中的最近消息，确保用户在短时间内能看到最新的对话内容。
// 3. 在更新会话的最后消息时间时，如果失败了，不影响主流程，但会记录日志以便排查问题。
func (p *syncDBPersister) PersistTurn(ctx context.Context, req PersistTurnRequest) error {
	if len(req.FullMessages) > 0 && p.historyCache != nil {
		if err := p.historyCache.SetRecentMessages(ctx, req.SessionID, req.FullMessages, p.recentTTL); err != nil {
			log.Warnf("[HistoryPersister] set recent cache failed, session_id=%s, err=%v", req.SessionID, err)
		}
	}

	baseSeq, err := p.msgRepo.GetMaxSequence(ctx, req.SessionID)
	if err != nil {
		return err
	}

	turn := buildTurnFromRequest(req)
	msgs := buildConversationMsgsFromTurn(req, baseSeq)

	if err := p.turnRepo.Create(ctx, turn); err != nil {
		return err
	}

	if err := p.msgRepo.BatchCreate(ctx, msgs); err != nil {
		return err
	}

	if err := p.sessionRepo.UpdateLastMessageAt(ctx, req.SessionID); err != nil {
		log.Warnf("[HistoryPersister] update session last_message_at failed, session_id=%s, err=%v", req.SessionID, err)
	}

	return nil
}

func (p *syncDBPersister) AppendMessage(ctx context.Context, req PersistMessageRequest) error {
	now := req.OccurredAt
	if now.IsZero() {
		now = time.Now()
	}

	msg := &model.ConversationMsg{
		ID:          generateMsgID(),
		SessionID:   req.SessionID,
		TurnID:      req.TurnID,
		RequestID:   req.RequestID,
		UserID:      req.UserID,
		Role:        model.MessageRole(req.Message.Role),
		Content:     req.Message.Content,
		ContentType: "text",
		Sequence:    req.Sequence,
		Status:      model.MessageStatusCompleted,
		Provider:    req.Provider,
		Model:       req.Model,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	return p.msgRepo.Create(ctx, msg)
}

func (p *syncDBPersister) RefreshRecent(ctx context.Context, sessionID string, messages []model.ChatMessage) error {
	if p.historyCache == nil {
		return nil
	}
	return p.historyCache.SetRecentMessages(ctx, sessionID, messages, p.recentTTL)
}
