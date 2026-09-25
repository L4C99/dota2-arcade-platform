# P1D validation — formal player Web

Date: 2026-09-25. Scope: P1D. Human Dota join and final player acceptance remain P1E work.

## Implemented

- Added a Vue 3, TypeScript, and Vite player page under `web/`. It follows the accepted warm cream, dark green, sage, and restrained orange design direction while using only the real Platform catalog, Session, ServerRequest, Allocation, JoinInfo, and stop APIs.
- The page creates or restores the P0 anonymous server Session, checks the current blocking request on load and polling, and remembers only the last request ID locally so a completed request can still be shown after refresh. The backend remains the owner authority. A lost POST response triggers current-request recovery.
- P1 states cover no request, waiting, allocating, creating, Ready with valid JoinInfo, Ready without JoinInfo, stopping, ended, and a safe exceptional state. Maintenance messages disable new submission. The current single preset displays its real ten-player limit; TemplateRevision and local template paths are absent.
- Ready shows the Controller-derived `connect` command with a copy action and manual Dota console help. The help uses the owner-supplied `-console` startup option from the accepted prototype; exact current-client steps still require P1E human verification. Steam/steamchina entry controls remain absent while neither URI is verified and enabled.
- The existing Platform Server can serve a trusted configured absolute Web directory behind the development HTTPS proxy. No new proxy, shell, filesystem browser, or direct d2core path was added.

## Checks

- `go test ./... -count=1`, `go vet ./...`: PASS, including a static-root route check.
- `npm run typecheck`, `npm test`, `npm run build`: PASS. The frontend has no lint script configured.
- Authorized development host candidate: HTTPS homepage, hashed JavaScript and CSS all returned 200. The same-origin API created an anonymous Session and returned exactly one ArcadeGame, one GamePreset with `maxPlayers=10`, and no current request. Existing P1C business create/stop integration had already exercised all live API states through full reclaim.
- Browser automation was unavailable: the in-app browser timed out and automatic review declined access to the user's active Edge window. Visual rendering and the final console-help wording are therefore **NOT VERIFIED** until P1E human use.

No real server was started solely for this page check. No VPK, firewall, cloud security group, NAT, or production configuration was changed. GitHub CI and the checkpoint SHA are tracked with the pushed P1D commit.
