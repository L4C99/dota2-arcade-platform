# 正式 Release 构建与发布

> `release.publish` 是游戏内容/玩法正式组合的数据库事务，不是 Git tag、GitHub Release 或 Production deployment。当前正式资产为 [v1.0.2](https://github.com/L4C99/dota2-arcade-platform/releases/tag/v1.0.2)，Production 保持 `v1.0.2-i4`。Owner 于 2026-10-01 取消 Final Release Adoption；本次资产发布不重启或替换 Production。项目进入 Feature Freeze / Maintenance Only。

## v1.0.2 已冻结资产

正式 Release 必须使用原始 `v1.0.2-final-20260930-b2` 的 11 个发布文件，固定源码/tag `23b76126fad250d1b476d06411e981a017f236ab`、version `v1.0.2`、buildTime `2026-09-30T13:32:01Z`。不重新构建、不重新压包、不修改 BUILD/MANIFEST/SHA256SUMS，不用 CI alignment artifact 或 I4 artifact 替代。构建遗留 staging 目录不属于发布资产。

Production I4 的真实 Linux 运行/OMG N7 真人闭环与 clean b2 的 Windows native/offline、隔离 Linux native/package 资格是两组证据。b2 没有完成 Production adoption，不能由正式发布推断已部署或通过其自身真人 I4。完整身份、hash、GitHub 下载读回结果及限制见[正式发布记录](validation/v1.0.2-final-release.md)。

v1.0.2 tag 保持在构建源码；发布后最小文档收口在 main 另作提交。冻结包内文档保留构建时点内容；当前发布状态以 Release 说明、main README 与正式发布记录为准。以下是通用构建流程，不授权重构建这批正式资产。

从目标源码 SHA 的**干净 checkout** 构建。需要 Go 1.27.1 或更新、Node.js 22/npm、Python 3.10 或更新。正式构建必须显式传入版本：`tools/release.py` 当前默认值仍是历史 RC 标签 `v1.0.0-rc1`，不能依赖默认值。

```text
python tools/release.py --version <VERSION> --output dist/<OUTPUT>
python tools/release_smoke.py dist/<OUTPUT>
```

`<VERSION>` 与 `<OUTPUT>` 由本次发布明确指定；输出目录不能预先存在。脚本拒绝脏源码，构建五个二进制、Production Web、三个部署包及独立 Web 包，并运行干净 `npm ci`、lint、typecheck、测试和构建。结束时再次检查源码身份。`release.py` **只 build/package**，不创建 tag、不发布 GitHub Release、不部署、不下载 d2core，也不接触 Dota。`dist/` 被 Git 忽略。

在 Linux 与 Windows 分别运行原生 smoke；交叉编译不能证明目标系统可运行。三个命令的 JSON `version` 给出准确 Git commit、dirty 状态、UTC buildTime、OS/arch 和 Go 版本。Controller 的离线 `check --config ABS` 只验证配置与 Secret 格式，不联系 Platform/d2core，也不证明节点、内容或游戏可用。

## 产物与完整性

`MANIFEST.json` 记录文件名、字节数、SHA256 和源码 SHA；`SHA256SUMS` 覆盖全部产物与 manifest。v1.0.2 包内 `PLATFORM-BUILD.json` 记录 Node API 1、最新 migration 22 和固定 d2core 依赖；`web/BUILD.json` 标识配套 Web bundle。`MIGRATIONS-SHA256SUMS` 记录内嵌 SQL 的字节校验，0001–0022 保持不可变。Migration 22 只增加 Admin I3 request-id 成功账本，供 validation.start 与 release.publish 幂等使用；升级、重复执行和备份恢复须通过独立 PostgreSQL 验证。本次 GitHub Release 不执行任何 Production migration。

| 包 | 运行内容 |
| --- | --- |
| `control-plane-linux-amd64.tar.gz` | platform-server、Web、内嵌 migration、文档、部署/示例配置及许可证资料 |
| `game-node-linux-amd64.tar.gz` | node-controller、content-tool、d2core 启动参考脚本、systemd/配置参考及许可证资料 |
| `game-node-windows-amd64.zip` | Windows Controller/Content Tool、启动脚本、配置/文档参考及许可证资料 |
| `web-any.tar.gz` | Production Web、构建身份及许可证资料 |

五个独立可执行文件也有校验值，分发时应同时提供配套许可证/声明。Linux tar 条目保留可执行权限。打包只允许 Git 跟踪的公共参考文档和配置；私有运行配置、`.local`、`node_modules`、VPK、备份、日志和凭据不进入包。部署包保留 `licenses/`、根 `LICENSE`、`THIRD_PARTY_NOTICES.md` 与 `DEPENDENCIES.json`。CI 上传的是临时 artifact，不自动创建 GitHub Release。

固定 SHA 的 CI manifest 是该次构建的产物身份。各次本地重建会有自己的 buildTime 和 checksum，不能混用 manifest。交接记录应包含完整源码 SHA 与运行链接；commit 无法包含自身 SHA 或依赖该 SHA 的二进制 checksum。

## 固定 d2core 获取与核验

节点单独从正式 [d2core v0.1.2 Release](https://github.com/L4C99/dota2-arcade-dedicated-core/releases/tag/v0.1.2)取得对应 ZIP 与官方校验文件；Platform 包不分发 d2core server binary。核对官方 `SHA256SUMS`、`BUILD.json` 与 `d2core version --json`：版本 0.1.2、commit `6dddb5892f962e70beb32fc30df4a78bce595528`、protocol/schema/format 1/1/2、一致的 buildTime、`gitDirty=false`。d2core 的 `BUILD.json` 与平台的 `PLATFORM-BUILD.json` 分开保存，不用 d2core `main` 或改名 RC 包替代。Controller 构建还须以 `go version -m` 确认实际嵌入的 Go module 为 v0.1.2。正式基线由[alignment amendment](specs/v1.0.2-core-v0.1.2-alignment.md)限定。

RC1 当时使用 v0.1.1 的许可和 ZIP 校验属于[历史 RC1 验收](validation/rc1.md)，不能用于校验 v0.1.2 包。历史源码/client 的授权证据 `db246b2bcce888b87d7854bb12012ea4e90e82cb` 与 `licenses/d2core-LICENSING.md` 保留。当前固定 Release 自带正式 MIT LICENSE，与 `licenses/d2core-LICENSE.txt` 一致；编译进 Controller 的 client/module 授权随 Platform 包分发。

## 发布边界与历史

正式 tag、GitHub Release、Production promotion 和回滚都是单独授权的动作；打包成功本身不执行这些动作。部署和回滚操作见[运维说明](operations.md)。已发布的 [v1.0.0](https://github.com/L4C99/dota2-arcade-platform/releases/tag/v1.0.0) 与 [v1.0.1](https://github.com/L4C99/dota2-arcade-platform/releases/tag/v1.0.1) 产物及说明保存在各自 Release 页面；版本变化见[Changelog](../CHANGELOG.md)。RC1 的构建与当时未完成门槛属于[历史验收记录](validation/rc1.md)。

固定 d2core 下，无可信 ID 的 unknown create 无安全自动 no-effect 证明：保留 quarantine、Owner escape、占用容量和历史，必要时走单独授权的节点退役/重建流程。`quarantined` 不能当作已回收；操作规则见[运维说明](operations.md)。
