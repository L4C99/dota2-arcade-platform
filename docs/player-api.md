# Player HTTP API (P1/P2)

All routes are same-origin `/api/v1` JSON routes. A server-issued anonymous Session cookie identifies the current User. State-changing calls require the configured `Origin`. Request bodies reject unknown fields, including `userId`, `partyId`, `role`, and `leaderId`; no client-supplied identity or role authorizes an action.

| Method | Route | Session-derived behavior |
| --- | --- | --- |
| POST | `/session` | Create or restore an anonymous User Session |
| GET | `/me` | Return current User ID |
| GET | `/catalog` | Current ArcadeGame/GamePreset catalog and maintenance facts |
| GET | `/party` | Current Party, members, persisted roles and `maxSize`, or `null` |
| POST | `/party` | Create a persistent Party; caller becomes its leader and member |
| GET | `/party/invite` | Leader only; retrieve current high-entropy invite credential |
| POST | `/party/invite/reset` | Leader only; revoke old credential and return replacement |
| POST | `/party/join` | Consume `{ "token": "..." }`; accepts active Party while below `max_party_size` |
| POST | `/party/leave` | Ordinary member only; does not stop a server |
| POST | `/party/members/{userId}/remove` | Leader only; cannot remove the leader; does not kick a Dota player |
| POST | `/party/disband` | Leader only; requires no blocking request or unreclaimed Allocation |
| POST | `/server-requests` | Solo User or current Party leader; body contains only `arcadeGameId` and `gamePresetId` |
| GET | `/server-requests/current` | Current User-owned or Party-owned blocking request, or `null` |
| GET | `/server-requests/{id}` | Request visible to its current business owner only |
| GET | `/server-requests/{id}/allocation` | Latest Allocation/JoinInfo for an authorized request |
| POST | `/server-requests/{id}/stop` | Solo owner or current Party leader; existing durable stop/reclaim path |

Party owner is held on `ServerRequest.owner_party_id` and never rewritten after members change. Ordinary members can read the same Party request and valid JoinInfo. A former member loses that access. Duplicate leader POSTs return the existing blocking Party request. A Party larger than the selected GamePreset's `max_players` receives `409 party_exceeds_preset` before any ServerRequest, Allocation or NodeJob is created. `PlatformSettings.max_party_size` is the separate membership limit and must be explicitly configured as a positive integer through `PLATFORM_MAX_PARTY_SIZE` before serving P2 traffic.

Invite credentials contain 32 random bytes. The leader-only invite response is intentionally sensitive: do not log, publish, or put it in validation documents. Reset serializes with consume; after reset commits, old credentials are invalid. Party discovery, public Party listing, matchmaking, leader transfer, direct Web-to-d2core calls, and P3/P4 controls are outside this API stage.
