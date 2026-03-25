package history

import (
	"context"
	"pai-smart-go/internal/model"
)

type NoopLoader struct{}

func NewNoopLoader() Loader {
	return &NoopLoader{}
}

func (l *NoopLoader) LoadHistory(ctx context.Context, opts LoadHistoryOptions) ([]model.ChatMessage, error) {
	return []model.ChatMessage{}, nil
}

type NoopPersister struct{}

func NewNoopPersister() Persister {
	return &NoopPersister{}
}

func (p *NoopPersister) PersistTurn(ctx context.Context, req PersistTurnRequest) error {
	return nil
}

func (p *NoopPersister) AppendMessage(ctx context.Context, req PersistMessageRequest) error {
	return nil
}

func (p *NoopPersister) RefreshRecent(ctx context.Context, sessionID string, messages []model.ChatMessage) error {
	return nil
}
