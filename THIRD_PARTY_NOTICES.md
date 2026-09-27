# Third-party notices

Platform source is MIT licensed; retain the root LICENSE in redistributed packages.

Node Controller incorporates code from **dota2-arcade-dedicated-core**. The fixed
Platform dependency is **v0.1.1 / 988720ad85af1f0d97bfe98ec4da4fcbb070beea**.
Its copyright is **Copyright (c) 2026 L4C99**. The complete MIT text is in
`licenses/d2core-LICENSE.txt`; retain it with compiled Controller distributions.
The historical-code grant, including compilation into Node Controller binaries,
is reproduced in `licenses/d2core-LICENSING.md` from
[db246b2bcce888b87d7854bb12012ea4e90e82cb](https://github.com/L4C99/dota2-arcade-dedicated-core/blob/db246b2bcce888b87d7854bb12012ea4e90e82cb/LICENSING.md).
That commit is licensing evidence only, not a runtime dependency upgrade.

Platform packages do **not** redistribute the d2core server executable. Obtain it
separately from the official fixed v0.1.1 Release as described in deploy/README.md.
The original tag, commit and Release assets remain unchanged by the clarification.

Release packaging collects complete license/notice files for the Go modules
compiled into the five supported binaries, the Go runtime, and installed npm
production dependencies. See `licenses/` and `DEPENDENCIES.json` in each package.
Vue runtime is MIT licensed; the conservative notice set also includes compiler
and peer tooling (including Apache-2.0 TypeScript). Go runtime and golang.org/x modules
use BSD-style terms, pgx/puddle/pgpassfile/pgservicefile and go-winio use MIT.
Third-party material retains its own terms; the d2core grant does not relicense it.

The locked npm build/test graph also includes ISC, BSD-2-Clause, BSD-3-Clause,
Apache-2.0, MIT-0, BlueOak-1.0.0 and CC0-1.0 packages. Build tools and node_modules
are not shipped. Preserve upstream notices if distribution scope changes.
