package auth

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/alexedwards/scs/postgresstore"
	"github.com/alexedwards/scs/v2"
)

// NewSessionManager builds an scs manager backed by the sessions table.
// secure=true sets the Secure cookie flag (enable behind TLS).
func NewSessionManager(db *sql.DB, secure bool) *scs.SessionManager {
	if db == nil {
		panic("NewSessionManager: nil db")
	}
	m := scs.New()
	m.Store = postgresstore.New(db) // starts a 5-min cleanup goroutine
	m.Lifetime = 7 * 24 * time.Hour
	m.Cookie.HttpOnly = true
	m.Cookie.SameSite = http.SameSiteLaxMode
	m.Cookie.Secure = secure
	m.Cookie.Path = "/"
	return m
}
