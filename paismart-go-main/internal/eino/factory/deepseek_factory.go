package factory

import (
	"context"

	"pai-smart-go/internal/config"
)

// DeepSeekFactory 是 DeepSeek 产品族工厂。
type DeepSeekFactory struct {
	cfg config.EinoConfig
}

// NewDeepSeekFactory 创建 DeepSeek 工厂。
func NewDeepSeekFactory(cfg config.EinoConfig) *DeepSeekFactory {
	return &DeepSeekFactory{
		cfg: cfg,
	}
}

// Create 创建 DeepSeek 产品族。
func (f *DeepSeekFactory) Create(ctx context.Context) (AIProducts, error) {
	return NewDeepSeekProducts(ctx, f.cfg)
}
