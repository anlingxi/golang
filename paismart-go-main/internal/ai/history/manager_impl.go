package history

import (
	"context"
	"pai-smart-go/internal/model"
)

type historyManager struct {
	loader    Loader
	persister Persister
}

func NewManager(loader Loader, persister Persister) Manager {
	return &historyManager{
		loader:    loader,
		persister: persister,
	}
}

func (m *historyManager) LoadHistory(ctx context.Context, opts LoadHistoryOptions) ([]model.ChatMessage, error) {
	return m.loader.LoadHistory(ctx, opts)
}

func (m *historyManager) PersistTurn(ctx context.Context, req PersistTurnRequest) error {
	return m.persister.PersistTurn(ctx, req)
}

func (m *historyManager) AppendMessage(ctx context.Context, req PersistMessageRequest) error {
	return m.persister.AppendMessage(ctx, req)
}

func (m *historyManager) RefreshRecent(ctx context.Context, sessionID string, messages []model.ChatMessage) error {
	return m.persister.RefreshRecent(ctx, sessionID, messages)
}
