package testutil

import (
	"database/sql"
	"fmt"
	"log/slog"
	"path/filepath"
	"runtime"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/jackc/pgx/v5/stdlib"
)

// migrationsPath resolves the migrations folder relative to THIS file's
// location on disk, not the caller's working directory — so it works
// correctly no matter which package imports RunMigrations.
func migrationsPath() string {
	_, thisFile, _, _ := runtime.Caller(0)
	// this file lives at internal/testutil/migrate.go, so migrations/ is ../../migrations
	dir := filepath.Dir(thisFile)
	return "file://" + filepath.ToSlash(filepath.Join(dir, "..", "..", "migrations"))
}

// RunMigrations applies all migrations to the given database, waiting
// for it to become reachable first (useful right after a testcontainer starts).
func RunMigrations(databaseURL string) error {
	if err := waitForDB(databaseURL, 30*time.Second); err != nil {
		return fmt.Errorf("database never became reachable: %w", err)
	}

	m, err := migrate.New(migrationsPath(), databaseURL)
	if err != nil {
		return fmt.Errorf("failed to init migrate: %w", err)
	}
	defer func() {
		sourceErr, dbErr := m.Close()
		if sourceErr != nil {
			slog.Error("migrate source close error: %v", "db_migration_source_error", sourceErr)
		}
		if dbErr != nil {
			slog.Error("migrate db close error: %v", "database_close_error", dbErr)
		}
	}()

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("failed to run migrations: %w", err)
	}
	return nil
}

func waitForDB(databaseURL string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error

	for time.Now().Before(deadline) {
		db, err := sql.Open("pgx", databaseURL)
		if err == nil {
			pingErr := db.Ping()
			if closeErr := db.Close(); closeErr != nil {
				slog.Error("db close error: %v", "database_close_error", closeErr)
			}
			if pingErr == nil {
				return nil
			}
			lastErr = pingErr
		} else {
			lastErr = err
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for database: %w", lastErr)
}
