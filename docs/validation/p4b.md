# P4B quarantine and player escape

P4A checkpoint: `f786c950b28df5e0705d9c69fbcd0738d5e9d135`; GitHub Actions run `36180213248` completed successfully across Ubuntu Go, Windows Go, PostgreSQL integration, and Web.

## IMPLEMENTED

- Migration 11 adds `PlatformSettings.quarantine_after_node_unreachable` with the frozen 15 minute default, `ServerRequest.abandoned` / `abandoned_at`, and `Allocation.quarantined_at`. The Platform `serve` command applies `PLATFORM_QUARANTINE_AFTER_NODE_UNREACHABLE` as a positive Go duration, using `15m` when unset. Example development and production configuration files show the default.
- A durable Platform scan isolates one possibly effectful Allocation when its Node has been unreachable beyond the configured threshold. It is restart safe and repeatable. Pure pending reservations are excluded. `quarantined` retains capacity, node, content snapshot, instance/job history, and the old port. A late ordinary NodeJob report cannot undo isolation; a proven full stop/reclaim can release it later.
- Database authorized operators can mark an effectful or possibly effectful Allocation quarantined before the threshold with `platform-server allocation quarantine <allocation-id>`. A pure reserved attempt whose create job is still pending is rejected. This is a state and admission action, not a process stop or capacity release. P4D adds the administrator Web surface and Audit.
- The owner or current Party leader can send `POST /api/v1/server-requests/{id}/abandon` with `{ "confirm": true }` after quarantine. The player UI requires a second confirmation that says the old Dota may still run and capacity remains occupied. The action changes only the old ServerRequest to `abandoned` and allows a subsequent separate request. Repeated calls are idempotent. Ordinary members and unrelated users are denied.
- Player states distinguish unknown/Node outage, cleanup exception, and abandoned history. The UI follows the P3 ivory, ink green, sage, and restrained warm warning styling; it never declares the old port or server stopped merely because of abandonment.

## AUTOMATED VERIFIED

- `gofmt`, `go test ./... -count=1`, `go vet ./...`, Web lint/typecheck, four Vitest tests, and production Web build: PASS locally.
- Migration 11 upgrade test applies the actual frozen P3 migrations 1–10 in a disposable schema, seeds an ended request, reclaimed Allocation, and NodeJob, upgrades to 11, and verifies those identities and history survive. Idempotent reapplication: PASS.
- Disposable PostgreSQL tests: frozen 900 second default; configured shorter test threshold; no early quarantine; threshold quarantine without capacity release; repeated scan; late create acceptance retention; manual early quarantine; unrelated user and Party member denial; repeated confirmed abandon; exactly one new business request; old node/content/history unchanged; later full reclaim after abandon releases capacity while old request remains abandoned: PASS.
- HTTP integration verifies explicit confirmation, unrelated player rejection, idempotence, and old occupied capacity: PASS. Full Store and HTTP PostgreSQL suites passed; zero `p4b_%` disposable schemas remained. Temporary remote test binaries were removed. Existing development business schema and services were not changed for these tests.
- Local Edge functional fixture checked quarantined and offline/unknown states at 1440, 390 and 320 px, plus the dismiss/accept confirmation flow at 390 px. All had no horizontal overflow. Screenshots are kept in ignored `.local/p4b-*.png` for later UI parity comparison.

## REAL-ENVIRONMENT VERIFIED

- No real heartbeat outage or Dota failure was induced in P4B. P4E will run the authorized sustained Linux/Windows outage and restart drills after formal development migration and backup.

## NOT VERIFIED

- Development services have not yet been upgraded to migration 11; the formal development DB backup/migration and real Node quarantine/escape drill remain for P4E.
- P4C next-game consumption of a paused intent, P4D administrator Web/Audit, and final owner UI acceptance remain later P4 work.
