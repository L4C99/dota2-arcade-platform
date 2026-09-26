# P4 owner UI review follow-up

The project owner reported no issue in manual functional flows, including the player next-game flow, but withheld final P4 UI acceptance pending visual changes. This follow-up is part of P4 UI parity, not P4 closure. Performance discussion is deferred at the owner's request. UI checkpoint `dbc466864b24fc0dc4647bfcd6ac1fee4faa0b04` passed [GitHub Actions run 36225225011](https://github.com/L4C99/dota2-arcade-platform/actions/runs/36225225011).

## IMPLEMENTED

- The running player's next-game action now sits beside end-server as a compact sage secondary action instead of a full-width dark primary bar. The stop and next-game API behavior and confirmation flow are unchanged.
- The administrator content page uses the frozen Chinese business terms “游廊游戏” and “玩法预设”, shows readable version/template labels, and groups status, maintenance message, and actions in compact rows.
- The node page presents short selectable node summaries and only one node's settings at a time. Online/heartbeat/compatibility and scheduling status are translated into Chinese. Controller and d2core versions, reconcile generation, and the reconcile action remain available under a technical-details disclosure. Controller-reported content facts remain read-only; platform allocation controls remain actionable.
- The requests/instances page separates active and historical requests, with one selected request's allocation attempts and actions shown in a detail panel. Pending Jobs and recent Parties are compact supporting panels. The audit page uses a selectable record list and a separate who/when/target/result/change detail panel instead of inline JSON. Important audit fields and lifecycle states have Chinese labels; IDs and diagnostic codes remain available for operations.
- Removed the global 320 px minimum body width, which caused page-level horizontal scrolling when the vertical scrollbar consumed viewport width.

## AUTOMATED VERIFIED

- `go test ./...` and `go vet ./...` passed with a workspace-local Go build cache. Web lint, typecheck, five Vitest cases and production build passed. No backend code, API, schema or migration changed.
- A disposable local browser fixture exercised admin node switching, request history filtering, and all five admin sections at 390 px and 320 px. Both widths had no page-level horizontal overflow. The player running/Ready state at 320 px showed balanced end-server and next-game actions with no horizontal overflow. The final bundle loaded without browser console errors.

## REAL-ENVIRONMENT VERIFIED

- The exact production Web bundle from the UI checkpoint was uploaded to the authorized development Web host after SHA-256 archive verification (`8f13f890d081a287e323d6ab92b656787f67fd681f5bba75bfbf54277698fc41`). The previous Web directory and launcher backup were preserved. The P4E Platform binary and business database were unchanged. The short development Platform restart returned healthy.
- With normal HTTPS verification, `/healthz`, `/`, `/admin`, and both new hashed JS/CSS assets returned HTTP 200. Read-only preflight and post-switch database checks showed zero active requests, occupied Allocations, open Jobs, or pending/paused next-game intents; both development Nodes had fresh heartbeats, Drain false and occupied capacity zero.
- Earlier P4E real lifecycle verification remains recorded in `p4e.md`; the local browser fixture is not real Dota evidence.

## NOT VERIFIED

- Authenticated browser interaction against the newly deployed administrator UI was not rechecked in this follow-up. The local fixture checked all administrator sections and the HTTPS resource check confirmed the deployed bundle. Final player and administrator UI owner acceptance remains pending. P4 overall and owner acceptance remain pending until the owner accepts the revised UI and the separate closure commit passes CI.

## Second owner feedback: capacity and party history

The owner reported that the online badge and capacity text in the overview were too close, that the node capacity wording needed refinement, and that a long list of dissolved Parties made the page unwieldy. The owner also asked about automatic administrator refresh and multi-map scope.

- The overview gives connectivity and capacity separate visual groups. Capacity uses a large occupied/desired value and a second line labeling both figures.
- Node details show occupied, desired capacity, and Controller-reported hard limit as separate figures with a short explanation. This changes presentation only; the hard limit remains a node fact.
- The Party panel shows active Parties first. Additional active Parties and dissolved history use collapsible sections with a bounded 290 px internal scroll area. The existing administrator overview query returns at most 100 recent Parties; the UI states this limit.
- The current full overview refresh replaces draft settings. Automatic polling of that endpoint could overwrite unsaved edits and repeatedly transfer requests, allocations, Jobs, Parties, and Audit data. Keep manual refresh for this checkpoint. A future performance discussion can define a small status-only poll, visible-tab scheduling, and preservation of form drafts before enabling automatic refresh.
- P4 displays the Node × ArcadeGame control, but the frozen `docs/specs/v1.md` assigns multiple ArcadeGame/GamePreset/ContentVersion setup to P5A. No extra map or content was created for this UI follow-up.

Automated verification: Web lint, typecheck, five Vitest cases, and production build passed. Local browser fixture checked desktop presentation, 390 px and 320 px layouts without horizontal overflow, and a 12-record dissolved Party section whose 919 px contents scroll within a 289 px viewport. The backend/API/schema did not change. Final owner acceptance remains pending.

The UI checkpoint is `ba19df025800a03e3ccc01080ce805178add8281`; [GitHub Actions run 36226367596](https://github.com/L4C99/dota2-arcade-platform/actions/runs/36226367596) passed Web, Ubuntu Go, Windows Go, and PostgreSQL integration. The matching Web bundle was verified by SHA-256 (`52ba89d081946e05aabc95aa5e0ceb2ba6a3fabe41cd805a8317e4e0b2bcf47c`) and switched on the authorized development Web host, preserving the previous Web directory and launcher backup. `/healthz`, `/`, `/admin`, and the new hashed JS/CSS assets returned HTTP 200 over HTTPS. Read-only checks before and after the switch found no active Requests, occupied Allocations, open Jobs, or pending/paused next-game intents. Both development Nodes had fresh heartbeats, Drain false, desired capacity 1, and occupied capacity 0. The P4E backend, database schema and Controllers were unchanged.
