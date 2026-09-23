# AGENTS.md

This repository is developed primarily with AI coding agents.

## Authority

- `docs/specs/v1.md` is the frozen V1 product/architecture/acceptance specification.
- Read the full frozen spec before substantial implementation work.
- The current user prompt authorizes only the named stage/substage. The full V1 spec is context, not permission to implement everything.
- If a user request appears to change a frozen invariant or V1 scope, do not silently reinterpret the spec. Point out the conflict and wait for explicit project-owner direction.
- Formal implementation/API/migration docs created later may refine implementation details, but may not contradict `docs/specs/v1.md`.

## Fixed d2core dependency

V1 is built against:

- Release/tag: `v0.1.1`
- Commit: `988720ad85af1f0d97bfe98ec4da4fcbb070beea`
- protocolVersion: `1`
- template schemaVersion: `1`
- disk formatVersion: `2`

Rules:

- Do not substitute d2core `main` behavior for the fixed release.
- Do not modify d2core as part of this platform project.
- Platform Server never talks directly to d2core.
- Node Controller talks to local d2core through its supported local API/client.
- Do not reimplement d2core process identity, port allocation, Ready detection, stop/reclaim, or recovery semantics.

## Stage discipline

Project order is:

`P-1 → P0 → P1 → P2 → P3 → P4 → P5`

For every coding session:

- Execute only the stage/substage explicitly authorized by the project owner.
- Do not implement later stages just because they are visible in the spec.
- Avoid temporary architecture that the immediately following stage would have to replace.
- When the authorized substage is complete, run the relevant checks, report results and remaining limitations, then stop.
- Independent code review, production deployment, tags, releases, and production-node access require separate explicit authorization.

## Core safety invariants

Never violate these:

- `ServerRequest`, `Allocation`, `NodeJob`, and d2core instance are distinct objects.
- The same owner has at most one blocking normal `ServerRequest`; duplicate submissions return the existing request.
- One `ServerRequest` may have sequential Allocation attempts; never rewrite an old attempt's node/history to represent a new attempt.
- Create idempotency is stable across Controller crashes: the d2core key is deterministically tied to immutable `node_job_id`.
- Freeze the resolved d2core create request before the first core call; retry the same NodeJob with the same key and same frozen request.
- Timeout / lost response means **unknown**, not failure.
- If create may have produced side effects, do not reassign to another node until reconciliation proves it safe.
- A clearly no-effect create rejection may release its reservation; an accepted/unknown/instance-producing create may not.
- An accepted instance does not release capacity until d2core confirms `lifecycle=reclaimed`, `process=stopped`, and `cleanup=complete`.
- `quarantined` still consumes capacity and does not mean the old server stopped.
- V1 business flows do **not** call d2core instance `restart`.
- “Next game” is stop → full reclaim → new ServerRequest → new Allocation → create.
- Never enable formal join controls before d2core Ready **and** valid JoinInfo exist.

## Content/version invariants

- `ContentVersion` is immutable.
- `ArcadeGame.current_content_version_id` is the platform target for a newly created Allocation attempt.
- `NodeContentBinding.reported_content_version_id` is Controller-reported node fact; Web/Admin must not fake it.
- `Allocation.content_version_id` is snapshotted when the Allocation attempt is created and is not silently changed.
- V1 uses Node-level content isolation. Do not implement same-node active multi-version coexistence or per-instance content isolation.
- Symlink/Junction switching is for version management after the relevant content has been drained, not live hot-switching.
- Content Tool does not automatically Drain and Node Controller does not automatically invoke Content Tool.

## Security / environment

- Do not commit credentials, cookies, node secrets, SSH keys, private deployment data, database backups, VPK/game assets, crash dumps, or private player logs.
- Do not access SSH, production nodes, production databases, or production services unless the current task explicitly authorizes it.
- Do not change firewalls, Dota/VPK content, or host-level service configuration unless the current task explicitly authorizes it.
- Do not add Web shell, arbitrary command execution, arbitrary process kill, or arbitrary file browsing.
- Keep production TLS verification enabled; no production `InsecureSkipVerify`.

## Cross-platform / tooling

- Platform Server production baseline: Linux amd64, Ubuntu 24.04 LTS first verified distro.
- Node Controller target: Windows amd64 + Linux amd64.
- Content Tool target when implemented: Windows + Linux.
- Go toolchain must satisfy the frozen d2core v0.1.1 dependency requirement (at least Go 1.27.1).
- d2core-related critical local paths must obey the fixed release's ASCII absolute-path constraints.

## Completion report

At the end of an authorized substage, report:

1. what was completed;
2. major files changed;
3. tests/checks actually run and their results;
4. anything not verified;
5. current Git status;
6. commit SHA if a commit was authorized/made;
7. the next suggested substage, without starting it.
