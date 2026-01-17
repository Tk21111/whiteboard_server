package auth

import (
	"net/http"

	"github.com/Tk21111/whiteboard_server/session"
)

func HandleAuthAsset() http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		token := r.URL.Query().Get("token")
		if token == "" {
			http.Error(w, "missing token", http.StatusBadRequest)
			return
		}

		userId, _, _, err := VerifyIDToken(token)
		if err != nil {
			http.Error(w, "invalid token", http.StatusForbidden)
			return
		}

		sessionToken, err := session.Create(userId)
		if err != nil {
			http.Error(w, "fail to create seesion", http.StatusInternalServerError)
			return
		}

		http.SetCookie(w, &http.Cookie{
			Name:     "assetCookie",
			Value:    sessionToken,
			Path:     "/",
			HttpOnly: true,
			Secure:   false,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   60 * 60 * 24 * 7,
		})

		w.WriteHeader(http.StatusOK)
	})
}

func HandleValidate() http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" {
			http.Error(w, "missing token", http.StatusBadRequest)
			return
		}

		token, err := ReadBearer(r)
		if token == "" || err != nil {
			http.Error(w, "cannot read token", 400)
		}

		_, _, _, err = VerifyIDToken(token)
		if err != nil {
			http.Error(w, "invalid token", http.StatusForbidden)
			return
		}

		w.WriteHeader(http.StatusOK)
	})
}
