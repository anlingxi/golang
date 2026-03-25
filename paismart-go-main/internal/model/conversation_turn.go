package model

import "time"

type TurnStatus string

const (
	TurnStatusRunning     TurnStatus = "running"
	TurnStatusCompleted   TurnStatus = "completed"
	TurnStatusInterrupted TurnStatus = "interrupted"
	TurnStatusFailed      TurnStatus = "failed"
)

type ConversationTurn struct {
	ID             string     `gorm:"primaryKey;type:varchar(64)"`
	SessionID      string     `gorm:"index;type:varchar(64);not null"`
	RequestID      string     `gorm:"index;type:varchar(64);not null"`
	UserID         uint       `gorm:"index;not null"`
	UserMsgID      string     `gorm:"type:varchar(64)"`
	AssistantMsgID string     `gorm:"type:varchar(64)"`
	Status         TurnStatus `gorm:"type:varchar(32);index;not null"`
	StopReason     string     `gorm:"type:varchar(64)"`
	ErrorMessage   string     `gorm:"type:text"`
	StartedAt      time.Time
	CompletedAt    *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}
