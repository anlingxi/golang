package parser

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path/filepath"

	einoparser "github.com/cloudwego/eino/components/document/parser"
	"github.com/cloudwego/eino/schema"

	"pai-smart-go/pkg/tika"
)

type TikaParser struct {
	client *tika.Client
}

func NewTikaParser(client *tika.Client) *TikaParser {
	return &TikaParser{client: client}
}

// 确保实现官方 Parser 接口
var _ einoparser.Parser = (*TikaParser)(nil)

func (p *TikaParser) Parse(ctx context.Context, reader io.Reader, opts ...einoparser.Option) ([]*schema.Document, error) {
	if p.client == nil {
		return nil, fmt.Errorf("tika client is nil")
	}

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read source failed: %w", err)
	}

	parsedOpts := &einoparser.Options{}
	for _, opt := range opts {
		opt(parsedOpts)
	}

	fileName := "unknown"
	if parsedOpts.URI != "" {
		fileName = filepath.Base(parsedOpts.URI)
	}

	text, err := p.client.ExtractText(bytes.NewReader(data), fileName)
	if err != nil {
		return nil, fmt.Errorf("tika extract text failed: %w", err)
	}

	if text == "" {
		return []*schema.Document{}, nil
	}

	meta := map[string]any{
		"parser": "tika",
		"source": parsedOpts.URI,
	}
	for k, v := range parsedOpts.ExtraMeta {
		meta[k] = v
	}

	return []*schema.Document{
		{
			ID:       fileName,
			Content:  text,
			MetaData: meta,
		},
	}, nil
}
