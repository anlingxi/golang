package factory

import (
	einoembedding "github.com/cloudwego/eino/components/embedding"

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
	MessageConverter() einotypes.MessageConverter

	// 直接暴露 Eino 官方 Embedder，供 document pipeline / semantic splitter / indexer 使用
	EinoEmbedder() einoembedding.Embedder

	Capabilities() ModelCapabilities
}
