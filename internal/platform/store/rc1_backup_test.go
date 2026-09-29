package store

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Compare complete rows (including credentials only in memory), not just counts.
func rc1Snapshot(t *testing.T, pool *pgxpool.Pool, upgrading bool) string {
	t.Helper()
	var result strings.Builder
	for _, table := range []string{"users", "user_sessions", "admin_users", "admin_sessions", "parties", "party_members", "party_invites", "arcade_games", "game_presets", "content_versions", "template_revisions", "nodes", "node_content_bindings", "node_entry_capabilities", "server_requests", "allocations", "node_jobs", "node_job_executions", "node_job_reports", "audit_events", "next_game_intents"} {
		row := "to_jsonb(r)"
		if upgrading && table == "next_game_intents" {
			row += " - 'failure_reason'"
		}
		var value string
		query := fmt.Sprintf("SELECT COALESCE(jsonb_agg(v ORDER BY v::text),'[]'::jsonb)::text FROM (SELECT %s AS v FROM %s r) x", row, pgx.Identifier{table}.Sanitize())
		if err := pool.QueryRow(context.Background(), query).Scan(&value); err != nil {
			t.Fatal(err)
		}
		result.WriteString(table + value)
	}
	return result.String()
}

func TestRC1BackupRestore(t *testing.T) {
	if runtime.GOOS != "linux" || os.Getenv("RC1_BACKUP_TEST") != "1" {
		t.Skip("opt-in Linux disposable database backup gate")
	}
	ctx := context.Background()
	base, err := pgxpool.New(ctx, os.Getenv("PLATFORM_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(base.Close)
	id, err := NewID()
	if err != nil {
		t.Fatal(err)
	}
	prefix := "rc1_" + strings.ReplaceAll(id, "-", "")
	source, restored := prefix+"_source", prefix+"_restore"
	for _, db := range []string{source, restored} {
		if _, err := base.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{db}.Sanitize()); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if _, err := base.Exec(ctx, "DROP DATABASE "+pgx.Identifier{db}.Sanitize()+" WITH (FORCE)"); err != nil {
				t.Error(err)
			}
		})
	}
	conf := base.Config().Copy()
	conf.ConnConfig.Database = source
	uri := &url.URL{Scheme: "postgres", User: url.UserPassword(conf.ConnConfig.User, conf.ConnConfig.Password), Host: fmt.Sprintf("%s:%d", conf.ConnConfig.Host, conf.ConnConfig.Port), Path: source, RawQuery: "sslmode=disable"}
	t.Setenv("PLATFORM_TEST_DATABASE_URL", uri.String())
	s, _, owner, request, _ := p4cRunning(t)
	if _, err := s.NextGameUserRequest(ctx, owner, request); err != nil {
		t.Fatal(err)
	}
	if err := s.ConfigurePartySize(ctx, 4); err != nil {
		t.Fatal(err)
	}
	u, _, err := s.CreateUserSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateParty(ctx, u); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateAdmin(ctx, "restore-fixture", "test-only-long-password"); err != nil {
		t.Fatal(err)
	}
	before := rc1Snapshot(t, s.Pool, false)
	dir := t.TempDir()
	serviceFile := filepath.Join(dir, "pg_service.conf")
	c := conf.ConnConfig
	for _, value := range []string{c.Host, c.User, c.Password} {
		if strings.ContainsAny(value, "\r\n") {
			t.Fatal("fixture service values must be single-line")
		}
	}
	// libpq service files use INI values, not connection-string quoting.
	service := fmt.Sprintf("[rc1]\nhost=%s\nport=%d\nuser=%s\npassword=%s\ndbname=%s\nsslmode=disable\n", c.Host, c.Port, c.User, c.Password, source)
	if err := os.WriteFile(serviceFile, []byte(service), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PGSERVICEFILE", serviceFile)
	t.Setenv("PGSERVICE", "rc1")
	t.Setenv("PGPASSWORD", "")
	dump := filepath.Join(dir, "source.dump")
	helper, err := filepath.Abs("../../../deploy/scripts/backup-postgres.sh")
	if err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("bash", helper, dump).CombinedOutput(); err != nil {
		t.Fatalf("backup: %v %s", err, out)
	}
	stat, err := os.Stat(dump)
	if err != nil || stat.Mode().Perm() != 0600 {
		t.Fatal("backup must be 0600")
	}
	if out, err := exec.Command("pg_restore", "--list", dump).CombinedOutput(); err != nil || !strings.Contains(string(out), "schema_migrations") {
		t.Fatalf("TOC: %v", err)
	}
	if _, err := exec.Command("bash", helper, dump).CombinedOutput(); err == nil {
		t.Fatal("backup overwrote existing file")
	}
	if err := os.Chmod(serviceFile, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := exec.Command("bash", helper, filepath.Join(dir, "unsafe.dump")).CombinedOutput(); err == nil {
		t.Fatal("public service file accepted")
	}
	if err := os.Chmod(serviceFile, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(serviceFile, []byte(strings.Replace(service, "dbname="+source, "dbname="+prefix+"_missing", 1)), 0600); err != nil {
		t.Fatal(err)
	}
	failedDump := filepath.Join(dir, "failed.dump")
	if _, err := exec.Command("bash", helper, failedDump).CombinedOutput(); err == nil {
		t.Fatal("failed pg_dump accepted")
	}
	if _, err := os.Stat(failedDump); !os.IsNotExist(err) {
		t.Fatal("failed dump was published")
	}
	if partials, err := filepath.Glob(failedDump + ".partial.*"); err != nil || len(partials) != 0 {
		t.Fatal("failed backup left partial files")
	}
	if err := os.WriteFile(serviceFile, []byte(service), 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("pg_restore", "--exit-on-error", "--no-owner", "--dbname", restored, dump).CombinedOutput(); err != nil {
		t.Fatalf("restore: %v %s", err, out)
	}
	conf.ConnConfig.Database = restored
	conf.ConnConfig.RuntimeParams["search_path"] = s.Pool.Config().ConnConfig.RuntimeParams["search_path"]
	pool, err := pgxpool.NewWithConfig(ctx, conf)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	r := &Store{Pool: pool}
	if err := r.ApplyMigrations(ctx); err != nil {
		t.Fatal(err)
	}
	if after := rc1Snapshot(t, pool, false); after != before {
		t.Fatal("restore row history mismatch")
	}
	var latest, count int
	if err := pool.QueryRow(ctx, `SELECT max(version),count(*) FROM schema_migrations`).Scan(&latest, &count); err != nil || latest != 21 || count != 21 {
		t.Fatalf("restored migrations %d/%d: %v", latest, count, err)
	}
	// No process or resource operation follows restore. Database recovery is not reclaim.
}
