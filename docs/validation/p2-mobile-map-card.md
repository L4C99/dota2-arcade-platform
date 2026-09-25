# P2 UI follow-up: long ArcadeGame name on narrow screens

Date: 2026-09-25. The project owner reported that a mobile map card's long name collided with its selection badge and Workshop line. The card used an absolutely positioned badge at widths above 410px while the name had no reserved area.

For screens up to 900px, the badge now occupies normal flex layout space. At mobile widths, it sits after the map name and Workshop ID; at tablet widths, it stays in its own flex slot alongside the text. Long names wrap inside the card. Desktop styling and the accepted P1 visual direction remain intact. No API, Party, request, stop, NodeJob, d2core or game content logic changed.

Frontend lint, typecheck, tests and production build passed, as did full Go tests and vet. An intentionally long Chinese map name was rendered in independent Edge viewports at 320, 390, 510, 700, 768 and 1440px. Bounding-box checks found no badge/name collision or horizontal page overflow, and mobile/tablet screenshots were visually inspected. The same six checks passed against the Web bundle actually served by the development site. HTTPS homepage, exact JS/CSS and health returned 200. Checkpoint `413f0c529996d2f236837fef3c0fea208bc7ba21` was pushed to `origin/main`; GitHub CI run `36149530042` completed SUCCESS.

At the post-deployment read-only check, a separate development server request and d2core instance were running, with capacity occupied 1/1. This UI-only deployment did not stop or alter the active instance. The instance was left running for its current user. Owner final UI acceptance remains **NOT VERIFIED** until the owner refreshes and confirms the map-card layout.
