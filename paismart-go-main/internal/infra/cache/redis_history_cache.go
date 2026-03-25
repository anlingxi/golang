package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"pai-smart-go/internal/model"
	"time"

	"github.com/go-redis/redis/v8"
)

// HistoryCache 定义了历史记录缓存的接口。
type HistoryCache interface {
	GetRecentMessages(ctx context.Context, sessionID string) ([]model.ChatMessage, error)
	SetRecentMessages(ctx context.Context, sessionID string, messages []model.ChatMessage, ttl time.Duration) error
	DeleteRecentMessages(ctx context.Context, sessionID string) error
}

type redisHistoryCache struct {
	rdb *redis.Client
}

func NewRedisHistoryCache(rdb *redis.Client) HistoryCache {
	return &redisHistoryCache{rdb: rdb}
}

// 为什么redis不需要锁？因为redis是单线程的，所有命令都是串行执行的，所以不需要担心并发访问导致的数据不一致问题。
// 即使多个客户端同时访问同一个key，redis也会按照顺序处理这些请求，确保数据的一致性和完整性。因此，在使用redis作为缓存时，
// 我们不需要额外的锁机制来保护数据。
func (c *redisHistoryCache) GetRecentMessages(ctx context.Context, sessionID string) ([]model.ChatMessage, error) {
	key := recentMessagesKey(sessionID)
	// 从 Redis 获取数据的是msg列表还是单条消息？应该是msg列表，因为一次对话可能包含多条消息。
	val, err := c.rdb.Get(ctx, key).Result()
	if err != nil {
		return nil, err
	}

	var msgs []model.ChatMessage
	// 从 Redis 获取到的数据是一个 JSON 字符串，需要将其反序列化为 []model.ChatMessage。
	if err := json.Unmarshal([]byte(val), &msgs); err != nil {
		return nil, err
	}
	return msgs, nil
}

func (c *redisHistoryCache) SetRecentMessages(ctx context.Context, sessionID string, messages []model.ChatMessage, ttl time.Duration) error {
	key := recentMessagesKey(sessionID)

	data, err := json.Marshal(messages)
	if err != nil {
		return err
	}
	// 为什么不用hash？因为我们需要存储的是一个消息列表，而不是单个消息的属性。使用hash的话，我们需要为每条消息创建一个hash key，
	// 这样会增加复杂度和存储空间的使用。而直接将整个消息列表序列化成JSON字符串存储在一个key中，更加简单和高效。
	// rdb.set的data格式是string，所以需要将messages序列化成json字符串。ttl是过期时间，单位是秒。
	// string和json的区别在于，string是一个简单的字符串，而json是一种数据交换格式，可以表示复杂的数据结构。
	// 在这里，我们需要将消息列表转换成json字符串来存储在redis中，这样我们就可以方便地将其反序列化回消息列表。
	return c.rdb.Set(ctx, key, data, ttl).Err()
}

func (c *redisHistoryCache) DeleteRecentMessages(ctx context.Context, sessionID string) error {
	key := recentMessagesKey(sessionID)
	return c.rdb.Del(ctx, key).Err()
}

func recentMessagesKey(sessionID string) string {
	// chat:session:{sessionID}:recent_msgs
	return fmt.Sprintf("chat:session:%s:recent_msgs", sessionID)
}
