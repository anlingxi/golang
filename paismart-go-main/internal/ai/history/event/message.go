package event

import (
	"pai-smart-go/internal/model"
	"time"
)

type TurnPersistedEvent struct {
	// 这个事件是给内部使用的，表示一个对话轮次已经被持久化了。它包含了对话轮次的相关信息，以及一个唯一的 EventID 和发生时间 OccurredAt。
	EventID    string    `json:"event_id"`
	UserID     uint      `json:"user_id"`
	SessionID  string    `json:"session_id"`
	TurnID     string    `json:"turn_id"`
	RequestID  string    `json:"request_id"`
	OccurredAt time.Time `json:"occurred_at"`
}

// RabbitMQ 真正用于异步落库的任务消息。
// 注意：这里不要 import history。
type PersistTurnTask struct {
	TaskID           string            `json:"task_id"`
	UserID           uint              `json:"user_id"`
	SessionID        string            `json:"session_id"`
	TurnID           string            `json:"turn_id"`
	RequestID        string            `json:"request_id"`
	Mode             string            `json:"mode"`
	Provider         string            `json:"provider"`
	Model            string            `json:"model"`
	UserMessage      model.ChatMessage `json:"user_message"`
	AssistantMessage model.ChatMessage `json:"assistant_message"`

	InputTokens   int    `json:"input_tokens"`
	OutputTokens  int    `json:"output_tokens"`
	LatencyMS     int64  `json:"latency_ms"`
	StopReason    string `json:"stop_reason"`
	ErrorMessage  string `json:"error_message"`
	TraceID       string `json:"trace_id"`
	IsInterrupted bool   `json:"is_interrupted"`

	OccurredAt time.Time `json:"occurred_at"`
}
