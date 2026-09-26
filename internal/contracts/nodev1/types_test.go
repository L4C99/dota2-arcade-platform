package nodev1

import (
	"testing"
	"time"
)

func TestA2SDiagnosticHeartbeatValidation(t *testing.T) {
	h := Heartbeat{OS: "linux", ControllerVersion: "dev", HardMaxInstances: 2,
		Network: NetworkFacts{ConnectHost: "node.example", LocalPortMin: 28000, LocalPortMax: 28001, MappingMode: "identity", A2SEnabled: true}}
	checkedAt := time.Now().UTC().Format(time.RFC3339Nano)
	h.A2SDiagnostics = []A2SDiagnostic{{InstanceID: "a", LocalPort: 28000, Status: "ok", CheckedAt: checkedAt},
		{InstanceID: "b", LocalPort: 28001, Status: "failed", CheckedAt: checkedAt}}
	if err := h.Validate(); err != nil {
		t.Fatal(err)
	}
	h.A2SDiagnostics[1].LocalPort = 28000
	if err := h.Validate(); err == nil {
		t.Fatal("duplicate instance port accepted")
	}
	h.A2SDiagnostics[1].LocalPort = 28001
	h.A2SDiagnostics[1].Status = "unknown"
	if err := h.Validate(); err == nil {
		t.Fatal("fabricated unknown result accepted")
	}
	h.A2SDiagnostics[1].Status = "failed"
	h.Network.A2SEnabled = false
	if err := h.Validate(); err == nil {
		t.Fatal("diagnostic accepted with A2S disabled")
	}
}

func TestHeartbeatValidation(t *testing.T) {
	h := Heartbeat{OS: "windows", ControllerVersion: "dev", HardMaxInstances: 2,
		Network: NetworkFacts{ConnectHost: "node.example", LocalPortMin: 28000, LocalPortMax: 28001,
			MappingMode: "explicit", Mappings: []PortMapping{{28000, 45123}, {28001, 46781}}},
		Content: []ContentFact{{WorkshopID: "123", ContentVersionID: "v1", State: "confirmed"}}}
	if err := h.Validate(); err != nil {
		t.Fatal(err)
	}
	h.Network.Mappings[1].Public = 45123
	if err := h.Validate(); err == nil {
		t.Fatal("duplicate public port accepted")
	}
	h.Network.Mappings[1].Public = 46781
	h.HardMaxInstances = 3
	if err := h.Validate(); err == nil {
		t.Fatal("hard max above pool accepted")
	}
}

func TestP3ENetworkMappingValidation(t *testing.T) {
	base := func() NetworkFacts {
		return NetworkFacts{ConnectHost: "vendor-nat.example.cn", LocalPortMin: 28000, LocalPortMax: 28001,
			MappingMode: "explicit", Mappings: []PortMapping{{Local: 28000, Public: 45123}, {Local: 28001, Public: 46781}}}
	}
	for _, host := range []string{"203.0.113.7", "node.example.cn", "vendor-nat.example.cn", "2001:db8::7"} {
		facts := base()
		facts.ConnectHost = host
		if err := facts.Validate(2); err != nil {
			t.Fatalf("valid connect host %q: %v", host, err)
		}
	}
	cases := []struct {
		name string
		edit func(*NetworkFacts)
		hard int
	}{
		{"missing local mapping", func(n *NetworkFacts) { n.Mappings = n.Mappings[:1] }, 2},
		{"duplicate local mapping", func(n *NetworkFacts) { n.Mappings[1].Local = 28000 }, 2},
		{"duplicate public port", func(n *NetworkFacts) { n.Mappings[1].Public = 45123 }, 2},
		{"public port zero", func(n *NetworkFacts) { n.Mappings[1].Public = 0 }, 2},
		{"local port outside pool", func(n *NetworkFacts) { n.Mappings[1].Local = 28002 }, 2},
		{"hard above pool", func(*NetworkFacts) {}, 3},
		{"negative hard", func(*NetworkFacts) {}, -1},
		{"identity with explicit entries", func(n *NetworkFacts) { n.MappingMode = "identity" }, 2},
		{"unknown mapping mode", func(n *NetworkFacts) { n.MappingMode = "guess" }, 2},
		{"invalid domain label", func(n *NetworkFacts) { n.ConnectHost = "a.-bad.example" }, 2},
		{"overlong domain label", func(n *NetworkFacts) {
			n.ConnectHost = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.example"
		}, 2},
		{"unverified protocol domain", func(n *NetworkFacts) { n.ProtocolIP = "vendor-nat.example.cn" }, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			facts := base()
			tc.edit(&facts)
			if err := facts.Validate(tc.hard); err == nil {
				t.Fatalf("invalid network facts accepted: %+v hard=%d", facts, tc.hard)
			}
		})
	}
}

func TestP3EEntryRevisionCoversMappingsAndHost(t *testing.T) {
	a := NetworkFacts{ConnectHost: "vendor-nat.example.cn", LocalPortMin: 28000, LocalPortMax: 28001,
		MappingMode: "explicit", Mappings: []PortMapping{{28000, 45123}, {28001, 46781}}}
	b := a
	b.Mappings = []PortMapping{{28001, 46781}, {28000, 45123}}
	if EntryConfigRevision(a) != EntryConfigRevision(b) {
		t.Fatal("mapping order changed entry revision")
	}
	b.Mappings[0].Public = 46782
	if EntryConfigRevision(a) == EntryConfigRevision(b) {
		t.Fatal("public mapping change did not change entry revision")
	}
	b = a
	b.ConnectHost = "other.example.cn"
	if EntryConfigRevision(a) == EntryConfigRevision(b) {
		t.Fatal("connect host change did not change entry revision")
	}
}
