package callbacks

import (
	einocb "github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components"
)

const (
	StageChatGenerate = "chat.generate"
	StageChatRetrieve = "chat.retrieve"
	StageChatModel    = "chat.model"

	StageHistoryLoad         = "history.load"
	StageHistoryCacheRead    = "history.cache.read"
	StageHistoryCacheWrite   = "history.cache.write"
	StageHistoryArchiveRead  = "history.archive.read"
	StageHistoryEventPublish = "history.event.publish"
	StageHistoryDegrade      = "history.degrade"
)

func BuildRunInfo(meta RunMeta) *einocb.RunInfo {
	comp := components.Component("Unknown")

	switch meta.Stage {
	case StageChatModel:
		comp = components.Component("ChatModel")
	case StageChatRetrieve:
		comp = components.Component("Retriever")
	case StageChatGenerate:
		comp = components.Component("Chain")
	case StageHistoryLoad:
		comp = components.Component("HistoryLoader")
	case StageHistoryCacheRead, StageHistoryCacheWrite:
		comp = components.Component("Store")
	case StageHistoryArchiveRead:
		comp = components.Component("Archive")
	case StageHistoryEventPublish:
		comp = components.Component("EventBus")
	case StageHistoryDegrade:
		comp = components.Component("Fallback")
	}

	return &einocb.RunInfo{
		Name:      meta.ComponentName(),
		Type:      meta.Provider,
		Component: comp,
	}
}
