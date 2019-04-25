package contextutils

import (
	"context"

	"go.uber.org/zap"
)

func SilenceLogger(ctx context.Context) context.Context {
	return withLogger(ctx, zap.NewNop().Sugar())
}

func WithLogger(ctx context.Context, name string) context.Context {
	return withLogger(ctx, fromContext(ctx).Named(name))
}

func WithLoggerValues(ctx context.Context, meta ...interface{}) context.Context {
	return withLogger(ctx, fromContext(ctx).With(meta...))
}

func LoggerFrom(ctx context.Context) *zap.SugaredLogger {
	return fromContext(ctx)
}

type ErrorHandler interface {
	HandleErr(error)
}

type ErrorLogger struct {
	ctx context.Context
}

func (h *ErrorLogger) HandleErr(err error) {
	if err == nil {
		return
	}
	fromContext(h.ctx).Errorf(err.Error())
}

type errorHandlerKey struct{}

func WithErrorHandler(ctx context.Context, errorHandler ErrorHandler) context.Context {
	return context.WithValue(ctx, errorHandlerKey{}, errorHandler)
}

func ErrorHandlerFrom(ctx context.Context) ErrorHandler {
	val := ctx.Value(errorHandlerKey{})
	if val == nil {
		return &ErrorLogger{
			ctx: ctx,
		}
	}
	errorHandler, ok := val.(ErrorHandler)
	if !ok {
		return &ErrorLogger{
			ctx: ctx,
		}
	}
	return errorHandler
}
