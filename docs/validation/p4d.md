# P4D administrator control, security, and Audit

P4C checkpoint: `6bf9347ee3852b3a72105e02550f62b4d528cef9`; GitHub Actions run `36183718672` passed Ubuntu Go, Windows Go, PostgreSQL integration, and Web.

## IMPLEMENTED

- Migration 13 adds a lightweight SiteAnnouncement, append-only AuditEvent history, durable Node reconcile request generations, and last reported Allocation instance facts. P3/P4A–C history is preserved. Public player catalog and page display the announcement independently of maintenance.
- Existing Argon2id `AdminUser` / independent `AdminSession` / CLI create and reset-password are used. The Web API requires a valid server-side AdminSession for every read and action, exact Origin for mutations, opaque `Secure`/`HttpOnly`/`SameSite=Lax` production cookie, absolute expiry, logout revocation, disabled-user and password-reset invalidation, new login token with previous browser token revocation, and IP + username login backoff. The backend ignores client identity claims.
- `/admin` follows the accepted ivory/ink-green/sage visual system. It shows global maintenance and announcement, games/presets/current ContentVersion, Node connectivity/version/compatibility/heartbeat/capacity/priority/Drain, Controller-reported content bindings, entry states, recent requests/Allocations/Party, open NodeJobs, structured errors and Audit.
- Typed, transactional administrator actions cover global and catalog maintenance, announcement, game/preset enabled and accepting, Node Drain/Resume/priority/desired capacity, Node × game allocation admission, safe waiting cancellation, trusted-instance stop, early quarantine, durable Node reconcile request, and Steam/steamchina verified/enabled controls. Important Web and CLI operator changes write Audit with actor, action, target, time and relevant state change. Automatic unreachable quarantine writes a system Audit record. The audit payload omits passwords, tokens, Node Secret, frozen local paths, SSH data and private network facts.
- Entry verification is a human claim for the Controller's current revision and requires reported A2S availability plus explicit confirmation. `enabled` cannot be set without `verified`; Controller revision change automatically clears both. The real development entry remains unverified/disabled pending P5C human verification. This UI does not switch `ArcadeGame.current_content_version_id` or perform P5 publishing.
- Controller reconciliation performs d2core list, converges open NodeJobs, then checks status of known active instances before claiming new work. Authenticated Node facts cannot change instance identity or old JoinInfo. Transport loss stays unknown; identity failure or changed port isolates the old Allocation; only d2core `reclaimed/stopped/complete` releases capacity. An administrator's reconcile generation remains durable across disconnects and is acknowledged only after a successful pass. Acknowledgement means a pass ran, not that every anomaly was resolved.

## AUTOMATED VERIFIED

- Local `gofmt`, `go test ./... -count=1`, `go vet ./...`, Web lint/typecheck, Vitest and production build: PASS in the P4D checkpoint run.
- Full Store and HTTP PostgreSQL suites ran in disposable schemas on the authorized development database host. P3-to-current formal migration upgrade and idempotent reapplication passed; no P3 history was rewritten. The development business schema was not upgraded during this checkpoint.
- PostgreSQL and HTTP tests cover administrator cookie flags, independent authentication, session rotation and logout, disabled account, password reset baseline, same-origin rejection, failed-login backoff, per-action Audit, Controller-only binding facts, safe cancellation/stop/quarantine, capacity retention, current A2S/entry verification fixture and revision invalidation, durable reconcile generation and repeated acknowledgement, active instance identity/port check, incomplete cleanup refusal, full reclaim and paused next-game preservation. Runner tests confirm active reconciliation never calls create or stop itself and does not claim incomplete cleanup as reclaimed.
- Edge fixture inspected login and the overview/content/nodes/instances/audit pages at 1440, 390 and 320 px. No page-level horizontal overflow; screenshots are retained only in ignored `.local/p4d-*.png`. Visual comparison with accepted P2/P3 direction found matching colors, spacing, card treatment and hierarchy.

## REAL-ENVIRONMENT VERIFIED

- Not yet. P4E will back up and migrate the authorized development business database, deploy the development build, exercise real Linux/Windows Node reconnect and d2core manager restart, and inspect actual Admin/Audit persistence. The Edge run above used fixture data, not a real administrator login.

## NOT VERIFIED

- Real development service upgrade and administrator UI owner acceptance remain. Real Steam/steamchina entry verification and enabling remain P5C work; P4 did not mark them verified. The P3 carried real sustained stale/offline drill is scheduled for P4E.
