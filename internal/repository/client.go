package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/dknathalage/tallyo/internal/audit"
	"github.com/dknathalage/tallyo/internal/db/gen"
	"github.com/google/uuid"
)

// Client is the domain view of a row in the clients table. Nullable columns are
// unwrapped to plain strings (""); nullable FKs to *int64 (nil when absent). The
// tier/payer names are resolved via LEFT JOIN.
type Client struct {
	ID              int64  `json:"id"`
	UUID            string `json:"uuid"`
	Name            string `json:"name"`
	Email           string `json:"email"`
	Phone           string `json:"phone"`
	Address         string `json:"address"`
	PricingTierID   *int64 `json:"pricingTierId"`
	PricingTierName string `json:"pricingTierName"`
	Metadata        string `json:"metadata"`
	PayerID         *int64 `json:"payerId"`
	PayerName       string `json:"payerName"`
	CreatedAt       string `json:"createdAt"`
	UpdatedAt       string `json:"updatedAt"`
}

// ClientInput is the writable subset of a client.
type ClientInput struct {
	Name          string `json:"name"`
	Email         string `json:"email"`
	Phone         string `json:"phone"`
	Address       string `json:"address"`
	PricingTierID *int64 `json:"pricingTierId"`
	Metadata      string `json:"metadata"`
	PayerID       *int64 `json:"payerId"`
}

// ClientsRepo reads and writes the clients table with audited mutations.
type ClientsRepo struct {
	db *sql.DB
}

// NewClients constructs a repository. A nil db is a programmer error.
func NewClients(db *sql.DB) *ClientsRepo {
	if db == nil {
		panic("repository: NewClients requires a non-nil *sql.DB")
	}
	return &ClientsRepo{db: db}
}

// List returns all clients ordered by name. When search is non-empty it filters
// to clients whose name or email matches the term (LIKE). The slice is non-nil.
func (r *ClientsRepo) List(ctx context.Context, search string) ([]*Client, error) {
	q := gen.New(r.db)
	if search == "" {
		rows, err := q.ListClients(ctx)
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
	rows, err := q.SearchClients(ctx, gen.SearchClientsParams{Name: like, Email: nz(like)})
	if err != nil {
		return nil, fmt.Errorf("search clients: %w", err)
	}
	out := make([]*Client, 0, len(rows))
	for i := range rows {
		out = append(out, toClientSearch(rows[i]))
	}
	return out, nil
}

// Get returns the client with resolved join names, or (nil, nil) when absent.
func (r *ClientsRepo) Get(ctx context.Context, id int64) (*Client, error) {
	row, err := gen.New(r.db).GetClient(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get client: %w", err)
	}
	return toClientGet(row), nil
}

// Create inserts a client and writes one audit row, atomically, then re-reads
// the row so the returned Client carries resolved tier/payer names.
func (r *ClientsRepo) Create(ctx context.Context, in ClientInput) (*Client, error) {
	if in.Name == "" {
		return nil, errors.New("create client: name is required")
	}
	metadata := in.Metadata
	if metadata == "" {
		metadata = "{}"
	}

	var newID int64
	err := audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339)
		c, e := gen.New(tx).CreateClient(ctx, gen.CreateClientParams{
			Uuid:          uuid.NewString(),
			Name:          in.Name,
			Email:         nz(in.Email),
			Phone:         nz(in.Phone),
			Address:       nz(in.Address),
			PricingTierID: nullID(in.PricingTierID),
			Metadata:      nz(metadata),
			PayerID:       nullID(in.PayerID),
			CreatedAt:     now,
			UpdatedAt:     now,
		})
		if e != nil {
			return fmt.Errorf("insert: %w", e)
		}
		newID = c.ID
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "client",
			EntityID:   c.ID,
			Action:     "create",
			Changes:    audit.Changes(map[string]any{"name": in.Name}),
		})
	})
	if err != nil {
		return nil, fmt.Errorf("create client: %w", err)
	}
	return r.Get(ctx, newID)
}

// Update writes the client's fields and one audit row, atomically, then re-reads
// the row for resolved names. Returns (nil, nil) when the client does not exist.
func (r *ClientsRepo) Update(ctx context.Context, id int64, in ClientInput) (*Client, error) {
	if in.Name == "" {
		return nil, errors.New("update client: name is required")
	}
	metadata := in.Metadata
	if metadata == "" {
		metadata = "{}"
	}

	var missing bool
	err := audit.WithTx(ctx, r.db, audit.Entry{
		EntityType: "client",
		EntityID:   id,
		Action:     "update",
		Changes:    audit.Changes(map[string]any{"name": in.Name}),
	}, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339)
		_, e := gen.New(tx).UpdateClient(ctx, gen.UpdateClientParams{
			Name:          in.Name,
			Email:         nz(in.Email),
			Phone:         nz(in.Phone),
			Address:       nz(in.Address),
			PricingTierID: nullID(in.PricingTierID),
			Metadata:      nz(metadata),
			PayerID:       nullID(in.PayerID),
			UpdatedAt:     now,
			ID:            id,
		})
		if errors.Is(e, sql.ErrNoRows) {
			missing = true
			return e
		}
		if e != nil {
			return fmt.Errorf("update: %w", e)
		}
		return nil
	})
	if missing {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("update client: %w", err)
	}
	return r.Get(ctx, id)
}

// Delete removes a client and writes one audit row, atomically.
func (r *ClientsRepo) Delete(ctx context.Context, id int64) error {
	return audit.WithTx(ctx, r.db, audit.Entry{
		EntityType: "client",
		EntityID:   id,
		Action:     "delete",
	}, func(tx *sql.Tx) error {
		if err := gen.New(tx).DeleteClient(ctx, id); err != nil {
			return fmt.Errorf("delete: %w", err)
		}
		return nil
	})
}

// BulkDelete removes several clients and writes one audit row, atomically. An
// empty id list is a no-op.
func (r *ClientsRepo) BulkDelete(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	return audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		q := gen.New(tx)
		for _, id := range ids { // bounded by len(ids)
			if err := q.DeleteClient(ctx, id); err != nil {
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

// nullID wraps an optional id into a sql.NullInt64 (invalid when nil).
func nullID(p *int64) sql.NullInt64 {
	if p == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *p, Valid: true}
}

// ptrID unwraps a sql.NullInt64 into a *int64 (nil when invalid).
func ptrID(n sql.NullInt64) *int64 {
	if !n.Valid {
		return nil
	}
	v := n.Int64
	return &v
}

// clientFields is the shared, flat shape of every clients join row (List, Search
// and Get produce identical structs under distinct gen type names).
type clientFields struct {
	id                         int64
	uuid, name                 string
	email, phone, address      sql.NullString
	pricingTierID              sql.NullInt64
	metadata                   sql.NullString
	payerID                    sql.NullInt64
	createdAt, updatedAt       string
	pricingTierName, payerName sql.NullString
}

// mapClientFields builds a domain Client from the unwrapped join columns.
func mapClientFields(f clientFields) *Client {
	return &Client{
		ID:              f.id,
		UUID:            f.uuid,
		Name:            f.name,
		Email:           f.email.String,
		Phone:           f.phone.String,
		Address:         f.address.String,
		PricingTierID:   ptrID(f.pricingTierID),
		PricingTierName: f.pricingTierName.String,
		Metadata:        f.metadata.String,
		PayerID:         ptrID(f.payerID),
		PayerName:       f.payerName.String,
		CreatedAt:       f.createdAt,
		UpdatedAt:       f.updatedAt,
	}
}

// toClientList adapts a ListClientsRow to the domain Client.
func toClientList(r gen.ListClientsRow) *Client {
	return mapClientFields(clientFields{
		id: r.ID, uuid: r.Uuid, name: r.Name,
		email: r.Email, phone: r.Phone, address: r.Address,
		pricingTierID: r.PricingTierID, metadata: r.Metadata, payerID: r.PayerID,
		createdAt: r.CreatedAt, updatedAt: r.UpdatedAt,
		pricingTierName: r.PricingTierName, payerName: r.PayerName,
	})
}

// toClientSearch adapts a SearchClientsRow to the domain Client.
func toClientSearch(r gen.SearchClientsRow) *Client {
	return mapClientFields(clientFields{
		id: r.ID, uuid: r.Uuid, name: r.Name,
		email: r.Email, phone: r.Phone, address: r.Address,
		pricingTierID: r.PricingTierID, metadata: r.Metadata, payerID: r.PayerID,
		createdAt: r.CreatedAt, updatedAt: r.UpdatedAt,
		pricingTierName: r.PricingTierName, payerName: r.PayerName,
	})
}

// toClientGet adapts a GetClientRow to the domain Client.
func toClientGet(r gen.GetClientRow) *Client {
	return mapClientFields(clientFields{
		id: r.ID, uuid: r.Uuid, name: r.Name,
		email: r.Email, phone: r.Phone, address: r.Address,
		pricingTierID: r.PricingTierID, metadata: r.Metadata, payerID: r.PayerID,
		createdAt: r.CreatedAt, updatedAt: r.UpdatedAt,
		pricingTierName: r.PricingTierName, payerName: r.PayerName,
	})
}
