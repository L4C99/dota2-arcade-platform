# P3 validation closure

Project-owner confirmation on 2026-09-26:

```text
P3A: PASS
P3B: PASS
P3C: PASS
P3D: PASS
P3E: PASS (implemented, automated/config-verified, and available real-environment scope only)
P3F: PASS
P3 UI owner acceptance: CONFIRMED
P3 overall: PASS
Owner acceptance: CONFIRMED
```

The owner inspected the current development site's P3 flow and final player UI and found no blocking issue. This closes P3 at its authorized scope; it does not claim a V1 release, production deployment, or real validation of absent network topologies. The final implementation baseline before this docs-only closure is `79b5fc95eeef6ebadc357237a1360fb748b5899a`, matching local HEAD and `origin/main` with a clean worktree before closure.

## Checkpoints and CI

These full SHAs were read from the actual linear Git history. Each P3 checkpoint and UI follow-up had a completed, successful GitHub Actions run before closure.

| Milestone | Full commit SHA | CI run |
| --- | --- | --- |
| P3 start / P2 closure | `5feb278e43802dc570a75d1fd12ae86229479717` | `36154274462` |
| P3A | `166fde2d4894d9f1be3f5ce1dc8b7cffcefc1371` | `36157251138` |
| P3B | `aba695a8bb18d5b622af1d2c41a630c347158d4f` | `36159980804` |
| P3C | `54cee65b8cedaebba16b584caf28795bf5b51e43` | `36161380352` |
| P3D | `0ed5715525e8e215e9b5425e79c77396f220b50e` | `36168065357` |
| P3E | `f4d7cb3dd0b9b58b89ca5d78c020d17ce33d96e8` | `36169603406` |
| P3F | `eac5551ffc00408cc6e7e123fc278b487cfed8bb` | `36172083539` |
| UI parity | `de4bada321e4314bf49bbe966a7cc46a78975673` | `36173094375` |
| Owner-driven auto/manual UI fix; final owner-confirmed UI baseline | `79b5fc95eeef6ebadc357237a1360fb748b5899a` | `36174770674` |

The P3A–P3F validation details are in `p3a.md` through `p3f.md`; the deployed UI parity check is in `p3-ui-parity.md`. The final UI fix has its own Git checkpoint and development deployment evidence in the ignored local inventory. The closure commit is a separate documentation-only successor to the final UI baseline; its SHA is identified by Git history and the final closure report after creation.

## Capacity, selection, and waiting

**Implemented and automated PostgreSQL integration verified:** Each Node reports `hard_max_instances`; the operator sets `desired_max_instances`, which cannot exceed the current hard report. Effective admission capacity is the lesser of hard and desired. Occupied capacity counts every Allocation attempt except `reclaimed` and `released_no_effect`, including unknown, failed-unreclaimed, and quarantined attempts. Candidate Node rows are locked and checked in stable order; capacity reservation, Allocation creation, and create NodeJob creation are atomic in one transaction. An accepted or unknown create keeps its reservation until d2core confirms `lifecycle=reclaimed`, `process=stopped`, `cleanup=complete`; a clearly proven no-effect rejection may use `released_no_effect`.

Auto selection considers only currently eligible Nodes, then priority descending and Node ID ascending. Manual selection persists one chosen Node and never silently migrates to another. Waiting requests are considered by `requested_at,id`; FIFO means the earliest **currently eligible** request for the relevant resource, not an unconditional global head-of-line lock. An ineligible manual waiter does not block a later request that can use unrelated eligible capacity, and retains its own queue time. Cancel is safe only while waiting with no Allocation attempt. Choosing a different manual Node requires cancelling and submitting a new ServerRequest with a new `requested_at`; old Allocation attempts are never rewritten. These cases passed P3A/P3B disposable-schema Store and HTTP integration suites, local Go checks, and Web checks.

**Real-environment verified:** Two independent development Nodes both reported hard/desired `1/1`. P3D HTTPS business requests showed the priority Node selected by auto, fallback to the other eligible Node after Drain, and manual waiting on the specified Node. The temporary priority change was restored to `0` on both Nodes. Current capacity and Drain state are recorded below.

## Stale/offline, unknown create, and attempt history

**Implemented and automated PostgreSQL integration verified:** Heartbeat age classifies a Node as online before two minutes, stale from two to five minutes, and offline afterward. Stale/offline Nodes do not receive new Allocations or claim new NodeJobs. Existing Allocations and occupied capacity remain. A create with possible side effects and a lost response remains unknown on its original Node; timeout or an empty list is not proof of no effect and cannot authorize a cross-Node retry. On reconnect, the Controller reads d2core list and reconciles open durable jobs before claiming new work. Each Allocation attempt retains its own immutable Node, content, template, and history; only a positively proven safe no-effect result permits a later attempt on another eligible Node. Manual requests never migrate. P3C's Store/HTTP disposable-schema suites exercised these cases.

**Real-environment verified:** P3F restarted only the Controller while one real instance remained running on each OS. Reconnection preserved the same request, Allocation, create NodeJob, frozen execution, instance, and port; there was no duplicate create. Both were formally stopped and fully reclaimed, then a new request on each reconnected Node independently reached Ready and full reclaim. A deliberate five-minute real outage on both Nodes was **NOT VERIFIED**; the stale/offline duration matrix is integration-test evidence. No P4 quarantined player escape flow was added or claimed.

## Two Nodes, Drain, and fixed d2core

**Real-environment verified, not merely CI:** The existing Linux amd64 development Node and the owner-selected Windows 10 amd64 VM were independently registered, authenticated, and heartbeating to the development Platform. Each used its own Node ID, Secret, Controller config, d2core data-dir, local port pool, content directory, template binding, and capacity. Both Controllers ran clean P3E-source builds with embedded revision `f4d7cb3dd0b9b58b89ca5d78c020d17ce33d96e8` and reported compatible fixed d2core **v0.1.1**, commit **`988720ad85af1f0d97bfe98ec4da4fcbb070beea`**, protocol 1, and `p1-test-v1 / confirmed` content. These were real processes on both operating systems.

Real create followed d2core operation/status to Dota Ready, captured the actual local port, and produced JoinInfo. Linux used port 28000; Windows used port 28100. On both Nodes, Controller restart/reconcile preserved the running instance without duplicate create; formal stop reached d2core `reclaimed/stopped/complete`, followed by another new Ready instance and another full reclaim. P3D also verified independent capacity, auto/manual scheduling, priority/fallback, and Drain/Resume: an existing instance continued while its Node was Drained; a new auto request used the other eligible Node; a manual request for the Drained Node kept waiting, then proceeded only after Resume and capacity became available. Drain and temporary priority were restored. Windows manager and Controller remain ordinary-account foreground processes rather than an unattended Windows service.

## Network evidence and limits

| Scope | Implemented | Automated/config verified | Real-environment verified |
| --- | --- | --- | --- |
| `connect_host`, optional distinct `protocol_ip`, identity and complete explicit local-to-public mappings | Yes | Yes, including malformed/missing/duplicate/out-of-pool rejection and entry-revision invalidation | Current Linux public identity and Windows private identity facts only |
| Actual d2core local port → mapping → JoinInfo | Yes | Yes, including explicit asymmetric mapping and suppression when mapping is missing or invalid | Linux 28000→28000 with its public connect host; Windows 28100→28100 with its private connect host |
| Direct Linux public reachability | Yes | Yes | Existing P1 human join; P3D/P3F real Ready and actual-port JoinInfo. P3E did not claim a new human join |
| Windows VM public human join | Yes, via the general model | Local identity mapping checked | **NOT VERIFIED**; VM is on a private network |
| Public game-node domain, vendor NAT/explicit forwarding, second independent public IP, shared/asymmetric public egress | Yes, via the general model where applicable | Config/model and rejection tests passed | **NOT VERIFIED**; those topologies do not exist in this development environment |
| A2S / Steam / steamchina verified workflow | No; belongs to P5 | No P3 claim | **NOT VERIFIED** |

An unmapped or invalid actual port never produces invented JoinInfo and never releases an occupied Allocation merely to make the UI look ready. No firewall, router, NAT, cloud security-group, DNS, or public mapping was changed or fabricated for P3.

## Player UI acceptance

**P3 UI owner acceptance: CONFIRMED.** The owner checked auto/manual Node selection, Node status, waiting reason, safe cancel, assigned Node, the P2 Party UI regression, and basic desktop/mobile behavior on the development site. The final owner-driven correction makes auto and manual one exclusive mode choice, reveals Node radios only in manual mode, requires a manual target before submit, clears hidden manual choice on return to auto, and states the effective selection immediately above submit. Local Web lint/typecheck/tests/build, isolated 1440/390/320 px browser checks, and checks against the exact deployed bundle passed. Current UI meets the P3 acceptance standard; further purely decorative adjustment is outside this closure.

## Read-only final development state

At approximately 2026-09-26 02:54 +08:00, the existing development Platform loopback health returned `ok`. Development PostgreSQL was at migration 10. Read-only queries found 29 `reclaimed` Allocations and **zero occupied or failed-unreclaimed Allocations**, 29 ended and three cancelled ServerRequests, zero blocking active requests, and no pending/claimed/accepted/unknown NodeJob. Create jobs were 33 `succeeded` plus one historical P0 `failed_with_effect`; stop jobs were 34 `succeeded`. That historical P0 job is integration-only, has no Allocation, and its recorded d2core instance was directly checked as `lifecycle=reclaimed`, `process=stopped`, `cleanup=complete`. It is explained history, not an open instance.

Both Nodes had heartbeat/report timestamps within seconds of the database read, compatible fixed d2core reports, `p1-test-v1 / confirmed`, desired/hard `1/1`, priority `0`, Drain `false`, accepting new work, and occupied `0`. Direct fixed d2core `list --json` on Linux and Windows returned `instances: []`; both d2core managers and both Controllers were running, with no Dota dedicated process observed. No new test instance, stop, DB cleanup, data-dir deletion, host change, or business-code change was made for closure. The ignored `.local/p0-host-inventory.md` records per-Node operational facts without Secret values.

The prior checkpoint CI runs all completed successfully, including the final UI baseline. This docs-only closure is separately committed and pushed in the ordinary way; its own CI and SHA are reported after GitHub Actions finishes.
