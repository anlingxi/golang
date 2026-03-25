package service

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"

	"pai-smart-go/internal/model"
	"pai-smart-go/pkg/log"
)

type AgentChatService struct {
	agent *adk.ChatModelAgent
}

func NewAgentChatService(agent *adk.ChatModelAgent) *AgentChatService {
	return &AgentChatService{agent: agent}
}

// Chat 同步接口，适合简单场景，或不需要流式输出的情况
func (s *AgentChatService) Chat(ctx context.Context, userMessage string) (string, error) {
	log.Infof("[AgentChatService] 开始, query=%s", userMessage)

	input := &adk.AgentInput{
		Messages: []*schema.Message{
			schema.UserMessage(userMessage),
		},
		EnableStreaming: false,
	}

	iter := s.agent.Run(ctx, input)

	var finalAnswer string
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		if event.Err != nil {
			return "", fmt.Errorf("Agent执行错误: %w", event.Err)
		}
		// AgentEvent没有Message字段，通过Output.MessageOutput取
		if event.Output == nil || event.Output.MessageOutput == nil {
			continue
		}
		mv := event.Output.MessageOutput
		// 只取Assistant角色的最终回答，跳过Tool调用的中间结果
		if mv.Role != schema.Assistant {
			continue
		}
		msg, err := mv.GetMessage()
		if err != nil {
			continue
		}
		if msg != nil && msg.Content != "" {
			finalAnswer = msg.Content // 取最后一条Assistant消息
		}
	}

	if finalAnswer == "" {
		return "", fmt.Errorf("Agent未返回任何内容")
	}

	log.Infof("[AgentChatService] 完成")
	return finalAnswer, nil
}

func (s *AgentChatService) ChatStream(
	ctx context.Context,
	user *model.User,
	userMessage string,
	writer StreamWriter,
	shouldStop func() bool,
) error {
	input := &adk.AgentInput{
		Messages: []*schema.Message{
			schema.UserMessage(userMessage),
		},
		EnableStreaming: true,
	}

	iter := s.agent.Run(ctx, input)

	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		if shouldStop != nil && shouldStop() {
			break
		}
		if event.Err != nil {
			break
		}
		if event.Output == nil || event.Output.MessageOutput == nil {
			continue
		}
		mv := event.Output.MessageOutput
		if mv.Role != schema.Assistant {
			continue
		}
		// 流式模式下从MessageStream读取
		if mv.IsStreaming && mv.MessageStream != nil {
			for {
				chunk, err := mv.MessageStream.Recv()
				if err != nil {
					break
				}
				if chunk.Content != "" && writer != nil {
					writer.WriteChunk(chunk.Content)
				}
			}
		} else if mv.Message != nil && mv.Message.Content != "" {
			if writer != nil {
				writer.WriteChunk(mv.Message.Content)
			}
		}
	}

	if writer != nil {
		writer.WriteDone()
	}
	return nil
}
