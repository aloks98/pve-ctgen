package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// DB wraps the SQLite database connection.
type DB struct {
	*sql.DB
}

// Open opens (or creates) the SQLite database and runs migrations.
func Open(dbPath string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}

	sqlDB, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(wal)&_pragma=foreign_keys(on)")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if err := sqlDB.Ping(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	db := &DB{sqlDB}
	if err := db.migrate(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return db, nil
}

func (db *DB) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS cloudinit_configs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE,
		content TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS templates (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		vm_id INTEGER NOT NULL,
		name TEXT NOT NULL UNIQUE,
		url TEXT NOT NULL,
		checksum_url TEXT DEFAULT '',
		tags TEXT DEFAULT '',
		cloudinit_id INTEGER,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (cloudinit_id) REFERENCES cloudinit_configs(id)
	);

	CREATE TABLE IF NOT EXISTS build_steps (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		command TEXT NOT NULL,
		sort_order INTEGER NOT NULL,
		is_default BOOLEAN DEFAULT 1,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS nodes (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE,
		display_name TEXT DEFAULT '',
		address TEXT NOT NULL,
		api_key TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS builds (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		build_id TEXT NOT NULL UNIQUE,
		template_id INTEGER NOT NULL,
		node_id INTEGER NOT NULL,
		status TEXT NOT NULL DEFAULT 'pending',
		started_at DATETIME,
		completed_at DATETIME,
		FOREIGN KEY (template_id) REFERENCES templates(id),
		FOREIGN KEY (node_id) REFERENCES nodes(id)
	);

	CREATE TABLE IF NOT EXISTS build_step_results (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		build_id TEXT NOT NULL,
		step_name TEXT NOT NULL,
		step_index INTEGER NOT NULL,
		status TEXT NOT NULL DEFAULT 'pending',
		log TEXT DEFAULT '',
		started_at DATETIME,
		completed_at DATETIME,
		FOREIGN KEY (build_id) REFERENCES builds(build_id)
	);
	`
	_, err := db.Exec(schema)
	return err
}
