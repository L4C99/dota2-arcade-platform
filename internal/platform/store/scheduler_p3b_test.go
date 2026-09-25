package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func p3Request(t *testing.T, s *Store, gameID, presetID, mode, nodeID string) ServerRequest {
	t.Helper()
	userID, _, err := s.CreateUserSession(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	r, created, err := s.CreateUserRequestSelected(context.Background(), userID, gameID, presetID, mode, nodeID)
	if err != nil || !created {
		t.Fatalf("create request: %+v %v", r, err)
	}
	return r
}

func allocatedNode(t *testing.T, s *Store, requestID string) string {
	t.Helper()
	var nodeID string
	if err := s.Pool.QueryRow(context.Background(), `SELECT node_id FROM allocations WHERE server_request_id=$1`, requestID).Scan(&nodeID); err != nil {
		t.Fatal(err)
	}
	return nodeID
}

func TestP3BAutoPriorityManualAndFIFO(t *testing.T) {
	s := playerTestStore(t)
	ctx := context.Background()
	gameID, presetID := seedPlayerCatalog(t, s)
	a := p3CapacityNode(t, s, "A", 1)
	b := p3CapacityNode(t, s, "B", 1)
	if err := s.SetNodePriority(ctx, a, 20); err != nil {
		t.Fatal(err)
	}
	if err := s.SetNodePriority(ctx, b, 10); err != nil {
		t.Fatal(err)
	}
	first := p3Request(t, s, gameID, presetID, "auto", "")
	if changed, err := s.TryAllocateOne(ctx); err != nil || !changed {
		t.Fatalf("first auto: %t %v", changed, err)
	}
	if node := allocatedNode(t, s, first.ID); node != a {
		t.Fatalf("high priority node = %s, want %s", node, a)
	}
	manual := p3Request(t, s, gameID, presetID, "manual", a)
	lateAuto := p3Request(t, s, gameID, presetID, "auto", "")
	if changed, err := s.TryAllocateOne(ctx); err != nil || !changed {
		t.Fatalf("unrelated auto blocked: %t %v", changed, err)
	}
	if node := allocatedNode(t, s, lateAuto.ID); node != b {
		t.Fatalf("auto fallback node = %s, want %s", node, b)
	}
	var state string
	var requestedAt time.Time
	if err := s.Pool.QueryRow(ctx, `SELECT state,requested_at FROM server_requests WHERE id=$1`, manual.ID).Scan(&state, &requestedAt); err != nil || state != "waiting" || !requestedAt.Equal(manual.RequestedAt) {
		t.Fatalf("manual moved or lost queue time: %s %s %v", state, requestedAt, err)
	}
	if changed, err := s.TryAllocateOne(ctx); err != nil || changed {
		t.Fatalf("manual stole another node: %t %v", changed, err)
	}
}

func TestP3BFIFOForSharedLastSlot(t *testing.T) {
	s := playerTestStore(t)
	ctx := context.Background()
	gameID, presetID := seedPlayerCatalog(t, s)
	nodeID := p3CapacityNode(t, s, "shared", 1)
	first := p3Request(t, s, gameID, presetID, "auto", "")
	second := p3Request(t, s, gameID, presetID, "manual", nodeID)
	if changed, err := s.TryAllocateOne(ctx); err != nil || !changed {
		t.Fatalf("first request did not win shared slot: %t %v", changed, err)
	}
	if got := allocatedNode(t, s, first.ID); got != nodeID {
		t.Fatalf("first request node = %s, want %s", got, nodeID)
	}
	var state string
	if err := s.Pool.QueryRow(ctx, `SELECT state FROM server_requests WHERE id=$1`, second.ID).Scan(&state); err != nil || state != "waiting" {
		t.Fatalf("later request state = %s: %v", state, err)
	}
}

func TestP3BSafeCancelAndNewQueueTime(t *testing.T) {
	s := playerTestStore(t)
	ctx := context.Background()
	gameID, presetID := seedPlayerCatalog(t, s)
	a := p3CapacityNode(t, s, "A", 1)
	b := p3CapacityNode(t, s, "B", 1)
	if _, err := s.Pool.Exec(ctx, `UPDATE nodes SET draining=true WHERE id=$1`, a); err != nil {
		t.Fatal(err)
	}
	userID, _, err := s.CreateUserSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	old, _, err := s.CreateUserRequestSelected(ctx, userID, gameID, presetID, "manual", a)
	if err != nil {
		t.Fatal(err)
	}
	if changed, err := s.TryAllocateOne(ctx); err != nil || changed {
		t.Fatalf("draining manual allocated: %t %v", changed, err)
	}
	cancelled, err := s.CancelUserRequest(ctx, userID, old.ID)
	if err != nil || cancelled.State != "cancelled" {
		t.Fatalf("safe cancel: %+v %v", cancelled, err)
	}
	newRequest, created, err := s.CreateUserRequestSelected(ctx, userID, gameID, presetID, "manual", b)
	if err != nil || !created || newRequest.ID == old.ID || !newRequest.RequestedAt.After(old.RequestedAt) {
		t.Fatalf("new manual request retained old queue identity: %+v %v", newRequest, err)
	}
	if changed, err := s.TryAllocateOne(ctx); err != nil || !changed {
		t.Fatalf("new manual B: %t %v", changed, err)
	}
	if node := allocatedNode(t, s, newRequest.ID); node != b {
		t.Fatalf("manual B allocated on %s", node)
	}
	if _, err := s.CancelUserRequest(ctx, userID, newRequest.ID); !errors.Is(err, ErrJobConflict) {
		t.Fatalf("allocated request cancelled: %v", err)
	}
	if _, err := s.CancelUserRequest(ctx, userID, old.ID); !errors.Is(err, ErrJobConflict) {
		t.Fatalf("old terminal request cancelled twice: %v", err)
	}
}

func TestP3BPriorityAfterEligibility(t *testing.T) {
	s := playerTestStore(t)
	ctx := context.Background()
	gameID, presetID := seedPlayerCatalog(t, s)
	a := p3CapacityNode(t, s, "A", 1)
	b := p3CapacityNode(t, s, "B", 1)
	if err := s.SetNodePriority(ctx, a, 100); err != nil {
		t.Fatal(err)
	}
	if err := s.SetNodePriority(ctx, b, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE node_content_bindings SET accepting_new_allocations=false WHERE node_id=$1`, a); err != nil {
		t.Fatal(err)
	}
	r := p3Request(t, s, gameID, presetID, "auto", "")
	if changed, err := s.TryAllocateOne(ctx); err != nil || !changed {
		t.Fatalf("eligible lower-priority node skipped: %t %v", changed, err)
	}
	if node := allocatedNode(t, s, r.ID); node != b {
		t.Fatalf("ineligible high priority node chosen: %s", node)
	}
}

func TestP3BEligibilityPredicates(t *testing.T) {
	cases := []struct {
		name        string
		change      func(context.Context, *testing.T, *Store, string, string, string)
		unavailable bool
	}{
		{"global maintenance", func(ctx context.Context, t *testing.T, s *Store, _, _, _ string) {
			_, err := s.Pool.Exec(ctx, `UPDATE platform_settings SET accepting_new_requests=false`)
			if err != nil {
				t.Fatal(err)
			}
		}, false},
		{"game disabled", func(ctx context.Context, t *testing.T, s *Store, _, gameID, _ string) {
			_, err := s.Pool.Exec(ctx, `UPDATE arcade_games SET enabled=false WHERE id=$1`, gameID)
			if err != nil {
				t.Fatal(err)
			}
		}, true},
		{"game paused", func(ctx context.Context, t *testing.T, s *Store, _, gameID, _ string) {
			_, err := s.Pool.Exec(ctx, `UPDATE arcade_games SET accepting_new_requests=false WHERE id=$1`, gameID)
			if err != nil {
				t.Fatal(err)
			}
		}, false},
		{"preset disabled", func(ctx context.Context, t *testing.T, s *Store, _, _, presetID string) {
			_, err := s.Pool.Exec(ctx, `UPDATE game_presets SET enabled=false WHERE id=$1`, presetID)
			if err != nil {
				t.Fatal(err)
			}
		}, true},
		{"preset paused", func(ctx context.Context, t *testing.T, s *Store, _, _, presetID string) {
			_, err := s.Pool.Exec(ctx, `UPDATE game_presets SET accepting_new_requests=false WHERE id=$1`, presetID)
			if err != nil {
				t.Fatal(err)
			}
		}, false},
		{"node disabled", func(ctx context.Context, t *testing.T, s *Store, nodeID, _, _ string) {
			_, err := s.Pool.Exec(ctx, `UPDATE nodes SET enabled=false WHERE id=$1`, nodeID)
			if err != nil {
				t.Fatal(err)
			}
		}, false},
		{"node paused", func(ctx context.Context, t *testing.T, s *Store, nodeID, _, _ string) {
			_, err := s.Pool.Exec(ctx, `UPDATE nodes SET accepting_new_requests=false WHERE id=$1`, nodeID)
			if err != nil {
				t.Fatal(err)
			}
		}, false},
		{"node draining", func(ctx context.Context, t *testing.T, s *Store, nodeID, _, _ string) {
			_, err := s.Pool.Exec(ctx, `UPDATE nodes SET draining=true WHERE id=$1`, nodeID)
			if err != nil {
				t.Fatal(err)
			}
		}, false},
		{"node stale", func(ctx context.Context, t *testing.T, s *Store, nodeID, _, _ string) {
			_, err := s.Pool.Exec(ctx, `UPDATE nodes SET last_heartbeat=now()-interval '3 minutes' WHERE id=$1`, nodeID)
			if err != nil {
				t.Fatal(err)
			}
		}, false},
		{"incompatible", func(ctx context.Context, t *testing.T, s *Store, nodeID, _, _ string) {
			_, err := s.Pool.Exec(ctx, `UPDATE node_reports SET compatibility_status='incompatible' WHERE node_id=$1`, nodeID)
			if err != nil {
				t.Fatal(err)
			}
		}, false},
		{"template unsupported", func(ctx context.Context, t *testing.T, s *Store, nodeID, _, _ string) {
			_, err := s.Pool.Exec(ctx, `DELETE FROM node_template_bindings WHERE node_id=$1`, nodeID)
			if err != nil {
				t.Fatal(err)
			}
		}, false},
		{"content unknown", func(ctx context.Context, t *testing.T, s *Store, nodeID, _, _ string) {
			_, err := s.Pool.Exec(ctx, `UPDATE node_content_bindings SET reported_state='unknown',reported_content_version_id=NULL WHERE node_id=$1`, nodeID)
			if err != nil {
				t.Fatal(err)
			}
		}, false},
		{"content mismatch", func(ctx context.Context, t *testing.T, s *Store, nodeID, gameID, _ string) {
			_, err := s.Pool.Exec(ctx, `INSERT INTO content_versions(id,arcade_game_id,content_sha256) VALUES('other',$1,repeat('b',64))`, gameID)
			if err != nil {
				t.Fatal(err)
			}
			_, err = s.Pool.Exec(ctx, `UPDATE node_content_bindings SET reported_content_version_id='other' WHERE node_id=$1`, nodeID)
			if err != nil {
				t.Fatal(err)
			}
		}, false},
		{"content paused", func(ctx context.Context, t *testing.T, s *Store, nodeID, _, _ string) {
			_, err := s.Pool.Exec(ctx, `UPDATE node_content_bindings SET accepting_new_allocations=false WHERE node_id=$1`, nodeID)
			if err != nil {
				t.Fatal(err)
			}
		}, false},
		{"full capacity", func(ctx context.Context, t *testing.T, s *Store, nodeID, _, _ string) {
			if err := s.SetDesiredCapacity(ctx, nodeID, 0); err != nil {
				t.Fatal(err)
			}
		}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := playerTestStore(t)
			ctx := context.Background()
			gameID, presetID := seedPlayerCatalog(t, s)
			nodeID := p3CapacityNode(t, s, "only", 1)
			r := p3Request(t, s, gameID, presetID, "auto", "")
			tc.change(ctx, t, s, nodeID, gameID, presetID)
			changed, err := s.TryAllocateOne(ctx)
			if err != nil || changed != tc.unavailable {
				t.Fatalf("eligibility %s changed=%t err=%v", tc.name, changed, err)
			}
			var state string
			if err := s.Pool.QueryRow(ctx, `SELECT state FROM server_requests WHERE id=$1`, r.ID).Scan(&state); err != nil {
				t.Fatal(err)
			}
			want := "waiting"
			if tc.unavailable {
				want = "unavailable"
			}
			if state != want {
				t.Fatalf("state=%s want %s", state, want)
			}
			var allocations int
			if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM allocations WHERE server_request_id=$1`, r.ID).Scan(&allocations); err != nil || allocations != 0 {
				t.Fatalf("allocations=%d err=%v", allocations, err)
			}
		})
	}
}
