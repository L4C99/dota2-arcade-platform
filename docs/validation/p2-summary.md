# P2 total exit report — owner UI confirmation pending

Date: 2026-09-25. Stage boundary: P2A → P2B → P2C → P2D only. P3 has not started.

## Checkpoints

| Point | Full SHA / status |
| --- | --- |
| P2 start / P1 closure | `93cbc4f70ab7268f0953d57f3bc9f9745a53b8fa` |
| P2A | `7c8bf955b42814a6fc9b4c2711529f8c2b38159a`, CI SUCCESS |
| P2B | `dc06e130a877c70c15f6eb2d9148938d0304c67c`, CI SUCCESS |
| P2C | `b216060fa970f04d564cecab8a8b3b2eddd12028`, CI SUCCESS |
| P2D functional + UI source | `69450e1eff23a85e3e9dd203b1c8ba4d7a7c4b74`, CI SUCCESS |
| UI parity | `4bc23c25c70837a1d9e856fad624b83856358ff4`, CI SUCCESS; invite navigation follow-up `e088c2e89b4d23b503281b7a5c1e9abed4f54fe1`, CI SUCCESS |
| Anonymous display-name supplement | `f9b373bf5835473c0fef87ded9b988cbb379e445`, CI SUCCESS |
| Origin/main / Git status | Final documentation checkpoint and status recorded at report delivery |

## Party model

| Invariant | Result |
| --- | --- |
| Persistent Party / PartyMember / PartyInvite | PASS — explicit migrations 7/8 and formal API |
| One User in at most one Party | PASS — database unique membership, User lock and concurrent test |
| Leader/member role and no leader transfer | PASS — persistent role, at-most-one leader, deferred leader-role FK; leader leave denied |
| `PlatformSettings.max_party_size` | PASS — explicit positive deployment value 10 for development; independent of preset size |
| `GamePreset.max_players` at request time | PASS — Party count checked before insert; existing instance unaffected by later joins |
| Party persistence after full reclaim | PASS — real d2core regression retained Party, leader, remaining member and invite |

## Owner and permissions

| Behavior | Result |
| --- | --- |
| Solo User owner remains functional | PASS — P1 regression suites and P2D solo owner check |
| Party owner exactly once | PASS — database exactly-one-owner check and Party FK; old owner/history unchanged |
| Leader create / duplicate POST | PASS — Session-derived role; concurrent POSTs return one blocking Party request |
| Member current request / Allocation / JoinInfo read | PASS — same IDs/data in integration and real HTTPS flow |
| Member create / stop rejected | PASS — 403 at Platform before new request/stop NodeJob |
| Invite retrieval/reset | PASS — leader only; old token invalid after reset |
| Member leave / leader remove | PASS — membership only, no stop/cancel/kick/capacity release |
| Idle disband / busy disband | PASS — formal idle API path; blocked with clear conflict during activity |

## Concurrency and security

Disposable-schema PostgreSQL tests passed for two users contending for the last Party slot, one User concurrently joining two available Parties, invite reset versus consume, same leader's concurrent duplicate submissions, and two Party leaders contending for the sole Node capacity slot. The latter produced only one Allocation and one create NodeJob. HTTP tests used independently server-issued Sessions and rejected forged `partyId`/role/User claims, foreign request/Allocation IDs, ordinary member leader actions, and former-member stale access. Frontend controls are supplemental; authorization is enforced by the backend. Details and evidence are in [P2D validation](p2d.md).

## P1 lifecycle regression and environment

The real development sequence reached Party-owned ServerRequest → Allocation → business create NodeJob → fixed d2core v0.1.1 Ready → valid JoinInfo. A new member joined the running Party and read that same JoinInfo; another left without stopping it. Leader stop produced a separate stop NodeJob. d2core status confirmed `lifecycle=reclaimed`, `process=stopped`, `cleanup=complete`; Allocation became reclaimed. P1 solo integration and all previous Go tests remain PASS. P1 human connect was not repeated because P2 does not require another Dota client acceptance.

Development PostgreSQL is at migration 9, `max_party_size=10`. The exact clean display-name Platform build and Web assets are served over the existing HTTPS entry; Controller/d2core remain at accepted P1/fixed-release versions. All 26 Users have a persistent display name, including 22 backfilled before the rollout. Final read-only counts: seven reclaimed Allocations, no active/open unknown business job or failed-unreclaimed attempt; the historical P0 `failed_with_effect` is explained and reclaimed. d2core list is empty, no dedicated process remains, and effective node capacity 1 has occupancy 0. Three active development Parties currently exist: the lifecycle regression Party has two members and one ended request; a later UI test Party of unidentified origin has two members and no request; the first failed automated browser run left one idle Party with two members and no request after its ephemeral Sessions were destroyed. All retain active invites. The latest A/B/C browser test Party and the post-migration role smoke Party were formally disbanded. No database wipe or direct Party deletion was used. Private deployment facts remain in ignored `.local/p0-host-inventory.md`.

## UI and remaining verification

The formal Party UI implements create/join/leave/remove/disband, role/member display, invite copy/reset, current Party server status, leader application/stop and member read-only JoinInfo. The [display-name supplement](p2-display-name.md) now adds stable Chinese member names without changing owner or permission rules. A focused comparison with the accepted P-1 Party prototype found matching card hierarchy, spacing approach, button hierarchy and mobile grid behavior; existing P1 server panels remain in place. Frontend lint, typecheck, tests and build pass; HTTPS page/JS/CSS return 200. Three independent Edge contexts completed the browser A/B/C flow, and masked desktop/mobile screenshots were inspected without visible layout damage. A [narrow-screen map-card follow-up](p2-mobile-map-card.md) fixed the owner's reported long-name overlap without changing business logic. Project-owner visual confirmation is still **NOT VERIFIED**; P2 overall PASS must not be declared until that confirmation.

P3/P4/P5 capabilities, production deployment, Windows real node, multi-node scheduling, A2S/URI verified workflows and new human Dota connect are **NOT VERIFIED** and outside this stage. No tag, Release, RC or independent whole-project review was started.
