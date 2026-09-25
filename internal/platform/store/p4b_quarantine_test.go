package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/L4C99/dota2-arcade-platform/internal/contracts/nodev1"
	"github.com/jackc/pgx/v5"
)

func TestP4BThresholdQuarantineAndAbandonKeepCapacity(t *testing.T) {
	s, nodeID, requestID, allocationID, jobID := p4aReserved(t)
	ctx := context.Background()
	var seconds float64
	if err := s.Pool.QueryRow(ctx, `SELECT EXTRACT(EPOCH FROM quarantine_after_node_unreachable)
		FROM platform_settings WHERE singleton=true`).Scan(&seconds); err != nil || seconds != 900 {
		t.Fatalf("frozen default threshold=%v: %v", seconds, err)
	}
	if err := s.ConfigureQuarantineThreshold(ctx, 2*time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := s.Pool.QueryRow(ctx, `SELECT EXTRACT(EPOCH FROM quarantine_after_node_unreachable)
		FROM platform_settings WHERE singleton=true`).Scan(&seconds); err != nil || seconds != 120 {
		t.Fatalf("configured threshold=%v: %v", seconds, err)
	}
	var ownerID string
	if err := s.Pool.QueryRow(ctx, `SELECT owner_user_id FROM server_requests WHERE id=$1`, requestID).Scan(&ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AbandonQuarantinedUserRequest(ctx, ownerID, requestID); !errors.Is(err, ErrJobConflict) {
		t.Fatalf("non-quarantined abandon: %v", err)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE nodes SET last_heartbeat=now()-interval '1 minute' WHERE id=$1`, nodeID); err != nil {
		t.Fatal(err)
	}
	if changed, err := s.QuarantineOneUnreachable(ctx); err != nil || changed {
		t.Fatalf("premature quarantine: %t %v", changed, err)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE nodes SET last_heartbeat=now()-interval '3 minutes' WHERE id=$1`, nodeID); err != nil {
		t.Fatal(err)
	}
	if changed, err := s.QuarantineOneUnreachable(ctx); err != nil || !changed {
		t.Fatalf("outage quarantine: %t %v", changed, err)
	}
	if changed, err := s.QuarantineOneUnreachable(ctx); err != nil || changed {
		t.Fatalf("repeat quarantine: %t %v", changed, err)
	}
	p4aStates(t, s, requestID, allocationID, "quarantined", "quarantined", 1)
	// A late create acceptance does not clear quarantine or release the slot.
	if _, err := s.PrepareCreate(ctx, nodeID, jobID, "/tmp/p4b-template.json", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReportJob(ctx, nodeID, jobID, nodev1.ReportRequest{State: "accepted", InstanceID: "i_p4b", OperationID: "o_create"}); err != nil {
		t.Fatal(err)
	}
	p4aStates(t, s, requestID, allocationID, "quarantined", "quarantined", 1)
	stranger, _, err := s.CreateUserSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AbandonQuarantinedUserRequest(ctx, stranger, requestID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("foreign abandon: %v", err)
	}
	for i := 0; i < 2; i++ {
		r, err := s.AbandonQuarantinedUserRequest(ctx, ownerID, requestID)
		if err != nil || r.State != "abandoned" {
			t.Fatalf("abandon #%d: %+v %v", i, r, err)
		}
	}
	p4aStates(t, s, requestID, allocationID, "abandoned", "quarantined", 1)
	if current, err := s.CurrentUserRequest(ctx, ownerID); err != nil || current != nil {
		t.Fatalf("old request still blocks: %+v %v", current, err)
	}
	var gameID, presetID, oldNodeID, oldContentID string
	if err := s.Pool.QueryRow(ctx, `SELECT arcade_game_id,game_preset_id FROM server_requests WHERE id=$1`, requestID).Scan(&gameID, &presetID); err != nil {
		t.Fatal(err)
	}
	if err := s.Pool.QueryRow(ctx, `SELECT node_id,content_version_id FROM allocations WHERE id=$1`, allocationID).Scan(&oldNodeID, &oldContentID); err != nil {
		t.Fatal(err)
	}
	newRequest, created, err := s.CreateUserRequest(ctx, ownerID, gameID, presetID)
	if err != nil || !created || newRequest.ID == requestID {
		t.Fatalf("new business request: %+v %t %v", newRequest, created, err)
	}
	if r, created, err := s.CreateUserRequest(ctx, ownerID, gameID, presetID); err != nil || created || r.ID != newRequest.ID {
		t.Fatalf("duplicate new request: %+v %t %v", r, created, err)
	}
	if changed, err := s.TryAllocateOne(ctx); err != nil || changed {
		t.Fatalf("quarantined capacity was reused: %t %v", changed, err)
	}
	var nodeAfter, contentAfter string
	if err := s.Pool.QueryRow(ctx, `SELECT node_id,content_version_id FROM allocations WHERE id=$1`, allocationID).Scan(&nodeAfter, &contentAfter); err != nil ||
		nodeAfter != oldNodeID || contentAfter != oldContentID {
		t.Fatalf("old attempt history changed: node=%s content=%s err=%v", nodeAfter, contentAfter, err)
	}
	p4aStates(t, s, requestID, allocationID, "abandoned", "quarantined", 1)
}

func TestP4BManualQuarantineAndLaterReclaimPreserveAbandonedRequest(t *testing.T) {
	s, nodeID, requestID, allocationID, jobID := p4aReserved(t)
	ctx := context.Background()
	var ownerID string
	if err := s.Pool.QueryRow(ctx, `SELECT owner_user_id FROM server_requests WHERE id=$1`, requestID).Scan(&ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE node_jobs SET state='pending' WHERE id=$1`, jobID); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkAllocationQuarantined(ctx, allocationID); !errors.Is(err, ErrJobConflict) {
		t.Fatalf("pure pending reservation was quarantined: %v", err)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE node_jobs SET state='claimed' WHERE id=$1`, jobID); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkAllocationQuarantined(ctx, allocationID); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkAllocationQuarantined(ctx, allocationID); err != nil {
		t.Fatalf("idempotent manual quarantine: %v", err)
	}
	if _, err := s.AbandonQuarantinedUserRequest(ctx, ownerID, requestID); err != nil {
		t.Fatal(err)
	}
	// Reconciled create identity permits a formal stop job. Repeated or late
	// reports cannot restore the abandoned business request.
	if _, err := s.PrepareCreate(ctx, nodeID, jobID, "/tmp/p4b-template.json", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReportJob(ctx, nodeID, jobID, nodev1.ReportRequest{State: "accepted", InstanceID: "i_p4b", OperationID: "o_create"}); err != nil {
		t.Fatal(err)
	}
	stopJobID, err := NewID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `INSERT INTO node_jobs(id,node_id,kind,integration_only,allocation_id,instance_id,state)
		VALUES($1,$2,'stop',false,$3,'i_p4b','claimed')`, stopJobID, nodeID, allocationID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReportJob(ctx, nodeID, stopJobID, nodev1.ReportRequest{State: "accepted", InstanceID: "i_p4b", OperationID: "o_stop"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReportJob(ctx, nodeID, stopJobID, nodev1.ReportRequest{State: "succeeded", InstanceID: "i_p4b", OperationID: "o_stop"}); err != nil {
		t.Fatal(err)
	}
	p4aStates(t, s, requestID, allocationID, "abandoned", "reclaimed", 0)
	if err := s.MarkAllocationQuarantined(ctx, allocationID); !errors.Is(err, ErrJobConflict) {
		t.Fatalf("reclaimed attempt was quarantined again: %v", err)
	}
}

func TestP4BPartyMemberCannotAbandon(t *testing.T) {
	s := playerTestStore(t)
	ctx := context.Background()
	if err := s.ConfigurePartySize(ctx, 3); err != nil {
		t.Fatal(err)
	}
	gameID, presetID := seedPlayerCatalog(t, s)
	leader, member := partyUser(t, s), partyUser(t, s)
	if _, err := s.CreateParty(ctx, leader); err != nil {
		t.Fatal(err)
	}
	invite, err := s.CurrentInvite(ctx, leader)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.JoinParty(ctx, member, invite.Token); err != nil {
		t.Fatal(err)
	}
	r, _, err := s.CreateUserRequest(ctx, leader, gameID, presetID)
	if err != nil {
		t.Fatal(err)
	}
	nodeID, _, err := s.RegisterNode(ctx, "party node", "linux")
	if err != nil {
		t.Fatal(err)
	}
	allocationID, err := NewID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `INSERT INTO allocations(id,server_request_id,arcade_game_id,attempt_sequence,node_id,
		content_version_id,template_revision_id,state,assigned_at)
		VALUES($1,$2,$3,1,$4,'test-v1','test-template','quarantined',now())`, allocationID, r.ID, gameID, nodeID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE server_requests SET state='quarantined' WHERE id=$1`, r.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AbandonQuarantinedUserRequest(ctx, member, r.ID); !errors.Is(err, ErrPartyForbidden) {
		t.Fatalf("member abandoned party server: %v", err)
	}
	if out, err := s.AbandonQuarantinedUserRequest(ctx, leader, r.ID); err != nil || out.State != "abandoned" {
		t.Fatalf("leader abandon: %+v %v", out, err)
	}
}
