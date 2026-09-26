# P5A multi-content checkpoint (in progress)

P5 start baseline: `ef767be1b7a95a50d4ccbde9117baaa970fce11f` on `origin/main`, clean before work. The owner authorized P5 automation first and will provide the actual second content later.

## Implemented

- Admin can register an ArcadeGame, immutable ContentVersion, TemplateRevision, GamePreset and node template binding. New games and presets start disabled and not accepting requests.
- Controller-reported NodeContentBinding stays distinct from admin-controlled `accepting_new_allocations`. Reported VPK SHA256 is checked against the immutable ContentVersion digest. Existing Controllers without the new digest remain readable during upgrade, but cannot receive a new human content validation.
- Human content validation requires an empty drained node, fresh compatible Controller report, confirmed target version and matching digest. Publication requires that validation plus a currently eligible matching node; it changes only `ArcadeGame.current_content_version_id`, and writes Audit. Existing Allocation version snapshots remain unchanged.
- Automated PostgreSQL tests cover two games, multiple presets/versions, unpublished visibility, digest mismatch, publication gates, waiting request version freeze and audit. A migration test upgrades a populated migration-13 schema in a disposable schema.

## Verification

- Local `go test ./...` passes; PostgreSQL Store/HTTP tests skip locally without `PLATFORM_TEST_DATABASE_URL`. GitHub Actions [run 36231578148](https://github.com/L4C99/dota2-arcade-platform/actions/runs/36231578148) passed all four jobs, including the disposable PostgreSQL integration suite and migration-13 upgrade test. The first CI run found and led to correction of an older P4 test's hard-coded final migration number.
- Local read-only development inventory found one verified real Workshop (`3564393242`, `p1-test-v1`) on Linux and Windows. No confirmed second VPK or game was found.

## NOT VERIFIED

- Actual second game/version registration, content preparation, real request and player entry await owner-provided content details and files. P5A is not marked PASS.
