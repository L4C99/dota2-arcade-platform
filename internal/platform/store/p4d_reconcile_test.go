package store

import (
	"context"
	"errors"
	"testing"

	"github.com/L4C99/dota2-arcade-platform/internal/contracts/nodev1"
)

func TestP4DActiveInstanceReconcileAndAdminRequest(t *testing.T) {
	s, nodeID, _, requestID, allocationID := p4cRunning(t)
	ctx := context.Background()
	adminID, err := s.CreateAdmin(ctx, "p4d-reconcile", "a long test password")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyAdminAction(ctx, adminID, AdminAction{Action: "node.reconcile", TargetID: nodeID}); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyAdminAction(ctx, adminID, AdminAction{Action: "node.reconcile", TargetID: nodeID}); err != nil {
		t.Fatal(err)
	}
	var requested, completed int64
	if err := s.Pool.QueryRow(ctx, `SELECT requested_generation,completed_generation FROM node_reconcile_requests WHERE node_id=$1`, nodeID).Scan(&requested, &completed); err != nil || requested != 2 || completed != 0 {
		t.Fatalf("generations %d/%d: %v", requested, completed, err)
	}
	beat, err := s.RecordHeartbeat(ctx, nodeID, p1TestHeartbeat("test-v1"))
	if err != nil || beat.ReconcileRequestedGeneration != 2 || beat.ReconcileCompletedGeneration != 0 {
		t.Fatalf("heartbeat generation: %+v %v", beat, err)
	}
	if err := s.CompleteNodeReconcile(ctx, nodeID, 1); !errors.Is(err, ErrJobConflict) {
		t.Fatalf("stale ack: %v", err)
	}
	if err := s.CompleteNodeReconcile(ctx, nodeID, 2); err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteNodeReconcile(ctx, nodeID, 2); err != nil {
		t.Fatalf("lost completion response retry: %v", err)
	}
	allocations, err := s.ActiveAllocationsForNode(ctx, nodeID)
	if err != nil || len(allocations) != 1 || allocations[0].ID != allocationID || allocations[0].InstanceID != "i_p4c" || allocations[0].HasOpenJob {
		t.Fatalf("active: %+v %v", allocations, err)
	}
	active := nodev1.InstanceFact{InstanceID: "i_p4c", Outcome: "active", Lifecycle: "active", Process: "running", Cleanup: "pending", Port: 28000}
	if err := s.ReportInstanceFact(ctx, nodeID, allocationID, active); err != nil {
		t.Fatal(err)
	}
	p4aStates(t, s, requestID, allocationID, "running", "running", 1)
	active.Port = 28001
	if err := s.ReportInstanceFact(ctx, nodeID, allocationID, active); err != nil {
		t.Fatal(err)
	}
	p4aStates(t, s, requestID, allocationID, "quarantined", "quarantined", 1)
	bad := nodev1.InstanceFact{InstanceID: "i_p4c", Outcome: "reclaimed", Lifecycle: "reclaimed", Process: "running", Cleanup: "complete", Port: 28000}
	if err := s.ReportInstanceFact(ctx, nodeID, allocationID, bad); !errors.Is(err, ErrJobConflict) {
		t.Fatalf("incomplete reclaim: %v", err)
	}
	p4aStates(t, s, requestID, allocationID, "quarantined", "quarantined", 1)
	good := nodev1.InstanceFact{InstanceID: "i_p4c", Outcome: "reclaimed", Lifecycle: "reclaimed", Process: "stopped", Cleanup: "complete", Port: 28000}
	if err := s.ReportInstanceFact(ctx, nodeID, allocationID, good); err != nil {
		t.Fatal(err)
	}
	if err := s.ReportInstanceFact(ctx, nodeID, allocationID, good); err != nil {
		t.Fatalf("repeated reclaim: %v", err)
	}
	p4aStates(t, s, requestID, allocationID, "ended", "reclaimed", 0)
	var oldPort int
	if err := s.Pool.QueryRow(ctx, `SELECT join_local_port FROM allocations WHERE id=$1`, allocationID).Scan(&oldPort); err != nil || oldPort != 28000 {
		t.Fatalf("join port changed: %d %v", oldPort, err)
	}
	allocations, err = s.ActiveAllocationsForNode(ctx, nodeID)
	if err != nil || len(allocations) != 0 {
		t.Fatalf("terminal still active: %+v %v", allocations, err)
	}
}

func TestP4DReconcileReclaimDoesNotConsumePausedNextGame(t *testing.T) {
	s, nodeID, ownerID, requestID, allocationID := p4cRunning(t)
	ctx := context.Background()
	if _, err := s.NextGameUserRequest(ctx, ownerID, requestID); err != nil {
		t.Fatal(err)
	}
	stopJob, err := s.ClaimNextJob(ctx, nodeID)
	if err != nil || stopJob == nil {
		t.Fatalf("stop claim: %+v %v", stopJob, err)
	}
	if _, err := s.ReportJob(ctx, nodeID, stopJob.ID, nodev1.ReportRequest{State: "accepted", InstanceID: "i_p4c", OperationID: "o_stop"}); err != nil {
		t.Fatal(err)
	}
	adminID, err := s.CreateAdmin(ctx, "paused-reconcile", "a long test password")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyAdminAction(ctx, adminID, AdminAction{Action: "allocation.quarantine", TargetID: allocationID}); err != nil {
		t.Fatal(err)
	}
	fact := nodev1.InstanceFact{InstanceID: "i_p4c", Outcome: "reclaimed", Lifecycle: "reclaimed", Process: "stopped", Cleanup: "complete", Port: 28000}
	if err := s.ReportInstanceFact(ctx, nodeID, allocationID, fact); err != nil {
		t.Fatal(err)
	}
	p4aStates(t, s, requestID, allocationID, "quarantined", "reclaimed", 0)
	if _, err := s.ReportJob(ctx, nodeID, stopJob.ID, nodev1.ReportRequest{State: "succeeded", InstanceID: "i_p4c", OperationID: "o_stop"}); err != nil {
		t.Fatal(err)
	}
	p4aStates(t, s, requestID, allocationID, "quarantined", "reclaimed", 0)
	intent, err := s.UserNextGameIntent(ctx, ownerID, requestID)
	if err != nil || intent.State != "paused" || intent.NewRequestID != nil {
		t.Fatalf("intent: %+v %v", intent, err)
	}
	if _, err := s.AbandonQuarantinedUserRequest(ctx, ownerID, requestID); err != nil {
		t.Fatal(err)
	}
	intent, err = s.UserNextGameIntent(ctx, ownerID, requestID)
	if err != nil || intent.State != "consumed" || intent.NewRequestID == nil {
		t.Fatalf("continue: %+v %v", intent, err)
	}
}
