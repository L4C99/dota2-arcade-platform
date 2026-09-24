package nodev1

import "testing"

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
