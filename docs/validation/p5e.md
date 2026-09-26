# P5E operations checkpoint (in progress)

`docs/operations.md` records backup, disposable restore check, database and binary upgrade, controlled rollback, Controller restart, fixed d2core/Dota maintenance, ContentVersion switching and rolling release, unknown-job reconciliation and human entry trials. `deploy/scripts/backup-postgres.sh` creates a timestamped custom-format dump without putting credentials on its command line and verifies it with `pg_restore --list`.

The shell script passes syntax checks. A real backup and restore drill, node installation, rolling release and rollback are **NOT VERIFIED**. P5E is not marked PASS.
