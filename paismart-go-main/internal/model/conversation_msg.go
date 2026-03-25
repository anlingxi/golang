package model

import "time"

type MessageRole string

const (
	MessageRoleSystem    MessageRole = "system"
	MessageRoleUser      MessageRole = "user"
	MessageRoleAssistant MessageRole = "assistant"
	MessageRoleTool      MessageRole = "tool"
)

type MessageStatus string

const (
	MessageStatusPending     MessageStatus = "pending"
	MessageStatusStreaming   MessageStatus = "streaming"
	MessageStatusCompleted   MessageStatus = "completed"
	MessageStatusInterrupted MessageStatus = "interrupted"
	MessageStatusFailed      MessageStatus = "failed"
)

type ConversationMsg struct {
	ID          string        `gorm:"primaryKey;type:varchar(64)"`
	SessionID   string        `gorm:"index;type:varchar(64);not null"`
	TurnID      string        `gorm:"index;type:varchar(64)"`
	RequestID   string        `gorm:"index;type:varchar(64)"`
	UserID      uint          `gorm:"index;not null"`
	Role        MessageRole   `gorm:"type:varchar(32);index;not null"`
	Content     string        `gorm:"type:longtext;not null"`
	ContentType string        `gorm:"type:varchar(32);default:text"`
	Sequence    int64         `gorm:"index;not null"`
	Status      MessageStatus `gorm:"type:varchar(32);index;not null"`
	Provider    string        `gorm:"type:varchar(64)"`
	Model       string        `gorm:"type:varchar(128)"`
	TokenInput  int
	TokenOutput int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
