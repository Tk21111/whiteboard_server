package auth

import (
	"context"
	"errors"

	"github.com/Tk21111/whiteboard_server/config"
)

func RequireUserId(ctx context.Context) (string, error) {
	id, ok := ctx.Value(config.ContextUserIDKey).(string)
	if !ok || id == "" {
		return "", errors.New("user not authenticated")
	}
	return id, nil
}
