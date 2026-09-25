package store

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/L4C99/dota2-arcade-platform/internal/contracts/nodev1"
)

func TestP3CNodeConnectivityWindows(t *testing.T) {
	now := time.Now().UTC()
	for _, tc := range []struct {
		name string
		age  time.Duration
		want string
	}{
		{"recent", time.Second, "online"},
		{"before-stale", nodeOnlineWindow - time.Second, "online"},
		{"stale", nodeOnlineWindow, "stale"},
		{"before-offline", nodeOfflineWindow - time.Second, "stale"},
		{"offline", nodeOfflineWindow, "offline"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			last := now.Add(-tc.age)
			if got := NodeConnectivity(&last, now); got != tc.want {
				t.Fatalf("connectivity = %s, want %s", got, tc.want)
			}
		})
	}
	if got := NodeConnectivity(nil, now); got != "offline" {
		t.Fatalf("missing heartbeat = %s", got)
	}
}

func TestP3CStaleOfflineRetainAndReconnect(t *testing.T) {
	s := playerTestStore(t)
	ctx := context.Background()
	gameID, presetID := seedPlayerCatalog(t, s)
	nodeID := p3CapacityNode(t, s, "recovering", 1)
	r := p3Request(t, s, gameID, presetID, "auto", "")
	if changed, err := s.TryAllocateOne(ctx); err != nil || !changed {
		t.Fatalf("initial reserve: %t %v", changed, err)
	}
	var jobID, allocationID string
	if err := s.Pool.QueryRow(ctx, `SELECT j.id,a.id FROM node_jobs j JOIN allocations a ON a.id=j.allocation_id
		WHERE a.server_request_id=$1`, r.ID).Scan(&jobID, &allocationID); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		age  string
		want string
	}{{"3 minutes", "stale"}, {"6 minutes", "offline"}} {
		if _, err := s.Pool.Exec(ctx, `UPDATE nodes SET last_heartbeat=now()-$2::interval WHERE id=$1`, nodeID, tc.age); err != nil {
			t.Fatal(err)
		}
		choices, err := s.PlayerNodeChoices(ctx, gameID, presetID)
		if err != nil || len(choices) != 1 || choices[0].Connectivity != tc.want || choices[0].Reason != "unreachable" {
			t.Fatalf("%s choices %+v: %v", tc.want, choices, err)
		}
		job, err := s.ClaimNextJob(ctx, nodeID)
		if err != nil || job != nil {
			t.Fatalf("%s claimed new job: %+v %v", tc.want, job, err)
		}
		capacity, err := s.Capacity(ctx, nodeID)
		if err != nil || capacity.Occupied != 1 {
			t.Fatalf("%s released old capacity: %+v %v", tc.want, capacity, err)
		}
		if changed, err := s.TryAllocateOne(ctx); err != nil || changed {
			t.Fatalf("%s reassigned existing request: %t %v", tc.want, changed, err)
		}
	}
	if _, err := s.RecordHeartbeat(ctx, nodeID, p1TestHeartbeat("test-v1")); err != nil {
		t.Fatal(err)
	}
	job, err := s.ClaimNextJob(ctx, nodeID)
	if err != nil || job == nil || job.ID != jobID {
		t.Fatalf("reconnect did not reclaim durable job: %+v %v", job, err)
	}
	var attempts int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM allocations WHERE server_request_id=$1`, r.ID).Scan(&attempts); err != nil || attempts != 1 {
		t.Fatalf("reconnect duplicated allocation %s: %d %v", allocationID, attempts, err)
	}
}

func TestP3CUnknownBlocksUntilNoEffectThenNewAttempt(t *testing.T) {
	s := playerTestStore(t)
	ctx := context.Background()
	gameID, presetID := seedPlayerCatalog(t, s)
	a := p3CapacityNode(t, s, "A", 1)
	b := p3CapacityNode(t, s, "B", 1)
	if err := s.SetNodePriority(ctx, a, 10); err != nil {
		t.Fatal(err)
	}
	r := p3Request(t, s, gameID, presetID, "auto", "")
	if changed, err := s.TryAllocateOne(ctx); err != nil || !changed {
		t.Fatalf("initial reserve: %t %v", changed, err)
	}
	if got := allocatedNode(t, s, r.ID); got != a {
		t.Fatalf("first node = %s", got)
	}
	job, err := s.ClaimNextJob(ctx, a)
	if err != nil || job == nil {
		t.Fatalf("first claim: %+v %v", job, err)
	}
	if _, err := s.PrepareCreate(ctx, a, job.ID, "/tmp/trusted-template.json", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReportJob(ctx, a, job.ID, nodev1.ReportRequest{State: "unknown"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE nodes SET last_heartbeat=now()-interval '6 minutes' WHERE id=$1`, a); err != nil {
		t.Fatal(err)
	}
	if changed, err := s.TryAllocateOne(ctx); err != nil || changed {
		t.Fatalf("unknown duplicated create on B: %t %v", changed, err)
	}
	var attempts int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM allocations WHERE server_request_id=$1`, r.ID).Scan(&attempts); err != nil || attempts != 1 {
		t.Fatalf("unknown attempts=%d: %v", attempts, err)
	}
	capacity, err := s.Capacity(ctx, a)
	if err != nil || capacity.Occupied != 1 {
		t.Fatalf("unknown released A capacity: %+v %v", capacity, err)
	}
	// The recovered Controller must first reconcile core list/operation/status.
	// This explicit no-effect report stands in for that positively proven fact.
	if _, err := s.RecordHeartbeat(ctx, a, p1TestHeartbeat("test-v1")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReportJob(ctx, a, job.ID, nodev1.ReportRequest{State: "rejected_no_effect", ErrorCode: "RECONCILED_NO_EFFECT", ErrorStage: "reconcile"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `INSERT INTO content_versions(id,arcade_game_id,content_sha256) VALUES('test-v2',$1,$2)`, gameID, strings.Repeat("b", 64)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE arcade_games SET current_content_version_id='test-v2' WHERE id=$1`, gameID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecordHeartbeat(ctx, b, p1TestHeartbeat("test-v2")); err != nil {
		t.Fatal(err)
	}
	if changed, err := s.TryAllocateOne(ctx); err != nil || !changed {
		t.Fatalf("safe second attempt: %t %v", changed, err)
	}
	rows, err := s.Pool.Query(ctx, `SELECT attempt_sequence,node_id,content_version_id,state FROM allocations WHERE server_request_id=$1 ORDER BY attempt_sequence`, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	type attempt struct {
		sequence             int
		node, content, state string
	}
	var got []attempt
	for rows.Next() {
		var x attempt
		if err := rows.Scan(&x.sequence, &x.node, &x.content, &x.state); err != nil {
			t.Fatal(err)
		}
		got = append(got, x)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != (attempt{1, a, "test-v1", "released_no_effect"}) || got[1] != (attempt{2, b, "test-v2", "reserved"}) {
		t.Fatalf("unsafe or rewritten attempt history: %+v", got)
	}
	capacity, err = s.Capacity(ctx, a)
	if err != nil || capacity.Occupied != 0 {
		t.Fatalf("proven no-effect still occupies A: %+v %v", capacity, err)
	}
	capacity, err = s.Capacity(ctx, b)
	if err != nil || capacity.Occupied != 1 {
		t.Fatalf("second attempt did not reserve B: %+v %v", capacity, err)
	}
}

func TestP3CManualNoEffectNeverMovesToOtherNode(t *testing.T) {
	s := playerTestStore(t)
	ctx := context.Background()
	gameID, presetID := seedPlayerCatalog(t, s)
	a := p3CapacityNode(t, s, "manual A", 1)
	b := p3CapacityNode(t, s, "other B", 1)
	r := p3Request(t, s, gameID, presetID, "manual", a)
	if changed, err := s.TryAllocateOne(ctx); err != nil || !changed {
		t.Fatalf("manual reserve: %t %v", changed, err)
	}
	job, err := s.ClaimNextJob(ctx, a)
	if err != nil || job == nil {
		t.Fatalf("manual claim: %+v %v", job, err)
	}
	if _, err := s.ReportJob(ctx, a, job.ID, nodev1.ReportRequest{State: "rejected_no_effect", ErrorCode: "INVALID_TEMPLATE", ErrorStage: "validate"}); err != nil {
		t.Fatal(err)
	}
	if changed, err := s.TryAllocateOne(ctx); err != nil || changed {
		t.Fatalf("manual no-effect moved to other node: %t %v", changed, err)
	}
	var state string
	var attempts int
	if err := s.Pool.QueryRow(ctx, `SELECT state FROM server_requests WHERE id=$1`, r.ID).Scan(&state); err != nil || state != "unavailable" {
		t.Fatalf("manual request state=%s: %v", state, err)
	}
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM allocations WHERE server_request_id=$1 AND node_id=$2`, r.ID, b).Scan(&attempts); err != nil || attempts != 0 {
		t.Fatalf("manual allocation on B=%d: %v", attempts, err)
	}
}
