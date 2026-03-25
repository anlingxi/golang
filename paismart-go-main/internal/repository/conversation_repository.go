// Package repository 提供了数据访问层的实现。
package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"pai-smart-go/internal/model"
	"time"

	"github.com/go-redis/redis/v8"
)

// ConversationRepository 定义了对话历史记录的操作接口。
type ConversationRepository interface {
	GetOrCreateConversationID(ctx context.Context, userID uint) (string, error)
	GetConversationHistory(ctx context.Context, conversationID string) ([]model.ChatMessage, error)
	UpdateConversationHistory(ctx context.Context, conversationID string, messages []model.ChatMessage) error
	GetAllUserConversationMappings(ctx context.Context) (map[uint]string, error)
}

type redisConversationRepository struct {
	redisClient *redis.Client
}

// NewConversationRepository 创建一个新的 ConversationRepository 实例。
func NewConversationRepository(redisClient *redis.Client) ConversationRepository {
	return &redisConversationRepository{redisClient: redisClient}
}

// GetOrCreateConversationID 获取或创建一个新的对话ID。
func (r *redisConversationRepository) GetOrCreateConversationID(ctx context.Context, userID uint) (string, error) {
	// 格式：user:{userID}:current_conversation -> conversationID
	// 这个设计的好处是可以快速通过 userID 获取当前对话ID，缺点是如果用户在多个设备登录可能会有冲突（后续可以改为 session 维度）
	// 这个key也不唯一啊，后续可以改为 session 维度，或者直接在前端生成 uuid 传过来
	// 如何改为session维度？可以在前端生成一个 uuid 作为 session ID，连接时携带过来，
	// 后端直接使用这个 session ID 作为 conversation ID，并且在 Redis 中维护 sessionID -> userID 的映射
	// （session:{sessionID}:user -> userID），这样就可以支持同一用户多设备登录了。
	// 可是这个是获取当前对话ID，如果前端每次连接都生成新的 session ID，那就相当于每次都创建新对话了，历史消息怎么获取？
	// 可以在前端存储当前 conversation ID，下次连接时携带过来，如果没有再创建新的。
	// session和conversation是一个意思吗？不完全是，session更偏向于连接层面的概念，conversation更偏向于对话内容的概念。
	// 一个用户可以有多个 session（比如多设备登录），但通常只有一个 active conversation（除非用户主动切换）。
	// 所以前端可以在 localStorage 里存储当前 active conversation ID，下次连接时携带过来，如果没有再创建新的。
	userKey := fmt.Sprintf("user:%d:current_conversation", userID)
	convID, err := r.redisClient.Get(ctx, userKey).Result()
	if err == redis.Nil {
		// generate uuid-like using timestamp+userID (avoid heavy deps)
		// 格式：{timestamp}-{userID}
		convID = fmt.Sprintf("%d-%d", time.Now().UnixNano(), userID)
		if err := r.redisClient.Set(ctx, userKey, convID, 7*24*time.Hour).Err(); err != nil {
			return "", fmt.Errorf("failed to set conversation id: %w", err)
		}
		return convID, nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to get conversation id: %w", err)
	}
	return convID, nil
}

// GetConversationHistory 从 Redis 获取对话历史记录。
func (r *redisConversationRepository) GetConversationHistory(ctx context.Context, conversationID string) ([]model.ChatMessage, error) {
	key := fmt.Sprintf("conversation:%s", conversationID)
	jsonData, err := r.redisClient.Get(ctx, key).Result()
	if err == redis.Nil {
		return []model.ChatMessage{}, nil // No history yet
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get conversation history: %w", err)
	}
	var messages []model.ChatMessage
	err = json.Unmarshal([]byte(jsonData), &messages)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal conversation history: %w", err)
	}
	return messages, nil
}

// UpdateConversationHistory 在 Redis 中更新对话历史记录。
func (r *redisConversationRepository) UpdateConversationHistory(ctx context.Context, conversationID string, messages []model.ChatMessage) error {
	key := fmt.Sprintf("conversation:%s", conversationID)
	// 保留最近 20 条
	if len(messages) > 20 {
		messages = messages[len(messages)-20:]
	}
	jsonData, err := json.Marshal(messages)
	if err != nil {
		return fmt.Errorf("failed to marshal conversation history: %w", err)
	}
	err = r.redisClient.Set(ctx, key, jsonData, 7*24*time.Hour).Err()
	if err != nil {
		return fmt.Errorf("failed to set conversation history: %w", err)
	}
	return nil
}

// GetAllUserConversationMappings returns map[userID]conversationID by scanning user:*:current_conversation
// 这个接口主要用于系统重启后从 Redis 恢复内存中的 userID -> conversationID 映射关系，确保 AIHelper 能够正确加载历史记录。
func (r *redisConversationRepository) GetAllUserConversationMappings(ctx context.Context) (map[uint]string, error) {
	keys, err := r.redisClient.Keys(ctx, "user:*:current_conversation").Result()
	if err != nil {
		return nil, fmt.Errorf("failed to scan user conversation keys: %w", err)
	}
	result := make(map[uint]string)
	for _, k := range keys {
		// k format: user:{uid}:current_conversation
		var uid uint
		_, scanErr := fmt.Sscanf(k, "user:%d:current_conversation", &uid)
		if scanErr != nil {
			continue
		}
		convID, getErr := r.redisClient.Get(ctx, k).Result()
		if getErr != nil {
			continue
		}
		result[uid] = convID
	}
	return result, nil
}
