package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/Goodmoder-ux/collect-bot/migrations"
	_ "modernc.org/sqlite"
)

// Open opens the SQLite database at path and applies the embedded migrations.
func Open(ctx context.Context, path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil && filepath.Dir(path) != "." {
		return nil, fmt.Errorf("sqlite: create database directory: %w", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("sqlite: open database: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqlite: ping database: %w", err)
	}
	if _, err := db.ExecContext(ctx, `PRAGMA foreign_keys = ON; PRAGMA busy_timeout = 5000; PRAGMA journal_mode = WAL; PRAGMA synchronous = NORMAL;`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqlite: configure database: %w", err)
	}
	if err := applyMigrations(ctx, db, migrations.FS); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

// Backup writes a consistent copy of the database to dest while it stays online.
func Backup(ctx context.Context, db *sql.DB, dest string) error {
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("sqlite: backup destination %s already exists", dest)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o750); err != nil {
		return fmt.Errorf("sqlite: create backup directory: %w", err)
	}
	if _, err := db.ExecContext(ctx, `VACUUM INTO ?`, dest); err != nil {
		return fmt.Errorf("sqlite: backup to %s: %w", dest, err)
	}
	return nil
}

func applyMigrations(ctx context.Context, db *sql.DB, fsys fs.FS) error {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return fmt.Errorf("sqlite: read migrations: %w", err)
	}
	var files []string
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".sql" {
			files = append(files, entry.Name())
		}
	}
	sort.Strings(files)
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version TEXT PRIMARY KEY)`); err != nil {
		return fmt.Errorf("sqlite: create migration table: %w", err)
	}
	for _, name := range files {
		var applied bool
		if err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = ?)`, name).Scan(&applied); err != nil {
			return fmt.Errorf("sqlite: check migration %s: %w", name, err)
		}
		if applied {
			continue
		}
		content, err := fs.ReadFile(fsys, name)
		if err != nil {
			return fmt.Errorf("sqlite: read migration %s: %w", name, err)
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("sqlite: begin migration %s: %w", name, err)
		}
		if _, err = tx.ExecContext(ctx, string(content)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("sqlite: apply migration %s: %w", name, err)
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO schema_migrations(version) VALUES (?)`, name); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("sqlite: record migration %s: %w", name, err)
		}
		if err = tx.Commit(); err != nil {
			return fmt.Errorf("sqlite: commit migration %s: %w", name, err)
		}
	}
	return nil
}
