package httpapi_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/L4C99/dota2-arcade-platform/internal/contracts/nodev1"
	"github.com/L4C99/dota2-arcade-platform/internal/controller/platformclient"
	"github.com/L4C99/dota2-arcade-platform/internal/platform/httpapi"
	"github.com/L4C99/dota2-arcade-platform/internal/platform/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestNodeCapabilityMixedVersionRecovery(t *testing.T) {
	dsn := os.Getenv("PLATFORM_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("disposable PostgreSQL unavailable")
	}
	ctx := context.Background()
	base, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := store.NewID()
	schema := "i2http_" + strings.ReplaceAll(id, "-", "")
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
	s := &store.Store{Pool: pool}
	t.Cleanup(s.Close)
	if err := s.ApplyMigrations(ctx); err != nil {
		t.Fatal(err)
	}
	nodeID, secret, err := s.RegisterNode(ctx, "I2 mixed node", "linux")
	if err != nil {
		t.Fatal(err)
	}
	gameID, _ := store.NewID()
	presetID, _ := store.NewID()
	if _, err := pool.Exec(ctx, `INSERT INTO arcade_games(id,workshop_id,display_name) VALUES($1,'12345','I2 game')`, gameID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO content_versions(id,arcade_game_id,content_sha256) VALUES('i2-v1',$1,$2)`, gameID, strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO template_revisions(id,arcade_game_id) VALUES('i2-template',$1)`, gameID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO game_presets(id,arcade_game_id,display_name,max_players,template_revision_id) VALUES($1,$2,'I2 preset',2,'i2-template')`, presetID, gameID); err != nil {
		t.Fatal(err)
	}
	adminID, err := s.CreateAdmin(ctx, "i2-admin", "test-only-long-password")
	if err != nil {
		t.Fatal(err)
	}
	handler, err := httpapi.NewHandler(s, httpapi.Config{PublicOrigin: "http://127.0.0.1:8080", Development: true})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	capableA := platformclient.New(server.URL, nodeID, secret)
	old := platformclient.New(server.URL, nodeID, secret)
	h := nodev1.Heartbeat{OS: "linux", ControllerVersion: "old-v1", NodeAPIVersion: 1, D2CoreVersion: nodev1.D2CoreVersion,
		D2CoreCommit: nodev1.D2CoreCommit, D2CoreProtocolVersion: 1, HardMaxInstances: 1,
		Network: nodev1.NetworkFacts{ConnectHost: "node.example", LocalPortMin: 28000, LocalPortMax: 28000, MappingMode: "identity"}, Content: []nodev1.ContentFact{}}
	if _, err := old.Heartbeat(ctx, h); err != nil {
		t.Fatal(err)
	}
	createID, allocationID, runID := mustID(t), mustID(t), mustID(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `INSERT INTO validation_runs
		(id,started_by_admin_user_id,node_id,arcade_game_id,game_preset_id,content_version_id,content_sha256,
		template_revision_id,template_binding_key,template_fingerprint_sha256,template_binding_generation,
		maintenance_epoch,content_fact_revision,template_fact_revision)
		VALUES($1,$2,$3,$4,$5,'i2-v1',$6,'i2-template','i2-key',$7,1,1,1,1)`, runID, adminID, nodeID, gameID, presetID, strings.Repeat("a", 64), strings.Repeat("f", 64)); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO allocations(id,purpose,validation_run_id,arcade_game_id,attempt_sequence,node_id,content_version_id,template_revision_id,state,assigned_at)
		VALUES($1,'validation',$2,$3,1,$4,'i2-v1','i2-template','reserved',now())`, allocationID, runID, gameID, nodeID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO node_jobs(id,node_id,kind,integration_only,allocation_id,template_binding_key,required_capability,
		expected_template_fingerprint_sha256,template_binding_generation,expected_workshop_id,expected_content_version_id,expected_vpk_sha256)
		VALUES($1,$2,'create',false,$3,'i2-key','content_validation_v102',$4,1,'12345','i2-v1',$5)`, createID, nodeID, allocationID,
		strings.Repeat("f", 64), strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	legacyID, err := s.CreateIntegrationCreateJob(ctx, nodeID, "legacy-key", 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := capableA.EnsureSession(ctx); err != nil {
		t.Fatal(err)
	}
	if err := old.ReportInventory(ctx, nodev1.InventoryReport{ScanID: "old-scan", Complete: true, Instances: []nodev1.InventoryInstance{}}); !statusIs(err, http.StatusForbidden) {
		t.Fatalf("old inventory: %v", err)
	}
	if err := capableA.ReportInventory(ctx, nodev1.InventoryReport{ScanID: "new-empty", Complete: true, Instances: []nodev1.InventoryInstance{}}); err != nil {
		t.Fatal(err)
	}
	if err := capableA.ReportInventory(ctx, nodev1.InventoryReport{ScanID: "new-empty", Complete: true, Instances: []nodev1.InventoryInstance{}}); !statusIs(err, http.StatusConflict) {
		t.Fatalf("duplicate inventory scan: %v", err)
	}
	job, err := capableA.Claim(ctx)
	if err != nil || job == nil || job.ID != createID || job.RequiredCapability != nodev1.RequiredContentValidationV102 {
		t.Fatalf("capable claim: %+v %v", job, err)
	}
	if _, err := capableA.Prepare(ctx, createID, nodev1.PrepareCreateRequest{Template: "C:/immutable/template.json", TemplateManifestAlgorithm: "future", ObservedTemplateFingerprintSHA256: strings.Repeat("f", 64)}); !statusIs(err, http.StatusConflict) {
		t.Fatalf("invalid algorithm: %v", err)
	}
	if _, err := capableA.Prepare(ctx, createID, nodev1.PrepareCreateRequest{Template: "C:/immutable/template.json", TemplateManifestAlgorithm: nodev1.TemplateManifestAlgorithmV1, ObservedTemplateFingerprintSHA256: "short"}); !statusIs(err, http.StatusConflict) {
		t.Fatalf("malformed digest: %v", err)
	}
	prepared, err := capableA.Prepare(ctx, createID, nodev1.PrepareCreateRequest{Template: "C:/immutable/template.json", Port: 0,
		TemplateManifestAlgorithm: nodev1.TemplateManifestAlgorithmV1, ObservedTemplateFingerprintSHA256: strings.Repeat("f", 64)})
	if err != nil || prepared.IdempotencyKey != store.CoreIdempotencyKey(createID) {
		t.Fatalf("prepare: %+v %v", prepared, err)
	}
	if _, err := capableA.Prepare(ctx, createID, nodev1.PrepareCreateRequest{Template: "C:/changed/template.json", TemplateManifestAlgorithm: nodev1.TemplateManifestAlgorithmV1, ObservedTemplateFingerprintSHA256: strings.Repeat("f", 64)}); !statusIs(err, http.StatusConflict) {
		t.Fatalf("changed frozen execution: %v", err)
	}
	if _, err := old.Heartbeat(ctx, h); err != nil {
		t.Fatal(err)
	}
	jobs, err := old.OpenJobs(ctx)
	if err != nil || len(jobs) != 1 || jobs[0].ID != legacyID {
		t.Fatalf("old open jobs leaked v102: %+v %v", jobs, err)
	}
	oldJob, err := old.Claim(ctx)
	if err != nil || oldJob == nil || oldJob.ID != legacyID {
		t.Fatalf("old legacy claim: %+v %v", oldJob, err)
	}
	if legacy, err := old.GetJob(ctx, legacyID); err != nil || legacy.ID != legacyID || legacy.RequiredCapability != "" {
		t.Fatalf("old legacy GET: %+v %v", legacy, err)
	}
	if _, err := old.Prepare(ctx, legacyID, nodev1.PrepareCreateRequest{Template: "C:/legacy/template.json"}); err != nil {
		t.Fatalf("old legacy prepare: %v", err)
	}
	if _, err := old.Report(ctx, legacyID, nodev1.ReportRequest{State: "rejected_no_effect", ErrorCode: "LOCAL_TEMPLATE_BINDING", ErrorStage: "validate"}); err != nil {
		t.Fatalf("old legacy report: %v", err)
	}
	if _, err := old.GetJob(ctx, createID); !statusIs(err, http.StatusNotFound) {
		t.Fatalf("old GET v102: %v", err)
	}
	if _, err := old.Prepare(ctx, createID, nodev1.PrepareCreateRequest{Template: "C:/immutable/template.json"}); !statusIs(err, http.StatusNotFound) {
		t.Fatalf("old prepare v102: %v", err)
	}
	if _, err := old.Report(ctx, createID, nodev1.ReportRequest{State: "rejected_no_effect", ErrorCode: "FAKE"}); !statusIs(err, http.StatusNotFound) {
		t.Fatalf("old report v102: %v", err)
	}
	if _, err := capableA.Report(ctx, createID, nodev1.ReportRequest{State: "accepted", InstanceID: "i_v102", OperationID: "o_v102"}); err != nil {
		t.Fatal(err)
	}
	if err := capableA.ReportInventory(ctx, nodev1.InventoryReport{ScanID: "new-active", Complete: true, Instances: []nodev1.InventoryInstance{{InstanceID: "i_v102", Lifecycle: "active", Process: "running", Cleanup: "pending"}}}); err != nil {
		t.Fatal(err)
	}
	var unaccounted int
	if err := pool.QueryRow(ctx, `SELECT unaccounted_count FROM node_inventory_snapshots WHERE node_id=$1 AND scan_id='new-active'`, nodeID).Scan(&unaccounted); err != nil || unaccounted != 0 {
		t.Fatalf("active allocation inventory accounting: %d %v", unaccounted, err)
	}
	active, err := old.ActiveAllocations(ctx)
	if err != nil || len(active) != 0 {
		t.Fatalf("old allocation leak: %+v %v", active, err)
	}
	if err := old.ReportInstanceFact(ctx, allocationID, nodev1.InstanceFact{InstanceID: "i_v102", Outcome: "uncertain"}); !statusIs(err, http.StatusNotFound) {
		t.Fatalf("old instance fact: %v", err)
	}
	capableC := platformclient.New(server.URL, nodeID, secret)
	if err := capableC.EnsureSession(ctx); err != nil {
		t.Fatal(err)
	}
	if err := capableA.CheckSession(ctx); !statusIs(err, http.StatusForbidden) {
		t.Fatalf("replaced session can still authorize core execution: %v", err)
	}
	if _, err := capableA.GetJob(ctx, createID); !statusIs(err, http.StatusNotFound) {
		t.Fatalf("session A survived C: %v", err)
	}
	recovered, err := capableC.GetJob(ctx, createID)
	if err != nil || recovered.FrozenCreate == nil || *recovered.FrozenCreate != prepared || recovered.ID != createID {
		t.Fatalf("recovery changed frozen execution: %+v %v", recovered, err)
	}
	if _, err := capableC.Report(ctx, createID, nodev1.ReportRequest{State: "succeeded", InstanceID: "i_v102", OperationID: "o_v102", JoinInfoErrorCode: "NO_JOIN"}); err != nil {
		t.Fatal(err)
	}
	stopID := mustID(t)
	if _, err := pool.Exec(ctx, `INSERT INTO node_jobs(id,node_id,kind,integration_only,allocation_id,instance_id,required_capability)
		VALUES($1,$2,'stop',false,$3,'i_v102','content_validation_v102')`, stopID, nodeID, allocationID); err != nil {
		t.Fatal(err)
	}
	jobs, err = old.OpenJobs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, j := range jobs {
		if j.ID == stopID {
			t.Fatal("old Controller saw v102 stop")
		}
	}
	if _, err := old.GetJob(ctx, stopID); !statusIs(err, http.StatusNotFound) {
		t.Fatalf("old GET stop: %v", err)
	}
	if _, err := old.Report(ctx, stopID, nodev1.ReportRequest{State: "unknown", InstanceID: "i_v102"}); !statusIs(err, http.StatusNotFound) {
		t.Fatalf("old report stop: %v", err)
	}
	if job, err := old.Claim(ctx); err != nil || job != nil {
		t.Fatalf("old claimed v102 stop: %+v %v", job, err)
	}
	if job, err := old.ClaimIndependentStop(ctx); err != nil || job != nil {
		t.Fatalf("old independently claimed v102 stop: %+v %v", job, err)
	}
	stop, err := capableC.Claim(ctx)
	if err != nil || stop == nil || stop.ID != stopID {
		t.Fatalf("capable stop recovery: %+v %v", stop, err)
	}
	capableD := platformclient.New(server.URL, nodeID, secret)
	if err := capableD.EnsureSession(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := capableC.GetJob(ctx, stopID); !statusIs(err, http.StatusNotFound) {
		t.Fatalf("replaced C read stop: %v", err)
	}
	if recoveredStop, err := capableD.GetJob(ctx, stopID); err != nil || recoveredStop.ID != stopID || recoveredStop.InstanceID != "i_v102" {
		t.Fatalf("stop recovery changed durable job: %+v %v", recoveredStop, err)
	}
	var occupied int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM allocations WHERE node_id=$1 AND state NOT IN ('reclaimed','released_no_effect')`, nodeID).Scan(&occupied); err != nil || occupied != 1 {
		t.Fatalf("capability loss released capacity: %d %v", occupied, err)
	}
}

func mustID(t *testing.T) string {
	t.Helper()
	id, err := store.NewID()
	if err != nil {
		t.Fatal(err)
	}
	return id
}
func statusIs(err error, code int) bool {
	var s *platformclient.StatusError
	return errors.As(err, &s) && s.StatusCode == code
}
