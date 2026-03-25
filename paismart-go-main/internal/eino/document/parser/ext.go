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
