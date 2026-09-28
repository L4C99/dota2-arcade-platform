# V1 当前架构

[冻结 V1 规格](specs/v1.md)、[Amendment 001](specs/v1-amendment-001-entry-verification.md) 和 [Amendment 002](specs/v1-amendment-002-human-trial-gate.md)定义产品与验收边界。本文说明当前 v1.0.1 的组件和运行关系；当前发布状态见[根 README](../README.md)。

```text
Browser ─HTTPS→ Caddy ─loopback HTTP→ Platform Server ─→ PostgreSQL
                                     ↑
                                     │ HTTPS，Node API v1（Controller 主动连接）
                                     │
             Node Controller ─本地 API/client→ d2core v0.1.1 ─→ Dota instances
                    │ 读取
             节点内容文件 ← Content Tool（离线，运维人员显式调用）
```

- `cmd/platform-server/`、`internal/platform/httpapi/` 和 `internal/platform/store/` 实现单实例 Platform、HTTP API、调度状态和数据库访问。PostgreSQL migration 内嵌于 `internal/platform/store/migrations/`，当前最新版本为 19。`ServerRequest`、顺序 `Allocation` attempts、持久 `NodeJob` 与 d2core instance 是不同身份。
- `cmd/node-controller/`、`internal/controller/` 和 `internal/contracts/nodev1/` 实现 Windows/Linux Controller 与 Node API v1。只有 Controller 通过固定 d2core v0.1.1 的正式本地 Go client 调用 d2core。create 在首次 core 调用前冻结请求及 key；有副作用歧义时保留 `unknown` 并对账，容量只在安全无副作用终结或完整回收后释放。
- v1.0.1 中，Controller 每轮读取持久任务，并用有界 worker 推进独立 NodeJob：worker 数取配置的 `hard_max_instances`，最少 2、最多 32；单个任务的延迟或传输错误不取消其他独立任务。Platform 的 claim 事务排除同一 Allocation 或已知 instance 上的其他 open Job。独立 Allocation 的 create 因而可以同时推进；PostgreSQL Allocation 预留仍按 `min(hard_max_instances, desired_max_instances)` 限制新实例容量。每轮 worker 退出后，下轮或重启都从持久 NodeJob 与 d2core 身份恢复。
- `cmd/content-tool/` 与 `internal/contenttool/` 实现 Windows/Linux 离线内容版本工具。它不自动 Drain、调用 Platform 或发布目录目标。Controller 读取节点实际内容及 SHA256；Platform 为未来 Allocation 记录发布目标。V1 以 Node 为内容版本隔离边界，不在活动实例之间热切换同一 Node 的版本。
- `web/` 是当前 Vue 应用，只访问 Platform；`prototype/p1/` 是保留的历史 mock。Dota 客户端的游戏流量直接到游戏 Node，不经过 Caddy 或 Platform。
- `deploy/` 提供 Caddy、systemd、Windows 启动与 PostgreSQL 备份资产。实际升级、Drain、回滚与恢复按[当前运维说明](operations.md)执行。

接口和发布流程分别见 [Node 协议](node-protocol.md)、[玩家 API](player-api.md)、[管理员 API](admin-api.md)及[Release 说明](release.md)。开发与验收过程的时点记录见[Validation 索引](validation/README.md)。
