package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

type GitQueryTool struct {
	client *gitHubClient
}

func NewGitQueryTool(client *gitHubClient) *GitQueryTool {
	return &GitQueryTool{client: client}
}

func (t *GitQueryTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "git_query",
		Desc: "查询 GitHub 仓库信息。当前仅支持 allow_repos 白名单中的 GitHub 仓库，且只读。",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"action": {
				Type:     schema.String,
				Desc:     "操作类型",
				Enum:     []string{"get_issue", "get_pull_request", "list_tree", "get_file"},
				Required: true,
			},
			"repo": {
				Type:     schema.String,
				Desc:     "GitHub 仓库，格式 owner/repo，必须在 allow_repos 白名单内",
				Required: true,
			},
			"number": {
				Type:     schema.Integer,
				Desc:     "issue 或 pull request 编号；当 action 为 get_issue 或 get_pull_request 时必填",
				Required: false,
			},
			"path": {
				Type:     schema.String,
				Desc:     "目录或文件路径；list_tree/get_file 时使用。list_tree 不传则列根目录",
				Required: false,
			},
			"ref": {
				Type:     schema.String,
				Desc:     "分支、tag 或 commit SHA，可选",
				Required: false,
			},
			"limit": {
				Type:     schema.Integer,
				Desc:     "仅 list_tree 使用，默认 50，最大 200",
				Required: false,
			},
		}),
	}, nil
}

func (t *GitQueryTool) InvokableRun(ctx context.Context, args string, opts ...tool.Option) (string, error) {
	var params struct {
		Action string `json:"action"`
		Repo   string `json:"repo"`
		Number int    `json:"number"`
		Path   string `json:"path"`
		Ref    string `json:"ref"`
		Limit  int    `json:"limit"`
	}
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", fmt.Errorf("参数解析失败: %w", err)
	}

	action := strings.TrimSpace(strings.ToLower(params.Action))
	repo := strings.TrimSpace(params.Repo)
	if action == "" {
		return "", fmt.Errorf("action 不能为空")
	}

	var (
		result any
		err    error
	)

	switch action {
	case "get_issue":
		result, err = t.client.GetIssue(ctx, repo, params.Number)
	case "get_pull_request":
		result, err = t.client.GetPullRequest(ctx, repo, params.Number)
	case "list_tree":
		limit := params.Limit
		if limit <= 0 {
			limit = 50
		}
		limit = clampInt(limit, 1, 200)
		result, err = t.client.ListTree(ctx, repo, params.Path, params.Ref, limit)
	case "get_file":
		result, err = t.client.GetFile(ctx, repo, params.Path, params.Ref)
	default:
		return "", fmt.Errorf("不支持的 action: %s", action)
	}
	if err != nil {
		return "", err
	}

	return marshalToolResult(map[string]any{
		"action": action,
		"data":   result,
	})
}
