# P4 validation and closure

Project-owner confirmation on 2026-09-26, after final player and administrator UI inspection and acceptance of the happy-path timing follow-up:

```text
P4A: PASS
P4B: PASS
P4C: PASS
P4D: PASS
P4E: PASS
P4 UI owner acceptance: CONFIRMED
P4 overall: PASS
Owner acceptance: CONFIRMED
```

The owner accepted the major P4 UI corrections; remaining small visual polish is deferred to P5/RC consistency work and does not block P4. No P5 implementation or RC work is authorized by this closure.

## Checkpoints and CI

| Milestone | Full commit SHA | GitHub Actions |
| --- | --- | --- |
| P4 start / P3 closure | `c233de400d9833a198624152c977b9bb3e810927` | Prior accepted P3 fact |
| P4A | `f786c950b28df5e0705d9c69fbcd0738d5e9d135` | `36180213248` success |
| P4B | `5c1585ce96b5f56876717a43ddbb5b71b165d539` | `36182155859` success |
| P4C | `6bf9347ee3852b3a72105e02550f62b4d528cef9` | `36183718672` success |
| P4D final formatting follow-up | `2265eb077e3317acd504ee4a71a12d07f20cfa2f` | `36187355122` success |
| P4E | `ea034bd823bf3d3920809c414087ee2966c2b8aa` | `36192301200` success |
| UI parity | `c7c6b826c0b7056cbcb366421236c4e78c356873` | `36192650288` success |
| Owner feedback UI revision | `dbc466864b24fc0dc4647bfcd6ac1fee4faa0b04` | `36225225011` success |
| UI development deployment evidence | `2bf522a053a0d2df1400472bebad9d3befc00674` | `36225549834` success |
| Capacity/Party UI follow-up | `ba19df025800a03e3ccc01080ce805178add8281` | `36226367596` success |
| No-queue startup timing recheck | `77c886494fd722aa03306adf4485763148540126` | `36227878285` success |

This document is the independent docs-only P4 closure. Its commit SHA and CI result are reported after the ordinary push; a commit cannot contain its own SHA or future CI run ID.

P4D's first commit `57209dba0206ebae816af62290426d4be127f156` had a Go formatting CI failure. The formatting-only follow-up above passed all Ubuntu, Windows, PostgreSQL and Web jobs. P4A–P4E and UI details are independently traceable in `p4a.md` through `p4e.md`, `p4-ui-parity.md`, and `p4-ui-owner-followup.md`. No P0–P3 re-review or P3 reopening was performed.

## IMPLEMENTED

- P4A converges create/stop/unknown failures without equating timeout to failure, changing frozen NodeJob execution, duplicating an Allocation attempt, or releasing a possibly effectful instance before full reclaim.
- P4B preserves quarantined capacity, port and history while allowing an owner or Party leader to confirm abandoning the old business block. The default unreachable threshold remains 15 minutes and is deployment configurable.
- P4C records durable next-game intent and uses stop → full reclaim → fresh FIFO ServerRequest → new Allocation/create, never d2core instance restart. Paused intent after quarantine requires confirmed owner continuation.
- P4D adds a separate secure AdminSession, Argon2id AdminUser CLI, typed administrator controls, Controller-only node facts, append-only Audit, and a matching admin UI. P4E added the audited stop path for an abandoned Request whose old Allocation remains quarantined.
- P4E covers component restart/reconcile, real dual-node sustained stale/offline recovery, real quarantine escape and full reclaim. P4 UI parity covers player/admin states, desktop and narrow mobile behavior. P5 content publishing, real entry verification and production deployment were not started.

## AUTOMATED VERIFIED

- At each P4 checkpoint, Go formatting/tests/vet, Web lint/typecheck/Vitest/build, disposable PostgreSQL integration and Ubuntu/Windows CI were run as recorded in the detailed validation files. The final P4E code, including the abandoned-quarantine recovery regression, passed full Store and HTTP PostgreSQL suites and all four GitHub Actions jobs. Formal migration upgrade and idempotent reapplication were tested before development business schema upgrade.
- The P4A fault matrix covers structured no-effect create rejection, accepted create failure with instance, lost create/stop responses, accepted/failed stop, cleanup failure, Controller restart during unknown, repeated reconciliation/reports, no duplicate create, no duplicate release and preserved Allocation attempt history. P4B/C/D suites cover owner/Party authorization, quarantine capacity, duplicate abandon/next-game, fresh timestamp/content snapshot, admin cookie/session security, action Audit, reconcile generation and controller-only content/entry facts.
- Edge fixture browser checks passed P4 player and admin key states at 1440, 390 and 320 px, with no page-level horizontal overflow. The mobile administrator navigation shows all five tabs without hidden horizontal scrolling. The accepted P2/P3 ivory, deep green, sage and restrained orange visual direction is preserved.
- A later no-queue happy-path timing recheck is recorded separately in `p4-happy-path-timing.md`: seven normal development creates and full reclaims, including Linux/Windows, manual/auto and one continuously observed Web display. This diagnostic did not change business code or establish a startup SLO.

## REAL-ENVIRONMENT VERIFIED

- After a formal backup, the existing development PostgreSQL business database upgraded from migration 10 to 13. The development Platform runs the exact clean P4E backend checkpoint; both development Controllers run the exact clean P4D build against fixed d2core v0.1.1. The development Web was subsequently updated to the accepted P4 UI bundle from `ba19df025800a03e3ccc01080ce805178add8281`. Development HTTPS health, player/admin HTML and hashed JS/CSS assets returned 200. Earlier browser parity checks covered desktop, 390 and 320 px with normal TLS verification and no page-level horizontal overflow; the latest owner follow-ups were checked locally at narrow widths and deployed to the development Web host.
- Ready Linux and Windows Allocations retained their Allocation/Job/instance identity and JoinInfo port through Platform, Controller and fixed d2core manager restarts. Windows manager recovery passed after preserving the original ordinary-account SSH job holding the Dota child; an earlier short-lived SSH launch that ended the child was recorded as a failed test setup, not a PASS. Each tested instance subsequently reached full stop/reclaim.
- Linux durable next-game survived Platform and Controller restart and created exactly one new manual request after reclaim. A separate real quarantine drill proved paused intent across Platform restart, confirmed abandon without freeing the still-running old resource, subsequent old full reclaim and one new Ready request. Another drill proved administrator stop/full reclaim after abandon, followed by release of the old capacity and normal scheduling of the waiting request. The one-use test AdminUser was disabled afterward; its existing session returned 401.
- The prior P3 dual-node sustained stale/offline drill is now real-environment verified by P4E: both Nodes were observed online → stale at about 132 seconds → offline at about 311 seconds → reconnect. During outage, both were unavailable for new allocation, old occupied capacity remained 1/1, d2core showed the same Ready instance and port, and no duplicate create occurred. Both were formally stopped and fully reclaimed after reconnect. This is P4 evidence; the P3 validation document remains as originally closed.

## DEVELOPMENT ENVIRONMENT AT P4E CHECKPOINT READ

Migration 13; 41 historical `reclaimed` Allocations and zero active, failed-unreclaimed or quarantined; 39 ended, two abandoned and three cancelled Requests; no open/unknown NodeJob; two consumed next-game intents and none pending/paused. Both Nodes were online, compatible, Drain false, priority 0, enabled/accepting true, hard/desired/occupied `1/1/0`, and reporting `p1-test-v1 / confirmed`. Direct fixed d2core lists were empty and no Dota process was observed. Platform accepted new requests, and its quarantine setting remained 15 minutes. Steam/steamchina verified/enabled remained false. One temporary test AdminUser was disabled, its existing Session was unusable, and relevant Audit history was retained. The database was not cleared; ended/reclaimed/Audit history remains explainable.

## ACCEPTED PERFORMANCE OBSERVATION

The owner accepted the diagnostic conclusions in `p4-happy-path-timing.md`. Seven sequential no-queue development creates on Linux and Windows all reached Ready and normal full reclaim. P1E's two no-queue human samples took 17.195–18.042 seconds to JoinInfo; the new samples ranged from 16.011 to 58.902 seconds. Controller's fixed five-second worker tick produced measured NodeJob claim waits of 0.533–4.873 seconds, a candidate for later optimization. Two approximately 55-second `create_started_at → ready_at` samples, one on each OS, remain unexplained at the internal Dota/d2core/Ready-detection level. There is no clear Platform-stage performance regression evidence and no Web-only large delay in the continuously observed loopback-mirror sample. The owner chose to carry this observation into P5/RC sampling, without a P4 performance refactor or formal SLO. Direct development-domain browser rendering latency was not independently measured in that timing follow-up.

## DEVELOPMENT ENVIRONMENT AT P4 CLOSURE READ

Migration 13; development HTTPS `/healthz` returned 200; global accepting new requests remained true and the quarantine threshold remained 15 minutes. Linux `p0d-linux-cn` and Windows `p3d-win10-vm` were online/compatible with heartbeat ages 3 and 1 seconds at the read, respectively. Both were enabled, accepting, Drain false, hard/desired/occupied `1/1/0`, and reported `p1-test-v1 / confirmed` with Node × game admission true. The current priorities were Linux `-5` and Windows `0`; this read-only closure did not change them. There were zero active Requests, occupied Allocations, failed-unreclaimed or quarantined Allocations, open/unknown NodeJobs, and pending/paused next-game intents. Four historical intents were consumed. Both fixed d2core lists were empty and both Dota process counts were zero. One development AdminUser was enabled, one disabled, one enabled-account AdminSession was valid at the read, and 45 AuditEvents were retained. Steam and steamchina verified/enabled counts were zero. Historical business and audit records were retained; the database was not cleared.

The owner reported no issue in manual functional flows, including next-game, and **confirmed final player and administrator UI acceptance**. Both visual follow-up rounds are tracked in `p4-ui-owner-followup.md`. Small remaining UI polish is deferred to P5/RC; no password belongs in Git, documentation, logs or chat.

## REMAINING NOT VERIFIED

- A forced real create/stop lost response at the exact d2core call boundary and a real wait through the automatic 15-minute quarantine threshold were not repeated in P4E. Their deterministic Controller/PostgreSQL fault tests passed; P4E's real drills instead used safe component restarts, sustained outage and early manual quarantine. The frozen default was read as 15 minutes after deployment.
- Windows VM public human connect, a second independent public topology, vendor NAT, complex asymmetric NAT, and P5 A2S/Steam/steamchina verified workflow remain NOT VERIFIED. No new public Node, NAT/firewall change, content switch or P5 work was undertaken.
- The cause of the two approximately 55-second startup/Ready-stage samples and direct development-domain browser rendering latency in the timing follow-up remain NOT VERIFIED. Neither prevents this owner-confirmed P4 closure; both are carried forward for later sampling. The seven samples are not an SLO or a production performance claim.

P4 stops here. No tag, RC, Release, production deployment or P5 work is part of this closure.
