package events

// Event types for auth service
// Using JSON serialization for now, will migrate to protobuf later

// EventMetadata contains common event information
type EventMetadata struct {
	EventID   string `json:"event_id"`
	EntityID  string `json:"entity_id"`
	Timestamp int64  `json:"timestamp"`
	Source    string `json:"source"`
	Version   int64  `json:"version,omitempty"`
}

// UserCreated event - published when a new user registers
type UserCreated struct {
	Metadata    EventMetadata `json:"metadata"`
	UserID      string        `json:"user_id"`
	Username    string        `json:"username"`
	DisplayName string        `json:"display_name"`
}

// UserDeleted event - published when a user is deleted
type UserDeleted struct {
	Metadata EventMetadata `json:"metadata"`
	UserID   string        `json:"user_id"`
	Username string        `json:"username"`
}

// UserUpdated event - published when user profile is updated
type UserUpdated struct {
	Metadata    EventMetadata `json:"metadata"`
	UserID      string        `json:"user_id"`
	Username    string        `json:"username"`
	DisplayName string        `json:"display_name"`
}

// SessionCreated event - published when a session is created (login)
type SessionCreated struct {
	Metadata  EventMetadata `json:"metadata"`
	SessionID string        `json:"session_id"`
	UserID    string        `json:"user_id"`
	ExpiresAt int64         `json:"expires_at"`
}

// SessionInvalidated event - published when a session is invalidated (logout)
type SessionInvalidated struct {
	Metadata  EventMetadata `json:"metadata"`
	SessionID string        `json:"session_id"`
	UserID    string        `json:"user_id"`
}

// IdentityPermissionsUpdated event - published when a service account's permissions are created or updated
type IdentityPermissionsUpdated struct {
	Metadata         EventMetadata       `json:"metadata"`
	ServiceAccountID string              `json:"service_account_id"`
	UserID           string              `json:"user_id"`
	Scopes           map[string][]string `json:"scopes"`
	Version          int64               `json:"version"`
}

// IdentityPermissionsDeleted event - published when a service account is deleted
type IdentityPermissionsDeleted struct {
	Metadata         EventMetadata `json:"metadata"`
	ServiceAccountID string        `json:"service_account_id"`
	UserID           string        `json:"user_id"`
	Version          int64         `json:"version"`
}
