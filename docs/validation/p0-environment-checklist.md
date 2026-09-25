# P0 development environment checklist

This public checklist records confirmation status only. Actual host addresses,
private paths, credentials, and runtime state belong in the Git-ignored local
inventory. Read this checklist and the local inventory before any later
real-node session. Recheck the host before a real operation; an unverified entry is not
authority to use a value.

Status as of 2026-09-25. `confirmed` means observed on the relevant host or
verified against a fixed release; `owner-provided` means supplied by the project
owner and still subject to host checks.

| Field | Needed by | Status | Non-sensitive verification note |
| --- | --- | --- | --- |
| Three development host roles and OS | P0D | owner-provided, observed | SSH read-only inspection confirmed two Ubuntu hosts and one Windows VM. |
| Development HTTPS hostname and DNS | P0D | confirmed | Owner supplied a dev hostname; A record and trusted TLS response were checked. |
| Development TLS storage and runtime isolation | P0D | confirmed | Caddy files are under a dedicated development directory; no permanent service was installed. |
| Platform binary, config, and runtime directory | P0D | confirmed | Development binary runs under a dedicated ordinary account from a separate runtime directory. |
| Development PostgreSQL cluster and database | P0D | confirmed | Dedicated data directory, database, Unix socket, and peer role; no default cluster or TCP database listener. |
| Development Node ID and Node Secret | P0D | confirmed | A development Node was registered; the Secret is kept only in a restricted runtime file, never in this checklist. |
| Controller executable, config, and run user | P0D | confirmed | Controller and d2core run as the same ordinary game user. |
| d2core v0.1.1 artifact identity | P0D | confirmed | Official ZIP checksum, BUILD identity, and remote `version --json` match the frozen release. |
| d2core executable, data-dir, IPC, serve arguments | P0D | confirmed | Private development data-dir and matching local port pool checked on the running process. |
| Dota root, executable, working directory | P0D | owner-provided, observed | Host inspection confirmed paths; real n6 instances launched in P0D/P0E. |
| Test VPK copy and current addon binding | P0D | confirmed | Owner asset copy was hashed on node; prior addon content is preserved in the isolated backup directory. |
| Template file, hash, and d2core check | P0D | confirmed | Linux path-adapted copy and separate 300-second timeout variant passed fixed d2core `check`. |
| Ready rule and real Dota Ready | P0D | confirmed | One Steam-login timeout was fully reclaimed; a second real create matched all Ready markers. Human join was not tested. |
| Observed create acceptance to Ready | P0D/P0E | confirmed | Successful real samples were about 10–16 seconds; the 120-second timeout run was a separate failure case. |
| Startup timeout | P0D/P0E; production later | development 300 seconds confirmed; production not frozen | 300 seconds is only the development template ceiling; production TemplateRevision/timeout awaits later real player data. |
| Local game port range and public mapping | P0D | local pool confirmed; external reachability unverified | Both components used the same generated range; public game-port access was not tested. |
| Controller-to-Platform HTTPS | P0D | confirmed | Authenticated heartbeat and durable jobs succeeded over trusted development HTTPS. |
| create / operation / status / list / stop / reclaim | P0D | confirmed | Real create reached Ready, stop operation succeeded, status reported full reclaim, and list emptied. |
| Platform, Controller, and d2core manager restart | P0E | confirmed | All three restarted while one real Dota instance was active; original IDs and Dota PID survived. |
| Lost create response and unknown recovery | P0E | confirmed | Real core acceptance with response discarded; Controller converged through the original frozen key and operation/status. |
| Disconnected node recovery | P0E | confirmed within heartbeat timeout | Node heartbeat expired for over two minutes; unknown job stayed on its original node and reconnected to the original instance. |
| Core history retention boundary | P0E | simulated; real expiry unverified | Old preparation times suppress automatic create replay; an actual 30-day expiration was not observed. |
| Final P0E stop/reclaim and clean state | P0E | confirmed | Three real test instances were fully reclaimed; final core list empty, no dedicated Dota process, no open unknown job. |
| Windows VM real core/Controller integration | later stage | not verified | VM paths were inspected; no real P0 d2core/Controller run occurred there. |
| Human join, public game ports, A2S, Steam/steamchina URI | later stage | not verified | No human client or public game-port/URI acceptance was performed in P0. |
| P0 owner acceptance | P0 closure | confirmed | Owner formally accepted P0A–P0E; this is not V1 review, RC, Release, or production acceptance. |
| P1A development catalog and request API | P1A | confirmed in development | One owner-confirmed test ContentVersion, n6 preset, logical development template binding, migration 4, isolated database tests and HTTP recovery check. See [P1A](p1a.md). |
| P1B content fact, Allocation and real business create/stop | P1B | confirmed in development | Controller SHA256 readback, migration 5, atomic one-node reservation, real Dota Ready and full reclaim through player-owned business objects. See [P1B](p1b.md). |
| P1C actual-port JoinInfo and connect data | P1C/P1E | confirmed in development and human connect | Migration 6, actual d2core port to Controller mapping, HTTPS connect command, real Ready, human entry, and full reclaim. A2S and one-click entries remain disabled. See [P1C](p1c.md) and [P1E](p1e.md). |
| P1D formal player Web | P1D/P1E | HTTPS/API, human core flow, and owner UI acceptance confirmed | Vue player page, Session/catalog/current request recovery, state and connect help, trusted static Web root. The later independent parity checkpoint resolved the presentation regression. See [P1D](p1d.md), [P1E](p1e.md), and [UI parity](p1d-parity.md). |
| P1E real player flow and full reclaim | P1E | owner-reported complete flow; system cleanup confirmed | Owner supplied ended-page screenshot and reported normal full flow; two real requests reached JoinInfo and full reclaim, no active instance or business job remains. Revised console-help steps were not separately rechecked at owner direction. See [P1E](p1e.md). |
| P1D UI parity regression | after P1E checkpoint | browser/CI checks passed; owner UI acceptance confirmed | Restored approved P-1 player layout and normal-flow presentation without changing API or business logic. True 1440/390/320 px browser checks covered idle, waiting, allocating, creating, Ready, no JoinInfo, stopping, ended, and maintenance messages. Real HTTPS/API checks passed after deployment. See [UI parity](p1d-parity.md). |
| P1 overall stage closure | P1 | PASS; owner acceptance confirmed | P1A–P1E and the independent UI parity checkpoint are complete. This is stage acceptance, not V1 independent review, RC, Release, or production deployment. See [P1 summary](p1-summary.md). |

Credential values, keys, TLS private material, VPKs, and private logs must
never be added to this file or committed to Git.
