package middleware

import (
	"net/http"
	"time"

	"github.com/Tk21111/whiteboard_server/internal/logx"
	"go.uber.org/zap"
)

func Logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		//bind logger with this ctx and add zap ( data into ctx in obj ({}) format)
		ctx := logx.With(r.Context(),
			zap.String("method", r.Method),
			zap.String("path", r.URL.Path),
			zap.String("ip", r.RemoteAddr),
		)

		r = r.WithContext(ctx)
		next.ServeHTTP(w, r)

		//get logger from ctx and add log
		logx.From(ctx).Info("http_request",
			zap.Duration("duration", time.Since(start)),
		)
	})
}
