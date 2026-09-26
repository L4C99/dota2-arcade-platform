# P4 player and administrator UI parity

Implementation baseline: P4E checkpoint `ea034bd823bf3d3920809c414087ee2966c2b8aa`; GitHub Actions run `36192301200` passed Ubuntu Go, Windows Go, PostgreSQL integration, and Web. A clean Linux Platform build embeds that full revision with `vcs.modified=false`; the matching production Web bundle is served by the authorized development site. The previous launcher and bundles remain available for rollback. This is a development deployment, not production.

## IMPLEMENTED

- Player presentation covers ordinary running and stop, unknown Node status, quarantine with retained-resource explanation, confirmed abandon, next-game pending/paused/reclaimed decision, maintenance and announcement. The text does not claim quarantine stops Dota or releases a port.
- Administrator presentation covers login, overview/maintenance/announcement, content and presets, Node and binding controls, instances/Jobs/Party, quarantine/reconcile/stop, capacity/priority/Drain, entry verified/enabled state, and Audit. An abandoned Request with a quarantined old Allocation still offers an audited stop action. The Web displays only structured error codes and operational facts needed for the task; it does not expose Node Secret, local absolute paths, SSH data or private network configuration.
- At 390 px and 320 px, all five administrator sections are visible in a two-row navigation rather than requiring a hidden horizontal scroll. Cards, spacing, typography and quiet warning colors follow the already accepted P2/P3 ivory, ink-green and sage direction.

## AUTOMATED VERIFIED

- Web lint, typecheck, five Vitest cases and production build passed after the final P4 UI change. Edge fixture browser checks exercised player unknown/quarantined/abandon confirmation and next-game running/stopping/paused/reclaimed states at 1440/390/320 px. Admin login, all five tabs and the abandoned-quarantine stop control were also checked. No page-level horizontal overflow was observed. Key screenshots remain in ignored `.local/p4b-*.png`, `.local/p4c-*.png`, `.local/p4d-*.png` and `.local/p4e-abandoned-admin-stop-390.png`.
- The fixture screenshots were compared with the accepted P2/P3 player flow. They retain the same warm background, deep green type/actions, pale sage selection/status areas, rounded cards, restrained orange warnings and generous spacing. The narrowest player and admin pages remain readable without horizontal scrolling.

## REAL-ENVIRONMENT VERIFIED

- Against the exact deployed development HTTPS build, Edge loaded the player `/` and administrator `/admin` login page at 1440, 390 and 320 px with normal TLS verification, no JavaScript page errors and no page-level horizontal overflow. The `/healthz` endpoint, both HTML routes and the hashed JS/CSS assets returned HTTP 200. Screenshots remain in ignored `.local/p4-live-*.png`.
- P4E's real Linux quarantine/next-game and post-abandon administrator stop drills exercised the underlying same-origin player and admin APIs, including AdminSession and Audit persistence. The one-use testing AdminUser was disabled afterward; its session returned 401. No entry was falsely marked verified or enabled.

## NOT VERIFIED

- Project-owner final player **and** administrator UI inspection/acceptance is pending. The development database currently has no enabled AdminUser for owner login; a one-time request for secure owner-controlled credential preparation was made separately. The test account was disabled and its password was not retained, logged or committed.
- A real Steam/steamchina entry experience, Windows VM public human connection, second public topology and external NAT variants remain outside P4 UI parity and are NOT VERIFIED.

The owner's subsequent visual feedback and revised UI checkpoint are recorded in `p4-ui-owner-followup.md`. Final owner acceptance remains pending there.
