package main

import (
	"context"
	"database/sql"
)

type App struct {
	ctx context.Context
	db  *sql.DB
}

func NewApp(db *sql.DB) *App { return &App{db: db} }

func (a *App) startup(ctx context.Context)  { a.ctx = ctx }
func (a *App) shutdown(ctx context.Context) { _ = a.db.Close() }
