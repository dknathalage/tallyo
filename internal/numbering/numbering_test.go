package numbering

import (
	"context"
	"database/sql"
	"path/filepath"
	"sync"
	"testing"

	appdb "github.com/dknathalage/tallyo/internal/db"
)

var testCfg = Config{Table: "doc_test", Column: "number", Prefix: "INV-", Pad: 4}

func setup(t *testing.T) *sql.DB {
	t.Helper()
	conn, err := appdb.Open(filepath.Join(t.TempDir(), "n.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := conn.Exec(`CREATE TABLE doc_test (id INTEGER PRIMARY KEY AUTOINCREMENT, number TEXT NOT NULL UNIQUE)`); err != nil {
		t.Fatalf("create: %v", err)
	}
	return conn
}

func TestNextStartsAtOne(t *testing.T) {
	conn := setup(t)
	defer conn.Close()
	tx, _ := conn.BeginTx(context.Background(), nil)
	defer tx.Rollback()
	n, err := Next(context.Background(), tx, testCfg)
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if n != "INV-0001" {
		t.Fatalf("got %q want INV-0001", n)
	}
}

func TestNextIncrements(t *testing.T) {
	conn := setup(t)
	defer conn.Close()
	ctx := context.Background()
	for _, want := range []string{"INV-0001", "INV-0002", "INV-0003"} {
		tx, _ := conn.BeginTx(ctx, nil)
		n, err := Next(ctx, tx, testCfg)
		if err != nil {
			t.Fatalf("Next: %v", err)
		}
		if n != want {
			t.Fatalf("got %q want %q", n, want)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO doc_test (number) VALUES (?)`, n); err != nil {
			t.Fatalf("insert: %v", err)
		}
		tx.Commit()
	}
}

func TestConcurrentCreateNoCollision(t *testing.T) {
	conn := setup(t)
	defer conn.Close()
	ctx := context.Background()
	const workers = 12
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- WithRetry(ctx, 10, func() error {
				tx, err := conn.BeginTx(ctx, nil)
				if err != nil {
					return err
				}
				defer tx.Rollback()
				n, err := Next(ctx, tx, testCfg)
				if err != nil {
					return err
				}
				if _, err := tx.ExecContext(ctx, `INSERT INTO doc_test (number) VALUES (?)`, n); err != nil {
					return err
				}
				return tx.Commit()
			})
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatalf("worker: %v", e)
		}
	}
	var count int
	conn.QueryRow(`SELECT COUNT(DISTINCT number) FROM doc_test`).Scan(&count)
	if count != workers {
		t.Fatalf("distinct numbers=%d want %d", count, workers)
	}
}
