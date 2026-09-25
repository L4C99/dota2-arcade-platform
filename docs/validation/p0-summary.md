# P0 validation closure

Date: 2026-09-25. The project owner formally accepted P0A through P0E.
This is the **P0 stage acceptance**, not an independent V1 code review, RC,
Production Release, or production deployment acceptance. No P1 work is
authorized by this record.

| Substage | Scope | Owner acceptance | Validated checkpoint |
| --- | --- | --- | --- |
| P0A | Repository, build, CI | PASS | `08307e31579c833b39d3ebc8633057e5413eec2b` |
| P0B | PostgreSQL, migrations, sessions, minimum durable NodeJob | PASS | `4e47a6548c6bdcfe3e837888c4247d4d3125b18b` |
| P0C | Node API v1, authentication, heartbeat, job claim/prepare/report | PASS | `e1a15a16fc9ac1acc498b99b354276906c89f58c` |
| P0D | Real fixed d2core calls and Dota Ready/reclaim | PASS | `6d55346100bd3a47b77a5b4d57f35d90f9544780` |
| P0E | Restarts, lost response, unknown reconciliation | PASS | `0fc1269a8c4134906dcc7bf3425a2dc968321831` |

**P0 overall: PASS. Owner acceptance: CONFIRMED.** The P0E SHA above is the
validated implementation baseline. This documentation-only closure commit does
not change that baseline.

## Fixed d2core dependency

P0D and P0E used the official d2core Release/tag `v0.1.1`, commit
`988720ad85af1f0d97bfe98ec4da4fcbb070beea`, protocol version 1,
template schema version 1, and disk format version 2. The binary identity was
checked on the game node. Later behavior on d2core `main` must not replace
facts from this fixed Release.

## Verified control loop

The real development loop was Platform Server → Node Controller → fixed d2core
→ Dota dedicated instance. P0D exercised create, operation, status, list,
Ready, stop, and full reclaim. P0E exercised Platform Server, Controller, and
d2core manager restarts while a Dota instance was running; d2core recovered
the original process and instance. A real accepted create response was
deliberately discarded: the durable job moved through
`unknown → accepted → succeeded` using its original idempotency key and frozen
request, with no second instance. After the node heartbeat expired, the
original unknown job stayed on its original node and reconciled on reconnect
through list plus operation/status. Stop was accepted as complete only after
`lifecycle=reclaimed`, `process=stopped`, and `cleanup=complete`.

At closure, d2core `list` is empty, no Dota dedicated process runs, and there
is no unresolved unknown or failed-but-unreclaimed test instance. The one
historical P0D `failed_with_effect` create is retained for audit; a separate
successful stop job fully reclaimed that instance. The development Platform,
PostgreSQL, Caddy, Controller, and d2core manager remain running for later
authorized work. See [P0D](p0d.md), [P0E](p0e.md), and the
[environment checklist](p0-environment-checklist.md) for the evidence and
limits. Real host facts are kept in the Git-ignored local inventory.

## Ready timing and startup timeout

Successful real create acceptance to d2core Ready took approximately
**10–16 seconds** in current P0 samples: about 13, 12.4, 15.6, and 10.1
seconds. An earlier, separate test used a 120-second startup timeout; its
Steam connection success marker did not appear before that deadline and
d2core returned `START_TIMEOUT`. That run does **not** measure normal startup
latency as 120 seconds, and its instance was fully reclaimed.

The successful development tests used a template variant with a **300-second
startup timeout ceiling**. This is a development limit, not the observed
startup duration or a production setting. Production `TemplateRevision` and
production startup timeout are **NOT YET FROZEN**; decide them later using P1
player end-to-end results and more real Dota startup samples.

## Checks and remaining limits

The final P0E implementation passed local `go test ./... -count=1`,
`go vet ./...`, and [GitHub CI on Ubuntu and Windows](https://github.com/L4C99/dota2-arcade-platform/actions/runs/36110790325).
Real Ubuntu development hosts exercised the Node API, PostgreSQL persistence,
trusted HTTPS, fixed d2core, and Dota. The development HTTPS health endpoint
returned 200 during closure checks. The local PostgreSQL test requiring
`PLATFORM_TEST_DATABASE_URL` was skipped; real job persistence was checked
through the development Platform API and database.

The following remain **NOT VERIFIED** and are inputs to later stages:

- Actual recovery after waiting for d2core's 30-day history to expire. The
  retention boundary was tested with simulated old timestamps only.
- Real d2core/Controller operation on the Windows VM.
- Human Dota client join and public game-port reachability.
- A2S with real players/public network; Steam URI and steamchina URI behavior.

These limits do not reverse the owner's P0 acceptance. P0 implemented the
minimal durable NodeJob control loop; player allocation and related business
flows belong to later authorized stages.

## Development-to-production carry-forward

The Linux Dota addon currently points to a development test content copy; its
prior content is preserved in a development backup. PostgreSQL package
installation left package-level service/settings, and development Caddy uses
its own configuration and a file capability. Development Platform,
PostgreSQL, Controller, and d2core processes remain running. Production
directories, database, Node Secret, and runtime configuration have not been
activated. Review these facts during a separately authorized deployment
transition. No environment cleanup, tag, Release, RC, or production deployment
was performed for P0 closure.
