package cache

import (
	"context"
	"time"
)

// RuntimeStore 定义了运行时状态存储的接口。
type RuntimeStore interface {
	// 目的是什么？在对话过程中，我们需要存储一些临时的运行时状态信息，比如当前会话是否活跃，是否需要停止等。这些信息对于管理对话的生命周期和控制对话流程非常重要。
	// 通过定义 RuntimeStore 接口，我们可以抽象出这些运行时状态的存储和管理逻辑，使得我们的系统更加模块化和可维护。
	SetSessionActive(ctx context.Context, sessionID string, ttl time.Duration) error
	// IsSessionActive 用于检查会话是否处于活跃状态。它的实现可能会查询一个缓存或者数据库，看看对应的 sessionID 是否存在并且没有过期。
	// 这个方法对于控制对话流程非常重要，因为我们可能需要根据会话的活跃状态来决定是否继续处理用户的请求，或者是否需要清理一些资源。
	IsSessionActive(ctx context.Context, sessionID string) (bool, error)
	RenewSessionTTL(ctx context.Context, sessionID string, ttl time.Duration) error
	// SetStopFlag 用于设置一个停止标志，表示当前会话需要停止处理后续的请求。这个方法可能会在某些特定的情况下被调用，比如用户明确表示要结束
	// 对话，或者系统检测到某些异常情况需要强制结束对话。
	SetStopFlag(ctx context.Context, sessionID string, requestID string, ttl time.Duration) error
	GetStopFlag(ctx context.Context, sessionID string, requestID string) (bool, error)
}
