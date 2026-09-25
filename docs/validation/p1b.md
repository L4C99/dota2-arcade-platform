# P1B validation — Allocation attempt and single-node capacity

Date: 2026-09-25. Scope: P1B only. JoinInfo, player Web, and human join belong to P1C–P1E.

## Implemented

- Added immutable Allocation attempt identity/history, a separate NodeJob association, Controller-reported NodeContentBinding, and Platform-controlled per-binding acceptance. The sole P1 node dispatch path atomically locks the request and node, checks all eligibility conditions, counts occupied attempts, reserves capacity, and creates a durable P0 NodeJob in one transaction.
- New Allocation attempts snapshot `ArcadeGame.current_content_version_id` at assignment. A waiting ServerRequest remains unbound. The schema prevents in-place changes to an attempt's node, content version, template revision, and request identity.
- Reused P0 claim/prepare/report and frozen create execution. Platform timestamps `create_started_at` on the first durable prepare and `ready_at` on the Controller's trusted d2core Ready report. Create unknown and all unreclaimed states retain capacity. Clear no-effect rejection releases only that attempt; a successful stop releases capacity only after the Controller verifies full d2core reclaim.
- Added owner-authorized player stop and Allocation lookup APIs. Stop obtains the instance ID from the durable create job; the browser supplies only its own request ID.
- Controller now verifies the current addon link, explicit ContentVersion metadata, and actual VPK SHA256 before reporting content confirmed. Its long-running heartbeat caches a hash only while the same file identity, size, and modification time remain unchanged.

## Tests and real development result

- Local `go test ./... -count=1` and `go vet ./...`: PASS.
- Isolated development PostgreSQL tests, including migration upgrade, concurrent last-slot reservation, content-version freeze point, unknown capacity retention, stop/reclaim, no-effect release, ownership, and binding mismatch/unknown gating: PASS.
- Development database backup preceded migration 5. The P1 owner-confirmed `p1-test-v1` metadata was written outside the addon directory as a read-only file. Current addon link and VPK SHA256 matched before and after. No VPK, patcher, addon link, d2core manager, firewall, or NAT configuration was changed.
- A real browser-equivalent HTTPS API request created one ServerRequest, one Allocation, and a business create NodeJob. The existing Controller/d2core v0.1.1 path reached Dota Ready. The owner's stop API created a distinct stop NodeJob and the instance reached `reclaimed/stopped/cleanup=complete`. Final d2core list was empty, no dedicated Dota process remained, and the Allocation was terminal `reclaimed`.

This no-queue sample used Platform UTC observations: `requested_at` 09:29:25.590429Z, `assigned_at` 09:29:26.305804Z, `create_started_at` 09:29:30.431922Z, `ready_at` 09:29:55.412045Z. Queue wait was 0.715s, dispatch/start delay 4.126s, and Dota startup 24.980s. `join_info_available_at` is null because P1C has not run. This single sample does not establish a performance SLO; P0 create-acceptance-to-Ready samples were about 10–16s under a different interval definition.

GitHub CI and the checkpoint SHA are tracked with the pushed P1B commit. Public game-port reachability, JoinInfo, and human Dota join remain NOT VERIFIED at this substage.
