package migration

import (
	"database/sql"
	"fmt"
	"sort"
)

var migrations = []struct {
	Name string
	SQL  string
}{
	{
		Name: "001_create_downloads",
		SQL: `CREATE TABLE IF NOT EXISTS downloads (
			id TEXT PRIMARY KEY,
			chat_id INTEGER NOT NULL,
			user_id INTEGER NOT NULL,
			url TEXT NOT NULL,
			file_path TEXT DEFAULT '',
			status TEXT NOT NULL DEFAULT 'pending',
			progress REAL NOT NULL DEFAULT 0,
			error TEXT DEFAULT '',
			file_size INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			completed_at DATETIME
		);
		CREATE INDEX IF NOT EXISTS idx_downloads_chat_id ON downloads(chat_id);
		CREATE INDEX IF NOT EXISTS idx_downloads_status ON downloads(status);`,
	},
	{
		Name: "002_create_users",
		SQL: `CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY,
			username TEXT DEFAULT '',
			first_name TEXT DEFAULT '',
			last_name TEXT DEFAULT '',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
	},
	{
		Name: "003_create_chats",
		SQL: `CREATE TABLE IF NOT EXISTS chats (
			id INTEGER PRIMARY KEY,
			title TEXT DEFAULT '',
			type TEXT DEFAULT 'private',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
	},
	{
		Name: "004_create_schema_migrations",
		SQL: `CREATE TABLE IF NOT EXISTS schema_migrations (
			name TEXT PRIMARY KEY,
			applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
	},
	{
		Name: "005_add_status_message_id_and_state",
		SQL: `ALTER TABLE downloads ADD COLUMN status_message_id INTEGER NOT NULL DEFAULT 0;
		CREATE TABLE IF NOT EXISTS state (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);`,
	},
}

// Run applies all pending database migrations.
func Run(db *sql.DB) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		name TEXT PRIMARY KEY,
		applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		return fmt.Errorf("creating migrations table: %w", err)
	}

	for _, m := range sortedMigrations() {
		var count int
		if err := db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE name = ?", m.Name).Scan(&count); err != nil {
			return fmt.Errorf("checking migration %s: %w", m.Name, err)
		}

		if count > 0 {
			continue
		}

		if _, err := db.Exec(m.SQL); err != nil {
			return fmt.Errorf("applying migration %s: %w", m.Name, err)
		}

		if _, err := db.Exec("INSERT INTO schema_migrations (name) VALUES (?)", m.Name); err != nil {
			return fmt.Errorf("recording migration %s: %w", m.Name, err)
		}
	}

	return nil
}

func sortedMigrations() []struct {
	Name string
	SQL  string
} {
	sorted := make([]struct {
		Name string
		SQL  string
	}, len(migrations))
	copy(sorted, migrations)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Name < sorted[j].Name
	})
	return sorted
}
