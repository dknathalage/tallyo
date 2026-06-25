package session

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/dknathalage/tallyo/internal/db"
	"github.com/dknathalage/tallyo/internal/db/gen"
)

// ListClient returns a client's sessions. When both from and to are
// non-empty it restricts to service_date ∈ [from, to]; otherwise it returns all.
func (r *SessionsRepo) ListClient(ctx context.Context, tenantID, clientID string, from, to string) ([]*Session, error) {
	if tenantID == "" || clientID == "" {
		return nil, errors.New("list sessions: tenant and client id required")
	}
	q := gen.New(r.db)
	if from != "" && to != "" {
		rows, err := q.ListSessionsByClientRange(ctx, gen.ListSessionsByClientRangeParams{
			TenantID: tenantID, ClientID: clientID, ServiceDate: from, ServiceDate_2: to,
		})
		if err != nil {
			return nil, fmt.Errorf("list client sessions range: %w", err)
		}
		return mapSessions(rows, sessionFieldsFromByPartRange)
	}
	rows, err := q.ListSessionsByClient(ctx, gen.ListSessionsByClientParams{
		TenantID: tenantID, ClientID: clientID,
	})
	if err != nil {
		return nil, fmt.Errorf("list client sessions: %w", err)
	}
	return mapSessions(rows, sessionFieldsFromByPart)
}

// List returns all of the tenant's sessions (newest service date first).
func (r *SessionsRepo) List(ctx context.Context, tenantID string) ([]*Session, error) {
	if tenantID == "" {
		return nil, errors.New("list sessions: tenant id required")
	}
	rows, err := gen.New(r.db).ListSessions(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	return mapSessions(rows, sessionFieldsFromList)
}

// ListByStatus returns the tenant's sessions in a given lifecycle status.
func (r *SessionsRepo) ListByStatus(ctx context.Context, tenantID string, status string) ([]*Session, error) {
	if tenantID == "" {
		return nil, errors.New("list sessions by status: tenant id required")
	}
	rows, err := gen.New(r.db).ListSessionsByStatus(ctx, gen.ListSessionsByStatusParams{TenantID: tenantID, Status: status})
	if err != nil {
		return nil, fmt.Errorf("list sessions by status: %w", err)
	}
	return mapSessions(rows, sessionFieldsFromByStatus)
}

// ListScheduled returns the tenant's scheduled (not yet recorded) sessions.
func (r *SessionsRepo) ListScheduled(ctx context.Context, tenantID string) ([]*Session, error) {
	if tenantID == "" {
		return nil, errors.New("list scheduled sessions: tenant id required")
	}
	rows, err := gen.New(r.db).ListScheduledSessions(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list scheduled sessions: %w", err)
	}
	return mapSessions(rows, sessionFieldsFromScheduled)
}

// ListRecordedUnbilled returns a client's recorded sessions that are not yet
// linked to an invoice (status 'recorded', invoice_id NULL).
func (r *SessionsRepo) ListRecordedUnbilled(ctx context.Context, tenantID, clientID string) ([]*Session, error) {
	if tenantID == "" || clientID == "" {
		return nil, errors.New("list recorded unbilled: tenant and client id required")
	}
	rows, err := gen.New(r.db).ListRecordedUnbilledByClient(ctx, gen.ListRecordedUnbilledByClientParams{
		TenantID: tenantID, ClientID: clientID,
	})
	if err != nil {
		return nil, fmt.Errorf("list recorded unbilled: %w", err)
	}
	return mapSessions(rows, sessionFieldsFromRecorded)
}

// UnbilledByClient aggregates the tenant's recorded-but-unbilled sessions per
// client (count and service-date span), ready for billing suggestions.
func (r *SessionsRepo) UnbilledByClient(ctx context.Context, tenantID string) ([]UnbilledAgg, error) {
	if tenantID == "" {
		return nil, errors.New("unbilled by client: tenant id required")
	}
	rows, err := gen.New(r.db).ClientUnbilledAgg(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("unbilled by client: %w", err)
	}
	out := make([]UnbilledAgg, 0, len(rows))
	for i := range rows { // bounded by len(rows)
		out = append(out, UnbilledAgg{
			ClientID: rows[i].ClientID,
			Count:    rows[i].Cnt,
			From:     anyToString(rows[i].FromDate),
			To:       anyToString(rows[i].ToDate),
		})
	}
	return out, nil
}

// encodeTags marshals tags to JSON TEXT, defaulting a nil slice to an empty
// array so the column is never NULL/"null".
func encodeTags(tags []string) (string, error) {
	if tags == nil {
		tags = []string{}
	}
	tb, err := json.Marshal(tags)
	if err != nil {
		return "", fmt.Errorf("session: marshal tags: %w", err)
	}
	return string(tb), nil
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

// validISODate reports whether s is a strict YYYY-MM-DD calendar date.
func validISODate(s string) bool {
	if len(s) != 10 {
		return false
	}
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

// sessionFields is the common projection shared by every enriched session read row
// (Get/GetByID/List*). The gen row types are nominally distinct but identical in
// shape; each is adapted into this struct so a single mapper (mapSession) builds
// the DTO. ClientUUID is the joined clients.uuid (NULL if the FK is
// dangling, which the clean schema forbids).
type sessionFields struct {
	id           string
	clientID     string
	clientUUID   sql.NullString
	serviceDate  string
	note         string
	tags         string
	status       string
	invoiceID    sql.NullString
	invoiceUUID  sql.NullString
	authorUserID sql.NullString
	createdAt    string
	updatedAt    string
}

func mapSession(f sessionFields) (*Session, error) {
	tags := []string{}
	if f.tags != "" {
		if err := json.Unmarshal([]byte(f.tags), &tags); err != nil {
			return nil, fmt.Errorf("session %s: unmarshal tags: %w", f.id, err)
		}
		if tags == nil {
			tags = []string{}
		}
	}
	return &Session{
		ID:           f.id,
		ClientID:     f.clientID,
		ClientUUID:   f.clientUUID.String,
		ServiceDate:  f.serviceDate,
		Note:         f.note,
		Tags:         tags,
		Status:       f.status,
		InvoiceID:    db.PtrStr(f.invoiceID),
		InvoiceUUID:  db.PtrStr(f.invoiceUUID),
		AuthorUserID: db.PtrStr(f.authorUserID),
		CreatedAt:    f.createdAt,
		UpdatedAt:    f.updatedAt,
	}, nil
}

// sessionFieldsFromGet / *FromByID / *FromList* adapt the (nominally distinct but
// identically shaped) enriched gen row types into the common sessionFields. They
// keep the per-query row scanners while a single mapper builds the DTO.
func sessionFieldsFromGet(r gen.GetSessionRow) sessionFields {
	return sessionFields{
		id: r.ID, clientID: r.ClientID, clientUUID: r.ClientUuid,
		serviceDate: r.ServiceDate, note: r.Note, tags: r.Tags, status: r.Status,
		invoiceID: r.InvoiceID, invoiceUUID: r.InvoiceUuid, authorUserID: r.AuthorUserID, createdAt: r.CreatedAt, updatedAt: r.UpdatedAt,
	}
}

func sessionFieldsFromByID(r gen.GetSessionByIDRow) sessionFields {
	return sessionFields{
		id: r.ID, clientID: r.ClientID, clientUUID: r.ClientUuid,
		serviceDate: r.ServiceDate, note: r.Note, tags: r.Tags, status: r.Status,
		invoiceID: r.InvoiceID, invoiceUUID: r.InvoiceUuid, authorUserID: r.AuthorUserID, createdAt: r.CreatedAt, updatedAt: r.UpdatedAt,
	}
}

func sessionFieldsFromList(r gen.ListSessionsRow) sessionFields {
	return sessionFields{
		id: r.ID, clientID: r.ClientID, clientUUID: r.ClientUuid,
		serviceDate: r.ServiceDate, note: r.Note, tags: r.Tags, status: r.Status,
		invoiceID: r.InvoiceID, invoiceUUID: r.InvoiceUuid, authorUserID: r.AuthorUserID, createdAt: r.CreatedAt, updatedAt: r.UpdatedAt,
	}
}

func sessionFieldsFromByPart(r gen.ListSessionsByClientRow) sessionFields {
	return sessionFields{
		id: r.ID, clientID: r.ClientID, clientUUID: r.ClientUuid,
		serviceDate: r.ServiceDate, note: r.Note, tags: r.Tags, status: r.Status,
		invoiceID: r.InvoiceID, invoiceUUID: r.InvoiceUuid, authorUserID: r.AuthorUserID, createdAt: r.CreatedAt, updatedAt: r.UpdatedAt,
	}
}

func sessionFieldsFromByPartRange(r gen.ListSessionsByClientRangeRow) sessionFields {
	return sessionFields{
		id: r.ID, clientID: r.ClientID, clientUUID: r.ClientUuid,
		serviceDate: r.ServiceDate, note: r.Note, tags: r.Tags, status: r.Status,
		invoiceID: r.InvoiceID, invoiceUUID: r.InvoiceUuid, authorUserID: r.AuthorUserID, createdAt: r.CreatedAt, updatedAt: r.UpdatedAt,
	}
}

func sessionFieldsFromByStatus(r gen.ListSessionsByStatusRow) sessionFields {
	return sessionFields{
		id: r.ID, clientID: r.ClientID, clientUUID: r.ClientUuid,
		serviceDate: r.ServiceDate, note: r.Note, tags: r.Tags, status: r.Status,
		invoiceID: r.InvoiceID, invoiceUUID: r.InvoiceUuid, authorUserID: r.AuthorUserID, createdAt: r.CreatedAt, updatedAt: r.UpdatedAt,
	}
}

func sessionFieldsFromScheduled(r gen.ListScheduledSessionsRow) sessionFields {
	return sessionFields{
		id: r.ID, clientID: r.ClientID, clientUUID: r.ClientUuid,
		serviceDate: r.ServiceDate, note: r.Note, tags: r.Tags, status: r.Status,
		invoiceID: r.InvoiceID, invoiceUUID: r.InvoiceUuid, authorUserID: r.AuthorUserID, createdAt: r.CreatedAt, updatedAt: r.UpdatedAt,
	}
}

func sessionFieldsFromRecorded(r gen.ListRecordedUnbilledByClientRow) sessionFields {
	return sessionFields{
		id: r.ID, clientID: r.ClientID, clientUUID: r.ClientUuid,
		serviceDate: r.ServiceDate, note: r.Note, tags: r.Tags, status: r.Status,
		invoiceID: r.InvoiceID, invoiceUUID: r.InvoiceUuid, authorUserID: r.AuthorUserID, createdAt: r.CreatedAt, updatedAt: r.UpdatedAt,
	}
}

// mapSessions maps a slice of enriched gen rows (via an adapter to sessionFields)
// into DTOs. Bounded by len(rows).
func mapSessions[T any](rows []T, adapt func(T) sessionFields) ([]*Session, error) {
	out := make([]*Session, 0, len(rows))
	for i := range rows { // bounded by len(rows)
		s, err := mapSession(adapt(rows[i]))
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}
