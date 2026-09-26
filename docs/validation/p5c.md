# P5C entry checkpoint (in progress)

## Implemented

- Controller checks A2S_INFO over local UDP for every currently Ready instance, including challenge handling. A2S remains a diagnostic and does not determine `connect` or scheduling eligibility.
- Admin entry verification requires an explicit human confirmation, note and exact full set of currently configured public ports. Verified and enabled remain separate. A network revision change clears verification, enabled state and port evidence. Migration invalidates legacy verifications that lacked full-port evidence.
- Player page displays a Steam or steamchina URI only when the backend supplies it after verified and enabled checks. It always retains `connect` help, including the client cold-start and Windows Win+R fallback.
- Unit tests exercise A2S challenge. PostgreSQL tests cover partial-port rejection, full-port acceptance and revision invalidation. GitHub Actions [run 36231578148](https://github.com/L4C99/dota2-arcade-platform/actions/runs/36231578148) passed all four jobs.
- On the drained Linux development node, the operator backed up the Controller config, set `protocol_ip=58.216.8.72` and enabled A2S diagnostics without changing the ten-port identity mapping. The Controller reported a fresh network revision and both URI entries remained unverified and disabled. An official temporary old-game instance reached full d2core Ready on 28000; the Controller reported `a2s_query_ok=false`, and direct local UDP A2S_INFO queries to loopback and the public IP timed out. The instance was stopped and fully reclaimed, the node reconciled and resumed. This is a failed diagnostic observation, not evidence that `connect` or either URI fails.
- The Admin verification action and button no longer require A2S query success. They still require explicit human confirmation, a note and every configured public port. This restores the frozen spec's separation between A2S diagnostics and real-client URI evidence. GitHub Actions [run 36244759743](https://github.com/L4C99/dota2-arcade-platform/actions/runs/36244759743) passed all jobs. Exact clean Platform/Web build `804109f` is live on the development Web host; HTTPS health and hashed Web asset returned 200. Both development nodes' verification and enabled flags remain false.

## NOT VERIFIED

- Real client entry for Steam and steamchina across every configured public port, browser cold-start and asymmetric NAT behavior. No entry has been marked verified on development nodes. P5C is not marked PASS.
- The current mapping sets are Linux 28000–28009 and Windows 28100–28109. The earlier real-client `connect` checks on Linux 28000 and Windows 28100 do not establish URI behavior on either complete set. Windows still has no independently verified public URI route.

## First owner Steam URI attempt on Linux port 28000

After the owner authorized a Steam-first URI trial, the idle Linux development node was drained and a single direct fixed-d2core instance of the verified new-game `custom` template was created explicitly on port 28000. The instance reached `active/running/ready`, including Steam login success. The owner attempted `steam://connect/58.216.8.72:28000` and reported that Steam failed to start Dota; exact client error and ordinary `connect` result were not yet supplied. This is a failed URI attempt, not a successful port verification.

Read-only diagnosis found that the Controller network config declares `a2sEnabled=true`, but UDP A2S_INFO queries to both loopback and the public IP on port 28000 timed out while the instance was Ready. The current Dota `gameinfo.gi` had no `GMS` or `Advertise` stanza; fixed d2core's A2S documentation calls for `GameInfo/GMS/Advertise 1` and notes that a new instance plus a separate query are required after changing it. The A2S gap may contribute to the URI failure, but the owner's client symptom is not yet specific enough to prove causation. No Dota installation file, firewall or network configuration was changed during this diagnosis.

At the owner's request, the exact temporary instance was stopped and confirmed `lifecycle=reclaimed`, `process=stopped`, `cleanup=complete`; d2core's active list was empty. The Controller's explicit reconcile completed generation 11/11, and the Linux node was resumed. Both URI verification/enabled flags remain false. Further port trials are paused pending owner authorization for the proposed reversible development Dota A2S configuration change and a fresh real-client check.
