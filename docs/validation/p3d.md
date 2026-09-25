# P3D second development Node and ordinary Drain

## Implementation

`platform-server node drain|resume <node-id>` controls only new Allocation admission through the existing `nodes.draining` fact. Existing Allocations, NodeJobs, and d2core instances continue their normal lifecycle. Auto scheduling can choose another eligible Node; manual selection waits for its specified Node. Resume still requires the normal compatibility, content, capacity, and heartbeat checks. No ContentVersion switching or P5 content workflow was added.

The Windows Controller's content readback now compares the directory objects behind the addon binding and metadata release path with `os.SameFile`. The previous path-string comparison reported `unknown` for a valid Windows junction because `filepath.EvalSymlinks` left the junction path unchanged on the actual VM. The replacement still requires a matching immutable metadata hash and a stable VPK file read. A Windows junction regression test and a real VM heartbeat both confirmed `p1-test-v1 / confirmed` after the change.

## Verification

- Local `go test ./... -count=1`, `go vet ./...`, Linux/Windows builds, and `git diff --check`: PASS. The Windows junction regression test ran and passed on Windows. The full Store and HTTP API integration suites passed against random disposable schemas in the authorized development PostgreSQL database; their temporary test binaries and schemas were cleaned.
- The owner selected and authorized the existing local Windows 10 amd64 VM as Node B. Its ordinary runtime account uses an independent Node ID/Secret, fixed d2core v0.1.1 Windows binary and BUILD identity, data-dir and Named Pipe, Controller config, 28100–28109 local port pool, runtime/log directories, Windows n6 300-second template, and content metadata. The official release ZIP SHA256, packaged BUILD identity, `version --json`, and copied VPK SHA256 were verified before admission. The prior Windows addon directory was preserved, and a directory junction binds the verified independent release. No VPK bytes were changed.
- A development database backup preceded migration 10. Node B was registered with its Secret stored in a restricted file outside Git. Its Controller heartbeat reported compatible fixed d2core, hard capacity 1, and `p1-test-v1 / confirmed`; the Platform admitted it only after those facts and the formal TemplateRevision binding were checked.
- Real HTTPS business requests exercised both nodes: auto chose priority Node A and reached Ready; Drain A left that instance running; the next auto request reached Ready on Windows Node B; manual A waited without an Allocation while A was drained and remained waiting after Resume while A was full. After A fully reclaimed, that original manual request reached Ready on A. All three requests then stopped and fully reclaimed. Each d2core `status` directly reported `lifecycle=reclaimed`, `process=stopped`, and `cleanup=complete`; both `list` results were empty. The development database retained distinct immutable Allocation histories, all new NodeJobs succeeded, and both Nodes ended with zero occupied slots. Temporary priority was restored and Drain was cleared.
- The development Platform candidate is healthy through its loopback and trusted HTTPS endpoints after an authorized short restart. An initial start attempt could not append to a legacy root-owned log; a new log owned by its ordinary runtime account restored the process. No existing business rows were reset or deleted.

## Limits

- Node B is on a private VM network. Its identity mapping and private JoinInfo support this controlled local test; public UDP reachability and a human join from the internet are **NOT VERIFIED**. No firewall, router, NAT, or cloud security-group setting was changed. P3E must record unavailable real topologies as **NOT VERIFIED**.
- The Windows d2core manager and Controller were run as the ordinary account in foreground SSH sessions for this real validation. No Windows service or unattended restart mechanism was created. P3F must recheck Windows/Linux Controller recovery and live state in its own substage.
- P3D checkpoint CI must pass before P3E begins.
