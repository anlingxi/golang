package model

import "time"

const (
	ProcessStatusPending    = 0
	ProcessStatusProcessing = 1
	ProcessStatusCompleted  = 2
	ProcessStatusFailed     = 3
)

type FileUpload struct {
	ID        uint   `gorm:"primaryKey;autoIncrement" json:"id"`
	FileMD5   string `gorm:"type:varchar(32);not null;index:idx_file_upload_md5_user,priority:1" json:"fileMd5"`
	FileName  string `gorm:"type:varchar(255);not null" json:"fileName"`
	TotalSize int64  `gorm:"not null" json:"totalSize"`

	// 旧上传状态：0 uploading, 1 completed, 2 failed
	Status int `gorm:"type:tinyint;not null;default:0" json:"status"`

	UserID   uint   `gorm:"not null;index:idx_file_upload_md5_user,priority:2" json:"userId"`
	OrgTag   string `gorm:"type:varchar(50)" json:"orgTag"`
	IsPublic bool   `gorm:"not null;default:false" json:"isPublic"`

	// 新增：文档后处理状态
	ProcessStatus  int        `gorm:"not null;default:0;column:process_status"` // 0 pending 1 processing 2 completed 3 failed
	ProcessStage   string     `gorm:"type:varchar(50);column:process_stage"`
	ProcessError   string     `gorm:"type:text;column:process_error"`
	ParserName     string     `gorm:"type:varchar(50);column:parser_name"`
	SplitterName   string     `gorm:"type:varchar(50);column:splitter_name"`
	EmbeddingModel string     `gorm:"type:varchar(100);column:embedding_model"`
	ChunkCount     int        `gorm:"not null;default:0;column:chunk_count"`
	ProcessedAt    *time.Time `gorm:"column:processed_at"`
}

func (FileUpload) TableName() string {
	return "file_upload"
}

type ChunkInfo struct {
	ID          uint   `gorm:"primaryKey;autoIncrement" json:"id"`
	FileMD5     string `gorm:"type:varchar(32);not null;index" json:"fileMd5"`
	ChunkIndex  int    `gorm:"not null" json:"chunkIndex"`
	ChunkMD5    string `gorm:"type:varchar(32);not null" json:"chunkMd5"`
	StoragePath string `gorm:"type:varchar(255);not null" json:"storagePath"`
}

func (ChunkInfo) TableName() string {
	return "chunk_info"
}
