package factory

import (
	"context"
	"fmt"

	einoopenaiemb "github.com/cloudwego/eino-ext/components/embedding/openai"
	einoembedding "github.com/cloudwego/eino/components/embedding"
	fmodel "github.com/cloudwego/eino/components/model"

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
	chatModel        *einochat.DeepSeekChatModel
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
	deepSeekChatModel, ok := chatModel.(*einochat.DeepSeekChatModel)
	if !ok {
		return nil, fmt.Errorf("unexpected deepseek chat model type: %T", chatModel)
	}

	// 第一批先把接口打通。
	// 第二批在这里接真实的 DeepSeek / OpenAI-compatible Eino embedder。
	var embedder einoembedding.Embedder
	if cfg.Embedding.APIKey != "" && cfg.Embedding.BaseURL != "" {
		dims := cfg.Embedding.Dimensions
		embedder, err = einoopenaiemb.NewEmbedder(ctx, &einoopenaiemb.EmbeddingConfig{
			APIKey:     cfg.Embedding.APIKey,
			BaseURL:    cfg.Embedding.BaseURL,
			Model:      cfg.Embedding.Model,
			Dimensions: &dims,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create eino embedder: %w", err)
		}
	}

	return &DeepSeekProducts{
		chatModel:        deepSeekChatModel,
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
	return p.chatModel
}

func (p *DeepSeekProducts) EinoChatModel() fmodel.ToolCallingChatModel {
	return p.chatModel.EinoModel()
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
