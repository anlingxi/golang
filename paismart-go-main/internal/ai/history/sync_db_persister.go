package history

import (
	"context"
	"pai-smart-go/internal/infra/cache"
	"pai-smart-go/internal/model"
	"pai-smart-go/internal/repository"
	"pai-smart-go/pkg/log"
	"time"
)

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
