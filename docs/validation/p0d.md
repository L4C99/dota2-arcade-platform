# P0D validation — fixed d2core real calls

Date: 2026-09-25. Scope: P0D only. P0E restart, unknown-create, and long
disconnect reconciliation remain separate work.

## Fixed identity and environment

- d2core official Release `v0.1.1`, commit
  `988720ad85af1f0d97bfe98ec4da4fcbb070beea`, protocol 1, template
  schema 1, disk format 2. The official Linux ZIP matched its Release SHA256;
  the node's `version --json` matched `BUILD.json` with `gitDirty=false`.
- A development Platform Server and independent PostgreSQL database ran on an
  Ubuntu 24.04 web test host, behind trusted HTTPS. The database used a
  development data directory and local Unix socket, with no TCP listener.
- Node Controller, d2core, and Dota ran as the same ordinary user on a separate
  Ubuntu 24.04 game test host. d2core data and configuration were in a reserved
  development tree outside the Dota tree. Its actual `serve` port range and
  the Controller's reported range both came from the same local development
  configuration. The owner-provided test VPK was copied and hashed; the prior
  addon was preserved before the development copy was bound.

## Actual integration results

1. An authenticated Controller heartbeat over HTTPS reported compatible with
   d2core `v0.1.1` protocol 1. Before each create call, Platform persisted a
   NodeJob-derived idempotency key, the resolved absolute template path, the
   requested automatic port value, and the request fingerprint.
2. The first n6 create was accepted and started Dota. Four of five Ready
   markers appeared; the Steam connection success marker did not appear before
   the provided template's 120-second deadline. d2core returned
   `START_TIMEOUT`, and Platform recorded `failed_with_effect`. A separate
   Platform stop job completed; d2core reported
   `reclaimed/stopped/cleanup=complete`, and `list` was empty.
3. A separate template copy changed only `startupSeconds` from 120 to 300;
   all Ready rules stayed the same and d2core `check` passed. A new integration
   NodeJob used a new deterministic key. Its create operation succeeded in
   about 13 seconds with all five Ready markers, including Steam connection
   success. d2core status reported `active/running/ready` and an actual port
   inside the configured pool. Platform reported the create job succeeded.
4. The second instance was stopped through another Platform NodeJob. Its d2core
   stop operation succeeded, status was `reclaimed/stopped/cleanup=complete`,
   `list` was empty, and no Dota process remained. No unknown operation or
   unreclaimed test instance remained at the end of P0D testing.

## Checks and limits

- `go test ./... -count=1`: passed locally on Windows.
- `go vet ./...`: passed locally.
- Linux Platform Server and Controller test binaries cross-built successfully;
  the real Ubuntu processes ran from copied binaries, never from a source tree.
- The earlier GitHub CI run for the same code baseline passed Ubuntu 24.04 and
  Windows checks. The real calls above were on Linux only.
- Human Dota join, public game-port reachability, Windows VM d2core operation,
  and P0E restart/unknown cases were not verified here. Ready does not claim
  a human player can join.

Real host addresses, Node IDs, runtime paths, logs, credentials, and transient
instance IDs are kept out of this public record. The current local facts and
status are in Git-ignored `.local/p0-host-inventory.md`.
