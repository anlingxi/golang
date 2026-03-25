package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"pai-smart-go/internal/config"
)

type CurrentTimeTool struct {
	defaultTimezone string
}

func NewCurrentTimeTool(cfg config.EinoCurrentTimeToolConfig) *CurrentTimeTool {
	defaultTimezone := strings.TrimSpace(cfg.DefaultTimezone)
	if defaultTimezone == "" {
		defaultTimezone = "Asia/Shanghai"
	}

	return &CurrentTimeTool{defaultTimezone: defaultTimezone}
}

func (t *CurrentTimeTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "get_current_time",
		Desc: "获取当前时间。适合回答今天、现在、截止时间、时区换算等问题。",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"timezone": {
				Type:     schema.String,
				Desc:     fmt.Sprintf("IANA 时区名称，例如 Asia/Shanghai、America/New_York；默认 %s", t.defaultTimezone),
				Required: false,
			},
		}),
	}, nil
}

func (t *CurrentTimeTool) InvokableRun(ctx context.Context, args string, opts ...tool.Option) (string, error) {
	var params struct {
		Timezone string `json:"timezone"`
	}
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", fmt.Errorf("参数解析失败: %w", err)
	}

	timezone := strings.TrimSpace(params.Timezone)
	if timezone == "" {
		timezone = t.defaultTimezone
	}

	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return "", fmt.Errorf("无效时区: %s", timezone)
	}

	now := time.Now().In(loc)
	return marshalToolResult(map[string]any{
		"timezone":      timezone,
		"rfc3339":       now.Format(time.RFC3339),
		"unix":          now.Unix(),
		"date":          now.Format("2006-01-02"),
		"time":          now.Format("15:04:05"),
		"weekday":       now.Weekday().String(),
		"formatted":     now.Format("2006-01-02 15:04:05 MST"),
		"utc_offset":    now.Format("-07:00"),
		"utc_formatted": now.UTC().Format(time.RFC3339),
	})
}
