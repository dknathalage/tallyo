package catalogue

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/dknathalage/tallyo/internal/apperr"
	"github.com/dknathalage/tallyo/internal/audit"
	"github.com/dknathalage/tallyo/internal/db"
	"github.com/dknathalage/tallyo/internal/db/gen"
	"github.com/dknathalage/tallyo/internal/ids"
	"github.com/dknathalage/tallyo/internal/listquery"
)

// catalogueListSelect mirrors ListCatalogue up to the WHERE. Keep in sync with
// internal/db/queries/catalogue.sql.
const catalogueListSelect = `SELECT * FROM catalogue_items WHERE tenant_id = ? AND is_current = 1`

// CatalogueCols is the listquery allowlist. Keys match JSON field names.
var CatalogueCols = listquery.Spec{
	"name":      {Col: "name", Filter: listquery.Text},
	"unitPrice": {Col: "unit_price", Filter: listquery.Number},
	"unit":      {Col: "unit", Filter: listquery.Text},
	"category":  {Col: "category", Filter: listquery.Text},
	"taxable":   {Col: "taxable", Filter: listquery.None},
}

// Repo reads and writes the catalogue_items table (tenant-scoped) with audited,
// copy-on-write mutations.
type Repo struct {
	db db.Executor
}

// NewRepo constructs a repository. A nil db is a programmer error.
func NewRepo(database db.Executor) *Repo {
	if database == nil {
		panic("catalogue: NewRepo requires a non-nil db")
	}
	return &Repo{db: database}
}

// List returns the tenant's current catalogue items ordered by name.
func (r *Repo) List(ctx context.Context, tenantID string) ([]*CatalogueItem, error) {
	rows, err := gen.New(r.db).ListCatalogue(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list catalogue: %w", err)
	}
	return mapCatalogueItems(rows), nil
}

// Search filters current catalogue rows whose code/name/category/unit match the
// term (LIKE), escaped.
func (r *Repo) Search(ctx context.Context, tenantID, q string) ([]*CatalogueItem, error) {
	like := "%" + escapeLike(q) + "%"
	rows, err := gen.New(r.db).SearchCatalogue(ctx, gen.SearchCatalogueParams{
		TenantID: tenantID,
		Code:     db.NzMaybe(like),
		Name:     like,
		Category: db.NzMaybe(like),
		Unit:     db.NzMaybe(like),
	})
	if err != nil {
		return nil, fmt.Errorf("search catalogue: %w", err)
	}
	return mapCatalogueItems(rows), nil
}

// Get returns a current catalogue item by uuid, or (nil, nil) when none matches.
func (r *Repo) Get(ctx context.Context, tenantID, uuid string) (*CatalogueItem, error) {
	row, err := gen.New(r.db).GetCatalogueItem(ctx, gen.GetCatalogueItemParams{TenantID: tenantID, ID: uuid})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get catalogue item: %w", err)
	}
	return toCatalogueItem(row), nil
}

// GetByID returns the exact version row by id (any is_current), or
// apperr.ErrNotFound. Used by the billing validator to price a line from its
// pinned catalogue version.
func (r *Repo) GetByID(ctx context.Context, tenantID, id string) (*CatalogueItem, error) {
	row, err := gen.New(r.db).GetCatalogueItem(ctx, gen.GetCatalogueItemParams{TenantID: tenantID, ID: id})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperr.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get catalogue item by id: %w", err)
	}
	return toCatalogueItem(row), nil
}

// GetCurrentByCode returns the current catalogue item for a code, or (nil, nil)
// when none matches (empty code never matches). Used by the smarts grounding to
// resolve a model-proposed code to a catalogue item.
func (r *Repo) GetCurrentByCode(ctx context.Context, tenantID, code string) (*CatalogueItem, error) {
	if code == "" {
		return nil, nil
	}
	row, err := gen.New(r.db).GetCurrentCatalogueByCode(ctx, gen.GetCurrentCatalogueByCodeParams{TenantID: tenantID, Code: nz(code)})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get catalogue by code: %w", err)
	}
	return toCatalogueItem(row), nil
}

// Create inserts a new catalogue item (new logical_id, version 1, current) and
// one audit row, atomically.
func (r *Repo) Create(ctx context.Context, tenantID string, in CatalogueItemInput) (*CatalogueItem, error) {
	if tenantID == "" {
		return nil, errors.New("create catalogue item: tenant id required")
	}
	var created gen.CatalogueItem
	err := audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		c, e := createVersion(ctx, gen.New(tx), tenantID, ids.New(), 1, in)
		if e != nil {
			return e
		}
		created = c
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "catalogue_item",
			EntityID:   c.ID,
			Action:     "create",
			Changes:    audit.Changes(map[string]any{"name": in.Name, "unitPrice": in.UnitPrice}),
		})
	})
	if err != nil {
		return nil, fmt.Errorf("create catalogue item: %w", err)
	}
	return toCatalogueItem(created), nil
}

// Update applies copy-on-write: if the current version row (id) is referenced by
// any invoice/estimate line it forks a new version (and freezes the old row);
// otherwise it mutates in place. Returns apperr.ErrNotFound when id is unknown.
func (r *Repo) Update(ctx context.Context, tenantID, id string, in CatalogueItemInput) (*CatalogueItem, error) {
	var result gen.CatalogueItem
	err := audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		c, e := updateCoW(ctx, gen.New(tx), tenantID, id, in)
		if e != nil {
			return e
		}
		result = c
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "catalogue_item",
			EntityID:   c.ID,
			Action:     "update",
			Changes:    audit.Changes(map[string]any{"name": in.Name}),
		})
	})
	if errors.Is(err, apperr.ErrNotFound) {
		return nil, apperr.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("update catalogue item: %w", err)
	}
	return toCatalogueItem(result), nil
}

// updateCoW runs the copy-on-write decision inside a tx. The current row is
// looked up by id; a referenced row forks, an unreferenced row updates in place.
func updateCoW(ctx context.Context, q *gen.Queries, tenantID, id string, in CatalogueItemInput) (gen.CatalogueItem, error) {
	cur, err := q.GetCatalogueItem(ctx, gen.GetCatalogueItemParams{TenantID: tenantID, ID: id})
	if errors.Is(err, sql.ErrNoRows) {
		return gen.CatalogueItem{}, apperr.ErrNotFound
	}
	if err != nil {
		return gen.CatalogueItem{}, fmt.Errorf("lookup: %w", err)
	}
	referenced, err := versionReferenced(ctx, q, id)
	if err != nil {
		return gen.CatalogueItem{}, err
	}
	if !referenced {
		updated, e := q.UpdateCatalogueItemInPlace(ctx, gen.UpdateCatalogueItemInPlaceParams{
			Code:      db.NzMaybe(in.Code),
			Name:      in.Name,
			Unit:      db.NzMaybe(in.Unit),
			Category:  db.NzMaybe(in.Category),
			UnitPrice: in.UnitPrice,
			Taxable:   db.B2i(in.Taxable),
			Metadata:  metadataOr(in.Metadata),
			UpdatedAt: now(),
			TenantID:  tenantID,
			ID:        id,
		})
		if e != nil {
			return gen.CatalogueItem{}, fmt.Errorf("update in place: %w", e)
		}
		return updated, nil
	}
	// Referenced: freeze the old row first (the partial unique index allows only
	// one is_current=1 per logical_id), then insert the new current version.
	maxV, err := maxVersion(ctx, q, tenantID, cur.LogicalID)
	if err != nil {
		return gen.CatalogueItem{}, err
	}
	if e := q.MarkCatalogueVersionStale(ctx, gen.MarkCatalogueVersionStaleParams{TenantID: tenantID, ID: id}); e != nil {
		return gen.CatalogueItem{}, fmt.Errorf("freeze old version: %w", e)
	}
	forked, err := createVersionWithLogical(ctx, q, tenantID, ids.New(), cur.LogicalID, maxV+1, in)
	if err != nil {
		return gen.CatalogueItem{}, err
	}
	return forked, nil
}

// Delete tombstones the item (is_current = 0 for the whole logical_id), resolved
// from the current version-row uuid. Unknown uuid -> apperr.ErrNotFound.
func (r *Repo) Delete(ctx context.Context, tenantID, uuid string) error {
	return audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		q := gen.New(tx)
		logicalID, e := q.GetCatalogueLogicalIDByUUID(ctx, gen.GetCatalogueLogicalIDByUUIDParams{TenantID: tenantID, ID: uuid})
		if errors.Is(e, sql.ErrNoRows) {
			return apperr.ErrNotFound
		}
		if e != nil {
			return fmt.Errorf("lookup: %w", e)
		}
		if e := q.TombstoneCatalogueLogical(ctx, gen.TombstoneCatalogueLogicalParams{TenantID: tenantID, LogicalID: logicalID}); e != nil {
			return fmt.Errorf("tombstone: %w", e)
		}
		return audit.Log(ctx, tx, audit.Entry{EntityType: "catalogue_item", EntityID: uuid, Action: "delete"})
	})
}

// ResolveCatalogueLogicalIDs resolves current version-row uuids to their
// logical_ids (order preserved), tenant-scoped. An unknown uuid is an error so
// bulk ops can 400.
func (r *Repo) ResolveCatalogueLogicalIDs(ctx context.Context, tenantID string, uuids []string) ([]string, error) {
	q := gen.New(r.db)
	out := make([]string, 0, len(uuids))
	for i := range uuids { // bounded by len(uuids)
		lid, err := q.GetCatalogueLogicalIDByUUID(ctx, gen.GetCatalogueLogicalIDByUUIDParams{TenantID: tenantID, ID: uuids[i]})
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("unknown catalogue item %q", uuids[i])
		}
		if err != nil {
			return nil, fmt.Errorf("resolve catalogue uuid: %w", err)
		}
		out = append(out, lid)
	}
	return out, nil
}

// BulkDelete tombstones several items by logical_id and writes one audit row.
func (r *Repo) BulkDelete(ctx context.Context, tenantID string, logicalIDs []string) error {
	if len(logicalIDs) == 0 {
		return nil
	}
	return audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		q := gen.New(tx)
		for _, lid := range logicalIDs { // bounded by len(logicalIDs)
			if err := q.TombstoneCatalogueLogical(ctx, gen.TombstoneCatalogueLogicalParams{TenantID: tenantID, LogicalID: lid}); err != nil {
				return fmt.Errorf("tombstone %s: %w", lid, err)
			}
		}
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "catalogue_item",
			EntityID:   "",
			Action:     "bulk_delete",
			Changes:    audit.Changes(map[string]any{"logicalIds": logicalIDs}),
		})
	})
}

// --- shared helpers ---

// createVersion inserts version 1 of a brand-new item (fresh logical_id).
func createVersion(ctx context.Context, q *gen.Queries, tenantID, id string, version int64, in CatalogueItemInput) (gen.CatalogueItem, error) {
	return createVersionWithLogical(ctx, q, tenantID, id, id, version, in)
}

// createVersionWithLogical inserts a current version row under logicalID.
func createVersionWithLogical(ctx context.Context, q *gen.Queries, tenantID, id, logicalID string, version int64, in CatalogueItemInput) (gen.CatalogueItem, error) {
	ts := now()
	c, err := q.CreateCatalogueItem(ctx, gen.CreateCatalogueItemParams{
		ID:        id,
		LogicalID: logicalID,
		TenantID:  tenantID,
		Code:      db.NzMaybe(in.Code),
		Name:      in.Name,
		Unit:      db.NzMaybe(in.Unit),
		Category:  db.NzMaybe(in.Category),
		UnitPrice: in.UnitPrice,
		Taxable:   db.B2i(in.Taxable),
		Metadata:  metadataOr(in.Metadata),
		Version:   version,
		IsCurrent: 1,
		CreatedAt: ts,
		UpdatedAt: ts,
	})
	if err != nil {
		return gen.CatalogueItem{}, fmt.Errorf("insert version: %w", err)
	}
	return c, nil
}

// versionReferenced reports whether a version row is referenced by any
// invoice/estimate line (the copy-on-write fork trigger).
func versionReferenced(ctx context.Context, q *gen.Queries, id string) (bool, error) {
	ref := db.NzMaybe(id)
	li, err := q.LineItemReferencesCatalogue(ctx, ref)
	if err != nil {
		return false, fmt.Errorf("check line refs: %w", err)
	}
	if li {
		return true, nil
	}
	eli, err := q.EstimateLineReferencesCatalogue(ctx, ref)
	if err != nil {
		return false, fmt.Errorf("check estimate-line refs: %w", err)
	}
	return eli, nil
}

// maxVersion returns the highest version number for a logical_id (0 when none).
func maxVersion(ctx context.Context, q *gen.Queries, tenantID, logicalID string) (int64, error) {
	v, err := q.MaxCatalogueVersionForLogical(ctx, gen.MaxCatalogueVersionForLogicalParams{TenantID: tenantID, LogicalID: logicalID})
	if err != nil {
		return 0, fmt.Errorf("max version: %w", err)
	}
	switch n := v.(type) {
	case int64:
		return n, nil
	case nil:
		return 0, nil
	default:
		return 0, fmt.Errorf("max version: unexpected type %T", v)
	}
}

func metadataOr(s string) string {
	if s == "" {
		return "{}"
	}
	return s
}

func now() string { return time.Now().UTC().Format(time.RFC3339) }

func mapCatalogueItems(rows []gen.CatalogueItem) []*CatalogueItem {
	out := make([]*CatalogueItem, 0, len(rows))
	for i := range rows { // bounded by len(rows)
		out = append(out, toCatalogueItem(rows[i]))
	}
	return out
}

func toCatalogueItem(row gen.CatalogueItem) *CatalogueItem {
	return &CatalogueItem{
		ID:        row.ID,
		LogicalID: row.LogicalID,
		Code:      row.Code.String,
		Name:      row.Name,
		Unit:      row.Unit.String,
		Category:  row.Category.String,
		UnitPrice: row.UnitPrice,
		Taxable:   row.Taxable == 1,
		Metadata:  row.Metadata,
		Version:   row.Version,
		IsCurrent: row.IsCurrent == 1,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}
