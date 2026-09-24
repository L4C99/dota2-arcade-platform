# Dota 2 Arcade Platform

面向 Dota 2 游廊玩家的自助专服平台。

**V1 P-1 UI / UX 原型已由项目所有者验收通过。**冻结的产品与架构规格见 [docs/specs/v1.md](docs/specs/v1.md)。P0 正按 P0A → P0E 顺序实施。

[prototype/p1/](prototype/p1/) 保留最终视觉与交互参考；仅使用模拟数据，不是生产前端，也不连接真实服务。

## 开发基线

Go 版本至少为 1.27.1。当前阶段可用以下命令检查源码：

```text
go test ./...
go vet ./...
go build ./cmd/platform-server ./cmd/node-controller
```

开发与未来正式部署使用独立的数据库、数据目录、Node Secret 和运行配置。源码工作树不是生产运行目录；正式部署使用独立构建产物与部署目录。真实凭据、节点配置和游戏资产不纳入 Git。
