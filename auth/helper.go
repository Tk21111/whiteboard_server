package auth

import "net/http"

func RequireUserId(r *http.Request) string {
	return r.Context().Value("userId").(string)
}
