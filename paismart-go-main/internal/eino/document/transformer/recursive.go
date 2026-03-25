package transformer

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/schema"
)

type RecursiveFallbackTransformer struct {
	ChunkSize    int
	ChunkOverlap int
}

func NewRecursiveFallbackTransformer(chunkSize, chunkOverlap int) *RecursiveFallbackTransformer {
	if chunkSize <= 0 {
		chunkSize = 1000
	}
	if chunkOverlap < 0 {
		chunkOverlap = 0
	}
	return &RecursiveFallbackTransformer{
		ChunkSize:    chunkSize,
		ChunkOverlap: chunkOverlap,
	}
}

func (t *RecursiveFallbackTransformer) Transform(ctx context.Context, docs []*schema.Document) ([]*schema.Document, error) {
	_ = ctx

	var chunks []*schema.Document
	step := t.ChunkSize - t.ChunkOverlap
	if step <= 0 {
		step = t.ChunkSize
	}

	for _, doc := range docs {
		runes := []rune(doc.Content)
		for start := 0; start < len(runes); start += step {
			end := start + t.ChunkSize
			if end > len(runes) {
				end = len(runes)
			}

			meta := map[string]any{}
			for k, v := range doc.MetaData {
				meta[k] = v
			}
			meta["splitter"] = "recursive_fallback"
			meta["start_offset"] = start
			meta["end_offset"] = end

			chunks = append(chunks, &schema.Document{
				ID:       fmt.Sprintf("%s-%d", doc.ID, len(chunks)),
				Content:  string(runes[start:end]),
				MetaData: meta,
			})

			if end == len(runes) {
				break
			}
		}
	}

	return chunks, nil
}
