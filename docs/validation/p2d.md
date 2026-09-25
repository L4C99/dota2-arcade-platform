# P2D validation — concurrency, authorization, UI and real regression

Date: 2026-09-25. P2 start SHA: `93cbc4f70ab7268f0953d57f3bc9f9745a53b8fa`.

## Automated matrix

| Area | Result | Evidence |
| --- | --- | --- |
| Party create, single membership, leader role | PASS | Migration 7 unique User membership and deferred leader membership FK; migration 8 persistent `PartyMember.role`, at most one leader and leader-role FK; PostgreSQL tests |
| Invite entropy, reset, full Party | PASS | 32 random bytes/canonical URL-safe token, leader-only retrieval/reset, concurrent last-slot test, reset/consume race and post-reset old-token rejection |
| Same User joins two Parties concurrently | PASS | Both Parties available; one join succeeds and the other returns already-in-Party; database has one membership |
| Solo and Party request owner | PASS | Exact-one-owner database check, Party FK and separate blocking partial unique indexes; solo/Party integration assertions |
| Leader create, duplicate POST | PASS | Concurrent same-leader requests return the same blocking request; one Party request, one Allocation and one create NodeJob |
| Ordinary member create/stop/reset/remove/disband | PASS | Session-backed HTTP integration returns 403; no member-created request or stop NodeJob |
| Forged User/Party/role claims and foreign IDs | PASS | Strict JSON rejects identity fields; Party B request and Allocation lookup/stop by Party A member or leader denied; stale former-member lookup/stop denied |
| GamePreset.max_players distinct from Party size | PASS | Over-limit request rejected before insert; an already-running instance remains active when later joins grow the Party |
| Active join, leave, removal, persistence | PASS | PostgreSQL lifecycle integration and real HTTPS/d2core flow; same request/JoinInfo visible to new member, owner/history unchanged, Party and invite retained after reclaim |
| Two Parties争 single-node last capacity | PASS | Concurrent allocation attempts against one effective slot produce one Allocation and one NodeJob; no oversell |
| Leader leave/disband while blocking | PASS | Backend denies both; idle disband uses formal API and leaves historical Party row |
| Web lint, typecheck, unit test and build | PASS | Formal Vue Party/Invite/member/active state uses P2 API; all frontend gates pass |
| Independent browser-context UI run | NOT VERIFIED | In-app browser opened the HTTPS tab but state capture timed out twice; three independent formal HTTP Sessions passed the live multi-user flow |
| Final owner UI acceptance | PENDING | Request once after the functional checkpoint and focused parity review |

## Real development regression

The owner-authorized isolated development database was backed up before migrations 7 and 8. The owner explicitly configured `PlatformSettings.max_party_size=10`; it was not inferred from n6's independent `max_players=10`. Migration 8 upgraded an existing live test Party and preserved its leader/member roles. A new Party created after migration 8 exposed a persisted leader role and invite, then was formally disbanded while idle.

Three independent server-issued anonymous Sessions completed the real HTTPS sequence: A created Party; B consumed its invite; A/B saw the same Party; A created one Party-owned ServerRequest; B saw the same request and its forged stop was rejected; the existing Controller and fixed d2core v0.1.1 reached Dota Ready and produced valid JoinInfo; C joined while the instance was running and saw the same request, Allocation and connect command; B left without stopping it; A stopped the server; d2core reported `lifecycle=reclaimed`, `process=stopped`, `cleanup=complete`. Party, leader, remaining member and active invite persisted. No human Dota client connect was repeated, as P1 already established that path.

The sole node still has effective capacity 1 and occupancy 0. At the last read-only check, Platform had seven historical reclaimed Allocations and no active/unknown business work; fixed d2core `list` was empty. One live test Party remains with A as leader and C as member. No Party or database history was cleared to manufacture a clean state. No d2core, VPK, template, firewall, NAT, cloud security group or production setting was changed. Private host details and backup paths remain in ignored `.local/p0-host-inventory.md`; no Session or Invite token appears in this public record.

## Focused P-1 Party parity review

The formal Party view follows the accepted P1 palette and the P-1 Party structure: member card and role badge, separate invite card with copy/reset hierarchy, active-server side card, two-column desktop and one-column mobile layouts. Existing server request and JoinInfo panels remain the same components. The mock-only P-1 scenarios for future P3/P4 capabilities are not wired into the formal Web. Static CSS/template inspection and a successful served bundle establish implementation parity; rendered desktop/mobile visual confirmation is **NOT VERIFIED** because the available browser tool timed out. Project-owner visual confirmation remains required before declaring P2 overall PASS.

Local `go test ./... -count=1`, `go vet ./...`, and frontend lint/typecheck/test/build passed. The complete store and HTTP integration suites passed against disposable schemas on the authorized development PostgreSQL. P2D functional and UI-source checkpoint `69450e1eff23a85e3e9dd203b1c8ba4d7a7c4b74` was pushed to `origin/main`; GitHub CI run `36134030542` completed SUCCESS. The development Platform now runs a locally verified `vcs.modified=false` binary from that exact SHA (remote/local SHA256 match), with the matching P2 Web bundle. HTTPS health, homepage and hashed JS/CSS all returned 200. Migration version is 8, explicit development `max_party_size` is 10, and the only node has desired/hard capacity 1/1 with zero occupied Allocations. Fixed d2core `list` is empty.
