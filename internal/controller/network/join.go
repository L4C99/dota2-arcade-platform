package network

import "github.com/L4C99/dota2-arcade-platform/internal/contracts/nodev1"

// JoinInfo uses the actual port returned by d2core status for this instance.
// The Controller's validated local mapping is the only source of public port.
func JoinInfo(facts nodev1.NetworkFacts, actualLocalPort int) (*nodev1.JoinInfo, string) {
	if actualLocalPort < facts.LocalPortMin || actualLocalPort > facts.LocalPortMax {
		return nil, "PORT_MAPPING_UNAVAILABLE"
	}
	publicPort := 0
	switch facts.MappingMode {
	case "identity":
		publicPort = actualLocalPort
	case "explicit":
		for _, mapping := range facts.Mappings {
			if mapping.Local == actualLocalPort {
				publicPort = mapping.Public
				break
			}
		}
	}
	if publicPort < 1 || publicPort > 65535 {
		return nil, "PORT_MAPPING_UNAVAILABLE"
	}
	join := &nodev1.JoinInfo{LocalPort: actualLocalPort, PublicPort: publicPort,
		ConnectHost: facts.ConnectHost, ProtocolIP: facts.ProtocolIP,
		EntryConfigRevision: nodev1.EntryConfigRevision(facts)}
	if join.Validate() != nil {
		return nil, "NETWORK_CONFIG_INVALID"
	}
	return join, ""
}
