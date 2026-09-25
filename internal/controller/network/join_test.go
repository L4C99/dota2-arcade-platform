package network

import (
	"testing"

	"github.com/L4C99/dota2-arcade-platform/internal/contracts/nodev1"
)

func TestJoinInfoUsesActualPort(t *testing.T) {
	facts := nodev1.NetworkFacts{ConnectHost: "node.example", LocalPortMin: 28000, LocalPortMax: 28001,
		MappingMode: "explicit", Mappings: []nodev1.PortMapping{{Local: 28000, Public: 45123}, {Local: 28001, Public: 46781}}}
	join, code := JoinInfo(facts, 28001)
	if code != "" || join == nil || join.LocalPort != 28001 || join.PublicPort != 46781 {
		t.Fatalf("actual port mapping: %+v %q", join, code)
	}
	join, code = JoinInfo(facts, 28002)
	if join != nil || code != "PORT_MAPPING_UNAVAILABLE" {
		t.Fatalf("missing mapping gave %+v %q", join, code)
	}
	facts.MappingMode = "identity"
	facts.Mappings = nil
	join, code = JoinInfo(facts, 28000)
	if code != "" || join == nil || join.PublicPort != 28000 {
		t.Fatalf("identity mapping: %+v %q", join, code)
	}
}
