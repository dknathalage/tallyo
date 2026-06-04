package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	appdb "github.com/dknathalage/tallyo/internal/db"
)

func main() {
	port := flag.Int("port", 8080, "HTTP listen port")
	flag.Parse()

	dir, err := appdb.DataDir()
	if err != nil {
		log.Fatalf("data dir: %v", err)
	}
	conn, err := appdb.Open(filepath.Join(dir, "tallyo-go.db"))
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	if err := appdb.Migrate(conn); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "ok")
	})
	srv := &http.Server{Addr: fmt.Sprintf(":%d", *port), Handler: mux}

	go func() {
		log.Printf("listening on :%d", *port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("serve: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Println("shutting down")
	_ = srv.Close()
	_ = conn.Close()
}
