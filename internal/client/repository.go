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

	"github.com/dknathalage/tallyo/internal/apperr"
	"github.com/dknathalage/tallyo/internal/audit"
	"github.com/dknathalage/tallyo/internal/db/gen"
	"github.com/dknathalage/tallyo/internal/ids"
	"github.com/dknathalage/tallyo/internal/listquery"
)

// clientListSelect mirrors the ListClients sqlc query body up to the WHERE.
// Keep in sync with internal/db/queries/clients.sql.
const clientListSelect = `SELECT p.*, pm.name AS payer_name, pm.id AS payer_uuid
FROM clients p
LEFT JOIN payers pm ON p.payer_id = pm.id AND pm.tenant_id = p.tenant_id
WHERE p.tenant_id = ?`

// ClientCols is the listquery allowlist for clients. Keys match the JSON field
// names so the frontend column key drives filter, sort, display, and
// drawer-edit with one identifier.
var ClientCols = listquery.Spec{
	"name":      {Col: "p.name", Filter: listquery.Text},
	"reference": {Col: "p.reference", Filter: listquery.Text},
	"email":     {Col: "p.email", Filter: listquery.Text},
	"payerName": {Col: "pm.name", Filter: listquery.Text},
}

// errPayerNotFound is the sentinel returned when an inbound payer
// uuid does not resolve to a row in the tenant. Handlers map it to a 400.
var errPayerNotFound = errors.New("payer not found")

// Client is the domain view of a row in the clients table. Nullable columns are
// unwrapped to plain strings (""). The public identifier is the uuid (json
// "id"); the internal row id stays out of the JSON. The payer FK is exposed as the
// related payer uuid (nil when self-managed), resolved via LEFT JOIN,
// never the internal row id.
type Client struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Reference string  `json:"reference"`
	PayerUUID *string `json:"payerId"`
	PayerName string  `json:"payerName"`
	Email     string  `json:"email"`
	Phone     string  `json:"phone"`
	Address   string  `json:"address"`
	Metadata  string  `json:"metadata"`
	CreatedAt string  `json:"createdAt"`
	UpdatedAt string  `json:"updatedAt"`
}

// ClientInput is the writable subset of a client. PayerUUID is the
// payer's uuid (nil/empty → self-managed); it is resolved to the payer row id (uuid)
// before insert/update.
type ClientInput struct {
	Name      string  `json:"name"`
	Reference string  `json:"reference"`
	PayerUUID *string `json:"payerId"`
	Email     string  `json:"email"`
	Phone     string  `json:"phone"`
	Address   string  `json:"address"`
	Metadata  string  `json:"metadata"`
}

// Validate checks the cheap required-field rules the service enforces before the
// repository runs. A failure is returned as an *apperr.ValidationError so the
// HTTP layer responds 422 with per-field detail. (client cannot import billing —
// billing's tests import client — so it uses the equivalent apperr type.) The
// unknown-payer rule is a DB-resolved domain rule and stays in the repository.
func (in ClientInput) Validate() error {
	ve := &apperr.ValidationError{}
	if in.Name == "" {
		ve.Errors = append(ve.Errors, apperr.FieldError{Line: 0, Field: "name", Message: "required"})
	}
	if len(ve.Errors) > 0 {
		return ve
	}
	return nil
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
func (r *ClientsRepo) List(ctx context.Context, tenantID string, search string) ([]*Client, error) {
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
func (r *ClientsRepo) Query(ctx context.Context, tenantID string, c listquery.Clause) ([]*Client, int64, error) {
	if tenantID == "" {
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
		var tenant string
		var payerID sql.NullString // internal FK column; never surfaced
		if err := rows.Scan(&f.id, &tenant, &f.name, &f.reference,
			&payerID, &f.email, &f.phone, &f.address,
			&f.metadata, &f.createdAt, &f.updatedAt, &f.payerName, &f.payerUUID); err != nil {
			return nil, 0, fmt.Errorf("scan client: %w", err)
		}
		out = append(out, mapClientFields(f))
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("query clients: %w", err)
	}
	return out, total, nil
}

// Get returns the tenant's client by uuid with resolved payer name +
// uuid, or (nil, nil) when absent.
func (r *ClientsRepo) Get(ctx context.Context, tenantID string, uuid string) (*Client, error) {
	row, err := gen.New(r.db).GetClient(ctx, gen.GetClientParams{TenantID: tenantID, ID: uuid})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get client: %w", err)
	}
	return toClientGet(row), nil
}

// GetByID returns the tenant's client by row id (uuid), for internal cross-slice reads
// (e.g. billing's plan-window lookup) that already hold the id. The public API
// addresses clients by uuid via Get.
func (r *ClientsRepo) GetByID(ctx context.Context, tenantID, id string) (*Client, error) {
	row, err := gen.New(r.db).GetClientByID(ctx, gen.GetClientByIDParams{TenantID: tenantID, ID: id})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get client by id: %w", err)
	}
	return toClientGetByID(row), nil
}

// resolvePayer resolves an inbound payer uuid to the payer row id (uuid) for
// insert/update. A nil/empty uuid → NULL FK. An unknown uuid (foreign or absent)
// → errPayerNotFound so the handler can 400.
func (r *ClientsRepo) resolvePayer(ctx context.Context, q *gen.Queries, tenantID string, pmUUID *string) (sql.NullString, error) {
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

// Create inserts a client and writes one audit row, atomically, then re-reads
// the row so the returned Client carries the payer name.
func (r *ClientsRepo) Create(ctx context.Context, tenantID string, in ClientInput) (*Client, error) {
	if tenantID == "" {
		return nil, errors.New("create client: tenant id required")
	}
	metadata := in.Metadata
	if metadata == "" {
		metadata = "{}"
	}

	var newUUID string
	err := audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		q := gen.New(tx)
		pmID, e := r.resolvePayer(ctx, q, tenantID, in.PayerUUID)
		if e != nil {
			return e
		}
		now := time.Now().UTC().Format(time.RFC3339)
		c, e := q.CreateClient(ctx, gen.CreateClientParams{
			ID:        ids.New(),
			TenantID:  tenantID,
			Name:      in.Name,
			Reference: db.NzMaybe(in.Reference),
			PayerID:   pmID,
			Email:     db.NzMaybe(in.Email),
			Phone:     db.NzMaybe(in.Phone),
			Address:   db.NzMaybe(in.Address),
			Metadata:  db.Nz(metadata),
			CreatedAt: now,
			UpdatedAt: now,
		})
		if e != nil {
			return fmt.Errorf("insert: %w", e)
		}
		newUUID = c.ID
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "client",
			EntityID:   c.ID,
			Action:     "create",
			Changes:    audit.Changes(map[string]any{"name": in.Name}),
		})
	})
	if errors.Is(err, errPayerNotFound) {
		return nil, errPayerNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("create client: %w", err)
	}
	return r.Get(ctx, tenantID, newUUID)
}

// Update writes the client's fields and one audit row, atomically, then
// re-reads. Returns apperr.ErrNotFound when the client does not exist. The audit
// entry records the row's id (uuid), resolved by-uuid in the same tx.
func (r *ClientsRepo) Update(ctx context.Context, tenantID string, uuid string, in ClientInput) (*Client, error) {
	metadata := in.Metadata
	if metadata == "" {
		metadata = "{}"
	}

	err := audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		q := gen.New(tx)
		pmID, e := r.resolvePayer(ctx, q, tenantID, in.PayerUUID)
		if e != nil {
			return e
		}
		now := time.Now().UTC().Format(time.RFC3339)
		row, e := q.UpdateClient(ctx, gen.UpdateClientParams{
			Name:      in.Name,
			Reference: db.NzMaybe(in.Reference),
			PayerID:   pmID,
			Email:     db.NzMaybe(in.Email),
			Phone:     db.NzMaybe(in.Phone),
			Address:   db.NzMaybe(in.Address),
			Metadata:  db.Nz(metadata),
			UpdatedAt: now,
			TenantID:  tenantID,
			ID:        uuid,
		})
		if errors.Is(e, sql.ErrNoRows) {
			return apperr.ErrNotFound
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
	if errors.Is(err, errPayerNotFound) {
		return nil, errPayerNotFound
	}
	if errors.Is(err, apperr.ErrNotFound) {
		return nil, apperr.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("update client: %w", err)
	}
	return r.Get(ctx, tenantID, uuid)
}

// Delete removes a client by uuid and writes one audit row, atomically. The
// audit entry records the row's id (uuid), resolved by-uuid in the same tx. A
// missing row returns apperr.ErrNotFound so the caller can 404.
func (r *ClientsRepo) Delete(ctx context.Context, tenantID string, uuid string) error {
	return audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		q := gen.New(tx)
		row, e := q.GetClient(ctx, gen.GetClientParams{TenantID: tenantID, ID: uuid})
		if errors.Is(e, sql.ErrNoRows) {
			return apperr.ErrNotFound
		}
		if e != nil {
			return fmt.Errorf("lookup: %w", e)
		}
		if e := q.DeleteClient(ctx, gen.DeleteClientParams{TenantID: tenantID, ID: uuid}); e != nil {
			return fmt.Errorf("delete: %w", e)
		}
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "client",
			EntityID:   row.ID,
			Action:     "delete",
		})
	})
}

// ResolveClientIDs resolves client uuids to their row ids (uuid) (preserving
// order), tenant-scoped. An unknown uuid is an error so bulk ops can 400.
func (r *ClientsRepo) ResolveClientIDs(ctx context.Context, tenantID string, clientUUIDs []string) ([]string, error) {
	q := gen.New(r.db)
	out := make([]string, 0, len(clientUUIDs))
	for i := range clientUUIDs { // bounded by len(clientUUIDs)
		id, err := q.GetClientIDByUUID(ctx, gen.GetClientIDByUUIDParams{TenantID: tenantID, ID: clientUUIDs[i]})
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
func (r *ClientsRepo) BulkDelete(ctx context.Context, tenantID string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	return audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		q := gen.New(tx)
		for _, id := range ids { // bounded by len(ids)
			if err := q.DeleteClientByID(ctx, gen.DeleteClientByIDParams{TenantID: tenantID, ID: id}); err != nil {
				return fmt.Errorf("delete %s: %w", id, err)
			}
		}
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "client",
			EntityID:   "",
			Action:     "bulk_delete",
			Changes:    audit.Changes(map[string]any{"ids": ids}),
		})
	})
}

// clientFields is the shared, flat shape of every clients join row (List,
// Search and Get produce identical structs under distinct gen names).
type clientFields struct {
	id, name                        string
	reference                       sql.NullString
	email, phone, address, metadata sql.NullString
	createdAt, updatedAt            string
	payerName, payerUUID            sql.NullString
}

// mapClientFields builds a domain Client from the unwrapped columns. The int
// payer_id is never surfaced; the joined payer uuid is.
func mapClientFields(f clientFields) *Client {
	return &Client{
		ID:        f.id,
		Name:      f.name,
		Reference: f.reference.String,
		PayerUUID: db.PtrStr(f.payerUUID),
		PayerName: f.payerName.String,
		Email:     f.email.String,
		Phone:     f.phone.String,
		Address:   f.address.String,
		Metadata:  f.metadata.String,
		CreatedAt: f.createdAt,
		UpdatedAt: f.updatedAt,
	}
}

func toClientList(r gen.ListClientsRow) *Client {
	return mapClientFields(clientFields{
		id: r.ID, name: r.Name,
		reference: r.Reference,
		email:     r.Email, phone: r.Phone, address: r.Address, metadata: r.Metadata,
		createdAt: r.CreatedAt, updatedAt: r.UpdatedAt,
		payerName: r.PayerName, payerUUID: r.PayerUuid,
	})
}

func toClientSearch(r gen.SearchClientsRow) *Client {
	return mapClientFields(clientFields{
		id: r.ID, name: r.Name,
		reference: r.Reference,
		email:     r.Email, phone: r.Phone, address: r.Address, metadata: r.Metadata,
		createdAt: r.CreatedAt, updatedAt: r.UpdatedAt,
		payerName: r.PayerName, payerUUID: r.PayerUuid,
	})
}

func toClientGet(r gen.GetClientRow) *Client {
	return mapClientFields(clientFields{
		id: r.ID, name: r.Name,
		reference: r.Reference,
		email:     r.Email, phone: r.Phone, address: r.Address, metadata: r.Metadata,
		createdAt: r.CreatedAt, updatedAt: r.UpdatedAt,
		payerName: r.PayerName, payerUUID: r.PayerUuid,
	})
}

func toClientGetByID(r gen.GetClientByIDRow) *Client {
	return mapClientFields(clientFields{
		id: r.ID, name: r.Name,
		reference: r.Reference,
		email:     r.Email, phone: r.Phone, address: r.Address, metadata: r.Metadata,
		createdAt: r.CreatedAt, updatedAt: r.UpdatedAt,
		payerName: r.PayerName, payerUUID: r.PayerUuid,
	})
}
