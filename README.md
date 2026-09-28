# Dota 2 Arcade Platform

面向 Dota 2 游廊玩家的自助专服平台。玩家以匿名 Session 选择游戏和玩法，单人或持久 Party 可以申请专属服务器；平台排队并选择节点，在 d2core 报告 Ready 且 JoinInfo 有效后提供连接信息。结束或下一局都要等待实例完整回收。

**当前正式版本：[v1.0.1](https://github.com/L4C99/dota2-arcade-platform/releases/tag/v1.0.1)。**该版本已正式发布、部署至 Production，完成发布后 smoke 与完整回收，当前 Production 已开放。`main` 是 v1.0.1 之后的开发与文档维护线；`v1.0.1` tag 固定对应正式发布源码 commit `8733b652372c83beb42a37f964652be9a82a7fd7`。`main` 后续提交本身不表示 Production 已重新部署。

v1.0.1 是仅修改代码的 create-dispatch Hotfix：同一 Node 上未解决的普通 create 不再阻塞其他独立 Allocation 的 create。独立 NodeJob 由 Controller 的有界 worker 推进，Store 仍阻止同一 Allocation 或已知 instance 的冲突领取。它没有改变数据库 schema、Node API、调度架构或 Steam readiness。Steam Ready marker 偶发长时间不出现并导致 `START_TIMEOUT` 是独立调查项；现有证据不足以判定 Steam、后端、网络或 marker 的根因。

## 组成与边界

| 组件 | 职责与目标平台 |
| --- | --- |
| Platform Server | Go 控制面、玩家与管理员 API、调度、持久 NodeJob 和 PostgreSQL migration；Production 基线为 Linux amd64，首个验证发行版为 Ubuntu 24.04 LTS。 |
| Node Controller | Windows amd64 / Linux amd64 节点代理；主动经 HTTPS 联系 Platform Server，并通过固定 d2core 本地 Go client 管理实例。 |
| Content Tool | Windows / Linux 离线运维工具；准备不可变 VPK 版本并显式切换目录链接，不自动 Drain、下载或发布平台目标。 |
| Web | Vue 3 / TypeScript / Vite 玩家与管理员界面；Production bundle 由 Platform Server 提供，经 Caddy 对外服务。 |

运行依赖 PostgreSQL、Caddy 和节点上的 [d2core v0.1.1](https://github.com/L4C99/dota2-arcade-dedicated-core/releases/tag/v0.1.1)，固定 commit 为 `988720ad85af1f0d97bfe98ec4da4fcbb070beea`。Node API version 仍为 **1**，最新 migration 仍为 **19**。Platform Server 不直接调用 d2core；游戏流量不经过 Web 控制面。V1 未引入 Redis、MQ、Kubernetes 或微服务拆分，也不提供活动实例内容热切换。详见[架构说明](docs/architecture.md)。

## 开发与构建

需要 Go **1.27.1 或更新**、Node.js 22、npm；PostgreSQL 集成测试需要独立测试数据库。示例配置见 [configs/examples](configs/examples/)，真实 Secret、节点路径、VPK 和备份保存在仓库外。Migration 位于 `internal/platform/store/migrations/`，由 `platform-server migrate` 显式应用。升级与恢复边界见[运维说明](docs/operations.md)。

```sh
go test ./...
go vet ./...
go build ./cmd/platform-server ./cmd/node-controller ./cmd/content-tool
cd web
npm ci
npm run lint
npm run typecheck
npm test
npm run build
```

设置 `PLATFORM_TEST_DATABASE_URL` 后，Go 测试还会运行 PostgreSQL 集成与升级测试；未设置时相应测试跳过。跨平台检查见 [CI](.github/workflows/ci.yml)。正式打包须从干净 checkout 显式传入 `--version`；构建脚本只打包，不创建 tag 或发布 Release。命令与校验步骤见[Release 构建说明](docs/release.md)。节点单独取得固定 d2core 及其 `BUILD.json`，Platform 包不附带 d2core server binary；依赖授权见 [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)。

部署资产位于 `deploy/`；当前运行、维护和恢复合同见[运维说明](docs/operations.md)。`prototype/p1/` 是 P-1 的历史 mock 参考，正式前端位于 `web/`。

## 文档层级

1. **当前状态入口：**本 README、[Changelog](CHANGELOG.md) 与 [v1.0.1 GitHub Release](https://github.com/L4C99/dota2-arcade-platform/releases/tag/v1.0.1)。
2. **当前运行合同与操作文档：**[架构](docs/architecture.md)、[玩家 API](docs/player-api.md)、[管理员 API](docs/admin-api.md)、[Node 协议](docs/node-protocol.md)、[运维](docs/operations.md)、[Release 构建与发布](docs/release.md)。
3. **冻结产品规格：**[V1 规格](docs/specs/v1.md)、[Amendment 001：入口验证](docs/specs/v1-amendment-001-entry-verification.md)、[Amendment 002：真人试用门槛](docs/specs/v1-amendment-002-human-trial-gate.md)。
4. **历史开发与验收证据：**[Validation 索引](docs/validation/README.md)。其中旧文件的 pending / NOT VERIFIED 结论属于形成时的时点记录，不能单独用来判断当前 Production 状态。

本项目采用 [MIT License](LICENSE)。
