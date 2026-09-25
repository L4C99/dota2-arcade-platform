# P2 UI acceptance supplement: anonymous display names

Date: 2026-09-25. Source checkpoint: `f9b373bf5835473c0fef87ded9b988cbb379e445`. The P2A–P2D functional checkpoints remain unchanged. Owner UI acceptance is still **NOT VERIFIED**.

## Model and migration

Migration 9 adds `users.display_name`. The migration runner backfills null values in the same transaction before applying `NOT NULL` and recording the migration ledger entry. Rerunning migrations does not replace names. New User creation stores a generated name in the same transaction as its Session. Names use 48 short Chinese modifiers × 48 Chinese nouns (2,304 combinations), selected with `crypto/rand`; duplicate full names are allowed. User ID and Session remain the only identity and authorization basis. There is no name edit/profile/search API.

The authorized development database was backed up before migration 9. The migration completed at version 9: 22 pre-existing Users were backfilled, with zero null names. After browser and restart tests, 26 Users have zero null names. Existing Session/Party/ServerRequest/Allocation relationships were retained; the active Party count stayed at three across the migration. No database wipe was used.

## API and UI

`/session` and `/me` return the stable `displayName` alongside the existing `userId`. Party member rows include `displayName`; the UI still uses `userId` to identify the current member and target a removal. The page header, Party member rows, leader/member labels, and the current User marker now show the name. The display name is never an owner or permission claim.

The limited UI pass clarified reset, leave, remove and disband confirmations, including the fact that membership changes do not stop or kick an existing Dota player. The remove confirmation names the selected member. Mobile now shows the User's name in the compact header. The accepted P1 visual direction and P2 server panels remain intact.

## Verification

Local `go test ./... -count=1`, `go vet ./...`, frontend lint, typecheck, three Vitest tests, and production build passed. The full Store and HTTP integration suites passed against disposable schemas on the authorized development PostgreSQL. These include pre-migration Party/Session linkage, idempotent backfill, stable names after Store reopen, new User format, duplicate names with ID-based authorization, and Party member API names. GitHub CI run `36144264131` completed SUCCESS.

Three independent Edge browser contexts exercised A/B/C name display, Party create, invite join/reset, member list and roles, leave, remove and idle disband through the formal UI. Desktop and 390px mobile masked screenshots were inspected without an obvious layout break; the test Party was formally disbanded. A separate real development Platform restart preserved a newly created User's `displayName` and Session identity. HTTPS homepage, exact JS/CSS and health returned 200.

Final read-only development state: migration 9; `max_party_size=10`; 26 named Users; three retained Parties with six current members (explained in the [P2 total report](p2-summary.md)); seven reclaimed Allocations; no active or open unknown instance; node desired/hard capacity 1/1 with occupancy 0; fixed d2core `list` empty. No d2core, Controller, content, port, firewall, NAT or production change was made.

Project-owner Party UI confirmation remains **NOT VERIFIED**. P2 overall remains **NOT YET OWNER-ACCEPTED**. P3 has not started.
