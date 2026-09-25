# P2 UI follow-up: server card and membership notices

Date: 2026-09-25. The project owner reported two UI issues while checking P2: leaving a Party claimed the current server would continue even when none was running, and the Party view labelled a solo request as a Party server while displaying the connection command alongside a button to view connection instructions.

The leave and remove confirmations/notices now say **if a server is already running, it continues**. The Party view's server card labels an active request according to the current business owner: solo as `单人服务器`, Party as `队伍服务器`. It omits the duplicate command preview and points to the full server page for connection instructions and copying. Ended/cancelled requests are labelled `本局已结束`. No API, request ownership, Allocation, NodeJob, d2core, content or network behavior changed.

Verification:

- Web lint, typecheck, tests and production build passed. Full Go tests and vet passed.
- Independent Edge browser contexts passed solo Ready, Party Ready, member leave and leader remove cases, including the single connection action and conditional notice. The same scenarios passed against the bundle actually served by the development site, with API responses mocked in the browser; they did not create or stop a game instance.
- Development HTTPS homepage, exact hashed JS/CSS assets and health returned 200 after Web-only deployment.
- Source checkpoint `7a717b749cb8e1d357f2d1301af4cbb70ceaaa27` was pushed to `origin/main`; GitHub CI run `36151023239` completed SUCCESS.

At the final read-only environment check, migration 9 was current, with three active Parties and five members. All 12 historical Allocations were reclaimed; node capacity was 1/1 with occupancy 0. Fixed d2core v0.1.1 listed no active instances. The previously running development instance had ended through its existing lifecycle before this check; this Web-only change did not stop it. The clean display-name Platform binary remains in service. Project-owner final P2 UI acceptance remains **NOT VERIFIED**. P3 has not started.
