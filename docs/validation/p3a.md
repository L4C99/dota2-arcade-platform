# P3A capacity checkpoint

Start baseline: `5feb278e43802dc570a75d1fd12ae86229479717` (`main` and `origin/main`, clean worktree). Its documented GitHub CI run `36154274462` succeeded across Ubuntu Go, Windows Go, PostgreSQL integration and Web. This checkpoint's own full SHA and subsequent CI result must be recorded after the commit exists.

The allocator now locks candidate Node rows in stable order, re-reads Controller reported facts after acquiring the lock, checks each Node's occupied attempts against `min(hard_max_instances, desired_max_instances)`, and reserves one slot with the Allocation and create NodeJob in one transaction. The Controller heartbeat remains the only source of hard capacity. A database-authorized operator CLI reads capacity and rejects a desired value above the current hard report; if a later heartbeat reduces hard, scheduling still uses the lower hard value without rewriting desired.

Automated PostgreSQL integration cases cover two independent Nodes under concurrent reservations, last-slot protection, unknown and failed-unreclaimed occupancy, desired reduction, rejected desired above hard, and hard shrink. Existing P1 lifecycle tests cover reclaim and no-effect release. The full Store suite and HTTP API integration suite passed against unique disposable schemas in the authorized development PostgreSQL database. A read-only post-test schema query returned no test schemas, and the temporary test binaries/directory were removed.

Local `go test ./... -count=1` and `go vet ./...` passed with a writable Go build cache. Web lint/typecheck, three Vitest tests and production build passed. Platform Server and Node Controller both built for Linux amd64 and Windows amd64; this is build evidence only, not real Controller runtime validation. The full Linux Store test binary passed 20 tests, including the two new P3A tests, and the HTTP API test binary passed four tests with PostgreSQL enabled. The owner expressly authorized SSH access to this development database host for these isolated tests after automatic approval review initially rejected an attempt lacking explicit host authorization. No existing business schema, runtime service or host configuration was changed.

P3A capacity behavior and regression checks: **PASS**. Checkpoint CI must pass before P3B begins. Real multi-node scheduling is P3D and remains **NOT VERIFIED**. P3B auto/manual priority and FIFO behavior has not started.

No development or production runtime was changed, and no P3B work has started.
