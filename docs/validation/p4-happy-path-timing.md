# P4 no-queue happy-path timing recheck

Date: 2026-09-26. Baseline: `origin/main` and local HEAD were both `913b161996ab09cad15a270ddeb775dbd17e3986` before testing. This is a diagnostic P4 follow-up, not P4 closure or a performance change.

## Method and safety

- The authorized development Platform, PostgreSQL, fixed d2core v0.1.1, existing `P1 测试游廊` / `n6` content and both existing Nodes were used. No content, host, network, database schema or service configuration changed.
- Before testing, both Nodes were compatible, online, not Draining, hard/desired/occupied `1/1/0`; there were no blocking Requests, occupied Allocations, open Jobs or d2core instances. Each request had free capacity before submission. Four samples used isolated player API Sessions and three used the same deployed Web bundle against the real development API through a local loopback mirror. Samples ran sequentially. Every instance was stopped through the normal player route and reached full `reclaimed/stopped/complete`; each Node returned to occupied `0` before the next test. The first sample's local timestamp parser failed after the instance was already Ready; its `finally` path stopped and fully reclaimed it, and the immutable Platform timestamps were read back from PostgreSQL. This was a measurement script error, not a Platform or d2core failure.
- All five business timepoints below come from the Platform's UTC timestamps, not the browser or cross-host subtraction. The browser display observation is a separate, approximate measurement. `queue_wait` includes scheduler admission time but no prior server occupying the selected slot.

## Exact Platform timestamps

All timestamps in this table are on **2026-09-26 UTC**. Row numbers identify the seven test Requests in order; full Request IDs are kept in the ignored local measurement record and the development database, not copied into this validation document.

| # | Node / OS | Mode | `requested_at` | `assigned_at` | `create_started_at` | `ready_at` | `join_info_available_at` |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 1 | p0d-linux-cn / Linux | manual | 07:30:15.228030Z | 07:30:15.266173Z | 07:30:19.190312Z | 07:31:14.129700Z | 07:31:14.129700Z |
| 2 | p3d-win10-vm / Windows | manual | 07:32:09.639466Z | 07:32:11.266391Z | 07:32:15.879177Z | 07:32:30.794050Z | 07:32:30.794050Z |
| 3 | p0d-linux-cn / Linux | manual | 07:32:51.533891Z | 07:32:53.266566Z | 07:32:54.182762Z | 07:33:14.125805Z | 07:33:14.125805Z |
| 4 | p3d-win10-vm / Windows | manual | 07:33:40.551056Z | 07:33:41.264778Z | 07:33:45.876576Z | 07:34:00.830800Z | 07:34:00.830800Z |
| 5 | p3d-win10-vm / Windows | auto | 07:37:14.816976Z | 07:37:15.264886Z | 07:37:15.874286Z | 07:37:30.827736Z | 07:37:30.827736Z |
| 6 | p0d-linux-cn / Linux | manual | 07:39:17.493918Z | 07:39:19.265069Z | 07:39:24.174169Z | 07:39:49.116224Z | 07:39:49.116224Z |
| 7 | p3d-win10-vm / Windows | manual | 07:41:08.311750Z | 07:41:09.265919Z | 07:41:10.868117Z | 07:42:05.786588Z | 07:42:05.786588Z |

## Intervals and job evidence

Seconds. `ready_to_joininfo_delay = join_info_available_at - ready_at`. All seven had capacity `hard/desired/occupied = 1/1/0` before create, `1/1/1` while Ready, and `1/1/0` after full reclaim.

| # | `queue_wait` | `dispatch_and_start_delay` | `dota_startup_time` | `ready_to_joininfo_delay` | `time_to_join` | Create Job claim wait | Claim → prepare |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 1 | 0.038 | 3.924 | 54.939 | 0.000 | 58.902 | 3.886 | 0.039 |
| 2 | 1.627 | 4.613 | 14.915 | 0.000 | 21.155 | 4.535 | 0.078 |
| 3 | 1.733 | 0.916 | 19.943 | 0.000 | 22.592 | 0.878 | 0.039 |
| 4 | 0.714 | 4.612 | 14.954 | 0.000 | 20.280 | 4.531 | 0.082 |
| 5 | 0.448 | 0.609 | 14.953 | 0.000 | 16.011 | 0.533 | 0.078 |
| 6 | 1.771 | 4.909 | 24.942 | 0.000 | 31.622 | 4.873 | 0.038 |
| 7 | 0.954 | 1.602 | 54.918 | 0.000 | 57.475 | 1.528 | 0.075 |

Every row has exactly one successful create Job, two normal create reports, and one successful stop Job. The Node Controller runs a five-second heartbeat/worker tick; the measured `job_created_at → claimed_at` range (0.533–4.873 s) fits waiting for that normal tick. No additional full claim cycle, duplicate create Job, unknown Job, or unexpected Allocation attempt was observed. Routine active-Allocation reconcile facts were recorded; the explicit reconcile-request/completed generations did not change (Linux `2/2`, Windows `0/0`). There was no recovery-triggered reconcile or exceptional retry. The two approximately 55-second samples had no structured d2core error and both reached normal Ready and full reclaim.

## Browser display observation

The exact deployed Web JS/CSS bundle was served from a temporary loopback origin with API calls forwarded over verified HTTPS to the real development Platform. The loopback mirror changed only the local browser cookie's `Secure` attribute so the same-origin HTTP test could hold its anonymous Session; it did not change application code or the development server. Direct in-app browser access to the development domain timed out during this measurement, so direct-domain rendering latency remains **NOT VERIFIED**.

The Web polls current request/allocation state every 2.5 seconds. Row 7 was observed continuously, approximately every 0.1 seconds: the page first displayed `可以进入` at workstation UTC 07:42:15.548, with the last non-ready observation at 07:42:15.439. The workstation clock was about 8.84 seconds ahead of the Platform clock by same-request submission comparison; after that approximate offset, the displayed state followed backend `join_info_available_at` by roughly **one second**. Click/network time and clock calibration make this an approximate UI observation, not a formal cross-host metric. Rows 5–6 visibly reached `可以进入`, but gaps between their browser observations prevent a reliable first-display delay. Rows 1–4 were API-driven and had no browser display observation. This evidence does not indicate a large Web-only delay; it does not measure the direct development domain end to end.

## Descriptive comparison with P1

P1E's two no-queue human samples on Linux had `time_to_join` **18.042 s** and **17.195 s**, with `dota_startup_time` **14.969 s** and **14.974 s**. Three current Windows samples had essentially the same startup stage (**14.915–14.954 s**) and total **16.011–21.155 s**; their spread mostly follows the normal 0.5–4.5-second NodeJob claim wait. Current Linux samples were **19.943 s**, **24.942 s**, and **54.939 s** in the startup/Ready stage, and one current Windows sample was **54.918 s**. The slow totals are therefore localized to `create_started_at → ready_at`, not queue admission, dispatch after claim, Ready → JoinInfo, or Web rendering. This interval includes d2core create, Dota process startup, Ready detection and the Controller's trusted report; these seven samples cannot identify which internal part produced the two long waits. The exact underlying Dota/d2core cause is **NOT VERIFIED**. There is **no clear Platform-stage performance regression** in these samples, while real startup/Ready time was more variable than the two P1E samples. No performance optimization or large refactor was made. These small, mixed-Node samples are not a formal SLO, P50/P95 or ETA.

## Final environment

After all seven normal stop/full reclaim operations: development `/healthz` returned 200; both Nodes had fresh heartbeats, Drain false, hard/desired/occupied `1/1/0`; active Requests, occupied Allocations, open/unknown Jobs and pending/paused next-game intents were all zero. Both fixed d2core lists and both Dota process counts were zero. Historical ended/reclaimed Requests, Allocations, Jobs and audit data were retained. P4 UI owner acceptance and P4 closure remain pending; P5 was not started.
