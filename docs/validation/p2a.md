# P2A validation — persistent Party, membership and invite

Date: 2026-09-25. Start SHA: `93cbc4f70ab7268f0953d57f3bc9f9745a53b8fa`.

## Implemented

- Migration 7 adds `parties`, `party_members`, `party_invites`, a real Party owner foreign key, and nullable `PlatformSettings.max_party_size` with a positive-value check. The migration assigns no inferred value. The Platform `serve` command requires the deployment's explicit positive `PLATFORM_MAX_PARTY_SIZE` and writes that choice to the setting; example configurations carry a placeholder.
- A database unique constraint on active `party_members.user_id` and a deferred Party-to-leader-member foreign key prevent multiple memberships and a live Party without its leader. Party creation inserts the Party, leader membership, and first invite atomically.
- Invites use 32 random bytes in canonical URL-safe encoding. The leader-only current/reset APIs never log tokens; reset revokes the previous credential in the same Party-locked transaction used by consume. The leader can retrieve the active link; ordinary members cannot. Party join serializes on the User and Party rows to protect both single membership and the last available slot.
- Session-derived Party create/read/invite/join/leave/remove/disband API routes are in place. Leader leave is rejected because V1 has no transfer. Disband is permitted only after blocking request and unreclaimed Allocation checks, and retains the Party row for request history.

## Checks

- Local `go test ./... -count=1` and `go vet ./...`: PASS using a workspace-local Go cache. Local Web lint/typecheck/test/build: PASS; Vitest/build required a sandbox exception for esbuild process spawning.
- Cross-compiled store integration tests ran as `arcadedev` against disposable schemas in the authorized isolated PostgreSQL development database. Migration upgrade, P1 regressions, P2A model/invite, last-slot concurrent join and multi-Party membership: PASS.
- The project owner explicitly supplied development `max_party_size=10`. It has **not yet been written to the persistent development PlatformSettings** at this checkpoint; migration and development deployment remain pending.

P2A checkpoint `7c8bf955b42814a6fc9b4c2711529f8c2b38159a` was pushed to `origin/main`; GitHub CI run `36130778400` completed SUCCESS. Real Party-owned request, UI, and active-instance validation belong to P2B–P2D.
