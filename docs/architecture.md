# V1 architecture

The [frozen V1 specification](specs/v1.md), [Amendment 001](specs/v1-amendment-001-entry-verification.md) and [Amendment 002](specs/v1-amendment-002-human-trial-gate.md) define product and acceptance rules. This page is a repository map for implementation review.

```text
Browser ─HTTPS→ Caddy ─loopback HTTP→ Platform Server ─→ PostgreSQL
                                     ↑
                                     │ HTTPS, Node API v1 (Controller initiated)
                                     │
             Node Controller ─local API/client→ d2core v0.1.1 ─→ Dota instances
                    │ reads
             Node content files ← Content Tool (offline, operator invoked)
```

- `cmd/platform-server/`, `internal/platform/httpapi/` and `internal/platform/store/` hold the single Platform process, HTTP APIs, scheduler state and database access. PostgreSQL migrations are embedded from `internal/platform/store/migrations/`; current latest is 19. `ServerRequest`, sequential `Allocation` attempts, durable `NodeJob` and d2core instance are distinct identities.
- `cmd/node-controller/`, `internal/controller/` and `internal/contracts/nodev1/` hold the Windows/Linux Controller and wire contract. Only the Controller calls d2core, through its supported local Go client. A create job freezes its resolved request and key before the first core call; unknown effects require reconciliation. Capacity is released only after full reclaim.
- `cmd/content-tool/` and `internal/contenttool/` hold the offline Windows/Linux version tool. It does not Drain, call Platform, or change the published catalog. The Controller reports actual local content and its digest; Platform records the target for future Allocations.
- `web/` is the current Vue app. It calls Platform only. `prototype/p1/` is retained historical mock UI, not a served application.
- `deploy/` contains reference Caddy, systemd, Windows startup and PostgreSQL backup assets. They are deployment examples; production rollout and unattended install verification remain later gates.

For detailed behavior see the [Node protocol](node-protocol.md), [player API](player-api.md), [administrator API](admin-api.md), [operations runbook](operations.md) and [P5 closure](validation/p5-summary.md). Game traffic goes directly from the Dota client to the game node, not through Caddy or Platform.
