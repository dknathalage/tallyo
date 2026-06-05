package main

import (
	"context"
	"flag"
	"fmt"
	"io/fs"
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
	tallyoweb "github.com/dknathalage/tallyo/web"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

// overdueSweepInterval is how often the background sweeper flips due invoices.
const overdueSweepInterval = 1 * time.Hour

// runSweeper flips overdue invoices and generates due recurring invoices on each
// tick until done is closed. It owns its single ticker and stops cleanly, so it
// never leaks a goroutine.
func runSweeper(inv *service.InvoiceService, rec *service.RecurringService, done <-chan struct{}) {
	ticker := time.NewTicker(overdueSweepInterval)
	defer ticker.Stop()
	for { // bounded by the done signal
		select {
		case <-done:
			return
		case <-ticker.C:
			if rows, err := inv.MarkOverdue(context.Background()); err != nil {
				log.Printf("overdue sweep: %v", err)
			} else if len(rows) > 0 {
				log.Printf("overdue sweep: %d invoice(s) flipped", len(rows))
			}
			if gens, err := rec.GenerateDue(context.Background()); err != nil {
				log.Printf("recurring sweep: %v", err)
			} else if len(gens) > 0 {
				log.Printf("recurring sweep: %d invoice(s) generated", len(gens))
			}
		}
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
	rateTierSvc := service.NewRateTierService(conn, hub)
	payerSvc := service.NewPayerService(conn, hub)
	taxRateSvc := service.NewTaxRateService(conn, hub)
	clientSvc := service.NewClientService(conn, hub)
	catalogSvc := service.NewCatalogService(conn, hub)
	invoiceSvc := service.NewInvoiceService(conn, hub)
	estimateSvc := service.NewEstimateService(conn, hub)
	paymentSvc := service.NewPaymentService(conn, hub)
	recurringSvc := service.NewRecurringService(conn, hub)
	columnMappingSvc := service.NewColumnMappingService(conn, hub)

	setup, err := httpapi.NewSetupHandler(users)
	if err != nil {
		return fmt.Errorf("setup handler: %w", err)
	}

	assets, err := fs.Sub(tallyoweb.Build, "build")
	if err != nil {
		return fmt.Errorf("sub web build: %w", err)
	}

	if _, err := fs.Stat(assets, "200.html"); err != nil {
		return fmt.Errorf("embedded SPA missing 200.html — run `npm run build` in web/ before `go build`: %w", err)
	}

	deps := httpapi.Deps{
		Assets:          assets,
		Users:           users,
		Session:         sm,
		Setup:           setup,
		Auth:            httpapi.NewAuthHandler(sm, users),
		Invites:         httpapi.NewInviteHandler(invites, users),
		Events:          httpapi.NewEventsHandler(hub),
		BusinessProfile: httpapi.NewBusinessProfileHandler(bpSvc),
		RateTiers:       httpapi.NewRateTierHandler(rateTierSvc),
		Payers:          httpapi.NewPayerHandler(payerSvc),
		TaxRates:        httpapi.NewTaxRateHandler(taxRateSvc),
		Clients:         httpapi.NewClientHandler(clientSvc),
		Catalog:         httpapi.NewCatalogHandler(catalogSvc),
		Invoices:        httpapi.NewInvoiceHandler(invoiceSvc),
		Estimates:       httpapi.NewEstimateHandler(estimateSvc),
		Payments:        httpapi.NewPaymentHandler(paymentSvc),
		Recurring:       httpapi.NewRecurringHandler(recurringSvc),
		ColumnMappings:  httpapi.NewColumnMappingHandler(columnMappingSvc),
		Export:          httpapi.NewExportHandler(catalogSvc, invoiceSvc, estimateSvc),
	}

	server := httpapi.NewServer(deps)
	srv := &http.Server{Addr: fmt.Sprintf(":%d", *port), Handler: server.Router}

	// Run one overdue sweep at startup, then keep a background sweeper running on
	// an hourly tick. The done channel stops the goroutine on shutdown so it does
	// not leak.
	if rows, err := invoiceSvc.MarkOverdue(context.Background()); err != nil {
		log.Printf("startup overdue sweep: %v", err)
	} else {
		log.Printf("startup overdue sweep: %d invoice(s) flipped", len(rows))
	}
	if gens, err := recurringSvc.GenerateDue(context.Background()); err != nil {
		log.Printf("startup recurring sweep: %v", err)
	} else {
		log.Printf("startup recurring sweep: %d invoice(s) generated", len(gens))
	}
	overdueDone := make(chan struct{})
	go runSweeper(invoiceSvc, recurringSvc, overdueDone)
	defer close(overdueDone)

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
