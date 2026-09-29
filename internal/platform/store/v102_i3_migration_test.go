package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestI3Migration21To22AndRerun(t *testing.T) {
	dsn := os.Getenv("PLATFORM_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("disposable PostgreSQL unavailable")
	}
	ctx := context.Background()
	base, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := NewID()
	schema := "i3upgrade_" + strings.ReplaceAll(id, "-", "")
	if _, err := base.Exec(ctx, `CREATE SCHEMA "`+schema+`"`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = base.Exec(ctx, `DROP SCHEMA "`+schema+`" CASCADE`); base.Close() })
	conf, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	conf.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, conf)
	if err != nil {
		t.Fatal(err)
	}
	s := &Store{Pool: pool}
	t.Cleanup(s.Close)
	if _, err := pool.Exec(ctx, `CREATE TABLE schema_migrations(version integer PRIMARY KEY,checksum text NOT NULL,applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		t.Fatal(err)
	}
	names, err := fs.Glob(migrationFiles, "migrations/*.sql")
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(names)
	for version, name := range names[:21] {
		data, err := migrationFiles.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, string(data), pgx.QueryExecModeSimpleProtocol); err != nil {
			t.Fatalf("migration %d: %v", version+1, err)
		}
		sum := sha256.Sum256(data)
		if _, err := pool.Exec(ctx, `INSERT INTO schema_migrations(version,checksum) VALUES($1,$2)`, version+1, hex.EncodeToString(sum[:])); err != nil {
			t.Fatal(err)
		}
	}
	admin, err := s.CreateAdmin(ctx, "i3-upgrade-admin", "test-only-long-password")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := s.ApplyMigrations(ctx); err != nil {
			t.Fatal(err)
		}
	}
	var latest, count int
	if err := pool.QueryRow(ctx, `SELECT max(version),count(*) FROM schema_migrations`).Scan(&latest, &count); err != nil || latest != 22 || count != 22 {
		t.Fatalf("migration ledger %d/%d: %v", latest, count, err)
	}
	result, _ := NewID()
	if _, err := pool.Exec(ctx, `INSERT INTO admin_i3_requests(action,request_id,payload_sha256,result_id,actor_admin_user_id)
		VALUES('validation.start','upgrade-proof',$1,$2,$3)`, strings.Repeat("a", 64), result, admin); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE admin_i3_requests SET result_id=$1 WHERE request_id='upgrade-proof'`, mustUpgradeID(t)); err == nil {
		t.Fatal("I3 request ledger changed after success")
	}
}
