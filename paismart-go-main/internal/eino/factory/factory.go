package factory

import "context"

// AIFactory 是顶层抽象工厂。
// 不再只产出单一 ChatModel，而是产出一整组 AI 产品。
type AIFactory interface {
	Create(ctx context.Context) (AIProducts, error)
}
