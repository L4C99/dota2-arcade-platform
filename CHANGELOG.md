# Changelog

## v1.0.1

发布日期：2026-09-27 UTC / 2026-09-28 Beijing。[正式 Release](https://github.com/L4C99/dota2-arcade-platform/releases/tag/v1.0.1)，源码 commit `8733b652372c83beb42a37f964652be9a82a7fd7`。

### Fixed

- 修复同一 Node 上 unresolved ordinary create 对其他 independent Allocation create 的 head-of-line blocking；独立 Allocation 可以并行推进。
- Store claim 保持同一 Allocation 与已知 instance 的冲突排除；Controller 以有界 worker 推进独立 NodeJob。

### 保持的安全边界

- 首次 core 调用前冻结 create request，NodeJob 对应稳定的 idempotency key；`unknown` 不等于 `failed`。
- 保留 FIX-05 的容量占用规则、Controller 重启恢复、实例身份隔离及独立 stop。
- 已受理实例只有经 d2core 确认 `reclaimed/stopped/complete` 才释放容量；Platform 的容量准入仍由 Allocation 预留控制。

### 未改变

- 无数据库 schema 变更；最新 migration 仍为 19，Node API 仍为 v1，d2core 仍固定 v0.1.1。
- 未修改 Steam readiness，也未重设计 scheduler。

### 验证与发布

- 固定 SHA CI、PostgreSQL 集成测试和 race 检查通过；完成三份独立 Hotfix Review。
- 完成 real-node 并发 create 的 After acceptance、Controller 重启恢复、Production promotion 与 post-release smoke；测试实例均完成完整回收。

### 独立已知调查项

- Steam readiness marker 偶发长时间不出现并导致 `START_TIMEOUT`。现有证据不足以确定 Steam、后端、网络或 marker 的根因；不属于 v1.0.1 修复范围。

## v1.0.0

首个正式 Production Release，源码 commit `3a451625e11659ce1e340a5b85e9f3a121d93b99`。[正式 Release](https://github.com/L4C99/dota2-arcade-platform/releases/tag/v1.0.0)。

- 提供匿名 Player flow、持久 Party、自动/手动 Node 选择，以及 Platform、Controller 与固定 d2core 的完整生命周期。
- 支持 Ready 与有效 JoinInfo 后进房、stop/next-game 的完整回收、ContentVersion/Template 管理和 Admin 运维控制。
- A.7 真人试用与 A.8 最终发布门槛均已完成。
