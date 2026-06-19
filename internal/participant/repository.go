// Package participant is the participant vertical slice: domain types, the
// audited repository over the participants table, the service (with SSE
// broadcast), and the HTTP handler. It depends only on platform packages
// (db/gen, audit, reqctx, realtime, httpx), never on other domain slices.
package participant

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/dknathalage/tallyo/internal/db"
	"time"

	"github.com/dknathalage/tallyo/internal/audit"
	"github.com/dknathalage/tallyo/internal/db/gen"
	"github.com/dknathalage/tallyo/internal/listquery"
	"github.com/google/uuid"
)

// participantListSelect mirrors the ListParticipants sqlc query body up to the
// WHERE. Keep in sync with internal/db/queries/participants.sql.
const participantListSelect = `SELECT p.*, pm.name AS plan_manager_name
FROM participants p
LEFT JOIN plan_managers pm ON p.plan_manager_id = pm.id AND pm.tenant_id = p.tenant_id
WHERE p.tenant_id = ?`

// ParticipantCols is the listquery allowlist for participants.
var ParticipantCols = listquery.Spec{
	"name":  {Col: "p.name", Filter: listquery.Text},
	"ndis":  {Col: "p.ndis_number", Filter: listquery.Text},
	"email": {Col: "p.email", Filter: listquery.Text},
	"mgmt":  {Col: "p.mgmt_type", Filter: listquery.Enum},
	"start": {Col: "p.plan_start", Filter: listquery.Date},
	"pm":    {Col: "pm.name", Filter: listquery.Text},
}

// Participant is the domain view of a row in the participants table. Nullable
// columns are unwrapped to plain strings (""); the plan_manager FK to *int64
// (nil when self-managed). The plan-manager name is resolved via LEFT JOIN.
type Participant struct {
	ID              int64  `json:"id"`
	UUID            string `json:"uuid"`
	Name            string `json:"name"`
	NDISNumber      string `json:"ndisNumber"`
	PlanStart       string `json:"planStart"`
	PlanEnd         string `json:"planEnd"`
	MgmtType        string `json:"mgmtType"`
	PlanManagerID   *int64 `json:"planManagerId"`
	PlanManagerName string `json:"planManagerName"`
	Email           string `json:"email"`
	Phone           string `json:"phone"`
	Address         string `json:"address"`
	Metadata        string `json:"metadata"`
	CreatedAt       string `json:"createdAt"`
	UpdatedAt       string `json:"updatedAt"`
}

// ParticipantInput is the writable subset of a participant.
type ParticipantInput struct {
	Name          string `json:"name"`
	NDISNumber    string `json:"ndisNumber"`
	PlanStart     string `json:"planStart"`
	PlanEnd       string `json:"planEnd"`
	MgmtType      string `json:"mgmtType"`
	PlanManagerID *int64 `json:"planManagerId"`
	Email         string `json:"email"`
	Phone         string `json:"phone"`
	Address       string `json:"address"`
	Metadata      string `json:"metadata"`
}

// ParticipantsRepo reads and writes the participants table (tenant-scoped) with
// audited mutations.
type ParticipantsRepo struct {
	db *sql.DB
}

// NewParticipants constructs a repository. A nil db is a programmer error.
func NewParticipants(db *sql.DB) *ParticipantsRepo {
	if db == nil {
		panic("participant: NewParticipants requires a non-nil *sql.DB")
	}
	return &ParticipantsRepo{db: db}
}

// List returns the tenant's participants ordered by name. When search is
// non-empty it filters to name, email, or NDIS number matches (LIKE).
func (r *ParticipantsRepo) List(ctx context.Context, tenantID int64, search string) ([]*Participant, error) {
	q := gen.New(r.db)
	if search == "" {
		rows, err := q.ListParticipants(ctx, tenantID)
		if err != nil {
			return nil, fmt.Errorf("list participants: %w", err)
		}
		out := make([]*Participant, 0, len(rows))
		for i := range rows {
			out = append(out, toParticipantList(rows[i]))
		}
		return out, nil
	}
	like := "%" + search + "%"
	rows, err := q.SearchParticipants(ctx, gen.SearchParticipantsParams{
		TenantID:   tenantID,
		Name:       like,
		Email:      db.Nz(like),
		NdisNumber: db.Nz(like),
	})
	if err != nil {
		return nil, fmt.Errorf("search participants: %w", err)
	}
	out := make([]*Participant, 0, len(rows))
	for i := range rows {
		out = append(out, toParticipantSearch(rows[i]))
	}
	return out, nil
}

// Query returns one page of participants plus the total row count for the
// filter (ignoring pagination). The clause is built by listquery from an
// allowlisted spec, so its Where/Order fragments are injection-safe.
func (r *ParticipantsRepo) Query(ctx context.Context, tenantID int64, c listquery.Clause) ([]*Participant, int64, error) {
	if tenantID == 0 {
		return nil, 0, errors.New("query participants: tenant id required")
	}
	var total int64
	countSQL := "SELECT count(*) FROM (" + participantListSelect + c.Where + ")"
	countArgs := append([]any{tenantID}, c.CountArgs()...)
	if err := r.db.QueryRowContext(ctx, countSQL, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count participants: %w", err)
	}
	sqlText := participantListSelect + c.Where + c.Order + c.Limit
	pageArgs := append([]any{tenantID}, c.Args...)
	rows, err := r.db.QueryContext(ctx, sqlText, pageArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("query participants: %w", err)
	}
	defer rows.Close()
	out := make([]*Participant, 0, 50)
	for rows.Next() { // bounded by LIMIT in the query
		var f participantFields
		var tenant int64
		if err := rows.Scan(&f.id, &f.uuid, &tenant, &f.name, &f.ndisNumber, &f.planStart,
			&f.planEnd, &f.mgmtType, &f.planManagerID, &f.email, &f.phone, &f.address,
			&f.metadata, &f.createdAt, &f.updatedAt, &f.planManagerName); err != nil {
			return nil, 0, fmt.Errorf("scan participant: %w", err)
		}
		out = append(out, mapParticipantFields(f))
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("query participants: %w", err)
	}
	return out, total, nil
}

// Get returns the tenant's participant with resolved plan-manager name, or
// (nil, nil) when absent.
func (r *ParticipantsRepo) Get(ctx context.Context, tenantID, id int64) (*Participant, error) {
	row, err := gen.New(r.db).GetParticipant(ctx, gen.GetParticipantParams{TenantID: tenantID, ID: id})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get participant: %w", err)
	}
	return toParticipantGet(row), nil
}

// Create inserts a participant and writes one audit row, atomically, then
// re-reads the row so the returned Participant carries the plan-manager name.
func (r *ParticipantsRepo) Create(ctx context.Context, tenantID int64, in ParticipantInput) (*Participant, error) {
	if tenantID == 0 {
		return nil, errors.New("create participant: tenant id required")
	}
	if in.Name == "" {
		return nil, errors.New("create participant: name is required")
	}
	metadata := in.Metadata
	if metadata == "" {
		metadata = "{}"
	}
	mgmtType := in.MgmtType
	if mgmtType == "" {
		mgmtType = "plan"
	}

	var newID int64
	err := audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339)
		c, e := gen.New(tx).CreateParticipant(ctx, gen.CreateParticipantParams{
			Uuid:          uuid.NewString(),
			TenantID:      tenantID,
			Name:          in.Name,
			NdisNumber:    db.NzMaybe(in.NDISNumber),
			PlanStart:     db.NzMaybe(in.PlanStart),
			PlanEnd:       db.NzMaybe(in.PlanEnd),
			MgmtType:      mgmtType,
			PlanManagerID: db.NullID(in.PlanManagerID),
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
		newID = c.ID
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "participant",
			EntityID:   c.ID,
			Action:     "create",
			Changes:    audit.Changes(map[string]any{"name": in.Name}),
		})
	})
	if err != nil {
		return nil, fmt.Errorf("create participant: %w", err)
	}
	return r.Get(ctx, tenantID, newID)
}

// Update writes the participant's fields and one audit row, atomically, then
// re-reads. Returns (nil, nil) when the participant does not exist.
func (r *ParticipantsRepo) Update(ctx context.Context, tenantID, id int64, in ParticipantInput) (*Participant, error) {
	if in.Name == "" {
		return nil, errors.New("update participant: name is required")
	}
	metadata := in.Metadata
	if metadata == "" {
		metadata = "{}"
	}
	mgmtType := in.MgmtType
	if mgmtType == "" {
		mgmtType = "plan"
	}

	var missing bool
	err := audit.WithTx(ctx, r.db, audit.Entry{
		EntityType: "participant",
		EntityID:   id,
		Action:     "update",
		Changes:    audit.Changes(map[string]any{"name": in.Name}),
	}, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339)
		_, e := gen.New(tx).UpdateParticipant(ctx, gen.UpdateParticipantParams{
			Name:          in.Name,
			NdisNumber:    db.NzMaybe(in.NDISNumber),
			PlanStart:     db.NzMaybe(in.PlanStart),
			PlanEnd:       db.NzMaybe(in.PlanEnd),
			MgmtType:      mgmtType,
			PlanManagerID: db.NullID(in.PlanManagerID),
			Email:         db.NzMaybe(in.Email),
			Phone:         db.NzMaybe(in.Phone),
			Address:       db.NzMaybe(in.Address),
			Metadata:      db.Nz(metadata),
			UpdatedAt:     now,
			TenantID:      tenantID,
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
		return nil, fmt.Errorf("update participant: %w", err)
	}
	return r.Get(ctx, tenantID, id)
}

// Delete removes a participant and writes one audit row, atomically.
func (r *ParticipantsRepo) Delete(ctx context.Context, tenantID, id int64) error {
	return audit.WithTx(ctx, r.db, audit.Entry{
		EntityType: "participant",
		EntityID:   id,
		Action:     "delete",
	}, func(tx *sql.Tx) error {
		if err := gen.New(tx).DeleteParticipant(ctx, gen.DeleteParticipantParams{TenantID: tenantID, ID: id}); err != nil {
			return fmt.Errorf("delete: %w", err)
		}
		return nil
	})
}

// BulkDelete removes several participants and writes one audit row, atomically.
// An empty id list is a no-op.
func (r *ParticipantsRepo) BulkDelete(ctx context.Context, tenantID int64, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	return audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		q := gen.New(tx)
		for _, id := range ids { // bounded by len(ids)
			if err := q.DeleteParticipant(ctx, gen.DeleteParticipantParams{TenantID: tenantID, ID: id}); err != nil {
				return fmt.Errorf("delete %d: %w", id, err)
			}
		}
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "participant",
			EntityID:   0,
			Action:     "bulk_delete",
			Changes:    audit.Changes(map[string]any{"ids": ids}),
		})
	})
}

// participantFields is the shared, flat shape of every participants join row
// (List, Search and Get produce identical structs under distinct gen names).
type participantFields struct {
	id                              int64
	uuid, name, mgmtType            string
	ndisNumber, planStart, planEnd  sql.NullString
	planManagerID                   sql.NullInt64
	email, phone, address, metadata sql.NullString
	createdAt, updatedAt            string
	planManagerName                 sql.NullString
}

// mapParticipantFields builds a domain Participant from the unwrapped columns.
func mapParticipantFields(f participantFields) *Participant {
	return &Participant{
		ID:              f.id,
		UUID:            f.uuid,
		Name:            f.name,
		NDISNumber:      f.ndisNumber.String,
		PlanStart:       f.planStart.String,
		PlanEnd:         f.planEnd.String,
		MgmtType:        f.mgmtType,
		PlanManagerID:   db.PtrID(f.planManagerID),
		PlanManagerName: f.planManagerName.String,
		Email:           f.email.String,
		Phone:           f.phone.String,
		Address:         f.address.String,
		Metadata:        f.metadata.String,
		CreatedAt:       f.createdAt,
		UpdatedAt:       f.updatedAt,
	}
}

func toParticipantList(r gen.ListParticipantsRow) *Participant {
	return mapParticipantFields(participantFields{
		id: r.ID, uuid: r.Uuid, name: r.Name, mgmtType: r.MgmtType,
		ndisNumber: r.NdisNumber, planStart: r.PlanStart, planEnd: r.PlanEnd,
		planManagerID: r.PlanManagerID,
		email:         r.Email, phone: r.Phone, address: r.Address, metadata: r.Metadata,
		createdAt: r.CreatedAt, updatedAt: r.UpdatedAt, planManagerName: r.PlanManagerName,
	})
}

func toParticipantSearch(r gen.SearchParticipantsRow) *Participant {
	return mapParticipantFields(participantFields{
		id: r.ID, uuid: r.Uuid, name: r.Name, mgmtType: r.MgmtType,
		ndisNumber: r.NdisNumber, planStart: r.PlanStart, planEnd: r.PlanEnd,
		planManagerID: r.PlanManagerID,
		email:         r.Email, phone: r.Phone, address: r.Address, metadata: r.Metadata,
		createdAt: r.CreatedAt, updatedAt: r.UpdatedAt, planManagerName: r.PlanManagerName,
	})
}

func toParticipantGet(r gen.GetParticipantRow) *Participant {
	return mapParticipantFields(participantFields{
		id: r.ID, uuid: r.Uuid, name: r.Name, mgmtType: r.MgmtType,
		ndisNumber: r.NdisNumber, planStart: r.PlanStart, planEnd: r.PlanEnd,
		planManagerID: r.PlanManagerID,
		email:         r.Email, phone: r.Phone, address: r.Address, metadata: r.Metadata,
		createdAt: r.CreatedAt, updatedAt: r.UpdatedAt, planManagerName: r.PlanManagerName,
	})
}
