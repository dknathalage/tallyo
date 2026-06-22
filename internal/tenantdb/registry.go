// Package tenantdb owns the per-tenant SQLite handle registry for the
// DB-per-tenant model. The control DB (tenants/users/sessions/catalogue) is one
// shared *sql.DB; each tenant's business data lives in its own
// tenants/tenant-<id>.db file, opened on demand and cached in a bounded LRU.
package tenantdb

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	appdb "github.com/dknathalage/tallyo/internal/db"
	"github.com/dknathalage/tallyo/internal/reqctx"
)

const (
	// maxOpen bounds how many tenant handles stay open at once. Hundreds of
	// tenants per deployment; reopening on a miss is cheap.
	maxOpen = 100
	// idleTTL: a handle untouched this long is eligible for eviction. Anything
	// used more recently is never closed, so an in-flight request's handle
	// survives. ponytail: idle handles closed; reopened on next hit.
	idleTTL = 5 * time.Minute
)

type entry struct {
	db       *sql.DB
	lastUsed time.Time
}

// Registry hands out the control handle and per-tenant handles. Safe for
// concurrent use.
type Registry struct {
	control *sql.DB
	dataDir string

	mu       sync.Mutex
	open     map[int64]*entry
	migrated map[int64]bool   // tenant DBs migrated this process
	uuids    map[int64]string // tenant id -> uuid (file name), cached from control
}

// New builds a registry over an already-open control DB and a data dir. Tenant
// files live under <dataDir>/tenants/.
func New(control *sql.DB, dataDir string) *Registry {
	if control == nil {
		panic("tenantdb: New requires a non-nil control *sql.DB")
	}
	return &Registry{
		control:  control,
		dataDir:  dataDir,
		open:     make(map[int64]*entry),
		migrated: make(map[int64]bool),
		uuids:    make(map[int64]string),
	}
}

// Control returns the shared control-plane handle.
func (r *Registry) Control() *sql.DB { return r.control }

// ForTenant resolves the tenant from the request context and returns its handle.
func (r *Registry) ForTenant(ctx context.Context) (*sql.DB, error) {
	id, ok := reqctx.TenantFrom(ctx)
	if !ok || id <= 0 {
		return nil, fmt.Errorf("tenantdb: no tenant in context")
	}
	return r.ForTenantID(id)
}

// ForTenantID returns the handle for an explicit tenant id (used by sweeps and
// provisioning, which run without a request context). Opens + migrates on first
// use, then caches.
func (r *Registry) ForTenantID(id int64) (*sql.DB, error) {
	if id <= 0 {
		return nil, fmt.Errorf("tenantdb: invalid tenant id %d", id)
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	if e, ok := r.open[id]; ok {
		e.lastUsed = time.Now()
		return e.db, nil
	}

	path, err := r.pathLocked(id)
	if err != nil {
		return nil, err
	}
	conn, err := appdb.Open(path)
	if err != nil {
		return nil, fmt.Errorf("tenantdb: open tenant %d: %w", id, err)
	}
	conn.SetMaxOpenConns(4) // each tenant is low-traffic
	conn.SetMaxIdleConns(4)

	if !r.migrated[id] {
		if err := appdb.MigrateTenant(conn); err != nil {
			conn.Close()
			return nil, fmt.Errorf("tenantdb: migrate tenant %d: %w", id, err)
		}
		r.migrated[id] = true
	}

	r.open[id] = &entry{db: conn, lastUsed: time.Now()}
	r.evictLocked()
	return conn, nil
}

// pathLocked is the file for a tenant's DB, named by the tenant UUID (the stable
// external handle) rather than the control-DB integer id. The id->uuid mapping is
// resolved once from the control DB and cached. Caller holds r.mu.
func (r *Registry) pathLocked(id int64) (string, error) {
	uuid, ok := r.uuids[id]
	if !ok {
		if err := r.control.QueryRowContext(context.Background(),
			"SELECT uuid FROM tenants WHERE id = ?", id).Scan(&uuid); err != nil {
			return "", fmt.Errorf("tenantdb: resolve uuid for tenant %d: %w", id, err)
		}
		r.uuids[id] = uuid
	}
	return filepath.Join(r.dataDir, "tenants", fmt.Sprintf("tenant-%s.db", uuid)), nil
}

// evictLocked closes the least-recently-used IDLE handle while over capacity.
// Only handles idle past idleTTL are closed, so an in-flight request is never
// disturbed. Caller holds r.mu.
func (r *Registry) evictLocked() {
	for len(r.open) > maxOpen {
		var victim int64 = -1
		var oldest time.Time
		for id, e := range r.open {
			if time.Since(e.lastUsed) < idleTTL {
				continue
			}
			if victim == -1 || e.lastUsed.Before(oldest) {
				victim, oldest = id, e.lastUsed
			}
		}
		if victim == -1 {
			return // nothing idle enough to evict; keep over cap rather than risk in-flight
		}
		r.open[victim].db.Close()
		delete(r.open, victim)
	}
}

// Sweep closes every handle idle past idleTTL (called on a ticker). Returns the
// number closed.
func (r *Registry) Sweep() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for id, e := range r.open {
		if time.Since(e.lastUsed) >= idleTTL {
			e.db.Close()
			delete(r.open, id)
			n++
		}
	}
	return n
}

// Close closes the control handle and every open tenant handle. Returns the
// first error. The control DB is closed last.
func (r *Registry) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	var first error
	for id, e := range r.open {
		if err := e.db.Close(); err != nil && first == nil {
			first = err
		}
		delete(r.open, id)
	}
	if err := r.control.Close(); err != nil && first == nil {
		first = err
	}
	return first
}
