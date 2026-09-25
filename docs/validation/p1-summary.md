# P1 阶段验收与收尾

日期：2026-09-25。项目所有者已确认 P1 UI，并授权在已成立的 P1E 功能验收基础上正式收尾。

| 阶段 | 结论 |
| --- | --- |
| P1A | PASS |
| P1B | PASS |
| P1C | PASS |
| P1D | PASS |
| P1E | PASS |
| **P1 overall** | **PASS** |
| **Owner acceptance** | **CONFIRMED** |
| **P1 UI owner acceptance** | **CONFIRMED** |

这是 P1 阶段验收。V1 最终独立代码审查、RC、Production Release、生产部署验收均尚未进行。P2 尚未开工。

## Checkpoints

下列完整 SHA 已与 Git 历史及各阶段验证记录核对。P1E 功能和后续 UI parity 是两个独立 checkpoint；后者没有重新执行或改写 P1E 功能验收。

| Checkpoint | Full SHA |
| --- | --- |
| P1 start / P0 closure | `b8397999b09ff301ec2a8b10350e37620ee70119` |
| P1A | `e6067f1e816bd916b372c7eec53995ad0912d655` |
| P1B | `2efdd7b405980e974515b2561570edce2d24e039` |
| P1C | `9ab15494adeff6d9e621f41fb5476b58ccd78018` |
| P1D | `f121b7daa2e492b2ae817f061d910997666df80d` |
| P1E automated interim | `758073cc6d35e15f5bbe3ec9c98aec43cfc1c94d` |
| P1E functional | `91b0b06c36899cf1b027bf50dd8c3d327c9bb768` |
| P1 UI parity | `df969b074510f00d162d46b1b5f7225e40215ae7` |

P1 closure 是新增本报告的普通提交。它自己的完整 SHA 和最终 `origin/main` 由提交后的项目所有者交接报告及本地环境台账记录；Git 提交内容无法包含该提交自己的 SHA。

## Product flow

| 能力 | 结果 | 验证依据 |
| --- | --- | --- |
| Anonymous User / server Session | PASS | 真实 HTTPS 浏览器建立匿名 Session；后端从 Session 确定 owner。 |
| Session 刷新、关闭重开及 Platform 重启后恢复 | PASS | Session 持久化测试、当前申请恢复测试、P1E 玩家流程与开发环境重启验证。 |
| ArcadeGame | PASS | 唯一正式测试游戏关联 Workshop `3564393242`。 |
| GamePreset | PASS | 唯一 n6 玩法；`max_players=10` 与节点容量独立。 |
| ContentVersion | PASS | 开发测试版 `p1-test-v1` 不可变，绑定经核对的 VPK SHA256；不代表生产版本。 |
| TemplateRevision 内部绑定 | PASS | 逻辑开发 revision 经 NodeTemplateBinding 映射到 Controller 受信任模板绑定；玩家页面不显示 revision 或绝对路径。 |
| ServerRequest / owner 校验 | PASS | Session-owned solo 申请和后端 owner 授权。 |
| Duplicate request protection | PASS | 同一 User 并发 POST 仅产生一个 blocking ServerRequest、一个 Allocation 和一个 create NodeJob。 |
| Current active request recovery | PASS | 丢失 POST 响应及刷新后查回同一申请；持久化重开保留业务关系。 |
| Allocation attempt | PASS | ServerRequest、Allocation、NodeJob 为独立对象；attempt 身份与历史保留。 |
| ContentVersion snapshot at Allocation creation | PASS | 分配事务读取 current ContentVersion 并冻结到 Allocation；等待中的请求不提前锁版本。 |
| NodeContentBinding | PASS | Controller 读取节点 metadata 和实际 VPK 哈希后上报 `p1-test-v1 / confirmed`；平台单独控制接收分配。 |
| Single-node capacity | PASS | 原子预留、有效容量 1；unknown 和未完整回收状态占位，最终占用 0。 |
| Durable NodeJob | PASS | 沿用 P0 冻结 create 请求、稳定幂等键、unknown reconciliation 和独立 stop Job。 |
| Real d2core create / real Dota | PASS | 固定 d2core v0.1.1 创建真实 n6 专服；两次真人会话实例均到达 Ready。 |
| Ready | PASS | 两次真人会话均收到真实 `active/running/ready`，Platform 记录 UTC `ready_at`。 |
| JoinInfo | PASS | Controller 使用实例实际 local game port 和节点 identity mapping 生成有效 connect；缺映射自动测试保持实例与容量。 |
| 网页 connect | PASS | 正式 Web 展示并可复制 connect 命令；未启用未验证的一键入口。 |
| Human Dota join / usable instance | PASS | 项目所有者报告真人进入并确认可用；不以 Ready、A2S 或日志代替真人证据。 |
| Player stop | PASS | 业务 owner 从网页结束，两次各产生独立 stop NodeJob。 |
| Full reclaim | PASS | 两个真人会话实例均确认 `lifecycle=reclaimed`、`process=stopped`、`cleanup=complete`。 |
| Capacity release | PASS | 真人测试终态及本轮只读复核：无 active Allocation，有效容量 1、占用 0。 |

详细实现与测试证据分别见 [P1A](p1a.md)、[P1B](p1b.md)、[P1C](p1c.md)、[P1D](p1d.md)、[P1E](p1e.md) 和 [UI parity](p1d-parity.md)。

## 真人端到端验收

项目所有者通过正式网页访问并使用匿名 Session，看到唯一 ArcadeGame/GamePreset，提交申请。系统生成 ServerRequest → Allocation → NodeJob，经固定 d2core v0.1.1 创建真实 Dota；Ready 后生成有效 JoinInfo，网页显示 connect。项目所有者报告使用 connect 实际进入并确认实例可用，随后在网页结束服务器。两次顺序申请均经 stop Job 达到 full reclaim、容量释放；结束页截图与系统终态相互印证。公开记录不包含玩家 IP、SteamID、Cookie 或私人客户端日志。

网页帮助提供可复制 `-console` 启动项、默认 `\` 键提示及实际热键核对提醒。项目所有者明确表示无需另外逐步验证修订后的控制台菜单/按键说明，因此**这些具体说明的当前客户端逐步复核为 NOT VERIFIED**；真人 connect 成功本身为 PASS。

P1 UI owner acceptance 为 **CONFIRMED**：正式 Web 已达到项目所有者对 P-1 已验收原型的一致性要求。P1E 功能 checkpoint 与 UI parity checkpoint 独立保留。发布前如有需要，可另做 UI consistency 检查；本次收尾不再进行装饰性润色。

## Platform UTC timing

时间点均为 Platform Server 的 UTC 观察值，未使用浏览器轮询时间或跨主机 wall clock 相减。P1B、P1C 为开发联调样本；P1E 的两行是项目所有者真人流程样本，均无已有实例造成的排队。

| 样本 | requested_at | assigned_at | create_started_at | ready_at | join_info_available_at |
| --- | --- | --- | --- | --- | --- |
| P1B | 2026-09-25 09:29:25.590429Z | 09:29:26.305804Z | 09:29:30.431922Z | 09:29:55.412045Z | 未发生（当时尚无 P1C JoinInfo） |
| P1C | 2026-09-25 09:43:35.270120Z | 09:43:35.700882Z | 09:43:40.254388Z | 09:44:00.232312Z | 09:44:00.232312Z |
| P1E 真人 1 | 2026-09-25 10:28:42.259350Z | 10:28:42.755289Z | 10:28:45.332351Z | 10:29:00.301593Z | 10:29:00.301593Z |
| P1E 真人 2 | 2026-09-25 10:30:53.105277Z | 10:30:54.753221Z | 10:30:55.326457Z | 10:31:10.300227Z | 10:31:10.300227Z |

| 样本 | queue_wait | dispatch_and_start_delay | dota_startup_time | time_to_join |
| --- | ---: | ---: | ---: | ---: |
| P1B | 0.715s | 4.126s | 24.980s | 未发生 |
| P1C | 0.431s | 4.554s | 19.978s | 24.962s |
| P1E 真人 1 | 0.496s | 2.577s | 14.969s | 18.042s |
| P1E 真人 2 | 1.648s | 0.573s | 14.974s | 17.195s |

`queue_wait = assigned_at - requested_at`；`dispatch_and_start_delay = create_started_at - assigned_at`；`dota_startup_time = ready_at - create_started_at`；`time_to_join = join_info_available_at - requested_at`。P0 已观察的正常 core create acceptance → Ready 约 10–16 秒，起点与 P1 的 `dota_startup_time` 不同。当前开发模板 startup timeout ceiling 为 300 秒，**不是正常启动耗时**；production TemplateRevision / production startup timeout **NOT YET FROZEN**。这些少量样本不构成正式 P50/P95、SLO 或 ETA。

## Remaining NOT VERIFIED

- 修订后控制台菜单与热键说明在当前 Dota 客户端的逐步复核；项目所有者明确免除此项。真人 connect 进房已通过。
- A2S query 及其 verified 工作流；Steam URI、steamchina URI 的真人使用及 verified/enabled 管理工作流。当前开发节点相关开关均关闭。
- Windows Node / Windows d2core 真实联调；第二节点、multi-node 与真实多节点调度；Party 及其余 P2+ 功能。
- 正式生产网络/NAT/防火墙矩阵，生产 ContentVersion 发布、生产 TemplateRevision、正式 startup timeout、生产部署与回滚。
- 固定 d2core 历史保留边界的真实 30 天到期；P0 仅模拟该边界。

## Development environment at closure

本轮未创建实例。只读复核时：ServerRequest 为 6 条 `ended`、1 条历史 `cancelled`；6 个 Allocation 全部 `reclaimed`；NodeJob 无 pending/claimed/accepted/unknown。一个历史 P0 `failed_with_effect` create 已由独立 stop 完整回收，不是 failed-but-unreclaimed。固定 d2core v0.1.1 `list` 返回空数组，游戏用户下无 Dota dedicated 进程。唯一节点 `hard_max_instances=1`、`desired_max_instances=1`，占用 0；已结束 request 不阻塞当前 User。

节点继续上报 `p1-test-v1 / confirmed` 且平台允许新分配；当前实际测试 VPK 哈希仍与不可变开发 ContentVersion metadata 一致。内部 n6 模板绑定仍为已验证的 300 秒开发变体；network mapping 为本节点 28000–28009 的 identity mapping。两次 P1E 真人连接已验证开发端口可用。A2S 和 URI 入口仍关闭。开发 Platform（clean P1D build）、Controller（clean P1C build）、d2core manager、PostgreSQL 和 Caddy 继续运行；正式 Web 使用独立 UI parity bundle。私有主机事实和具体路径保存在 Git 忽略的 `.local/p0-host-inventory.md`。

本次只固化 P1 阶段事实，没有清空数据库、删除历史、改动 VPK、网络规则或生产配置。P2 具备从已验收 P1 基线继续进行工程规划的条件；实际开工仍须项目所有者另行授权。
