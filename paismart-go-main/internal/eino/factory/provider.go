package factory

import (
	einoembedding "github.com/cloudwego/eino/components/embedding"
<<<<<<< HEAD
=======
	fmodel "github.com/cloudwego/eino/components/model"
>>>>>>> 36dc5c1 (first commit)

	einotypes "pai-smart-go/internal/eino/types"
	"pai-smart-go/internal/service"
)

type ModelCapabilities struct {
	SupportsStreaming bool
	SupportsSearch    bool
	SupportsEmbedding bool
}

// AIProducts 表示某个 provider 工厂创建出来的一整组 AI 产品。
type AIProducts interface {
	ChatModel() service.ChatModel
<<<<<<< HEAD
=======
	// EinoChatModel 暴露底层 Eino 原生 ToolCallingChatModel，供 ADK Agent 直接使用。
	// 与 EinoEmbedder() 对称，业务层用 ChatModel()，ADK 层用 EinoChatModel()。
	EinoChatModel() fmodel.ToolCallingChatModel
>>>>>>> 36dc5c1 (first commit)
	MessageConverter() einotypes.MessageConverter

	// 直接暴露 Eino 官方 Embedder，供 document pipeline / semantic splitter / indexer 使用
	EinoEmbedder() einoembedding.Embedder

	Capabilities() ModelCapabilities
}
