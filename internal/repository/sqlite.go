package repository

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// NewSQLiteDB opens a SQLite database at the given path.
//
// Access is serialized through a single connection and a busy timeout is set,
// so concurrent writes from multiple goroutines (e.g. the download engine's
// status/progress updates) wait for the write lock instead of failing
// immediately with SQLITE_BUSY ("database is locked").
func NewSQLiteDB(dbPath string) (*sql.DB, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating database directory: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// SQLite permits a single writer at a time. A single pooled connection
	// serializes all statements within this process; busy_timeout is a backstop
	// that makes writers wait rather than error for cross-process contention.
	db.SetMaxOpenConns(1)

	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("setting busy timeout: %w", err)
	}

	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("setting WAL mode: %w", err)
	}

	// foreign_keys is a connection-specific pragma; this relies on the single
	// pooled connection above to apply it to every statement.
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("enabling foreign keys: %w", err)
	}

	return db, nil
}
