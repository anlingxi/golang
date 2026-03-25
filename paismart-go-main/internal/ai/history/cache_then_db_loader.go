package history

import (
	"context"
	"pai-smart-go/internal/infra/cache"
	"pai-smart-go/internal/model"
	"pai-smart-go/internal/repository"
	"pai-smart-go/pkg/log"
	"time"
)

// cacheThenDBLoader 实现了 Loader 接口，优先从缓存加载历史记录，如果缓存未命中则从数据库加载。
type cacheThenDBLoader struct {
	historyCache cache.HistoryCache
	msgRepo      repository.ConversationMsgRepository
	recentTTL    time.Duration
}

func NewCacheThenDBLoader(
	historyCache cache.HistoryCache,
	msgRepo repository.ConversationMsgRepository,
	recentTTL time.Duration,
) Loader {
	return &cacheThenDBLoader{
		historyCache: historyCache,
		msgRepo:      msgRepo,
		recentTTL:    recentTTL,
	}
}

func (l *cacheThenDBLoader) LoadHistory(ctx context.Context, opts LoadHistoryOptions) ([]model.ChatMessage, error) {
	// 1. 如果 PreferRecentCache 为 true，且 historyCache 不为 nil，则尝试从缓存加载最近的消息。
	// 2. 如果缓存命中且有消息，则直接返回缓存中的消息。
	// 3. 否则，从数据库加载最近的消息。
	// 4. 将从数据库加载的消息转换为 ChatMessage 列表。
	// 5. 如果 historyCache 不为 nil 且加载到的消息列表不为空，则将这些消息设置到缓存中，设置过期时间为 recentTTL。
	// 6. 返回加载到的消息列表。
	if opts.PreferRecentCache && l.historyCache != nil {
		cached, err := l.historyCache.GetRecentMessages(ctx, opts.SessionID)
		if err == nil && len(cached) > 0 {
			log.Infof("[HistoryLoader] recent cache hit, session_id=%s, len=%d", opts.SessionID, len(cached))
			return cached, nil
		}
	}

	limit := opts.Limit
	if limit <= 0 {
		limit = 20
	}

	msgs, err := l.msgRepo.ListRecentBySessionID(ctx, opts.SessionID, limit)
	if err != nil {
		return nil, err
	}

	chatMessages := toChatMessages(msgs)
	// 为什么要设置缓存？因为如果用户在短时间内多次请求历史记录，或者在对话过程中频繁地加载历史记录，那么每次都访问数据库可能会带来较大的性能开销。
	// 通过将最近的消息缓存起来，我们可以在一定时间内直接从缓存中获取历史记录，减少数据库的访问次数，提高系统的响应速度和性能。
	// 当然，我们也需要设置合理的过期时间，确保缓存中的数据不会过时，同时也不会占用过多的内存资源。
	// 只有在成功加载到消息且 historyCache 不为 nil 的情况下才设置缓存，这样可以避免将空结果或者错误结果缓存起来。
	if l.historyCache != nil && len(chatMessages) > 0 {
		if err := l.historyCache.SetRecentMessages(ctx, opts.SessionID, chatMessages, l.recentTTL); err != nil {
			log.Warnf("[HistoryLoader] set recent cache failed, session_id=%s, err=%v", opts.SessionID, err)
		}
	}

	return chatMessages, nil
}
