# P2B validation — Party request owner and leader permissions

Date: 2026-09-25. P2A checkpoint: `7c8bf955b42814a6fc9b4c2711529f8c2b38159a` (GitHub CI success).

## Implemented

- The player request API now resolves the current owner from the server-side Session's User membership. A solo User retains User ownership; a Party member has the Party owner, and only its leader may submit. Ordinary member POST returns a clear 403 instead of creating a solo request.
- A Party row lock, the existing Party partial unique index, and the current-request lookup preserve duplicate POST behavior for a Party owner. Every current member reads the same request and Allocation. Request and Allocation lookups require current ownership, so a guessed foreign ID does not grant access.
- Party submissions check member count against the selected GamePreset's `max_players` before inserting a ServerRequest. The separate Platform Party size remains a membership limit.
- The stop path checks the current Session User's Party role before creating a NodeJob. Only the leader may stop a Party-owned running request. Solo stop remains tied to its User owner.
- Leader leave remains rejected because V1 has no transfer. Disband checks blocking requests and unreclaimed Allocations; the Party itself remains a distinct durable owner.

## Checks

- Local `go test ./... -count=1`, `go vet ./...`: PASS.
- Development PostgreSQL disposable-schema store suite: PASS, including solo/Party owner exclusivity, member create/stop denial, foreign request denial, shared current request, duplicate Party POST, disband guard, GamePreset size precheck, and prior P1 lifecycle tests.
- Web lint, typecheck, test, build: PASS. No P2 Web UI change is claimed at this checkpoint.

The real Party-owned d2core lifecycle and UI remain for P2C/P2D. GitHub CI result and the complete P2B checkpoint SHA are recorded after push.
