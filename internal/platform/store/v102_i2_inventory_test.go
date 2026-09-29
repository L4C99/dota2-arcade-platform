package store

import (
	"context"
	"strings"
	"testing"

	"github.com/L4C99/dota2-arcade-platform/internal/contracts/nodev1"
)

func TestI2InventoryHeartbeatLinkAndUnaccountedGate(t *testing.T) {
	s, c := v102ValidationFixture(t, 1)
	ctx := context.Background()
	h := p1TestHeartbeat("test-v1")
	h.HardMaxInstances = 1
	h.Content[0].VPKSHA256 = strings.Repeat("a", 64)
	h.Capabilities = []string{nodev1.CapabilityContentValidationV102, nodev1.CapabilityTemplateManifestV1, nodev1.CapabilityCoreInventoryV1}
	h.TemplateFacts = []nodev1.TemplateFact{{BindingKey: "test-binding", State: "confirmed", ManifestAlgorithm: nodev1.TemplateManifestAlgorithmV1, FingerprintSHA256: strings.Repeat("f", 64)}}
	h.InventoryScanID, h.InventoryState = "missing-scan", "confirmed"
	if _, err := s.RecordHeartbeat(ctx, c.NodeID, h); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReserveValidation(ctx, c); err == nil {
		t.Fatal("heartbeat linked to nonexistent scan")
	}
	h.InventoryState = "unknown"
	if _, err := s.RecordHeartbeat(ctx, c.NodeID, h); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReserveValidation(ctx, c); err == nil {
		t.Fatal("unknown inventory gated as empty")
	}
	if _, err := s.RecordInventory(ctx, c.NodeID, "third-scan", true, "", []InventoryInstance{{InstanceID: "integration-only", Lifecycle: "active", Process: "running", Cleanup: "pending"}}); err != nil {
		t.Fatal(err)
	}
	h.InventoryScanID, h.InventoryState = "third-scan", "confirmed"
	if _, err := s.RecordHeartbeat(ctx, c.NodeID, h); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReserveValidation(ctx, c); err == nil {
		t.Fatal("unaccounted instance did not block reserve")
	}
	var revision int64
	if err := s.Pool.QueryRow(ctx, `SELECT template_fact_revision FROM node_template_facts WHERE node_id=$1 AND template_revision_id='test-template'`, c.NodeID).Scan(&revision); err != nil || revision != 1 {
		t.Fatalf("repeat template fact revision %d: %v", revision, err)
	}
}
