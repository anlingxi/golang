package history

import (
	"context"
	"fmt"
	"pai-smart-go/internal/ai/history/event"
	"pai-smart-go/internal/infra/cache"
	"pai-smart-go/internal/model"
	"pai-smart-go/pkg/log"
	"time"
)

// cacheAndAsyncDBPersister 实现了 Persister 接口，先将对话轮次数据写入缓存，然后异步发布持久化任务到消息队列，
// 由后台消费者负责将数据持久化到数据库。
type cacheAndAsyncDBPersister struct {
	historyCache  cache.HistoryCache
	taskPublisher event.PersistTaskPublisher
	recentTTL     time.Duration
}

func NewCacheAndAsyncDBPersister(
	historyCache cache.HistoryCache,
	taskPublisher event.PersistTaskPublisher,
	recentTTL time.Duration,
) Persister {
	return &cacheAndAsyncDBPersister{
		historyCache:  historyCache,
		taskPublisher: taskPublisher,
		recentTTL:     recentTTL,
	}
}

// PersistTurn 的实现逻辑如下：
// 1. recent cache 继续同步刷新：如果请求中包含 FullMessages 且 historyCache 不为 nil，则将 FullMessages 设置到缓存中，
// 过期时间为 recentTTL。
// 2. 发布异步持久化任务：构建一个 PersistTurnTask 对象，包含对话轮次的相关信息，然后通过 taskPublisher.PublishPersistTurnTask
// 方法将任务发布到消息队列中，等待后台消费者处理。
func (p *cacheAndAsyncDBPersister) PersistTurn(ctx context.Context, req PersistTurnRequest) error {
	// 1. recent cache 继续同步刷新
	if len(req.FullMessages) > 0 && p.historyCache != nil {
		if err := p.historyCache.SetRecentMessages(ctx, req.SessionID, req.FullMessages, p.recentTTL); err != nil {
			log.Warnf("[AsyncHistoryPersister] set recent cache failed, session_id=%s, err=%v", req.SessionID, err)
		}
	}

	// 2. 发布异步持久化任务
	task := event.PersistTurnTask{
		TaskID:           buildTaskID(req),
		UserID:           req.UserID,
		SessionID:        req.SessionID,
		TurnID:           req.TurnID,
		RequestID:        req.RequestID,
		Mode:             req.Mode,
		Provider:         req.Provider,
		Model:            req.Model,
		UserMessage:      req.UserMessage,
		AssistantMessage: req.AssistantMessage,

		InputTokens:   req.RunMeta.InputTokens,
		OutputTokens:  req.RunMeta.OutputTokens,
		LatencyMS:     req.RunMeta.LatencyMS,
		StopReason:    req.RunMeta.StopReason,
		ErrorMessage:  req.RunMeta.ErrorMessage,
		TraceID:       req.RunMeta.TraceID,
		IsInterrupted: req.RunMeta.IsInterrupted,

		OccurredAt: req.OccurredAt,
	}

	if err := p.taskPublisher.PublishPersistTurnTask(ctx, task); err != nil {
		return err
	}

	return nil
}

func (p *cacheAndAsyncDBPersister) AppendMessage(ctx context.Context, req PersistMessageRequest) error {
	// 第一版先不走 MQ，仍然可以后续按需扩展
	return fmt.Errorf("AppendMessage not implemented in async persister")
}

func (p *cacheAndAsyncDBPersister) RefreshRecent(ctx context.Context, sessionID string, messages []model.ChatMessage) error {
	if p.historyCache == nil {
		return nil
	}
	return p.historyCache.SetRecentMessages(ctx, sessionID, messages, p.recentTTL)
}

func buildTaskID(req PersistTurnRequest) string {
	if req.TurnID != "" {
		return "persist_turn_" + req.TurnID
	}
	return fmt.Sprintf("persist_turn_%d", time.Now().UnixNano())
}
