package store

import (
	"context"
	"sync"
	"testing"

	"github.com/L4C99/dota2-arcade-platform/internal/contracts/nodev1"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestP1EDuplicatePostHasOneAllocationAndJob(t *testing.T) {
	s := playerTestStore(t)
	ctx := context.Background()
	gameID, presetID := seedPlayerCatalog(t, s)
	nodeID, _, err := s.RegisterNode(ctx, "test node", "linux")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `INSERT INTO node_template_bindings(node_id,template_revision_id,binding_key)
		VALUES($1,'test-template','test-binding')`, nodeID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecordHeartbeat(ctx, nodeID, p1TestHeartbeat("test-v1")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE nodes SET desired_max_instances=1 WHERE id=$1`, nodeID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE node_content_bindings SET accepting_new_allocations=true WHERE node_id=$1`, nodeID); err != nil {
		t.Fatal(err)
	}
	userID, _, err := s.CreateUserSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	const posts = 8
	ids := make([]string, posts)
	errs := make([]error, posts)
	var wg sync.WaitGroup
	for i := range ids {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			request, _, err := s.CreateUserRequest(ctx, userID, gameID, presetID)
			ids[i], errs[i] = request.ID, err
		}(i)
	}
	wg.Wait()
	for i := range ids {
		if errs[i] != nil || ids[i] == "" || ids[i] != ids[0] {
			t.Fatalf("POST %d: %q %v", i, ids[i], errs[i])
		}
	}
	// The browser may lose every POST response and still recover this ID.
	current, err := s.CurrentUserRequest(ctx, userID)
	if err != nil || current == nil || current.ID != ids[0] {
		t.Fatalf("current after lost responses %+v: %v", current, err)
	}
	if changed, err := s.TryAllocateOne(ctx); err != nil || !changed {
		t.Fatalf("allocate changed=%t: %v", changed, err)
	}
	if changed, err := s.TryAllocateOne(ctx); err != nil || changed {
		t.Fatalf("duplicate allocation changed=%t: %v", changed, err)
	}
	var requests, allocations, jobs int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM server_requests WHERE owner_user_id=$1`, userID).Scan(&requests); err != nil {
		t.Fatal(err)
	}
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM allocations WHERE server_request_id=$1`, current.ID).Scan(&allocations); err != nil {
		t.Fatal(err)
	}
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM node_jobs j JOIN allocations a ON a.id=j.allocation_id
		WHERE a.server_request_id=$1`, current.ID).Scan(&jobs); err != nil {
		t.Fatal(err)
	}
	if requests != 1 || allocations != 1 || jobs != 1 {
		t.Fatalf("duplicate business objects: requests=%d allocations=%d jobs=%d", requests, allocations, jobs)
	}
}

// A fresh Store/pool represents a Platform process restart while the business
// request and a frozen, unresolved create remain durable in PostgreSQL.
func TestP1EPlatformRestartKeepsBusinessAssociation(t *testing.T) {
	s := playerTestStore(t)
	ctx := context.Background()
	gameID, presetID := seedPlayerCatalog(t, s)
	nodeID, _, err := s.RegisterNode(ctx, "test node", "linux")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `INSERT INTO node_template_bindings(node_id,template_revision_id,binding_key)
		VALUES($1,'test-template','test-binding')`, nodeID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecordHeartbeat(ctx, nodeID, p1TestHeartbeat("test-v1")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE nodes SET desired_max_instances=1 WHERE id=$1`, nodeID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE node_content_bindings SET accepting_new_allocations=true WHERE node_id=$1`, nodeID); err != nil {
		t.Fatal(err)
	}
	userID, token, err := s.CreateUserSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	request, _, err := s.CreateUserRequest(ctx, userID, gameID, presetID)
	if err != nil {
		t.Fatal(err)
	}
	if changed, err := s.TryAllocateOne(ctx); err != nil || !changed {
		t.Fatalf("allocation changed=%t: %v", changed, err)
	}
	allocation, err := s.UserRequestAllocation(ctx, userID, request.ID)
	if err != nil || allocation == nil {
		t.Fatalf("allocation %+v: %v", allocation, err)
	}
	job, err := s.ClaimNextJob(ctx, nodeID)
	if err != nil || job == nil {
		t.Fatalf("create job %+v: %v", job, err)
	}
	if _, err := s.PrepareCreate(ctx, nodeID, job.ID, "/tmp/trusted-template.json", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReportJob(ctx, nodeID, job.ID, nodev1.ReportRequest{State: "unknown"}); err != nil {
		t.Fatal(err)
	}
	before, err := s.JobForNode(ctx, nodeID, job.ID)
	if err != nil || before.FrozenCreate == nil {
		t.Fatalf("frozen job %+v: %v", before, err)
	}
	config := s.Pool.Config()
	s.Close()
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	resumed := &Store{Pool: pool}
	t.Cleanup(resumed.Close)
	if got, err := resumed.UserForToken(ctx, token); err != nil || got != userID {
		t.Fatalf("session after restart %s: %v", got, err)
	}
	current, err := resumed.CurrentUserRequest(ctx, userID)
	if err != nil || current == nil || current.ID != request.ID {
		t.Fatalf("request after restart %+v: %v", current, err)
	}
	visible, err := resumed.UserRequestAllocation(ctx, userID, request.ID)
	if err != nil || visible == nil || visible.ID != allocation.ID || visible.NodeID != nodeID || visible.ContentVersionID != "test-v1" {
		t.Fatalf("allocation after restart %+v: %v", visible, err)
	}
	after, err := resumed.JobForNode(ctx, nodeID, job.ID)
	if err != nil || after.FrozenCreate == nil || after.FrozenCreate.IdempotencyKey != before.FrozenCreate.IdempotencyKey || after.FrozenCreate.Template != before.FrozenCreate.Template {
		t.Fatalf("job after restart %+v: %v", after, err)
	}
	var relatedRequestID string
	if err := resumed.Pool.QueryRow(ctx, `SELECT a.server_request_id FROM node_jobs j
		JOIN allocations a ON a.id=j.allocation_id WHERE j.id=$1`, job.ID).Scan(&relatedRequestID); err != nil || relatedRequestID != request.ID {
		t.Fatalf("business association %q: %v", relatedRequestID, err)
	}
	if changed, err := resumed.TryAllocateOne(ctx); err != nil || changed {
		t.Fatalf("restarted Platform duplicated allocation: %t %v", changed, err)
	}
}
