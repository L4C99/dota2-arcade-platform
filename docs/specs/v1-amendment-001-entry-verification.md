# V1.0 Amendment 001 — 一键入口验证语义

日期：2026-09-27。项目所有者在已授权的 P5 阶段明确批准本修订。产品仍为 V1；原冻结规格除本修订覆盖处继续有效。本修订不产生 RC6，不重新打开 P0–P4，也不授权 RC、Tag、Release 或生产部署。

## 范围与原因

本修订仅涉及 A2S、Steam/蒸汽平台一键入口的真人验证、开放与网络修订失效。原 V1 §17/P5C 要求节点级 `verified` 覆盖当前配置下全部可能分配的公网端口，P5C 实现要求管理员提交完整 `verifiedPorts` 和备注。几十个端口逐一创建实例并真人进入不可维护，也不能从空闲端口取得 A2S 查询事实。项目所有者正式撤销完整端口覆盖要求，改为可信管理员对当前 Node、当前 `entry_config_revision` 下真实客户端成功通过相应 URI 进入一个真实 Ready 实例的明确确认。

## 正式规则

- `a2s_enabled` 表示节点部署声明启用 A2S。`a2s_query_ok` 只描述 Controller 对当前所有可查询 Ready 实例的实时查询结果；没有 Ready 实例时无实时查询事实，布尔值 `false` 不等于故障。A2S 不是端口池认证，也不是玩家入口、`connect`、容量或调度的运行时门槛。
- Steam 与蒸汽平台各自独立保存 `verified`、`enabled`、验证时间、管理员和网络修订。`verified=true` 必须由已认证管理员二次确认真实客户端通过对应 URI 进入真实 Ready 实例；平台记录声明，不要求端口列表、覆盖率或必填备注。A2S 成功不能自动验证；A2S 失败不能自动撤销验证或关闭按钮。
- `verified=false` 时不允许 `enabled=true`。撤销验证同时关闭入口并清除验证时间/人员。网络入口事实改变引起 `entry_config_revision` 改变时，两种入口的验证与开放均失效。普通代码、Web、Controller、d2core 重启，VPK/ContentVersion 变更和实例生命周期变化，若入口网络事实未变，本身不触发重验。
- 玩家 URI 仅在实例 Ready、有效 JoinInfo、`protocol_ip`、Allocation 保存的 entry revision 与 Node 当前 revision 一致，且对应 scheme 已验证并开放时提供。无当前端口历史覆盖要求，也无实时 A2S 成功要求。`connect <host>:<actual-public-port>` 永远保留为正式兜底。
- 管理员界面分别提供“确认真人验证 / 撤销真人验证”和“开启入口 / 关闭入口”，确认真人验证必须二次确认。空闲节点的 A2S 文案不得断言查询失败。

## 文档和兼容性

受影响的原规格章节为 §16、§17、P5C、§45；配套更新管理员 API、运维说明和 P5C 验证记录。旧 migration 15 的文件与校验历史保持不变。实施采用新的前向 migration 17：移除旧端口覆盖约束与 `steam[_china]_verified_ports`、强制备注列；保留既有 verified/enabled、时间、管理员、revision 与 Audit。`entry.update` 不再接受 `verifiedPorts` 或 `verificationNote`；新的确认只需 Node、scheme、`verified=true`、`confirmed=true`。这属于 P5 尚未正式发布阶段的 API 收敛，旧完整端口请求需更新，不能再作为验证条件。

## 既有证据

P5C 起初按旧冻结规则实现和测试，原记录保留。Linux 开发节点经项目所有者授权运行固定 d2core v0.1.1 `a2s enable` 并保留原文件备份；新实例的 28000–28003 A2S 查询成功，其中项目所有者用真实 Steam URI 在 28000–28002 成功进服，所有临时实例完整回收。旧规则下这只是部分端口证据；本修订下，它足以支持**当前 Linux Node 当前 entry revision 的 Steam 真人验证**，前提是发布前再次核对该 revision 未变化。它不证明蒸汽平台或 Windows 公网入口。无需为补齐端口集合继续创建实例。

本修订不改变 ServerRequest、Allocation、NodeJob、Party、ContentVersion、d2core 生命周期、网络映射或安全回收语义；不引入逐端口验证模型、自动网络配置或生产节点操作。
