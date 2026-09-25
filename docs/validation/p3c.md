# P3C stale/offline, unknown effects, and safe attempt history

P3C derives Node connectivity from the last server-recorded heartbeat: online before 2 minutes, stale from 2 to 5 minutes, and offline after 5 minutes or when never reported. The allocator and NodeJob claim path both require online. An existing Allocation and its occupied slot remain unchanged through stale/offline; Controller reconnect still reads d2core list and open durable jobs before claiming anything new.

A create with a lost response remains unknown on its original Node. It is not copied to another Node or treated as no effect. After a positively proven no-effect rejection, an auto ServerRequest returns to waiting with its original queue time and may receive a later Allocation attempt on another eligible Node. Its old attempt retains its original Node/content/history. The next attempt snapshots the then-current target ContentVersion. A rejected Node is excluded from immediate repeat attempts for that request; if all known Nodes have rejected without effect, the request ends unavailable. Manual requests never switch Nodes. P4 quarantine and player escape controls are not introduced here.

## Verification

- Local Go suite and vet: PASS at the P3C checkpoint.
- Full Store and HTTP PostgreSQL integration suites: PASS as `arcadedev` in random disposable schemas of the existing development database. Read-only post-run query found no remaining test schema; temporary binaries were removed. The Store cases cover stale/offline claim blocking, capacity and old attempt retention, reconnect to the same durable job, unknown create without duplicate, manual refusal to switch Nodes, and safe two-attempt history/content snapshot after an explicit no-effect result.
- Web lint/typecheck/unit/build and Linux/Windows builds: PASS at the P3C checkpoint.

## Limits

- No real Node or Dota process was disconnected in this substage. The integration suite uses disposable schemas and simulated heartbeat/report facts; P3D/P3F carry real multi-Node and dual-platform checks.
- The current fixed d2core/Controller path does not infer no effect from an ambiguous transport loss or an empty list. Such an unknown remains blocked until a reliable reconciliation result; no unsafe timeout fallback exists.
