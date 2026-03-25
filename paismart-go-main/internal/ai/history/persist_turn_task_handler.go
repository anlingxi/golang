package history

import (
	"context"
	"pai-smart-go/internal/ai/history/event"
	"pai-smart-go/internal/repository"

	"gorm.io/gorm"
)

// persistTurnTaskHandler 处理 PersistTurnTask 任务，负责将对话轮次数据持久化到数据库
type persistTurnTaskHandler struct {
	turnRepo      repository.ConversationTurnRepository
	syncPersister Persister
}

func NewPersistTurnTaskHandler(
	turnRepo repository.ConversationTurnRepository,
	syncPersister Persister,
) event.PersistTaskHandler {
	return &persistTurnTaskHandler{
		turnRepo:      turnRepo,
		syncPersister: syncPersister,
	}
}

func (h *persistTurnTaskHandler) HandlePersistTurnTask(ctx context.Context, task event.PersistTurnTask) error {
	// 检查任务是否已经处理过
	// 幂等判断：如果 TurnID 不为空，说明这是一个更新任务，先查询数据库看是否存在该 TurnID 的记录，如果存在则说明已经处理过了，
	// 可以直接返回成功；如果查询出错且不是记录不存在的错误，则返回错误；如果记录不存在，则继续往下处理。
	if task.TurnID != "" {
		_, err := h.turnRepo.GetByID(ctx, task.TurnID)
		if err == nil {
			return nil
		}
		if err != nil && err != gorm.ErrRecordNotFound {
			return err
		}
	}

	req := PersistTurnRequest{
		UserID:           task.UserID,
		SessionID:        task.SessionID,
		TurnID:           task.TurnID,
		RequestID:        task.RequestID,
		Mode:             task.Mode,
		Provider:         task.Provider,
		Model:            task.Model,
		UserMessage:      task.UserMessage,
		AssistantMessage: task.AssistantMessage,
		RunMeta: RunMeta{
			InputTokens:   task.InputTokens,
			OutputTokens:  task.OutputTokens,
			LatencyMS:     task.LatencyMS,
			StopReason:    task.StopReason,
			ErrorMessage:  task.ErrorMessage,
			TraceID:       task.TraceID,
			IsInterrupted: task.IsInterrupted,
		},
		OccurredAt: task.OccurredAt,
	}

	return h.syncPersister.PersistTurn(ctx, req)
}
