package parser

import (
	"context"

	htmlparser "github.com/cloudwego/eino-ext/components/document/parser/html"
	pdfparser "github.com/cloudwego/eino-ext/components/document/parser/pdf"
	einoparser "github.com/cloudwego/eino/components/document/parser"

	"pai-smart-go/pkg/tika"
)

func NewExtParser(ctx context.Context, tikaClient *tika.Client) (einoparser.Parser, error) {
	textParser := einoparser.TextParser{}

	// 都是eino官方的
	htmlP, err := htmlparser.NewParser(ctx, &htmlparser.Config{})
	if err != nil {
		return nil, err
	}
	// 这里有一个关于是否每页解析为单独文档的选项，把每页变成单独的文档和把一个所有也当成一个文档有什么区别？
	pdfP, err := pdfparser.NewPDFParser(ctx, &pdfparser.Config{})
	if err != nil {
		return nil, err
	}

	tikaP := NewTikaParser(tikaClient)

	return einoparser.NewExtParser(ctx, &einoparser.ExtParserConfig{
		Parsers: map[string]einoparser.Parser{
			".html": htmlP,
			".htm":  htmlP,
			".pdf":  pdfP,
			".txt":  textParser,
			".md":   textParser,
			".doc":  tikaP,
			".docx": tikaP,
			".ppt":  tikaP,
			".pptx": tikaP,
			".xls":  tikaP,
			".xlsx": tikaP,
		},
		FallbackParser: tikaP,
	})
}
package parser

import (
	"context"

	htmlparser "github.com/cloudwego/eino-ext/components/document/parser/html"
	pdfparser "github.com/cloudwego/eino-ext/components/document/parser/pdf"
	einoparser "github.com/cloudwego/eino/components/document/parser"

	"pai-smart-go/pkg/tika"
)

func NewExtParser(ctx context.Context, tikaClient *tika.Client) (einoparser.Parser, error) {
	textParser := einoparser.TextParser{}

	htmlP, err := htmlparser.NewParser(ctx, &htmlparser.Config{})
	if err != nil {
		return nil, err
	}

	pdfP, err := pdfparser.NewPDFParser(ctx, &pdfparser.Config{})
	if err != nil {
		return nil, err
	}

	tikaP := NewTikaParser(tikaClient)

	return einoparser.NewExtParser(ctx, &einoparser.ExtParserConfig{
		Parsers: map[string]einoparser.Parser{
			".html": htmlP,
			".htm":  htmlP,
			".pdf":  pdfP,
			".txt":  textParser,
			".md":   textParser,
			".doc":  tikaP,
			".docx": tikaP,
			".ppt":  tikaP,
			".pptx": tikaP,
			".xls":  tikaP,
			".xlsx": tikaP,
		},
		FallbackParser: tikaP,
	})
}
