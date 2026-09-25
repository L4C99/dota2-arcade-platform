package store

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/L4C99/dota2-arcade-platform/internal/contracts/nodev1"
)

func p4cRunning(t *testing.T) (*Store, string, string, string, string) {
	t.Helper()
	s, nodeID, requestID, allocationID, jobID := p4aReserved(t)
	ctx := context.Background()
	if _, err := s.PrepareCreate(ctx, nodeID, jobID, "/tmp/p4c-template.json", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReportJob(ctx, nodeID, jobID, nodev1.ReportRequest{State: "accepted", InstanceID: "i_p4c", OperationID: "o_create"}); err != nil {
		t.Fatal(err)
	}
	join := &nodev1.JoinInfo{LocalPort: 28000, PublicPort: 28000, ConnectHost: "127.0.0.1",
		EntryConfigRevision: nodev1.EntryConfigRevision(p1TestHeartbeat("test-v1").Network)}
	if _, err := s.ReportJob(ctx, nodeID, jobID, nodev1.ReportRequest{State: "succeeded", InstanceID: "i_p4c", OperationID: "o_create", JoinInfo: join}); err != nil {
		t.Fatal(err)
	}
	var ownerID string
	if err := s.Pool.QueryRow(ctx, `SELECT owner_user_id FROM server_requests WHERE id=$1`, requestID).Scan(&ownerID); err != nil {
		t.Fatal(err)
	}
	return s, nodeID, ownerID, requestID, allocationID
}

func TestP4CNextGameConsumesOnceAfterFullReclaim(t *testing.T) {
	s, nodeID, ownerID, requestID, allocationID := p4cRunning(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	errs := make([]error, 5)
	for i := range errs {
		wg.Add(1)
		go func(i int) { defer wg.Done(); _, errs[i] = s.NextGameUserRequest(ctx, ownerID, requestID) }(i)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatalf("double-click: %v", err)
		}
	}
	p4aStates(t, s, requestID, allocationID, "stopping", "stopping", 1)
	if intent, err := s.UserNextGameIntent(ctx, ownerID, requestID); err != nil || intent == nil || intent.State != "pending" {
		t.Fatalf("pending intent: %+v %v", intent, err)
	}
	var stopJobs int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM node_jobs WHERE allocation_id=$1 AND kind='stop'`, allocationID).Scan(&stopJobs); err != nil || stopJobs != 1 {
		t.Fatalf("stop jobs=%d: %v", stopJobs, err)
	}
	stopJob, err := s.ClaimNextJob(ctx, nodeID)
	if err != nil || stopJob == nil || stopJob.Kind != "stop" {
		t.Fatalf("claim stop %+v: %v", stopJob, err)
	}
	if _, err := s.ReportJob(ctx, nodeID, stopJob.ID, nodev1.ReportRequest{State: "unknown", InstanceID: "i_p4c"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReportJob(ctx, nodeID, stopJob.ID, nodev1.ReportRequest{State: "accepted", InstanceID: "i_p4c", OperationID: "o_stop"}); err != nil {
		t.Fatal(err)
	}
	if intent, err := s.UserNextGameIntent(ctx, ownerID, requestID); err != nil || intent.State != "pending" || intent.NewRequestID != nil {
		t.Fatalf("early request: %+v %v", intent, err)
	}
	for i := 0; i < 3; i++ {
		if _, err := s.ReportJob(ctx, nodeID, stopJob.ID, nodev1.ReportRequest{State: "succeeded", InstanceID: "i_p4c", OperationID: "o_stop"}); err != nil {
			t.Fatalf("reclaim report #%d: %v", i, err)
		}
	}
	p4aStates(t, s, requestID, allocationID, "ended", "reclaimed", 0)
	intent, err := s.UserNextGameIntent(ctx, ownerID, requestID)
	if err != nil || intent == nil || intent.State != "consumed" || intent.NewRequestID == nil {
		t.Fatalf("consumed intent: %+v %v", intent, err)
	}
	var count int
	var newAt, reclaimedAt time.Time
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM server_requests WHERE owner_user_id=$1`, ownerID).Scan(&count); err != nil || count != 2 {
		t.Fatalf("requests=%d: %v", count, err)
	}
	if err := s.Pool.QueryRow(ctx, `SELECT requested_at FROM server_requests WHERE id=$1`, *intent.NewRequestID).Scan(&newAt); err != nil {
		t.Fatal(err)
	}
	if err := s.Pool.QueryRow(ctx, `SELECT reclaimed_at FROM allocations WHERE id=$1`, allocationID).Scan(&reclaimedAt); err != nil {
		t.Fatal(err)
	}
	if newAt.Before(reclaimedAt) {
		t.Fatalf("new request predates reclaim: %s < %s", newAt, reclaimedAt)
	}
	var gameID string
	if err := s.Pool.QueryRow(ctx, `SELECT arcade_game_id FROM server_requests WHERE id=$1`, requestID).Scan(&gameID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `INSERT INTO content_versions(id,arcade_game_id,content_sha256) VALUES('test-v2',$1,$2)`, gameID, strings.Repeat("b", 64)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE arcade_games SET current_content_version_id='test-v2' WHERE id=$1`, gameID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecordHeartbeat(ctx, nodeID, p1TestHeartbeat("test-v2")); err != nil {
		t.Fatal(err)
	}
	if changed, err := s.TryAllocateOne(ctx); err != nil || !changed {
		t.Fatalf("next allocation: %t %v", changed, err)
	}
	var version string
	if err := s.Pool.QueryRow(ctx, `SELECT content_version_id FROM allocations WHERE server_request_id=$1`, *intent.NewRequestID).Scan(&version); err != nil || version != "test-v2" {
		t.Fatalf("new content=%s: %v", version, err)
	}
}

func TestP4CQuarantinePausesThenAbandonConsumesOnce(t *testing.T) {
	s, nodeID, ownerID, requestID, allocationID := p4cRunning(t)
	ctx := context.Background()
	if _, err := s.NextGameUserRequest(ctx, ownerID, requestID); err != nil {
		t.Fatal(err)
	}
	stopJob, err := s.ClaimNextJob(ctx, nodeID)
	if err != nil || stopJob == nil {
		t.Fatalf("claim: %+v %v", stopJob, err)
	}
	if _, err := s.ReportJob(ctx, nodeID, stopJob.ID, nodev1.ReportRequest{State: "accepted", InstanceID: "i_p4c", OperationID: "o_stop"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReportJob(ctx, nodeID, stopJob.ID, nodev1.ReportRequest{State: "failed_with_effect", InstanceID: "i_p4c", OperationID: "o_stop", ErrorCode: "CLEANUP_FAILED", ErrorStage: "cleanup"}); err != nil {
		t.Fatal(err)
	}
	p4aStates(t, s, requestID, allocationID, "quarantined", "quarantined", 1)
	intent, err := s.UserNextGameIntent(ctx, ownerID, requestID)
	if err != nil || intent.State != "paused" || intent.NewRequestID != nil {
		t.Fatalf("paused intent: %+v %v", intent, err)
	}
	other, _, err := s.CreateUserSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AbandonQuarantinedUserRequest(ctx, other, requestID); err == nil {
		t.Fatal("foreign abandon accepted")
	}
	for i := 0; i < 2; i++ {
		if _, err := s.AbandonQuarantinedUserRequest(ctx, ownerID, requestID); err != nil {
			t.Fatal(err)
		}
	}
	p4aStates(t, s, requestID, allocationID, "abandoned", "quarantined", 1)
	intent, err = s.UserNextGameIntent(ctx, ownerID, requestID)
	if err != nil || intent.State != "consumed" || intent.NewRequestID == nil {
		t.Fatalf("continued intent: %+v %v", intent, err)
	}
	var count int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM server_requests WHERE owner_user_id=$1`, ownerID).Scan(&count); err != nil || count != 2 {
		t.Fatalf("requests=%d: %v", count, err)
	}
	if changed, err := s.TryAllocateOne(ctx); err != nil || changed {
		t.Fatalf("quarantined slot reused: %t %v", changed, err)
	}
}

func TestP4COrdinaryStopDoesNotCreateNextGameIntent(t *testing.T) {
	s, nodeID, ownerID, requestID, _ := p4cRunning(t)
	ctx := context.Background()
	if _, err := s.StopUserRequest(ctx, ownerID, requestID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.NextGameUserRequest(ctx, ownerID, requestID); !errors.Is(err, ErrJobConflict) {
		t.Fatalf("late next game: %v", err)
	}
	stopJob, err := s.ClaimNextJob(ctx, nodeID)
	if err != nil || stopJob == nil {
		t.Fatalf("claim: %+v %v", stopJob, err)
	}
	if _, err := s.ReportJob(ctx, nodeID, stopJob.ID, nodev1.ReportRequest{State: "accepted", InstanceID: "i_p4c", OperationID: "o_stop"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReportJob(ctx, nodeID, stopJob.ID, nodev1.ReportRequest{State: "succeeded", InstanceID: "i_p4c", OperationID: "o_stop"}); err != nil {
		t.Fatal(err)
	}
	intent, err := s.UserNextGameIntent(ctx, ownerID, requestID)
	if err != nil || intent != nil {
		t.Fatalf("unexpected intent: %+v %v", intent, err)
	}
}

func TestP4CManualNodeInheritedAfterReclaim(t *testing.T) {
	s, nodeID, ownerID, requestID, _ := p4cRunning(t)
	ctx := context.Background()
	if _, err := s.Pool.Exec(ctx, `UPDATE server_requests SET node_selection_mode='manual',manual_node_id=$2 WHERE id=$1`, requestID, nodeID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.NextGameUserRequest(ctx, ownerID, requestID); err != nil {
		t.Fatal(err)
	}
	stopJob, err := s.ClaimNextJob(ctx, nodeID)
	if err != nil || stopJob == nil {
		t.Fatalf("claim: %+v %v", stopJob, err)
	}
	for _, state := range []string{"accepted", "succeeded"} {
		if _, err := s.ReportJob(ctx, nodeID, stopJob.ID, nodev1.ReportRequest{State: state, InstanceID: "i_p4c", OperationID: "o_stop"}); err != nil {
			t.Fatal(err)
		}
	}
	intent, err := s.UserNextGameIntent(ctx, ownerID, requestID)
	if err != nil || intent.NewRequestID == nil {
		t.Fatalf("intent: %+v %v", intent, err)
	}
	var mode, inheritedNode string
	if err := s.Pool.QueryRow(ctx, `SELECT node_selection_mode,manual_node_id FROM server_requests WHERE id=$1`, *intent.NewRequestID).Scan(&mode, &inheritedNode); err != nil || mode != "manual" || inheritedNode != nodeID {
		t.Fatalf("inherit mode=%s node=%s: %v", mode, inheritedNode, err)
	}
}

func TestP4CPartyMemberCannotRequestNextGame(t *testing.T) {
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
	nodeID, _, err := s.RegisterNode(ctx, "p4c party node", "linux")
	if err != nil {
		t.Fatal(err)
	}
	allocationID, err := NewID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `INSERT INTO allocations(id,server_request_id,arcade_game_id,attempt_sequence,node_id,content_version_id,template_revision_id,state,assigned_at)
		VALUES($1,$2,$3,1,$4,'test-v1','test-template','running',now())`, allocationID, r.ID, gameID, nodeID); err != nil {
		t.Fatal(err)
	}
	jobID, err := NewID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `INSERT INTO node_jobs(id,node_id,kind,integration_only,allocation_id,instance_id,state)
		VALUES($1,$2,'create',false,$3,'i_party','succeeded')`, jobID, nodeID, allocationID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE server_requests SET state='running' WHERE id=$1`, r.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.NextGameUserRequest(ctx, member, r.ID); !errors.Is(err, ErrPartyForbidden) {
		t.Fatalf("member next game: %v", err)
	}
	if _, err := s.NextGameUserRequest(ctx, leader, r.ID); err != nil {
		t.Fatalf("leader next game: %v", err)
	}
}
