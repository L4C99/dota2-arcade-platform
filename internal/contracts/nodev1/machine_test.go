package nodev1

import (
	"strings"
	"testing"
)

func TestAdditiveMachineHeartbeatValidation(t *testing.T) {
	h := Heartbeat{OS: "linux", ControllerVersion: "old", NodeAPIVersion: 1, HardMaxInstances: 1,
		Network: NetworkFacts{ConnectHost: "node.example", LocalPortMin: 28000, LocalPortMax: 28000, MappingMode: "identity"}}
	if err := h.Validate(); err != nil {
		t.Fatalf("old heartbeat rejected: %v", err)
	}
	h.Capabilities = []string{CapabilityContentValidationV102, CapabilityTemplateManifestV1, CapabilityCoreInventoryV1}
	h.InventoryScanID, h.InventoryState = "scan-1", "confirmed"
	h.TemplateFacts = []TemplateFact{{BindingKey: "binding", State: "confirmed", ManifestAlgorithm: TemplateManifestAlgorithmV1, FingerprintSHA256: strings.Repeat("a", 64)}}
	if err := h.Validate(); err != nil {
		t.Fatalf("new heartbeat rejected: %v", err)
	}
	cases := []struct {
		name   string
		change func(*Heartbeat)
	}{
		{"duplicate capability", func(x *Heartbeat) { x.Capabilities = append(x.Capabilities, x.Capabilities[0]) }},
		{"unknown capability", func(x *Heartbeat) { x.Capabilities = append(x.Capabilities, "future") }},
		{"missing scan", func(x *Heartbeat) { x.InventoryScanID = "" }},
		{"duplicate binding", func(x *Heartbeat) { x.TemplateFacts = append(x.TemplateFacts, x.TemplateFacts[0]) }},
		{"bad algorithm", func(x *Heartbeat) { x.TemplateFacts[0].ManifestAlgorithm = "future" }},
		{"unknown with digest", func(x *Heartbeat) { x.TemplateFacts[0].State = "unknown" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			x := h
			x.Capabilities = append([]string(nil), h.Capabilities...)
			x.TemplateFacts = append([]TemplateFact(nil), h.TemplateFacts...)
			tc.change(&x)
			if x.Validate() == nil {
				t.Fatal("invalid heartbeat accepted")
			}
		})
	}
}

func TestInventoryReportCompleteness(t *testing.T) {
	if err := (InventoryReport{ScanID: "one", Complete: true, Instances: []InventoryInstance{}}).Validate(); err != nil {
		t.Fatal(err)
	}
	for _, r := range []InventoryReport{
		{ScanID: "one", Complete: false, Instances: []InventoryInstance{}},
		{ScanID: "one", Complete: true, ErrorCode: "FAILED"},
		{ScanID: "one", Complete: true, Instances: []InventoryInstance{{InstanceID: "i", Lifecycle: "active"}}},
		{ScanID: "one", Complete: true, Instances: []InventoryInstance{{InstanceID: "i", Lifecycle: "reclaimed", Process: "stopped", Cleanup: "complete"}}},
	} {
		if r.Validate() == nil {
			t.Fatalf("invalid inventory accepted: %+v", r)
		}
	}
}
