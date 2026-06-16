package agent

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/dknathalage/tallyo/internal/db/gen"
)

// ErrConflict signals a restore target changed since the checkpoint captured it.
var ErrConflict = errors.New("agent: checkpoint revert conflict")

// isConflict reports whether err is (or wraps) ErrConflict.
func isConflict(err error) bool { return errors.Is(err, ErrConflict) }

// checkpointKey is the unexported context key for the active checkpoint id.
type checkpointKey struct{}

// withCheckpoint returns a context carrying the active checkpoint id. The
// execute loop sets it before running a risky tool so the tool's Record call
// (and the gate) can find it.
func withCheckpoint(ctx context.Context, id int64) context.Context {
	return context.WithValue(ctx, checkpointKey{}, id)
}

// checkpointFrom returns the active checkpoint id from ctx and whether one is
// set. A zero id is treated as absent.
func checkpointFrom(ctx context.Context) (int64, bool) {
	v, ok := ctx.Value(checkpointKey{}).(int64)
	if !ok || v == 0 {
		return 0, false
	}
	return v, true
}

// Change is one recorded mutation within a checkpoint.
type Change struct {
	Table         string
	PK            int64
	Op            string // "create" | "update"
	BeforeRow     json.RawMessage
	AfterRow      json.RawMessage
	EntityVersion string
}

// Conflict identifies a change that was skipped during revert because the live
// row's version no longer matched the captured version.
type Conflict struct {
	Table string
	PK    int64
}

// RestoreFunc applies the inverse of one change via the service layer. It must
// return ErrConflict (sentinel) when the live row's version no longer matches
// ch.EntityVersion so Revert can record and skip it.
type RestoreFunc func(ctx context.Context, ch Change) error

// Checkpoint records mutations a risky tool makes so a turn can be reverted.
// Each Record is its own audited transaction taken AFTER the service call that
// it describes commits; a checkpoint is therefore a best-effort log, not an
// atomic snapshot of the service writes.
type Checkpoint struct {
	store *Store
	db    *sql.DB
}

// NewCheckpoint constructs a Checkpoint. A nil store or db is a programmer error.
func NewCheckpoint(store *Store, db *sql.DB) *Checkpoint {
	if store == nil || db == nil {
		panic("agent: NewCheckpoint requires a non-nil store and db")
	}
	return &Checkpoint{store: store, db: db}
}

// Open creates an open checkpoint tied to the assistant message and returns its
// id. A zero message id is a programmer error.
func (c *Checkpoint) Open(ctx context.Context, messageID int64) (int64, error) {
	if c.store == nil {
		return 0, fmt.Errorf("checkpoint: nil store")
	}
	if messageID == 0 {
		return 0, fmt.Errorf("checkpoint: zero message id")
	}
	chk, err := c.store.CreateCheckpoint(ctx, messageID, "open")
	if err != nil {
		return 0, fmt.Errorf("checkpoint: open: %w", err)
	}
	return chk.ID, nil
}

// Record persists one change under the checkpoint. It runs in its own audited
// transaction, taken AFTER the service call it describes (see B1): the change
// log is not enrolled in the service's transaction, so a small window exists
// where the service write committed but the change was not yet recorded.
func (c *Checkpoint) Record(ctx context.Context, checkpointID int64, ord int, ch Change) error {
	if checkpointID == 0 || ch.Table == "" {
		return fmt.Errorf("checkpoint: invalid record (checkpoint=%d table=%q)", checkpointID, ch.Table)
	}
	if ch.Op != "create" && ch.Op != "update" {
		return fmt.Errorf("checkpoint: invalid op %q", ch.Op)
	}
	if _, err := c.store.CreateCheckpointChange(ctx, gen.CreateCheckpointChangeParams{
		CheckpointID:  checkpointID,
		Ordinal:       int64(ord),
		TableName:     ch.Table,
		Pk:            ch.PK,
		Op:            ch.Op,
		BeforeRow:     nullRaw(ch.BeforeRow),
		AfterRow:      string(ch.AfterRow),
		EntityVersion: ch.EntityVersion,
	}); err != nil {
		return fmt.Errorf("checkpoint: record: %w", err)
	}
	return nil
}

// Revert restores every change under the checkpoint in reverse-ordinal order
// (the order ListCheckpointChanges returns), skipping rows whose live version no
// longer matches (conflicts). It marks the checkpoint reverted and returns the
// conflicts it skipped. A non-conflict restore error aborts the revert.
func (c *Checkpoint) Revert(ctx context.Context, checkpointID int64, restore RestoreFunc) ([]Conflict, error) {
	if checkpointID == 0 {
		return nil, fmt.Errorf("checkpoint: zero checkpoint id")
	}
	if restore == nil {
		return nil, fmt.Errorf("checkpoint: nil restore func")
	}
	rows, err := c.store.ListCheckpointChanges(ctx, checkpointID)
	if err != nil {
		return nil, fmt.Errorf("checkpoint: revert: %w", err)
	}

	conflicts := make([]Conflict, 0, len(rows))
	for i := range rows { // bounded by len(rows), already ordinal DESC
		ch := toChange(rows[i])
		if e := restore(ctx, ch); e != nil {
			if isConflict(e) {
				conflicts = append(conflicts, Conflict{Table: ch.Table, PK: ch.PK})
				continue
			}
			return nil, fmt.Errorf("checkpoint: revert change pk=%d: %w", ch.PK, e)
		}
	}

	if err := c.store.MarkCheckpointReverted(ctx, checkpointID); err != nil {
		return nil, fmt.Errorf("checkpoint: mark reverted: %w", err)
	}
	return conflicts, nil
}

// toChange maps a generated checkpoint-change row to the domain Change shape.
func toChange(r gen.AgentCheckpointChange) Change {
	var before json.RawMessage
	if r.BeforeRow.Valid && r.BeforeRow.String != "" {
		before = json.RawMessage(r.BeforeRow.String)
	}
	return Change{
		Table:         r.TableName,
		PK:            r.Pk,
		Op:            r.Op,
		BeforeRow:     before,
		AfterRow:      json.RawMessage(r.AfterRow),
		EntityVersion: r.EntityVersion,
	}
}

// nullRaw wraps a raw-JSON before-row as a nullable string column (NULL when
// empty so the create-op shape is uniform).
func nullRaw(raw json.RawMessage) sql.NullString {
	if len(raw) == 0 {
		return sql.NullString{}
	}
	return sql.NullString{String: string(raw), Valid: true}
}
