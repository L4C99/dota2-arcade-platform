# Node API v1（P0C/P0D）

Node Controller 主动连接 Platform Server 的 `/api/node/v1`。正式环境经 Caddy 使用 HTTPS；Platform Server 只接收 Caddy 转发的本地 HTTP。开发模式只允许 Controller 访问 loopback HTTP。生产不关闭 TLS 证书校验。

每个请求携带 `X-Node-ID: <uuid>` 和 `Authorization: Bearer <node-secret>`。节点由 `platform-server node register <display-name> <windows|linux>` 显式登记。原始高熵 Secret 只在登记时输出一次；数据库保存 SHA-256 verifier。Controller 从独立的本机 Secret 文件读取原值，配置文件和日志不包含 Secret。

## 心跳与节点事实

`POST /heartbeat` 发送 `Heartbeat` JSON：OS、Controller 版本、Node API 版本、d2core `BUILD.json` 版本与 commit、已建立的本地 protocolVersion、`hardMaxInstances`、本机网络映射以及内容读回事事实。服务端使用自己的 UTC 时间记录 `reportedAt`。只有 Node API v1、固定 d2core `0.1.1` / `988720ad85af1f0d97bfe98ec4da4fcbb070beea` 且本地协议 v1 已验证时，状态为 `compatible`。

Controller 在首次真正连接 d2core 之前上报 `d2coreProtocolVersion=0`；它不能凭期望值伪称连接已验证。`BUILD.json` 不可读时版本为 `unknown`，该节点不领取新任务。

网络事实包含 `connectHost`、可选 `protocolIp`、本地游戏端口范围、`identity` 或完整 `explicit` 端口映射，以及 A2S 部署声明。Controller 与服务端都验证映射覆盖范围、重复公网端口和 `hardMaxInstances <= 端口池大小`。本地端口范围必须与 d2core serve 的 `--port-min/--port-max` 来自同一部署配置；P0/P5 安装验收还要核对实际服务参数。心跳不会自动改防火墙/NAT。

P3E 的 `connectHost` 可是节点的直连 IP、自有域名或厂商 NAT 域名；域名按完整 DNS label 校验，但不推断其公网可达性。`protocolIp` 只接受显式 IP，可以留空；Controller 不会把 NAT 域名解析成 Steam URI 所需的 IP。`identity` 将 d2core 实际本地端口映射到同号公网端口，`explicit` 必须逐一覆盖本地池并保持公网端口唯一。JoinInfo 使用该实例由 d2core `status` 报告的实际端口，未覆盖时返回 `PORT_MAPPING_UNAVAILABLE` 且实例继续占容量。网络事实变更会改变 entry revision，使旧 JoinInfo 不再展示。固定 d2core v0.1.1 API 不能读取管理器当前 serve 端口范围，部署时必须从同一配置生成 Controller 与 serve 参数并核对运行中的参数；不声称从本地 API 自动发现。

内容事实只在 Controller 能同时读回当前目录链接、metadata 指向和 VPK 文件时报告 `confirmed`；读不到或不一致时报告 `unknown`。P0C 保存原始节点事实快照供诊断；P1B 将建立正式 `NodeContentBinding` 调度模型。Web/Admin 不能通过节点 API 伪造这些事实。

## durable NodeJob

- `GET /jobs/open`：列出本节点所有未终结任务及已冻结执行参数。Controller 启动或重连时先读取并对账。
- `POST /jobs/claim`：在兼容且近期有心跳的节点上，事务性领取最早的 pending job。无任务返回 204。
- `GET /jobs/{id}`：查询本节点任务；其他节点的任务返回 404。
- `POST /jobs/{id}/prepare`：create job 在首次 d2core 调用前提交已解析的本机模板绝对路径和请求 port。服务端从不可变 `node_job_id` 派生 d2core idempotencyKey，并将完整请求参数及 SHA-256 指纹持久化。相同 prepare 可重试；任何变化均拒绝。数据库触发器禁止修改或删除冻结记录。
- `POST /jobs/{id}/report`：幂等报告 `accepted`、`unknown`、`succeeded`、`rejected_no_effect` 或 `failed_with_effect`，并保存 instanceId/operationId 和结构化 core 错误。已知 ID 不允许换成另一个 ID；终态不能回退。

P0 的任务明确标记 `integration_only=true`，由本地管理员流程创建；P1 才引入 ServerRequest / Allocation 关联、排队和容量业务。timeout 或响应丢失必须报告 unknown 并对账，不能把它当作无副作用失败。d2core `accepted` 不是完成；Controller 必须再查 operation 与 status。

## P3A 节点容量运维控制

`hard_max_instances` 仍只来自 Controller 心跳中的本地部署事实。具有 Platform 数据库访问权限的运维人员可在 Platform 主机运行 `platform-server node capacity <node-id>` 查看最近报告的 hard、平台 desired、当前 occupied 和有效上限；`platform-server node capacity <node-id> <desired>` 设置平台调度上限。设置值必须在 `0..hard` 内，尚无有效心跳报告的节点不可设置。此命令不改变 Controller 本地配置。

Controller 后续把 hard 降低时，历史 desired 可以暂时高于 hard；调度始终使用两者的较小值。occupied 包含除 `reclaimed`、`released_no_effect` 外的所有 Allocation attempt。这个控制面只依赖本地主机既有数据库权限，不向玩家提供容量写入入口。

## P3B 自动调度 priority

拥有 Platform 数据库访问权限的运维人员可使用 `platform-server node priority <node-id>` 查看节点的调度优先级，或使用 `platform-server node priority <node-id> <integer>` 设置它。默认值为 0；仅自动申请在全部 eligibility 条件成立后使用 priority，数值越高越优先，平局按 Node ID 稳定排序。手动申请始终只等待指定节点。priority 不改变 Controller 报告的节点事实，也不覆盖容量、内容、心跳或维护限制。

## P3C 心跳可用性与恢复

Platform 以服务端记录的最新心跳时间判定 Node `online`（小于 2 分钟）、`stale`（2 到小于 5 分钟）、`offline`（至少 5 分钟或从无心跳）。只有 `online` Node 可接收新 Allocation 和待领取的 NodeJob。此阈值是 P3 调度/领取阈值，与 P4 的异常隔离计时无关。变成 stale/offline 不改变已有 Allocation、NodeJob 或 occupied，也不推断 d2core instance 已停止。

Controller 恢复联系后仍先执行 d2core list，再读取本 Node 的 open durable jobs、已冻结 create 参数，并通过 operation/status 对账；可能有副作用的 unknown create 始终保留原 Node 和容量。只有明确的 `rejected_no_effect` 才释放该 attempt。自动请求随后可在另一个 eligible Node 生成顺序递增的新 attempt，并重新快照当前内容版本；曾明确拒绝该请求的 Node 不会立即重复尝试。手动请求不改派到其他 Node。若全部已尝试 Node 均明确无副作用地拒绝，申请进入 `unavailable`，避免无限重试。完整 P4 quarantine/玩家脱困流程不在 P3C。

P4B 将 `PlatformSettings.quarantine_after_node_unreachable` 的冻结默认值设为 15 分钟。Platform 部署可通过 `PLATFORM_QUARANTINE_AFTER_NODE_UNREACHABLE` 指定正的 Go duration（例如 `20m`）；服务启动时把部署值写入 PlatformSettings。阈值到达且 Allocation 可能有副作用时，后台循环将其标记 `quarantined`，但保留 Node 容量、原端口和全部历史；这不表示 d2core 已停止。迟到的普通 create/stop 报告不能把已隔离 Allocation 恢复为可用状态，只有明确的资源终态证明才可结束占用。数据库授权的运维人员可用 `platform-server allocation quarantine <allocation-id>` 根据诊断提前隔离；P4D 管理员 Web 控制面将提供相应操作与 Audit。

## P4D/P4E 完整实例对账

兼容心跳响应可带 `reconcileRequestedGeneration` / `reconcileCompletedGeneration`。管理员请求对账只增加目标 Node 的持久代数；Controller 在一次成功的对账轮次后通过 `POST /reconcile/complete` 回报同一代数。回报表示已执行一次对账，不表示所有异常已经回收或隔离已解除。断线或 Controller 重启后，未完成代数仍可从心跳恢复。

每轮 Controller 先读取 d2core `list` 和未终结 NodeJob，按原冻结请求、operation/status 收敛；然后用 `GET /allocations/active` 读取本 Node 已知实例身份，逐个查询 d2core `status`，通过 `POST /allocations/{id}/fact` 回报所见状态。存在未终结 NodeJob 的 Allocation 先由该 Job 的原路径处理。每份实例事实必须匹配旧 create Job 的 immutable instance ID。传输错误保留未知；只有结构化身份错误进入隔离。只有 `lifecycle=reclaimed`、`process=stopped`、`cleanup=complete` 同时成立才更新 Allocation 为 `reclaimed` 并释放容量。活跃实例端口与冻结 JoinInfo 不同会隔离，绝不悄悄改写旧端口。重复事实幂等，不生成第二个 create 或新的 Allocation attempt。

这些端点仅接受原 Node 的 Secret。Web 管理员可以请求对账，但不能上报 Controller 的实例、内容或网络事实；管理员总览只显示对账代数和非敏感状态摘要。

## P3D 普通 Node Drain

运维人员在 Platform 主机用 `platform-server node drain <node-id>` 关闭该 Node 的新 Allocation admission，用 `platform-server node resume <node-id>` 恢复。命令可重复执行；写入的是 Platform 的 `nodes.draining` 控制值。Drain 后已有 Allocation、NodeJob 与 d2core instance 继续原生命周期，pending 的已有 Job 仍可被该 Node 领取；不自动 stop、cancel、reclaim、切换内容版本或清理端口。自动申请可选其他 eligible Node，手动申请保留原 `requested_at` 并等待目标 Node Resume。Resume 仍需通过所有其他内容、容量、兼容与心跳检查。此操作不执行 P5 内容滚动工作流。

## Controller 本地配置

Controller 使用显式绝对路径 JSON 配置，其中包含 Platform URL、Node ID、Secret 文件路径、d2core `BUILD.json` 和 data-dir、端口映射、硬上限、逻辑模板到本机绝对路径的绑定，以及可选内容 metadata/当前链接位置。所有 d2core 关键路径须为 ASCII 绝对路径。节点 Secret、真实节点路径、端口和部署参数保存在仓库外的开发或生产专用目录。源码工作树不作为生产运行目录。

## P0D 本地核心调用

Controller 用固定 v0.1.1 Go client 对同用户 d2core manager 发出 `list/create/stop/operation/status`。`list` 成功才将 protocolVersion 报为 1。每次领取新任务前读取未终结任务；集成 create job 只携带逻辑模板绑定键和请求 port。Controller 从本机绑定解析 ASCII 绝对模板路径，调用 `prepare` 持久化 key、路径、port 和指纹，校验服务端返回的冻结值，再将原值提交 d2core。key 与不可变 NodeJob ID 确定性绑定。

`accepted` 后保存两个 core ID；仅在 operation 终结且 status 显示 create 为 active/running/ready，或 stop 为 reclaimed/stopped/complete 时报告 `succeeded`。丢失 create 响应保留 `unknown`，不换 key、不盲目再次 create。明确的无副作用 validate/protocol 拒绝可报告 `rejected_no_effect`；已有 core ID 或无法判断副作用时报告 `unknown`。重启和长断联的完整对账属于 P0E 验收。
