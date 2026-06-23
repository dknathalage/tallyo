// Package client is the client vertical slice: domain types, the audited
// repository over the clients table, the service (with SSE broadcast), and the
// HTTP handler. It depends only on platform packages (db/gen, audit, reqctx,
// realtime, httpx), never on other domain slices.
package client

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/dknathalage/tallyo/internal/db"

	"github.com/dknathalage/tallyo/internal/audit"
	"github.com/dknathalage/tallyo/internal/db/gen"
	"github.com/dknathalage/tallyo/internal/listquery"
	"github.com/google/uuid"
)

// clientListSelect mirrors the ListClients sqlc query body up to the WHERE.
// Keep in sync with internal/db/queries/clients.sql.
const clientListSelect = `SELECT p.*, pm.name AS plan_manager_name, pm.uuid AS plan_manager_uuid
FROM clients p
LEFT JOIN plan_managers pm ON p.plan_manager_id = pm.id AND pm.tenant_id = p.tenant_id
WHERE p.tenant_id = ?`

// ClientCols is the listquery allowlist for clients. Keys match the JSON field
// names so the frontend column key drives filter, sort, display, and
// drawer-edit with one identifier.
var ClientCols = listquery.Spec{
	"name":            {Col: "p.name", Filter: listquery.Text},
	"type":            {Col: "p.type", Filter: listquery.Enum},
	"reference":       {Col: "p.reference", Filter: listquery.Text},
	"email":           {Col: "p.email", Filter: listquery.Text},
	"mgmtType":        {Col: "p.mgmt_type", Filter: listquery.Enum},
	"planStart":       {Col: "p.plan_start", Filter: listquery.Date},
	"planEnd":         {Col: "p.plan_end", Filter: listquery.Date},
	"planManagerName": {Col: "pm.name", Filter: listquery.Text},
}

// errPlanManagerNotFound is the sentinel returned when an inbound plan-manager
// uuid does not resolve to a row in the tenant. Handlers map it to a 400.
var errPlanManagerNotFound = errors.New("plan manager not found")

// Client is the domain view of a row in the clients table. Nullable columns are
// unwrapped to plain strings (""). The public identifier is the uuid (json
// "id"); the int PK is internal-only. The plan-manager FK is exposed as the
// related plan-manager uuid (nil when self-managed), resolved via LEFT JOIN,
// never the int FK.
type Client struct {
	ID              int64   `json:"-"`
	UUID            string  `json:"id"`
	Name            string  `json:"name"`
	Type            string  `json:"type"`
	Reference       string  `json:"reference"`
	PlanStart       string  `json:"planStart"`
	PlanEnd         string  `json:"planEnd"`
	MgmtType        string  `json:"mgmtType"`
	PlanManagerUUID *string `json:"planManagerId"`
	PlanManagerName string  `json:"planManagerName"`
	Email           string  `json:"email"`
	Phone           string  `json:"phone"`
	Address         string  `json:"address"`
	Metadata        string  `json:"metadata"`
	CreatedAt       string  `json:"createdAt"`
	UpdatedAt       string  `json:"updatedAt"`
}

// ClientInput is the writable subset of a client. PlanManagerUUID is the
// plan-manager's uuid (nil/empty → self-managed); it is resolved to the int FK
// before insert/update.
type ClientInput struct {
	Name            string  `json:"name"`
	Type            string  `json:"type"`
	Reference       string  `json:"reference"`
	PlanStart       string  `json:"planStart"`
	PlanEnd         string  `json:"planEnd"`
	MgmtType        string  `json:"mgmtType"`
	PlanManagerUUID *string `json:"planManagerId"`
	Email           string  `json:"email"`
	Phone           string  `json:"phone"`
	Address         string  `json:"address"`
	Metadata        string  `json:"metadata"`
}

// ClientsRepo reads and writes the clients table (tenant-scoped) with audited
// mutations.
type ClientsRepo struct {
	db db.Executor
}

// NewClients constructs a repository. A nil db is a programmer error.
func NewClients(db db.Executor) *ClientsRepo {
	if db == nil {
		panic("client: NewClients requires a non-nil *sql.DB")
	}
	return &ClientsRepo{db: db}
}

// List returns the tenant's clients ordered by name. When search is non-empty
// it filters to name, email, or reference matches (LIKE).
func (r *ClientsRepo) List(ctx context.Context, tenantID int64, search string) ([]*Client, error) {
	q := gen.New(r.db)
	if search == "" {
		rows, err := q.ListClients(ctx, tenantID)
		if err != nil {
			return nil, fmt.Errorf("list clients: %w", err)
		}
		out := make([]*Client, 0, len(rows))
		for i := range rows {
			out = append(out, toClientList(rows[i]))
		}
		return out, nil
	}
	like := "%" + search + "%"
	rows, err := q.SearchClients(ctx, gen.SearchClientsParams{
		TenantID:  tenantID,
		Name:      like,
		Email:     db.Nz(like),
		Reference: db.Nz(like),
	})
	if err != nil {
		return nil, fmt.Errorf("search clients: %w", err)
	}
	out := make([]*Client, 0, len(rows))
	for i := range rows {
		out = append(out, toClientSearch(rows[i]))
	}
	return out, nil
}

// Query returns one page of clients plus the total row count for the filter
// (ignoring pagination). The clause is built by listquery from an allowlisted
// spec, so its Where/Order fragments are injection-safe.
func (r *ClientsRepo) Query(ctx context.Context, tenantID int64, c listquery.Clause) ([]*Client, int64, error) {
	if tenantID == 0 {
		return nil, 0, errors.New("query clients: tenant id required")
	}
	var total int64
	countSQL := "SELECT count(*) FROM (" + clientListSelect + c.Where + ")"
	countArgs := append([]any{tenantID}, c.CountArgs()...)
	if err := r.db.QueryRowContext(ctx, countSQL, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count clients: %w", err)
	}
	sqlText := clientListSelect + c.Where + c.Order + c.Limit
	pageArgs := append([]any{tenantID}, c.Args...)
	rows, err := r.db.QueryContext(ctx, sqlText, pageArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("query clients: %w", err)
	}
	defer rows.Close()
	out := make([]*Client, 0, 50)
	for rows.Next() { // bounded by LIMIT in the query
		var f clientFields
		var tenant int64
		var planManagerID sql.NullInt64 // internal FK column; never surfaced
		if err := rows.Scan(&f.id, &f.uuid, &tenant, &f.name, &f.typ, &f.reference, &f.planStart,
			&f.planEnd, &f.mgmtType, &planManagerID, &f.email, &f.phone, &f.address,
			&f.metadata, &f.createdAt, &f.updatedAt, &f.planManagerName, &f.planManagerUUID); err != nil {
			return nil, 0, fmt.Errorf("scan client: %w", err)
		}
		out = append(out, mapClientFields(f))
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("query clients: %w", err)
	}
	return out, total, nil
}

// Get returns the tenant's client by uuid with resolved plan-manager name +
// uuid, or (nil, nil) when absent.
func (r *ClientsRepo) Get(ctx context.Context, tenantID int64, uuid string) (*Client, error) {
	row, err := gen.New(r.db).GetClient(ctx, gen.GetClientParams{TenantID: tenantID, Uuid: uuid})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get client: %w", err)
	}
	return toClientGet(row), nil
}

// GetByID returns the tenant's client by int PK, for internal cross-slice reads
// (e.g. billing's plan-window lookup) that already hold the FK. The public API
// addresses clients by uuid via Get.
func (r *ClientsRepo) GetByID(ctx context.Context, tenantID, id int64) (*Client, error) {
	row, err := gen.New(r.db).GetClientByID(ctx, gen.GetClientByIDParams{TenantID: tenantID, ID: id})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get client by id: %w", err)
	}
	return toClientGetByID(row), nil
}

// resolvePlanManager translates an inbound plan-manager uuid into the int FK for
// insert/update. A nil/empty uuid → NULL FK. An unknown uuid (foreign or absent)
// → errPlanManagerNotFound so the handler can 400.
func (r *ClientsRepo) resolvePlanManager(ctx context.Context, q *gen.Queries, tenantID int64, pmUUID *string) (sql.NullInt64, error) {
	if pmUUID == nil || *pmUUID == "" {
		return sql.NullInt64{}, nil
	}
	id, err := q.GetPlanManagerIDByUUID(ctx, gen.GetPlanManagerIDByUUIDParams{TenantID: tenantID, Uuid: *pmUUID})
	if errors.Is(err, sql.ErrNoRows) {
		return sql.NullInt64{}, errPlanManagerNotFound
	}
	if err != nil {
		return sql.NullInt64{}, fmt.Errorf("resolve plan manager: %w", err)
	}
	return sql.NullInt64{Int64: id, Valid: true}, nil
}

// normType maps an inbound client type to a stored enum value, defaulting to
// 'standard' when empty.
func normType(t string) string {
	if t == "" {
		return "standard"
	}
	return t
}

// Create inserts a client and writes one audit row, atomically, then re-reads
// the row so the returned Client carries the plan-manager name.
func (r *ClientsRepo) Create(ctx context.Context, tenantID int64, in ClientInput) (*Client, error) {
	if tenantID == 0 {
		return nil, errors.New("create client: tenant id required")
	}
	if in.Name == "" {
		return nil, errors.New("create client: name is required")
	}
	metadata := in.Metadata
	if metadata == "" {
		metadata = "{}"
	}

	var newUUID string
	err := audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		q := gen.New(tx)
		pmID, e := r.resolvePlanManager(ctx, q, tenantID, in.PlanManagerUUID)
		if e != nil {
			return e
		}
		now := time.Now().UTC().Format(time.RFC3339)
		c, e := q.CreateClient(ctx, gen.CreateClientParams{
			Uuid:          uuid.NewString(),
			TenantID:      tenantID,
			Name:          in.Name,
			Type:          normType(in.Type),
			Reference:     db.NzMaybe(in.Reference),
			PlanStart:     db.NzMaybe(in.PlanStart),
			PlanEnd:       db.NzMaybe(in.PlanEnd),
			MgmtType:      db.NzMaybe(in.MgmtType),
			PlanManagerID: pmID,
			Email:         db.NzMaybe(in.Email),
			Phone:         db.NzMaybe(in.Phone),
			Address:       db.NzMaybe(in.Address),
			Metadata:      db.Nz(metadata),
			CreatedAt:     now,
			UpdatedAt:     now,
		})
		if e != nil {
			return fmt.Errorf("insert: %w", e)
		}
		newUUID = c.Uuid
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "client",
			EntityID:   c.ID,
			Action:     "create",
			Changes:    audit.Changes(map[string]any{"name": in.Name}),
		})
	})
	if errors.Is(err, errPlanManagerNotFound) {
		return nil, errPlanManagerNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("create client: %w", err)
	}
	return r.Get(ctx, tenantID, newUUID)
}

// Update writes the client's fields and one audit row, atomically, then
// re-reads. Returns (nil, nil) when the client does not exist. The audit entry
// records the row's int PK, resolved by-uuid in the same tx.
func (r *ClientsRepo) Update(ctx context.Context, tenantID int64, uuid string, in ClientInput) (*Client, error) {
	if in.Name == "" {
		return nil, errors.New("update client: name is required")
	}
	metadata := in.Metadata
	if metadata == "" {
		metadata = "{}"
	}

	var missing bool
	err := audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		q := gen.New(tx)
		pmID, e := r.resolvePlanManager(ctx, q, tenantID, in.PlanManagerUUID)
		if e != nil {
			return e
		}
		now := time.Now().UTC().Format(time.RFC3339)
		row, e := q.UpdateClient(ctx, gen.UpdateClientParams{
			Name:          in.Name,
			Type:          normType(in.Type),
			Reference:     db.NzMaybe(in.Reference),
			PlanStart:     db.NzMaybe(in.PlanStart),
			PlanEnd:       db.NzMaybe(in.PlanEnd),
			MgmtType:      db.NzMaybe(in.MgmtType),
			PlanManagerID: pmID,
			Email:         db.NzMaybe(in.Email),
			Phone:         db.NzMaybe(in.Phone),
			Address:       db.NzMaybe(in.Address),
			Metadata:      db.Nz(metadata),
			UpdatedAt:     now,
			TenantID:      tenantID,
			Uuid:          uuid,
		})
		if errors.Is(e, sql.ErrNoRows) {
			missing = true
			return e
		}
		if e != nil {
			return fmt.Errorf("update: %w", e)
		}
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "client",
			EntityID:   row.ID,
			Action:     "update",
			Changes:    audit.Changes(map[string]any{"name": in.Name}),
		})
	})
	if errors.Is(err, errPlanManagerNotFound) {
		return nil, errPlanManagerNotFound
	}
	if missing {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("update client: %w", err)
	}
	return r.Get(ctx, tenantID, uuid)
}

// Delete removes a client by uuid and writes one audit row, atomically. The
// audit entry records the row's int PK, resolved by-uuid in the same tx. A
// missing row is a silent no-op.
func (r *ClientsRepo) Delete(ctx context.Context, tenantID int64, uuid string) error {
	return audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		q := gen.New(tx)
		row, e := q.GetClient(ctx, gen.GetClientParams{TenantID: tenantID, Uuid: uuid})
		if errors.Is(e, sql.ErrNoRows) {
			return nil
		}
		if e != nil {
			return fmt.Errorf("lookup: %w", e)
		}
		if e := q.DeleteClient(ctx, gen.DeleteClientParams{TenantID: tenantID, Uuid: uuid}); e != nil {
			return fmt.Errorf("delete: %w", e)
		}
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "client",
			EntityID:   row.ID,
			Action:     "delete",
		})
	})
}

// ResolveClientIDs translates client uuids into their int PKs (preserving
// order), tenant-scoped. An unknown uuid is an error so bulk ops can 400.
func (r *ClientsRepo) ResolveClientIDs(ctx context.Context, tenantID int64, clientUUIDs []string) ([]int64, error) {
	q := gen.New(r.db)
	out := make([]int64, 0, len(clientUUIDs))
	for i := range clientUUIDs { // bounded by len(clientUUIDs)
		id, err := q.GetClientIDByUUID(ctx, gen.GetClientIDByUUIDParams{TenantID: tenantID, Uuid: clientUUIDs[i]})
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("unknown client %q", clientUUIDs[i])
		}
		if err != nil {
			return nil, fmt.Errorf("resolve client uuid: %w", err)
		}
		out = append(out, id)
	}
	return out, nil
}

// BulkDelete removes several clients and writes one audit row, atomically. An
// empty id list is a no-op.
func (r *ClientsRepo) BulkDelete(ctx context.Context, tenantID int64, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	return audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		q := gen.New(tx)
		for _, id := range ids { // bounded by len(ids)
			if err := q.DeleteClientByID(ctx, gen.DeleteClientByIDParams{TenantID: tenantID, ID: id}); err != nil {
				return fmt.Errorf("delete %d: %w", id, err)
			}
		}
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "client",
			EntityID:   0,
			Action:     "bulk_delete",
			Changes:    audit.Changes(map[string]any{"ids": ids}),
		})
	})
}

// clientFields is the shared, flat shape of every clients join row (List,
// Search and Get produce identical structs under distinct gen names).
type clientFields struct {
	id                                      int64
	uuid, name, typ                         string
	reference, planStart, planEnd, mgmtType sql.NullString
	email, phone, address, metadata         sql.NullString
	createdAt, updatedAt                    string
	planManagerName, planManagerUUID        sql.NullString
}

// mapClientFields builds a domain Client from the unwrapped columns. The int
// plan_manager_id is never surfaced; the joined plan-manager uuid is.
func mapClientFields(f clientFields) *Client {
	return &Client{
		ID:              f.id,
		UUID:            f.uuid,
		Name:            f.name,
		Type:            f.typ,
		Reference:       f.reference.String,
		PlanStart:       f.planStart.String,
		PlanEnd:         f.planEnd.String,
		MgmtType:        f.mgmtType.String,
		PlanManagerUUID: db.PtrStr(f.planManagerUUID),
		PlanManagerName: f.planManagerName.String,
		Email:           f.email.String,
		Phone:           f.phone.String,
		Address:         f.address.String,
		Metadata:        f.metadata.String,
		CreatedAt:       f.createdAt,
		UpdatedAt:       f.updatedAt,
	}
}

func toClientList(r gen.ListClientsRow) *Client {
	return mapClientFields(clientFields{
		id: r.ID, uuid: r.Uuid, name: r.Name, typ: r.Type,
		reference: r.Reference, planStart: r.PlanStart, planEnd: r.PlanEnd, mgmtType: r.MgmtType,
		email: r.Email, phone: r.Phone, address: r.Address, metadata: r.Metadata,
		createdAt: r.CreatedAt, updatedAt: r.UpdatedAt,
		planManagerName: r.PlanManagerName, planManagerUUID: r.PlanManagerUuid,
	})
}

func toClientSearch(r gen.SearchClientsRow) *Client {
	return mapClientFields(clientFields{
		id: r.ID, uuid: r.Uuid, name: r.Name, typ: r.Type,
		reference: r.Reference, planStart: r.PlanStart, planEnd: r.PlanEnd, mgmtType: r.MgmtType,
		email: r.Email, phone: r.Phone, address: r.Address, metadata: r.Metadata,
		createdAt: r.CreatedAt, updatedAt: r.UpdatedAt,
		planManagerName: r.PlanManagerName, planManagerUUID: r.PlanManagerUuid,
	})
}

func toClientGet(r gen.GetClientRow) *Client {
	return mapClientFields(clientFields{
		id: r.ID, uuid: r.Uuid, name: r.Name, typ: r.Type,
		reference: r.Reference, planStart: r.PlanStart, planEnd: r.PlanEnd, mgmtType: r.MgmtType,
		email: r.Email, phone: r.Phone, address: r.Address, metadata: r.Metadata,
		createdAt: r.CreatedAt, updatedAt: r.UpdatedAt,
		planManagerName: r.PlanManagerName, planManagerUUID: r.PlanManagerUuid,
	})
}

func toClientGetByID(r gen.GetClientByIDRow) *Client {
	return mapClientFields(clientFields{
		id: r.ID, uuid: r.Uuid, name: r.Name, typ: r.Type,
		reference: r.Reference, planStart: r.PlanStart, planEnd: r.PlanEnd, mgmtType: r.MgmtType,
		email: r.Email, phone: r.Phone, address: r.Address, metadata: r.Metadata,
		createdAt: r.CreatedAt, updatedAt: r.UpdatedAt,
		planManagerName: r.PlanManagerName, planManagerUUID: r.PlanManagerUuid,
	})
}
