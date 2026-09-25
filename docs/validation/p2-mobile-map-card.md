# P2 UI follow-up: long ArcadeGame name on narrow screens

This is a point-in-time UI checkpoint. The later [P2 final validation](p2-summary.md) records the project owner's confirmed UI acceptance and overall P2 PASS.

Date: 2026-09-25. The project owner reported that a mobile map card's long name collided with its selection badge and Workshop line. The card used an absolutely positioned badge at widths above 410px while the name had no reserved area.

For screens up to 900px, the badge now occupies normal flex layout space. At mobile widths, it sits after the map name and Workshop ID; at tablet widths, it stays in its own flex slot alongside the text. Long names wrap inside the card. Desktop styling and the accepted P1 visual direction remain intact. No API, Party, request, stop, NodeJob, d2core or game content logic changed.

Frontend lint, typecheck, tests and production build passed, as did full Go tests and vet. An intentionally long Chinese map name was rendered in independent Edge viewports at 320, 390, 510, 700, 768 and 1440px. Bounding-box checks found no badge/name collision or horizontal page overflow, and mobile/tablet screenshots were visually inspected. The same six checks passed against the Web bundle actually served by the development site. HTTPS homepage, exact JS/CSS and health returned 200. Checkpoint `413f0c529996d2f236837fef3c0fea208bc7ba21` was pushed to `origin/main`; GitHub CI run `36149530042` completed SUCCESS.

At the post-deployment read-only check, a separate development server request and d2core instance were running, with capacity occupied 1/1. This UI-only deployment did not stop or alter the active instance. The instance was left running for its current user. Owner final UI acceptance remains **NOT VERIFIED** until the owner refreshes and confirms the map-card layout.

## Desktop status placement follow-up

The owner preferred the mobile card's reading order on desktop too. The status pill now follows the map name and Workshop ID inside the same text column, aligned left at every width. This applies equally to `已选择`, `可申请`, and `维护中`. The card keeps the previously accepted art, typography and colors.

Web lint, typecheck, tests and build, full Go tests and vet passed. Edge rendered an intentionally long map name at 320, 390, 510, 700, 768 and 1440px: the status stayed below the Workshop line, aligned left, without horizontal overflow. Desktop, tablet and mobile screenshots were inspected. The same six checks passed against the exact bundle served by the development site; homepage, JS/CSS and health returned 200. Source checkpoint `c2b9180245d2e51e8ed584bfea554f42be7349e4` was pushed to `origin/main`; GitHub CI run `36152535361` completed SUCCESS.

The Web-only deployment did not alter the Platform binary, Controller or game lifecycle. Final read-only state showed all 13 historical Allocations reclaimed, node occupancy 0/1, and no d2core instances. Project-owner final P2 UI acceptance remains **NOT VERIFIED** until this placement is confirmed.
