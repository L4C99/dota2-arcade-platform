# A.1 Candidate preparation record

Date: 2026-09-27. Starting local HEAD and fetched `origin/main`: `75dd861679820f07cfe80f007aab990a58990e40` (`docs: close P5 under amended human trial gate`). The starting worktree was clean. P5 is Owner Accepted / Closed under V1.0 Amendments 001 and 002; this pass prepares a fixed repository tree for subsequent independent review. It does not perform that review, RC, deployment, tag or release.

## Repository disposition

- No tracked file was deleted. No unused production path could be established with enough certainty to remove safely. `cmd/`, `internal/`, `web/`, `deploy/`, `configs/`, embedded migrations and their tests were inspected for obvious debug code, generated output, stale examples and orphan assets. No business code or migration was changed.
- `prototype/p1/` remains a P-1 historical mock UI reference. `web/` is the current product frontend. `docs/validation/` checkpoints and P0–P5 summaries remain as development evidence; the new index explains their point-in-time status. Original P5C/P5F rule and pending records remain historical; current authority is the frozen spec with Amendments 001/002 and the P5 closure.
- README now describes completed P0–P5 implementation, component/platform boundaries, V1 exclusions, prerequisites, build/test commands, release artifact path and remaining gates. A short architecture map supplies the previously missing `docs/architecture.md`. The top of `v1.md` clarifies that §52's P-1 next-step statement is a historical freeze-time record. P5C gained a link to final P5 closure and its development public IP was redacted without changing test conclusions. `.gitignore` now also covers Content Tool local binaries, logs and common backup outputs.
- Production content names, GamePreset names and development database objects were left untouched. Formal production catalog naming belongs after RC1 and before RC2 / first external trial, as directed by the owner.

## Security and configuration scan

Tracked paths were checked for environment files, keys, private certificates, VPKs, dumps, logs, binaries and generated Web output; none is tracked. The ignored `.local/`, `.local-appdata/`, `.cache/`, `web/dist/` and `web/node_modules/` were not added. Text searches covered password/passwd, secret, token, cookie, session, private key, SSH, database URL/DSN, Authorization/Bearer, Steam credentials/Guard, Admin and Node credentials, IP/hostname, private absolute paths, `.env`, pg_dump/backup, VPK/crash data, player logs, SteamID and nicknames. Matched lines were classified as field names, code, documentation, localhost or reserved example values. No actual credential, private key, dump, VPK or player log was found in the final tracked tree. The development public IP in `p5c.md` was removed from the current tree; prior Git history is intentionally not rewritten.

Controller JSON examples parse and use the current config keys. Platform environment examples use placeholders; production keeps TLS verification (`sslmode=verify-full`) and no real password. Caddy/systemd/Windows/backup assets use reference paths and do not contain host-specific credentials. Windows unattended installation and fresh Ubuntu systemd installation are not claimed as verified.

## Migration and build checks

Embedded migration numbers are unique and continuous from 0001 through **0018**. `migrations.go` embeds these files and checks applied checksums; no historical SQL was edited or squashed. The latest-version upgrade fixture expects 18. Rollback remains forward-migration-aware: replacing a binary does not revert schema; `docs/operations.md` requires backup, separate restore and resource reconciliation for incompatible recovery.

Local checks after cleanup: `gofmt -l cmd internal` clean; `go test ./...` PASS; `go vet ./...` PASS; `go build ./cmd/platform-server ./cmd/node-controller ./cmd/content-tool` PASS; Web `npm run lint`, `npm run typecheck`, `npm test` (22 tests) and `npm run build` PASS; Windows deployment PowerShell scripts parsed; example JSON parsed; `git diff --check` PASS. The initial sandboxed Vitest launch could not spawn esbuild (`EPERM`); the same test and production build passed outside that restriction. `PLATFORM_TEST_DATABASE_URL` was not set locally, so PostgreSQL integration tests rely on the exact-SHA CI job. Linux shell syntax is checked by CI.

The CI workflow has four required jobs: Ubuntu Go, Windows Go, Web and PostgreSQL integration/migration. Cleanup commit `6c018546250917eea650e2b37fd902ccd2c617fa` passed all four in [CI run 36271411983](https://github.com/L4C99/dota2-arcade-platform/actions/runs/36271411983). This result is for the cleanup tree before this record was updated. The final document-containing Candidate SHA must independently pass the same workflow; its exact run URL and freeze decision are recorded with the final handoff. Prior P5 CI is historical evidence and does not satisfy this gate.

## Deferred and not verified

- **DEFERRED TO A.7 / FINAL RELEASE BLOCKER:** two real people in one Party entering and playing in the same real Dota instance through the formal Player Flow, followed by normal end or next game and full reclaim. Amendment 002 explicitly permits A.1 Candidate Freeze before this trial; it does not waive final Release acceptance.
- **NOT VERIFIED:** Windows public Steam/steamchina entry and public player route; fresh Ubuntu reference installation; Windows unattended startup/restart; production TLS/Secret configuration, production deployment and production recovery. P5D/P5E remain development/reference PASS with those limitations. These are later installation, RC and rollout checks, not evidence of a repository hygiene blocker.
- Formal RC1 artifacts, checksums, third-party redistribution review and production catalog naming remain future stage work. They are not produced by this preparation pass.

Candidate Ready requires a clean tree, `origin/main == HEAD`, no unresolved Candidate BLOCKER and all four CI jobs green on the final SHA. No product feature is added after that fixed SHA; next authorized step is independent review of that SHA.
