package httpapi_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/L4C99/dota2-arcade-platform/internal/contracts/nodev1"
	"github.com/L4C99/dota2-arcade-platform/internal/controller/platformclient"
	"github.com/L4C99/dota2-arcade-platform/internal/platform/httpapi"
	"github.com/L4C99/dota2-arcade-platform/internal/platform/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestNodeAPIWithPostgres(t *testing.T) {
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
	id, err := store.NewID()
	if err != nil {
		t.Fatal(err)
	}
	schema := "p0c_" + strings.ReplaceAll(id, "-", "")
	if _, err := base.Exec(ctx, `CREATE SCHEMA "`+schema+`"`); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := base.Exec(ctx, `DROP SCHEMA "`+schema+`" CASCADE`); err != nil {
			t.Errorf("test schema cleanup: %v", err)
		}
	}()
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	s := &store.Store{Pool: pool}
	defer s.Close()
	if err := s.ApplyMigrations(ctx); err != nil {
		t.Fatal(err)
	}
	nodeID, secret, err := s.RegisterNode(ctx, "integration node", "windows")
	if err != nil {
		t.Fatal(err)
	}
	jobID, err := s.CreateIntegrationJob(ctx, nodeID, "create", "")
	if err != nil {
		t.Fatal(err)
	}
	handler, err := httpapi.NewHandler(s, httpapi.Config{PublicOrigin: "http://127.0.0.1:8080", Development: true})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	client := platformclient.New(server.URL, nodeID, secret)
	wrong := platformclient.New(server.URL, nodeID, "wrong")
	if _, err := wrong.OpenJobs(ctx); err == nil {
		t.Fatal("wrong node secret accepted")
	} else {
		var status *platformclient.StatusError
		if !errors.As(err, &status) || status.StatusCode != http.StatusUnauthorized {
			t.Fatalf("wrong secret status: %v", err)
		}
	}
	h := nodev1.Heartbeat{OS: "windows", ControllerVersion: "p0c-test", NodeAPIVersion: nodev1.APIVersion, D2CoreVersion: nodev1.D2CoreVersion,
		D2CoreCommit: nodev1.D2CoreCommit, HardMaxInstances: 1,
		Network: nodev1.NetworkFacts{ConnectHost: "node.example", LocalPortMin: 28000, LocalPortMax: 28000, MappingMode: "identity"},
		Content: []nodev1.ContentFact{{WorkshopID: "123", ContentVersionID: "v1", State: "confirmed"}}}
	result, err := client.Heartbeat(ctx, h)
	if err != nil || result.CompatibilityStatus != "incompatible" {
		t.Fatalf("unverified core protocol accepted: %+v %v", result, err)
	}
	if job, err := client.Claim(ctx); err != nil || job != nil {
		t.Fatalf("claimed while incompatible: %+v %v", job, err)
	}
	h.D2CoreProtocolVersion = nodev1.D2CoreProtocolVersion
	h.NodeAPIVersion = 0
	result, err = client.Heartbeat(ctx, h)
	if err != nil || result.CompatibilityStatus != "incompatible" {
		t.Fatalf("wrong Node API version accepted: %+v %v", result, err)
	}
	h.NodeAPIVersion = nodev1.APIVersion
	result, err = client.Heartbeat(ctx, h)
	if err != nil || result.CompatibilityStatus != "compatible" {
		t.Fatalf("compatible heartbeat: %+v %v", result, err)
	}
	var contentJSON string
	if err := s.Pool.QueryRow(ctx, "SELECT content_facts::text FROM node_reports WHERE node_id=$1", nodeID).Scan(&contentJSON); err != nil || !strings.Contains(contentJSON, "v1") {
		t.Fatalf("content fact was not persisted: %q %v", contentJSON, err)
	}
	job, err := client.Claim(ctx)
	if err != nil || job == nil || job.ID != jobID || job.State != "claimed" {
		t.Fatalf("claim: %+v %v", job, err)
	}
	frozen, err := client.Prepare(ctx, jobID, nodev1.PrepareCreateRequest{Template: "C:/test/template.json", Port: 0})
	if err != nil || frozen.IdempotencyKey != store.CoreIdempotencyKey(jobID) {
		t.Fatalf("prepare: %+v %v", frozen, err)
	}
	again, err := client.Prepare(ctx, jobID, nodev1.PrepareCreateRequest{Template: "C:/test/template.json", Port: 0})
	if err != nil || again != frozen {
		t.Fatalf("idempotent prepare: %+v %v", again, err)
	}
	if _, err := client.Prepare(ctx, jobID, nodev1.PrepareCreateRequest{Template: "C:/changed/template.json", Port: 0}); err == nil {
		t.Fatal("changed create request accepted")
	}
	accepted := nodev1.ReportRequest{State: "accepted", InstanceID: "i_test", OperationID: "o_test"}
	if _, err := client.Report(ctx, jobID, accepted); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Report(ctx, jobID, accepted); err != nil {
		t.Fatalf("repeat report: %v", err)
	}
	if _, err := client.Report(ctx, jobID, nodev1.ReportRequest{State: "unknown"}); err != nil {
		t.Fatal(err)
	}
	open, err := client.OpenJobs(ctx)
	if err != nil || len(open) != 1 || open[0].State != "unknown" || open[0].FrozenCreate == nil || open[0].InstanceID != "i_test" {
		t.Fatalf("open job after unknown: %+v %v", open, err)
	}
	if _, err := client.Report(ctx, jobID, nodev1.ReportRequest{State: "succeeded", InstanceID: "i_test", OperationID: "o_test"}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Report(ctx, jobID, nodev1.ReportRequest{State: "accepted", InstanceID: "i_test", OperationID: "o_test"}); err == nil {
		t.Fatal("terminal job regressed")
	}
	open, err = client.OpenJobs(ctx)
	if err != nil || len(open) != 0 {
		t.Fatalf("terminal job remained open: %+v %v", open, err)
	}
	secondJobID, err := s.CreateIntegrationJob(ctx, nodeID, "create", "")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan *nodev1.Job, 2)
	errorsCh := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			claimed, claimErr := client.Claim(ctx)
			results <- claimed
			errorsCh <- claimErr
		}()
	}
	wg.Wait()
	close(results)
	close(errorsCh)
	for claimErr := range errorsCh {
		if claimErr != nil {
			t.Fatal(claimErr)
		}
	}
	claimedCount := 0
	for claimed := range results {
		if claimed != nil {
			claimedCount++
			if claimed.ID != secondJobID {
				t.Fatalf("wrong job claimed: %+v", claimed)
			}
		}
	}
	if claimedCount != 1 {
		t.Fatalf("concurrent claim count=%d", claimedCount)
	}
	var reports int
	if err := s.Pool.QueryRow(ctx, "SELECT count(*) FROM node_job_reports WHERE node_job_id=$1", jobID).Scan(&reports); err != nil || reports != 3 {
		t.Fatalf("idempotent report history count=%d err=%v", reports, err)
	}
	otherID, otherSecret, err := s.RegisterNode(ctx, "other node", "windows")
	if err != nil {
		t.Fatal(err)
	}
	other := platformclient.New(server.URL, otherID, otherSecret)
	if _, err := other.GetJob(ctx, jobID); err == nil {
		t.Fatal("other node read a job")
	}
}
