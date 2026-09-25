# P3B auto/manual Node selection and FIFO checkpoint

P3B adds a durable selection mode to each ServerRequest. Existing requests migrate to `auto`; `manual` records one Node ID and waits for that Node only. Auto scheduling evaluates the target content and node eligibility, then orders eligible nodes by operator priority (descending) and Node ID (ascending). Waiting requests are considered in `requested_at,id` order. A temporarily ineligible earlier request retains its queue time and does not prevent a later request from using unrelated available capacity. The scheduler serializes concurrent cycles and reserves capacity in the same transaction as the Allocation and create NodeJob.

The player API now provides presentation-safe Node choices and a safe cancel route. Cancel requires `waiting` and zero Allocation attempts and respects the P2 Party leader. Changing a manual Node is cancel plus a new request and a new `requested_at`. The Web uses the accepted P-1 visual direction for auto/manual selection, Node status, waiting reasons, and cancel. It shows no speculative queue ETA or unverified JoinInfo.

## Verification

- Local `go test ./... -count=1` and `go vet ./...`: PASS.
- Full Store and HTTP API Linux integration binaries run as `arcadedev` against the existing development PostgreSQL database: PASS. Every test used its own random disposable schema; read-only post-run schema query found none, and both temporary binaries were removed. Existing business schema, services, and content were untouched.
- Store cases cover priority after eligibility, auto/manual/FIFO, unrelated capacity while manual waits, safe cancel and new queue time, global/game/preset/node maintenance, stale heartbeat, compatibility, template/content mismatch, and full capacity. Existing P0/P1/P2/P3A Store tests also passed.
- HTTP integration covers safe Node response, durable manual intent and duplicate submit, ownership of cancel, unsafe duplicate cancel, and default auto replacement. Existing HTTP integration tests passed.
- Web lint, typecheck, unit tests, production build: PASS. Isolated Edge fixtures at 1440, 390, and 320 CSS px exercised manual submission, full-node waiting reason, safe cancel, and no horizontal overflow. Desktop/mobile selector, waiting, and cancelled screenshots were visually inspected against the accepted layout direction.

## Remaining limits

- The new binary and migration were tested in disposable schemas only; no development business schema migration, Platform service replacement, real NodeJob, Dota create, or real player connect was performed in P3B.
- Node availability is a momentary hint. Capacity and other facts are checked again inside the scheduler transaction; the UI gives no exact ETA.
- P3C stale/offline and unknown-side-effect recovery, P3D second real Node/Drain, P3E external topology, and P3F dual-platform validation remain in their authorized later substages.
