package store

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/L4C99/dota2-arcade-platform/internal/contracts/nodev1"
)

func TestP5AMultiCatalogPublicationAndWaitingVersion(t *testing.T) {
	s := playerTestStore(t)
	ctx := context.Background()
	game1, preset1 := seedPlayerCatalog(t, s)
	node1 := p3CapacityNode(t, s, "first content", 1)
	adminID, err := s.CreateAdmin(ctx, "p5a-admin", "a long test password")
	if err != nil {
		t.Fatal(err)
	}
	act := func(a AdminAction) {
		t.Helper()
		if err := s.ApplyAdminAction(ctx, adminID, a); err != nil {
			t.Fatalf("%s: %v", a.Action, err)
		}
	}
	act(AdminAction{Action: "game.create", WorkshopID: "1234567890", DisplayName: "Second game"})
	var game2 string
	if err := s.Pool.QueryRow(ctx, `SELECT id FROM arcade_games WHERE workshop_id='1234567890'`).Scan(&game2); err != nil {
		t.Fatal(err)
	}
	act(AdminAction{Action: "template.create", TargetID: game2, TemplateRevisionID: "second-template", Description: "known startup semantics"})
	act(AdminAction{Action: "content.create", TargetID: game2, ContentVersionID: "second-v1", ContentSHA256: strings.Repeat("b", 64)})
	act(AdminAction{Action: "preset.create", TargetID: game2, DisplayName: "Solo", TemplateRevisionID: "second-template", MaxPlayers: 1})
	act(AdminAction{Action: "preset.create", TargetID: game2, DisplayName: "Group", TemplateRevisionID: "second-template", MaxPlayers: 8})
	catalog, err := s.PlayerCatalog(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Games) != 1 || len(catalog.Presets) != 1 {
		t.Fatal("new content was exposed before publication")
	}
	if err := s.ApplyAdminAction(ctx, adminID, AdminAction{Action: "content.publish", TargetID: game2, ContentVersionID: "second-v1", Confirmed: true}); !errors.Is(err, ErrJobConflict) {
		t.Fatalf("unvalidated publication: %v", err)
	}
	node2, _, err := s.RegisterNode(ctx, "second content", "linux")
	if err != nil {
		t.Fatal(err)
	}
	act(AdminAction{Action: "template_binding.upsert", TargetID: node2, TemplateRevisionID: "second-template", BindingKey: "second-binding"})
	h := p1TestHeartbeat("test-v1")
	h.Content = []nodev1.ContentFact{{WorkshopID: "1234567890", ContentVersionID: "second-v1", VPKSHA256: strings.Repeat("b", 64), State: "confirmed"}}
	wrong := h
	wrong.Content = []nodev1.ContentFact{{WorkshopID: "1234567890", ContentVersionID: "second-v1", VPKSHA256: strings.Repeat("d", 64), State: "confirmed"}}
	if _, err := s.RecordHeartbeat(ctx, node2, wrong); err != nil {
		t.Fatal(err)
	}
	var wrongState string
	if err := s.Pool.QueryRow(ctx, `SELECT reported_state FROM node_content_bindings WHERE node_id=$1 AND arcade_game_id=$2`, node2, game2).Scan(&wrongState); err != nil || wrongState != "unknown" {
		t.Fatalf("mismatched digest accepted: %s %v", wrongState, err)
	}
	if _, err := s.RecordHeartbeat(ctx, node2, h); err != nil {
		t.Fatal(err)
	}
	if err := s.SetDesiredCapacity(ctx, node2, 1); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyAdminAction(ctx, adminID, AdminAction{Action: "content.validate", TargetID: node2, GameID: game2, ContentVersionID: "second-v1", Confirmed: true}); !errors.Is(err, ErrJobConflict) {
		t.Fatalf("validation without drain: %v", err)
	}
	act(AdminAction{Action: "node.update", TargetID: node2, Draining: boolPtr(true)})
	act(AdminAction{Action: "content.validate", TargetID: node2, GameID: game2, ContentVersionID: "second-v1", Confirmed: true})
	if err := s.ApplyAdminAction(ctx, adminID, AdminAction{Action: "content.publish", TargetID: game2, ContentVersionID: "second-v1", Confirmed: true}); !errors.Is(err, ErrJobConflict) {
		t.Fatalf("publication on drained node: %v", err)
	}
	act(AdminAction{Action: "node.update", TargetID: node2, Draining: boolPtr(false)})
	act(AdminAction{Action: "content.publish", TargetID: game2, ContentVersionID: "second-v1", Confirmed: true})
	var accepting bool
	if err := s.Pool.QueryRow(ctx, `SELECT accepting_new_allocations FROM node_content_bindings WHERE node_id=$1 AND arcade_game_id=$2`, node2, game2).Scan(&accepting); err != nil || accepting {
		t.Fatalf("publication changed binding admission: %v %v", accepting, err)
	}
	act(AdminAction{Action: "binding.update", TargetID: node2, GameID: game2, Accepting: boolPtr(true)})
	act(AdminAction{Action: "game.update", TargetID: game2, Enabled: boolPtr(true), Accepting: boolPtr(true)})
	var groupPreset string
	if err := s.Pool.QueryRow(ctx, `SELECT id FROM game_presets WHERE arcade_game_id=$1 AND display_name='Group'`, game2).Scan(&groupPreset); err != nil {
		t.Fatal(err)
	}
	act(AdminAction{Action: "preset.update", TargetID: groupPreset, Enabled: boolPtr(true), Accepting: boolPtr(true)})
	catalog, err = s.PlayerCatalog(ctx)
	if err != nil || len(catalog.Games) != 2 || len(catalog.Presets) != 2 {
		t.Fatalf("multi catalog: %+v %v", catalog, err)
	}

	// A waiting request has no version until a matching node is admitted.
	act(AdminAction{Action: "content.create", TargetID: game1, ContentVersionID: "test-v2", ContentSHA256: strings.Repeat("c", 64)})
	act(AdminAction{Action: "node.update", TargetID: node1, Draining: boolPtr(true)})
	waiting := p3Request(t, s, game1, preset1, "manual", node1)
	if changed, err := s.TryAllocateOne(ctx); err != nil || changed {
		t.Fatalf("drained waiting allocated: %v %v", changed, err)
	}
	h = p1TestHeartbeat("test-v2")
	h.Content[0].VPKSHA256 = strings.Repeat("c", 64)
	if _, err := s.RecordHeartbeat(ctx, node1, h); err != nil {
		t.Fatal(err)
	}
	act(AdminAction{Action: "content.validate", TargetID: node1, GameID: game1, ContentVersionID: "test-v2", Confirmed: true})
	act(AdminAction{Action: "node.update", TargetID: node1, Draining: boolPtr(false)})
	act(AdminAction{Action: "content.publish", TargetID: game1, ContentVersionID: "test-v2", Confirmed: true})
	if changed, err := s.TryAllocateOne(ctx); err != nil || !changed {
		t.Fatalf("waiting did not allocate: %v %v", changed, err)
	}
	var version, assignedNode string
	if err := s.Pool.QueryRow(ctx, `SELECT content_version_id,node_id FROM allocations WHERE server_request_id=$1`, waiting.ID).Scan(&version, &assignedNode); err != nil {
		t.Fatal(err)
	}
	if version != "test-v2" || assignedNode != node1 {
		t.Fatalf("waiting version/node = %s/%s", version, assignedNode)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE arcade_games SET current_content_version_id='test-v1' WHERE id=$1`, game1); err != nil {
		t.Fatal(err)
	}
	if err := s.Pool.QueryRow(ctx, `SELECT content_version_id FROM allocations WHERE server_request_id=$1`, waiting.ID).Scan(&version); err != nil || version != "test-v2" {
		t.Fatalf("existing allocation version changed: %s %v", version, err)
	}
	var otherBindingCount int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM node_content_bindings WHERE node_id=$1 AND arcade_game_id=$2`, node2, game1).Scan(&otherBindingCount); err != nil || otherBindingCount != 0 {
		t.Fatalf("unrelated game binding changed: %d %v", otherBindingCount, err)
	}
	var audits int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE action='content.publish'`).Scan(&audits); err != nil || audits != 2 {
		t.Fatalf("publication audit count %d: %v", audits, err)
	}
}
