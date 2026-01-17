package auth

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"

	"google.golang.org/api/idtoken"
)

var googleClientID = os.Getenv("GOOGLE_CLIENT_ID")

func VerifyIDToken(token string) (userID, name, profilePic string, err error) {
	payload, err := idtoken.Validate(context.Background(), token, googleClientID)
	if err != nil {
		return "", "", "", err
	}

	// user id
	sub, ok := payload.Claims["sub"].(string)
	if !ok {
		return "", "", "", errors.New("invalid sub")
	}

	// display name
	name, _ = payload.Claims["name"].(string)

	// profile picture
	profilePic, _ = payload.Claims["picture"].(string)

	return sub, name, profilePic, nil
}

func ReadBearer(r *http.Request) (string, error) {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return "", errors.New("No auth header")
	}

	const prefix = "Bearer "
	if !strings.HasPrefix(auth, prefix) {
		return "", errors.New("Invalid auth header")
	}

	token := strings.TrimSpace(auth[len((prefix)):])
	if token == "" {
		return "", errors.New("empty token")
	}

	return token, nil
}
