# P0 development environment checklist

This public checklist records confirmation status only. Actual host addresses,
private paths, credentials, and runtime state belong in the Git-ignored local
inventory. Read this checklist and the local inventory before P0E or a new P0
session. Recheck the host before a real operation; an unverified entry is not
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
| Dota root, executable, working directory | P0D | owner-provided, observed | Read-only host inspection confirmed paths; no game launched yet. |
| Test VPK copy and current addon binding | P0D | confirmed | Owner asset copy was hashed on node; prior addon content is preserved in the isolated backup directory. |
| Template file, hash, and d2core check | P0D | confirmed | Linux path-adapted copy and separate 300-second timeout variant passed fixed d2core `check`. |
| Ready rule and real Dota Ready | P0D | confirmed | One Steam-login timeout was fully reclaimed; a second real create matched all Ready markers. Human join was not tested. |
| Local game port range and public mapping | P0D | local pool confirmed; external reachability unverified | Both components used the same generated range; public game-port access was not tested. |
| Controller-to-Platform HTTPS | P0D | confirmed | Authenticated heartbeat and durable jobs succeeded over trusted development HTTPS. |
| create / operation / status / list / stop / reclaim | P0D | confirmed | Real create reached Ready, stop operation succeeded, status reported full reclaim, and list emptied. |
| Restart and unknown-create reconciliation | P0E | not started | Start only after P0D acceptance and execution audit. |

Credential values, keys, TLS private material, VPKs, and private logs must
never be added to this file or committed to Git.
