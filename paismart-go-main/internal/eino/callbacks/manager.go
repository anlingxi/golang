package callbacks

import (
	"context"

	"pai-smart-go/internal/config"

	einocb "github.com/cloudwego/eino/callbacks"
)

type Manager struct {
	cfg      config.EinoCallbackConfig
	handlers []einocb.Handler
}

func NewManager(cfg config.EinoCallbackConfig) *Manager {
	handlers := make([]einocb.Handler, 0, 2)

	if cfg.EnableLogging {
		handlers = append(handlers, NewLoggingHandler())
	}

	return &Manager{
		cfg:      cfg,
		handlers: handlers,
	}
}

func (m *Manager) Enabled() bool {
	return m != nil && len(m.handlers) > 0
}

func (m *Manager) InitCallbacks(ctx context.Context, meta RunMeta) context.Context {
	if !m.Enabled() {
		return ctx
	}
	return einocb.InitCallbacks(ctx, BuildRunInfo(meta), m.handlers...)
}

func (m *Manager) ReuseForStage(ctx context.Context, meta RunMeta) context.Context {
	if !m.Enabled() {
		return ctx
	}
	return einocb.ReuseHandlers(ctx, BuildRunInfo(meta))
}
