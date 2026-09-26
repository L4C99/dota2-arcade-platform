# Dota 2 Arcade Platform

面向 Dota 2 游廊玩家的自助专服平台。

**P0–P4 已由项目所有者验收，P5 开发进行中。** 冻结的产品与架构规格见 [docs/specs/v1.md](docs/specs/v1.md)，最近完成阶段的事实见 [P4 验收与收尾](docs/validation/p4-summary.md)。P5 开发和测试不等于 V1 Release 或生产部署。

[prototype/p1/](prototype/p1/) 保留最终视觉与交互参考；仅使用模拟数据，不是生产前端，也不连接真实服务。

## 开发基线

Go 版本至少为 1.27.1。当前阶段可用以下命令检查源码：

```text
go test ./...
go vet ./...
go build ./cmd/platform-server ./cmd/node-controller ./cmd/content-tool
```

开发与未来正式部署使用独立的数据库、数据目录、Node Secret 和运行配置。源码工作树不是生产运行目录；正式部署使用独立构建产物与部署目录。真实凭据、节点配置和游戏资产不纳入 Git。

P5 的离线 Content Tool、跨平台参考安装资产与升级/回滚流程见 [deploy/README.md](deploy/README.md) 和 [docs/operations.md](docs/operations.md)。Content Tool 不会自动排空节点、下载内容或发布平台目标版本。

P0B 起，`platform-server` 提供 `migrate`、`serve`、`admin create <username>`、`admin reset-password <username>` 和 `version`。管理员密码从 stdin 管道输入，不作为命令参数。服务启动会先校验并应用显式 migration；失败则拒绝监听。环境变量示例位于 [configs/examples/](configs/examples/)，开发实例只绑定本机回环地址，正式实例默认要求 HTTPS 公网 origin 与 Secure Cookie。

P0C 的 [Node API v1](docs/node-protocol.md) 提供预置节点认证、事实心跳与 durable NodeJob claim/prepare/report。`platform-server node register <name> <windows|linux>` 登记节点并仅输出一次 Secret。`node-controller heartbeat|run --config <绝对路径>` 读取本机配置和独立 Secret 文件。

P0D 的 Controller 使用固定 d2core v0.1.1 Go client 连接同用户本地 API。成功调用 `list` 后才报告 protocol v1；兼容心跳后先读取未终结任务，再领取新任务。集成测试任务由 `platform-server node integration-job <node-id> create <template-binding> [port]` 或 `... stop <instance-id>` 建立。`template-binding` 是 Controller 本机配置的逻辑键，Server 不接收或下发任意本机路径。真实 Linux 开发节点的 create → Ready → stop → full reclaim 验证见 [P0D 验证记录](docs/validation/p0d.md)；P0E 的重启与 unknown 对账见 [P0E 验证记录](docs/validation/p0e.md)。本机真实环境事实保存在 Git 忽略的 `.local/p0-host-inventory.md`；公开字段检查见 [P0 环境清单](docs/validation/p0-environment-checklist.md)。
