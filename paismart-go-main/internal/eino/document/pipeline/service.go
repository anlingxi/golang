package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"
	"unicode/utf8"

	einotransformer "github.com/cloudwego/eino/components/document"
	einoparser "github.com/cloudwego/eino/components/document/parser"
	einoembedding "github.com/cloudwego/eino/components/embedding"
	einoindexer "github.com/cloudwego/eino/components/indexer"
	"github.com/cloudwego/eino/schema"
	"github.com/minio/minio-go/v7"

	"pai-smart-go/internal/model"
	"pai-smart-go/internal/repository"
	"pai-smart-go/pkg/log"
	"pai-smart-go/pkg/storage"
)

type Service struct {
	minioBucket string

	parser      einoparser.Parser
	transformer einotransformer.Transformer
	indexer     einoindexer.Indexer
	embedder    einoembedding.Embedder

	docVectorRepo repository.DocumentVectorRepository
	uploadRepo    repository.UploadRepository
}

func NewService(
	minioBucket string,
	parser einoparser.Parser,
	transformer einotransformer.Transformer,
	indexer einoindexer.Indexer,
	embedder einoembedding.Embedder,
	docVectorRepo repository.DocumentVectorRepository,
	uploadRepo repository.UploadRepository,
) *Service {
	return &Service{
		minioBucket:   minioBucket,
		parser:        parser,
		transformer:   transformer,
		indexer:       indexer,
		embedder:      embedder,
		docVectorRepo: docVectorRepo,
		uploadRepo:    uploadRepo,
	}
}

func (s *Service) Process(ctx context.Context, req ProcessRequest) (result *ProcessResult, err error) {
	acquired, err := s.uploadRepo.TryMarkFileProcessing(req.FileMD5, req.UserID)
	if err != nil {
		return nil, fmt.Errorf("mark file processing failed: %w", err)
	}
	if !acquired {
		log.Infof("[DocumentPipeline] skip duplicated task file_md5=%s user_id=%d", req.FileMD5, req.UserID)
		return &ProcessResult{
			FileMD5:    req.FileMD5,
			FileName:   req.FileName,
			ChunkCount: 0,
		}, nil
	}

	stage := "fetching"
	var chunkCount int
	defer func() {
		if err == nil {
			if completeErr := s.uploadRepo.MarkFileProcessingCompleted(req.FileMD5, req.UserID, chunkCount); completeErr != nil {
				log.Errorf("[DocumentPipeline] mark completed failed file_md5=%s user_id=%d err=%v", req.FileMD5, req.UserID, completeErr)
				err = fmt.Errorf("mark file processing completed failed: %w", completeErr)
			}
			return
		}
		if failErr := s.uploadRepo.MarkFileProcessingFailed(req.FileMD5, req.UserID, stage, err.Error()); failErr != nil {
			log.Errorf("[DocumentPipeline] mark failed failed file_md5=%s user_id=%d err=%v", req.FileMD5, req.UserID, failErr)
		}
	}()

	// 1. 从 MinIO 获取文件内容
	object, err := storage.MinioClient.GetObject(ctx, s.minioBucket, req.ObjectKey, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("get object from minio failed: %w", err)
	}
	defer object.Close()

	data, err := io.ReadAll(object)
	if err != nil {
		return nil, fmt.Errorf("read object from minio failed: %w", err)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("empty file content")
	}
	// 2. 解析、转换、入库、建索引
	stage = "parsing"
	extraMeta := map[string]any{
		"file_md5":  req.FileMD5,
		"file_name": req.FileName,
		"user_id":   req.UserID,
		"org_tag":   req.OrgTag,
		"is_public": req.IsPublic,
		"mime_type": req.MimeType,
		"source":    req.ObjectKey,
	}
	// 调用eino的parser解析文档，生成初始的doc对象，meta里会带上我们传的extraMeta
	docs, err := s.parser.Parse(
		ctx,
		bytes.NewReader(data),
		einoparser.WithURI(req.FileName),
		einoparser.WithExtraMeta(extraMeta),
	)
	if err != nil {
		return nil, fmt.Errorf("parse document failed: %w", err)
	}
	if len(docs) == 0 {
		return nil, fmt.Errorf("no parsed docs generated")
	}
	// 调用eino的transformer对初始doc进行转换，生成最终要入库和建索引的chunk级别的doc对象，meta里会自动带上chunk_id等信息，
	// 我们也会补齐一些必要的meta字段（比如file_md5、user_id等）以方便后续入库和建索引使用
	stage = "transforming"
	chunks, err := s.transformer.Transform(ctx, docs)
	if err != nil {
		return nil, fmt.Errorf("transform document failed: %w", err)
	}
	if len(chunks) == 0 {
		return nil, fmt.Errorf("no chunks generated")
	}
	chunkCount = len(chunks)
	for i, doc := range chunks {
		if doc.MetaData == nil {
			doc.MetaData = map[string]any{}
		}
		doc.MetaData["chunk_id"] = i
		if _, ok := doc.MetaData["file_md5"]; !ok {
			doc.MetaData["file_md5"] = req.FileMD5
		}
		if _, ok := doc.MetaData["file_name"]; !ok {
			doc.MetaData["file_name"] = req.FileName
		}
		if _, ok := doc.MetaData["user_id"]; !ok {
			doc.MetaData["user_id"] = req.UserID
		}
		if _, ok := doc.MetaData["org_tag"]; !ok {
			doc.MetaData["org_tag"] = req.OrgTag
		}
		if _, ok := doc.MetaData["is_public"]; !ok {
			doc.MetaData["is_public"] = req.IsPublic
		}
	}

	stage = "persisting"
	err = s.persistChunks(ctx, req, chunks)
	if err != nil {
		return nil, err
	}

	// 可选：先把这批 chunk 标记成 indexing
	stage = "indexing"
	err = s.markChunksIndexing(ctx, req.FileMD5, len(chunks))
	if err != nil {
		return nil, fmt.Errorf("mark chunks indexing failed: %w", err)
	}

	var storedIDs []string
	if s.indexer != nil {
		storedIDs, err = s.indexer.Store(ctx, chunks)
		if err != nil {
			// 写 ES 失败，整批回写 failed
			_ = s.markChunksIndexFailed(ctx, req.FileMD5, len(chunks), err.Error())
			return nil, fmt.Errorf("store chunks to indexer failed: %w", err)
		}

		// 写 ES 成功，按顺序回写 indexed
		err = s.markChunksIndexed(ctx, req.FileMD5, storedIDs)
		if err != nil {
			return nil, fmt.Errorf("mark chunks indexed failed: %w", err)
		}
	}

	log.Infof("[DocumentPipeline] processed file=%s chunks=%d indexed=%d", req.FileName, len(chunks), len(storedIDs))

	result = &ProcessResult{
		FileMD5:      req.FileMD5,
		FileName:     req.FileName,
		ChunkCount:   len(chunks),
		StoredDocIDs: storedIDs,
	}
	return result, nil
}

func (s *Service) persistChunks(ctx context.Context, req ProcessRequest, docs []*schema.Document) error {
	_ = ctx

	for i, doc := range docs {
		meta := doc.MetaData
		if meta == nil {
			meta = map[string]any{}
		}

		metadataJSON, _ := json.Marshal(meta)

		record := &model.DocumentVector{
			FileMD5:      req.FileMD5,
			ChunkID:      i,
			ChunkUID:     doc.ID,
			TextContent:  doc.Content,
			CharCount:    utf8.RuneCountInString(doc.Content),
			TokenCount:   0,
			PageNo:       getIntMeta(meta, "page_no"),
			SectionPath:  getStringMeta(meta, "section_path"),
			StartOffset:  getIntMeta(meta, "start_offset"),
			EndOffset:    getIntMeta(meta, "end_offset"),
			ParserName:   getStringMeta(meta, "parser"),
			SplitterName: getStringMeta(meta, "splitter"),
			MetadataJSON: string(metadataJSON),
			ModelVersion: "eino_document_pipeline_v1",
			UserID:       req.UserID,
			OrgTag:       req.OrgTag,
			IsPublic:     req.IsPublic,

			IndexStatus: "pending",
			IndexError:  "",
			ESDocID:     "",
			IndexedAt:   nil,
			RetryCount:  0,
		}

		if err := s.docVectorRepo.Create(record); err != nil {
			return fmt.Errorf("persist chunk failed: chunk=%d err=%w meta=%v", i, err, meta)
		}
	}

	return nil
}
func getStringMeta(meta map[string]any, key string) string {
	v, ok := meta[key]
	if !ok || v == nil {
		return ""
	}
	s, ok := v.(string)
	if ok {
		return s
	}
	return fmt.Sprint(v)
}

func getIntMeta(meta map[string]any, key string) int {
	v, ok := meta[key]
	if !ok || v == nil {
		return 0
	}
	switch t := v.(type) {
	case int:
		return t
	case int32:
		return int(t)
	case int64:
		return int(t)
	case float64:
		return int(t)
	default:
		return 0
	}
}

func (s *Service) markChunksIndexing(ctx context.Context, fileMD5 string, chunkCount int) error {
	for i := 0; i < chunkCount; i++ {
		if err := s.docVectorRepo.UpdateIndexStatus(ctx, fileMD5, i, "indexing", "", "", nil, false); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) markChunksIndexed(ctx context.Context, fileMD5 string, storedIDs []string) error {
	now := time.Now()

	for i, esDocID := range storedIDs {
		if err := s.docVectorRepo.UpdateIndexStatus(
			ctx,
			fileMD5,
			i,
			"indexed",
			"",
			esDocID,
			&now,
			false,
		); err != nil {
			return err
		}
	}

	return nil
}

func (s *Service) markChunksIndexFailed(ctx context.Context, fileMD5 string, chunkCount int, errMsg string) error {
	for i := 0; i < chunkCount; i++ {
		if err := s.docVectorRepo.UpdateIndexStatus(
			ctx,
			fileMD5,
			i,
			"failed",
			errMsg,
			"",
			nil,
			true, // retry_count + 1
		); err != nil {
			return err
		}
	}
	return nil
}
