package repository

import (
	"context"
	"database/sql"
	"fmt"
)

// StateRepository defines a simple key-value store for persistent state.
type StateRepository interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string) error
	Delete(ctx context.Context, key string) error
}

// SQLiteStateRepository implements StateRepository using SQLite.
type SQLiteStateRepository struct {
	db *sql.DB
}

// NewSQLiteStateRepository creates a new SQLite state repository.
func NewSQLiteStateRepository(db *sql.DB) *SQLiteStateRepository {
	return &SQLiteStateRepository{db: db}
}

// Get returns the value for key, or "" if the key is not present.
func (r *SQLiteStateRepository) Get(ctx context.Context, key string) (string, error) {
	var value string
	err := r.db.QueryRowContext(ctx, `SELECT value FROM state WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("getting state %q: %w", key, err)
	}
	return value, nil
}

// Set upserts the value for key.
func (r *SQLiteStateRepository) Set(ctx context.Context, key, value string) error {
	query := `INSERT INTO state (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`
	_, err := r.db.ExecContext(ctx, query, key, value)
	if err != nil {
		return fmt.Errorf("setting state %q: %w", key, err)
	}
	return nil
}

// Delete removes the value for key.
func (r *SQLiteStateRepository) Delete(ctx context.Context, key string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM state WHERE key = ?`, key)
	if err != nil {
		return fmt.Errorf("deleting state %q: %w", key, err)
	}
	return nil
}
