# 管理员 HTTP API

管理员界面位于 `/admin`，使用同源 `/api/v1/admin`。管理员 cookie 与匿名玩家 Session 分离。Production 要求 HTTPS；cookie 具有 `Secure`、`HttpOnly`、`SameSite=Lax`、host-only 属性，绝对过期时间为 12 小时。成功登录发出新的 opaque token，并撤销该浏览器的上一个 token。退出立即撤销；禁用 AdminUser 或重置密码通过服务端 credential version 使旧 Session 失效。登录失败按 IP 与规范化 username 指数退避。修改请求要求配置的精确 Origin，不启用 wildcard CORS；后端每次请求都重新验证 AdminUser。

| Method | Route | 用途 |
| --- | --- | --- |
| POST | `/login` | 用户名/密码登录；PostgreSQL 存储 Argon2id verifier |
| GET | `/me` | 检查独立 AdminSession |
| POST | `/logout` | 撤销当前 AdminSession |
| GET | `/overview` | 返回脱敏设置、目录、Node 与绑定事实、入口状态、最近 request/Allocation/open Job、Party 摘要和 Audit |
| POST | `/actions` | 在一个数据库事务中执行有类型的管理员状态转换、授权与审计 |

`POST /actions` 接收 `action` 及该动作的字段。支持：`global.update`（`accepting`、`message`）、`announcement.update`（`message`）、`game.update` / `preset.update`（`targetId`、`enabled`、`accepting`、`message`）、`node.update`（`targetId`、`draining`、`priority`、`desired`）、`binding.update`（Node `targetId`、`gameId`、`accepting`）、`node.reconcile`（`targetId`）、`request.cancel`（`targetId`）、`request.stop`（`targetId`）、`allocation.quarantine`（`targetId`），以及 `entry.update`（Node `targetId`、`entry=steam|steamchina`、`verified` 或 `enabled`；新真人验证须 `confirmed=true`）。非法状态转换返回 409，格式错误返回 400，未知 JSON 字段被拒绝。

内容管理动作包括：`game.create`（`workshopId`、`displayName`）、`template.create`（游戏 `targetId`、`templateRevisionId`、`description`）、`content.create`（游戏 `targetId`、`contentVersionId`、`contentSha256`）、`preset.create`（游戏 `targetId`、`displayName`、`maxPlayers`、`templateRevisionId`）及 `template_binding.upsert`（Node `targetId`、`templateRevisionId`、`bindingKey`）。新游戏与玩法初始均禁用且不接收申请。`ContentVersion` 不可变；binding key 是 Controller 本地逻辑名，不是 Web 提交的路径。

`content.validate`（Node `targetId`、`gameId`、`contentVersionId`、`confirmed=true`）只在 Node Drain、空闲，且新近兼容 Controller 上报的版本与 VPK SHA256 均匹配后，记录管理员的真人验收；它不能设置 Controller 上报事实。`content.publish`（游戏 `targetId`、`contentVersionId`、`confirmed=true`）要求此前已在恢复服务且当前兼容的 Node 上验收，其上报 digest 仍匹配不可变版本。Node 的 binding 可暂时 `accepting_new_allocations=false`：先发布目标，再由运维单独开放该绑定。发布仅改变未来 Allocation 的游戏目标并审计旧/新值；waiting request 在实际分配时快照目标，已有 Allocation 不被改写。两个动作都不修改 Node 文件或创建验证实例。

`node.update` 不能把 desired 容量设得高于 Controller 上报的 hard 上限。`binding.update` 只改变 Platform admission，不能修改上报的内容版本、状态或时间。按 [V1.0 Amendment 001](specs/v1-amendment-001-entry-verification.md)，`entry.update` 的真人验证须提供目标 Node、`entry=steam|steamchina`、`verified=true` 与 `confirmed=true`：可信管理员确认真实客户端在当前入口配置 revision 下通过对应 URI 进入真实 Ready 实例。端口列表与验证备注不是 API 字段。同一 revision 中 `verified` 单向成立：管理员提交 `verified=false` 被拒绝，重复确认冲突；日常关闭使用 `enabled=false` 并保留验证元数据，`enabled=true` 要求已验证。只有 Controller 上报的网络 revision 变化才清除两种 scheme 的验证、开放、时间与操作者。A2S 诊断不改变这些状态或玩家 URI。历史 migration 15/17 不被改写；migration 18 新增逐实例诊断。

`GET /overview` 在 Node 对象上提供 `a2sEnabled` 部署事实（未上报时省略）；当前 Allocation 可带 `instanceId`、`a2sLocalPort`、`a2sStatus=ok|failed` 和 `a2sCheckedAt`。运行中 Allocation 有 instance ID 但无 A2S status，表示没有当前查询事实，不等于查询失败。Entry 对象只含 revision 以及 Steam/steamchina 各自的验证、开放与验证时间，不包含 A2S 或端口覆盖列表。旧聚合 `a2sQueryOk` 只为 Node 心跳/存储兼容保留。玩家 API 和 JoinInfo 不返回 A2S 字段。

`request.cancel` 只接受没有 Allocation 的纯 waiting request。`request.stop` 为可信现存实例创建正常 stop NodeJob，直至 d2core 完整回收前仍占容量；旧请求已 abandoned 且旧 Allocation 仍 quarantined 时也可请求 stop，请求继续保持 abandoned，旧历史保留，完整回收后才释放旧容量。已有 open stop Job 或已回收 Allocation 不能再建 stop。`allocation.quarantine` 保留容量、节点、端口、内容和历史，拒绝纯 pending 预留。`node.reconcile` 为目标 Controller 排入持久代数；完成仅表示执行过一轮，不表示所有异常都已消失。

Audit 记录管理员身份、动作、目标、时间和相关前后状态，不记录密码、Session token、Node Secret、SSH key、冻结本地路径或原始私有网络事实。CLI 运维动作和自动不可达隔离使用独立 actor kind。`/overview` 不返回原始网络事实或本地路径，只显示结构化错误码。没有 Web shell、任意命令、进程 kill、文件浏览、防火墙或 NAT 控制。

## A.4 安全修复后的约束

`allocation.terminate_pending` 的 `targetId` 为 Allocation ID。仅允许 reserved + create pending + `claimed_at` 为空 + 无 execution/instance/operation/report 证据。事务锁住 claim 使用的同一 NodeJob；若 claim 已赢则返回 409，继续占容。成功后 Job 为 `rejected_no_effect`（`PENDING_TERMINATED`）、Allocation 为 `released_no_effect`、Request 为 `unavailable`；写 Audit，重复调用幂等，旧 Job 永不可领取。普通玩家 cancel 规则不变。

FIX-09：`entry.update` 验证及 `enabled=true` 要求页面展示的 `expectedEntryConfigRevision`；缺失或过期返回 409，不修改状态，也不写成功 Audit。关闭入口不需要证明 revision。刷新后应按新配置重新做真实客户端验证。

FIX-11：数据库撤销失败时 logout 返回 503 并保留 cookie；已失效 token 仍幂等。管理员登录在同一事务中插入替换 Session 并撤销旧 cookie token，要么全部提交，要么都不提交。UI 退出失败时保持已认证状态并提供重试。

内容标识：Workshop ID 为 1–20 位 ASCII 十进制数字；ContentVersion ID 长 1–128 位 ASCII，首位字母或数字，后续可用字母、数字、点、下划线、连字符。`current`、`previous`、`pending` 不区分大小写地保留。目录、Controller contract/config 和 Content Tool 共用此语法；既有不可变 ID 不自动重命名，后续发布前应审计旧非法 ID。
