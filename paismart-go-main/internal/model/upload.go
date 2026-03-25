package model

import "time"

type FileUpload struct {
	ID uint `gorm:"primaryKey;autoIncrement" json:"id"`
	// 这是索引字段，必须是32字符的MD5值，属于 什么索引？联合索引，包含FileMD5和UserID，priority:1表示这是联合索引中的第一个字段
	// 因为不同用户可能会上传相同内容的文件，所以FileMD5不能单独作为唯一索引，必须和UserID一起才能唯一标识一个文件上传记录
	FileMD5   string `gorm:"type:varchar(32);not null;index:idx_file_upload_md5_user,priority:1" json:"fileMd5"`
	FileName  string `gorm:"type:varchar(255);not null" json:"fileName"`
	TotalSize int64  `gorm:"not null" json:"totalSize"`

	// 旧上传状态：0 uploading, 1 completed, 2 failed
	Status int `gorm:"type:tinyint;not null;default:0" json:"status"`

	UserID   uint   `gorm:"not null;index:idx_file_upload_md5_user,priority:2" json:"userId"`
	OrgTag   string `gorm:"type:varchar(50)" json:"orgTag"`
	IsPublic bool   `gorm:"not null;default:false" json:"isPublic"`

	// 新增：文档后处理状态 pending是什么意思：文档上传完成后，进入待处理状态，等待后台任务进行解析和处理。这个状态表示文件已经成功上传，
	// 但还没有开始处理。后台任务会定期扫描数据库中处于pending状态的记录，开始处理这些文件，并更新状态为processing、completed或failed。
	// 如果两个conmsuer同时拿到了一条消息，都通过了幂等检查，都处理同一个文档怎么办？通过数据库的行锁机制来保证同一时间只有一个消费者能够处理同一条
	// 记录。当一个消费者开始处理一条记录时，它会在数据库中对该记录加锁，其他消费者在尝试处理同一条记录时会被阻塞，直到锁被释放。
	// 这样可以确保即使多个消费者同时拿到消息，也不会出现重复处理同一文档的情况。
	// 这个锁需要自己加，还是天生的？需要自己加，可以在查询时使用FOR UPDATE语句来加锁，或者在GORM中使用Select("FOR UPDATE")来实现行锁。
	// 在加锁查到后，是立刻修改字段，还是把后面的向量化一些流程都放到事务里面了，如果处理时间长，锁一直不被释放？这里是如何解决的？
	// 可以在处理过程中定期更新数据库中的状态字段，或者使用心跳机制来延长锁的持有时间。
	// 如果处理时间过长，可以考虑将处理过程拆分成多个步骤，每个步骤完成后都更新一次状态，这样可以减少锁的持有时间。
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
package model

import "time"

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
