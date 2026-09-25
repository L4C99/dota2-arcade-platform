package store

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

const migrationLockID int64 = 60566257581001

// ApplyMigrations serializes schema changes and refuses a changed or unknown migration.
func (s *Store) ApplyMigrations(ctx context.Context) error {
	conn, err := s.Pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", migrationLockID); err != nil {
		return err
	}
	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = conn.Exec(unlockCtx, "SELECT pg_advisory_unlock($1)", migrationLockID)
	}()
	if _, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
        version integer PRIMARY KEY,
        checksum text NOT NULL,
        applied_at timestamptz NOT NULL DEFAULT now()
    )`); err != nil {
		return fmt.Errorf("migration ledger: %w", err)
	}
	rows, err := conn.Query(ctx, "SELECT version, checksum FROM schema_migrations")
	if err != nil {
		return err
	}
	applied := make(map[int]string)
	for rows.Next() {
		var version int
		var checksum string
		if err := rows.Scan(&version, &checksum); err != nil {
			rows.Close()
			return err
		}
		applied[version] = checksum
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	names, err := fs.Glob(migrationFiles, "migrations/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(names)
	seen := make(map[int]bool)
	for _, name := range names {
		prefix, _, ok := strings.Cut(path.Base(name), "_")
		if !ok {
			return fmt.Errorf("invalid migration name %q", name)
		}
		version, err := strconv.Atoi(prefix)
		if err != nil || version <= 0 || seen[version] {
			return fmt.Errorf("invalid or duplicate migration version %q", name)
		}
		seen[version] = true
	}
	for version := range applied {
		if !seen[version] {
			return fmt.Errorf("database has unknown migration %d", version)
		}
	}
	for _, name := range names {
		prefix, _, ok := strings.Cut(path.Base(name), "_")
		if !ok {
			return fmt.Errorf("invalid migration name %q", name)
		}
		version, err := strconv.Atoi(prefix)
		if err != nil || version <= 0 {
			return fmt.Errorf("invalid migration version %q", name)
		}
		content, err := migrationFiles.ReadFile(name)
		if err != nil {
			return err
		}
		digest := sha256.Sum256(content)
		checksum := hex.EncodeToString(digest[:])
		if old, ok := applied[version]; ok {
			if old != checksum {
				return fmt.Errorf("migration %d checksum changed", version)
			}
			continue
		}
		tx, err := conn.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, string(content), pgx.QueryExecModeSimpleProtocol); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("apply migration %d: %w", version, err)
		}
		if version == 9 {
			if err := backfillUserDisplayNames(ctx, tx); err != nil {
				_ = tx.Rollback(ctx)
				return fmt.Errorf("backfill migration 9: %w", err)
			}
		}
		if _, err := tx.Exec(ctx, "INSERT INTO schema_migrations(version, checksum) VALUES($1,$2)", version, checksum); err != nil {
			_ = tx.Rollback(ctx)
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
	}
	return nil
}
