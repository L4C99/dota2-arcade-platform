# P5C entry checkpoint (in progress)

## Implemented

- Controller checks A2S_INFO over local UDP for every currently Ready instance, including challenge handling. A2S remains a diagnostic and does not determine `connect` or scheduling eligibility.
- Admin entry verification requires an explicit human confirmation, note and exact full set of currently configured public ports. Verified and enabled remain separate. A network revision change clears verification, enabled state and port evidence. Migration invalidates legacy verifications that lacked full-port evidence.
- Player page displays a Steam or steamchina URI only when the backend supplies it after verified and enabled checks. It always retains `connect` help, including the client cold-start and Windows Win+R fallback.
- Unit tests exercise A2S challenge. PostgreSQL tests cover partial-port rejection, full-port acceptance and revision invalidation. GitHub Actions [run 36231578148](https://github.com/L4C99/dota2-arcade-platform/actions/runs/36231578148) passed all four jobs.
- On the drained Linux development node, the operator backed up the Controller config, set `protocol_ip=58.216.8.72` and enabled A2S diagnostics without changing the ten-port identity mapping. The Controller reported a fresh network revision and both URI entries remained unverified and disabled. An official temporary old-game instance reached full d2core Ready on 28000; the Controller reported `a2s_query_ok=false`, and direct local UDP A2S_INFO queries to loopback and the public IP timed out. The instance was stopped and fully reclaimed, the node reconciled and resumed. This is a failed diagnostic observation, not evidence that `connect` or either URI fails.
- The Admin verification action and button no longer require A2S query success. They still require explicit human confirmation, a note and every configured public port. This restores the frozen spec's separation between A2S diagnostics and real-client URI evidence. The development nodes' verification and enabled flags remain false.

## NOT VERIFIED

- Real client entry for Steam and steamchina across every configured public port, browser cold-start and asymmetric NAT behavior. No entry has been marked verified on development nodes. P5C is not marked PASS.
- The current mapping sets are Linux 28000–28009 and Windows 28100–28109. The earlier real-client `connect` checks on Linux 28000 and Windows 28100 do not establish URI behavior on either complete set. Windows still has no independently verified public URI route.
