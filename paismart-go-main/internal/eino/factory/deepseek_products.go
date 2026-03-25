package factory

import (
	"context"
	"fmt"

	einoembedding "github.com/cloudwego/eino/components/embedding"

	"pai-smart-go/internal/config"
	einochat "pai-smart-go/internal/eino/chat"
	einotypes "pai-smart-go/internal/eino/types"
	"pai-smart-go/internal/service"
)

// DefaultMessageConverter 是默认消息转换器实现。
type DefaultMessageConverter struct{}

func NewDefaultMessageConverter() einotypes.MessageConverter {
	return &DefaultMessageConverter{}
}

func (c *DefaultMessageConverter) ToSchemaMessages(msgs []einotypes.BusinessChatMessage) ([]einotypes.SchemaChatMessage, error) {
	return einotypes.ConvertBusinessMessages(msgs)
}

// DeepSeekProducts 是 DeepSeek provider 对应的一整组 AI 产品。
type DeepSeekProducts struct {
	chatModel interface { /* service.ChatModel */
	}
	messageConverter einotypes.MessageConverter

	// 改成官方 Eino Embedder
	einoEmbedder einoembedding.Embedder
	capabilities ModelCapabilities
}

// NewDeepSeekProducts 创建 DeepSeek 产品族。
func NewDeepSeekProducts(ctx context.Context, cfg config.EinoConfig) (AIProducts, error) {
	chatModel, err := einochat.NewDeepSeekChatModel(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create DeepSeek chat model: %w", err)
	}

	// 第一批先把接口打通。
	// 第二批在这里接真实的 DeepSeek / OpenAI-compatible Eino embedder。
	var embedder einoembedding.Embedder

	return &DeepSeekProducts{
		chatModel:        chatModel,
		messageConverter: NewDefaultMessageConverter(),
		einoEmbedder:     embedder,
		capabilities: ModelCapabilities{
			SupportsStreaming: true,
			SupportsSearch:    false,
			SupportsEmbedding: embedder != nil,
		},
	}, nil
}

func (p *DeepSeekProducts) ChatModel() service.ChatModel {
	return p.chatModel.(service.ChatModel)
}

func (p *DeepSeekProducts) MessageConverter() einotypes.MessageConverter {
	return p.messageConverter
}

func (p *DeepSeekProducts) EinoEmbedder() einoembedding.Embedder {
	return p.einoEmbedder
}

func (p *DeepSeekProducts) Capabilities() ModelCapabilities {
	return p.capabilities
}
