package history

import (
	"context"
	"pai-smart-go/internal/model"
)

type Loader interface {
	LoadHistory(ctx context.Context, opts LoadHistoryOptions) ([]model.ChatMessage, error)
}
