package history

import (
	"pai-smart-go/internal/model"
	"time"
)

// LoadHistoryOptions 定义加载历史记录的选项。
type LoadHistoryOptions struct {
	SessionID         string
	UserID            uint
	Limit             int
	PreferRecentCache bool
	IncludeSystem     bool
}

// PersistTurnRequest 定义了持久化对话轮次所需的信息。
// 为什么要定义对话轮次，是因为在一次对话中，用户和助手可能会多次交互，每次交互都可以视为一个轮次。
// 通过定义对话轮次，我们可以更好地组织和管理对话历史记录，方便后续的查询和分析。
// turnid 是一个唯一标识符，用于区分不同的对话轮次。它可以是一个随机生成的字符串，或者是一个递增的数字，具体取决于系统的设计和需求。
// requestid 是一个唯一标识符，用于区分不同的请求。它可以是一个随机生成的字符串，或者是一个递增的数字，具体取决于系统的设计和需求。
type PersistTurnRequest struct {
	UserID           uint
	SessionID        string
	TurnID           string
	RequestID        string
	Mode             string
	Provider         string
	Model            string
	UserMessage      model.ChatMessage
	AssistantMessage model.ChatMessage
	FullMessages     []model.ChatMessage
	RunMeta          RunMeta
	OccurredAt       time.Time
}

type RunMeta struct {
	InputTokens   int
	OutputTokens  int
	LatencyMS     int64
	StopReason    string
	ErrorMessage  string
	TraceID       string
	IsInterrupted bool
}

type PersistMessageRequest struct {
	UserID     uint
	SessionID  string
	RequestID  string
	TurnID     string
	Message    model.ChatMessage
	Sequence   int64
	Provider   string
	Model      string
	OccurredAt time.Time
}
