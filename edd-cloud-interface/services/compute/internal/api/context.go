package api

import (
	"context"
)

type contextKey string

const userContextKey contextKey = "user"

type userInfo struct {
	UserID   string // nanoid
	Username string
}

func setUserContext(ctx context.Context, userID, username string) context.Context {
	return context.WithValue(ctx, userContextKey, &userInfo{
		UserID:   userID,
		Username: username,
	})
}

// getUserFromContext returns (userID, username, ok)
func getUserFromContext(ctx context.Context) (string, string, bool) {
	info, ok := ctx.Value(userContextKey).(*userInfo)
	if !ok || info == nil {
		return "", "", false
	}
	return info.UserID, info.Username, true
}
