# P1E validation — real player closure

Date: 2026-09-25. Scope: the authorized P1 solo player development environment.

## Human result

The project owner reported that the complete Web → Dota → Web stop flow worked and supplied a screenshot of the page in its `已结束` state. The same anonymous owner produced two successive real ServerRequests during this session; both reached Ready with JoinInfo and later ended through separate successful stop NodeJobs. The owner's report is the evidence for actual Dota client entry and usable play; server logs and Ready markers alone are not counted as human entry evidence. No private player IP or client log is retained in this public record.

The owner identified a player-help regression from the accepted P-1 prototype. The formal Web now restores a copy button for the `-console` Steam launch option, suggests the default `\` console key, and tells players to check or rebind the actual in-game console key if it differs. The completed human session used the earlier Web help. At the owner's explicit direction, the precise current-client menu and key steps were not separately rechecked after this wording change; that detail remains **NOT VERIFIED** rather than being presented as independently proven.

## Automated and system evidence

- Isolated development PostgreSQL tests: concurrent duplicate submissions for one User produce one blocking ServerRequest, one Allocation, and one create NodeJob; a lost POST response recovers the current request; reopening the Platform Store retains the Session, ServerRequest, Allocation, frozen NodeJob, and their relationship; Ready without a port mapping leaves the instance running and capacity occupied.
- Both human-session Allocations snapshotted `p1-test-v1` at assignment, used the confirmed NodeContentBinding and existing durable NodeJob path, and received JoinInfo for d2core's actual local game port 28000 mapped to public port 28000. A2S and both URI entries remained disabled and unverified.
- Both stop jobs succeeded. For each real d2core instance, fixed v0.1.1 status showed `lifecycle=reclaimed`, `process=stopped`, `cleanup=complete`. Final d2core list was empty, no Dota process remained under the development game user, and the Platform database had zero active Allocations, zero blocking ServerRequests, zero open unknown/business NodeJobs, and zero failed-unreclaimed Allocations. The sole node had effective capacity 1 with occupancy 0.
- Development services remain running. No firewall, cloud security group, NAT, VPK, patcher, addon link, or production configuration was changed.

## Platform UTC timing

Both samples had no prior active Allocation occupying the single slot when requested. Intervals use Platform observation timestamps, not browser polling or cross-host clock subtraction.

| Sample | requested_at | assigned_at | create_started_at | ready_at | join_info_available_at | queue_wait | dispatch/start | Dota startup | time_to_join |
| --- | --- | --- | --- | --- | --- | ---: | ---: | ---: | ---: |
| First | 2026-09-25 10:28:42.259350Z | 10:28:42.755289Z | 10:28:45.332351Z | 10:29:00.301593Z | 10:29:00.301593Z | 0.496s | 2.577s | 14.969s | 18.042s |
| Second | 2026-09-25 10:30:53.105277Z | 10:30:54.753221Z | 10:30:55.326457Z | 10:31:10.300227Z | 10:31:10.300227Z | 1.648s | 0.573s | 14.974s | 17.195s |

P0's approximately 10–16s samples measured core create acceptance to Ready, a narrower interval than P1 `dota_startup_time`. These few samples establish no formal SLO or ETA.

## Boundaries

The owner-confirmed connect path is PASS. Independent validation of the revised console-help wording is NOT VERIFIED by explicit owner direction. A2S queries, Steam/steamchina URIs, Windows Node, multi-node scheduling, Party, production timeout, production content/version release, and production deployment were outside P1 and remain NOT VERIFIED.

The owner subsequently confirmed the formal Web's visual and interaction parity in a separate [UI checkpoint](p1d-parity.md). That approval does not replace this P1E functional checkpoint or imply a second human Dota run. The combined P1 stage acceptance is recorded in the [P1 summary](p1-summary.md).
