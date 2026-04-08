package chat

import (
	"context"
	"fmt"
	"io"
	"strings"

	openai "github.com/cloudwego/eino-ext/components/model/openai"
	einocb "github.com/cloudwego/eino/callbacks"
	fmodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"pai-smart-go/internal/config"
	einotypes "pai-smart-go/internal/eino/types"
	"pai-smart-go/internal/service"
)

type DeepSeekChatModel struct {
	provider  string
	modelName string
	chatModel *openai.ChatModel
}

func NewDeepSeekChatModel(cfg config.EinoConfig) (service.ChatModel, error) {
	modelCfg := cfg.ChatModel
	if strings.TrimSpace(modelCfg.Model) == "" {
		return nil, fmt.Errorf("deepseek model is empty")
	}
	if strings.TrimSpace(modelCfg.APIKey) == "" {
		return nil, fmt.Errorf("deepseek api key is empty")
	}
	if strings.TrimSpace(modelCfg.BaseURL) == "" {
		return nil, fmt.Errorf("deepseek base url is empty")
	}

	chatModel, err := openai.NewChatModel(context.Background(), &openai.ChatModelConfig{
		BaseURL: modelCfg.BaseURL,
		APIKey:  modelCfg.APIKey,
		Model:   modelCfg.Model,
	})
	if err != nil {
		return nil, fmt.Errorf("create deepseek chat model failed: %w", err)
	}

	return &DeepSeekChatModel{
		provider:  "deepseek",
		modelName: modelCfg.Model,
		chatModel: chatModel,
	}, nil
}

func (m *DeepSeekChatModel) EinoModel() fmodel.ToolCallingChatModel {
	return m.chatModel
}

func (m *DeepSeekChatModel) Stream(
	ctx context.Context,
	messages []service.ChatMessage,
	onChunk func(delta string) error,
) (answer string, err error) {
	if m.chatModel == nil {
		return "", fmt.Errorf("deepseek chat model is nil")
	}

	businessMsgs := make([]einotypes.BusinessChatMessage, 0, len(messages))
	for _, msg := range messages {
		businessMsgs = append(businessMsgs, einotypes.BusinessChatMessage{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	normalizedMsgs, err := einotypes.ConvertBusinessMessages(businessMsgs)
	if err != nil {
		return "", fmt.Errorf("convert business messages failed: %w", err)
	}

	schemaMsgs, err := einotypes.ToEinoSchemaMessages(normalizedMsgs)
	if err != nil {
		return "", fmt.Errorf("convert to eino schema messages failed: %w", err)
	}

	ctx = einocb.OnStart(ctx, &fmodel.CallbackInput{
		Messages: schemaMsgs,
		Config: &fmodel.Config{
			Model: m.modelName,
		},
	})

	defer func() {
		if err != nil {
			_ = einocb.OnError(ctx, err)
		}
	}()

	stream, err := m.chatModel.Stream(ctx, schemaMsgs)
	if err != nil {
		return "", fmt.Errorf("deepseek stream failed: %w", err)
	}
	defer stream.Close()

	var fullAnswer strings.Builder

	for {
		msg, recvErr := stream.Recv()
		if recvErr != nil {
			if recvErr == io.EOF {
				break
			}
			err = fmt.Errorf("receive deepseek stream chunk failed: %w", recvErr)
			return "", err
		}

		if msg == nil {
			continue
		}

		delta := extractMessageText(msg)
		if delta == "" {
			continue
		}

		fullAnswer.WriteString(delta)

		if onChunk != nil {
			if callErr := onChunk(delta); callErr != nil {
				err = callErr
				return "", err
			}
		}
	}

	answer = fullAnswer.String()

	_ = einocb.OnEnd(ctx, &fmodel.CallbackOutput{
		Message: &schema.Message{
			Role:    schema.Assistant,
			Content: answer,
		},
	})

	return answer, nil
}

func extractMessageText(msg *schema.Message) string {
	if msg == nil {
		return ""
	}
	return msg.Content
}
