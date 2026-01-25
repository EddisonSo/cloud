package events

// Event types for user management
// These types must match the events published by auth-service

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

// CachedUser represents a user in the local cache
type CachedUser struct {
	UserID      string `json:"user_id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
}
