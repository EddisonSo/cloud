package db

// NewWithUsersForTest builds a DB whose in-memory user cache is pre-seeded and
// which has NO underlying *sql.DB connection. It is intended solely for handler
// tests that exercise cache-hit read paths (e.g. GetUserByUsername /
// GetUserByID) without a live Postgres. Any method that falls through to a real
// query will panic on the nil embedded *sql.DB, which keeps accidental misuse
// loud.
func NewWithUsersForTest(users ...*User) *DB {
	db := &DB{
		usersByUsername: make(map[string]*User),
		usersByID:       make(map[string]*User),
	}
	for _, u := range users {
		db.usersByUsername[u.Username] = u
		db.usersByID[u.UserID] = u
	}
	return db
}
