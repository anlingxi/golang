// Package repository 定义了与数据库进行数据交换的接口和实现。
package repository

import (
	"context"
	"pai-smart-go/internal/model"
	"pai-smart-go/pkg/log"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"
)

// UploadRepository 接口定义了文件上传相关的数据持久化操作。
type UploadRepository interface {
	// FileUpload operations
	CreateFileUploadRecord(record *model.FileUpload) error
	GetFileUploadRecord(fileMD5 string, userID uint) (*model.FileUpload, error)
	UpdateFileUploadStatus(recordID uint, status int) error
	TryMarkFileProcessing(fileMD5 string, userID uint) (bool, error)
	MarkFileProcessingCompleted(fileMD5 string, userID uint, chunkCount int) error
	MarkFileProcessingFailed(fileMD5 string, userID uint, stage string, processErr string) error
	FindFilesByUserID(userID uint) ([]model.FileUpload, error)
	FindAccessibleFiles(userID uint, orgTags []string) ([]model.FileUpload, error)
	DeleteFileUploadRecord(fileMD5 string, userID uint) error
	UpdateFileUploadRecord(record *model.FileUpload) error
	FindBatchByMD5s(md5s []string) ([]*model.FileUpload, error)

	// ChunkInfo operations (GORM)
	CreateChunkInfoRecord(record *model.ChunkInfo) error
	GetChunkInfoRecords(fileMD5 string) ([]model.ChunkInfo, error)

	// Chunk status operations (Redis)
	IsChunkUploaded(ctx context.Context, fileMD5 string, userID uint, chunkIndex int) (bool, error)
	MarkChunkUploaded(ctx context.Context, fileMD5 string, userID uint, chunkIndex int) error
	GetUploadedChunksFromRedis(ctx context.Context, fileMD5 string, userID uint, totalChunks int) ([]int, error)
	DeleteUploadMark(ctx context.Context, fileMD5 string, userID uint) error
}

// uploadRepository 是 UploadRepository 接口的 GORM+Redis 实现。
type uploadRepository struct {
	db          *gorm.DB
	redisClient *redis.Client
}

// NewUploadRepository 创建一个新的 UploadRepository 实例。
func NewUploadRepository(db *gorm.DB, redisClient *redis.Client) UploadRepository {
	return &uploadRepository{db: db, redisClient: redisClient}
}

// getRedisUploadKey generates the redis key for upload status.
func (r *uploadRepository) getRedisUploadKey(fileMD5 string, userID uint) string {
	return "upload:" + strconv.FormatUint(uint64(userID), 10) + ":" + fileMD5
}

// CreateFileUploadRecord 在数据库中创建一个新的文件上传总记录。
func (r *uploadRepository) CreateFileUploadRecord(record *model.FileUpload) error {
	return r.db.Create(record).Error
}

// GetFileUploadRecord 根据文件 MD5 和用户 ID 检索文件上传记录。
func (r *uploadRepository) GetFileUploadRecord(fileMD5 string, userID uint) (*model.FileUpload, error) {
	var record model.FileUpload
	err := r.db.Where("file_md5 = ? AND user_id = ?", fileMD5, userID).First(&record).Error
	if err != nil {
		return nil, err
	}
	return &record, nil
}

// FindBatchByMD5s finds file upload records by a slice of MD5s.
func (r *uploadRepository) FindBatchByMD5s(md5s []string) ([]*model.FileUpload, error) {
	var records []*model.FileUpload
	if len(md5s) == 0 {
		return records, nil
	}
	err := r.db.Where("file_md5 IN ?", md5s).Find(&records).Error
	return records, err
}

// UpdateFileUploadStatus 更新指定文件上传记录的状态。
func (r *uploadRepository) UpdateFileUploadStatus(recordID uint, status int) error {
	return r.db.Model(&model.FileUpload{}).Where("id = ?", recordID).Update("status", status).Error
}

// TryMarkFileProcessing 尝试将文件的处理状态从待处理或失败更新为处理中，确保同一时间只有一个处理器能成功标记并处理该文件。
// 如何保证的？通过 SQL 的 WHERE 条件限制只有当当前状态是待处理或失败时才允许更新为处理中，并且通过 RowsAffected 判断是否成功更新了记录。
// 如果返回 true，说明成功标记了文件为处理中；如果返回 false，说明可能已经有其他处理器标记了该文件，当前处理器应该放弃处理。
// 这是原子操作吗？
func (r *uploadRepository) TryMarkFileProcessing(fileMD5 string, userID uint) (bool, error) {
	result := r.db.Model(&model.FileUpload{}).
		Where("file_md5 = ? AND user_id = ? AND process_status IN ?", fileMD5, userID, []int{
			model.ProcessStatusPending,
			model.ProcessStatusFailed,
		}).
		Updates(map[string]any{
			"process_status": model.ProcessStatusProcessing,
			"process_stage":  "processing",
			"process_error":  "",
		})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}

// MarkFileProcessingCompleted 将文件的处理状态更新为完成，并记录分块数量和处理完成时间。
func (r *uploadRepository) MarkFileProcessingCompleted(fileMD5 string, userID uint, chunkCount int) error {
	now := time.Now()
	return r.db.Model(&model.FileUpload{}).
		Where("file_md5 = ? AND user_id = ?", fileMD5, userID).
		Updates(map[string]any{
			"process_status": model.ProcessStatusCompleted,
			"process_stage":  "indexed",
			"process_error":  "",
			"chunk_count":    chunkCount,
			"processed_at":   &now,
		}).Error
}

// MarkFileProcessingFailed 将文件的处理状态更新为失败，并记录失败阶段和错误信息。
func (r *uploadRepository) MarkFileProcessingFailed(fileMD5 string, userID uint, stage string, processErr string) error {
	return r.db.Model(&model.FileUpload{}).
		Where("file_md5 = ? AND user_id = ?", fileMD5, userID).
		Updates(map[string]any{
			"process_status": model.ProcessStatusFailed,
			"process_stage":  stage,
			"process_error":  processErr,
		}).Error
}

// GetChunkInfoRecords 获取指定文件已上传的所有分块信息 (from DB, used for merge)。
func (r *uploadRepository) GetChunkInfoRecords(fileMD5 string) ([]model.ChunkInfo, error) {
	var chunks []model.ChunkInfo
	err := r.db.Where("file_md5 = ?", fileMD5).Order("chunk_index asc").Find(&chunks).Error
	return chunks, err
}

// FindFilesByUserID 查找指定用户上传的所有文件。
func (r *uploadRepository) FindFilesByUserID(userID uint) ([]model.FileUpload, error) {
	var files []model.FileUpload
	err := r.db.Where("user_id = ?", userID).Find(&files).Error
	return files, err
}

// FindAccessibleFiles 查找用户可访问的所有文件。
// 包括：用户自己的文件；任意 is_public=true 的文件（全局可见）；以及用户所属组织内的公开文件。
func (r *uploadRepository) FindAccessibleFiles(userID uint, orgTags []string) ([]model.FileUpload, error) {
	var files []model.FileUpload
	// 查询条件：status=1 AND (user_id=? OR is_public=true OR (org_tag IN ? AND is_public=true))
	err := r.db.Where("status = ?", 1).
		Where(r.db.Where("user_id = ?", userID).
			Or("is_public = ?", true).
			Or("org_tag IN ? AND is_public = ?", orgTags, true)).
		Find(&files).Error
	return files, err
}

// DeleteFileUploadRecord 删除一个文件上传记录。
func (r *uploadRepository) DeleteFileUploadRecord(fileMD5 string, userID uint) error {
	return r.db.Where("file_md5 = ? AND user_id = ?", fileMD5, userID).Delete(&model.FileUpload{}).Error
}

// UpdateFileUploadRecord 更新一个文件上传记录。
func (r *uploadRepository) UpdateFileUploadRecord(record *model.FileUpload) error {
	return r.db.Save(record).Error
}

// CreateChunkInfoRecord 在数据库中创建一个新的文件分块记录。
func (r *uploadRepository) CreateChunkInfoRecord(record *model.ChunkInfo) error {
	return r.db.Create(record).Error
}

// IsChunkUploaded checks if a chunk is marked as uploaded in Redis.
func (r *uploadRepository) IsChunkUploaded(ctx context.Context, fileMD5 string, userID uint, chunkIndex int) (bool, error) {
	key := r.getRedisUploadKey(fileMD5, userID)
	val, err := r.redisClient.GetBit(ctx, key, int64(chunkIndex)).Result()
	if err == nil {
		return val == 1, nil
	}

	// Redis 不可用，降级到 MySQL
	log.Warnf("[UploadRepo] Redis 不可用，降级到 MySQL (IsChunkUploaded), fileMD5=%s, chunkIndex=%d, err=%v",
		fileMD5, chunkIndex, err)
	return r.isChunkUploadedFromDB(fileMD5, chunkIndex)

}

// MarkChunkUploaded marks a chunk as uploaded in Redis.
func (r *uploadRepository) MarkChunkUploaded(ctx context.Context, fileMD5 string, userID uint, chunkIndex int) error {
	key := r.getRedisUploadKey(fileMD5, userID)
	err := r.redisClient.SetBit(ctx, key, int64(chunkIndex), 1).Err()
	if err != nil {
		// MySQL 已持久化，降级为 no-op，不向上返回错误
		log.Warnf("[UploadRepo] Redis 不可用，跳过 Redis 标记 (MarkChunkUploaded), fileMD5=%s, chunkIndex=%d, err=%v",
			fileMD5, chunkIndex, err)
	}
	return nil
}

// GetUploadedChunksFromRedis retrieves the list of uploaded chunk indexes from Redis bitmap.
// 根据文件 MD5 和用户 ID 从 Redis 中获取已上传的分块索引列表。totalChunks 用于限制返回的索引范围，避免返回过多无效索引。
func (r *uploadRepository) GetUploadedChunksFromRedis(ctx context.Context, fileMD5 string, userID uint, totalChunks int) ([]int, error) {
	if totalChunks == 0 {
		return []int{}, nil
	}
	key := r.getRedisUploadKey(fileMD5, userID)
	bitmap, err := r.redisClient.Get(ctx, key).Bytes()
	if err == nil {
		uploaded := make([]int, 0)
		for i := 0; i < totalChunks; i++ {
			byteIndex := i / 8
			bitIndex := i % 8
			if byteIndex < len(bitmap) && (bitmap[byteIndex]>>(7-bitIndex))&1 == 1 {
				uploaded = append(uploaded, i)
			}
		}
		return uploaded, nil
	}

	if err == redis.Nil {
		// 键不存在：可能是 Redis 重启丢失数据，查 MySQL 并尝试回填 Redis
		chunks, dbErr := r.getUploadedChunksFromDB(fileMD5, totalChunks)
		if dbErr != nil {
			return nil, dbErr
		}
		// 回填：将 MySQL 中已上传的分片状态写回 Redis bitmap
		// 此处 Redis 确实可达（能收到 Nil 响应），回填大概率成功
		if len(chunks) > 0 {
			for _, idx := range chunks {
				if setErr := r.redisClient.SetBit(ctx, key, int64(idx), 1).Err(); setErr != nil {
					log.Warnf("[UploadRepo] Redis 回填失败, fileMD5=%s, chunkIndex=%d, err=%v",
						fileMD5, idx, setErr)
					break // 回填失败说明 Redis 又出问题了，不必继续
				}
			}
		}
		return chunks, nil
	}

	if err != redis.Nil {
		log.Warnf("[UploadRepo] Redis 不可用，降级到 MySQL (GetUploadedChunks), fileMD5=%s, err=%v", fileMD5, err)
	}
	// redis.Nil 或连接错误，均查 MySQL
	return r.getUploadedChunksFromDB(fileMD5, totalChunks)
}

// DeleteUploadMark deletes the upload status key from Redis.
func (r *uploadRepository) DeleteUploadMark(ctx context.Context, fileMD5 string, userID uint) error {
	key := r.getRedisUploadKey(fileMD5, userID)
	err := r.redisClient.Del(ctx, key).Err()
	if err != nil {
		log.Warnf("[UploadRepo] Redis 不可用，跳过 Redis 清理 (DeleteUploadMark), fileMD5=%s, err=%v", fileMD5, err)
	}
	return nil
}

// 新增私有查询
// isChunkUploadedFromDB 在 Redis 不可用时，从 chunk_info 表核验分片是否已上传。
func (r *uploadRepository) isChunkUploadedFromDB(fileMD5 string, chunkIndex int) (bool, error) {
	var count int64
	err := r.db.Model(&model.ChunkInfo{}).
		Where("file_md5 = ? AND chunk_index = ?", fileMD5, chunkIndex).
		Count(&count).Error
	return count > 0, err
}

// getUploadedChunksFromDB 在 Redis 不可用时，从 chunk_info 表获取已上传分片索引列表。
func (r *uploadRepository) getUploadedChunksFromDB(fileMD5 string, totalChunks int) ([]int, error) {
	var chunks []model.ChunkInfo
	err := r.db.Where("file_md5 = ? AND chunk_index < ?", fileMD5, totalChunks).
		Select("chunk_index").
		Find(&chunks).Error
	if err != nil {
		return nil, err
	}
	seen := make(map[int]struct{})
	result := make([]int, 0, len(chunks))
	for _, c := range chunks {
		if _, exists := seen[c.ChunkIndex]; !exists {
			seen[c.ChunkIndex] = struct{}{}
			result = append(result, c.ChunkIndex)
		}
	}
	return result, nil
}
