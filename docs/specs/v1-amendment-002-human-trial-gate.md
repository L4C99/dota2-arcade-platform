# V1.0 Amendment 002 — Human Trial Gate Relocation

日期：2026-09-27。项目所有者在已授权的 P5 最终收口阶段正式批准本修订。产品仍为 V1；原冻结规格与 Amendment 001 除本修订覆盖处继续有效。本修订不产生 RC6，不重新打开 RC4/RC5，不授权 Candidate Freeze、独立审查、RC、生产部署、Tag 或 Release。

## 范围与原因

原 P5F 要求两位真人经同一 Party 进入同一个真实 Dota 实例。当前开发环境只有项目所有者一名真人和一个真实 Dota 客户端；Owner 决定不在小范围生产试用之前专为开发环境邀请第二位玩家。两个匿名浏览器 Session、Bot、mock 或一人多开不能代替两位真人。本修订仅调整该真实多人验收的时点，保留验收内容和最终 Release 门槛，不改变 Party、ServerRequest、Allocation、Node、d2core、Ready、JoinInfo、stop/reclaim 或任何玩家业务语义。

## P5F 开发阶段正式标准

1. 至少一名真实 Owner 通过正式 Player Web Flow 完成网页申请 → Allocation → d2core create → Ready → JoinInfo → 真人进入并正常游戏 → 页面结束服务器 → stop → full reclaim。既有有效真实证据可以复用，不为形式重复创建实例。
2. 两个独立匿名 Session 验证 Party create、invite、join、双方观察同一 Party、leave/disband 和权限边界。真实浏览器/API 操作与自动化权限测试应分别如实标注。
3. 上述两个 Session 只能证明 Party 开发阶段功能，不能声称两位真人同局。P5F 可在满足前两项时 PASS，同时记录真实多人同局 **NOT VERIFIED / DEFERRED TO A.7**。

## A.7 与最终 Release 门槛

两位及以上真人在同一 Party，经正式 Player Flow 进入同一个真实 Dota 实例，正常进入和游玩，正常结束或下一局，并确认 full reclaim，必须在 A.7 小范围 Production Rollout 中取得真实证据。自然参与小范围试用的真实玩家可以提供该证据。该验收不再阻塞 P5 Closure、A.1 Candidate Freeze、独立审查、RC1、Owner A.6 RC2 smoke 或生产部署准备；它仍然阻塞最终 `v1.0.0` Tag / GitHub Release。未发生的多人游戏不得写为 PASS。

## 受影响记录与既有证据

定向修订 `v1.md` 的 P5F、§45、A.1、A.6、A.7、A.8，并同步 `docs/validation/p5f.md` 与 P5 closure。P5F 原两真人规则和先前 pending 判断保留为历史记录。开发站点既有证据包括：Owner 单人经 Player Web 完成真实开服、进服、游玩、页面结束和完整回收；两个独立匿名 Session 完成 Party API 与网页流程；Party 权限有 PostgreSQL 集成测试。它们不构成两位真人同局证据。详情与具体限制见 `../validation/p5f.md`。

本修订不改变现有 API、migration、存储状态或调度门槛，也不创建新自动化、多开替代方案或额外开发阶段。后续 Release 负责人必须在 A.7 取得并核查真实多人证据后，才能解除 A.8 门槛。
