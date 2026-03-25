package history

import (
	"context"
	"pai-smart-go/internal/model"
)

// Manager 管理历史记录的接口
type Manager interface {
	LoadHistory(ctx context.Context, opts LoadHistoryOptions) ([]model.ChatMessage, error)
	PersistTurn(ctx context.Context, req PersistTurnRequest) error
	AppendMessage(ctx context.Context, req PersistMessageRequest) error
	// RefreshRecent 刷新最近的历史记录
	RefreshRecent(ctx context.Context, sessionID string, messages []model.ChatMessage) error
}
