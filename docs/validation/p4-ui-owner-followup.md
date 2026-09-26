# P4 owner UI review follow-up

The project owner reported no issue in manual functional flows, including the player next-game flow, but withheld final P4 UI acceptance pending visual changes. This follow-up is part of P4 UI parity, not P4 closure. Performance discussion is deferred at the owner's request.

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

- Pending deployment and recheck of this follow-up Web bundle on the authorized development site. Earlier P4E real lifecycle verification remains recorded in `p4e.md`; the local browser fixture is not real Dota evidence.

## NOT VERIFIED

- Final player and administrator UI owner acceptance remains pending. P4 overall and owner acceptance remain pending until the owner accepts the revised UI and the separate closure commit passes CI.
