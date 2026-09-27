package store

import (
	"context"
	"errors"
	"github.com/L4C99/dota2-arcade-platform/internal/contracts/nodev1"
	"testing"
)

func TestA4OnlineUnknownContainmentAndLateReports(t *testing.T) {
	for _, terminal := range []string{"succeeded", "failed_with_effect"} {
		t.Run(terminal, func(t *testing.T) {
			s, node, request, allocation, job := p4aReserved(t)
			ctx := context.Background()
			if _, err := s.PrepareCreate(ctx, node, job, "/tmp/a4.json", 0); err != nil {
				t.Fatal(err)
			}
			frozen, err := s.FrozenCreateForJob(ctx, node, job)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.ReportJob(ctx, node, job, nodev1.ReportRequest{State: "unknown"}); err != nil {
				t.Fatal(err)
			}
			// B's rejection cannot release A, even with a fabricated proof label.
			for i := 0; i < 3; i++ {
				for _, code := range []string{"NO_PORT_AVAILABLE", "PORT_IN_USE", "RECONCILED_NO_EFFECT"} {
					if _, err := s.ReportJob(ctx, node, job, nodev1.ReportRequest{State: "rejected_no_effect", ErrorCode: code, ErrorStage: "reconcile"}); !errors.Is(err, ErrJobConflict) {
						t.Fatalf("unsafe release %s: %v", code, err)
					}
					p4aStates(t, s, request, allocation, "creating", "create_unknown", 1)
				}
			}
			admin, err := s.CreateAdmin(ctx, "a4-admin", "a long test password")
			if err != nil {
				t.Fatal(err)
			}
			if err := s.ApplyAdminAction(ctx, admin, AdminAction{Action: "allocation.quarantine", TargetID: allocation}); err != nil {
				t.Fatal(err)
			}
			p4aStates(t, s, request, allocation, "quarantined", "quarantined", 1)
			var owner, game, preset string
			if err := s.Pool.QueryRow(ctx, `SELECT owner_user_id,arcade_game_id,game_preset_id FROM server_requests WHERE id=$1`, request).Scan(&owner, &game, &preset); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 2; i++ {
				if _, err := s.AbandonQuarantinedUserRequest(ctx, owner, request); err != nil {
					t.Fatal(err)
				}
			}
			fresh, created, err := s.CreateUserRequest(ctx, owner, game, preset)
			if err != nil || !created || fresh.ID == request {
				t.Fatalf("fresh: %+v %v", fresh, err)
			}
			drain := true
			if err := s.ApplyAdminAction(ctx, admin, AdminAction{Action: "node.update", TargetID: node, Draining: &drain}); err != nil {
				t.Fatal(err)
			}
			if changed, err := s.TryAllocateOne(ctx); err != nil || changed {
				t.Fatalf("retired node allocated: %v %v", changed, err)
			}
			// A finally executes, preserving its original IDs/history rather than fresh request.
			if _, err := s.ReportJob(ctx, node, job, nodev1.ReportRequest{State: "accepted", InstanceID: "late-a", OperationID: "late-op"}); err != nil {
				t.Fatal(err)
			}
			report := nodev1.ReportRequest{State: terminal, InstanceID: "late-a", OperationID: "late-op"}
			if terminal == "succeeded" {
				report.JoinInfoErrorCode = "PORT_MAPPING_UNAVAILABLE"
			}
			if _, err := s.ReportJob(ctx, node, job, report); err != nil {
				t.Fatal(err)
			}
			p4aStates(t, s, request, allocation, "abandoned", "quarantined", 1)
			current, err := s.CurrentUserRequest(ctx, owner)
			if err != nil || current == nil || current.ID != fresh.ID {
				t.Fatalf("fresh request changed: %+v %v", current, err)
			}
			after, err := s.FrozenCreateForJob(ctx, node, job)
			if err != nil || after.IdempotencyKey != frozen.IdempotencyKey || after.TemplatePath != frozen.TemplatePath {
				t.Fatal("frozen history changed")
			}
			fact := nodev1.InstanceFact{InstanceID: "late-a", Outcome: "reclaimed", Lifecycle: "reclaimed", Process: "stopped", Cleanup: "complete"}
			if err := s.ReportInstanceFact(ctx, node, allocation, fact); err != nil {
				t.Fatal(err)
			}
			p4aStates(t, s, request, allocation, "abandoned", "reclaimed", 0)
		})
	}
}
