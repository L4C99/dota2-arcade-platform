package store

import (
	"context"
	"testing"

	"github.com/L4C99/dota2-arcade-platform/internal/contracts/nodev1"
)

func v102AwaitingHumanRun(t *testing.T) (*Store, ValidationCandidate, ValidationRun) {
	t.Helper()
	s, candidate := v102ValidationFixture(t, 1)
	ctx := context.Background()
	run, err := s.ReserveValidation(ctx, candidate)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE node_jobs SET state='claimed' WHERE id=$1`, run.CreateJobID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PrepareCreateWithManifest(ctx, candidate.NodeID, run.CreateJobID, "C:/versioned/template.json", 0,
		"template-manifest-sha256-v1", candidate.ExpectedTemplateFingerprintSHA256); err != nil {
		t.Fatal(err)
	}
	beginV102TestOperation(t, s, candidate.NodeID, run.CreateJobID)
	if _, err := s.ReportJob(ctx, candidate.NodeID, run.CreateJobID, nodev1.ReportRequest{
		State: "accepted", InstanceID: "i_validation", OperationID: "o_create",
	}); err != nil {
		t.Fatal(err)
	}
	join := &nodev1.JoinInfo{LocalPort: 28000, PublicPort: 28000, ConnectHost: "127.0.0.1",
		EntryConfigRevision: nodev1.EntryConfigRevision(p1TestHeartbeat("test-v1").Network)}
	if _, err := s.ReportJob(ctx, candidate.NodeID, run.CreateJobID, nodev1.ReportRequest{
		State: "succeeded", InstanceID: "i_validation", OperationID: "o_create", JoinInfo: join,
	}); err != nil {
		t.Fatal(err)
	}
	run, err = s.ValidationRun(ctx, run.ID)
	if err != nil || run.State != "awaiting_human" || run.HumanResult != "pending" {
		t.Fatalf("expected pending human confirmation: %+v %v", run, err)
	}
	return s, candidate, run
}

func v102ReclaimedFact() nodev1.InstanceFact {
	return nodev1.InstanceFact{InstanceID: "i_validation", Outcome: "reclaimed",
		Lifecycle: "reclaimed", Process: "stopped", Cleanup: "complete", Port: 28000}
}

func assertV102ReclaimedAllocation(t *testing.T, s *Store, run ValidationRun) {
	t.Helper()
	ctx := context.Background()
	var state string
	var occupied int
	if err := s.Pool.QueryRow(ctx, `SELECT state FROM allocations WHERE id=$1`, run.AllocationID).Scan(&state); err != nil || state != "reclaimed" {
		t.Fatalf("Allocation state=%s err=%v", state, err)
	}
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM allocations WHERE node_id=$1
		AND state NOT IN ('reclaimed','released_no_effect')`, run.NodeID).Scan(&occupied); err != nil || occupied != 0 {
		t.Fatalf("occupied capacity=%d err=%v", occupied, err)
	}
}

func TestV102ReclaimedFactBeforeHumanConfirmation(t *testing.T) {
	s, candidate, run := v102AwaitingHumanRun(t)
	ctx := context.Background()
	var stopJobs int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM node_jobs WHERE allocation_id=$1 AND kind='stop'`, run.AllocationID).Scan(&stopJobs); err != nil || stopJobs != 0 {
		t.Fatalf("premature stop jobs=%d err=%v", stopJobs, err)
	}
	if err := s.ReportInstanceFact(ctx, candidate.NodeID, run.AllocationID, v102ReclaimedFact()); err != nil {
		t.Fatal(err)
	}
	assertV102ReclaimedAllocation(t, s, run)
	run, err := s.ValidationRun(ctx, run.ID)
	if err != nil || run.State != "awaiting_human" || run.HumanResult != "pending" || run.PassedAt != nil {
		t.Fatalf("reclaim bypassed human confirmation: %+v %v", run, err)
	}
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM node_jobs WHERE allocation_id=$1 AND kind='stop'`, run.AllocationID).Scan(&stopJobs); err != nil || stopJobs != 0 {
		t.Fatalf("reclaim created stop job=%d err=%v", stopJobs, err)
	}
	if _, err := s.FinalizeValidationRun(ctx, run.ID); err == nil {
		t.Fatal("reclaimed resource without human confirmation passed")
	}
	run, err = s.FinalizeValidationFailure(ctx, run.ID, "RECLAIMED_BEFORE_CONFIRM")
	if err != nil || run.State != "failed" || run.ResultCode != "FAIL" {
		t.Fatalf("safe failure finalization: %+v %v", run, err)
	}
}

func TestV102QuarantinedRunReclaimedFactStaysFailurePath(t *testing.T) {
	s, candidate, run := v102AwaitingHumanRun(t)
	ctx := context.Background()
	if err := s.ReportInstanceFact(ctx, candidate.NodeID, run.AllocationID, nodev1.InstanceFact{
		InstanceID: "i_validation", Outcome: "identity_unverified", Lifecycle: "unknown",
		Process: "unknown", Cleanup: "unknown", Port: 28000,
	}); err != nil {
		t.Fatal(err)
	}
	run, err := s.ValidationRun(ctx, run.ID)
	if err != nil || run.State != "quarantined" {
		t.Fatalf("quarantine state: %+v %v", run, err)
	}
	var occupied int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM allocations WHERE node_id=$1
		AND state NOT IN ('reclaimed','released_no_effect')`, candidate.NodeID).Scan(&occupied); err != nil || occupied != 1 {
		t.Fatalf("quarantine released capacity=%d err=%v", occupied, err)
	}
	if err := s.ReportInstanceFact(ctx, candidate.NodeID, run.AllocationID, v102ReclaimedFact()); err != nil {
		t.Fatal(err)
	}
	assertV102ReclaimedAllocation(t, s, run)
	run, err = s.ValidationRun(ctx, run.ID)
	if err != nil || run.State != "quarantined" || run.PassedAt != nil {
		t.Fatalf("quarantine returned to PASS path: %+v %v", run, err)
	}
	if _, err := s.FinalizeValidationRun(ctx, run.ID); err == nil {
		t.Fatal("quarantined Run passed after resource reclaim")
	}
	run, err = s.FinalizeValidationFailure(ctx, run.ID, "QUARANTINED_RECLAIMED")
	if err != nil || run.State != "failed" || run.ResultCode != "FAIL" {
		t.Fatalf("quarantine failure finalization: %+v %v", run, err)
	}
}

func TestV102HumanFailFormalStopAndReclaim(t *testing.T) {
	s, candidate, run := v102AwaitingHumanRun(t)
	ctx := context.Background()
	run, err := s.ConfirmValidationHuman(ctx, run.ID, candidate.AdminID, "fail")
	if err != nil || run.State != "stop_pending" || run.HumanResult != "fail" || run.StopJobID == "" {
		t.Fatalf("formal fail stop: %+v %v", run, err)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE node_jobs SET state='claimed' WHERE id=$1`, run.StopJobID); err != nil {
		t.Fatal(err)
	}
	beginV102TestOperation(t, s, candidate.NodeID, run.StopJobID)
	if _, err := s.ReportJob(ctx, candidate.NodeID, run.StopJobID, nodev1.ReportRequest{
		State: "accepted", InstanceID: "i_validation", OperationID: "o_stop",
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.ReportInstanceFact(ctx, candidate.NodeID, run.AllocationID, v102ReclaimedFact()); err != nil {
		t.Fatal(err)
	}
	assertV102ReclaimedAllocation(t, s, run)
	run, err = s.ValidationRun(ctx, run.ID)
	if err != nil || run.State != "reclaim_observing" || run.HumanResult != "fail" {
		t.Fatalf("formal stop observation: %+v %v", run, err)
	}
	if _, err := s.FinalizeValidationFailure(ctx, run.ID, "HUMAN_FAILED"); err == nil {
		t.Fatal("failure finalized with unresolved stop Job")
	}
	if _, err := s.ReportJob(ctx, candidate.NodeID, run.StopJobID, nodev1.ReportRequest{
		State: "succeeded", InstanceID: "i_validation", OperationID: "o_stop",
	}); err != nil {
		t.Fatal(err)
	}
	run, err = s.FinalizeValidationFailure(ctx, run.ID, "HUMAN_FAILED")
	if err != nil || run.State != "failed" || run.ResultCode != "FAIL" || run.PassedAt != nil {
		t.Fatalf("human failure after full reclaim: %+v %v", run, err)
	}
}

func TestV102ReclaimedFactRunStateEdges(t *testing.T) {
	s, candidate := v102ValidationFixture(t, 1)
	ctx := context.Background()
	run, err := s.ReserveValidation(ctx, candidate)
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range []string{
		"create_pending", "create_observing", "create_unknown", "ready_no_join", "awaiting_human",
		"cleanup_required", "stop_pending", "reclaim_observing", "stop_unknown", "quarantined",
	} {
		t.Run(state, func(t *testing.T) {
			tx, err := s.Pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			if _, err := tx.Exec(ctx, `UPDATE validation_runs SET state=$2 WHERE id=$1`, run.ID, state); err != nil {
				t.Fatal(err)
			}
			if err := advanceValidationFromFact(ctx, tx, run.ID, "reclaimed", false); err != nil {
				t.Fatal(err)
			}
			var got string
			if err := tx.QueryRow(ctx, `SELECT state FROM validation_runs WHERE id=$1`, run.ID).Scan(&got); err != nil || got != state {
				t.Fatalf("reclaimed fact moved %s to %s: %v", state, got, err)
			}
		})
	}
}
