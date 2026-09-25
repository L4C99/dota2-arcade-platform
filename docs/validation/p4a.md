# P4A create, stop, and unknown failure convergence

P4 start baseline: `c233de400d9833a198624152c977b9bb3e810927` (`HEAD` and `origin/main` after fetch and fast-forward-only pull; clean worktree).

## IMPLEMENTED

- A structured create rejection before any known core ID still reaches `released_no_effect`. A later validation rejection cannot erase an earlier unknown create. The only accepted unknown-to-no-effect report is an explicit `RECONCILED_NO_EFFECT` / `reconcile` assertion after positive core reconciliation; the Controller currently keeps ambiguous unknown calls open instead of making that assertion.
- Accepted create failure with a known instance retains the Allocation and creates one durable stop job. Concurrent and repeated failure reports do not create multiple stop jobs or another Allocation attempt.
- An untrusted create operation or instance identity enters `quarantined` with occupied capacity; no automatic stop is sent to an identity the Controller cannot trust.
- A stop job reports success only after status shows `reclaimed/stopped/complete`. If an operation reports failure but status proves full reclaim, resource state can converge to reclaimed. Explicit cleanup failure, stop failure, or untrusted identity becomes `failed_with_effect` and quarantines the Allocation. A transport loss remains open for reconciliation.
- NodeJob terminal state remains separate from Allocation resource state. `quarantined` continues to occupy the old Node slot. No V1 business path invokes d2core `restart`.

## AUTOMATED VERIFIED

- `gofmt`, `go test ./... -count=1`, and `go vet ./...`: PASS on local Windows with a writable workspace Go cache.
- P4A Controller runner fault tests: structured stop rejection, accepted stop, lost stop response, terminal operation with incomplete or failed cleanup, identity uncertainty, full reclaim despite operation failure, accepted create identity uncertainty, same-key unknown create, and restart reconciliation: PASS.
- P4A Store tests on the authorized development PostgreSQL host using unique disposable schemas: unknown create keeps one attempt and occupied capacity; a later ordinary validation rejection cannot release it; accepted create failure creates exactly one stop job under six concurrent reports; unknown/accepted stop keeps capacity; explicit cleanup failure quarantines and a terminal job cannot be rewritten; untrusted create identity quarantines without auto stop: PASS.
- Full Store and HTTP PostgreSQL integration binaries: PASS. A post-test query found zero remaining `p4a_%` schemas. The two temporary test binaries were removed from the development database host. Existing business schema and services were not modified.

## REAL-ENVIRONMENT VERIFIED

- No real Dota failure was induced for P4A. The existing P3 real Ready/stop/full-reclaim baseline remains a prior fact, not new P4A fault evidence.

## NOT VERIFIED

- Deliberate real Dota stop/cleanup failures were not attempted because reproducing them would require damaging d2core or the host. P4E will test safe component restarts, sustained outage, reconnect, and full reclaim.
- This checkpoint does not include P4B's configurable quarantine timer or player escape, P4C next-game, P4D admin, or P4E real restart drills.
