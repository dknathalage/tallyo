package customitem

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

// customItemListSelect mirrors the ListCustomItems sqlc query body up to the
// WHERE. Keep in sync with internal/db/queries/custom_items.sql.
const customItemListSelect = `SELECT * FROM custom_items WHERE tenant_id = ?`

// CustomItemCols is the listquery allowlist for custom items. Keys match the
// JSON field names so the frontend column key drives filter, sort, display, and
// drawer-edit with one identifier.
var CustomItemCols = listquery.Spec{
	"name":    {Col: "name", Filter: listquery.Text},
	"rate":    {Col: "rate", Filter: listquery.Number},
	"unit":    {Col: "unit", Filter: listquery.Text},
	"taxable": {Col: "taxable", Filter: listquery.None},
}

// CustomItem is the domain view of a row in the custom_items table.
type CustomItem struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Rate      float64 `json:"rate"`
	Unit      string  `json:"unit"`
	Taxable   bool    `json:"taxable"`
	Metadata  string  `json:"metadata"`
	CreatedAt string  `json:"createdAt"`
	UpdatedAt string  `json:"updatedAt"`
}

// CustomItemInput is the writable subset of a custom item.
type CustomItemInput struct {
	Name     string  `json:"name"`
	Rate     float64 `json:"rate"`
	Unit     string  `json:"unit"`
	Taxable  bool    `json:"taxable"`
	Metadata string  `json:"metadata"`
}

// Repo reads and writes the custom_items table (tenant-scoped) with
// audited mutations.
type Repo struct {
	db db.Executor
}

// NewRepo constructs a repository. A nil db is a programmer error.
func NewRepo(db db.Executor) *Repo {
	if db == nil {
		panic("customitem: NewRepo requires a non-nil *sql.DB")
	}
	return &Repo{db: db}
}

// List returns the tenant's custom items ordered by name.
func (r *Repo) List(ctx context.Context, tenantID string) ([]*CustomItem, error) {
	rows, err := gen.New(r.db).ListCustomItems(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list custom items: %w", err)
	}
	return mapCustomItems(rows), nil
}

// Query returns one page of custom items plus the total row count for the
// filter (ignoring pagination). The clause is built by listquery from an
// allowlisted spec, so its Where/Order fragments are injection-safe.
func (r *Repo) Query(ctx context.Context, tenantID string, c listquery.Clause) ([]*CustomItem, int64, error) {
	if tenantID == "" {
		return nil, 0, errors.New("query custom items: tenant id required")
	}
	var total int64
	countSQL := "SELECT count(*) FROM (" + customItemListSelect + c.Where + ")"
	countArgs := append([]any{tenantID}, c.CountArgs()...)
	if err := r.db.QueryRowContext(ctx, countSQL, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count custom items: %w", err)
	}
	order := c.Order
	if order == "" {
		order = " ORDER BY name"
	}
	sqlText := customItemListSelect + c.Where + order + c.Limit
	pageArgs := append([]any{tenantID}, c.Args...)
	rows, err := r.db.QueryContext(ctx, sqlText, pageArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("query custom items: %w", err)
	}
	defer rows.Close()
	out := make([]*CustomItem, 0, 50)
	for rows.Next() { // bounded by LIMIT in the query
		var (
			id        string
			tenant    string
			name      string
			rate      float64
			unit      sql.NullString
			taxable   int64
			metadata  sql.NullString
			createdAt string
			updatedAt string
		)
		if err := rows.Scan(&id, &tenant, &name, &rate, &unit,
			&taxable, &metadata, &createdAt, &updatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan custom item: %w", err)
		}
		out = append(out, &CustomItem{
			ID:        id,
			Name:      name,
			Rate:      rate,
			Unit:      unit.String,
			Taxable:   taxable == 1,
			Metadata:  metadata.String,
			CreatedAt: createdAt,
			UpdatedAt: updatedAt,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("query custom items: %w", err)
	}
	return out, total, nil
}

// Search filters the tenant's custom items whose name matches the term (LIKE).
func (r *Repo) Search(ctx context.Context, tenantID string, q string) ([]*CustomItem, error) {
	like := "%" + q + "%"
	rows, err := gen.New(r.db).SearchCustomItems(ctx, gen.SearchCustomItemsParams{
		TenantID: tenantID,
		Name:     like,
	})
	if err != nil {
		return nil, fmt.Errorf("search custom items: %w", err)
	}
	return mapCustomItems(rows), nil
}

// Get returns the tenant's custom item by uuid, or (nil, nil) when none matches.
func (r *Repo) Get(ctx context.Context, tenantID string, uuid string) (*CustomItem, error) {
	row, err := gen.New(r.db).GetCustomItem(ctx, gen.GetCustomItemParams{TenantID: tenantID, ID: uuid})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get custom item: %w", err)
	}
	return toCustomItem(row), nil
}

// Create inserts a custom item and writes one audit row, atomically.
func (r *Repo) Create(ctx context.Context, tenantID string, in CustomItemInput) (*CustomItem, error) {
	if tenantID == "" {
		return nil, errors.New("create custom item: tenant id required")
	}
	if in.Name == "" {
		return nil, errors.New("create custom item: name is required")
	}
	metadata := in.Metadata
	if metadata == "" {
		metadata = "{}"
	}

	var created gen.CustomItem
	err := audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339)
		c, e := gen.New(tx).CreateCustomItem(ctx, gen.CreateCustomItemParams{
			ID:        ids.New(),
			TenantID:  tenantID,
			Name:      in.Name,
			Rate:      in.Rate,
			Unit:      db.NzMaybe(in.Unit),
			Taxable:   db.B2i(in.Taxable),
			Metadata:  db.Nz(metadata),
			CreatedAt: now,
			UpdatedAt: now,
		})
		if e != nil {
			return fmt.Errorf("insert: %w", e)
		}
		created = c
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "custom_item",
			EntityID:   c.ID,
			Action:     "create",
			Changes:    audit.Changes(map[string]any{"name": in.Name, "rate": in.Rate}),
		})
	})
	if err != nil {
		return nil, fmt.Errorf("create custom item: %w", err)
	}
	return toCustomItem(created), nil
}

// Update writes the custom item's fields and one audit row, atomically. The
// audit entry records the row's id, looked up by-uuid inside the tx. Returns
// (nil, nil) when the item does not exist so the caller can 404.
func (r *Repo) Update(ctx context.Context, tenantID string, uuid string, in CustomItemInput) (*CustomItem, error) {
	if in.Name == "" {
		return nil, errors.New("update custom item: name is required")
	}
	metadata := in.Metadata
	if metadata == "" {
		metadata = "{}"
	}

	var updated gen.CustomItem
	var missing bool
	err := audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339)
		c, e := gen.New(tx).UpdateCustomItem(ctx, gen.UpdateCustomItemParams{
			Name:      in.Name,
			Rate:      in.Rate,
			Unit:      db.NzMaybe(in.Unit),
			Taxable:   db.B2i(in.Taxable),
			Metadata:  db.Nz(metadata),
			UpdatedAt: now,
			TenantID:  tenantID,
			ID:        uuid,
		})
		if errors.Is(e, sql.ErrNoRows) {
			missing = true
			return e
		}
		if e != nil {
			return fmt.Errorf("update: %w", e)
		}
		updated = c
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "custom_item",
			EntityID:   c.ID,
			Action:     "update",
			Changes:    audit.Changes(map[string]any{"name": in.Name}),
		})
	})
	if missing {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("update custom item: %w", err)
	}
	return toCustomItem(updated), nil
}

// Delete removes a custom item by uuid and writes one audit row, atomically. The
// audit entry records the row's id, looked up by-uuid in the same tx.
func (r *Repo) Delete(ctx context.Context, tenantID string, uuid string) error {
	return audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		q := gen.New(tx)
		row, e := q.GetCustomItem(ctx, gen.GetCustomItemParams{TenantID: tenantID, ID: uuid})
		if errors.Is(e, sql.ErrNoRows) {
			return nil
		}
		if e != nil {
			return fmt.Errorf("lookup: %w", e)
		}
		if e := q.DeleteCustomItem(ctx, gen.DeleteCustomItemParams{TenantID: tenantID, ID: uuid}); e != nil {
			return fmt.Errorf("delete: %w", e)
		}
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "custom_item",
			EntityID:   row.ID,
			Action:     "delete",
		})
	})
}

// ResolveCustomItemIDs resolves custom-item uuids to their row ids (uuid)
// (preserving order), tenant-scoped. An unknown uuid is an error so bulk ops can
// 400.
func (r *Repo) ResolveCustomItemIDs(ctx context.Context, tenantID string, itemUUIDs []string) ([]string, error) {
	q := gen.New(r.db)
	out := make([]string, 0, len(itemUUIDs))
	for i := range itemUUIDs { // bounded by len(itemUUIDs)
		id, err := q.GetCustomItemIDByUUID(ctx, gen.GetCustomItemIDByUUIDParams{TenantID: tenantID, ID: itemUUIDs[i]})
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("unknown custom item %q", itemUUIDs[i])
		}
		if err != nil {
			return nil, fmt.Errorf("resolve custom item uuid: %w", err)
		}
		out = append(out, id)
	}
	return out, nil
}

// BulkDelete removes several custom items and writes one audit row, atomically.
// An empty id list is a no-op.
func (r *Repo) BulkDelete(ctx context.Context, tenantID string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	return audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		q := gen.New(tx)
		for _, id := range ids { // bounded by len(ids)
			if err := q.DeleteCustomItemByID(ctx, gen.DeleteCustomItemByIDParams{TenantID: tenantID, ID: id}); err != nil {
				return fmt.Errorf("delete %s: %w", id, err)
			}
		}
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "custom_item",
			EntityID:   "",
			Action:     "bulk_delete",
			Changes:    audit.Changes(map[string]any{"ids": ids}),
		})
	})
}

func mapCustomItems(rows []gen.CustomItem) []*CustomItem {
	out := make([]*CustomItem, 0, len(rows))
	for i := range rows { // bounded by len(rows)
		out = append(out, toCustomItem(rows[i]))
	}
	return out
}

func toCustomItem(row gen.CustomItem) *CustomItem {
	return &CustomItem{
		ID:        row.ID,
		Name:      row.Name,
		Rate:      row.Rate,
		Unit:      row.Unit.String,
		Taxable:   row.Taxable == 1,
		Metadata:  row.Metadata.String,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}
