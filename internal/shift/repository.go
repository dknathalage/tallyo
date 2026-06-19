// Package shift is the shift vertical slice: domain types, the audited
// repository over the shifts table, the service (with SSE broadcast), and the
// HTTP handler. It depends only on platform packages (db/gen, audit, reqctx,
// realtime, httpx), never on other domain slices.
package shift

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/dknathalage/tallyo/internal/db"
	"time"

	"github.com/dknathalage/tallyo/internal/audit"
	"github.com/dknathalage/tallyo/internal/db/gen"
	"github.com/google/uuid"
)

// Measure is one structured outcome captured during a shift (e.g. a goal score
// or a billable quantity). It is stored as JSON inside the shift's measures
// column.
type Measure struct {
	Label string  `json:"label"`
	Value float64 `json:"value"`
	Unit  string  `json:"unit"`
	Code  string  `json:"code"`
}

// Shift is the domain view of a row in the shifts table — the delivered-support
// unit a provider records for a participant. Measures and Tags are stored as
// JSON TEXT and are never nil. Status moves through the lifecycle
// scheduled→recorded→drafted→sent→paid; InvoiceID is set once the shift is
// drafted onto an invoice.
type Shift struct {
	ID            int64     `json:"id"`
	UUID          string    `json:"uuid"`
	ParticipantID int64     `json:"participantId"`
	ServiceDate   string    `json:"serviceDate"`
	StartTime     string    `json:"startTime"`
	EndTime       string    `json:"endTime"`
	Hours         float64   `json:"hours"`
	Km            float64   `json:"km"`
	Measures      []Measure `json:"measures"`
	Note          string    `json:"note"`
	Tags          []string  `json:"tags"`
	Status        string    `json:"status"`
	InvoiceID     *int64    `json:"invoiceId"`
	AuthorUserID  *int64    `json:"authorUserId"`
	CreatedAt     string    `json:"createdAt"`
	UpdatedAt     string    `json:"updatedAt"`
}

// ShiftInput is the writable subset of a shift.
type ShiftInput struct {
	ParticipantID int64     `json:"participantId"`
	ServiceDate   string    `json:"serviceDate"`
	StartTime     string    `json:"startTime"`
	EndTime       string    `json:"endTime"`
	Hours         float64   `json:"hours"`
	Km            float64   `json:"km"`
	Measures      []Measure `json:"measures"`
	Note          string    `json:"note"`
	Tags          []string  `json:"tags"`
	Status        string    `json:"status"`
}

// UnbilledAgg summarises a participant's recorded-but-unbilled shifts: how many there
// are and the service-date span they cover.
type UnbilledAgg struct {
	ParticipantID int64  `json:"participantId"`
	Count         int64  `json:"count"`
	From          string `json:"from"`
	To            string `json:"to"`
}

// ShiftsRepo reads and writes the shifts table (tenant-scoped) with audited
// mutations.
type ShiftsRepo struct {
	db *sql.DB
}

// NewShifts constructs a repository. A nil db is a programmer error.
func NewShifts(db *sql.DB) *ShiftsRepo {
	if db == nil {
		panic("shift: NewShifts requires a non-nil *sql.DB")
	}
	return &ShiftsRepo{db: db}
}

// Create inserts a shift and writes one audit row, atomically. authorUserID is
// the user the shift is attributed to (nil when unknown). Status defaults to
// 'recorded' when the input leaves it empty.
func (r *ShiftsRepo) Create(ctx context.Context, tenantID int64, authorUserID *int64, in ShiftInput) (*Shift, error) {
	if tenantID == 0 {
		return nil, errors.New("create shift: tenant id required")
	}
	if in.ParticipantID == 0 {
		return nil, errors.New("create shift: participant id required")
	}
	if !validISODate(in.ServiceDate) {
		return nil, errors.New("create shift: service date must be a valid YYYY-MM-DD date")
	}
	if err := assertShiftNonNegative(in); err != nil {
		return nil, err
	}
	measures, tags, err := encodeShiftJSON(in)
	if err != nil {
		return nil, err
	}
	status := in.Status
	if status == "" {
		status = "recorded"
	}

	var newID int64
	err = audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339)
		s, e := gen.New(tx).CreateShift(ctx, gen.CreateShiftParams{
			Uuid:          uuid.NewString(),
			TenantID:      tenantID,
			ParticipantID: in.ParticipantID,
			ServiceDate:   in.ServiceDate,
			StartTime:     in.StartTime,
			EndTime:       in.EndTime,
			Hours:         in.Hours,
			Km:            in.Km,
			Measures:      measures,
			Note:          in.Note,
			Tags:          tags,
			Status:        status,
			AuthorUserID:  db.NullID(authorUserID),
			CreatedAt:     now,
			UpdatedAt:     now,
		})
		if e != nil {
			return fmt.Errorf("insert: %w", e)
		}
		newID = s.ID
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "shift", EntityID: s.ID, Action: "create",
			Changes: audit.Changes(map[string]any{"participantId": in.ParticipantID, "serviceDate": in.ServiceDate}),
		})
	})
	if err != nil {
		return nil, fmt.Errorf("create shift: %w", err)
	}
	return r.Get(ctx, tenantID, newID)
}

// Get returns the tenant's shift by id, or (nil, nil) when absent.
func (r *ShiftsRepo) Get(ctx context.Context, tenantID, id int64) (*Shift, error) {
	row, err := gen.New(r.db).GetShift(ctx, gen.GetShiftParams{TenantID: tenantID, ID: id})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get shift: %w", err)
	}
	return toShift(row)
}

// ListParticipant returns a participant's shifts. When both from and to are
// non-empty it restricts to service_date ∈ [from, to]; otherwise it returns all.
func (r *ShiftsRepo) ListParticipant(ctx context.Context, tenantID, participantID int64, from, to string) ([]*Shift, error) {
	if tenantID == 0 || participantID == 0 {
		return nil, errors.New("list shifts: tenant and participant id required")
	}
	q := gen.New(r.db)
	if from != "" && to != "" {
		rows, err := q.ListShiftsByParticipantRange(ctx, gen.ListShiftsByParticipantRangeParams{
			TenantID: tenantID, ParticipantID: participantID, ServiceDate: from, ServiceDate_2: to,
		})
		if err != nil {
			return nil, fmt.Errorf("list participant shifts range: %w", err)
		}
		return toShifts(rows)
	}
	rows, err := q.ListShiftsByParticipant(ctx, gen.ListShiftsByParticipantParams{
		TenantID: tenantID, ParticipantID: participantID,
	})
	if err != nil {
		return nil, fmt.Errorf("list participant shifts: %w", err)
	}
	return toShifts(rows)
}

// List returns all of the tenant's shifts (newest service date first).
func (r *ShiftsRepo) List(ctx context.Context, tenantID int64) ([]*Shift, error) {
	if tenantID == 0 {
		return nil, errors.New("list shifts: tenant id required")
	}
	rows, err := gen.New(r.db).ListShifts(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list shifts: %w", err)
	}
	return toShifts(rows)
}

// ListByStatus returns the tenant's shifts in a given lifecycle status.
func (r *ShiftsRepo) ListByStatus(ctx context.Context, tenantID int64, status string) ([]*Shift, error) {
	if tenantID == 0 {
		return nil, errors.New("list shifts by status: tenant id required")
	}
	rows, err := gen.New(r.db).ListShiftsByStatus(ctx, gen.ListShiftsByStatusParams{TenantID: tenantID, Status: status})
	if err != nil {
		return nil, fmt.Errorf("list shifts by status: %w", err)
	}
	return toShifts(rows)
}

// ListScheduled returns the tenant's scheduled (not yet recorded) shifts.
func (r *ShiftsRepo) ListScheduled(ctx context.Context, tenantID int64) ([]*Shift, error) {
	if tenantID == 0 {
		return nil, errors.New("list scheduled shifts: tenant id required")
	}
	rows, err := gen.New(r.db).ListScheduledShifts(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list scheduled shifts: %w", err)
	}
	return toShifts(rows)
}

// ListRecordedUnbilled returns a participant's recorded shifts that are not yet
// linked to an invoice (status 'recorded', invoice_id NULL).
func (r *ShiftsRepo) ListRecordedUnbilled(ctx context.Context, tenantID, participantID int64) ([]*Shift, error) {
	if tenantID == 0 || participantID == 0 {
		return nil, errors.New("list recorded unbilled: tenant and participant id required")
	}
	rows, err := gen.New(r.db).ListRecordedUnbilledByParticipant(ctx, gen.ListRecordedUnbilledByParticipantParams{
		TenantID: tenantID, ParticipantID: participantID,
	})
	if err != nil {
		return nil, fmt.Errorf("list recorded unbilled: %w", err)
	}
	return toShifts(rows)
}

// Update rewrites a shift's editable fields and writes one audit row, atomically.
// Returns (nil, nil) when the shift does not exist for the tenant.
func (r *ShiftsRepo) Update(ctx context.Context, tenantID, id int64, in ShiftInput) (*Shift, error) {
	if !validISODate(in.ServiceDate) {
		return nil, errors.New("update shift: service date must be a valid YYYY-MM-DD date")
	}
	if err := assertShiftNonNegative(in); err != nil {
		return nil, err
	}
	measures, tags, err := encodeShiftJSON(in)
	if err != nil {
		return nil, err
	}
	status := in.Status
	if status == "" {
		status = "recorded"
	}

	var missing bool
	err = audit.WithTx(ctx, r.db, audit.Entry{
		EntityType: "shift", EntityID: id, Action: "update",
	}, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339)
		_, e := gen.New(tx).UpdateShift(ctx, gen.UpdateShiftParams{
			ServiceDate: in.ServiceDate,
			StartTime:   in.StartTime,
			EndTime:     in.EndTime,
			Hours:       in.Hours,
			Km:          in.Km,
			Measures:    measures,
			Note:        in.Note,
			Tags:        tags,
			Status:      status,
			UpdatedAt:   now,
			TenantID:    tenantID,
			ID:          id,
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
		return nil, fmt.Errorf("update shift: %w", err)
	}
	return r.Get(ctx, tenantID, id)
}

// UpdateStatus sets a shift's lifecycle status and writes one audit row.
func (r *ShiftsRepo) UpdateStatus(ctx context.Context, tenantID, id int64, status string) error {
	if status == "" {
		return errors.New("update shift status: status required")
	}
	return audit.WithTx(ctx, r.db, audit.Entry{
		EntityType: "shift", EntityID: id, Action: "status",
		Changes: audit.Changes(map[string]any{"status": status}),
	}, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339)
		if err := gen.New(tx).UpdateShiftStatus(ctx, gen.UpdateShiftStatusParams{
			Status: status, UpdatedAt: now, TenantID: tenantID, ID: id,
		}); err != nil {
			return fmt.Errorf("update status: %w", err)
		}
		return nil
	})
}

// SetInvoice links a shift to an invoice and sets its status, atomically.
func (r *ShiftsRepo) SetInvoice(ctx context.Context, tenantID, id, invoiceID int64, status string) error {
	if invoiceID == 0 {
		return errors.New("set shift invoice: invoice id required")
	}
	if status == "" {
		return errors.New("set shift invoice: status required")
	}
	return audit.WithTx(ctx, r.db, audit.Entry{
		EntityType: "shift", EntityID: id, Action: "bill",
		Changes: audit.Changes(map[string]any{"invoiceId": invoiceID, "status": status}),
	}, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339)
		if err := gen.New(tx).SetShiftInvoice(ctx, gen.SetShiftInvoiceParams{
			InvoiceID: sql.NullInt64{Int64: invoiceID, Valid: true}, Status: status,
			UpdatedAt: now, TenantID: tenantID, ID: id,
		}); err != nil {
			return fmt.Errorf("set invoice: %w", err)
		}
		return nil
	})
}

// SetStatusForInvoice sets the status of every shift linked to an invoice (e.g.
// cascading 'sent'/'paid' from the invoice), atomically.
func (r *ShiftsRepo) SetStatusForInvoice(ctx context.Context, tenantID, invoiceID int64, status string) error {
	if invoiceID == 0 {
		return errors.New("set status for invoice: invoice id required")
	}
	if status == "" {
		return errors.New("set status for invoice: status required")
	}
	return audit.WithTx(ctx, r.db, audit.Entry{
		EntityType: "shift", EntityID: 0, Action: "status",
		Changes: audit.Changes(map[string]any{"invoiceId": invoiceID, "status": status}),
	}, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339)
		if err := gen.New(tx).SetStatusForInvoice(ctx, gen.SetStatusForInvoiceParams{
			Status: status, UpdatedAt: now, TenantID: tenantID,
			InvoiceID: sql.NullInt64{Int64: invoiceID, Valid: true},
		}); err != nil {
			return fmt.Errorf("set status for invoice: %w", err)
		}
		return nil
	})
}

// ClearForInvoice reverts every shift linked to an invoice back to 'recorded'
// with a NULL invoice_id (used when the invoice is deleted), atomically.
func (r *ShiftsRepo) ClearForInvoice(ctx context.Context, tenantID, invoiceID int64) error {
	if invoiceID == 0 {
		return errors.New("clear shifts for invoice: invoice id required")
	}
	return audit.WithTx(ctx, r.db, audit.Entry{
		EntityType: "shift", EntityID: 0, Action: "unbill",
		Changes: audit.Changes(map[string]any{"invoiceId": invoiceID}),
	}, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339)
		if err := gen.New(tx).ClearShiftsForInvoice(ctx, gen.ClearShiftsForInvoiceParams{
			UpdatedAt: now, TenantID: tenantID,
			InvoiceID: sql.NullInt64{Int64: invoiceID, Valid: true},
		}); err != nil {
			return fmt.Errorf("clear for invoice: %w", err)
		}
		return nil
	})
}

// Delete removes a shift and writes one audit row, atomically.
func (r *ShiftsRepo) Delete(ctx context.Context, tenantID, id int64) error {
	return audit.WithTx(ctx, r.db, audit.Entry{
		EntityType: "shift", EntityID: id, Action: "delete",
	}, func(tx *sql.Tx) error {
		if err := gen.New(tx).DeleteShift(ctx, gen.DeleteShiftParams{TenantID: tenantID, ID: id}); err != nil {
			return fmt.Errorf("delete: %w", err)
		}
		return nil
	})
}

// UnbilledByParticipant aggregates the tenant's recorded-but-unbilled shifts per
// participant (count and service-date span), ready for billing suggestions.
func (r *ShiftsRepo) UnbilledByParticipant(ctx context.Context, tenantID int64) ([]UnbilledAgg, error) {
	if tenantID == 0 {
		return nil, errors.New("unbilled by participant: tenant id required")
	}
	rows, err := gen.New(r.db).ParticipantUnbilledAgg(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("unbilled by participant: %w", err)
	}
	out := make([]UnbilledAgg, 0, len(rows))
	for i := range rows { // bounded by len(rows)
		out = append(out, UnbilledAgg{
			ParticipantID: rows[i].ParticipantID,
			Count:         rows[i].Cnt,
			From:          anyToString(rows[i].FromDate),
			To:            anyToString(rows[i].ToDate),
		})
	}
	return out, nil
}

// assertShiftNonNegative rejects negative quantities at the boundary.
func assertShiftNonNegative(in ShiftInput) error {
	if in.Hours < 0 {
		return errors.New("shift: hours must not be negative")
	}
	if in.Km < 0 {
		return errors.New("shift: km must not be negative")
	}
	return nil
}

// encodeShiftJSON marshals measures and tags to JSON TEXT, defaulting nil
// slices to empty arrays so the columns are never NULL/"null".
func encodeShiftJSON(in ShiftInput) (measures string, tags string, err error) {
	measureVals := in.Measures
	if measureVals == nil {
		measureVals = []Measure{}
	}
	mb, err := json.Marshal(measureVals)
	if err != nil {
		return "", "", fmt.Errorf("shift: marshal measures: %w", err)
	}
	tagVals := in.Tags
	if tagVals == nil {
		tagVals = []string{}
	}
	tb, err := json.Marshal(tagVals)
	if err != nil {
		return "", "", fmt.Errorf("shift: marshal tags: %w", err)
	}
	return string(mb), string(tb), nil
}

// anyToString coerces a SQLite MIN/MAX aggregate (scanned as interface{}) into a
// string. Empty groups would not appear (GROUP BY), so a nil yields "".
func anyToString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case []byte:
		return string(t)
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", t)
	}
}

func toShift(r gen.Shift) (*Shift, error) {
	measures := []Measure{}
	if r.Measures != "" {
		if err := json.Unmarshal([]byte(r.Measures), &measures); err != nil {
			return nil, fmt.Errorf("shift %d: unmarshal measures: %w", r.ID, err)
		}
		if measures == nil {
			measures = []Measure{}
		}
	}
	tags := []string{}
	if r.Tags != "" {
		if err := json.Unmarshal([]byte(r.Tags), &tags); err != nil {
			return nil, fmt.Errorf("shift %d: unmarshal tags: %w", r.ID, err)
		}
		if tags == nil {
			tags = []string{}
		}
	}
	return &Shift{
		ID:            r.ID,
		UUID:          r.Uuid,
		ParticipantID: r.ParticipantID,
		ServiceDate:   r.ServiceDate,
		StartTime:     r.StartTime,
		EndTime:       r.EndTime,
		Hours:         r.Hours,
		Km:            r.Km,
		Measures:      measures,
		Note:          r.Note,
		Tags:          tags,
		Status:        r.Status,
		InvoiceID:     db.PtrID(r.InvoiceID),
		AuthorUserID:  db.PtrID(r.AuthorUserID),
		CreatedAt:     r.CreatedAt,
		UpdatedAt:     r.UpdatedAt,
	}, nil
}

func toShifts(rows []gen.Shift) ([]*Shift, error) {
	out := make([]*Shift, 0, len(rows))
	for i := range rows { // bounded by len(rows)
		s, err := toShift(rows[i])
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

// validISODate reports whether s is a strict YYYY-MM-DD calendar date.
func validISODate(s string) bool {
	if len(s) != 10 {
		return false
	}
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}
