package history

import (
	"context"
	"pai-smart-go/internal/model"
)

type Persister interface {
	PersistTurn(ctx context.Context, req PersistTurnRequest) error
	AppendMessage(ctx context.Context, req PersistMessageRequest) error
	RefreshRecent(ctx context.Context, sessionID string, messages []model.ChatMessage) error
}
