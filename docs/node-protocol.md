# Node API v1（P0C/P0D）

Node Controller 主动连接 Platform Server 的 `/api/node/v1`。正式环境经 Caddy 使用 HTTPS；Platform Server 只接收 Caddy 转发的本地 HTTP。开发模式只允许 Controller 访问 loopback HTTP。生产不关闭 TLS 证书校验。

每个请求携带 `X-Node-ID: <uuid>` 和 `Authorization: Bearer <node-secret>`。节点由 `platform-server node register <display-name> <windows|linux>` 显式登记。原始高熵 Secret 只在登记时输出一次；数据库保存 SHA-256 verifier。Controller 从独立的本机 Secret 文件读取原值，配置文件和日志不包含 Secret。

## 心跳与节点事实

`POST /heartbeat` 发送 `Heartbeat` JSON：OS、Controller 版本、Node API 版本、d2core `BUILD.json` 版本与 commit、已建立的本地 protocolVersion、`hardMaxInstances`、本机网络映射以及内容读回事事实。服务端使用自己的 UTC 时间记录 `reportedAt`。只有 Node API v1、固定 d2core `0.1.1` / `988720ad85af1f0d97bfe98ec4da4fcbb070beea` 且本地协议 v1 已验证时，状态为 `compatible`。

Controller 在首次真正连接 d2core 之前上报 `d2coreProtocolVersion=0`；它不能凭期望值伪称连接已验证。`BUILD.json` 不可读时版本为 `unknown`，该节点不领取新任务。

网络事实包含 `connectHost`、可选 `protocolIp`、本地游戏端口范围、`identity` 或完整 `explicit` 端口映射，以及 A2S 部署声明。Controller 与服务端都验证映射覆盖范围、重复公网端口和 `hardMaxInstances <= 端口池大小`。本地端口范围必须与 d2core serve 的 `--port-min/--port-max` 来自同一部署配置；P0/P5 安装验收还要核对实际服务参数。心跳不会自动改防火墙/NAT。

内容事实只在 Controller 能同时读回当前目录链接、metadata 指向和 VPK 文件时报告 `confirmed`；读不到或不一致时报告 `unknown`。P0C 保存原始节点事实快照供诊断；P1B 将建立正式 `NodeContentBinding` 调度模型。Web/Admin 不能通过节点 API 伪造这些事实。

## durable NodeJob

- `GET /jobs/open`：列出本节点所有未终结任务及已冻结执行参数。Controller 启动或重连时先读取并对账。
- `POST /jobs/claim`：在兼容且近期有心跳的节点上，事务性领取最早的 pending job。无任务返回 204。
- `GET /jobs/{id}`：查询本节点任务；其他节点的任务返回 404。
- `POST /jobs/{id}/prepare`：create job 在首次 d2core 调用前提交已解析的本机模板绝对路径和请求 port。服务端从不可变 `node_job_id` 派生 d2core idempotencyKey，并将完整请求参数及 SHA-256 指纹持久化。相同 prepare 可重试；任何变化均拒绝。数据库触发器禁止修改或删除冻结记录。
- `POST /jobs/{id}/report`：幂等报告 `accepted`、`unknown`、`succeeded`、`rejected_no_effect` 或 `failed_with_effect`，并保存 instanceId/operationId 和结构化 core 错误。已知 ID 不允许换成另一个 ID；终态不能回退。

P0 的任务明确标记 `integration_only=true`，由本地管理员流程创建；P1 才引入 ServerRequest / Allocation 关联、排队和容量业务。timeout 或响应丢失必须报告 unknown 并对账，不能把它当作无副作用失败。d2core `accepted` 不是完成；Controller 必须再查 operation 与 status。

## Controller 本地配置

Controller 使用显式绝对路径 JSON 配置，其中包含 Platform URL、Node ID、Secret 文件路径、d2core `BUILD.json` 和 data-dir、端口映射、硬上限、逻辑模板到本机绝对路径的绑定，以及可选内容 metadata/当前链接位置。所有 d2core 关键路径须为 ASCII 绝对路径。节点 Secret、真实节点路径、端口和部署参数保存在仓库外的开发或生产专用目录。源码工作树不作为生产运行目录。

## P0D 本地核心调用

Controller 用固定 v0.1.1 Go client 对同用户 d2core manager 发出 `list/create/stop/operation/status`。`list` 成功才将 protocolVersion 报为 1。每次领取新任务前读取未终结任务；集成 create job 只携带逻辑模板绑定键和请求 port。Controller 从本机绑定解析 ASCII 绝对模板路径，调用 `prepare` 持久化 key、路径、port 和指纹，校验服务端返回的冻结值，再将原值提交 d2core。key 与不可变 NodeJob ID 确定性绑定。

`accepted` 后保存两个 core ID；仅在 operation 终结且 status 显示 create 为 active/running/ready，或 stop 为 reclaimed/stopped/complete 时报告 `succeeded`。丢失 create 响应保留 `unknown`，不换 key、不盲目再次 create。明确的无副作用 validate/protocol 拒绝可报告 `rejected_no_effect`；已有 core ID 或无法判断副作用时报告 `unknown`。重启和长断联的完整对账属于 P0E 验收。
