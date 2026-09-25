# P3E network mapping and JoinInfo

## Model and configuration checks

`connectHost` accepts an IP or a syntactically valid DNS hostname. Each DNS label must have valid length and edges; malformed hostnames are rejected before Controller startup or heartbeat. `protocolIp` remains an optional explicit IP and is never inferred by resolving `connectHost`. Both identity and complete explicit mappings are supported. Validation rejects missing, duplicate, out-of-pool, and invalid public mappings, identity mappings with explicit entries, unknown modes, and a hard instance limit above the local port count. Mapping order does not change the entry revision; a public mapping or host change does.

The Controller maps the specific port returned by d2core `status` for a Ready instance. A port without a declared mapping produces `PORT_MAPPING_UNAVAILABLE` without releasing the occupied Allocation or inventing JoinInfo. The Platform stores the reported actual/public port and connect host, and hides an existing JoinInfo if later heartbeat facts change the entry revision. Tests cover an asymmetric explicit mapping with a vendor-style domain, a separate `protocolIp`, and no Steam/steamchina URI until the later verified-entry workflow.

Fixed d2core v0.1.1 has no local API for reading its running serve port bounds. Read-only checks of both current development Nodes found their actual `serve --port-min/--port-max` equal to their Controller-declared local pools and hard capacity within each pool. The Windows development launcher now reads its d2core data-dir and port bounds from the same Controller configuration before starting the manager; its syntax was checked without starting a second manager. No firewall, router, NAT, cloud security-group, or production service configuration changed.

## Real topology evidence

The existing Linux development Node has a directly assigned public IP and identity mapping. In the immediately preceding P3D real runs, d2core reported local port 28000 for two distinct instances; the Controller and Platform recorded public port 28000 and a JoinInfo `connect` target using that Node's configured IP. Both instances reached Ready and later reclaimed completely. The prior P1 human join established reachability for this existing Node; this P3E check does not claim a new human join.

The second Windows Node is on a private VM network. Its P3D real instance used d2core local port 28100, and the Controller and Platform recorded the same identity-mapped port and its private connect host. The instance reached Ready and fully reclaimed. That JoinInfo is a local-network fact, **not** evidence of internet reachability.

| Topology | Model/config tests | Real external validation |
| --- | --- | --- |
| Direct public IP, identity mapping on existing Linux Node | PASS | Actual-port JoinInfo PASS; existing P1 human join PASS |
| Windows VM private IP, identity mapping | PASS | Real local-port JoinInfo PASS; public join **NOT VERIFIED** |
| Public game-node domain | PASS | **NOT VERIFIED**: no configured game-node domain |
| Vendor NAT domain and explicit mapping | PASS | **NOT VERIFIED**: no such NAT or port forwarding exists |
| Shared/asymmetric public egress | PASS | **NOT VERIFIED**: no such topology exists |

P3E does not enable A2S or Steam/steamchina URI verification. No DNS result, second public IP, vendor NAT, asymmetric NAT, or public port mapping was fabricated.

## Verification gate

The network contract and actual-port unit suites, full `go test ./... -count=1`, `go vet ./...`, Web lint/typecheck/four tests/build, and Linux/Windows Platform and Controller builds passed. The complete Store and HTTP API PostgreSQL integration suites passed in random disposable schemas on the authorized development database; the post-run schema list contained only `public` and PostgreSQL system schemas. P3E checkpoint CI must pass before P3F begins.
