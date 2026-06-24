// Package payer is the payer vertical slice: domain types, the
// audited repository over the payers table, the service (with SSE
// broadcast), and the HTTP handler. It depends only on platform packages
// (db/gen, audit, reqctx, realtime, httpx), never on other domain slices.
package payer

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/dknathalage/tallyo/internal/db"
	"time"

	"github.com/dknathalage/tallyo/internal/audit"
	"github.com/dknathalage/tallyo/internal/db/gen"
	"github.com/dknathalage/tallyo/internal/ids"
	"github.com/dknathalage/tallyo/internal/listquery"
)

// payerListSelect is the base SELECT for the listquery-driven Query path.
// listquery splices its safe WHERE/ORDER/LIMIT fragments after the tenant filter.
const payerListSelect = `SELECT * FROM payers WHERE tenant_id = ?`

// PayerCols is the listquery allowlist for payers. Keys match the
// JSON field names so the frontend column key drives filter, sort, and display.
var PayerCols = listquery.Spec{
	"name":  {Col: "name", Filter: listquery.Text},
	"email": {Col: "email", Filter: listquery.Text},
	"phone": {Col: "phone", Filter: listquery.Text},
}

// Payer is the domain view of a row in the payers table. All
// nullable columns are unwrapped to plain strings ("" when absent).
type Payer struct {
	ID        int64  `json:"-"`
	UUID      string `json:"id"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	Phone     string `json:"phone"`
	Address   string `json:"address"`
	Metadata  string `json:"metadata"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// PayerInput is the writable subset of a payer.
type PayerInput struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Phone    string `json:"phone"`
	Address  string `json:"address"`
	Metadata string `json:"metadata"`
}

// PayersRepo reads and writes the payers table (tenant-scoped) with
// audited mutations.
type PayersRepo struct {
	db db.Executor
}

// NewPayers constructs a repository. A nil db is a programmer error.
func NewPayers(db db.Executor) *PayersRepo {
	if db == nil {
		panic("payer: NewPayers requires a non-nil *sql.DB")
	}
	return &PayersRepo{db: db}
}

// List returns the tenant's payers ordered by name. When search is
// non-empty it filters to name or email matches (LIKE).
func (r *PayersRepo) List(ctx context.Context, tenantID int64, search string) ([]*Payer, error) {
	q := gen.New(r.db)
	if search == "" {
		rows, err := q.ListPayers(ctx, tenantID)
		if err != nil {
			return nil, fmt.Errorf("list payers: %w", err)
		}
		return mapPayers(rows), nil
	}
	like := "%" + search + "%"
	rows, err := q.SearchPayers(ctx, gen.SearchPayersParams{
		TenantID: tenantID,
		Name:     like,
		Email:    db.Nz(like),
	})
	if err != nil {
		return nil, fmt.Errorf("search payers: %w", err)
	}
	return mapPayers(rows), nil
}

// Get returns the tenant's payer by uuid, or (nil, nil) when none matches.
func (r *PayersRepo) Get(ctx context.Context, tenantID int64, uuid string) (*Payer, error) {
	row, err := gen.New(r.db).GetPayer(ctx, gen.GetPayerParams{TenantID: tenantID, Uuid: uuid})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get payer: %w", err)
	}
	return toPayer(row), nil
}

// Query returns one page of payers plus the total row count for the
// filter (ignoring pagination). The clause is built by listquery from an
// allowlisted spec, so its Where/Order fragments are injection-safe.
func (r *PayersRepo) Query(ctx context.Context, tenantID int64, c listquery.Clause) ([]*Payer, int64, error) {
	if tenantID == 0 {
		return nil, 0, errors.New("query payers: tenant id required")
	}
	var total int64
	countSQL := "SELECT count(*) FROM (" + payerListSelect + c.Where + ")"
	countArgs := append([]any{tenantID}, c.CountArgs()...)
	if err := r.db.QueryRowContext(ctx, countSQL, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count payers: %w", err)
	}
	order := c.Order
	if order == "" {
		order = " ORDER BY name"
	}
	sqlText := payerListSelect + c.Where + order + c.Limit
	pageArgs := append([]any{tenantID}, c.Args...)
	rows, err := r.db.QueryContext(ctx, sqlText, pageArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("query payers: %w", err)
	}
	defer rows.Close()
	out := make([]*Payer, 0, 50)
	for rows.Next() { // bounded by LIMIT in the query
		var row gen.Payer
		var tenant int64
		if err := rows.Scan(&row.ID, &row.Uuid, &tenant, &row.Name, &row.Email,
			&row.Phone, &row.Address, &row.Metadata, &row.CreatedAt, &row.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan payer: %w", err)
		}
		out = append(out, toPayer(row))
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("query payers: %w", err)
	}
	return out, total, nil
}

// Create inserts a payer and writes one audit row, atomically.
func (r *PayersRepo) Create(ctx context.Context, tenantID int64, in PayerInput) (*Payer, error) {
	if tenantID == 0 {
		return nil, errors.New("create payer: tenant id required")
	}
	if in.Name == "" {
		return nil, errors.New("create payer: name is required")
	}
	metadata := in.Metadata
	if metadata == "" {
		metadata = "{}"
	}

	var created gen.Payer
	err := audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339)
		p, e := gen.New(tx).CreatePayer(ctx, gen.CreatePayerParams{
			Uuid:      ids.New(),
			TenantID:  tenantID,
			Name:      in.Name,
			Email:     db.Nz(in.Email),
			Phone:     db.Nz(in.Phone),
			Address:   db.Nz(in.Address),
			Metadata:  db.Nz(metadata),
			CreatedAt: now,
			UpdatedAt: now,
		})
		if e != nil {
			return fmt.Errorf("insert: %w", e)
		}
		created = p
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "payer",
			EntityID:   p.ID,
			Action:     "create",
			Changes:    audit.Changes(map[string]any{"name": in.Name, "email": in.Email}),
		})
	})
	if err != nil {
		return nil, fmt.Errorf("create payer: %w", err)
	}
	return toPayer(created), nil
}

// Update writes the payer's fields and one audit row, atomically. The
// audit entry records the row's int PK, resolved by-uuid inside the tx. Returns
// (nil, nil) when the row does not exist so the caller can 404.
func (r *PayersRepo) Update(ctx context.Context, tenantID int64, uuid string, in PayerInput) (*Payer, error) {
	if in.Name == "" {
		return nil, errors.New("update payer: name is required")
	}
	metadata := in.Metadata
	if metadata == "" {
		metadata = "{}"
	}

	var updated gen.Payer
	var missing bool
	err := audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339)
		p, e := gen.New(tx).UpdatePayer(ctx, gen.UpdatePayerParams{
			Name:      in.Name,
			Email:     db.Nz(in.Email),
			Phone:     db.Nz(in.Phone),
			Address:   db.Nz(in.Address),
			Metadata:  db.Nz(metadata),
			UpdatedAt: now,
			TenantID:  tenantID,
			Uuid:      uuid,
		})
		if errors.Is(e, sql.ErrNoRows) {
			missing = true
			return e
		}
		if e != nil {
			return fmt.Errorf("update: %w", e)
		}
		updated = p
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "payer",
			EntityID:   p.ID,
			Action:     "update",
			Changes:    audit.Changes(map[string]any{"name": in.Name}),
		})
	})
	if missing {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("update payer: %w", err)
	}
	return toPayer(updated), nil
}

// Delete removes a payer by uuid and writes one audit row, atomically.
// The audit entry records the row's int PK, resolved by-uuid in the same tx.
func (r *PayersRepo) Delete(ctx context.Context, tenantID int64, uuid string) error {
	return audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		q := gen.New(tx)
		row, e := q.GetPayer(ctx, gen.GetPayerParams{TenantID: tenantID, Uuid: uuid})
		if errors.Is(e, sql.ErrNoRows) {
			return nil
		}
		if e != nil {
			return fmt.Errorf("lookup: %w", e)
		}
		if e := q.DeletePayer(ctx, gen.DeletePayerParams{TenantID: tenantID, Uuid: uuid}); e != nil {
			return fmt.Errorf("delete: %w", e)
		}
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "payer",
			EntityID:   row.ID,
			Action:     "delete",
		})
	})
}

// ResolvePayerIDs translates payer uuids into their int PKs
// (preserving order), tenant-scoped. An unknown uuid is an error so bulk ops can
// 400.
func (r *PayersRepo) ResolvePayerIDs(ctx context.Context, tenantID int64, pmUUIDs []string) ([]int64, error) {
	q := gen.New(r.db)
	out := make([]int64, 0, len(pmUUIDs))
	for i := range pmUUIDs { // bounded by len(pmUUIDs)
		id, err := q.GetPayerIDByUUID(ctx, gen.GetPayerIDByUUIDParams{TenantID: tenantID, Uuid: pmUUIDs[i]})
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("unknown payer %q", pmUUIDs[i])
		}
		if err != nil {
			return nil, fmt.Errorf("resolve payer uuid: %w", err)
		}
		out = append(out, id)
	}
	return out, nil
}

// BulkDelete removes several payers and writes one audit row, atomically.
// An empty id list is a no-op.
func (r *PayersRepo) BulkDelete(ctx context.Context, tenantID int64, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	return audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		q := gen.New(tx)
		for _, id := range ids { // bounded by len(ids)
			if err := q.DeletePayerByID(ctx, gen.DeletePayerByIDParams{TenantID: tenantID, ID: id}); err != nil {
				return fmt.Errorf("delete %d: %w", id, err)
			}
		}
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "payer",
			EntityID:   0,
			Action:     "bulk_delete",
			Changes:    audit.Changes(map[string]any{"ids": ids}),
		})
	})
}

// mapPayers converts a slice of generated rows to a non-nil domain slice.
func mapPayers(rows []gen.Payer) []*Payer {
	out := make([]*Payer, 0, len(rows))
	for i := range rows {
		out = append(out, toPayer(rows[i]))
	}
	return out
}

// toPayer maps a generated row to the domain Payer.
func toPayer(row gen.Payer) *Payer {
	return &Payer{
		ID:        row.ID,
		UUID:      row.Uuid,
		Name:      row.Name,
		Email:     row.Email.String,
		Phone:     row.Phone.String,
		Address:   row.Address.String,
		Metadata:  row.Metadata.String,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}
