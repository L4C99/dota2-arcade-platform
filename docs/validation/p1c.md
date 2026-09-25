# P1C validation — JoinInfo and connect entry

Date: 2026-09-25. Scope: P1C. Player Web and human Dota join remain P1D/P1E work.

## Implemented

- Controller generates JoinInfo from the concrete d2core instance status port and its validated local identity or explicit public-port mapping. An unmapped actual port produces a structured error; the Ready instance remains running and occupies capacity.
- Platform records `ready_at` and `join_info_available_at` at its own UTC observation time, only after Ready and valid JoinInfo. The player Allocation API exposes a copyable `connect` command, never the local template path.
- Node entry capabilities retain separate A2S enabled/query results and Steam/steamchina verified/enabled flags. A network-config revision change clears URI verification. URI fields are absent until both verified and enabled; the development node keeps A2S and both URI entries off.
- The manual connect path is specified for the player page: copy the command, enable/open the Dota 2 console, paste the command, and execute it. The exact current client menu steps must be checked during P1E human use before finalizing player help, as required by the frozen spec.

## Tests and real development result

- Local Go tests and vet: PASS. Isolated development PostgreSQL tests cover Ready without mapping retaining capacity, revision invalidation, and valid JoinInfo exposure: PASS.
- Development database backup preceded migration 6. Candidate Platform and Controller binaries were deployed on the authorized development hosts; one real business Allocation reached d2core v0.1.1 `active/running/ready`. Fixed-release `status` reported actual local port 28000, and the Controller identity mapping produced public port 28000. The HTTPS player API returned `connect` with the configured development host and that port. It returned no Steam or steamchina URI.
- The owner stop API created a separate stop NodeJob. d2core confirmed full reclaim. Final list was empty, no dedicated Dota process remained, and no active or unknown business job remained. No firewall, cloud security group, NAT, VPK, patcher, or addon link was changed.

Platform UTC observations for this no-queue sample: `requested_at` 09:43:35.270120Z, `assigned_at` 09:43:35.700882Z, `create_started_at` 09:43:40.254388Z, `ready_at` and `join_info_available_at` 09:44:00.232312Z. Queue wait was 0.431s, dispatch/start delay 4.554s, Dota startup 19.978s, and time to JoinInfo 24.962s. These intervals do not establish an SLO; P0's approximately 10–16s samples measured core create acceptance to Ready.

Public UDP reachability, current client console steps, human join, A2S query, and Steam/steamchina URI use are **NOT VERIFIED**. GitHub CI and the checkpoint SHA are tracked with the pushed P1C commit.
