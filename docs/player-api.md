# 玩家 HTTP API

所有 route 都是同源的 `/api/v1` JSON 接口。服务端签发的匿名 Session cookie 标识当前 User；修改状态的请求需要配置的 `Origin`。请求体拒绝未知字段，包括 `userId`、`partyId`、`role` 和 `leaderId`；客户端提供的身份或角色不能授权操作。

按 [V1.0 Amendment 001](specs/v1-amendment-001-entry-verification.md)，玩家 API 与 JoinInfo 只提供最终 Ready/入口结果和永久的 `connect` 兜底，不暴露 Node A2S 配置、逐实例查询状态或查询时间。A2S 失败不隐藏其他条件仍有效的 URI。

| Method | Route | 基于 Session 的行为 |
| --- | --- | --- |
| POST | `/session` | 创建或恢复匿名 User Session；返回 `userId` 与稳定的 `displayName` |
| GET | `/me` | 返回当前 User ID 与 `displayName` |
| GET | `/catalog` | 返回当前 ArcadeGame/GamePreset 目录与维护事实 |
| GET | `/party` | 返回当前 Party、带 `displayName` 的成员、持久角色与 `maxSize`；无 Party 时返回 `null` |
| POST | `/party` | 创建持久 Party；调用者成为队长和成员 |
| GET | `/party/invite` | 仅队长读取当前高熵邀请凭据 |
| POST | `/party/invite/reset` | 仅队长撤销旧凭据并取得新凭据 |
| POST | `/party/join` | 使用 `{ "token": "..." }` 加入未满 `max_party_size` 的活动 Party |
| POST | `/party/leave` | 仅普通队员退队；不停止服务器 |
| POST | `/party/members/{userId}/remove` | 仅队长移除队员；不能移除自己，也不踢出 Dota 玩家 |
| POST | `/party/disband` | 仅队长在没有 blocking request 或未回收 Allocation 时解散 |
| GET | `/nodes?arcadeGameId={id}&gamePresetId={id}` | 返回所选内容可用的玩家安全 Node 名称、可用性原因与参考空槽数 |
| POST | `/server-requests` | Solo User 或当前 Party 队长提交 `arcadeGameId`、`gamePresetId`，可选 `nodeSelectionMode`（默认 `auto`，或 `manual`）；`manual` 必须提供 `manualNodeId` |
| GET | `/server-requests/current` | 返回当前 User/Party owner 的 blocking request；无则 `null` |
| GET | `/server-requests/{id}` | 仅当前业务 owner 可见 |
| GET | `/server-requests/{id}/allocation` | 返回获授权请求的最新 Allocation/JoinInfo |
| POST | `/server-requests/{id}/stop` | Solo owner 或当前 Party 队长发起持久 stop/reclaim |
| POST | `/server-requests/{id}/next-game` | Solo owner 或当前 Party 队长原子记录持久 next-game intent，并启动正常 stop/reclaim；重复调用返回同一旧请求 |
| GET | `/server-requests/{id}/next-game` | 授权 owner 读取 `pending`、`paused` 或 `consumed` intent；消费后含新 request ID，无 intent 时为 `null` |
| POST | `/server-requests/{id}/abandon` | Solo owner 或当前 Party 队长提交 `{ "confirm": true }`，仅用于 quarantined Allocation，只解除旧请求的业务阻塞 |
| POST | `/server-requests/{id}/cancel` | Solo owner 或当前 Party 队长仅能取消尚无 Allocation attempt 的 waiting request |

Party owner 固定在 `ServerRequest.owner_party_id`，成员变化后不会改写。普通队员可读取同一 Party 的请求与有效 JoinInfo；离队后失去访问权。队长重复 POST 返回原 blocking Party request。Party 人数超过所选 GamePreset 的 `max_players` 时，在创建 ServerRequest、Allocation、NodeJob 之前返回 `409 party_exceeds_preset`。`PlatformSettings.max_party_size` 是独立的 Party 成员上限，服务 P2 流量前必须通过 `PLATFORM_MAX_PARTY_SIZE` 显式配置为正整数。

`users.display_name` 是持久、仅用于展示的匿名名称。Migration 9 回填旧 User，不改变其 ID、Session 或 Party 成员关系；新 User 的名称与 Session 在同一事务中生成。`/me`、`/session` 与 Party 成员行均展示该名称，允许重名。所有权和授权只取决于 Session/User ID，不取决于名称；没有名称编辑、搜索或 profile API。

邀请凭据包含 32 个随机字节。正式 Web 在 URL fragment（`#invite=...`）中分享，使页面请求与 Referer header 不携带凭据。仅队长可读的邀请响应属于敏感信息，不得写入日志或 validation 文档。Reset 与 consume 串行化；reset commit 后旧凭据立即失效。公开 Party 列表、匹配、队长转让及 Web 直连 d2core 均不属于该 API。

每个 ServerRequest 持久保存自动/手动选择。`auto` 在所有 eligible Node 中选最高 priority，平局按 Node ID；`manual` 只等指定 Node，即使它满载、Drain、离线或内容未就绪。`/nodes` 是展示提示，空槽数可能在提交前变化；其 `connectivity: online|stale|offline` 来自服务端记录的心跳时间。调度器在预留事务中重新检查同一 Node 资格。重复提交返回原 blocking request 及其原选择。修改手动选择只能取消安全 waiting request 后以新 `requestedAt` 重交；取消不能释放已预留或可能已经创建的实例。`/nodes` 不返回 Node Secret、路径或网络配置。

`quarantined` Allocation 仍占容量，表示清理结果不确定。`/abandon` 是独立、显式确认的 owner 操作：旧 ServerRequest 变为 `abandoned`，允许新业务请求，但不会停止 Dota、释放旧端口或 Node 容量、改写旧 Allocation 的节点/内容/历史，也不会把旧 Allocation 变为 `reclaimed`。重复 abandon 幂等；仅后续 d2core 确认完整回收才释放容量。普通 Party 成员与其他玩家不能执行。

next-game intent 与 stop Job 在同一事务中记录。成功完整回收后，恰好创建一个新的 FIFO ServerRequest，使用新的 `requestedAt`，继承 ArcadeGame、GamePreset 和 auto/manual 选择（含手动 Node）；新 Allocation 重新快照届时的 ContentVersion。stop 进入 quarantine 时 intent 暂停，不创建新请求；队长确认 `/abandon` 后消费一次，旧占用仍保留。如果旧资源在 owner 决定前被证实完整回收，请求仍停留在 quarantine 决策状态，等待队长确认继续。普通 `/stop` 不创建 next-game intent；此流程不调用 d2core instance restart。

`/catalog` 的轻量 `siteAnnouncement` 独立于全局维护向玩家展示，不影响调度。管理员 API 与 cookie 和玩家侧分离。

A.4 FIX-07：next-game intent 可返回 `failureReason`。资格失效时 state=`paused` 且没有 `newRequestId`；旧资源回收/abandon 正常提交，页面要求重新选择，不自动重试建申请。所有新请求共用当前 Party 人数、global/game/preset admission 和 manual Node 校验。
