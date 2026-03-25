package service

import (
	"context"
	"strings"
)

type MockChatModel struct{}

func NewMockChatModel() ChatModel {
	return &MockChatModel{}
}

func (m *MockChatModel) Stream(
	ctx context.Context,
	messages []ChatMessage,
	onChunk func(delta string) error,
) (string, error) {
	answer := "【占位回复】新的 AIHelper 会话主链已接通，下一步请接入真正的 Eino ChatModel。"

	parts := strings.Split(answer, "")
	for _, p := range parts {
		if err := onChunk(p); err != nil {
			return "", err
		}
	}

	return answer, nil
}
