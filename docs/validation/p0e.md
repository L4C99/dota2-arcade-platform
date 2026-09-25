# P0E validation — restart and unknown-create recovery

Date: 2026-09-25. Scope: P0E only. No P1 business flow was implemented.

## Fixed dependency and real environment

- The Linux game test node ran the official d2core `v0.1.1` Release binary.
  `version --json` after the tests reported commit
  `988720ad85af1f0d97bfe98ec4da4fcbb070beea`, protocol 1, template
  schema 1, disk format 2, Go 1.27.1, and `gitDirty=false`.
- Platform Server used the isolated development PostgreSQL database behind
  trusted HTTPS. Controller and d2core ran as the same ordinary user with
  separate development binaries, configuration, data, and logs. Real Dota
  instances used the owner-provided n6 content copy and the P0D-verified
  300-second template variant. The game port was inside the 28000–28009
  development pool.
- Private host addresses, runtime paths, transient IDs, and precise current
  state remain in Git-ignored `.local/p0-host-inventory.md`.

## Real recovery results

1. A real create reached `active/running/ready`. Platform Server restarted
   while Dota was running; health returned and its durable NodeJob retained
   the same instance and operation IDs.
2. Controller restarted while that instance was running. After its compatible
   HTTPS heartbeat, d2core `list` still contained exactly the original
   instance; no second create appeared.
3. The fixed d2core manager stopped while Dota remained alive, then restarted
   on the same development data directory. Its `list`, `operation`, and
   `status` recovered the original instance, operation, and Dota PID.
4. A development-only one-shot wrapper sent a real frozen create request to
   d2core and discarded the accepted response. Platform durably recorded
   `unknown` without core IDs. The ordinary Controller then reconciled by
   calling `list`, retrying the same frozen request with the same
   NodeJob-derived key, and reading the original operation/status. Durable
   reports were `unknown → accepted → succeeded` with the original IDs;
   d2core had exactly one instance throughout.
5. The same lost-response scenario was repeated with Controller disconnected
   until the node heartbeat was 2 minutes 42 seconds old. The original
   `unknown` job remained on its original node and d2core retained one
   instance. On reconnect, the ordinary Controller converged to the original
   IDs without a second instance or a new key.
6. Each of the three P0E instances received a separate Platform stop NodeJob.
   Stop success was reported only after d2core status showed
   `reclaimed/stopped/cleanup=complete`. Final `list` was empty and no Dota
   dedicated process remained. No open `unknown` job remained. One historical
   P0D `failed_with_effect` create is explained by its separate successful
   stop and full reclaim.

## Safety behavior and checks

- Reconciliation now reads d2core `list` before open Platform jobs, then
  operation/status for known IDs, and only then claims new work. A create
  request is persisted with its deterministic key and fingerprint before the
  first core call. A response loss retries only that same frozen request.
- d2core v0.1.1 can expire reclaimed history. The Controller uses the
  persisted preparation time and actual `list.storage.historyDays` to stop
  automatic same-key retries near expiry. An uncertain job remains `unknown`
  and blocks new claims. The retention boundary was tested with simulated
  old timestamps; waiting 30 real days was not practical in P0E.
- `go test ./... -count=1` and `go vet ./...` passed on the local Windows
  development machine. The PostgreSQL test that requires
  `PLATFORM_TEST_DATABASE_URL` was skipped locally; the development database
  was exercised through real Node API and job persistence. Linux Platform and
  Controller binaries were cross-built and executed on Ubuntu 24.04.

Human Dota join, external game-port reachability, Windows VM real d2core
operation, and actual 30-day history expiration remain unverified. They are
not claimed by these P0E tests. No tag, Release, production deployment,
firewall, cloud security group, or NAT change was made.

## Ready timing clarification

Successful real create acceptance to d2core Ready took about 12.4, 15.6,
and 10.1 seconds in the three P0E runs. Together with the P0D success at
about 13 seconds, observed normal Ready latency is approximately 10–16
seconds. The earlier P0D `START_TIMEOUT` was a separate run using a
120-second ceiling; its Steam success marker was still missing at that
deadline. The later 300-second ceiling applies only to the current
development template variant. It is neither normal startup latency nor a
frozen production timeout; production `TemplateRevision` and timeout remain
to be decided from later player end-to-end data and more startup samples.
