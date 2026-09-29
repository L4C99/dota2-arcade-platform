# V1 Production 运维与恢复；v1.0.2 I3 候选流程

本文是当前 V1/v1.0.1 Production 的运维与恢复 runbook。正式版本状态见[根 README](../README.md)与 [v1.0.1 Release](https://github.com/L4C99/dota2-arcade-platform/releases/tag/v1.0.1)。固定 d2core 为 **v0.1.1**，commit `988720ad85af1f0d97bfe98ec4da4fcbb070beea`；Platform Server 不直接调用它。实际维护、升级和故障恢复须有对应操作授权，并按本节的安全边界执行。

## v1.0.2 I3 候选：Node × Game 日常内容/模板维护

此节是 I3 候选源码的操作合同，**尚未部署 Production**。下面的旧 `content.validate/content.publish` 与整 Node Drain 步骤只适用于仍合格的 `legacy_v1` 游戏；已进入 v1.0.2 合同的游戏必须使用本节。Dota、d2core、Controller binary、OS 等整机维护仍走 Node Drain。

1. Admin 选择目标 Node 和 ArcadeGame，提交 `maintenance.begin`，携 `expectedMaintenanceEpoch/requestId/reason`。Platform 在同一事务关闭该 Node × Game binding 并增加 epoch；不会 Drain 全节点、停止已有实例或操作其它游戏。检查 scoped status 的 `targetOccupied=0`、`targetUnresolvedJobs=0`，异常隔离仍计占用。不得在旧实例活动时切换目标游戏内容链接。
2. 运维在节点本地显式执行 Content Tool 的 `prepare/switch/status` 或准备版本化模板与依赖。Web 不上传文件，也不远程执行命令。等待 Controller 报告目标 ContentVersion、真实 VPK SHA、模板 manifest 指纹及 binding generation；inventory 须完整、新鲜、`unaccounted=0`。Admin 的期望指纹与 Controller 事实分别记录，不能互相代填。`reconcileCompleted` 只说明完成核对循环，不能替代 machine proof。
3. 每个拟开放的 Preset 分别 `validation.start`。Platform 按目标 Node、Preset、候选内容与模板、维护 epoch、容量、scoped drain 和当前机器事实原子建立 ValidationRun、Validation Allocation、create Job；重试同一 `requestId` 返回原 Run。等 Ready 与有效 JoinInfo，在 Admin 详情读取 connect，项目 Owner 用真人客户端进服验玩法，提交 `validation.confirm pass|fail`。确认 pass 只是人工声明；正式 stop Job、完整 reclaim、所有 Job 终结、事实继续有效后，服务端才可把 Run 终结为 PASS。启动失败但有可信 instance、无 JoinInfo 或中止时用 `validation.stop`；不输入任意 instance ID。未知且无可信 ID 时只对账或隔离，不猜测 stop。
4. `release.publish` 提交 expected-old 内容指针与逐 Preset 旧/新模板、旧/新接受状态和当前 PASS Run ID。首次新合同发布以及任何内容指针变化必须列出全部现存 Preset；无证明的玩法保持暂停。成功一次事务写 immutable release、逐玩法记录、正式指针与 Audit。后续同内容的模板单独发布可只列目标 Preset。发布前证明 Node binding 保持关闭，发布后按当前证明分别重新开放 Node binding 和 Preset。waiting Request 在真正分配时读取正式组合；旧 Allocation 快照不改。
5. 回退先在节点本地真实恢复旧 bytes，等 Controller 报告旧内容 SHA/模板指纹，在本次维护 epoch 重新逐玩法取得 PASS，再用新的 `release.publish` 和 `rollbackOfReleaseId`；旧 release 不改。不能用旧 epoch PASS 或直接改数据库指针。

玩家调度对 `v1_0_2` Preset 每次在同一 Allocation 事务重新查 Node × Preset × 当前内容 × 当前模板的有效 PASS，以及 epoch、内容/模板事实代次、inventory、兼容性与心跳。绑定开放也不能绕过该硬门槛。漂移只阻止**新的** Allocation，不自动停止已有实例。普通路径只查 Platform PostgreSQL 持久事实，不同步等待 Controller RPC。

### 无可信 ID 的 unknown create 与节点退出调度

持久 operation-start marker 可能已提交，但真实 core call 未发生，随后固定 d2core 的 history 过期。此时空 `list`、等待、重启或重复拒绝均不能证明 no-effect；保留原 Job、Allocation、容量与端口。Admin 可用 `node.reconcile` 继续追原 key；必要时 `allocation.quarantine`，让 Owner/Party 按既有 escape 放弃异常请求并新建申请。**仅有可信 instance ID** 才可建立 formal stop，且须完整 reclaim 才释放。无法收敛时 Node Drain、desired capacity 0，按独立授权的节点退役流程处理；不得添加 force no-effect/force release 按钮，也不得换 Node ID 伪造空容量。

部署前核对 `parent(d2coreDataDir)` 已存在、由 operator 控制、service account 可写；Linux 不得 world-writable。同一个实际 d2core data directory 只能由一个 NodeID/Controller 持有，不能以两个逻辑 Node 复用同一状态目录。此检查是部署门槛，不改变当前 ownership lock 代码。

## 操作前基线核对

维护前记录当前构建 SHA、schema migration、`/healthz`、Admin Audit、活动 ServerRequest、Allocation、open NodeJob、quarantine、next-game intent、节点连通性和 Drain、hard/desired/occupied 容量、Controller 与固定 d2core 构建身份、d2core `list`、Dota 进程、模板绑定、ContentRoot 链接以及 Controller 上报的 NodeContentBinding。不能根据 UI 已结束或 `quarantined` 推断实例已回收；只有 d2core 确认 `reclaimed/stopped/complete` 才释放容量。

部署凭据、数据库备份、VPK 和私有玩家日志须留在 Git 外。Controller、d2core 和 Dota 使用同一普通节点账户。每个节点的 Controller `network.localPortMin/localPortMax` 与 d2core manager `--port-min/--port-max` 必须来自同一部署配置；固定 d2core v0.1.1 的本地 API 不能回报 manager 的端口边界。

## 数据库与 Platform 升级

1. 公告维护窗口并暂停新申请；等待活动实例结束，或保留旧版 Platform 直至其结束。记录 open Job 与容量。
2. 用 `deploy/scripts/backup-postgres.sh` 将带时间戳的 PostgreSQL custom-format 备份写入受保护的持久目录。依次核对 `pg_restore --list` 和**独立临时数据库**的恢复结果；备份不得提交 Git。
3. 把 Platform binary 与 Web 资源配对打包到新的不可变版本目录。保留上一版本目录和私有部署环境文件；不要从源码树运行。
4. 核对包的 SHA256、构建身份与配对 Web bundle。维护窗口中停止 Platform 写入，在私有环境下显式运行候选版本的 `platform-server migrate`。成功后才原子切换 `current` 并启动 Platform；systemd 会重复执行幂等 migration，失败则拒绝启动。核对本机和公网 HTTPS health、玩家/管理员页面、AdminSession、Audit、Party 及 open 资源；逐节点 reconcile，不能为使 health 变绿而清除 unknown Job。
5. 仅代码回滚前先确认旧版与当前 schema 兼容，再将 `current` 指回旧版并重启。替换 binary **不会撤销**前向 migration。旧版无法读取新 schema 时，应停止写入，将已验证的升级前备份恢复到新的数据库实例，独立检查 Node/Allocation 副作用，再切换私有 DB URL。这是受控恢复，不能对现有数据库做 `DROP/recreate`；恢复业务行也不代表 d2core 实例停止。

## Controller 升级与回滚（Linux / Windows）

逐节点执行：先 Node Drain，等待完整回收后确认 occupied=0、open NodeJob=0、d2core `list` 为空。保留旧 Controller binary 与私有配置，替换前核对新版身份和配置。重启后要求兼容心跳，请求 Controller reconcile，并核对 Allocation 身份、ContentVersion、Entry revision/verified/enabled 及内容、入口、网络绑定事实均未意外变化，然后 Resume。Controller 回滚也遵守同样的 Drain、空实例、reconcile 边界。Job 响应丢失时保持 `unknown` 直到对账，不能把同一 ServerRequest 改派到别的节点。Linux 使用 `deploy/systemd/` 的 unit；Windows 使用 `deploy/windows/install-node.ps1` 安装的启动任务与 transcript，并在目标环境核验无人值守启动/重启。

### v1.0.1 create-dispatch 历史验收（已完成）

该验收已完成，原 Candidate 的步骤与判据保留在[历史验证记录](validation/v1.0.1-create-dispatch.md)。当时的 Production Before 观察为：独立 A 的 create 保持 accepted 期间，B 从 Job 创建到 claim 等待 173.578 秒；A 终结到 B claim 约 5.068 秒。After 验收以 `B.create_started_at < A.create Job terminal report time` 为关键判据，另核对两个独立实例的 Ready/JoinInfo、分别完整回收，以及 Controller 重启后沿原 instance/operation 身份恢复。私有观察与验收明细保存在仓库外。本段是历史验收背景，不是每次日常升级必须重跑的 Production 故障注入步骤。

## d2core 与 Dota 维护

V1 依赖固定为 d2core v0.1.1。未来升级 d2core 须单独授权：Drain Node，等待所有实例完整回收，备份私有数据与配置，人工升级，核对 build/protocol 与 `list`，让 Controller resync，最后 Resume。不能在活动实例下替换 d2core。

Dota/App570 更新也须单独授权并人工执行：Drain Node，等待实例结束，人工执行 SteamCMD 更新；仅在 Drain 期间用正式 TemplateRevision 创建本地验证实例，以真实客户端进服测试，显式 stop 并确认完整回收，再 Controller resync、Resume。Controller 和 Content Tool 都不会更新 Dota。

## legacy_v1 的 ContentVersion 与 VPK 滚动发布（历史旧合同）

以下步骤保留给仍合格的旧合同游戏。Admin 内容页把 **接入新地图** 与 **更新现有地图 VPK** 放在「旧合同 legacy_v1 内容流程与登记工具」折叠区；其节点级验收提示不构成 v1.0.2 ValidationRun。新合同的正式逐玩法流程在内容页主区域。旧页面的 **现有地图管理** 显示目标版本和 admission；其它对象控制仍在 **高级管理 · 手动管理平台记录**。旧控制含义：

| Admin area | Use it for | What it actually changes |
| --- | --- | --- |
| Content → register map/version/template/preset | Introduce a new Workshop game, immutable VPK version, startup template revision, or player-selectable mode | Platform catalog records only; no file is uploaded or switched. A VPK-only update does not require a new game or template revision. |
| Node → template mapping | Make an existing template revision usable on a specific Node | The Platform's revision-to-Controller binding key. The key and formal template file must already exist in that Node's Controller configuration; the Web page does not create them. A VPK-only update does not change this mapping. |
| Content → publish target version | After Node preparation, matching Controller readback, human entry/content validation and full reclaim | `ArcadeGame.current_content_version_id` for future Allocations only. It does not change Node files or existing Allocations. |

The practical order is register only the needed records, prepare the Node with Content Tool, verify and record the matching content on the Node, then publish the new target and reopen the relevant Node × game admission. The Admin UI cannot perform Content Tool operations or create a local validation instance.

### 首次接入一张新地图时

这与下文的「现有地图换 VPK」不同：新 Workshop ID 尚无游戏、版本、玩法和节点模板映射记录。一次完整接入按如下顺序完成；已经存在的记录不要重复创建。

1. 在运维电脑上取得地图 VPK 和该地图各玩法的正式启动模板。把 VPK 分别复制到每台目标节点的**隔离暂存位置**，逐份计算 SHA256；保留原始文件，不让 Content Tool 直接依赖会变化的来源文件。确定临时名称、Workshop ID、不可变 ContentVersion ID、各 TemplateRevision ID、玩法人数上限及每节点本地 Controller 模板绑定键。正式对外名称以后可单独调整。
2. Admin「地图、玩法与版本」登记游戏、VPK 版本及 SHA256、必要的启动模板修订和玩法预设。新游戏/玩法保持停用与暂停申请。一个玩法对应一份正式模板修订；若不同玩法实际使用不同启动文件，不要把它们都映射到同一模板。这里只写平台目录记录，不上传 VPK、模板文件，也不启动服务器。
3. 逐台准备节点：在目标节点以普通运行账户，用 Content Tool 对隔离副本执行 `prepare`、`switch`、`status`，命令形式与下文第 3 步相同。先配置该节点 Controller 对新 Workshop ID 的 `metadataPath=<ContentRoot>/<WorkshopID>/metadata/current.json`、`currentLinkPath=<DotaRoot>/game/dota_addons/<WorkshopID>`，使它能报告真实版本和 SHA256；在节点本地准备、检查每种玩法的正式模板文件与 Controller 绑定键。Admin「节点」→「让此节点找到玩法的启动模板」将各平台 TemplateRevision ID 绑定到**此节点已有的** Controller 键。Web 保存映射不会在节点创建文件或键。按部署流程重启/核对 Controller，要求兼容心跳、该地图 `reported_state=confirmed`、版本和 SHA256 都与登记值一致。
4. 在该节点进入 Node Drain，确认整台节点占用 0；按下文第 5 步用固定 d2core v0.1.1 的正式模板逐一创建本地临时验证实例，真人进入并测试每种要开放的玩法。每局都显式 stop，确认 `reclaimed/stopped/cleanup=complete` 和 `list` 无残留，再测试下一种。请求 Controller resync 并确认临时实例已消失。仍在 Drain 且占用 0 时，Admin「接入新地图」对该节点确认**内容版本验收**；结束 Node Drain。逐玩法勾选仅是当前浏览器提示，不是独立的持久验收记录；正式记录仍为 Node + ArcadeGame + ContentVersion。
5. 至少一台可用节点完成验收并恢复后，Admin「接入新地图」将新版本**发布为新开服版本**。先只打开确已完成验收的节点地图分配开关，再启用经过真人测试的游戏及玩法申请。用普通玩家网页发起一次真实开服，确认 Allocation 快照版本、Ready/JoinInfo、实际玩法和最终完整回收。其他节点逐台执行 3–4 步，再单独打开其地图分配；发布后的目标版本不会替节点切换磁盘。

新地图接入与 VPK 更新都必须遵守 [V1 §21 的 Node Drain 临时实例边界](specs/v1.md)。Steam/蒸汽平台 URI 验证按当前入口网络修订单独管理，不因新增地图自动获得真人入口确认，也不因内容变化自动失效既有网络修订下的入口确认。

### 管理员实际怎么做：现有地图换一份 VPK

**当前页面若显示节点已上报的版本等于平台当前发布版本、此版本已有内容版本验收记录、且「新分配已允许」，无需点击任何按钮。**「地图分配」卡片只控制指定节点上的指定地图能否接受*新的*服务器；按「暂停此图的新分配」不会停止已有服务器，也不会切换文件。内容版本验收记录不要求每次开服重新确认。Steam/蒸汽平台一键入口的真人确认是另一件事。

下列步骤仅用于**更换 VPK**。先记下地图的 Workshop ID、当前/新 ContentVersion ID、隔离副本 VPK 的 SHA256、目标节点以及对应玩法的正式模板绝对路径。命令中的路径须换成目标节点上实际存在的 ASCII 绝对路径；Linux/Windows 各自在本机执行，不能把本机或另一节点的原始 VPK 路径直接交给节点使用。先为每台节点建立独立隔离副本，核对 SHA256，再操作。VPK-only 更新不必重建地图、玩法、TemplateRevision 或模板映射。

1. **登记**：Admin「地图、玩法与版本」→「更新现有地图 VPK」→选择地图，填写未使用过的版本 ID 和隔离副本的 SHA256。此时平台不会把文件传给节点，也不会改变当前发布版本。若只有这一台节点承载此地图，先在高级管理的「地图设置/玩法设置」暂停新申请；多节点滚动时可让未更新的节点继续接旧版。
2. **封住目标节点的这张图**：Admin「节点」→选中目标节点→「这台节点上的地图」→该地图「暂停此图的新分配」。等待目标节点上这张图的所有旧 Allocation 正常结束，并用 Platform 资源记录与节点上固定 d2core 的 `list/status` 核对完整 `reclaimed/stopped/cleanup=complete`。同节点的另一张图可继续运行，但切换目标图的链接前必须确认没有目标图旧实例。异常隔离、未知或仅 UI 显示结束均不是回收证明。
3. **在目标节点切换内容**：以运行 d2core/Controller 的普通节点账户使用该节点的 Content Tool。`CONTENT_ROOT` 和 `DOTA_ROOT` 分别指向该节点的内容仓库和 Dota 安装根目录；也可在每条命令传 `--content-root ABS --dota-root ABS`。示例命令如下，`<...>` 均须替换成此节点的真实路径/ID，Windows 可执行文件为 `content-tool.exe`：

   ```text
   content-tool --content-root <CONTENT_ROOT> --dota-root <DOTA_ROOT> prepare <WorkshopID> <新版本ID> <隔离副本VPK绝对路径>
   content-tool --content-root <CONTENT_ROOT> --dota-root <DOTA_ROOT> switch <WorkshopID> <新版本ID>
   content-tool --content-root <CONTENT_ROOT> --dota-root <DOTA_ROOT> status <WorkshopID>
   ```

   `prepare` 将隔离副本复制为不可变 release；核对输出 SHA256 与后台登记值一致。`switch` 才切换整个 addon 目录链接和 `metadata/current.json`；`status` 应显示新版本及一致的链接/元数据。不要在旧实例运行时切换、原地覆盖 VPK 或修改旧 release。
4. **核对节点事实**：Admin「调度与容量」→「组件版本与核对」→「请求完整核对」。等待 Controller 上报本卡片的版本、状态「已确认」、SHA256 与登记值一致。后台只能读取这项事实，不能代节点填写。若未确认，停在这里排查 Content Tool status、Controller 的本地路径配置和心跳。
5. **临时测试窗口**：Admin「调度与容量」→「进入节点维护」，确认*整台节点*占用为 0，并核对 d2core `list` 和待处理任务；Node Drain 不会自动停止已有实例。仅在 Drain 中，以同一普通账户、固定 d2core v0.1.1、正式玩法模板进行维护实例测试。以下命令只展示固定 CLI 的调用形状，不可照抄占位路径；使用与正在运行的 manager 相同的 data-dir，给此次新测试生成**唯一** idempotency key，不要启动第二个 manager：

   ```text
   d2core check --template <正式模板绝对路径> --json
   d2core create --template <正式模板绝对路径> --idempotency-key <本次唯一键> --data-dir <现有d2core数据绝对路径> --json
   d2core operation <create返回的operationId> --data-dir <同一数据目录> --json
   d2core status <create返回的instanceId> --data-dir <同一数据目录> --json
   ```

   等正式 Ready（包括 Steam 登录条件），用真实 Dota 客户端进入并验证玩法。测试结束后，显式执行 `d2core stop <instanceId> --data-dir <同一数据目录> --json`，轮询 stop 返回的 operation，并用 `status` 核对 `reclaimed/stopped/cleanup=complete`、`list` 核对没有遗留实例。不要将 CLI 超时理解为没有创建；沿原 key/instance 对账，不能换 key 重建。再从 Admin 请求 Controller 完整核对，确认临时实例已消失/已完整回收。
6. **验收与开放**：保持 Drain 且占用 0 时，在该地图的任务卡片点「确认内容版本验收」并完成二次确认；它只写平台验收记录。然后「结束节点维护」。Admin「地图、玩法与版本」点击「发布为新开服版本」，再打开该节点此图的新分配。单节点时最后恢复地图/玩法的新申请。发布只影响之后创建的 Allocation，已有 Allocation 不改版。

**两节点滚动**：先按 2–5 步更新 A，A 的地图分配开关仍保持关闭；在 A 完成验证并结束 Drain 后发布平台新目标，再打开 A 的该图新分配。B 此时仍报告旧版，版本不匹配，因此不会收到新版新 Allocation；B 上既有旧实例继续到自然结束。再按 2–6 步更新 B，B 不需再次发布同一个平台目标。不要为了“保持两台一致”在 B 有旧实例时热切换。整个过程的完整技术约束见 [V1 §21](specs/v1.md)。

**失败恢复**：保留该节点此图的分配暂停。先 `content-tool ... status <WorkshopID>` 检查链接与元数据；按工具的显式重试/`rollback <WorkshopID>` 路径恢复，重新核对 Controller 上报与对应版本真人验证。不要仅把后台目标切回旧 ID 就推断节点磁盘已回滚，也不要把 `quarantined` 当作完整回收。

Content Tool is offline: `content-tool status <WorkshopID>`, `prepare <WorkshopID> <version> <source-vpk>`, `switch <WorkshopID> <version>`, `rollback <WorkshopID>`. Pass absolute ASCII `CONTENT_ROOT` and `DOTA_ROOT` (or flags). `prepare` copies an immutable VPK into `<ContentRoot>/<WorkshopID>/releases/<version>/pak01_dir.vpk`, stores SHA256/size metadata outside the release, and does not touch the Dota link. `switch` updates the whole addon directory link/Junction and `metadata/current.json`; Controller readback must confirm it. An interrupted operation leaves a transition record that the next explicit `switch`/`rollback` recovers. A leftover lock requires an operator to confirm the old process is gone before manually removing only that lock. Keep old release directories for rollback; never overwrite an old version in place.

For a one-time migration of a pre-Content-Tool addon link, first Drain and prove no unreclaimed Allocation or d2core instance. Copy the currently linked VPK into isolated staging, verify its known SHA256, and `prepare` that copy under its existing ContentVersion ID. If the old Junction already targets the prepared release, verify that exact target before adopting the prepared version metadata as `metadata/current.json`; `status` must confirm link and metadata agree. If the old symlink targets an unversioned directory, preserve the symlink outside the addon tree on the same filesystem and use `switch` to the SHA-identical prepared release; retain the old target and backup symlink. Point the Controller binding at Content Tool's `metadata/current.json`, restart it and require confirmed digest readback. These are migration-only steps; later releases use `prepare`/`switch`/`rollback` without manual metadata adoption. Do not pass a live source VPK directly to `prepare` or overwrite the old release.

For a single content-bearing node: pause ArcadeGame/GamePreset requests and waiting allocation, set that NodeContentBinding `accepting=false`, wait for relevant Allocations to fully reclaim, `prepare`, `switch`, wait for Controller reported target version, briefly Node Drain, use d2core official local CLI/client for a temporary formal-template validation instance, have a human validate content and entry, stop and verify `reclaimed/stopped/complete`, request Controller resync and confirm the temporary instance is absent, record the human validation in Admin, Resume, publish `current_content_version_id`, set binding `accepting=true`, then lift content maintenance. Content Tool never performs Drain or Platform publication.

For two nodes, update A first with binding `accepting=false`; after full content drain and local validation/resync, Resume A while its binding stays closed. While global current is still old, A's new reported version does not match and receives no old-version Allocation. Publish global current only after A is ready, then open A's binding. New Allocations go only to matching A. Then update B with `accepting=false`, preserving any active old-version instance until normal reclaim. Validate/resync B and set `accepting=true`. The old release remains available for rollback. No same-node active old/new content coexistence is promised.

If a content switch fails, keep binding `accepting=false` and Node Drain as needed. Read `status`, inspect pending metadata and links, retry the explicit switch to recover, or `rollback` to the previous known release. Controller must again report the rolled-back version and local human validation must pass before publishing the corresponding Platform target or resuming allocations. Never manually copy over the current VPK.

## Entry validation and small human trial

`a2s_enabled` is the Node's deployment fact. The Controller checks each actual Ready instance separately; Admin Allocation details show its local port, `ok/failed` and last check. No Ready instance means no query fact, not a failure. Legacy aggregate `a2s_query_ok` is deprecated/derived and must not be used for entry/node health, player URI, connect, capacity or scheduling. Under [V1.0 Amendment 001](specs/v1-amendment-001-entry-verification.md), for each Node and independently for Steam and steamchina, a trusted administrator confirms that a real client used the scheme URI to enter one real Ready instance under the current `entry_config_revision`. Admin requires explicit second confirmation, then records `verified_at`, `verified_by` and Audit. Within the same revision `verified` remains true; if an entry misbehaves, turn `enabled=false`, retain the attestation, and restore `enabled=true` after repair. Changing protocol IP, local/public mappings or other URI/A2S deployment configuration changes the revision and clears both schemes' verification and enabled flags. Routine Platform/Controller restarts, Web updates, content changes and instance lifecycle do not require reverification when entry network facts remain unchanged. No port-pool A2S or per-port human coverage is required. The always available fallback after Ready and valid JoinInfo is `connect <host>:<actual-public-port>`.

A.7 小范围 Production 多人试用已在正式 V1 Release 前完成。其历史验收标准为：至少两位真人通过 Web 加入同一 Party，选择游戏/玩法和 Node 模式，申请并等待 Ready/JoinInfo，进入同一真实 Dota 实例并游玩，随后正常 stop 或 next-game，确认完整回收与 Party 持久性。两个浏览器 Session 或 Bot 不能代替两位真人；P5F 开发收口原本只要求一名真实 Owner 的 Player Web 流程和两个独立匿名 Party Session，见 [Amendment 002](specs/v1-amendment-002-human-trial-gate.md)。这段说明属于已完成的历史门槛，当前 Release 状态以[根 README](../README.md)为准。

## V1 Known Limitation：永久未知 create（FIX-05，Owner accepted）

固定 d2core v0.1.1 无法为已发出、响应未知且无可信 IDs 的 create 提供安全 no-effect 证明。Platform fail-closed；容量可能无限期保留，旧请求也可能迟到产生真实实例。list 空集、重试拒绝、相同路径/marker、manager restart 或等待均不能释放容量。

运营步骤：

1. 管理员在请求详情核对原 Allocation/NodeJob，执行 `allocation.quarantine`，确认“容量和端口仍占用”。在线长期 unknown 同样允许；纯 pending 从未 claim 不使用此动作。
2. Owner/Party leader 在玩家页面二次确认“放弃此异常服务器并继续”。旧 Request 变 abandoned，旧 Allocation 仍 quarantined/occupied，旧 Job、frozen key、版本和节点历史保留。普通成员/无关用户无此权限。重复放弃幂等。
3. Owner 可创建独立的新 ServerRequest，可能需要备用节点。旧请求迟到启动时可能与新实例并存；隔离/放弃不宣称旧 Dota 已停止。
4. 若原 key 后来恢复可信 IDs，通过既有身份核验、stop 和 full reclaim 收敛。只有 reclaimed + stopped + cleanup complete 才释放旧占用。
5. 无法收敛时，在 Admin 节点设置 Drain 并把 desired capacity 设为 0，写入审计；必要时使用已有受控运维禁用节点。节点退出调度，但 occupied 账本和旧 Node/Allocation/Job 永久保留，不删除、不改成 reclaimed/released_no_effect。
6. 禁止以重新注册同一物理节点、换 Node ID 的方式恢复账面容量。登记替代节点前核对物理资产台账，不能把旧机器伪装成新机器。重新投入旧机器只能在旧资源经受支持路径完整回收后，或经 Owner 另行授权、审计的整机退役/重建流程。行政退役不等于资源回收，不生成 RECONCILED_NO_EFFECT。本 A.4 不授权实际退役重建主机操作。

Owner Acceptance（2026-09-27）：项目所有者接受以上 V1 安全优先的可用性残余限制，允许旧容量永久保留，由经授权 Agent/管理员执行审计隔离、玩家脱困和必要节点退役/重建。Owner 不要求升级 d2core。未来自动回收此类 unknown 需另起版本设计并审查 recovery proof primitive；不是数据已证明回收，也不是允许误释放。

A.4 migration 19 只新增 next_game_intents.failure_reason，保留 1–18 checksum 与所有合法历史。升级前照常备份；回滚应用时保留 additive column，不执行破坏性 down migration。旧程序不理解失败原因，不应恢复自动消费这些 paused intents；发生回滚应暂停新申请并由管理员核对。

A.4 login proxy contract: production Server binds loopback and only Caddy may forward public traffic. Caddy must overwrite X-Platform-Client-IP with {remote_host}; client X-Forwarded-For is ignored. Do not expose the upstream port or allow untrusted local proxy processes. Development ignores the proxy header. Missing/invalid dedicated headers fall back to the peer address. The login limiter retains active penalties under key floods, refuses new keys when its 10,000 slots are occupied, and reclaims only entries idle over one hour.

Content Tool rejects ContentRoot at or below the resolved Dota game/dota_addons directory before preparing releases. Windows resolves Junction aliases through a directory handle; path-component containment is case-insensitive on Windows. Keep immutable releases in a separate content directory.
