package history

import (
	"fmt"
	"pai-smart-go/internal/model"
	"time"
)

func toChatMessages(msgs []model.ConversationMsg) []model.ChatMessage {
	result := make([]model.ChatMessage, 0, len(msgs))
	for _, msg := range msgs {
		result = append(result, model.ChatMessage{
			Role:      string(msg.Role),
			Content:   msg.Content,
			Provider:  msg.Provider,
			Model:     msg.Model,
			Timestamp: msg.CreatedAt,
		})
	}
	return result
}

func buildConversationMsgsFromTurn(req PersistTurnRequest, baseSequence int64) []*model.ConversationMsg {
	now := req.OccurredAt
	if now.IsZero() {
		now = time.Now()
	}

	userMsg := &model.ConversationMsg{
		ID:          generateMsgID(),
		SessionID:   req.SessionID,
		TurnID:      req.TurnID,
		RequestID:   req.RequestID,
		UserID:      req.UserID,
		Role:        model.MessageRoleUser,
		Content:     req.UserMessage.Content,
		ContentType: "text",
		Sequence:    baseSequence + 1,
		Status:      model.MessageStatusCompleted,
		Provider:    req.Provider,
		Model:       req.Model,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	assistantMsg := &model.ConversationMsg{
		ID:          generateMsgID(),
		SessionID:   req.SessionID,
		TurnID:      req.TurnID,
		RequestID:   req.RequestID,
		UserID:      req.UserID,
		Role:        model.MessageRoleAssistant,
		Content:     req.AssistantMessage.Content,
		ContentType: "text",
		Sequence:    baseSequence + 2,
		Status:      buildAssistantMessageStatus(req.RunMeta),
		Provider:    req.Provider,
		Model:       req.Model,
		TokenInput:  req.RunMeta.InputTokens,
		TokenOutput: req.RunMeta.OutputTokens,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	return []*model.ConversationMsg{userMsg, assistantMsg}
}

func buildTurnFromRequest(req PersistTurnRequest) *model.ConversationTurn {
	now := req.OccurredAt
	if now.IsZero() {
		now = time.Now()
	}

	status := model.TurnStatusCompleted
	if req.RunMeta.IsInterrupted {
		status = model.TurnStatusInterrupted
	}
	if req.RunMeta.ErrorMessage != "" {
		status = model.TurnStatusFailed
	}

	return &model.ConversationTurn{
		ID:           req.TurnID,
		SessionID:    req.SessionID,
		RequestID:    req.RequestID,
		UserID:       req.UserID,
		Status:       status,
		StopReason:   req.RunMeta.StopReason,
		ErrorMessage: req.RunMeta.ErrorMessage,
		StartedAt:    now,
		CompletedAt:  ptrTime(now),
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

func buildAssistantMessageStatus(meta RunMeta) model.MessageStatus {
	if meta.ErrorMessage != "" {
		return model.MessageStatusFailed
	}
	if meta.IsInterrupted {
		return model.MessageStatusInterrupted
	}
	return model.MessageStatusCompleted
}

func ptrTime(t time.Time) *time.Time {
	return &t
}

func generateMsgID() string {
	return fmt.Sprintf("msg_%d", time.Now().UnixNano())
}
