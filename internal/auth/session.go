package auth

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/alexedwards/scs/sqlite3store"
	"github.com/alexedwards/scs/v2"
)

// NewSessionManager builds an scs manager backed by the auth_sessions table.
// (The table is auth_sessions, not "sessions", to avoid colliding with the
// session-entity table in the tenant DBs — sqlc reads both schemas into one
// type catalog.) secure=true sets the Secure cookie flag (enable behind TLS).
func NewSessionManager(db *sql.DB, secure bool) *scs.SessionManager {
	if db == nil {
		panic("NewSessionManager: nil db")
	}
	m := scs.New()
	// auth_sessions store; 5-min cleanup goroutine (the prior New default).
	m.Store = sqlite3store.NewWithConfig(db, sqlite3store.Config{
		TableName:       "auth_sessions",
		CleanUpInterval: 5 * time.Minute,
	})
	m.Lifetime = 7 * 24 * time.Hour
	m.Cookie.HttpOnly = true
	m.Cookie.SameSite = http.SameSiteLaxMode
	m.Cookie.Secure = secure
	m.Cookie.Path = "/"
	return m
}
