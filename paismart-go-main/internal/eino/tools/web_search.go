package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"pai-smart-go/internal/config"
)

type tavilyClient struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

func newTavilyClient(cfg config.EinoWebSearchToolConfig) (*tavilyClient, error) {
	provider := strings.TrimSpace(strings.ToLower(cfg.Provider))
	if provider == "" {
		provider = "tavily"
	}
	if provider != "tavily" {
		return nil, fmt.Errorf("web_search 当前仅支持 tavily provider")
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, fmt.Errorf("web_search 已启用，但 tavily api_key 为空")
	}

	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		baseURL = "https://api.tavily.com"
	}

	httpClient, err := newHTTPClient(cfg.TimeoutSeconds, cfg.ProxyURL)
	if err != nil {
		return nil, err
	}

	return &tavilyClient{
		baseURL: baseURL,
		apiKey:  strings.TrimSpace(cfg.APIKey),
		client:  httpClient,
	}, nil
}

func (c *tavilyClient) Search(ctx context.Context, query string, maxResults int, searchDepth string) (*tavilySearchResponse, error) {
	payload := map[string]any{
		"query":               query,
		"max_results":         maxResults,
		"search_depth":        searchDepth,
		"include_answer":      false,
		"include_raw_content": false,
		"include_images":      false,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("构造 tavily 请求失败: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/search", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("创建 tavily 请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("调用 tavily 失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取 tavily 响应失败: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("tavily 返回错误状态 %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var result tavilySearchResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("解析 tavily 响应失败: %w", err)
	}

	return &result, nil
}

type tavilySearchResponse struct {
	Query   string               `json:"query"`
	Results []tavilySearchResult `json:"results"`
}

type tavilySearchResult struct {
	Title   string  `json:"title"`
	URL     string  `json:"url"`
	Content string  `json:"content"`
	Score   float64 `json:"score"`
}

type WebSearchTool struct {
	client            *tavilyClient
	defaultMaxResults int
	maxMaxResults     int
	searchDepth       string
}

func NewWebSearchTool(client *tavilyClient, cfg config.EinoWebSearchToolConfig) *WebSearchTool {
	defaultMaxResults := cfg.DefaultMaxResults
	if defaultMaxResults <= 0 {
		defaultMaxResults = 5
	}
	maxMaxResults := cfg.MaxMaxResults
	if maxMaxResults <= 0 {
		maxMaxResults = 10
	}
	if maxMaxResults < defaultMaxResults {
		maxMaxResults = defaultMaxResults
	}

	searchDepth := strings.TrimSpace(strings.ToLower(cfg.SearchDepth))
	if searchDepth == "" {
		searchDepth = "basic"
	}

	return &WebSearchTool{
		client:            client,
		defaultMaxResults: defaultMaxResults,
		maxMaxResults:     maxMaxResults,
		searchDepth:       searchDepth,
	}
}

func (t *WebSearchTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "web_search",
		Desc: "搜索公开互联网信息，适合查询知识库外的最新资料、官方文档入口、新闻或项目官网。",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"query": {
				Type:     schema.String,
				Desc:     "搜索关键词或问题描述",
				Required: true,
			},
			"max_results": {
				Type:     schema.Integer,
				Desc:     fmt.Sprintf("返回结果数，默认 %d，最大 %d", t.defaultMaxResults, t.maxMaxResults),
				Required: false,
			},
		}),
	}, nil
}

func (t *WebSearchTool) InvokableRun(ctx context.Context, args string, opts ...tool.Option) (string, error) {
	var params struct {
		Query      string `json:"query"`
		MaxResults int    `json:"max_results"`
	}
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", fmt.Errorf("参数解析失败: %w", err)
	}

	query := strings.TrimSpace(params.Query)
	if query == "" {
		return "", fmt.Errorf("query 不能为空")
	}

	maxResults := params.MaxResults
	if maxResults <= 0 {
		maxResults = t.defaultMaxResults
	}
	maxResults = clampInt(maxResults, 1, t.maxMaxResults)

	result, err := t.client.Search(ctx, query, maxResults, t.searchDepth)
	if err != nil {
		return "", err
	}

	return marshalToolResult(map[string]any{
		"query":        query,
		"result_count": len(result.Results),
		"results":      result.Results,
	})
}
