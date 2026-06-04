package httpapi

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/dknathalage/tallyo/internal/auth"
)

type ctxKey int

const userCtxKey ctxKey = 0

// Recover turns panics into 500s without crashing the server.
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("panic: %v", rec)
				WriteError(w, http.StatusInternalServerError, "internal error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// RequestLogger logs method, path, status, and duration for each request.
func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		log.Printf("%s %s %d %s", r.Method, r.URL.Path, sw.status, time.Since(start))
	})
}

// statusWriter captures the status code written to the response.
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (s *statusWriter) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// Unwrap exposes the wrapped writer so http.ResponseController can reach
// optional interfaces (e.g. http.Flusher) on writers further down the chain.
// Without this, streaming endpoints (SSE) cannot flush through this wrapper.
func (s *statusWriter) Unwrap() http.ResponseWriter {
	return s.ResponseWriter
}

// RequireAuth requires a valid session whose userID maps to an existing user.
// The user is re-checked against the store on every request so deleting a user
// invalidates their session immediately. Nil dependencies are programmer errors.
func RequireAuth(sm *scs.SessionManager, users *auth.UsersRepo) func(http.Handler) http.Handler {
	if sm == nil || users == nil {
		panic("RequireAuth: nil dep")
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := sm.GetInt(r.Context(), "userID")
			if id == 0 {
				WriteError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			u, err := users.GetByID(r.Context(), int64(id))
			if err != nil {
				WriteError(w, http.StatusInternalServerError, "internal error")
				return
			}
			if u == nil { // user deleted → invalidate session
				if derr := sm.Destroy(r.Context()); derr != nil {
					log.Printf("RequireAuth: destroy session: %v", derr)
				}
				WriteError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			ctx := context.WithValue(r.Context(), userCtxKey, u)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// UserFrom returns the authenticated user stored on the request context, or nil.
func UserFrom(ctx context.Context) *auth.User {
	u, _ := ctx.Value(userCtxKey).(*auth.User)
	return u
}
