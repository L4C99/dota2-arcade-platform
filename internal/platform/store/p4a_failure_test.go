package store

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/L4C99/dota2-arcade-platform/internal/contracts/nodev1"
)

func p4aReserved(t *testing.T) (*Store, string, string, string, string) {
	t.Helper()
	s := playerTestStore(t)
	ctx := context.Background()
	gameID, presetID := seedPlayerCatalog(t, s)
	nodeID, _, err := s.RegisterNode(ctx, "p4a node", "linux")
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
	r, _, err := s.CreateUserRequest(ctx, userID, gameID, presetID)
	if err != nil {
		t.Fatal(err)
	}
	if changed, err := s.TryAllocateOne(ctx); err != nil || !changed {
		t.Fatalf("allocate: %t %v", changed, err)
	}
	var allocationID string
	if err := s.Pool.QueryRow(ctx, `SELECT id FROM allocations WHERE server_request_id=$1`, r.ID).Scan(&allocationID); err != nil {
		t.Fatal(err)
	}
	job, err := s.ClaimNextJob(ctx, nodeID)
	if err != nil || job == nil || job.Kind != "create" {
		t.Fatalf("claim create: %+v %v", job, err)
	}
	return s, nodeID, r.ID, allocationID, job.ID
}

func p4aStates(t *testing.T, s *Store, requestID, allocationID string, wantRequest, wantAllocation string, wantOccupied int) {
	t.Helper()
	ctx := context.Background()
	var requestState, allocationState string
	var occupied int
	if err := s.Pool.QueryRow(ctx, `SELECT state FROM server_requests WHERE id=$1`, requestID).Scan(&requestState); err != nil {
		t.Fatal(err)
	}
	if err := s.Pool.QueryRow(ctx, `SELECT state FROM allocations WHERE id=$1`, allocationID).Scan(&allocationState); err != nil {
		t.Fatal(err)
	}
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM allocations WHERE state NOT IN ('reclaimed','released_no_effect')`).Scan(&occupied); err != nil {
		t.Fatal(err)
	}
	if requestState != wantRequest || allocationState != wantAllocation || occupied != wantOccupied {
		t.Fatalf("request=%s allocation=%s occupied=%d; want %s/%s/%d", requestState, allocationState, occupied,
			wantRequest, wantAllocation, wantOccupied)
	}
}

func TestP4ANoEffectAndUnknownNeverDuplicateAttempt(t *testing.T) {
	s, nodeID, requestID, allocationID, jobID := p4aReserved(t)
	ctx := context.Background()
	if _, err := s.PrepareCreate(ctx, nodeID, jobID, "/tmp/p4a-template.json", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReportJob(ctx, nodeID, jobID, nodev1.ReportRequest{State: "unknown"}); err != nil {
		t.Fatal(err)
	}
	if changed, err := s.TryAllocateOne(ctx); err != nil || changed {
		t.Fatalf("unknown create caused dispatch: %t %v", changed, err)
	}
	p4aStates(t, s, requestID, allocationID, "creating", "create_unknown", 1)
	var jobs, attempts int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM node_jobs WHERE allocation_id=$1 AND kind='create'`, allocationID).Scan(&jobs); err != nil || jobs != 1 {
		t.Fatalf("create jobs=%d: %v", jobs, err)
	}
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM allocations WHERE server_request_id=$1`, requestID).Scan(&attempts); err != nil || attempts != 1 {
		t.Fatalf("attempts=%d: %v", attempts, err)
	}
	// An unknown call cannot be downgraded to a no-effect rejection based on
	// a later validation error. The Controller must keep reconciling it.
	if _, err := s.ReportJob(ctx, nodeID, jobID, nodev1.ReportRequest{State: "rejected_no_effect", ErrorCode: "PORT_IN_USE", ErrorStage: "validate"}); err == nil {
		t.Fatal("ambiguous unknown was released by a later rejection")
	}
	p4aStates(t, s, requestID, allocationID, "creating", "create_unknown", 1)
}

func TestP4AAcceptedCreateFailureAutoStopAndQuarantine(t *testing.T) {
	s, nodeID, requestID, allocationID, jobID := p4aReserved(t)
	ctx := context.Background()
	if _, err := s.PrepareCreate(ctx, nodeID, jobID, "/tmp/p4a-template.json", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReportJob(ctx, nodeID, jobID, nodev1.ReportRequest{State: "accepted", InstanceID: "i_p4a", OperationID: "o_create"}); err != nil {
		t.Fatal(err)
	}
	failure := nodev1.ReportRequest{State: "failed_with_effect", InstanceID: "i_p4a", OperationID: "o_create", ErrorCode: "START_FAILED", ErrorStage: "spawn"}
	var wg sync.WaitGroup
	errs := make([]error, 6)
	for i := range errs {
		wg.Add(1)
		go func(i int) { defer wg.Done(); _, errs[i] = s.ReportJob(ctx, nodeID, jobID, failure) }(i)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatalf("repeated failure report: %v", err)
		}
	}
	p4aStates(t, s, requestID, allocationID, "stopping", "stopping", 1)
	var stopJobs int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM node_jobs WHERE allocation_id=$1 AND kind='stop'`, allocationID).Scan(&stopJobs); err != nil || stopJobs != 1 {
		t.Fatalf("auto stop jobs=%d: %v", stopJobs, err)
	}
	stopJob, err := s.ClaimNextJob(ctx, nodeID)
	if err != nil || stopJob == nil || stopJob.Kind != "stop" || stopJob.InstanceID != "i_p4a" {
		t.Fatalf("auto stop: %+v %v", stopJob, err)
	}
	if _, err := s.ReportJob(ctx, nodeID, stopJob.ID, nodev1.ReportRequest{State: "unknown", InstanceID: "i_p4a"}); err != nil {
		t.Fatal(err)
	}
	p4aStates(t, s, requestID, allocationID, "stopping", "stopping", 1)
	if _, err := s.ReportJob(ctx, nodeID, stopJob.ID, nodev1.ReportRequest{State: "accepted", InstanceID: "i_p4a", OperationID: "o_stop"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReportJob(ctx, nodeID, stopJob.ID, nodev1.ReportRequest{State: "failed_with_effect", InstanceID: "i_p4a", OperationID: "o_stop", ErrorCode: "CLEANUP_FAILED", ErrorStage: "cleanup"}); err != nil {
		t.Fatal(err)
	}
	p4aStates(t, s, requestID, allocationID, "quarantined", "quarantined", 1)
	if _, err := s.ReportJob(ctx, nodeID, stopJob.ID, nodev1.ReportRequest{State: "succeeded", InstanceID: "i_p4a", OperationID: "o_stop"}); !errors.Is(err, ErrJobConflict) {
		t.Fatalf("terminal failure was overwritten: %v", err)
	}
	var attempts int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM allocations WHERE server_request_id=$1`, requestID).Scan(&attempts); err != nil || attempts != 1 {
		t.Fatalf("allocation history changed: %d %v", attempts, err)
	}
}

func TestP4AUntrustedCreateIdentityQuarantinesWithoutStop(t *testing.T) {
	s, nodeID, requestID, allocationID, jobID := p4aReserved(t)
	ctx := context.Background()
	if _, err := s.PrepareCreate(ctx, nodeID, jobID, "/tmp/p4a-template.json", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReportJob(ctx, nodeID, jobID, nodev1.ReportRequest{State: "accepted", InstanceID: "i_p4a", OperationID: "o_create"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReportJob(ctx, nodeID, jobID, nodev1.ReportRequest{State: "failed_with_effect", InstanceID: "i_p4a",
		OperationID: "o_create", ErrorCode: "IDENTITY_UNVERIFIED", ErrorStage: "recover"}); err != nil {
		t.Fatal(err)
	}
	p4aStates(t, s, requestID, allocationID, "quarantined", "quarantined", 1)
	var stopJobs int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM node_jobs WHERE allocation_id=$1 AND kind='stop'`, allocationID).Scan(&stopJobs); err != nil || stopJobs != 0 {
		t.Fatalf("untrusted identity triggered stop: %d %v", stopJobs, err)
	}
}
