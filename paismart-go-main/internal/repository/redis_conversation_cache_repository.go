package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"pai-smart-go/internal/model"

	"github.com/go-redis/redis/v8"
)

type RedisConversationCacheRepository struct {
	rdb *redis.Client
}

func NewRedisConversationCacheRepository(rdb *redis.Client) *RedisConversationCacheRepository {
	return &RedisConversationCacheRepository{rdb: rdb}
}

func (r *RedisConversationCacheRepository) LoadRecent(ctx context.Context, conversationID string) ([]model.ChatMessage, error) {
	key := fmt.Sprintf("conversation:%s", conversationID)

	val, err := r.rdb.Get(ctx, key).Result()
	if err == redis.Nil {
		return []model.ChatMessage{}, nil
	}
	if err != nil {
		return nil, err
	}

	var history []model.ChatMessage
	if err := json.Unmarshal([]byte(val), &history); err != nil {
		return nil, err
	}
	return history, nil
}

func (r *RedisConversationCacheRepository) SaveRecent(ctx context.Context, conversationID string, messages []model.ChatMessage) error {
	key := fmt.Sprintf("conversation:%s", conversationID)

	const maxHistory = 20
	if len(messages) > maxHistory {
		messages = messages[len(messages)-maxHistory:]
	}

	b, err := json.Marshal(messages)
	if err != nil {
		return err
	}

	return r.rdb.Set(ctx, key, b, 0).Err()
}
