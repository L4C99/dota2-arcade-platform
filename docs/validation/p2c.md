# P2C validation — membership changes during an active server

Date: 2026-09-25. P2B checkpoint: `dc06e130a877c70c15f6eb2d9148938d0304c67c` (GitHub CI success).

The P2A/P2B transaction paths permit invite consumption, ordinary member leave, and leader removal during a Party-owned active request. They change only Party membership. P2C adds a PostgreSQL integration test that exercises these paths against an allocated, reported Ready instance with JoinInfo, then a separate stop/reclaim NodeJob.

## Integration result

- A leader submits while the Party has one member and the GamePreset allows one player. The request allocates once, its create NodeJob reaches the simulated Controller Ready report, and JoinInfo becomes available.
- Two users then consume the live invite; both recover the same Party-owned ServerRequest, Allocation ID, and JoinInfo. Their later membership makes the Party larger than that GamePreset's max_players but leaves the running instance untouched.
- One ordinary member leaves; the leader removes the other. Both lose access to the Party request. The original request remains owned by the Party, its Allocation stays running, and the create NodeJob count remains one.
- Leader disband remains blocked while the request is active. Leader stop creates a separate stop NodeJob; full reclaim leaves Party identity, leader membership, and the invite intact. The former member can join again using that retained invite.

Local `go test ./... -count=1`, `go vet ./...`, and Web lint/typecheck/test/build: PASS. The complete store suite, including P1 regressions and this P2C test, passed against disposable schemas in the authorized isolated development PostgreSQL. This integration uses simulated Controller reports; real d2core regression is reserved for P2D. GitHub CI result and complete checkpoint SHA are recorded after push.
