// Package model 包含了应用的数据模型定义。
package model

import "time"

// ChatMessage 代表存储在 Redis 中的单条对话消息。
type ChatMessage struct {
	Role      string    `json:"role"` // "user" 或 "assistant"
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
	Provider  string    `json:"provider,omitempty"` // 可选，模型提供商``
	Model     string    `json:"model,omitempty"`    // 可选，模型名称
}

// Conversation 代表一次单独的问答交互。
type Conversation struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    uint      `gorm:"index;not null" json:"userId"`
	Question  string    `gorm:"type:text;not null" json:"question"`
	Answer    string    `gorm:"type:text;not null" json:"answer"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"createdAt"`
}

func (Conversation) TableName() string {
	return "conversations"
}
