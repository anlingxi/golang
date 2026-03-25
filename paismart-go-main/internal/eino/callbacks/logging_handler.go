package callbacks

import (
	"context"
	"fmt"
	"time"

	"pai-smart-go/pkg/log"

	einocb "github.com/cloudwego/eino/callbacks"
)

type callbackStartAtKey struct{}

func NewLoggingHandler() einocb.Handler {
	return einocb.NewHandlerBuilder().
		OnStartFn(func(ctx context.Context, info *einocb.RunInfo, input einocb.CallbackInput) context.Context {
			startAt := time.Now()

			name := ""
			typ := ""
			component := ""

			if info != nil {
				name = info.Name
				typ = info.Type
				component = fmt.Sprintf("%v", info.Component)
			}

			log.Infof("[EinoCallback] start name=%s type=%s component=%s",
				name, typ, component)

			return context.WithValue(ctx, callbackStartAtKey{}, startAt)
		}).
		OnEndFn(func(ctx context.Context, info *einocb.RunInfo, output einocb.CallbackOutput) context.Context {
			var cost time.Duration
			if v := ctx.Value(callbackStartAtKey{}); v != nil {
				if startAt, ok := v.(time.Time); ok {
					cost = time.Since(startAt)
				}
			}

			name := ""
			typ := ""
			component := ""

			if info != nil {
				name = info.Name
				typ = info.Type
				component = fmt.Sprintf("%v", info.Component)
			}

			log.Infof("[EinoCallback] end name=%s type=%s component=%s duration=%s",
				name, typ, component, cost)

			return ctx
		}).
		OnErrorFn(func(ctx context.Context, info *einocb.RunInfo, err error) context.Context {
			name := ""
			typ := ""
			component := ""

			if info != nil {
				name = info.Name
				typ = info.Type
				component = fmt.Sprintf("%v", info.Component)
			}

			log.Errorf("[EinoCallback] error name=%s type=%s component=%s err=%v",
				name, typ, component, err)

			return ctx
		}).
		Build()
}
