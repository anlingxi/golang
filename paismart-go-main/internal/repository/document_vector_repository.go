package repository

import (
	"context"
	"pai-smart-go/internal/model"
	"time"

	"gorm.io/gorm"
)

// DocumentVectorRepository 定义了对 document_vectors 表的数据操作接口。
type DocumentVectorRepository interface {
	Create(vector *model.DocumentVector) error
	BatchCreate(vectors []*model.DocumentVector) error
	FindByFileMD5(fileMD5 string) ([]*model.DocumentVector, error)
	DeleteByFileMD5(fileMD5 string) error
	UpdateIndexStatus(
		ctx context.Context,
		fileMD5 string,
		chunkID int,
		status string,
		indexError string,
		esDocID string,
		indexedAt *time.Time,
		incrementRetry bool,
	) error
}

type documentVectorRepository struct {
	db *gorm.DB
}

// NewDocumentVectorRepository 创建一个新的 DocumentVectorRepository 实例。
func NewDocumentVectorRepository(db *gorm.DB) DocumentVectorRepository {
	return &documentVectorRepository{db: db}
}

func (r *documentVectorRepository) Create(vector *model.DocumentVector) error {
	return r.db.Create(vector).Error
}

// BatchCreate 批量创建文档向量记录。
func (r *documentVectorRepository) BatchCreate(vectors []*model.DocumentVector) error {
	if len(vectors) == 0 {
		return nil
	}
	return r.db.CreateInBatches(vectors, 100).Error // 每100条记录一批
}

// FindByFileMD5 根据文件MD5查找所有相关的文档向量记录。
func (r *documentVectorRepository) FindByFileMD5(fileMD5 string) ([]*model.DocumentVector, error) {
	var vectors []*model.DocumentVector
	err := r.db.Where("file_md5 = ?", fileMD5).Find(&vectors).Error
	return vectors, err
}

// DeleteByFileMD5 根据文件MD5删除所有相关的文档向量记录。
func (r *documentVectorRepository) DeleteByFileMD5(fileMD5 string) error {
	return r.db.Where("file_md5 = ?", fileMD5).Delete(&model.DocumentVector{}).Error
}

func (r *documentVectorRepository) UpdateIndexStatus(
	ctx context.Context,
	fileMD5 string,
	chunkID int,
	status string,
	indexError string,
	esDocID string,
	indexedAt *time.Time,
	incrementRetry bool,
) error {
	updates := map[string]any{
		"index_status": status,
		"index_error":  indexError,
		"es_doc_id":    esDocID,
		"indexed_at":   indexedAt,
	}

	db := r.db.WithContext(ctx).Model(&model.DocumentVector{}).
		Where("file_md5 = ? AND chunk_id = ?", fileMD5, chunkID)

	if incrementRetry {
		return db.Updates(updates).
			UpdateColumn("retry_count", gorm.Expr("retry_count + 1")).Error
	}

	return db.Updates(updates).Error
}
