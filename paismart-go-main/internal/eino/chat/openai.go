package chat

import (
	"fmt"

	"pai-smart-go/internal/config"
	einocallbacks "pai-smart-go/internal/eino/callbacks"
	"pai-smart-go/internal/service"
)

func NewOpenAIChatModel(
	cfg config.EinoConfig,
	callbackManager *einocallbacks.Manager,
) (service.ChatModel, error) {
	return nil, fmt.Errorf("openai chat model is not implemented yet")
}
