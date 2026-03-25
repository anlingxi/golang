package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"pai-smart-go/internal/model"
	"pai-smart-go/internal/service"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"pai-smart-go/internal/config"
)

type KnowledgeSearchTool struct {
	searchSvc   service.SearchService
	user        *model.User // 当前用户，用于权限过滤
	defaultTopK int
	maxTopK     int
}

// 注入用户信息防止权限问题，确保检索结果符合用户的访问权限
func NewKnowledgeSearchTool(
	svc service.SearchService,
	user *model.User,
	cfg config.EinoKnowledgeSearchToolConfig,
) *KnowledgeSearchTool {
	defaultTopK := cfg.DefaultTopK
	if defaultTopK <= 0 {
		defaultTopK = 5
	}
	maxTopK := cfg.MaxTopK
	if maxTopK <= 0 {
		maxTopK = 10
	}
	if maxTopK < defaultTopK {
		maxTopK = defaultTopK
	}

	return &KnowledgeSearchTool{
		searchSvc:   svc,
		user:        user,
		defaultTopK: defaultTopK,
		maxTopK:     maxTopK,
	}
}

// Info 描述这个工具是干什么的——LLM靠这个决定要不要调它
func (t *KnowledgeSearchTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "knowledge_search",
		Desc: "搜索内部知识库，返回与问题最相关的文档片段。" +
			"当用户询问知识库中的内容、需要查找文档资料、" +
			"或问题涉及专业知识时，使用此工具检索相关信息。",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"query": {
				Type:     schema.String,
				Desc:     "搜索关键词或问题描述，尽量简洁精炼，聚焦核心语义",
				Required: true,
			},
			"top_k": {
				Type: schema.Integer,
				Desc: fmt.Sprintf("返回文档数量，默认 %d，最大 %d", t.defaultTopK, t.maxTopK),
			},
		}),
	}, nil
}

// InvokableRun Eino框架在LLM决定调用此工具时自动调用
func (t *KnowledgeSearchTool) InvokableRun(ctx context.Context, args string, opts ...tool.Option) (string, error) {
	var params struct {
		Query string `json:"query"`
		TopK  int    `json:"top_k"`
	}
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", fmt.Errorf("参数解析失败: %w", err)
	}
	if params.TopK <= 0 {
		params.TopK = t.defaultTopK
	}
	params.TopK = clampInt(params.TopK, 1, t.maxTopK)

	// 直接复用你已有的混合检索，权限过滤逻辑完全不变
	results, err := t.searchSvc.HybridSearch(ctx, params.Query, params.TopK, t.user)
	if err != nil {
		return "", fmt.Errorf("知识库检索失败: %w", err)
	}

	return formatResults(results), nil
}

func formatResults(results []model.SearchResponseDTO) string {
	if len(results) == 0 {
		return "未找到相关文档，请尝试换个关键词搜索。"
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("共找到 %d 条相关文档：\n\n", len(results)))
	for i, r := range results {
		sb.WriteString(fmt.Sprintf("【文档%d】来源：%s（相关度：%.4f）\n", i+1, r.FileName, r.Score))
		sb.WriteString(r.TextContent)
		sb.WriteString("\n\n")
	}
	return sb.String()
}
