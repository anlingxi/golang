package builder

import (
	"context"
	"fmt"

	einoindexer "github.com/cloudwego/eino/components/indexer"
	"github.com/cloudwego/eino/schema"
	"github.com/elastic/go-elasticsearch/v8"

	"github.com/cloudwego/eino-ext/components/indexer/es8"

	"pai-smart-go/internal/config"
	documentparser "pai-smart-go/internal/eino/document/parser"
	documentpipeline "pai-smart-go/internal/eino/document/pipeline"
	documenttransformer "pai-smart-go/internal/eino/document/transformer"
	"pai-smart-go/internal/eino/factory"
	"pai-smart-go/internal/language/langdetect"
	"pai-smart-go/internal/repository"
	"pai-smart-go/pkg/tika"
)

const (
	esFieldContent      = "text_content"
	esFieldContentZH    = "text_content_zh"
	esFieldContentEN    = "text_content_en"
	esFieldContentCode  = "text_content_code"
	esFieldLang         = "lang"
	esFieldVector       = "vector"
	esFieldFileMD5      = "file_md5"
	esFieldFileName     = "file_name"
	esFieldUserID       = "user_id"
	esFieldOrgTag       = "org_tag"
	esFieldIsPublic     = "is_public"
	esFieldChunkID      = "chunk_id"
	esFieldParserName   = "parser_name"
	esFieldSplitterName = "splitter_name"
	esFieldSectionPath  = "section_path"
)

func NewPipeline(
	ctx context.Context,
	cfg config.Config,
	esClient *elasticsearch.Client,
	products factory.AIProducts,
	docVectorRepo repository.DocumentVectorRepository,
	uploadRepo repository.UploadRepository,
) (*documentpipeline.Service, error) {
	tikaClient := tika.NewClient(cfg.Tika)

	parser, err := documentparser.NewExtParser(ctx, tikaClient)
	if err != nil {
		return nil, fmt.Errorf("init ext parser failed: %w", err)
	}

	transformer, err := documenttransformer.NewPrimaryTransformer(
		ctx,
		products.EinoEmbedder(),
		documenttransformer.SplitConfig{
			UseSemantic:  products.EinoEmbedder() != nil,
			MinChunkSize: 200,
			Percentile:   0.85,
			BufferSize:   1,
			ChunkSize:    1000,
			ChunkOverlap: 100,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("init primary transformer failed: %w", err)
	}

	var indexer einoindexer.Indexer
	if products.EinoEmbedder() != nil {
		indexer, err = es8.NewIndexer(
			ctx,
			&es8.IndexerConfig{
				Client:    esClient,
				Index:     cfg.Elasticsearch.IndexName,
				BatchSize: 10,
				Embedding: products.EinoEmbedder(),
				DocumentToFields: func(ctx context.Context, doc *schema.Document) (map[string]es8.FieldValue, error) {
					meta := doc.MetaData
					if meta == nil {
						meta = map[string]any{}
					}
					// 个人小作坊的语言检测
					contentType := langdetect.DetectContentType(doc.Content)

					fields := map[string]es8.FieldValue{
						esFieldContent: {
							Value:    doc.Content,
							EmbedKey: esFieldVector,
						},
						esFieldLang: {
							Value: contentType,
						},
						esFieldFileMD5: {
							Value: meta["file_md5"],
						},
						esFieldFileName: {
							Value: meta["file_name"],
						},
						esFieldUserID: {
							Value: meta["user_id"],
						},
						esFieldOrgTag: {
							Value: meta["org_tag"],
						},
						esFieldIsPublic: {
							Value: meta["is_public"],
						},
						esFieldChunkID: {
							Value: meta["chunk_id"],
						},
						esFieldParserName: {
							Value: meta["parser"],
						},
						esFieldSplitterName: {
							Value: meta["splitter"],
						},
						esFieldSectionPath: {
							Value: meta["section_path"],
						},
					}

					switch contentType {
					case langdetect.ContentTypeZH:
						fields[esFieldContentZH] = es8.FieldValue{Value: doc.Content}
					case langdetect.ContentTypeEN:
						fields[esFieldContentEN] = es8.FieldValue{Value: doc.Content}
					case langdetect.ContentTypeCode:
						fields[esFieldContentCode] = es8.FieldValue{Value: doc.Content}
					case langdetect.ContentTypeMixed:
						fields[esFieldContentZH] = es8.FieldValue{Value: doc.Content}
						fields[esFieldContentEN] = es8.FieldValue{Value: doc.Content}
					default:
						fields[esFieldContentEN] = es8.FieldValue{Value: doc.Content}
					}

					return fields, nil
				},
			},
		)
		if err != nil {
			return nil, fmt.Errorf("init indexer failed: %w", err)
		}
	}

	return documentpipeline.NewService(
		cfg.MinIO.BucketName,
		parser,
		transformer,
		indexer,
		products.EinoEmbedder(),
		docVectorRepo,
		uploadRepo,
	), nil
}
