package auth

import (
	"context"
	"errors"

	"google.golang.org/api/idtoken"
)

var googleClientID = "YOUR_GOOGLE_CLIENT_ID"

func VerifyIDToken(token string) (string, error) {
	payload, err := idtoken.Validate(context.Background(), token, googleClientID)
	if err != nil {
		return "", err
	}

	sub, ok := payload.Claims["sub"].(string)
	if !ok {
		return "", errors.New("invalid sub")
	}

	return sub, nil
}
