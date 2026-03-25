package tools

import (
	"fmt"

	"github.com/cloudwego/eino/components/tool"

	"pai-smart-go/internal/config"
	"pai-smart-go/internal/model"
	"pai-smart-go/internal/service"
)

type Builder interface {
	Build(user *model.User) ([]tool.BaseTool, error)
}

type builder struct {
	cfg          config.EinoAgentToolsConfig
	searchSvc    service.SearchService
	documentSvc  service.DocumentService
	tavilyClient *tavilyClient
	gitHubClient *gitHubClient
}

func NewBuilder(
	cfg config.EinoAgentToolsConfig,
	searchSvc service.SearchService,
	documentSvc service.DocumentService,
) (Builder, error) {
	b := &builder{
		cfg:         cfg,
		searchSvc:   searchSvc,
		documentSvc: documentSvc,
	}

	if cfg.WebSearch.Enabled {
		client, err := newTavilyClient(cfg.WebSearch)
		if err != nil {
			return nil, err
		}
		b.tavilyClient = client
	}

	if cfg.GitQuery.Enabled {
		client, err := newGitHubClient(cfg.GitQuery)
		if err != nil {
			return nil, err
		}
		b.gitHubClient = client
	}

	return b, nil
}

func (b *builder) Build(user *model.User) ([]tool.BaseTool, error) {
	if user == nil {
		return nil, fmt.Errorf("user is nil")
	}

	var tools []tool.BaseTool

	if b.cfg.KnowledgeSearch.Enabled {
		if b.searchSvc == nil {
			return nil, fmt.Errorf("knowledge_search 已启用，但 search service 未注入")
		}
		tools = append(tools, NewKnowledgeSearchTool(b.searchSvc, user, b.cfg.KnowledgeSearch))
	}

	if b.cfg.ListDocuments.Enabled {
		if b.documentSvc == nil {
			return nil, fmt.Errorf("list_documents 已启用，但 document service 未注入")
		}
		tools = append(tools, NewListDocumentsTool(b.documentSvc, user, b.cfg.ListDocuments))
	}

	if b.cfg.CurrentTime.Enabled {
		tools = append(tools, NewCurrentTimeTool(b.cfg.CurrentTime))
	}

	if b.cfg.WebSearch.Enabled && b.tavilyClient != nil {
		tools = append(tools, NewWebSearchTool(b.tavilyClient, b.cfg.WebSearch))
	}

	if b.cfg.GitQuery.Enabled && b.gitHubClient != nil {
		tools = append(tools, NewGitQueryTool(b.gitHubClient))
	}

	return tools, nil
}
