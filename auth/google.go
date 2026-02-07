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

type GoogleUser struct {
	UserID    string
	Name      string
	GivenName string
	Email     string
	Picture   string
}

func VerifyIDToken(token string) (*GoogleUser, error) {
	payload, err := idtoken.Validate(context.Background(), token, googleClientID)
	if err != nil {
		return nil, err
	}

	claims := payload.Claims

	// Required
	sub, ok := claims["sub"].(string)
	if !ok || sub == "" {
		return nil, errors.New("invalid sub claim")
	}

	// Optional but useful
	name, _ := claims["name"].(string)
	givenName, _ := claims["given_name"].(string)
	email, _ := claims["email"].(string)
	picture, _ := claims["picture"].(string)

	return &GoogleUser{
		UserID:    sub,
		Name:      name,
		GivenName: givenName,
		Email:     email,
		Picture:   picture,
	}, nil
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
