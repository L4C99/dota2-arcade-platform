package store

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/L4C99/dota2-arcade-platform/internal/contracts/nodev1"
	"github.com/jackc/pgx/v5"
)

func p1TestHeartbeat(version string) nodev1.Heartbeat {
	return nodev1.Heartbeat{OS: "linux", ControllerVersion: "test", NodeAPIVersion: nodev1.APIVersion,
		D2CoreVersion: nodev1.D2CoreVersion, D2CoreCommit: nodev1.D2CoreCommit,
		D2CoreProtocolVersion: nodev1.D2CoreProtocolVersion, HardMaxInstances: 1,
		Network: nodev1.NetworkFacts{ConnectHost: "127.0.0.1", LocalPortMin: 28000, LocalPortMax: 28000, MappingMode: "identity"},
		Content: []nodev1.ContentFact{{WorkshopID: "3564393242", ContentVersionID: version, State: "confirmed"}}}
}

func TestP1BAllocationCapacityAndLifecycle(t *testing.T) {
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
	if _, err := s.Pool.Exec(ctx, `INSERT INTO content_versions(id,arcade_game_id,content_sha256)
		VALUES('test-v2',$1,$2)`, gameID, strings.Repeat("b", 64)); err != nil {
		t.Fatal(err)
	}
	firstUser, _, err := s.CreateUserSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	secondUser, _, err := s.CreateUserSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	first, _, err := s.CreateUserRequest(ctx, firstUser, gameID, presetID)
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := s.CreateUserRequest(ctx, secondUser, gameID, presetID)
	if err != nil {
		t.Fatal(err)
	}
	// Waiting requests do not freeze the content version.
	if _, err := s.Pool.Exec(ctx, `UPDATE arcade_games SET current_content_version_id='test-v2' WHERE id=$1`, gameID); err != nil {
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
	if changed, err := s.TryAllocateOne(ctx); err != nil || changed {
		t.Fatalf("mismatched content scheduled: %t %v", changed, err)
	}
	empty := p1TestHeartbeat("test-v2")
	empty.Content = []nodev1.ContentFact{}
	if _, err := s.RecordHeartbeat(ctx, nodeID, empty); err != nil {
		t.Fatal(err)
	}
	if changed, err := s.TryAllocateOne(ctx); err != nil || changed {
		t.Fatalf("unknown content scheduled: %t %v", changed, err)
	}
	if _, err := s.RecordHeartbeat(ctx, nodeID, p1TestHeartbeat("test-v2")); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make([]error, 4)
	for i := range errs {
		wg.Add(1)
		go func(i int) { defer wg.Done(); _, errs[i] = s.TryAllocateOne(ctx) }(i)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM allocations`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("allocations=%d: %v", count, err)
	}
	var allocationID, allocatedRequestID, contentID string
	if err := s.Pool.QueryRow(ctx, `SELECT id,server_request_id,content_version_id FROM allocations`).Scan(&allocationID, &allocatedRequestID, &contentID); err != nil {
		t.Fatal(err)
	}
	if contentID != "test-v2" {
		t.Fatalf("waiting version was frozen early: %s", contentID)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE allocations SET content_version_id='test-v1' WHERE id=$1`, allocationID); err == nil {
		t.Fatal("Allocation content version changed")
	}
	owner := firstUser
	waitingUser := secondUser
	waitingRequest := second
	if allocatedRequestID == second.ID {
		owner, waitingUser, waitingRequest = secondUser, firstUser, first
	}
	if _, err := s.StopUserRequest(ctx, waitingUser, allocatedRequestID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("other user stopped server: %v", err)
	}
	job, err := s.ClaimNextJob(ctx, nodeID)
	if err != nil || job == nil || job.IntegrationOnly {
		t.Fatalf("business create job %+v: %v", job, err)
	}
	if _, err := s.PrepareCreate(ctx, nodeID, job.ID, "/tmp/trusted-template.json", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReportJob(ctx, nodeID, job.ID, nodev1.ReportRequest{State: "unknown"}); err != nil {
		t.Fatal(err)
	}
	if changed, err := s.TryAllocateOne(ctx); err != nil || changed {
		t.Fatalf("unknown create released capacity: %t %v", changed, err)
	}
	if _, err := s.ReportJob(ctx, nodeID, job.ID, nodev1.ReportRequest{State: "accepted", InstanceID: "i_test1", OperationID: "o_test1"}); err != nil {
		t.Fatal(err)
	}
	join := &nodev1.JoinInfo{LocalPort: 28000, PublicPort: 28000, ConnectHost: "127.0.0.1",
		EntryConfigRevision: nodev1.EntryConfigRevision(p1TestHeartbeat("test-v2").Network)}
	if _, err := s.ReportJob(ctx, nodeID, job.ID, nodev1.ReportRequest{State: "succeeded", InstanceID: "i_test1", OperationID: "o_test1", JoinInfo: join}); err != nil {
		t.Fatal(err)
	}
	var started, ready bool
	if err := s.Pool.QueryRow(ctx, `SELECT create_started_at IS NOT NULL,ready_at IS NOT NULL FROM allocations WHERE id=$1`, allocationID).Scan(&started, &ready); err != nil || !started || !ready {
		t.Fatalf("timestamps start=%t ready=%t: %v", started, ready, err)
	}
	visible, err := s.UserRequestAllocation(ctx, owner, allocatedRequestID)
	if err != nil || visible.JoinInfo == nil || visible.JoinInfo.ConnectCommand != "connect 127.0.0.1:28000" ||
		visible.JoinInfo.SteamURI != "" || visible.JoinInfo.SteamChinaURI != "" || visible.JoinInfoAvailableAt == nil {
		t.Fatalf("unverified URI or missing connect: %+v %v", visible, err)
	}
	if changed, err := s.TryAllocateOne(ctx); err != nil || changed {
		t.Fatalf("capacity oversold before stop: %t %v", changed, err)
	}
	if _, err := s.StopUserRequest(ctx, owner, allocatedRequestID); err != nil {
		t.Fatal(err)
	}
	if changed, err := s.TryAllocateOne(ctx); err != nil || changed {
		t.Fatalf("capacity released during stopping: %t %v", changed, err)
	}
	stopJob, err := s.ClaimNextJob(ctx, nodeID)
	if err != nil || stopJob == nil || stopJob.Kind != "stop" {
		t.Fatalf("stop job %+v: %v", stopJob, err)
	}
	if _, err := s.ReportJob(ctx, nodeID, stopJob.ID, nodev1.ReportRequest{State: "accepted", InstanceID: "i_test1", OperationID: "o_stop1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReportJob(ctx, nodeID, stopJob.ID, nodev1.ReportRequest{State: "succeeded", InstanceID: "i_test1", OperationID: "o_stop1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE arcade_games SET current_content_version_id='test-v1' WHERE id=$1`, gameID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecordHeartbeat(ctx, nodeID, p1TestHeartbeat("test-v1")); err != nil {
		t.Fatal(err)
	}
	if changed, err := s.TryAllocateOne(ctx); err != nil || !changed {
		t.Fatalf("capacity not released after reclaim: %t %v", changed, err)
	}
	var secondContent string
	if err := s.Pool.QueryRow(ctx, `SELECT content_version_id FROM allocations WHERE server_request_id=$1`, waitingRequest.ID).Scan(&secondContent); err != nil || secondContent != "test-v1" {
		t.Fatalf("second attempt content=%s: %v", secondContent, err)
	}
	secondJob, err := s.ClaimNextJob(ctx, nodeID)
	if err != nil || secondJob == nil {
		t.Fatalf("second job %+v: %v", secondJob, err)
	}
	if _, err := s.ReportJob(ctx, nodeID, secondJob.ID, nodev1.ReportRequest{State: "rejected_no_effect", ErrorCode: "LOCAL_TEMPLATE_BINDING", ErrorStage: "validate"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM allocations WHERE state NOT IN ('reclaimed','released_no_effect')`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("capacity after no-effect=%d: %v", count, err)
	}
}
