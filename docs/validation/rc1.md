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

Engineering implementation checks are in progress; **not yet RC1 READY**.
Initial local Windows Go full suite on fresh disposable PostgreSQL 16.4 passed,
including Store/HTTP/A.4 regressions. The expanded 18→19 history snapshot fixture
passed after correcting its synthetic create sequence to use accepted before
succeeded and a structured JoinInfo-unavailable reason. This was a test-fixture
correction, not a business behavior change. Web clean npm ci/lint/typecheck/27
tests/build passed. Coreproof and vet passed. Final SHA must rerun all gates.

CI now includes Linux backup→independent restore→row snapshot/migration sanity
and disposal, systemd syntax with isolated paths, pointer switch/rollback fixture,
all supported package builds and native Linux/Windows artifact smoke. Final
results, commits and CI run are recorded when these gates complete.

## Final identity and artifact evidence

The eventual candidate SHA is the exact clean commit verified by the final CI
run and handoff. Its source-dependent hashes cannot be embedded into the same
commit. `MANIFEST.json` records each filename, size, SHA256 and sourceSHA;
`SHA256SUMS` covers all nine artifacts and the manifest. Retain these ignored
release outputs with the final handoff; see [build procedure](../release.md).
No artifact is committed to Git or published as a GitHub Release.

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
