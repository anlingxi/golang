package model

import "time"

type SessionMode string

const (
	SessionModeNormal SessionMode = "normal_chat"
	SessionModeRAG    SessionMode = "rag_chat"
	SessionModeAgent  SessionMode = "agent_chat"
)

type SessionStatus string

const (
	SessionStatusActive   SessionStatus = "active"
	SessionStatusArchived SessionStatus = "archived"
	SessionStatusDeleted  SessionStatus = "deleted"
)

type Session struct {
	ID            string        `gorm:"primaryKey;type:varchar(64)"`
	UserID        uint          `gorm:"index;not null"`
	Title         string        `gorm:"type:varchar(255)"`
	Mode          SessionMode   `gorm:"type:varchar(32);index;not null"`
	Provider      string        `gorm:"type:varchar(64)"`
	Model         string        `gorm:"type:varchar(128)"`
	Status        SessionStatus `gorm:"type:varchar(32);index;not null"`
	LastMessageAt *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}
