package transformer

import (
	"context"
	"fmt"

	einotransformer "github.com/cloudwego/eino/components/document"
	einoembedding "github.com/cloudwego/eino/components/embedding"

	semantic "github.com/cloudwego/eino-ext/components/document/transformer/splitter/semantic"
)

type SplitConfig struct {
	UseSemantic  bool
	MinChunkSize int
	Percentile   float64
	BufferSize   int

	ChunkSize    int
	ChunkOverlap int
}

func NewPrimaryTransformer(
	ctx context.Context,
	embedder einoembedding.Embedder,
	cfg SplitConfig,
) (einotransformer.Transformer, error) {
	if cfg.UseSemantic {
		if embedder == nil {
			return nil, fmt.Errorf("semantic splitter requires embedder")
		}
		return semantic.NewSplitter(ctx, &semantic.Config{
			Embedding:    embedder,
			BufferSize:   cfg.BufferSize,
			MinChunkSize: cfg.MinChunkSize,
			Separators:   []string{"\n\n", "\n", "。", "！", "？", ".", "!", "?"},
			Percentile:   cfg.Percentile,
		})
	}

	return NewRecursiveFallbackTransformer(cfg.ChunkSize, cfg.ChunkOverlap), nil
}
