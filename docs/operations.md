# V1 operations and recovery

This runbook describes the P5 reference assets and development verification workflow. It does not authorize production changes. Fixed d2core is **v0.1.1**, commit `988720ad85af1f0d97bfe98ec4da4fcbb070beea`; Platform Server never calls it directly.

## Baseline checks

Before maintenance, record the current build SHA, schema migration, `/healthz`, Admin Audit, active ServerRequests, Allocations, open NodeJobs, quarantine, next-game intents, node connectivity, Drain, hard/desired/occupied capacity, Controller and fixed d2core build, direct d2core list, Dota processes, template bindings, ContentRoot links and Controller-reported NodeContentBindings. Do not infer reclamation from a stopped UI card or `quarantined` state. A resource is free only after d2core confirms `reclaimed/stopped/complete`.

Keep deployment credentials, database dumps, VPKs and private player logs outside Git. Controller, d2core and Dota run under the same ordinary node account. On each node, the Controller `network.localPortMin/localPortMax` and d2core manager `--port-min/--port-max` must come from one deployment config; fixed d2core v0.1.1 cannot report the manager's bounds over its local API.

## Database and Platform upgrade

1. Announce a maintenance window and pause new requests. Let active instances end or keep the old Platform build available until they do. Record active jobs and capacity.
2. Take a timestamped PostgreSQL custom-format dump with `deploy/scripts/backup-postgres.sh` into a protected, persistent backup directory. Verify `pg_restore --list` and a restore into a **separate disposable database** before relying on it. Never commit the dump.
3. Build and package Platform binary and Web assets together under a new immutable release directory. Preserve the previous directory and deployment environment file. Do not run from a source tree.
4. Point the `current` symlink at the candidate, run `platform-server migrate` explicitly (the systemd unit also refuses startup if migration fails), then start Platform. Verify migration version, health, player/admin UI, AdminSession, Audit, Party and open resource states. Reconcile every node; never clear an unknown job merely to make health green.
5. For code-only rollback, point `current` back to the previous build and restart after checking schema compatibility. A forward-only migration is **not** undone by replacing a binary. If the old build cannot read the new schema, stop writes, restore the verified pre-upgrade dump into a new database instance, inspect all Node/Allocation side effects independently, then switch the private DB URL to the recovered database. This is a controlled recovery, not `DROP/recreate` of the live DB. Never assume restoring business rows stopped d2core instances.

## Controller update, Linux and Windows

For each node in turn: set Node Drain, wait for all existing Allocations to reach full reclaim, record direct d2core list and processes, replace only the Controller build, restart it, request Controller resync, verify compatible heartbeat and unchanged content/network facts, then Resume. The previous binary remains available for rollback. If a job response is lost, leave its outcome unknown until reconciliation; do not dispatch the same ServerRequest to another node. On Linux use the systemd units in `deploy/systemd/`; on Windows use the two startup tasks and transcripts installed by `deploy/windows/install-node.ps1`.

## d2core or Dota maintenance

The V1 dependency remains fixed at d2core v0.1.1. Any future d2core update requires separate authorization: Drain node, wait for all instances to be fully reclaimed, back up its private data and config, upgrade manually, check build/protocol and direct list, let Controller resync, then Resume. Never update d2core beneath active instances.

Dota/App570 updates are manual and separately authorized. Drain node, let instances end, run SteamCMD update manually, create a local validation instance using the formal TemplateRevision only while drained, test real client entry, explicitly stop and confirm full reclaim, Controller resync, then Resume. Neither Controller nor Content Tool updates Dota.

## ContentVersion and VPK rolling release

The Admin UI has three separate controls that are easy to confuse:

| Admin area | Use it for | What it actually changes |
| --- | --- | --- |
| Content → register map/version/template/preset | Introduce a new Workshop game, immutable VPK version, startup template revision, or player-selectable mode | Platform catalog records only; no file is uploaded or switched. A VPK-only update does not require a new game or template revision. |
| Node → template mapping | Make an existing template revision usable on a specific Node | The Platform's revision-to-Controller binding key. The key and formal template file must already exist in that Node's Controller configuration; the Web page does not create them. A VPK-only update does not change this mapping. |
| Content → publish target version | After Node preparation, matching Controller readback, human entry/content validation and full reclaim | `ArcadeGame.current_content_version_id` for future Allocations only. It does not change Node files or existing Allocations. |

The practical order is register only the needed records, prepare the Node with Content Tool, verify and record the matching content on the Node, then publish the new target and reopen the relevant Node × game admission. The Admin UI cannot perform Content Tool operations or create a local validation instance.

### 管理员实际怎么做：现有地图换一份 VPK

**当前页面若显示节点已上报的版本等于平台目标、此版本已有真人验证记录、且「新分配已允许」，无需点击任何按钮。**「地图分配」卡片只控制指定节点上的指定地图能否接受*新的*服务器；按「暂停此图的新分配」不会停止已有服务器，也不会切换文件。下方的真人验证记录是已完成测试的凭据，不要求每次开服重新记录。Steam/蒸汽平台一键入口的真人确认是另一件事。

下列步骤仅用于**更换 VPK**。先记下地图的 Workshop ID、当前/新 ContentVersion ID、隔离副本 VPK 的 SHA256、目标节点以及对应玩法的正式模板绝对路径。命令中的路径须换成目标节点上实际存在的 ASCII 绝对路径；Linux/Windows 各自在本机执行，不能把本机或另一节点的原始 VPK 路径直接交给节点使用。先为每台节点建立独立隔离副本，核对 SHA256，再操作。VPK-only 更新不必重建地图、玩法、TemplateRevision 或模板映射。

1. **登记**：Admin「地图、玩法与版本」→选择该地图→「登记一份新的 VPK 内容版本」，填写未使用过的版本 ID 和隔离副本的 SHA256。此时平台不会把文件传给节点，也不会改变新开服目标。若只有这一台节点承载此地图，先在「地图设置/玩法设置」暂停新申请；多节点滚动时可让未更新的节点继续接旧版。
2. **封住目标节点的这张图**：Admin「节点」→选中目标节点→「这台节点上的地图」→该地图「暂停此图的新分配」。等待目标节点上这张图的所有旧 Allocation 正常结束，并用 Platform 资源记录与节点上固定 d2core 的 `list/status` 核对完整 `reclaimed/stopped/cleanup=complete`。同节点的另一张图可继续运行，但切换目标图的链接前必须确认没有目标图旧实例。异常隔离、未知或仅 UI 显示结束均不是回收证明。
3. **在目标节点切换内容**：以运行 d2core/Controller 的普通节点账户使用该节点的 Content Tool。`CONTENT_ROOT` 和 `DOTA_ROOT` 分别指向该节点的内容仓库和 Dota 安装根目录；也可在每条命令传 `--content-root ABS --dota-root ABS`。示例命令如下，`<...>` 均须替换成此节点的真实路径/ID，Windows 可执行文件为 `content-tool.exe`：

   ```text
   content-tool --content-root <CONTENT_ROOT> --dota-root <DOTA_ROOT> prepare <WorkshopID> <新版本ID> <隔离副本VPK绝对路径>
   content-tool --content-root <CONTENT_ROOT> --dota-root <DOTA_ROOT> switch <WorkshopID> <新版本ID>
   content-tool --content-root <CONTENT_ROOT> --dota-root <DOTA_ROOT> status <WorkshopID>
   ```

   `prepare` 将隔离副本复制为不可变 release；核对输出 SHA256 与后台登记值一致。`switch` 才切换整个 addon 目录链接和 `metadata/current.json`；`status` 应显示新版本及一致的链接/元数据。不要在旧实例运行时切换、原地覆盖 VPK 或修改旧 release。
4. **核对节点事实**：Admin「调度与容量」→「组件版本、核对与历史任务」→「请求完整核对」。等待 Controller 上报本卡片的版本、状态「已确认」、SHA256 与登记值一致。后台只能读取这项事实，不能代节点填写。若未确认，停在这里排查 Content Tool status、Controller 的本地路径配置和心跳。
5. **临时测试窗口**：Admin「调度与容量」→「进入节点维护」，确认*整台节点*占用为 0，并核对 d2core `list` 和待处理任务；Node Drain 不会自动停止已有实例。仅在 Drain 中，以同一普通账户、固定 d2core v0.1.1、正式玩法模板进行维护实例测试。以下命令只展示固定 CLI 的调用形状，不可照抄占位路径；使用与正在运行的 manager 相同的 data-dir，给此次新测试生成**唯一** idempotency key，不要启动第二个 manager：

   ```text
   d2core check --template <正式模板绝对路径> --json
   d2core create --template <正式模板绝对路径> --idempotency-key <本次唯一键> --data-dir <现有d2core数据绝对路径> --json
   d2core operation <create返回的operationId> --data-dir <同一数据目录> --json
   d2core status <create返回的instanceId> --data-dir <同一数据目录> --json
   ```

   等正式 Ready（包括 Steam 登录条件），用真实 Dota 客户端进入并验证玩法。测试结束后，显式执行 `d2core stop <instanceId> --data-dir <同一数据目录> --json`，轮询 stop 返回的 operation，并用 `status` 核对 `reclaimed/stopped/cleanup=complete`、`list` 核对没有遗留实例。不要将 CLI 超时理解为没有创建；沿原 key/instance 对账，不能换 key 重建。再从 Admin 请求 Controller 完整核对，确认临时实例已消失/已完整回收。
6. **留证与开放**：保持 Drain 且占用 0 时，在该地图卡片点「记录已完成的真人验证」并完成二次确认；它只写平台验证凭据。然后「结束节点维护」。Admin「地图、玩法与版本」选择新版本，点击「切换新开服目标」；再回节点卡片「恢复此图的新分配」。单节点时最后恢复地图/玩法的新申请。发布只影响之后创建的 Allocation，已有 Allocation 不改版。

**两节点滚动**：先按 2–5 步更新 A，A 的地图分配开关仍保持关闭；在 A 完成验证并结束 Drain 后发布平台新目标，再打开 A 的该图新分配。B 此时仍报告旧版，版本不匹配，因此不会收到新版新 Allocation；B 上既有旧实例继续到自然结束。再按 2–6 步更新 B，B 不需再次发布同一个平台目标。不要为了“保持两台一致”在 B 有旧实例时热切换。整个过程的完整技术约束见 [V1 §21](specs/v1.md)。

**失败恢复**：保留该节点此图的分配暂停。先 `content-tool ... status <WorkshopID>` 检查链接与元数据；按工具的显式重试/`rollback <WorkshopID>` 路径恢复，重新核对 Controller 上报与对应版本真人验证。不要仅把后台目标切回旧 ID 就推断节点磁盘已回滚，也不要把 `quarantined` 当作完整回收。

Content Tool is offline: `content-tool status <WorkshopID>`, `prepare <WorkshopID> <version> <source-vpk>`, `switch <WorkshopID> <version>`, `rollback <WorkshopID>`. Pass absolute ASCII `CONTENT_ROOT` and `DOTA_ROOT` (or flags). `prepare` copies an immutable VPK into `<ContentRoot>/<WorkshopID>/releases/<version>/pak01_dir.vpk`, stores SHA256/size metadata outside the release, and does not touch the Dota link. `switch` updates the whole addon directory link/Junction and `metadata/current.json`; Controller readback must confirm it. An interrupted operation leaves a transition record that the next explicit `switch`/`rollback` recovers. A leftover lock requires an operator to confirm the old process is gone before manually removing only that lock. Keep old release directories for rollback; never overwrite an old version in place.

For a one-time migration of a pre-Content-Tool addon link, first Drain and prove no unreclaimed Allocation or d2core instance. Copy the currently linked VPK into isolated staging, verify its known SHA256, and `prepare` that copy under its existing ContentVersion ID. If the old Junction already targets the prepared release, verify that exact target before adopting the prepared version metadata as `metadata/current.json`; `status` must confirm link and metadata agree. If the old symlink targets an unversioned directory, preserve the symlink outside the addon tree on the same filesystem and use `switch` to the SHA-identical prepared release; retain the old target and backup symlink. Point the Controller binding at Content Tool's `metadata/current.json`, restart it and require confirmed digest readback. These are migration-only steps; later releases use `prepare`/`switch`/`rollback` without manual metadata adoption. Do not pass a live source VPK directly to `prepare` or overwrite the old release.

For a single content-bearing node: pause ArcadeGame/GamePreset requests and waiting allocation, set that NodeContentBinding `accepting=false`, wait for relevant Allocations to fully reclaim, `prepare`, `switch`, wait for Controller reported target version, briefly Node Drain, use d2core official local CLI/client for a temporary formal-template validation instance, have a human validate content and entry, stop and verify `reclaimed/stopped/complete`, request Controller resync and confirm the temporary instance is absent, record the human validation in Admin, Resume, publish `current_content_version_id`, set binding `accepting=true`, then lift content maintenance. Content Tool never performs Drain or Platform publication.

For two nodes, update A first with binding `accepting=false`; after full content drain and local validation/resync, Resume A while its binding stays closed. While global current is still old, A's new reported version does not match and receives no old-version Allocation. Publish global current only after A is ready, then open A's binding. New Allocations go only to matching A. Then update B with `accepting=false`, preserving any active old-version instance until normal reclaim. Validate/resync B and set `accepting=true`. The old release remains available for rollback. No same-node active old/new content coexistence is promised.

If a content switch fails, keep binding `accepting=false` and Node Drain as needed. Read `status`, inspect pending metadata and links, retry the explicit switch to recover, or `rollback` to the previous known release. Controller must again report the rolled-back version and local human validation must pass before publishing the corresponding Platform target or resuming allocations. Never manually copy over the current VPK.

## Entry validation and small human trial

`a2s_enabled` is the Node's deployment fact. The Controller checks each actual Ready instance separately; Admin Allocation details show its local port, `ok/failed` and last check. No Ready instance means no query fact, not a failure. Legacy aggregate `a2s_query_ok` is deprecated/derived and must not be used for entry/node health, player URI, connect, capacity or scheduling. Under [V1.0 Amendment 001](specs/v1-amendment-001-entry-verification.md), for each Node and independently for Steam and steamchina, a trusted administrator confirms that a real client used the scheme URI to enter one real Ready instance under the current `entry_config_revision`. Admin requires explicit second confirmation, then records `verified_at`, `verified_by` and Audit. Within the same revision `verified` remains true; if an entry misbehaves, turn `enabled=false`, retain the attestation, and restore `enabled=true` after repair. Changing protocol IP, local/public mappings or other URI/A2S deployment configuration changes the revision and clears both schemes' verification and enabled flags. Routine Platform/Controller restarts, Web updates, content changes and instance lifecycle do not require reverification when entry network facts remain unchanged. No port-pool A2S or per-port human coverage is required. The always available fallback after Ready and valid JoinInfo is `connect <host>:<actual-public-port>`.

For a small multiplayer trial, use at least two real people: create or join a Party through the Web, choose Game/Preset and node mode, request, wait for Ready/JoinInfo, join the same real Dota server, play, then normal stop or next game and confirm full reclaim and Party persistence. Record `requested_at`, `assigned_at`, `create_started_at`, `ready_at` and `join_info_available_at` for natural trials. Browser sessions or bots do not count as real people. Do not force faults during their game.
