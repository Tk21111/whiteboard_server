package middleware

import (
	"context"
	"fmt"
	"net/http"

	"github.com/Tk21111/whiteboard_server/auth"
	"github.com/Tk21111/whiteboard_server/config"
	"github.com/Tk21111/whiteboard_server/db"
)

func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, err := auth.ReadBearer(r)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		user, err := auth.VerifyIDToken(token)
		if err != nil {
			fmt.Println(err)
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}

		ctx := r.Context()
		ctx = context.WithValue(ctx, config.ContextUserIDKey, user.UserID)
		ctx = context.WithValue(ctx, config.ContextUserNameKey, user.Name)
		ctx = context.WithValue(ctx, config.ContextUserPicKey, user.Picture)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

var allowedOrigins = map[string]bool{
	"http://localhost:5173":                                    true,
	"http://localhost:3000":                                    true,
	"http://127.0.0.1:5173":                                    true,
	"http://192.168.1.105:5173":                                true,
	"https://noctambulous-logan-multicircuited.ngrok-free.dev": true,
	"https://whiteboard.printhelloworld.xyz":                   true,
}

func CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")

		if allowedOrigins[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}

		w.Header().Set("Cross-Origin-Opener-Policy", "same-origin-allow-popups")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Credentials", "true")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func RequireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		cookie, err := r.Cookie("assetCookie")
		if err != nil {
			http.Error(w, "no cookie", http.StatusUnauthorized)
			return
		}

		claims, err := auth.ParseJWT(cookie.Value)
		if err != nil {
			http.Error(w, "parse jwt fail", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(
			r.Context(),
			config.ContextUserIDKey,
			claims.UserID,
		)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Helper to avoid logging secrets
func short(s string) string {
	if len(s) <= 6 {
		return s
	}
	return s[:6] + "..."
}

func RequireRole(next http.Handler, reqRole int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		role, err := db.GetUserRole(r.Context().Value(config.ContextUserIDKey).(string))
		if err != nil {
			http.Error(w, "cannot get user role", 500)
			return
		}

		if role < reqRole {
			http.Error(w, "no perm", 403)
			return
		}

		next.ServeHTTP(w, r)
	})
}
