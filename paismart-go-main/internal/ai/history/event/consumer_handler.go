package event

import "context"

// 轻量事件 handler，先保留。
type ConsumerHandler interface {
	HandleTurnPersisted(ctx context.Context, evt TurnPersistedEvent) error
}

// RabbitMQ 持久化任务 handler。
type PersistTaskHandler interface {
	HandlePersistTurnTask(ctx context.Context, task PersistTurnTask) error
}
