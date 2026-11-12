// Command migrate applies the SQL migrations and loads the reference
// catalog of Annex A controls.
//
// It connects with the administrator role given by DATABASE_MIGRATE_URL
// (falling back to DATABASE_URL). The API itself must use a different,
// non-superuser role so that Row-Level Security is enforced.
//
// Usage:
//
//	DATABASE_MIGRATE_URL=postgres://sgsi:...@localhost:5432/sgsi?sslmode=disable migrate
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jsortizq/politicas-iso27001-pyme/internal/config"
	"github.com/jsortizq/politicas-iso27001-pyme/internal/database"
	"github.com/jsortizq/politicas-iso27001-pyme/migrations"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "migrate:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg := config.Load()
	dsn := cfg.Database.MigrateURL
	if dsn == "" {
		dsn = cfg.Database.URL
	}
	if dsn == "" {
		return fmt.Errorf("set DATABASE_MIGRATE_URL (administrator role) or DATABASE_URL")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	db, err := database.Open(ctx, dsn, 2)
	if err != nil {
		return err
	}
	defer db.Close()

	applied, err := database.Migrate(ctx, db, migrations.FS)
	if err != nil {
		return err
	}
	if len(applied) == 0 {
		fmt.Println("migraciones: nada pendiente")
	}
	for _, v := range applied {
		fmt.Println("migración aplicada:", v)
	}

	n, err := database.SeedCatalog(ctx, db)
	if err != nil {
		return err
	}
	fmt.Printf("catálogo: %d controles del Anexo A cargados\n", n)
	return nil
}
