# P4C durable next-game intent

P4B checkpoint: `5c1585ce96b5f56876717a43ddbb5b71b165d539`; GitHub Actions run `36182155859` passed Ubuntu Go, Windows Go, PostgreSQL integration, and Web.

## IMPLEMENTED

- Migration 12 adds `next_game_intents`, keyed by the immutable source ServerRequest and Allocation IDs. The intent and stop NodeJob enter the database in one transaction. Duplicate next-game clicks reuse that intent and the same stop job.
- A terminal stop report that proves full reclaim marks the old Allocation `reclaimed`, ends the old request, and consumes the intent by creating exactly one fresh waiting ServerRequest in one transaction. The new request inherits ArcadeGame, GamePreset, and auto/manual Node selection, but gets its own post-reclaim `requested_at`. Its Allocation will snapshot the then-current game ContentVersion under the existing scheduler.
- A stop failure or uncertain resource isolated as `quarantined` pauses the intent. It does not make a new request or free the old capacity. The owner/Party leader can use the existing confirmed abandon action to consume the paused intent exactly once. If the old resource is fully reclaimed after quarantine but before that decision, the request remains in its decision state; the UI explains that reclaim is complete and still asks the leader before creating the next request.
- An ordinary stop has no intent and creates no next request. No normal business code calls d2core instance `restart`.
- Player API and UI expose pending, paused, and consumed intent. The UI labels stopping, next-game pause, and the confirmed continue action. Party ordinary members cannot start or continue the next game.

## AUTOMATED VERIFIED

- Disposable PostgreSQL Store tests cover concurrent duplicate clicks, a single stop job, response loss/unknown then accepted then full reclaim, repeated terminal report, fresh request timestamp after reclaim, then-current ContentVersion at next Allocation, manual Node inheritance, quarantine pause, owner-only abandon, idempotent consumption, occupied old capacity, ordinary stop, and Party leader denial for ordinary members: PASS.
- Local `gofmt`, `go test ./... -count=1`, `go vet ./...`, Web lint/typecheck, five Vitest tests, and production build: PASS.
- Full Store and HTTP suites, including formal migration chain and P3-to-current upgrade test, passed on the authorized development PostgreSQL host in disposable schemas. Temporary test binaries were removed. The development business schema and services were not changed.
- Local Edge fixture checked ready, stopping with pending intent, quarantined with paused intent, and reclaimed with paused intent at 1440, 390, and 320 px. All 12 states had no horizontal overflow; the recovered continue button required a second confirmation. Screenshots are retained only in ignored `.local/p4c-*.png`.

## REAL-ENVIRONMENT VERIFIED

- Not yet. P4E will apply the formal development migration after backup and exercise lifecycle/restart with authorized development instances.

## NOT VERIFIED

- P4E real development restart/response-loss rehearsal, P4D administrator/Audit integration, and final owner UI acceptance remain.
