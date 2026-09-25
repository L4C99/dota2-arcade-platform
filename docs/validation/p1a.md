# P1A validation — catalog and solo ServerRequest

Date: 2026-09-25. Scope: P1A only. P1B Allocation dispatch and later player flow are not claimed here.

## Implemented

- Added durable PlatformSettings, ArcadeGame, ContentVersion, TemplateRevision, GamePreset, NodeTemplateBinding, and solo User-owned ServerRequest records. The schema reserves a distinct Party owner field for P2 and enforces exactly one owner.
- ContentVersion rows are immutable. ArcadeGame's current version must belong to that game. GamePreset references a same-game logical TemplateRevision. The node binding stores only a logical key; the Controller's existing trusted local config resolves the absolute path.
- Added session-authorized player catalog, create request, current active request, and owner-authorized request lookup APIs. POST derives owner solely from the existing server-side Session. An owner row lock and partial unique index enforce one blocking request even under concurrent submissions.
- New requests check global, ArcadeGame, and GamePreset enable/maintenance controls. Duplicate requests return the existing request even during maintenance.

## Development data and checks

The single P1 development catalog uses Workshop `3564393242`, n6 with `max_players=10`, and owner-confirmed development ContentVersion `p1-test-v1`. Its immutable content hash matches the previously verified VPK and a fresh read-only hash of the current node file. The logical development TemplateRevision maps to the P0-validated 300-second-ceiling n6 template through an existing Controller binding key. Neither value is a production release decision. No VPK, addon link, patcher, or host network configuration was changed.

- Local `go test ./... -count=1`: PASS. Local `go vet ./...`: PASS.
- Real isolated PostgreSQL schema tests on the development database: `TestPostgresDurability`, `TestUpgradeFromP0B`, and `TestP1ARequestConcurrencyAndMaintenance`: PASS. The concurrency test submitted twelve requests for one User and observed one durable request. It also checked maintenance, owner access, and ContentVersion immutability.
- Development database backup was taken before migration 4. Migration and one-catalog seed succeeded without replacing the P0 data.
- Candidate HTTP handler against the development database: anonymous Session, catalog, first POST (201), duplicate POST (200 with same ID), and current request recovery: PASS. The pure waiting test request was then marked cancelled. Final P1A state: no blocking player request and no non-integration NodeJob.

GitHub CI and the checkpoint SHA are tracked with the pushed P1A commit. Public game-port reachability and human Dota join remain NOT VERIFIED for P1A.
