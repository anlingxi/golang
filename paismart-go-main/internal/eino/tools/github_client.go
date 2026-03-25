package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"pai-smart-go/internal/config"
)

type gitHubClient struct {
	baseURL      string
	apiKey       string
	client       *http.Client
	maxFileChars int
	allowRepos   map[string]struct{}
}

func newGitHubClient(cfg config.EinoGitQueryToolConfig) (*gitHubClient, error) {
	if len(cfg.AllowRepos) == 0 {
		return nil, fmt.Errorf("git_query 已启用，但 allow_repos 为空")
	}

	httpClient, err := newHTTPClient(cfg.TimeoutSeconds, cfg.ProxyURL)
	if err != nil {
		return nil, err
	}

	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		baseURL = "https://api.github.com"
	}

	maxFileChars := cfg.MaxFileChars
	if maxFileChars <= 0 {
		maxFileChars = 12000
	}

	allowRepos := make(map[string]struct{}, len(cfg.AllowRepos))
	for _, repo := range cfg.AllowRepos {
		normalized := normalizeRepo(repo)
		if normalized == "" {
			continue
		}
		allowRepos[normalized] = struct{}{}
	}
	if len(allowRepos) == 0 {
		return nil, fmt.Errorf("git_query allow_repos 中没有有效仓库")
	}

	return &gitHubClient{
		baseURL:      baseURL,
		apiKey:       strings.TrimSpace(cfg.APIKey),
		client:       httpClient,
		maxFileChars: maxFileChars,
		allowRepos:   allowRepos,
	}, nil
}

func (c *gitHubClient) ensureAllowed(repo string) error {
	normalized := normalizeRepo(repo)
	if normalized == "" {
		return fmt.Errorf("repo 不能为空")
	}
	if _, ok := c.allowRepos[normalized]; !ok {
		return fmt.Errorf("仓库 %s 不在 allow_repos 白名单内", repo)
	}
	return nil
}

func (c *gitHubClient) newRequest(ctx context.Context, method, path string, query url.Values) (*http.Request, error) {
	endpoint := c.baseURL + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	return req, nil
}

func (c *gitHubClient) doJSON(req *http.Request, target any) error {
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("GitHub 请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取 GitHub 响应失败: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("GitHub 返回错误状态 %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("解析 GitHub 响应失败: %w", err)
	}
	return nil
}

func (c *gitHubClient) doBytes(req *http.Request) ([]byte, error) {
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GitHub 请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取 GitHub 响应失败: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("GitHub 返回错误状态 %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	return body, nil
}

func (c *gitHubClient) GetIssue(ctx context.Context, repo string, number int) (map[string]any, error) {
	if err := c.ensureAllowed(repo); err != nil {
		return nil, err
	}
	if number <= 0 {
		return nil, fmt.Errorf("issue number 必须大于 0")
	}

	req, err := c.newRequest(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/issues/%d", repo, number), nil)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Number    int    `json:"number"`
		Title     string `json:"title"`
		State     string `json:"state"`
		HTMLURL   string `json:"html_url"`
		Body      string `json:"body"`
		Comments  int    `json:"comments"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
		User      struct {
			Login string `json:"login"`
		} `json:"user"`
		Labels []struct {
			Name string `json:"name"`
		} `json:"labels"`
	}
	if err := c.doJSON(req, &resp); err != nil {
		return nil, err
	}

	labels := make([]string, 0, len(resp.Labels))
	for _, label := range resp.Labels {
		labels = append(labels, label.Name)
	}

	return map[string]any{
		"repo":       repo,
		"number":     resp.Number,
		"title":      resp.Title,
		"state":      resp.State,
		"url":        resp.HTMLURL,
		"author":     resp.User.Login,
		"comments":   resp.Comments,
		"labels":     labels,
		"body":       resp.Body,
		"created_at": resp.CreatedAt,
		"updated_at": resp.UpdatedAt,
	}, nil
}

func (c *gitHubClient) GetPullRequest(ctx context.Context, repo string, number int) (map[string]any, error) {
	if err := c.ensureAllowed(repo); err != nil {
		return nil, err
	}
	if number <= 0 {
		return nil, fmt.Errorf("pull request number 必须大于 0")
	}

	req, err := c.newRequest(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/pulls/%d", repo, number), nil)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Number       int    `json:"number"`
		Title        string `json:"title"`
		State        string `json:"state"`
		HTMLURL      string `json:"html_url"`
		Body         string `json:"body"`
		Draft        bool   `json:"draft"`
		Merged       bool   `json:"merged"`
		Additions    int    `json:"additions"`
		Deletions    int    `json:"deletions"`
		ChangedFiles int    `json:"changed_files"`
		CreatedAt    string `json:"created_at"`
		UpdatedAt    string `json:"updated_at"`
		User         struct {
			Login string `json:"login"`
		} `json:"user"`
		Base struct {
			Ref string `json:"ref"`
		} `json:"base"`
		Head struct {
			Ref string `json:"ref"`
		} `json:"head"`
	}
	if err := c.doJSON(req, &resp); err != nil {
		return nil, err
	}

	return map[string]any{
		"repo":          repo,
		"number":        resp.Number,
		"title":         resp.Title,
		"state":         resp.State,
		"url":           resp.HTMLURL,
		"author":        resp.User.Login,
		"draft":         resp.Draft,
		"merged":        resp.Merged,
		"base_ref":      resp.Base.Ref,
		"head_ref":      resp.Head.Ref,
		"additions":     resp.Additions,
		"deletions":     resp.Deletions,
		"changed_files": resp.ChangedFiles,
		"body":          resp.Body,
		"created_at":    resp.CreatedAt,
		"updated_at":    resp.UpdatedAt,
	}, nil
}

func (c *gitHubClient) ListTree(ctx context.Context, repo, path, ref string, limit int) (map[string]any, error) {
	if err := c.ensureAllowed(repo); err != nil {
		return nil, err
	}

	query := url.Values{}
	if strings.TrimSpace(ref) != "" {
		query.Set("ref", ref)
	}

	cleanPath := strings.Trim(strings.TrimSpace(path), "/")
	contentPath := fmt.Sprintf("/repos/%s/contents", repo)
	if cleanPath != "" {
		contentPath += "/" + encodeContentPath(cleanPath)
	}
	req, err := c.newRequest(ctx, http.MethodGet, contentPath, query)
	if err != nil {
		return nil, err
	}

	var entries []struct {
		Name    string `json:"name"`
		Path    string `json:"path"`
		Type    string `json:"type"`
		Size    int64  `json:"size"`
		HTMLURL string `json:"html_url"`
		SHA     string `json:"sha"`
	}
	if err := c.doJSON(req, &entries); err != nil {
		return nil, err
	}

	if limit > 0 && len(entries) > limit {
		entries = entries[:limit]
	}

	return map[string]any{
		"repo":           repo,
		"path":           cleanPath,
		"ref":            ref,
		"returned_count": len(entries),
		"entries":        entries,
	}, nil
}

func (c *gitHubClient) GetFile(ctx context.Context, repo, path, ref string) (map[string]any, error) {
	if err := c.ensureAllowed(repo); err != nil {
		return nil, err
	}
	cleanPath := strings.Trim(strings.TrimSpace(path), "/")
	if cleanPath == "" {
		return nil, fmt.Errorf("path 不能为空")
	}

	query := url.Values{}
	if strings.TrimSpace(ref) != "" {
		query.Set("ref", ref)
	}

	req, err := c.newRequest(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/contents/%s", repo, encodeContentPath(cleanPath)), query)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Name        string `json:"name"`
		Path        string `json:"path"`
		SHA         string `json:"sha"`
		Size        int    `json:"size"`
		HTMLURL     string `json:"html_url"`
		DownloadURL string `json:"download_url"`
		Type        string `json:"type"`
		Content     string `json:"content"`
		Encoding    string `json:"encoding"`
	}
	if err := c.doJSON(req, &resp); err != nil {
		return nil, err
	}

	if resp.Type != "file" {
		return nil, fmt.Errorf("path %s 不是文件", cleanPath)
	}

	content := resp.Content
	if resp.Encoding == "base64" {
		decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(resp.Content, "\n", ""))
		if err != nil {
			return nil, fmt.Errorf("解码文件内容失败: %w", err)
		}
		content = string(decoded)
	}

	truncated := false
	if len(content) > c.maxFileChars {
		content = content[:c.maxFileChars]
		truncated = true
	}

	return map[string]any{
		"repo":           repo,
		"path":           resp.Path,
		"ref":            ref,
		"sha":            resp.SHA,
		"size":           resp.Size,
		"url":            resp.HTMLURL,
		"download_url":   resp.DownloadURL,
		"truncated":      truncated,
		"max_file_chars": c.maxFileChars,
		"content":        content,
	}, nil
}

func encodeContentPath(path string) string {
	if path == "" {
		return ""
	}
	parts := strings.Split(path, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}
