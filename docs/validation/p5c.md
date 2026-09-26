# P5C entry checkpoint (in progress)

## Implemented

- Controller checks A2S_INFO over local UDP for every currently Ready instance, including challenge handling. A2S remains a diagnostic and does not determine `connect` or scheduling eligibility.
- Admin entry verification requires an explicit human confirmation, note and exact full set of currently configured public ports. Verified and enabled remain separate. A network revision change clears verification, enabled state and port evidence. Migration invalidates legacy verifications that lacked full-port evidence.
- Player page displays a Steam or steamchina URI only when the backend supplies it after verified and enabled checks. It always retains `connect` help, including the client cold-start and Windows Win+R fallback.
- Unit tests exercise A2S challenge. PostgreSQL tests cover partial-port rejection, full-port acceptance and revision invalidation; their CI result is pending.

## NOT VERIFIED

- Real client entry for Steam and steamchina across every configured public port, browser cold-start and asymmetric NAT behavior. No entry has been marked verified on development nodes. P5C is not marked PASS.
