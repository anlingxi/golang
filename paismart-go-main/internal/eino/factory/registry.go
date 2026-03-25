package factory

import (
	"context"
	"fmt"
	"strings"

	"pai-smart-go/internal/config"
)

// Registry 负责根据 provider 返回具体工厂。
type Registry struct {
	cfg config.EinoConfig
}

// NewRegistry 创建工厂注册中心。
func NewRegistry(cfg config.EinoConfig) *Registry {
	return &Registry{cfg: cfg}
}

// GetFactory 根据 provider 获取具体工厂。
func (r *Registry) GetFactory(ctx context.Context) (AIFactory, error) {
	provider := strings.ToLower(strings.TrimSpace(r.cfg.ChatModel.Provider))
	if provider == "" {
		return nil, fmt.Errorf("eino chat model provider is empty")
	}

	switch provider {
	case "deepseek":
		return NewDeepSeekFactory(r.cfg), nil
	case "openai":
		return nil, fmt.Errorf("openai factory is not implemented yet")
	case "qwen":
		return nil, fmt.Errorf("qwen factory is not implemented yet")
	default:
		return nil, fmt.Errorf("unsupported eino provider: %s", provider)
	}
}
