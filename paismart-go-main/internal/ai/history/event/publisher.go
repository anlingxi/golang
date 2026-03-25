package event

import "context"

// 轻量事件接口，先保留。
type Publisher interface {
	PublishTurnPersisted(ctx context.Context, evt TurnPersistedEvent) error
}

// RabbitMQ 异步持久化任务发布接口。
type PersistTaskPublisher interface {
	PublishPersistTurnTask(ctx context.Context, task PersistTurnTask) error
}
