package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/dknathalage/tallyo/internal/auth"
	appdb "github.com/dknathalage/tallyo/internal/db"
	httpapi "github.com/dknathalage/tallyo/internal/http"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/service"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	port := flag.Int("port", 8080, "HTTP listen port")
	dataDir := flag.String("data-dir", "", "data directory for the SQLite database (default: OS app data dir)")
	secureCookie := flag.Bool("secure-cookie", false, "mark the session cookie Secure (HTTPS only)")
	flag.Parse()

	dir := *dataDir
	if dir == "" {
		d, err := appdb.DataDir()
		if err != nil {
			return fmt.Errorf("data dir: %w", err)
		}
		dir = d
	}

	conn, err := appdb.Open(filepath.Join(dir, "tallyo-go.db"))
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer func() {
		if cerr := conn.Close(); cerr != nil {
			log.Printf("close db: %v", cerr)
		}
	}()

	if err := appdb.Migrate(conn); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	hub := realtime.NewHub()
	sm := auth.NewSessionManager(conn, *secureCookie)
	users := auth.NewUsers(conn)
	invites := auth.NewInvites(conn)
	bpSvc := service.NewBusinessProfileService(conn, hub)

	setup, err := httpapi.NewSetupHandler(users)
	if err != nil {
		return fmt.Errorf("setup handler: %w", err)
	}

	deps := httpapi.Deps{
		// T18 will replace this with the embedded web.Build sub-FS. For now the
		// SPA build dir is served directly; it exists (has .gitkeep) so the
		// server boots and the /api routes work.
		Assets:          os.DirFS("web/build"),
		Users:           users,
		Session:         sm,
		Setup:           setup,
		Auth:            httpapi.NewAuthHandler(sm, users),
		Invites:         httpapi.NewInviteHandler(invites, users),
		Events:          httpapi.NewEventsHandler(hub),
		BusinessProfile: httpapi.NewBusinessProfileHandler(bpSvc),
	}

	server := httpapi.NewServer(deps)
	srv := &http.Server{Addr: fmt.Sprintf(":%d", *port), Handler: server.Router}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("listening on :%d", *port)
		if serveErr := srv.ListenAndServe(); serveErr != nil && serveErr != http.ErrServerClosed {
			errCh <- serveErr
			return
		}
		errCh <- nil
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("serve: %w", err)
		}
		return nil
	case <-stop:
		log.Println("shutting down")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			return fmt.Errorf("shutdown: %w", err)
		}
		return nil
	}
}
