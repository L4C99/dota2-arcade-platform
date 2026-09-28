# Validation 历史记录索引

**本目录的 validation records 是 point-in-time historical evidence（历史时点证据）。**文件里的“当前”、pending、`NOT VERIFIED`、`DEFERRED TO A.7` 等描述以各文件形成时为准。P0–P5、A.4、RC1 和 v1.0.1 Hotfix 构成开发与验收历史链；不能从单个旧文件推断现在的 Production 状态，也不应倒写历史文件让它们看似在发布后形成。

**当前结论：**V1 已以 [v1.0.0](https://github.com/L4C99/dota2-arcade-platform/releases/tag/v1.0.0) 正式发布；后续 create-dispatch Hotfix 已以 [v1.0.1](https://github.com/L4C99/dota2-arcade-platform/releases/tag/v1.0.1) 正式发布并完成 Production promotion。A.7 真人试用和 A.8 最终发布门槛均已完成。旧记录中“最终 Release 仍被 A.7 阻塞”等说法是**当时判断**，不是今天的未完成事项。

## 推荐阅读顺序

1. **当前状态和合同：**[根 README](../../README.md)、[Changelog](../../CHANGELOG.md)、[正式 Release](https://github.com/L4C99/dota2-arcade-platform/releases/tag/v1.0.1)；现行操作和接口见[架构](../architecture.md)、[玩家 API](../player-api.md)、[管理员 API](../admin-api.md)、[Node 协议](../node-protocol.md)、[运维](../operations.md)与[发布说明](../release.md)。
2. **冻结产品规则：**[V1 规格](../specs/v1.md)、[Amendment 001](../specs/v1-amendment-001-entry-verification.md)、[Amendment 002](../specs/v1-amendment-002-human-trial-gate.md)。修订明确改变了原规格对应验收语义；旧 validation 只反映其时点。
3. **V1 开发链：**依次阅读 [P0](p0-summary.md) → [P1](p1-summary.md) → [P2](p2-summary.md) → [P3](p3-summary.md) → [P4](p4-summary.md) → [P5](p5-summary.md) 收口，再看 [A.4 定向修复](a4-repair.md) → [RC1 工程候选](rc1.md)。原 A.2/A.3 审查基线 `9bf6d7d6b3195ca8af954a702ed69827f22afc4d` 保留不变。
4. **正式发布与 Hotfix 链：**先看 [v1.0.0 Release](https://github.com/L4C99/dota2-arcade-platform/releases/tag/v1.0.0)，再看 [v1.0.1 create-dispatch 验证](v1.0.1-create-dispatch.md)和 [v1.0.1 Release](https://github.com/L4C99/dota2-arcade-platform/releases/tag/v1.0.1)。Hotfix 文件的 Candidate 正文仍是当时的验证证据；顶部 post-release note 说明其后续发布结果。

历史 migration-18、P5F pending、多平台公网验收或 RC 阶段未完成项，只描述对应检查点。当前运行合同与发布状态分别以现行 docs、根 README 和正式 Release 为准。Production 私有证据保存在仓库外，不复制到本目录。
