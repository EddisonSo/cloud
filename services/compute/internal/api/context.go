package api

import (
	"context"
)

type contextKey string

const userContextKey contextKey = "user"

type userInfo struct {
	InternalID int64  // DB integer ID
	UserID     string // nanoid for external use
	Username   string
}

func setUserContext(ctx context.Context, internalID int64, userID, username string) context.Context {
	return context.WithValue(ctx, userContextKey, &userInfo{
		InternalID: internalID,
		UserID:     userID,
		Username:   username,
	})
}

// getUserFromContext returns (internalID, userID, username, ok)
// internalID is the DB integer ID, userID is the nanoid
func getUserFromContext(ctx context.Context) (int64, string, string, bool) {
	info, ok := ctx.Value(userContextKey).(*userInfo)
	if !ok || info == nil {
		return 0, "", "", false
	}
	return info.InternalID, info.UserID, info.Username, true
}
