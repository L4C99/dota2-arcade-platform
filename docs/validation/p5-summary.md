# P5 final closure — PASS under V1.0 Amendments 001 and 002

日期：2026-09-27。收口授权开始时 `origin/main` 与本地 HEAD 均为 `50124329290df8312d9f8224c2df398681554cff`，工作区干净。项目所有者确认当前玩家端与管理员端 UI 为 **P5 UI Owner Acceptance = CONFIRMED**，停止继续进行主观视觉或功能打磨。复杂内容/节点维护按 `docs/operations.md` 由受授权 Agent 执行，Owner 负责真实 Dota 内容验收及高风险授权；本收口不增加远程执行能力。

| 子阶段 | 最终状态 | 依据与边界 |
| --- | --- | --- |
| [P5A](p5a.md) 多地图/玩法/内容版本 | **PASS** | 两张真实 Workshop 地图、不可变版本、三种新图玩法、Controller 真实内容与 SHA256 上报、发布和正常 Player Allocation 已验证。Windows 新图节点仅有内网连通，未开放为公网玩家节点。 |
| [P5B](p5b.md) Content Tool 与滚动发布 | **PASS** | 两节点隔离副本、prepare/switch/status/rollback、Linux symlink/Windows Junction、旧图 legacy 两节点真实滚动、真人内容验收、旧 Allocation 保留、新版本新 Allocation、完整回收均有证据。无生产内容切换。 |
| [P5C](p5c.md) A2S 与一键入口 | **PASS，按 [Amendment 001](../specs/v1-amendment-001-entry-verification.md)** | A2S 为真实 Ready 实例诊断；Owner 通过真实 Steam URI 在 Linux 28000–28002 进服，并报告蒸汽平台 URI 实际进服。两种 scheme 在 Linux 当前入口 revision 下分别确认和开放；旧“所有端口逐一真人验证”规则已撤销。Windows 公网 Steam/steamchina 入口仍 **NOT VERIFIED**。 |
| [P5D](p5d.md) 参考部署 | **PASS（参考实现与开发预检）** | Ubuntu Web 控制面、Linux/Windows Controller 安装资产和真实开发配置预检、开发控制器升级与兼容心跳已验证。全新 Ubuntu systemd 安装、Windows 无人值守任务/重启、生产 TLS/Secret 配置与生产部署仍 **NOT VERIFIED**，留给后续安装/上线阶段。 |
| [P5E](p5e.md) 运维 | **PASS（文档与开发演练）** | 受保护 PostgreSQL 备份及隔离恢复、显式迁移、开发 Web/Controller 升级、代码回滚、内容滚动/回滚 runbook 已完成。故意失败迁移、schema 不兼容旧 binary 的真实恢复、实际 d2core/Dota 升级和生产恢复仍 **NOT VERIFIED**。 |
| [P5F](p5f.md) 开发阶段真人与 Party 流程 | **PASS，按 [Amendment 002](../specs/v1-amendment-002-human-trial-gate.md)** | Owner 一人通过正式 Player Web 完成 request → Allocation → create → Ready/JoinInfo → 真人进服游玩 → 页面 stop → full reclaim；两个独立匿名 Session 完成 Party API/网页流程，权限边界有集成测试。**两位真人同一 Party/实例仍 NOT VERIFIED，DEFERRED TO A.7，阻塞最终 V1 Release。** |
| [P5 UI](p5-ui-parity.md) | **OWNER ACCEPTED** | Owner 明确确认当前玩家与管理员 UI 达到 V1 可接受水平，不再因主观文案、间距或布局偏好延长 P5。后续只处理真实 bug、验收失败、审查裁决或 Release blocker。 |

## 未验证与后续门槛

- **DEFERRED TO A.7 / FINAL RELEASE BLOCKER：**两位及以上真人在同一 Party，经正式 Player Flow 进入同一个真实 Dota 实例，正常游玩，结束或下一局并 full reclaim。两个匿名 Session、两窗口、Bot、mock 或一人多开均不是该证据。小范围生产试用中的真实玩家可自然完成这项验收。
- **NOT VERIFIED，属于后续环境/上线工作：**Windows 节点公网 URI 与公网玩家路由；全新参考安装和无人值守重启；生产 TLS、Secrets、数据库恢复和生产部署。它们不得从开发预检推断为已通过。
- 启动耗时的现有日志表明 Steam 登录 Ready marker 可能解释部分波动；这不是删减 Ready/JoinInfo 条件或大规模性能重构的依据。[P5F](p5f.md) 保留具体样本与证据边界。

收口时仅做文档与验收门槛修订，不修改业务代码、API、migration、ContentVersion/Allocation/Party/d2core 语义，不创建真实测试实例，也不访问生产节点。开发环境只读核对得到活跃 Allocation、open NodeJob、活动 ServerRequest 均为 `0|0|0`。P5 至此停止；A.1 Candidate Freeze、独立审查、RC、生产部署、Tag 和 Release 均须后续单独授权。
