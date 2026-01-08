package logx

import (
	"context"

	"go.uber.org/zap"
)

type ctxKey string

const loggerKey ctxKey = "logger"

func With(ctx context.Context, fields ...zap.Field) context.Context {
	l := From(ctx).With(fields...)
	return context.WithValue(ctx, loggerKey, l)
}

func From(ctx context.Context) *zap.Logger {
	if l, ok := ctx.Value(loggerKey).(*zap.Logger); ok {
		return l
	}
	return L
}
