package middleware

import (
	"context"
	"net/http"

	"github.com/Tk21111/whiteboard_server/auth"
)

type contextKey string

const userKey contextKey = "user"

func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, err := auth.ReadBearer(r)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		user, err := auth.VerifyIDToken(token)
		if err != nil {
			http.Error(w, "invalid token", http.StatusUnauthorized)
		}

		ctx := context.WithValue(r.Context(), userKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
