package httpapi

import (
	"testing"
	"time"

	"github.com/L4C99/dota2-arcade-platform/internal/contracts/nodev1"
)

func TestCapabilityLeaseIsolation(t *testing.T) {
	leases := newCapabilityLeases()
	capabilities := []string{nodev1.CapabilityContentValidationV102, nodev1.CapabilityTemplateManifestV1, nodev1.CapabilityCoreInventoryV1}
	a, ok := leases.issue("node-A", nodev1.CapabilitySessionRequest{ProcessID: "process-AAAAAAAA", Capabilities: capabilities})
	if !ok || a.Token == "" || !leases.valid("node-A", a.Token, nodev1.CapabilityContentValidationV102) {
		t.Fatal("session A unavailable")
	}
	if leases.valid("node-B", a.Token, nodev1.CapabilityContentValidationV102) || leases.valid("node-A", "shared-node-secret", nodev1.CapabilityContentValidationV102) {
		t.Fatal("lease escaped node or credential boundary")
	}
	b, ok := leases.issue("node-A", nodev1.CapabilitySessionRequest{ProcessID: "process-BBBBBBBB", Capabilities: capabilities})
	if !ok || leases.valid("node-A", a.Token, nodev1.CapabilityContentValidationV102) || !leases.valid("node-A", b.Token, nodev1.CapabilityContentValidationV102) {
		t.Fatal("replacement did not invalidate A")
	}
	leases.mu.Lock()
	lease := leases.byNode["node-A"]
	lease.expires = time.Now().Add(-time.Second)
	leases.byNode["node-A"] = lease
	leases.mu.Unlock()
	if leases.valid("node-A", b.Token, nodev1.CapabilityContentValidationV102) {
		t.Fatal("expired lease accepted")
	}
	if _, ok := leases.issue("node-A", nodev1.CapabilitySessionRequest{ProcessID: "process-CCCCCCCC", Capabilities: []string{nodev1.CapabilityContentValidationV102}}); ok {
		t.Fatal("partial capability session accepted")
	}
}

func TestLeaseReplacementWaitsForInFlightRequest(t *testing.T) {
	leases := newCapabilityLeases()
	capabilities := []string{nodev1.CapabilityContentValidationV102, nodev1.CapabilityTemplateManifestV1, nodev1.CapabilityCoreInventoryV1}
	a, ok := leases.issue("node-A", nodev1.CapabilitySessionRequest{ProcessID: "process-AAAAAAAA", Capabilities: capabilities})
	if !ok {
		t.Fatal("session A")
	}
	leases.mu.RLock() // the fenced HTTP request holds this through its DB operation
	finished := make(chan struct{})
	go func() {
		_, _ = leases.issue("node-A", nodev1.CapabilitySessionRequest{ProcessID: "process-BBBBBBBB", Capabilities: capabilities})
		close(finished)
	}()
	select {
	case <-finished:
		t.Fatal("new session overtook an in-flight request")
	case <-time.After(30 * time.Millisecond):
	}
	if !leases.validLocked("node-A", a.Token, nodev1.CapabilityContentValidationV102) {
		t.Fatal("in-flight A lost authority before replacement")
	}
	leases.mu.RUnlock()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("replacement did not complete")
	}
	if leases.valid("node-A", a.Token, nodev1.CapabilityContentValidationV102) {
		t.Fatal("A survived replacement")
	}
}
