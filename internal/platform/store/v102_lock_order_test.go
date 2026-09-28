package store

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/L4C99/dota2-arcade-platform/internal/contracts/nodev1"
)

func TestV102MaintenanceCloseVsScheduler(t *testing.T) {
	t.Run("close first", func(t *testing.T) {
		s, c := v102ValidationFixture(t, 1)
		ctx := context.Background()
		user, _, err := s.CreateUserSession(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := s.CreateUserRequest(ctx, user, c.GameID, c.PresetID); err != nil {
			t.Fatal(err)
		}
		if changed, err := s.TryAllocateOne(ctx); err != nil || changed {
			t.Fatalf("closed binding admitted player: %t %v", changed, err)
		}
	})
	t.Run("scheduler first", func(t *testing.T) {
		s := playerTestStore(t)
		ctx := context.Background()
		game, preset := seedPlayerCatalog(t, s)
		admin, err := s.CreateAdmin(ctx, "maintenance-admin", "a long test password")
		if err != nil {
			t.Fatal(err)
		}
		node := p3CapacityNode(t, s, "scheduler-first", 1)
		user, _, err := s.CreateUserSession(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := s.CreateUserRequest(ctx, user, game, preset); err != nil {
			t.Fatal(err)
		}
		if changed, err := s.TryAllocateOne(ctx); err != nil || !changed {
			t.Fatalf("player reservation: %t %v", changed, err)
		}
		if epoch, err := s.BeginScopedMaintenance(ctx, node, game, admin, "scheduler-first", "maintenance", 0); err != nil || epoch != 1 {
			t.Fatalf("maintenance: %d %v", epoch, err)
		}
		var occupied int
		var accepting bool
		if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM allocations WHERE node_id=$1 AND state NOT IN ('reclaimed','released_no_effect')`, node).Scan(&occupied); err != nil || occupied != 1 {
			t.Fatalf("old resource lost: %d %v", occupied, err)
		}
		if err := s.Pool.QueryRow(ctx, `SELECT accepting_new_allocations FROM node_content_bindings WHERE node_id=$1 AND arcade_game_id=$2`, node, game).Scan(&accepting); err != nil || accepting {
			t.Fatalf("binding not closed: %t %v", accepting, err)
		}
	})
}

// Hold the Request row so both contenders are known to be waiting at the
// common first lock. Releasing it forces the critical state transitions to
// interleave without relying on timing alone.
func withBlockedRequest(t *testing.T, s *Store, requestID string, work func(context.Context) error, other func(context.Context) error) (error, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var locked string
	if err := tx.QueryRow(ctx, `SELECT id FROM server_requests WHERE id=$1 FOR UPDATE`, requestID).Scan(&locked); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	var first, second error
	wg.Add(2)
	go func() { defer wg.Done(); <-start; first = work(ctx) }()
	go func() { defer wg.Done(); <-start; second = other(ctx) }()
	close(start)
	time.Sleep(100 * time.Millisecond)
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	if ctx.Err() != nil {
		t.Fatalf("deadlock or timeout: %v; first=%v second=%v", ctx.Err(), first, second)
	}
	return first, second
}

func TestV102RequestStopVsInstanceFact(t *testing.T) {
	for _, tc := range []struct {
		name     string
		fact     nodev1.InstanceFact
		occupied int
	}{
		{"quarantine", nodev1.InstanceFact{InstanceID: "i_p4c", Outcome: "identity_unverified", Lifecycle: "unknown", Process: "unknown", Cleanup: "unknown", Port: 28000}, 1},
		{"reclaimed", nodev1.InstanceFact{InstanceID: "i_p4c", Outcome: "reclaimed", Lifecycle: "reclaimed", Process: "stopped", Cleanup: "complete", Port: 28000}, 0},
		{"stop related active", nodev1.InstanceFact{InstanceID: "i_p4c", Outcome: "active", Lifecycle: "active", Process: "running", Cleanup: "pending", Port: 28000}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, node, owner, request, allocation := p4cRunning(t)
			stopErr, factErr := withBlockedRequest(t, s, request,
				func(ctx context.Context) error { _, err := s.StopUserRequest(ctx, owner, request); return err },
				func(ctx context.Context) error { return s.ReportInstanceFact(ctx, node, allocation, tc.fact) })
			if factErr != nil {
				t.Fatalf("fact: %v", factErr)
			}
			if stopErr != nil && !errors.Is(stopErr, ErrJobConflict) {
				t.Fatalf("stop: %v", stopErr)
			}
			ctx := context.Background()
			var occupied, jobs int
			if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM allocations WHERE node_id=$1 AND state NOT IN ('reclaimed','released_no_effect')`, node).Scan(&occupied); err != nil || occupied != tc.occupied {
				t.Fatalf("capacity=%d want=%d %v", occupied, tc.occupied, err)
			}
			if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM node_jobs WHERE allocation_id=$1 AND kind='stop'`, allocation).Scan(&jobs); err != nil || jobs > 1 {
				t.Fatalf("stop jobs=%d %v", jobs, err)
			}
		})
	}
}

func TestV102RequestStopVsReportJob(t *testing.T) {
	s, node, request, allocation, job := p4aReserved(t)
	ctx := context.Background()
	if _, err := s.PrepareCreate(ctx, node, job, "C:/versioned/template.json", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReportJob(ctx, node, job, nodev1.ReportRequest{State: "accepted", InstanceID: "i_effect", OperationID: "o_create"}); err != nil {
		t.Fatal(err)
	}
	var owner string
	if err := s.Pool.QueryRow(ctx, `SELECT owner_user_id FROM server_requests WHERE id=$1`, request).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	stopErr, reportErr := withBlockedRequest(t, s, request,
		func(ctx context.Context) error { _, err := s.StopUserRequest(ctx, owner, request); return err },
		func(ctx context.Context) error {
			_, err := s.ReportJob(ctx, node, job, nodev1.ReportRequest{State: "failed_with_effect", InstanceID: "i_effect", OperationID: "o_create", ErrorCode: "ENGINE_EXIT"})
			return err
		})
	if reportErr != nil {
		t.Fatalf("report: %v", reportErr)
	}
	if stopErr != nil && !errors.Is(stopErr, ErrJobConflict) {
		t.Fatalf("stop: %v", stopErr)
	}
	var jobs, occupied int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM node_jobs WHERE allocation_id=$1 AND kind='stop'`, allocation).Scan(&jobs); err != nil || jobs != 1 {
		t.Fatalf("automatic stop jobs=%d %v", jobs, err)
	}
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM allocations WHERE id=$1 AND state NOT IN ('reclaimed','released_no_effect')`, allocation).Scan(&occupied); err != nil || occupied != 1 {
		t.Fatalf("effectful capacity=%d %v", occupied, err)
	}
}

func TestV102NextGameVsAdminStop(t *testing.T) {
	s, _, owner, request, allocation := p4cRunning(t)
	ctx := context.Background()
	admin, err := s.CreateAdmin(ctx, "i1-stop-admin", "a long test password")
	if err != nil {
		t.Fatal(err)
	}
	nextErr, adminErr := withBlockedRequest(t, s, request,
		func(ctx context.Context) error { _, err := s.NextGameUserRequest(ctx, owner, request); return err },
		func(ctx context.Context) error {
			return s.ApplyAdminAction(ctx, admin, AdminAction{Action: "request.stop", TargetID: request})
		})
	if nextErr != nil && !errors.Is(nextErr, ErrJobConflict) {
		t.Fatalf("next game: %v", nextErr)
	}
	if adminErr != nil && !errors.Is(adminErr, ErrJobConflict) {
		t.Fatalf("admin stop: %v", adminErr)
	}
	var jobs int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM node_jobs WHERE allocation_id=$1 AND kind='stop'`, allocation).Scan(&jobs); err != nil || jobs != 1 {
		t.Fatalf("stop jobs=%d %v", jobs, err)
	}
}
