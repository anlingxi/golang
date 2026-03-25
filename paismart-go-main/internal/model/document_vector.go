package model

import "time"

// DocumentVector 对应于数据库中的 document_vectors 表。
// 它的结构与 Java 项目中的 DocumentVector 实体完全一致。
type DocumentVector struct {
	VectorID     uint       `gorm:"primaryKey;autoIncrement;column:vector_id"`
	FileMD5      string     `gorm:"type:varchar(32);not null;index;column:file_md5"`
	ChunkID      int        `gorm:"not null;column:chunk_id"`
	ChunkUID     string     `gorm:"type:varchar(100);index;column:chunk_uid"`
	TextContent  string     `gorm:"type:text;column:text_content"`
	CharCount    int        `gorm:"not null;default:0;column:char_count"`
	TokenCount   int        `gorm:"not null;default:0;column:token_count"`
	PageNo       int        `gorm:"column:page_no"`
	SectionPath  string     `gorm:"type:varchar(255);column:section_path"`
	StartOffset  int        `gorm:"column:start_offset"`
	EndOffset    int        `gorm:"column:end_offset"`
	ParserName   string     `gorm:"type:varchar(50);column:parser_name"`
	SplitterName string     `gorm:"type:varchar(50);column:splitter_name"`
	MetadataJSON string     `gorm:"type:longtext;column:metadata_json"`
	ModelVersion string     `gorm:"type:varchar(50);column:model_version"`
	UserID       uint       `gorm:"not null;column:user_id"`
	OrgTag       string     `gorm:"type:varchar(50);column:org_tag"`
	IsPublic     bool       `gorm:"not null;default:false;column:is_public"`
	IndexStatus  string     `gorm:"type:varchar(20);not null;default:'pending';index;column:index_status"`
	IndexError   string     `gorm:"type:text;column:index_error"`
	ESDocID      string     `gorm:"type:varchar(128);index;column:es_doc_id"`
	IndexedAt    *time.Time `gorm:"column:indexed_at"`
	RetryCount   int        `gorm:"not null;default:0;column:retry_count"`
}

func (DocumentVector) TableName() string {
	return "document_vectors"
}
