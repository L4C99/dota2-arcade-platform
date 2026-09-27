package store

import (
	"context"
	"github.com/L4C99/dota2-arcade-platform/internal/contracts/nodev1"
	"testing"
)

func TestA4NextGameOversizePartyAndAfterAbandon(t *testing.T) {
	for _, abandon := range []bool{false, true} {
		t.Run(map[bool]string{false: "reclaim", true: "abandon"}[abandon], func(t *testing.T) {
			s, node, _, request, allocation := p4cRunning(t)
			ctx := context.Background()
			if err := s.ConfigurePartySize(ctx, 4); err != nil {
				t.Fatal(err)
			}
			leader, member := partyUser(t, s), partyUser(t, s)
			party, err := s.CreateParty(ctx, leader)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.Pool.Exec(ctx, `UPDATE server_requests SET owner_user_id=NULL,owner_party_id=$2 WHERE id=$1`, request, party); err != nil {
				t.Fatal(err)
			}
			if _, err := s.NextGameUserRequest(ctx, leader, request); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Pool.Exec(ctx, `UPDATE game_presets SET max_players=1`); err != nil {
				t.Fatal(err)
			}
			invite, err := s.CurrentInvite(ctx, leader)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.JoinParty(ctx, member, invite.Token); err != nil {
				t.Fatal(err)
			}
			if abandon {
				if err := s.MarkAllocationQuarantined(ctx, allocation); err != nil {
					t.Fatal(err)
				}
				if _, err := s.AbandonQuarantinedUserRequest(ctx, leader, request); err != nil {
					t.Fatal(err)
				}
				p4aStates(t, s, request, allocation, "abandoned", "quarantined", 1)
			} else {
				if err := s.ReportInstanceFact(ctx, node, allocation, nodev1.InstanceFact{InstanceID: "i_p4c", Outcome: "reclaimed", Lifecycle: "reclaimed", Process: "stopped", Cleanup: "complete"}); err != nil {
					t.Fatal(err)
				}
				p4aStates(t, s, request, allocation, "ended", "reclaimed", 0)
			}
			intent, err := s.UserNextGameIntent(ctx, leader, request)
			if err != nil || intent.FailureReason == "" || intent.NewRequestID != nil {
				t.Fatalf("intent %+v %v", intent, err)
			}
		})
	}
}

func TestA4NextGameEligibilityFailureCommitsReclaim(t *testing.T) {
	for _, scenario := range []string{"global", "game", "preset", "disabled", "manual"} {
		t.Run(scenario, func(t *testing.T) {
			s, node, owner, request, allocation := p4cRunning(t)
			ctx := context.Background()
			if _, err := s.NextGameUserRequest(ctx, owner, request); err != nil {
				t.Fatal(err)
			}
			var query string
			switch scenario {
			case "global":
				query = `UPDATE platform_settings SET accepting_new_requests=false`
			case "game":
				query = `UPDATE arcade_games SET accepting_new_requests=false`
			case "preset":
				query = `UPDATE game_presets SET accepting_new_requests=false`
			case "disabled":
				query = `UPDATE game_presets SET enabled=false`
			case "manual":
				if _, err := s.Pool.Exec(ctx, `UPDATE server_requests SET node_selection_mode='manual',manual_node_id=$2 WHERE id=$1`, request, node); err != nil {
					t.Fatal(err)
				}
				query = `UPDATE nodes SET enabled=false`
			}
			if _, err := s.Pool.Exec(ctx, query); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 2; i++ {
				if err := s.ReportInstanceFact(ctx, node, allocation, nodev1.InstanceFact{InstanceID: "i_p4c", Outcome: "reclaimed", Lifecycle: "reclaimed", Process: "stopped", Cleanup: "complete"}); err != nil {
					t.Fatal(err)
				}
			}
			p4aStates(t, s, request, allocation, "ended", "reclaimed", 0)
			intent, err := s.UserNextGameIntent(ctx, owner, request)
			if err != nil || intent.FailureReason == "" || intent.NewRequestID != nil {
				t.Fatalf("intent %+v %v", intent, err)
			}
			var count int
			if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM server_requests`).Scan(&count); err != nil || count != 1 {
				t.Fatalf("illegal fresh request %d %v", count, err)
			}
		})
	}
}
