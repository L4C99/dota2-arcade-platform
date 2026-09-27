# Dota 2 Arcade Platform

面向 Dota 2 游廊玩家的自助专服平台。玩家以匿名会话选择游戏和玩法，单人或持久 Party 可申请专属服务器；平台排队并选择节点，在 d2core 报告 Ready 且 JoinInfo 有效后提供连接信息，结束或下一局须等待完整回收。管理员可管理内容发布目标、节点准入、维护和经真人确认的一键入口。

**状态：V1 implementation complete；A.4 已完成，当前为 RC1 engineering candidate。** [RC1 记录](docs/validation/rc1.md)说明工程检查、产物与剩余门槛；这不是 production release。A.2/A.3 原审查基线 `9bf6d7d6b3195ca8af954a702ed69827f22afc4d` 永久保留，[A.4 收口](docs/validation/a4-repair.md)记录定向修复。[P5 收口](docs/validation/p5-summary.md)保留开发验收证据。两位真人同一 Party 进入同一真实实例的试用已按 [Amendment 002](docs/specs/v1-amendment-002-human-trial-gate.md)延至 A.7，仍是最终 Release 的必过门槛。

## 组成与边界

| 组件 | 职责与目标平台 |
| --- | --- |
| Platform Server | Go 控制面、玩家与管理员 API、调度、持久 NodeJob 和 PostgreSQL migration。生产基线为 Linux amd64，首个参考发行版为 Ubuntu 24.04 LTS。 |
| Node Controller | Windows amd64 / Linux amd64 节点代理，主动通过 HTTPS 联系 Platform Server，并通过固定 d2core 本地 Go client 管理实例。 |
| Content Tool | Windows / Linux 离线运维工具，准备不可变 VPK 版本并显式切换目录链接；不自动 Drain、下载或发布平台目标。 |
| Web | Vue 3 / TypeScript / Vite 玩家与管理员界面，生产 bundle 由 Platform Server 提供，经 Caddy 对外服务。 |

Platform Server 不直接调用 d2core；游戏流量不经过 Web 控制面。运行依赖 PostgreSQL、Caddy 和每个节点上的 [d2core v0.1.1](https://github.com/L4C99/dota2-arcade-dedicated-core/releases/tag/v0.1.1)（固定 commit `988720ad85af1f0d97bfe98ec4da4fcbb070beea`，protocolVersion 1）。完整数据与组件边界见[架构说明](docs/architecture.md)。

V1 不提供 Steam 登录、公开 Party 发现或匹配、自动下载 VPK、自动防火墙/NAT 配置、活动实例内容热切换或游戏节点容器化。正式内容名称及 production catalog 留待 RC 后、首次外部测试前冻结；仓库和开发数据库中的测试名称不代表正式目录。

## 开发与构建

需要 Go **1.27.1 或更新**、Node.js 22、npm；PostgreSQL 集成测试需要独立的测试数据库。示例配置在 [configs/examples](configs/examples/)，真实 Secret、数据库连接信息、节点路径、VPK 和备份保存在仓库外。Go migration 位于 `internal/platform/store/migrations/`，当前最新版本为 **19**，由 `platform-server migrate` 显式应用；迁移是前向的，恢复流程见[运维说明](docs/operations.md)。

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

设置 `PLATFORM_TEST_DATABASE_URL` 后，Go 测试还会运行 PostgreSQL 集成和升级测试；未设置时相应测试跳过。跨平台和 PostgreSQL CI 定义在 [ci.yml](.github/workflows/ci.yml)。

从干净 checkout 用 Python 3.10+ 执行 `python tools/release.py --output dist/rc1`，再运行 `python tools/release_smoke.py dist/rc1`。构建生成 Linux 三命令、Windows Controller/Content Tool、production Web、三个部署包及 SHA256/manifest；不发布 Windows Platform Server。详情见[打包说明](docs/release.md)。节点单独取得固定 d2core v0.1.1 及其 `BUILD.json`，Platform 包不附带 d2core server binary；Controller 编入的 client 授权及依赖声明见 [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)。

部署参考入口为 [deploy/README.md](deploy/README.md)，具体安装、升级、备份及恢复见[运维说明](docs/operations.md)。Ubuntu 全新 systemd 安装、Windows 无人值守启动与公网入口、生产恢复及生产部署仍须按后续阶段实际验证。`prototype/p1/` 仅保留 P-1 历史视觉与交互参考，使用模拟数据，**不是当前 Web 实现**；正式前端在 `web/`。

## 文档

- [冻结 V1 规格](docs/specs/v1.md)及当前正式的 [Amendment 001：入口验证](docs/specs/v1-amendment-001-entry-verification.md)、[Amendment 002：真人试用门槛](docs/specs/v1-amendment-002-human-trial-gate.md)
- [架构](docs/architecture.md)、[Node 协议](docs/node-protocol.md)、[玩家 API](docs/player-api.md)、[管理员 API](docs/admin-api.md)、[运维](docs/operations.md)
- [Validation 阅读顺序](docs/validation/README.md)与 [P5 收口](docs/validation/p5-summary.md)

本项目采用 [MIT License](LICENSE)。
