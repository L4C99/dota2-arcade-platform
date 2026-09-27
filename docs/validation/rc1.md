# A.5 RC1 Engineering Candidate

Starting Post-A.4 SHA: `2b2a872b7f6f784a02701b198b7f8d1bc365e5cd`.
Initial and resumed fetch both confirmed origin/main=HEAD at that SHA, clean.
Original A.2/A.3 review baseline remains
`9bf6d7d6b3195ca8af954a702ed69827f22afc4d`; no history rewrite.

## Licensing blocker — RESOLVED

The initial STOP found no historical d2core license. Owner supplied the formal
clarification at `db246b2bcce888b87d7854bb12012ea4e90e82cb`; LICENSE,
LICENSING.md, README and docs/delivery.md were reread from that exact commit.
The MIT grant explicitly covers L4C99-owned historical v0.1.1 code at
`988720ad85af1f0d97bfe98ec4da4fcbb070beea`, compilation/derivative distribution,
and its Go client incorporated into Controller binaries. Third parties keep
their own terms. Complete copyright/license and scope evidence are in licenses/.

Decision A: no d2core server binary redistribution. Runtime/protocol/disk/API
baseline remains v0.1.1, protocol 1, disk 2. GitHub annotated tag resolves to the
original fixed commit. Existing Release assets have September 21 upload/update
timestamps; ZIP digests match the fixed release checksums recorded in
[release.md](../release.md). The clarification explicitly states tag/assets were
not modified, overwritten or reissued; this work made no changes to d2core.

Platform uses MIT. Compiled Go module licenses and installed npm production
license/notice files are collected into every package. Review found MIT/BSD
runtime terms, with MIT/ISC/BSD/Apache-2.0/MIT-0/BlueOak/CC0 in the locked npm
build/test graph. No additional redistribution blocker found. Build tools and
node_modules are not shipped. npm audit retains the two A.4 moderate reports
for Vitest/@vitest/mocker (one advisory, GHSA-82fw-gwwq-j7x9), dev-only and absent
from the production bundle; no forced major dependency update was performed.

## Engineering scope

Shared JSON version output includes version, exact Git SHA, UTC build time and
actual Go VCS dirty flag. Clean build script emits five supported binaries,
production Web and three deploy packages plus standalone Web archive. Version
label is `v1.0.0-rc1`, without creating a Git tag. Web BUILD.json and package
PLATFORM-BUILD.json share the build identity. Node API remains 1; migration latest
remains 19. No SQL 0001–0019 edit or migration 20.

Only Player/Admin numeric map labels changed to `Workshop ID <number>`.
No layout, style, API, DB or business behavior changed. Controller's offline
`check` and command help/version exit before creating clients or contacting
services; the heartbeat/run/lifecycle code is unchanged.

Deployment docs cover private account/directories, fixed core verification, jq,
Windows ValidateOnly, paired binary/Web version directories, backup/migrate/
switch/health ordering and schema-aware rollback. The pointer-only helper does
not migrate, restart services or assert health. Backup uses private libpq files,
0600 custom-format output, TOC checking, unique temporary files and no overwrite.
Restore is explicitly independent of Dota reclamation.

## Validation state

Implementation SHA `23b4b2c0352653d0deb2fda244c310053028e1ef` passed all five jobs
in [CI 36310003148](https://github.com/L4C99/dota2-arcade-platform/actions/runs/36310003148):
Ubuntu Go, Windows Go, Web/package/Linux smoke, PostgreSQL, Windows artifact smoke.
The final document/gate commit must independently pass these jobs before the
handoff declares RC1 READY. The final run is discoverable by its exact head_sha
in GitHub Actions and is linked in the handoff; this earlier run is not substituted.

| Gate | Actual verification |
| --- | --- |
| Go | Local Windows full `go test ./... -count=1` with fresh disposable PostgreSQL 16.4 PASS (Store 38.850s, HTTP 3.631s); Ubuntu/Windows exact implementation-SHA CI test/vet/build PASS; gofmt and diff whitespace checks PASS |
| Fixed coreproof | `go -C tests/coreproof test -mod=readonly -count=1 ./...` PASS locally and both CI OSes; no IPC/Dota claim |
| Web | Clean npm ci, lint, typecheck, all 27 existing tests, production build PASS; same gates run inside packaging |
| Migration | Fresh 1→19, 18→19, repeated current 19 PASS; existing checksum rejection test retained; all 0001–0019 Git SQL blobs unchanged from Post-A.4; LF attributes and build-time Git-byte comparison protect cross-platform embedded checksums |
| History | Upgrade compares full snapshots of User/Session, Party/members/invite, requests/allocations/jobs/frozen executions/reports, content/catalog, Entry and Audit; pre-existing next-game intent fields preserved, new failure_reason NULL; A.4 invalid-next-game eligibility/failure-reason tests remain PASS |
| Backup/restore | Linux CI uses separate disposable source and restore databases, private 0600 libpq service file, actual helper custom-format dump/TOC, 0600 output and no-overwrite/private-file rejection; restores with pg_restore, compares full key-table rows, reruns migration/checksums at 19, then drops both DBs and removes test files. Final gate also checks failed dump cleanup |
| Deploy | bash syntax, JSON parse, missing-jq rejection, isolated systemd unit verify and pointer update/rollback/traversal-rejection PASS. PowerShell parse and isolated ValidateOnly PASS. Final native Windows artifact gate additionally removes each required binary/BUILD.json to verify rejection; no scheduled task registered |
| Artifact | Five supported binaries, ELF/PE amd64 check, clean SHA/version/time identity, archive contents, Linux executable modes, SHA256 and required notices PASS. Native Linux/Windows help/config/Content Tool prepare/status/invalid-path smoke uses disposable synthetic data, not game VPK |
| Platform artifact | Packaged Linux Platform migrate, /healthz, player/admin static routes and matching Web BUILD.json PASS with a disposable DB; subprocess stopped and database dropped |

Caddy was not available in these runners/local environment, so actual Caddy
binary syntax validation is **NOT VERIFIED / optional check unavailable**. The
reference dedicated header overwrite was statically checked; existing A.4 trusted
proxy login-throttling regression passed. systemd verification substitutes
isolated executable/directory paths and does not claim a fresh Ubuntu installation.

Preflight failures are retained: initial build printing Unicode test symbols
failed under Windows GBK; output now explicitly uses UTF-8. First backup fixture
quoted INI service values as connection-string values, causing a hostname lookup
failure; corrected to libpq service syntax. Its initial create upgrade fixture
was corrected to use accepted before succeeded and structured missing JoinInfo.
These corrections did not change lifecycle/business semantics. A transient
GitHub TLS push failure succeeded on normal retry; no force push/history rewrite.

Current-tree hygiene scanned tracked paths and text for credentials, token/cookie,
private keys, Admin/Node/Steam credentials, DSNs, private/public development hosts,
SteamID/nicknames, .env/.local, dumps/logs/VPKs and build outputs. No sensitive
payload or prohibited tracked artifact found. Matches are field names, fixtures,
loopback CI credentials, reserved example addresses and example placeholders.
Archive allowlists and integrity smoke reject secrets/game assets/backups/local
directories; notices are retained. Historical checkpoints remain point-in-time
records, with superseded context in the validation index.

## Final identity and artifact evidence

The eventual candidate SHA is the exact clean commit verified by the final CI
run and handoff. Its source-dependent hashes cannot be embedded into the same
commit. `MANIFEST.json` records each filename, size, SHA256 and sourceSHA;
`SHA256SUMS` covers all nine artifacts and the manifest. Retain these ignored
release outputs with the final handoff; see [build procedure](../release.md).
No artifact is committed to Git or published as a GitHub Release.

Commits before final closure:

- `24e3ef6`: license resolution, exact historical-code grant and MIT notice.
- `d1376a6`: RC1 identity, packaging, deploy assets, documentation and numeric UI labels.
- `23b4b2c`: libpq fixture/Windows encoding correction and packaged Platform health.
- The closure commit containing this record completes LF checksum protection,
  Windows package dependency negatives and backup error cleanup, then reruns CI.

Artifacts (all version v1.0.0-rc1; exact full source SHA, size and SHA256 are in
the final MANIFEST.json/SHA256SUMS):

| Filename | OS/architecture |
| --- | --- |
| platform-server-linux-amd64 | Linux amd64 |
| node-controller-linux-amd64 | Linux amd64 |
| content-tool-linux-amd64 | Linux amd64 |
| node-controller-windows-amd64.exe | Windows amd64 |
| content-tool-windows-amd64.exe | Windows amd64 |
| dota-arcade-v1.0.0-rc1-control-plane-linux-amd64.tar.gz | Linux amd64 + Web |
| dota-arcade-v1.0.0-rc1-game-node-linux-amd64.tar.gz | Linux amd64 |
| dota-arcade-v1.0.0-rc1-game-node-windows-amd64.zip | Windows amd64 |
| dota-arcade-v1.0.0-rc1-web-any.tar.gz | Browser bundle |

## Known limitations / remaining gates

1. Fixed-core unknown create without trustworthy IDs has no safe automatic
   no-effect proof: fail-closed, quarantine, Owner escape, retained capacity and
   history, separately authorized Agent-operated node retirement/rebuild.
   Capacity may be lost permanently; automatic recovery is not implemented.
2. Windows public Steam/steamchina entry: NOT VERIFIED.
3. Fresh Ubuntu reference install: NOT VERIFIED, later environment gate.
4. Windows scheduled-task unattended install/reboot: NOT VERIFIED.
5. Production TLS/Secrets, DB, recovery and rollout: NOT VERIFIED.
6. Two or more real people in the same Party and same Dota instance through
   Player Flow, normal play, stop/next game and full reclaim: DEFERRED TO A.7,
   still BLOCKS FINAL v1.0.0 RELEASE.

RC2 preparation remains separately authorized: formal production catalog/names,
App570 check, private production branding, fresh deployment, Windows unattended,
Owner RC2 smoke, Windows public entry if applicable, and A.7 real multiplayer.
No production access, real Dota instance, App570 update, task registration, tag
or GitHub Release was performed for this engineering-only change.
