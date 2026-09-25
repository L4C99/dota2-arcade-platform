# P2 Party UI focused parity checkpoint

Date: 2026-09-25. Parent functional checkpoint: `69450e1eff23a85e3e9dd203b1c8ba4d7a7c4b74`.

The formal Party page was compared against `prototype/p1/party.html` and `party.css` while keeping the accepted P1 server panels. It has the same two-column desktop/card hierarchy and one-column mobile flow: member rows and leader/member badges in the main card, a separate invitation card with copy/reset emphasis, and a current-server summary card at the side. The formal page uses existing P1 design colors, typography, buttons and spacing rather than mock API/data. Future P3/P4 prototype controls remain absent.

Final inspection found that a query-string invitation URL could propagate its credential through page requests or Referer headers. The formal link now uses `#invite=...`; fragments are not sent as the HTTP request target. The landing page reads this fragment, the join API receives only the token body, and the browser URL is cleaned after a successful join. A frontend test asserts the generated URL has an empty query and the expected fragment. Invite token values are not recorded here.

Web lint, typecheck, tests (including the new invite-link test), and build: PASS. The in-app browser opened the development page but timed out twice when asked for page state, so rendered desktop/mobile parity and independent browser-context UI interaction remain **NOT VERIFIED**. The owner has been asked for a single final UI confirmation on the exact development site before P2 overall PASS is declared.

Full checkpoint SHA, CI result, exact Web asset and deployment check are recorded after push.
