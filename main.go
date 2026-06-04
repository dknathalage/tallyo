package main

import (
	"embed"
	"log"
	"path/filepath"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	appdb "github.com/dknathalage/tallyo/internal/db"
	"github.com/dknathalage/tallyo/internal/service"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
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

	app := NewApp(conn)
	bp := service.NewBusinessProfileService(conn)

	err = wails.Run(&options.App{
		Title:       "Tallyo",
		Width:       1200,
		Height:      800,
		AssetServer: &assetserver.Options{Assets: assets},
		OnStartup:   app.startup,
		OnShutdown:  app.shutdown,
		Bind: []any{
			app,
			bp,
		},
	})
	if err != nil {
		log.Fatalf("wails run: %v", err)
	}
}
