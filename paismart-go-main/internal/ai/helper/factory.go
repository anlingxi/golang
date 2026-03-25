package helper

import (
	"pai-smart-go/internal/ai/history"
	"pai-smart-go/internal/service"
)

// DefaultFactory 是 AIHelperFactory 的默认实现。
type DefaultFactory struct {
	chatService    service.ChatService
	historyManager history.Manager
}

// NewDefaultFactory 创建默认 AIHelper 工厂。
func NewDefaultFactory(
	chatService service.ChatService,
	historyManager history.Manager,
) AIHelperFactory {
	return &DefaultFactory{
		chatService:    chatService,
		historyManager: historyManager,
	}
}

// Create 创建一个新的 AIHelper。
func (f *DefaultFactory) Create(userID uint, sessionID string) (*AIHelper, error) {
	// 这里应该返回&AIHelper实例，为什么直接返回NewAIHelper？因为 NewAIHelper 已经封装了 AIHelper 的创建逻辑，包括初始化和依赖注入，
	// 所以直接调用它可以简化代码。
	return NewAIHelper(
		userID,
		sessionID,
		f.chatService,
		f.historyManager,
	), nil
}
