package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestDisplayNameUpgradePreservesPartyAndSession(t *testing.T) {
	dsn := os.Getenv("PLATFORM_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("PLATFORM_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	base, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer base.Close()
	id, err := NewID()
	if err != nil {
		t.Fatal(err)
	}
	schema := "name_" + strings.ReplaceAll(id, "-", "")
	if _, err := base.Exec(ctx, `CREATE SCHEMA "`+schema+`"`); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = base.Exec(ctx, `DROP SCHEMA "`+schema+`" CASCADE`) }()
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	s := &Store{Pool: pool}
	if _, err := pool.Exec(ctx, `CREATE TABLE schema_migrations(version integer PRIMARY KEY, checksum text NOT NULL, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		t.Fatal(err)
	}
	for version := 1; version <= 8; version++ {
		matches, err := migrationFiles.ReadDir("migrations")
		if err != nil {
			t.Fatal(err)
		}
		var filename string
		for _, item := range matches {
			if strings.HasPrefix(item.Name(), fmt.Sprintf("%04d_", version)) {
				filename = item.Name()
				break
			}
		}
		if filename == "" {
			t.Fatalf("missing migration %d", version)
		}
		content, err := migrationFiles.ReadFile("migrations/" + filename)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, string(content), pgx.QueryExecModeSimpleProtocol); err != nil {
			t.Fatalf("migration %d: %v", version, err)
		}
		checksum := sha256.Sum256(content)
		if _, err := pool.Exec(ctx, `INSERT INTO schema_migrations(version,checksum) VALUES($1,$2)`, version, hex.EncodeToString(checksum[:])); err != nil {
			t.Fatal(err)
		}
	}
	leader, _ := NewID()
	member, _ := NewID()
	session, _ := NewID()
	party, _ := NewID()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `INSERT INTO users(id) VALUES($1),($2)`, leader, member); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO user_sessions(id,user_id,token_hash,expires_at) VALUES($1,$2,decode(repeat('00',32),'hex'),now()+interval '1 day')`, session, leader); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO parties(id,leader_user_id) VALUES($1,$2)`, party, leader); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO party_members(party_id,user_id,role) VALUES($1,$2,'leader'),($1,$3,'member')`, party, leader, member); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyMigrations(ctx); err != nil {
		t.Fatal(err)
	}
	for _, user := range []string{leader, member} {
		name, err := s.UserDisplayName(ctx, user)
		if err != nil || !chineseDisplayName.MatchString(name) {
			t.Fatalf("backfill %s: %q %v", user, name, err)
		}
	}
	if _, err := pool.Exec(ctx, `UPDATE users SET display_name='薄荷水獭' WHERE id=$1`, leader); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyMigrations(ctx); err != nil {
		t.Fatal(err)
	}
	var gotParty, gotSession, gotName string
	if err := pool.QueryRow(ctx, `SELECT m.party_id,s.id,u.display_name FROM party_members m JOIN user_sessions s ON s.user_id=m.user_id JOIN users u ON u.id=m.user_id WHERE m.user_id=$1`, leader).Scan(&gotParty, &gotSession, &gotName); err != nil {
		t.Fatal(err)
	}
	if gotParty != party || gotSession != session || gotName != "薄荷水獭" {
		t.Fatalf("upgrade changed relations/name: %s %s %s", gotParty, gotSession, gotName)
	}
}
