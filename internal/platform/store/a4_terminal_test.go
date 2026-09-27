package store

import (
	"context"
	"errors"
	"testing"

	"github.com/L4C99/dota2-arcade-platform/internal/contracts/nodev1"
)

func TestA4ReleasedAttemptCannotAffectReplacement(t *testing.T) {
	s, node, request, allocation, job := p4aReserved(t)
	p3CapacityNode(t, s, "replacement", 1)
	ctx := context.Background()
	rejection := nodev1.ReportRequest{State: "rejected_no_effect", ErrorCode: "LOCAL_TEMPLATE_BINDING", ErrorStage: "validate"}
	if _, err := s.ReportJob(ctx, node, job, rejection); err != nil {
		t.Fatal(err)
	}
	if changed, err := s.TryAllocateOne(ctx); err != nil || !changed {
		t.Fatalf("replacement: %v %v", changed, err)
	}
	if _, err := s.ReportJob(ctx, node, job, rejection); err != nil {
		t.Fatal(err)
	}
	for _, state := range []string{"accepted", "unknown", "failed_with_effect"} {
		if _, err := s.ReportJob(ctx, node, job, nodev1.ReportRequest{State: state, InstanceID: "late", OperationID: "late"}); !errors.Is(err, ErrJobConflict) {
			t.Fatalf("late %s: %v", state, err)
		}
	}
	p4aStates(t, s, request, allocation, "allocating", "released_no_effect", 1)
	var attempts int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM allocations WHERE server_request_id=$1`, request).Scan(&attempts); err != nil || attempts != 2 {
		t.Fatalf("attempts %d: %v", attempts, err)
	}
}

func TestA4TerminalReclaimRejectsLateBusinessTransitions(t *testing.T) {
	for _, nextGame := range []bool{false, true} {
		for _, late := range []string{"accepted", "unknown", "failed_with_effect"} {
			name := late
			if nextGame {
				name += "_next_game"
			}
			t.Run(name, func(t *testing.T) {
				s, node, owner, request, allocation := p4cRunning(t)
				ctx := context.Background()
				if nextGame {
					if _, err := s.NextGameUserRequest(ctx, owner, request); err != nil {
						t.Fatal(err)
					}
				} else {
					if _, err := s.StopUserRequest(ctx, owner, request); err != nil {
						t.Fatal(err)
					}
				}
				job, err := s.ClaimNextJob(ctx, node)
				if err != nil || job == nil {
					t.Fatalf("claim: %v", err)
				}
				if err := s.ReportInstanceFact(ctx, node, allocation, nodev1.InstanceFact{InstanceID: "i_p4c", Outcome: "reclaimed", Lifecycle: "reclaimed", Process: "stopped", Cleanup: "complete"}); err != nil {
					t.Fatal(err)
				}
				report := nodev1.ReportRequest{State: late, InstanceID: "i_p4c", OperationID: "o_stop"}
				if _, err := s.ReportJob(ctx, node, job.ID, report); err != nil {
					t.Fatal(err)
				}
				if _, err := s.ReportJob(ctx, node, job.ID, report); err != nil {
					t.Fatal(err)
				}
				p4aStates(t, s, request, allocation, "ended", "reclaimed", 0)
				var blocking int
				if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM server_requests WHERE owner_user_id=$1 AND state='waiting'`, owner).Scan(&blocking); err != nil {
					t.Fatal(err)
				}
				want := 0
				if nextGame {
					want = 1
				}
				if blocking != want {
					t.Fatalf("fresh waiting requests=%d want %d", blocking, want)
				}
			})
		}
	}
}
