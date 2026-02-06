package api

import (
	"context"
)

type contextKey string

const userContextKey contextKey = "user"

type userInfo struct {
	UserID   string // nanoid
	Username string
	AuthType string              // "session" or "api_token"
	Scopes   map[string][]string // nil for session (full access)
}

func setUserContext(ctx context.Context, userID, username string) context.Context {
	return context.WithValue(ctx, userContextKey, &userInfo{
		UserID:   userID,
		Username: username,
		AuthType: "session",
	})
}

func setAPITokenContext(ctx context.Context, userID string, scopes map[string][]string) context.Context {
	return context.WithValue(ctx, userContextKey, &userInfo{
		UserID:   userID,
		AuthType: "api_token",
		Scopes:   scopes,
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

func getUserInfoFromContext(ctx context.Context) *userInfo {
	info, ok := ctx.Value(userContextKey).(*userInfo)
	if !ok {
		return nil
	}
	return info
}
