# Player HTTP API (P1/P2/P3B)

All routes are same-origin `/api/v1` JSON routes. A server-issued anonymous Session cookie identifies the current User. State-changing calls require the configured `Origin`. Request bodies reject unknown fields, including `userId`, `partyId`, `role`, and `leaderId`; no client-supplied identity or role authorizes an action.

| Method | Route | Session-derived behavior |
| --- | --- | --- |
| POST | `/session` | Create or restore an anonymous User Session; return `userId` and stable `displayName` |
| GET | `/me` | Return current User ID and `displayName` |
| GET | `/catalog` | Current ArcadeGame/GamePreset catalog and maintenance facts |
| GET | `/party` | Current Party, members with `displayName`, persisted roles and `maxSize`, or `null` |
| POST | `/party` | Create a persistent Party; caller becomes its leader and member |
| GET | `/party/invite` | Leader only; retrieve current high-entropy invite credential |
| POST | `/party/invite/reset` | Leader only; revoke old credential and return replacement |
| POST | `/party/join` | Consume `{ "token": "..." }`; accepts active Party while below `max_party_size` |
| POST | `/party/leave` | Ordinary member only; does not stop a server |
| POST | `/party/members/{userId}/remove` | Leader only; cannot remove the leader; does not kick a Dota player |
| POST | `/party/disband` | Leader only; requires no blocking request or unreclaimed Allocation |
| GET | `/nodes?arcadeGameId={id}&gamePresetId={id}` | Player-safe Node names, availability reasons, and advisory free slots for the selected content |
| POST | `/server-requests` | Solo User or current Party leader; `arcadeGameId`, `gamePresetId`, optional `nodeSelectionMode` (`auto` by default or `manual`) and `manualNodeId` (required for manual) |
| GET | `/server-requests/current` | Current User-owned or Party-owned blocking request, or `null` |
| GET | `/server-requests/{id}` | Request visible to its current business owner only |
| GET | `/server-requests/{id}/allocation` | Latest Allocation/JoinInfo for an authorized request |
| POST | `/server-requests/{id}/stop` | Solo owner or current Party leader; existing durable stop/reclaim path |
| POST | `/server-requests/{id}/abandon` | Solo owner or current Party leader; requires `{ "confirm": true }` and a quarantined Allocation; removes only the old request's business blocking |
| POST | `/server-requests/{id}/cancel` | Solo owner or current Party leader; succeeds only while waiting with no Allocation attempt |

Party owner is held on `ServerRequest.owner_party_id` and never rewritten after members change. Ordinary members can read the same Party request and valid JoinInfo. A former member loses that access. Duplicate leader POSTs return the existing blocking Party request. A Party larger than the selected GamePreset's `max_players` receives `409 party_exceeds_preset` before any ServerRequest, Allocation or NodeJob is created. `PlatformSettings.max_party_size` is the separate membership limit and must be explicitly configured as a positive integer through `PLATFORM_MAX_PARTY_SIZE` before serving P2 traffic.

`users.display_name` is a persistent, presentation-only anonymous name. Migration 9 backfills existing Users without changing their IDs, Sessions or Party membership. New Users receive a name in the same transaction as their Session; `/me` and `/session` return it and Party member rows expose it for display. Duplicate names are allowed. Session/User ID, never the name, determines ownership and authorization. There is no edit, search or profile API for names.

Invite credentials contain 32 random bytes. The formal Web shares them in a URL fragment (`#invite=...`) so page requests and Referer headers do not carry the credential. The leader-only invite response is intentionally sensitive: do not log, publish, or put it in validation documents. Reset serializes with consume; after reset commits, old credentials are invalid. Party discovery, public Party listing, matchmaking, leader transfer, and direct Web-to-d2core calls are outside this API.

P3B persists the selection on each ServerRequest. `auto` considers all eligible nodes and chooses highest priority, breaking ties by Node ID. `manual` waits only for the chosen Node, including when it is full, draining, unreachable, or its content is unready. `/nodes` is a presentation hint; its free slot count may change before submission. P3C includes `connectivity: online|stale|offline`, derived from server-recorded heartbeat age, in each Node choice. The scheduler checks the same Node eligibility facts again inside the reservation transaction. A duplicate submission returns the existing blocking request and its original selection. A manual selection can be changed only by cancelling a safe waiting request and submitting a new one with a new `requestedAt`; cancellation cannot release a reserved or potentially created server. Node secrets, paths, and network configuration are never returned by `/nodes`.

P4B exposes `quarantined` as an occupied Allocation with uncertain cleanup. `/abandon` is a separate, explicitly confirmed owner action. It makes the old ServerRequest `abandoned` and permits a new business request; it does not stop Dota, free the old port or node capacity, change the old Allocation's node/content/history, or turn the old Allocation into `reclaimed`. Repeated `/abandon` is idempotent. Only a later d2core-confirmed full reclaim releases that capacity. Ordinary Party members and unrelated players cannot perform the action.
