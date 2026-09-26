# P5A multi-content checkpoint (in progress)

P5 start baseline: `ef767be1b7a95a50d4ccbde9117baaa970fce11f` on `origin/main`, clean before work. The owner subsequently supplied and authorized [two real development VPK inputs](p5-content-input.md), three new-game templates and deployment to both development nodes.

## Implemented

- Admin can register an ArcadeGame, immutable ContentVersion, TemplateRevision, GamePreset and node template binding. New games and presets start disabled and not accepting requests.
- Controller-reported NodeContentBinding stays distinct from admin-controlled `accepting_new_allocations`. Reported VPK SHA256 is checked against the immutable ContentVersion digest. Existing Controllers without the new digest remain readable during upgrade, but cannot receive a new human content validation.
- Human content validation requires an empty drained node, fresh compatible Controller report, confirmed target version and matching digest. Publication requires that validation plus a currently eligible matching node; it changes only `ArcadeGame.current_content_version_id`, and writes Audit. Existing Allocation version snapshots remain unchanged.
- Automated PostgreSQL tests cover two games, multiple presets/versions, unpublished visibility, digest mismatch, publication gates, waiting request version freeze and audit. A migration test upgrades a populated migration-13 schema in a disposable schema.

## Verification

- Local `go test ./...` passes; PostgreSQL Store/HTTP tests skip locally without `PLATFORM_TEST_DATABASE_URL`. GitHub Actions [run 36231578148](https://github.com/L4C99/dota2-arcade-platform/actions/runs/36231578148) passed all four jobs, including the disposable PostgreSQL integration suite and migration-13 upgrade test. The first CI run found and led to correction of an older P4 test's hard-coded final migration number.
- Before modification, read-only development inventory found only Workshop `3564393242` (`p1-test-v1`) on both nodes, migration 13, zero active Allocations and zero open NodeJobs. Owner-supplied `2307479570` and distinct legacy `3564393242` VPKs were copied into isolated local storage and verified byte-for-byte by SHA256.
- The development database was backed up before migration 14, the archive was restored successfully into a separate disposable database (which still showed migration 13), and the live development schema reached migration 16. The exact P5 Platform/Web release passed HTTPS health and asset checks. Both P5 Controllers remained compatible.
- Audited Admin API actions registered Workshop `2307479570` as temporary `P5 测试游廊`, ContentVersion `p5-2307479570-v1`, three distinct TemplateRevisions and three 10-player GamePresets, with six Linux/Windows template bindings. The existing game received the separately immutable `p5-3564393242-legacy` ContentVersion. One-use setup Admin accounts were disabled afterward. The new game and presets remain disabled, have no published current version, and do not accept requests.
- Both Controllers independently reported `p5-2307479570-v1` as confirmed from the actual addon link/Junction and SHA256 `1e086308024da977f0ce47d61aaa3cb411ca29a51f6ece7a38f607a7f74187f2`. On the drained development nodes, fixed d2core v0.1.1 reached Ready with the new game's `custom` template on Linux and Windows; Linux also reached Ready with its distinct `dota` and `hard` templates. Each temporary instance was explicitly stopped and reached `reclaimed/stopped/complete`; Controller reconciliation completed before Resume. Steam login success remained part of every Ready condition.

## NOT VERIFIED

- Real player entry, human content validation, Admin publication of `current_content_version_id`, enabling the new game/presets/bindings, and a formal multi-content player request remain pending. The new game is deliberately not exposed to players yet. P5A is not marked PASS.
