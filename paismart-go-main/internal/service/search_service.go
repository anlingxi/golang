// Package service 提供了搜索相关的业务逻辑。
package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/elastic/go-elasticsearch/v8"

	"pai-smart-go/internal/language/langdetect"
	"pai-smart-go/internal/model"
	"pai-smart-go/internal/repository"
	"pai-smart-go/pkg/embedding"
	"pai-smart-go/pkg/log"
)

const (
	defaultTopK      = 10
	recallMultiplier = 30
	rrfRankConstant  = 60.0
)

// SearchService 接口定义了搜索操作。
type SearchService interface {
	HybridSearch(ctx context.Context, query string, topK int, user *model.User) ([]model.SearchResponseDTO, error)
}

type searchService struct {
	embeddingClient embedding.Client
	esClient        *elasticsearch.Client
	userService     UserService
	uploadRepo      repository.UploadRepository
	indexName       string
}

// NewSearchService 创建一个新的 SearchService 实例。
func NewSearchService(
	embeddingClient embedding.Client,
	esClient *elasticsearch.Client,
	userService UserService,
	uploadRepo repository.UploadRepository,
	indexName string,
) SearchService {
	if strings.TrimSpace(indexName) == "" {
		indexName = "knowledge_base"
	}
	return &searchService{
		embeddingClient: embeddingClient,
		esClient:        esClient,
		userService:     userService,
		uploadRepo:      uploadRepo,
		indexName:       indexName,
	}
}

// HybridSearch 执行双路召回（kNN + BM25）并通过 RRF 融合排序。
func (s *searchService) HybridSearch(ctx context.Context, query string, topK int, user *model.User) ([]model.SearchResponseDTO, error) {
	if user == nil {
		return nil, fmt.Errorf("user is nil")
	}
	if topK <= 0 {
		topK = defaultTopK
	}

	log.Infof("[SearchService] 开始执行混合搜索, query='%s', topK=%d, user=%s", query, topK, user.Username)

	userEffectiveTags, err := s.userService.GetUserEffectiveOrgTags(user)
	if err != nil {
		log.Errorf("[SearchService] 获取用户有效组织标签失败: %v", err)
		userEffectiveTags = []string{}
	}

	normalized, phrase := normalizeQuery(query)
	searchText := normalized
	if strings.TrimSpace(searchText) == "" {
		searchText = query
	}
	queryLang := langdetect.DetectContentType(searchText)
	recallK := topK * recallMultiplier
	if recallK < topK {
		recallK = topK
	}

	queryVector, err := s.embeddingClient.CreateEmbedding(ctx, query)
	if err != nil {
		log.Errorf("[SearchService] 向量化查询失败: %v", err)
		return nil, fmt.Errorf("failed to create query embedding: %w", err)
	}

	accessFilter := buildAccessFilter(user.ID, userEffectiveTags)
	knnQuery := buildKNNQuery(queryVector, recallK, accessFilter)
	bm25Query := buildBM25Query(searchText, phrase, queryLang, recallK, accessFilter)

	knnHits, err := s.executeSearch(ctx, knnQuery)
	if err != nil {
		return nil, fmt.Errorf("knn search failed: %w", err)
	}
	bm25Hits, err := s.executeSearch(ctx, bm25Query)
	if err != nil {
		return nil, fmt.Errorf("bm25 search failed: %w", err)
	}

	fused := fuseHitsByRRF(knnHits, bm25Hits, rrfRankConstant, topK)
	if len(fused) == 0 {
		return []model.SearchResponseDTO{}, nil
	}

	return s.buildDTOs(fused)
}

type esSearchHit struct {
	ID     string           `json:"_id"`
	Source model.EsDocument `json:"_source"`
	Score  float64          `json:"_score"`
}

type esSearchResponse struct {
	Hits struct {
		Hits []esSearchHit `json:"hits"`
	} `json:"hits"`
}

func (s *searchService) executeSearch(ctx context.Context, body map[string]any) ([]esSearchHit, error) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(body); err != nil {
		return nil, fmt.Errorf("failed to encode es query: %w", err)
	}

	res, err := s.esClient.Search(
		s.esClient.Search.WithContext(ctx),
		s.esClient.Search.WithIndex(s.indexName),
		s.esClient.Search.WithBody(&buf),
		s.esClient.Search.WithTrackTotalHits(true),
	)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.IsError() {
		bodyBytes, _ := io.ReadAll(res.Body)
		return nil, fmt.Errorf("elasticsearch returned an error: %s, body=%s", res.String(), string(bodyBytes))
	}

	var esResp esSearchResponse
	if err := json.NewDecoder(res.Body).Decode(&esResp); err != nil {
		return nil, fmt.Errorf("failed to decode es response: %w", err)
	}
	return esResp.Hits.Hits, nil
}

func (s *searchService) buildDTOs(hits []esSearchHit) ([]model.SearchResponseDTO, error) {
	md5Set := make(map[string]struct{}, len(hits))
	for _, hit := range hits {
		if hit.Source.FileMD5 != "" {
			md5Set[hit.Source.FileMD5] = struct{}{}
		}
	}

	md5List := make([]string, 0, len(md5Set))
	for md5 := range md5Set {
		md5List = append(md5List, md5)
	}

	fileInfos, err := s.uploadRepo.FindBatchByMD5s(md5List)
	if err != nil {
		return nil, fmt.Errorf("批量查询文件信息失败: %w", err)
	}
	fileNameMap := make(map[string]string, len(fileInfos))
	for _, info := range fileInfos {
		fileNameMap[info.FileMD5] = info.FileName
	}

	results := make([]model.SearchResponseDTO, 0, len(hits))
	for _, hit := range hits {
		fileName := fileNameMap[hit.Source.FileMD5]
		if fileName == "" {
			fileName = "未知文件"
		}
		results = append(results, model.SearchResponseDTO{
			FileMD5:     hit.Source.FileMD5,
			FileName:    fileName,
			ChunkID:     hit.Source.ChunkID,
			TextContent: hit.Source.TextContent,
			Score:       hit.Score,
			UserID:      strconv.FormatUint(uint64(hit.Source.UserID), 10),
			OrgTag:      hit.Source.OrgTag,
			IsPublic:    hit.Source.IsPublic,
		})
	}

	return results, nil
}

func buildAccessFilter(userID uint, userEffectiveTags []string) map[string]any {
	should := []map[string]any{
		{"term": map[string]any{"user_id": userID}},
		{"term": map[string]any{"is_public": true}},
	}
	if len(userEffectiveTags) > 0 {
		should = append(should, map[string]any{
			"terms": map[string]any{"org_tag": userEffectiveTags},
		})
	}
	return map[string]any{
		"bool": map[string]any{
			"should":               should,
			"minimum_should_match": 1,
		},
	}
}

func buildKNNQuery(queryVector []float32, recallK int, filter map[string]any) map[string]any {
	knn := map[string]any{
		"field":          "vector",
		"query_vector":   queryVector,
		"k":              recallK,
		"num_candidates": recallK,
		"filter":         filter,
	}
	return map[string]any{
		"knn":  knn,
		"size": recallK,
	}
}

func buildBM25Query(text, phrase, lang string, recallK int, filter map[string]any) map[string]any {
	if strings.TrimSpace(text) == "" {
		text = phrase
	}
	fields := langToFields(lang)

	boolQuery := map[string]any{
		"must": []any{
			map[string]any{
				"multi_match": map[string]any{
					"query":  text,
					"fields": fields,
					"type":   "best_fields",
				},
			},
		},
		"filter": filter,
	}
	if should := buildPhraseShould(phrase, fields); len(should) > 0 {
		boolQuery["should"] = should
	}

	return map[string]any{
		"query": map[string]any{
			"bool": boolQuery,
		},
		"size": recallK,
	}
}

func langToFields(lang string) []string {
	switch lang {
	case langdetect.ContentTypeZH:
		return []string{"text_content_zh^3", "text_content^1"}
	case langdetect.ContentTypeEN:
		return []string{"text_content_en^3", "text_content^1"}
	case langdetect.ContentTypeCode:
		return []string{"text_content_code^3", "text_content^1"}
	case langdetect.ContentTypeMixed:
		return []string{"text_content_zh^2", "text_content_en^2", "text_content^1"}
	default:
		return []string{"text_content"}
	}
}

func buildPhraseShould(phrase string, fields []string) []map[string]any {
	if strings.TrimSpace(phrase) == "" {
		return nil
	}
	should := make([]map[string]any, 0, len(fields))
	for _, field := range fields {
		baseField := strings.Split(field, "^")[0]
		should = append(should, map[string]any{
			"match_phrase": map[string]any{
				baseField: map[string]any{
					"query": phrase,
					"boost": 3.0,
				},
			},
		})
	}
	return should
}

func fuseHitsByRRF(knnHits, bm25Hits []esSearchHit, rankConstant float64, topK int) []esSearchHit {
	type fusedEntry struct {
		Hit      esSearchHit
		RRFScore float64
		BestRank int
	}

	fused := make(map[string]*fusedEntry, len(knnHits)+len(bm25Hits))

	merge := func(hits []esSearchHit) {
		for i, hit := range hits {
			rank := i + 1
			key := hitKey(hit)
			if key == "" {
				continue
			}
			entry, ok := fused[key]
			if !ok {
				entry = &fusedEntry{
					Hit:      hit,
					RRFScore: 0,
					BestRank: rank,
				}
				fused[key] = entry
			}
			entry.RRFScore += 1.0 / (rankConstant + float64(rank))
			if rank < entry.BestRank {
				entry.BestRank = rank
			}
		}
	}

	merge(knnHits)
	merge(bm25Hits)

	entries := make([]*fusedEntry, 0, len(fused))
	for _, e := range fused {
		e.Hit.Score = e.RRFScore
		entries = append(entries, e)
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].RRFScore == entries[j].RRFScore {
			return entries[i].BestRank < entries[j].BestRank
		}
		return entries[i].RRFScore > entries[j].RRFScore
	})

	if topK > len(entries) {
		topK = len(entries)
	}

	mergedHits := make([]esSearchHit, 0, topK)
	for i := 0; i < topK; i++ {
		mergedHits = append(mergedHits, entries[i].Hit)
	}

	return mergedHits
}

func hitKey(hit esSearchHit) string {
	if hit.Source.VectorID != "" {
		return hit.Source.VectorID
	}
	if hit.ID != "" {
		return hit.ID
	}
	if hit.Source.FileMD5 == "" {
		return ""
	}
	return fmt.Sprintf("%s#%d", hit.Source.FileMD5, hit.Source.ChunkID)
}

// normalizeQuery 对用户查询进行轻量去噪与短语提取。
// 返回值：规范化后的查询（用于 BM25）与核心短语（用于 match_phrase 兜底）。
func normalizeQuery(q string) (string, string) {
	if q == "" {
		return q, ""
	}
	lower := strings.ToLower(q)
	// 去除常见口语/功能词
	stopPhrases := []string{"是谁", "是什么", "是啥", "请问", "怎么", "如何", "告诉我", "严格", "按照", "不要补充", "的区别", "区别", "吗", "呢", "？", "?"}
	for _, sp := range stopPhrases {
		lower = strings.ReplaceAll(lower, sp, " ")
	}
	// 仅保留中文、英文、数字与空白
	reKeep := regexp.MustCompile(`[^\p{Han}a-z0-9\s]+`)
	kept := reKeep.ReplaceAllString(lower, " ")
	// 归一空白
	reSpace := regexp.MustCompile(`\s+`)
	kept = strings.TrimSpace(reSpace.ReplaceAllString(kept, " "))
	if kept == "" {
		return q, ""
	}
	return kept, kept
}
