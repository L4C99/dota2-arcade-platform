package store

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func TestA4PendingTerminationCompetesWithClaim(t *testing.T) {
	for _, order := range []string{"terminate", "claim", "race"} {
		t.Run(order, func(t *testing.T) {
			s, node, request, allocation, job := p4aReserved(t)
			ctx := context.Background()
			// Restore the fixture to the never-claimed reservation at dispatch time.
			if _, err := s.Pool.Exec(ctx, `UPDATE node_jobs SET state='pending',claimed_at=NULL WHERE id=$1`, job); err != nil {
				t.Fatal(err)
			}
			admin, err := s.CreateAdmin(ctx, "terminator", "a long test password")
			if err != nil {
				t.Fatal(err)
			}
			action := AdminAction{Action: "allocation.terminate_pending", TargetID: allocation}
			var termErr, claimErr error
			var claimed bool
			terminate := func() { termErr = s.ApplyAdminAction(ctx, admin, action) }
			claim := func() { j, e := s.ClaimNextJob(ctx, node); claimErr = e; claimed = j != nil }
			switch order {
			case "terminate":
				terminate()
				claim()
			case "claim":
				claim()
				terminate()
			default:
				var wg sync.WaitGroup
				wg.Add(2)
				go func() { defer wg.Done(); terminate() }()
				go func() { defer wg.Done(); claim() }()
				wg.Wait()
			}
			if claimErr != nil {
				t.Fatal(claimErr)
			}
			if claimed {
				if !errors.Is(termErr, ErrJobConflict) {
					t.Fatalf("claimed yet terminated: %v", termErr)
				}
				p4aStates(t, s, request, allocation, "allocating", "reserved", 1)
			} else {
				if termErr != nil {
					t.Fatal(termErr)
				}
				if err := s.ApplyAdminAction(ctx, admin, action); err != nil {
					t.Fatal(err)
				}
				p4aStates(t, s, request, allocation, "unavailable", "released_no_effect", 0)
				if j, err := s.ClaimNextJob(ctx, node); err != nil || j != nil {
					t.Fatalf("terminal job claimed: %+v %v", j, err)
				}
			}
		})
	}
}
