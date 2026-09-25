# P1D UI/UX parity regression after P1E

Date: 2026-09-25. Baseline: the pushed P1E functional checkpoint `91b0b06c36899cf1b027bf50dd8c3d327c9bb768`. The owner confirmed the real application, Ready, JoinInfo, connect, human join, stop, and full-reclaim path. After the independent UI parity checkpoint `df969b074510f00d162d46b1b5f7225e40215ae7`, the owner explicitly confirmed final P1 UI acceptance.

## Approved reference and restored presentation

- Compared the accepted `prototype/p1/index.html` and `flow.html` with the formal `web/`. The prototype's Party and Admin pages describe later stages and were not brought into P1.
- Replaced the oversized formal hero with the approved compact page heading. Restored the prototype's content width, two-column application layout, card sizing, type hierarchy, light borders, soft background layers, and mobile spacing.
- Restored the compact game/preset selection cards, single-user summary, application confirmation hierarchy, status facts, bordered progress steps, phase badges, and restrained button treatment. Formal content remains the one real ArcadeGame and one real n6 GamePreset with its ten-player limit.
- Brought the connect command and its copy button into one bordered card. The console help retains the copyable `-console` option, suggested default `\` key, and reminder to check the actual in-game hotkey. The Help panel now uses the accepted readable text scale and separators.
- Kept real maintenance/error notices and disabled submission visible. The UI does not display TemplateRevision, node template paths, or unverified Steam/steamchina controls.

The prototype's mock games, extra presets, Party data, manual/multi-node selector, simulation toolbar, next-game action, and unverified one-click entry buttons were intentionally omitted. The P1 screen says that the platform checks its current development node. No mock API or fixture is shipped in `web/`.

## Checks performed

- `npm run lint`, `npm run typecheck`, `npm test`, `npm run build`: passed. ESLint was added to the formal Web scripts.
- `go test ./... -count=1`, `go vet ./...`: passed. No Go source, HTTP contract, migration, or state machine was modified.
- Isolated Edge checks at actual 1440, 390, and 320 CSS-pixel viewports covered idle, waiting, allocating, creating, Ready, Ready without JoinInfo, stopping, and ended. All 24 combinations had no page-wide horizontal overflow. The Ready state showed connect and the owner stop action; the no-JoinInfo state showed no connect and kept the stop action. Clipboard checks copied the exact connect command and `-console` option. Global, game, and preset maintenance each displayed its message and disabled Apply.
- Browser states other than the live idle/ended state used a Git-ignored local fixture server for visual inspection only. The P1E real human and full-reclaim evidence remains the functional baseline; this pass did not change the request, polling, Session, stop, or JoinInfo functions.

The exact parity Web bundle was deployed to the isolated development Web directory. Real HTTPS browser checks at 1440 and 390 CSS-pixel widths restored anonymous Sessions and displayed the one real game, one n6 preset, `max_players=10`, and no current blocking request from the actual Platform API. The served hashed JavaScript matched the parity bundle; both widths had no horizontal overflow. CI run `36126617898` passed Ubuntu Go, Windows Go, PostgreSQL integration, and Web including lint. No additional Dota instance was created for this UI-only pass.

**P1 UI owner acceptance: CONFIRMED.** This confirmation concerns the accepted P-1 visual and interaction parity. The earlier P1E functional checkpoint remains the separate real end-to-end acceptance baseline; UI parity did not repeat it or alter its business semantics. See [P1 stage summary](p1-summary.md).
