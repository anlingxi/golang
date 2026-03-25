package chat

import (
	"fmt"

	"pai-smart-go/internal/config"
	einocallbacks "pai-smart-go/internal/eino/callbacks"
	"pai-smart-go/internal/service"
)

func NewQwenChatModel(
	cfg config.EinoConfig,
	callbackManager *einocallbacks.Manager,
) (service.ChatModel, error) {
	return nil, fmt.Errorf("qwen chat model is not implemented yet")
}
