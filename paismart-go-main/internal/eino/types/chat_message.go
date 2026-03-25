package types

import (
	"fmt"
	"strings"

	"github.com/cloudwego/eino/schema"
)

// BusinessChatMessage 表示业务层消息，不直接依赖 service 包，避免循环依赖。
type BusinessChatMessage struct {
	Role    string
	Content string
}

// SchemaChatMessage 是对 Eino schema.Message 的轻量包装别名思维。
// 这里直接用 struct，避免业务层直接被外部类型绑死。
type SchemaChatMessage struct {
	Role    string
	Content string
}

// MessageConverter 是消息转换器接口。
// 后续不同 provider 如果需要不同消息适配，也可以做 provider-specific converter。
type MessageConverter interface {
	ToSchemaMessages(msgs []BusinessChatMessage) ([]SchemaChatMessage, error)
}

func normalizeRole(role string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "system":
		return "system", nil
	case "user":
		return "user", nil
	case "assistant":
		return "assistant", nil
	default:
		return "", fmt.Errorf("unsupported chat role: %s", role)
	}
}

// ConvertBusinessMessages 将业务消息转为中间 schema 消息结构。
func ConvertBusinessMessages(msgs []BusinessChatMessage) ([]SchemaChatMessage, error) {
	out := make([]SchemaChatMessage, 0, len(msgs))

	for i, msg := range msgs {
		role, err := normalizeRole(msg.Role)
		if err != nil {
			return nil, fmt.Errorf("normalize role at index %d failed: %w", i, err)
		}

		content := strings.TrimSpace(msg.Content)
		if content == "" {
			return nil, fmt.Errorf("message content at index %d is empty", i)
		}

		out = append(out, SchemaChatMessage{
			Role:    role,
			Content: content,
		})
	}

	return out, nil
}

// ToEinoSchemaMessages 把中间消息结构真正转成 Eino schema.Message。
func ToEinoSchemaMessages(msgs []SchemaChatMessage) ([]*schema.Message, error) {
	out := make([]*schema.Message, 0, len(msgs))

	for i, msg := range msgs {
		switch msg.Role {
		case "system":
			out = append(out, &schema.Message{
				Role:    schema.System,
				Content: msg.Content,
			})
		case "user":
			out = append(out, &schema.Message{
				Role:    schema.User,
				Content: msg.Content,
			})
		case "assistant":
			out = append(out, &schema.Message{
				Role:    schema.Assistant,
				Content: msg.Content,
			})
		default:
			return nil, fmt.Errorf("unsupported schema message role at index %d: %s", i, msg.Role)
		}
	}

	return out, nil
}
