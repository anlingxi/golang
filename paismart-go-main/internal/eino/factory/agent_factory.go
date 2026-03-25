package factory

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
)

// NewKnowledgeAgent 创建知识库问答 Agent。
// products 提供底层 Eino ChatModel；tools 由调用方在请求上下文中组装（含用户权限绑定）。
func NewKnowledgeAgent(
	ctx context.Context,
	products AIProducts,
	tools []tool.BaseTool,
) (*adk.ChatModelAgent, error) {
	agent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:        "knowledge-agent",
		Description: "知识库问答助手",
		Model:       products.EinoChatModel(),
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: tools,
			},
		},
		MaxIterations: 5,
	})
	if err != nil {
		return nil, fmt.Errorf("创建Agent失败: %w", err)
	}

	return agent, nil
}
