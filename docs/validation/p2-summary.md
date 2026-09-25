# P2 final validation and owner acceptance

Date: 2026-09-25. This is the P2A → P2D stage closure. The project owner confirmed the final Party UI, anonymous display names, map-card layout and related copy/layout follow-ups against the exact `origin/main` baseline `25228ce7c2660724659fb5f77b4597e7e7c25676`.

| Acceptance | Result |
| --- | --- |
| P2A | **PASS** |
| P2B | **PASS** |
| P2C | **PASS** |
| P2D | **PASS** |
| P2 UI owner acceptance | **CONFIRMED** |
| P2 overall | **PASS** |
| Owner acceptance | **CONFIRMED** |

This is P2 stage acceptance. It is not V1 independent whole-project code review, RC, Release, production deployment or P3 acceptance. No P3 work has started.

## Git checkpoints

The following full SHAs were read from the history between the P1 closure and the owner-confirmed P2 UI baseline. UI and documentation follow-ups are part of P2 closure, not new product stages.

| Checkpoint | Full SHA |
| --- | --- |
| P2 start / P1 closure | `93cbc4f70ab7268f0953d57f3bc9f9745a53b8fa` |
| P2A | `7c8bf955b42814a6fc9b4c2711529f8c2b38159a` |
| P2B | `dc06e130a877c70c15f6eb2d9148938d0304c67c` |
| P2C | `b216060fa970f04d564cecab8a8b3b2eddd12028` |
| P2D functional and initial UI | `69450e1eff23a85e3e9dd203b1c8ba4d7a7c4b74` |
| Party UI parity and secure Invite fragment | `4bc23c25c70837a1d9e856fad624b83856358ff4` |
| Invite fragment navigation follow-up | `e088c2e89b4d23b503281b7a5c1e9abed4f54fe1` |
| Initial P2 exit evidence, before owner UI acceptance | `e7249f2c8a8f108df99581fe954407664e28226b` |
| Anonymous display-name implementation | `f9b373bf5835473c0fef87ded9b988cbb379e445` |
| Display-name validation and documentation | `c2e6c41c962f5225ddc2dde24c6c73d69be63e17` |
| Narrow-screen long map-name fix | `413f0c529996d2f236837fef3c0fea208bc7ba21` |
| Narrow-screen map-card validation | `3d9a1748eee983ed17789b96d8e7ca1c1d4b9164` |
| Server-card owner label, connection action and membership copy | `7a717b749cb8e1d357f2d1301af4cbb70ceaaa27` |
| Server-card validation | `122158b877afd6f283913e51e5e4899d6071f25e` |
| Map-card badge order at all widths | `c2b9180245d2e51e8ed584bfea554f42be7349e4` |
| Map-card badge validation / owner-confirmed final UI baseline | `25228ce7c2660724659fb5f77b4597e7e7c25676` |

All implementation checkpoint CI runs passed. The final UI baseline's GitHub CI run `36152937491` passed Ubuntu Go, Windows Go, PostgreSQL integration and Web. The separate docs-only P2 closure commit and its CI are identified in the final handoff after that commit exists.

## Party model and membership

| Invariant | Result |
| --- | --- |
| Persistent `Party`, `PartyMember`, `PartyInvite` | **PASS** — explicit migrations and formal APIs; no JSON substitute |
| One User in at most one active Party | **PASS** — database uniqueness, transactional locking and concurrent tests |
| Leader/member roles | **PASS** — persisted role and backend authorization from the current Session |
| No leader transfer | **PASS** — leader leave rejected; no leaderless active Party |
| Party persistence | **PASS** — Party, leader, remaining members and Invite survive stop and full reclaim |
| `PlatformSettings.max_party_size` | **PASS** — Party membership hard limit, including last-slot concurrency |
| `GamePreset.max_players` | **PASS** — request-time Party size check before ServerRequest creation; later joins do not stop an existing instance |

The current development/deployment value of `PlatformSettings.max_party_size` is **10**, explicitly chosen by the owner. It is not a permanent hardcoded value and may be set to another positive integer by a later deployment. It is independent of the n6 GamePreset's `max_players=10` and node instance capacity.

## Owner, permissions and active membership

When a User is outside a Party, its normal `ServerRequest` is User-owned and the P1 solo path remains valid. When a User belongs to a Party, the normal request is Party-owned; only the leader may create or stop it. The database enforces exactly one owner. Party members share one current ServerRequest, Allocation and valid JoinInfo; duplicate leader submissions resolve to one blocking request. A member cannot create a fallback solo request.

An ordinary member may view the Party, members, current request/Allocation and JoinInfo, connect, and leave. The Platform backend rejects ordinary member create, stop, Invite reset, member removal and disband. The leader may retrieve/reset Invite, remove an ordinary member and disband while idle. Leader leave/disband is blocked during a blocking request or unreclaimed Allocation. Browser-hidden controls are not the security boundary: authorization follows `Session → User.id → current Party membership/role`, and foreign IDs or client-provided role/owner claims grant nothing.

During a Party-owned active server, ordinary member leave and leader removal change only membership. They do not stop or cancel the instance/request, release Allocation capacity, rewrite the historical Party owner, create another Allocation/NodeJob, or kick a Dota player. While the Party has room under `max_party_size`, a new User may consume its Invite during activity and immediately see the same request and JoinInfo. The Party remains after leader stop and full reclaim. [P2C validation](p2c.md) records the integration sequence; [P2D validation](p2d.md) records the real fixed-d2core regression.

## Invite, concurrency and authorization

**PASS**: high-entropy 32-byte Invite credentials, leader-only retrieval/reset, old token rejection after reset, Party-full rejection and already-in-Party rejection. PostgreSQL integration covers concurrent Invite consumption, Invite reset/consume race, two Users contending for the last Party slot and one User concurrently joining two Parties. Same-leader duplicate POSTs produce one blocking Party request, one Allocation and one create NodeJob. Two Party leaders contending for the sole node slot cannot oversell capacity. Public validation does not record a real Invite token.

**PASS**: backend rejection of forged `partyId`, role and User claims; foreign Party request/Allocation lookup or stop; guessed request/Allocation IDs; ordinary member create/stop/reset/remove/disband; and former-member stale access. These checks use server-issued Sessions rather than edited Cookie identity. See the [P2D matrix](p2d.md).

## Anonymous display name

`User.display_name` is a persistent presentation field. New Users receive a Chinese modifier + animal/noun combination from 48 × 48 words (2,304 combinations), without a numeric suffix. Duplicate names are allowed and there is no global unique business constraint. Migration 9 backfilled existing Users. Tests and a real Platform restart showed the name remains stable with Session recovery. It does not participate in authentication, Session identity, owner selection, Party membership, leader permission, authorization or primary keys. The authorization chain remains `Session → User.id → Party membership/role`. See the [display-name validation](p2-display-name.md).

## P1 lifecycle and UI acceptance

P1 solo behavior remains covered by regression tests. The real Party-owned development flow reached ServerRequest → Allocation → create NodeJob → fixed d2core v0.1.1 Ready → valid JoinInfo. A member joined during activity and saw the same request/JoinInfo; another left without stopping the instance. Leader stop created a separate stop NodeJob and d2core confirmed `lifecycle=reclaimed`, `process=stopped`, `cleanup=complete`. Allocation became terminal and capacity was released. P1's human Dota connect acceptance was not repeated for P2.

The owner has now **confirmed** the Party UI, anonymous display names, leader/member presentation, Invite and Party operations, final map-card layout, desktop/mobile basic behavior, and the necessary copy/layout corrections. Focused P-1 Party parity and independent Edge contexts covered the A/B/C flow with formal Sessions and APIs; later layout probes used mocked API responses against the exact served Web bundle. Earlier P2 validation files that say owner confirmation was pending are point-in-time checkpoint records; this final report supersedes that interim status. No further decorative polish or broad UI refactor is part of P2 closure. Later stages may adjust UI for their own features; a separate final consistency pass may occur before a future Release.

## Final read-only development environment

At the closure check on 2026-09-25 around 23:22 China time: development PostgreSQL migration **9**; 51 Users and zero missing display names; three retained active test Parties with five current members; ServerRequests `cancelled=1`, `ended=13`; Allocations `reclaimed=13`, none active or failed-unreclaimed; NodeJobs `succeeded=35` plus one explained historical P0 `failed_with_effect`, with no open unknown. The sole node reported desired/hard capacity **1/1**, occupancy **0**. Fixed d2core v0.1.1 `list` returned no instances; no Dota dedicated process was observed. Platform, Controller and d2core services were running. The Platform binary remains the clean display-name build `f9b373bf5835473c0fef87ded9b988cbb379e445`; the served Web UI is the final owner-confirmed `c2b9180245d2e51e8ed584bfea554f42be7349e4` build. No new instance was created for closure.

Retained User, Party, membership, ended request and reclaimed Allocation history is explained test state; no database wipe, direct Party deletion or d2core data-dir reset was performed. Non-sensitive deployment details are in ignored `.local/p0-host-inventory.md`.

## Remaining outside P2 — NOT VERIFIED

Multi-node scheduling, a real Windows Node, the P3 network/NAT matrix, P4 next-game/quarantined/Admin flows, P5 content rolling, A2S/Steam/steamchina verified workflows, production deployment and production Release remain **NOT VERIFIED**. No tag, RC, Release, independent whole-project review or production action was started. Work stops at P2 closure pending separate owner authorization for P3.
