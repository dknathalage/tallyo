package recurring

// NOTE (J4): rewritten for the tenant-scoped recurring_templates schema.
// Templates carry client_id / payer_id and a JSON line_items column.
// The stored line shape is catalogue-aware (code, serviceDate, unit, unitPrice,
// taxable). tax_rate is a stored percentage on the template; generation computes
// the tax amount from it. price-cap / plan-window validation is J10.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/dknathalage/tallyo/internal/audit"
	"github.com/dknathalage/tallyo/internal/billing"
	"github.com/dknathalage/tallyo/internal/db"
	"github.com/dknathalage/tallyo/internal/db/gen"
	"github.com/dknathalage/tallyo/internal/ids"
)

// Repo reads and writes recurring_templates (tenant-scoped) and
// generates invoices from them. Generation advances next_due in the same
// transaction as the invoice insert so re-running the due sweep never
// double-generates.
type Repo struct {
	db   db.Executor
	snap *billing.SnapshotBuilder
}

// NewRepo constructs a repository. A nil db is a programmer error.
func NewRepo(db db.Executor) *Repo {
	if db == nil {
		panic("recurring: NewRepo requires a non-nil *sql.DB")
	}
	return &Repo{db: db, snap: billing.NewSnapshotBuilder(db)}
}

// validFrequencies is the closed set of supported cadences.
var validFrequencies = map[string]bool{"weekly": true, "monthly": true, "quarterly": true}

// errClientNotFound / errPayerNotFound are the sentinels returned when
// an inbound uuid does not resolve to a row in the tenant. Handlers map them to a
// 400 validation error.
var (
	errClientNotFound = errors.New("recurring: client not found")
	errPayerNotFound  = errors.New("recurring: payer not found")
)

// validate checks a writable template input at the module boundary.
func (r *Repo) validate(in RecurringInput) error {
	if in.Name == "" {
		return errors.New("recurring: name is required")
	}
	if in.ClientUUID == nil || *in.ClientUUID == "" {
		return errors.New("recurring: client required")
	}
	if !validFrequencies[in.Frequency] {
		return errors.New("recurring: invalid frequency")
	}
	if in.NextDue == "" {
		return errors.New("recurring: next due is required")
	}
	return nil
}

// resolveClient resolves the required inbound client uuid to the client row id
// (uuid) for insert/update. An unknown uuid (foreign or absent) → errClientNotFound.
func (r *Repo) resolveClient(ctx context.Context, q *gen.Queries, tenantID string, pUUID *string) (sql.NullString, error) {
	if pUUID == nil || *pUUID == "" {
		return sql.NullString{}, errClientNotFound
	}
	id, err := q.GetClientIDByUUID(ctx, gen.GetClientIDByUUIDParams{TenantID: tenantID, ID: *pUUID})
	if errors.Is(err, sql.ErrNoRows) {
		return sql.NullString{}, errClientNotFound
	}
	if err != nil {
		return sql.NullString{}, fmt.Errorf("resolve client: %w", err)
	}
	return sql.NullString{String: id, Valid: true}, nil
}

// resolvePayer resolves an optional inbound payer uuid to the payer row id
// (uuid). A nil/empty uuid → NULL FK. An unknown uuid → errPayerNotFound.
func (r *Repo) resolvePayer(ctx context.Context, q *gen.Queries, tenantID string, pmUUID *string) (sql.NullString, error) {
	if pmUUID == nil || *pmUUID == "" {
		return sql.NullString{}, nil
	}
	id, err := q.GetPayerIDByUUID(ctx, gen.GetPayerIDByUUIDParams{TenantID: tenantID, ID: *pmUUID})
	if errors.Is(err, sql.ErrNoRows) {
		return sql.NullString{}, errPayerNotFound
	}
	if err != nil {
		return sql.NullString{}, fmt.Errorf("resolve payer: %w", err)
	}
	return sql.NullString{String: id, Valid: true}, nil
}

// Get returns the template by uuid (with client/payer uuids, name,
// and line items), or (nil, nil) when absent. Internal callers (generation,
// delete) rely on the (nil, nil) skip; the HTTP CRUD path 404s on nil.
func (r *Repo) Get(ctx context.Context, tenantID string, uuid string) (*RecurringTemplate, error) {
	row, err := gen.New(r.db).GetRecurringTemplate(ctx, gen.GetRecurringTemplateParams{TenantID: tenantID, ID: uuid})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get recurring: %w", err)
	}
	return getRowToTemplate(row), nil
}

// Create validates and inserts a template, auditing the create with the real id,
// then re-reads the row.
func (r *Repo) Create(ctx context.Context, tenantID string, in RecurringInput) (*RecurringTemplate, error) {
	if tenantID == "" {
		return nil, errors.New("create recurring: tenant id required")
	}
	if err := r.validate(in); err != nil {
		return nil, err
	}
	lineItemsJSON, err := marshalLines(in.LineItems)
	if err != nil {
		return nil, fmt.Errorf("create recurring: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	var newUUID string
	err = audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		q := gen.New(tx)
		pid, e := r.resolveClient(ctx, q, tenantID, in.ClientUUID)
		if e != nil {
			return e
		}
		pmID, e := r.resolvePayer(ctx, q, tenantID, in.PayerUUID)
		if e != nil {
			return e
		}
		tpl, e := q.CreateRecurringTemplate(ctx, gen.CreateRecurringTemplateParams{
			ID:        ids.New(),
			TenantID:  tenantID,
			ClientID:  pid,
			PayerID:   pmID,
			Name:      in.Name,
			Frequency: in.Frequency,
			NextDue:   in.NextDue,
			LineItems: lineItemsJSON,
			TaxRate:   in.TaxRate,
			Notes:     in.Notes,
			IsActive:  db.B2i(in.IsActive),
			CreatedAt: now,
			UpdatedAt: now,
		})
		if e != nil {
			return fmt.Errorf("insert: %w", e)
		}
		newUUID = tpl.ID
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "recurring_template", EntityID: tpl.ID, Action: "create",
		})
	})
	if errors.Is(err, errClientNotFound) {
		return nil, errClientNotFound
	}
	if errors.Is(err, errPayerNotFound) {
		return nil, errPayerNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("create recurring: %w", err)
	}
	return r.Get(ctx, tenantID, newUUID)
}

// Update validates and rewrites a template by uuid, atomically with one audit
// row. Returns (nil, nil) when the template does not exist. The audit entry
// records the row's id (uuid), resolved by-uuid inside the same tx.
func (r *Repo) Update(ctx context.Context, tenantID string, uuid string, in RecurringInput) (*RecurringTemplate, error) {
	if err := r.validate(in); err != nil {
		return nil, err
	}
	lineItemsJSON, err := marshalLines(in.LineItems)
	if err != nil {
		return nil, fmt.Errorf("update recurring: %w", err)
	}
	var missing bool
	err = audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		q := gen.New(tx)
		existing, e := q.GetRecurringTemplate(ctx, gen.GetRecurringTemplateParams{TenantID: tenantID, ID: uuid})
		if errors.Is(e, sql.ErrNoRows) {
			missing = true
			return e
		}
		if e != nil {
			return fmt.Errorf("load existing: %w", e)
		}
		pid, e := r.resolveClient(ctx, q, tenantID, in.ClientUUID)
		if e != nil {
			return e
		}
		pmID, e := r.resolvePayer(ctx, q, tenantID, in.PayerUUID)
		if e != nil {
			return e
		}
		if _, e := q.UpdateRecurringTemplate(ctx, gen.UpdateRecurringTemplateParams{
			ClientID:  pid,
			PayerID:   pmID,
			Name:      in.Name,
			Frequency: in.Frequency,
			NextDue:   in.NextDue,
			LineItems: lineItemsJSON,
			TaxRate:   in.TaxRate,
			Notes:     in.Notes,
			IsActive:  db.B2i(in.IsActive),
			UpdatedAt: time.Now().UTC().Format(time.RFC3339),
			TenantID:  tenantID,
			ID:        uuid,
		}); e != nil {
			return fmt.Errorf("update: %w", e)
		}
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "recurring_template", EntityID: existing.ID, Action: "update",
		})
	})
	if missing {
		return nil, nil
	}
	if errors.Is(err, errClientNotFound) {
		return nil, errClientNotFound
	}
	if errors.Is(err, errPayerNotFound) {
		return nil, errPayerNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("update recurring: %w", err)
	}
	return r.Get(ctx, tenantID, uuid)
}

// Delete removes a template by uuid and writes one audit row, atomically. The
// audit entry records the row's id (uuid), resolved by-uuid in the same tx. A
// missing row is a silent no-op.
func (r *Repo) Delete(ctx context.Context, tenantID string, uuid string) error {
	return audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		q := gen.New(tx)
		row, e := q.GetRecurringTemplate(ctx, gen.GetRecurringTemplateParams{TenantID: tenantID, ID: uuid})
		if errors.Is(e, sql.ErrNoRows) {
			return nil
		}
		if e != nil {
			return fmt.Errorf("lookup: %w", e)
		}
		if e := q.DeleteRecurringTemplate(ctx, gen.DeleteRecurringTemplateParams{TenantID: tenantID, ID: uuid}); e != nil {
			return fmt.Errorf("delete: %w", e)
		}
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "recurring_template", EntityID: row.ID, Action: "delete",
		})
	})
}
