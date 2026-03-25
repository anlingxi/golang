package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"pai-smart-go/internal/config"
	"pai-smart-go/internal/model"
	"pai-smart-go/internal/service"
)

type ListDocumentsTool struct {
	documentSvc  service.DocumentService
	user         *model.User
	defaultLimit int
	maxLimit     int
}

func NewListDocumentsTool(
	documentSvc service.DocumentService,
	user *model.User,
	cfg config.EinoListDocumentsToolConfig,
) *ListDocumentsTool {
	defaultLimit := cfg.DefaultLimit
	if defaultLimit <= 0 {
		defaultLimit = 20
	}
	maxLimit := cfg.MaxLimit
	if maxLimit <= 0 {
		maxLimit = 100
	}
	if maxLimit < defaultLimit {
		maxLimit = defaultLimit
	}

	return &ListDocumentsTool{
		documentSvc:  documentSvc,
		user:         user,
		defaultLimit: defaultLimit,
		maxLimit:     maxLimit,
	}
}

func (t *ListDocumentsTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "list_documents",
		Desc: "列出当前用户可访问或本人上传的知识库文档元信息。" +
			"适合在回答前先确认有哪些文档、文件是否已处理完成、或按文件名筛选候选资料。",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"scope": {
				Type:     schema.String,
				Desc:     "文档范围，可选 accessible 或 uploaded，默认 accessible",
				Enum:     []string{"accessible", "uploaded"},
				Required: false,
			},
			"keyword": {
				Type:     schema.String,
				Desc:     "按文件名或组织标签进行模糊过滤，可选",
				Required: false,
			},
			"limit": {
				Type:     schema.Integer,
				Desc:     fmt.Sprintf("返回条数，默认 %d，最大 %d", t.defaultLimit, t.maxLimit),
				Required: false,
			},
		}),
	}, nil
}

func (t *ListDocumentsTool) InvokableRun(ctx context.Context, args string, opts ...tool.Option) (string, error) {
	var params struct {
		Scope   string `json:"scope"`
		Keyword string `json:"keyword"`
		Limit   int    `json:"limit"`
	}
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", fmt.Errorf("参数解析失败: %w", err)
	}

	scope := strings.TrimSpace(strings.ToLower(params.Scope))
	if scope == "" {
		scope = "accessible"
	}
	if scope != "accessible" && scope != "uploaded" {
		return "", fmt.Errorf("scope 仅支持 accessible 或 uploaded")
	}

	limit := params.Limit
	if limit <= 0 {
		limit = t.defaultLimit
	}
	limit = clampInt(limit, 1, t.maxLimit)

	type documentItem struct {
		FileMD5        string `json:"file_md5"`
		FileName       string `json:"file_name"`
		TotalSize      int64  `json:"total_size"`
		IsPublic       bool   `json:"is_public"`
		OrgTag         string `json:"org_tag,omitempty"`
		OrgTagName     string `json:"org_tag_name,omitempty"`
		OwnedByCurrent bool   `json:"owned_by_current_user"`
		ProcessStatus  int    `json:"process_status"`
		ProcessStage   string `json:"process_stage,omitempty"`
		ParserName     string `json:"parser_name,omitempty"`
		SplitterName   string `json:"splitter_name,omitempty"`
		EmbeddingModel string `json:"embedding_model,omitempty"`
		ChunkCount     int    `json:"chunk_count"`
	}

	items := make([]documentItem, 0)
	keyword := strings.TrimSpace(strings.ToLower(params.Keyword))

	appendIfMatch := func(item documentItem) {
		if keyword != "" {
			target := strings.ToLower(item.FileName + " " + item.OrgTag + " " + item.OrgTagName)
			if !strings.Contains(target, keyword) {
				return
			}
		}
		items = append(items, item)
	}

	if scope == "uploaded" {
		files, err := t.documentSvc.ListUploadedFiles(t.user.ID)
		if err != nil {
			return "", fmt.Errorf("获取用户上传文档失败: %w", err)
		}
		for _, file := range files {
			appendIfMatch(documentItem{
				FileMD5:        file.FileMD5,
				FileName:       file.FileName,
				TotalSize:      file.TotalSize,
				IsPublic:       file.IsPublic,
				OrgTag:         file.OrgTag,
				OrgTagName:     file.OrgTagName,
				OwnedByCurrent: true,
				ProcessStatus:  file.ProcessStatus,
				ProcessStage:   file.ProcessStage,
				ParserName:     file.ParserName,
				SplitterName:   file.SplitterName,
				EmbeddingModel: file.EmbeddingModel,
				ChunkCount:     file.ChunkCount,
			})
		}
	} else {
		files, err := t.documentSvc.ListAccessibleFiles(t.user)
		if err != nil {
			return "", fmt.Errorf("获取可访问文档失败: %w", err)
		}
		for _, file := range files {
			appendIfMatch(documentItem{
				FileMD5:        file.FileMD5,
				FileName:       file.FileName,
				TotalSize:      file.TotalSize,
				IsPublic:       file.IsPublic,
				OrgTag:         file.OrgTag,
				OwnedByCurrent: file.UserID == t.user.ID,
				ProcessStatus:  file.ProcessStatus,
				ProcessStage:   file.ProcessStage,
				ParserName:     file.ParserName,
				SplitterName:   file.SplitterName,
				EmbeddingModel: file.EmbeddingModel,
				ChunkCount:     file.ChunkCount,
			})
		}
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].OwnedByCurrent != items[j].OwnedByCurrent {
			return items[i].OwnedByCurrent
		}
		return items[i].FileName < items[j].FileName
	})

	matchedCount := len(items)
	if len(items) > limit {
		items = items[:limit]
	}

	return marshalToolResult(map[string]any{
		"scope":          scope,
		"keyword":        params.Keyword,
		"matched_count":  matchedCount,
		"returned_count": len(items),
		"documents":      items,
	})
}
