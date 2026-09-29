# Future Core + Platform Re-architecture Debt Ledger — Platform

本账本记录已获 Owner 接受、当前版本不要求立即修复的技术债。基线为 Platform `2dee863012821aac9d9f9d492c58c4f24b5f4a6d`；条目描述的是该提交的合同与实现，并非对未来代码的预先裁决。

## 收录与阶段规则

1. 本文件不是当前 release blocker 清单。只有已明确裁决为 `DEFERRED`、`ACCEPTED DEBT`、`FUTURE RE-ARCHITECTURE` 或 `NON-BLOCKING HARDENING` 的事项才能成为正式条目；普通功能愿望与未经裁决的 review 提议不能进入。
2. `REJECTED` finding 留在历史 review/validation 中，不改名为债务。已 `FIXED` / `CLOSED` 的 finding 不作为未修债务重复登记。
3. `CURRENT HARD GATE` 仍须按其原阶段完成，登记或提及于此不得使其延期。特别是 I3 / 任何中间部署前的 B-M1：`validation_contract=v1_0_2` 的玩家调度必须逐 `Node × Preset × formal content/template combination` 检查有效 PASS。
4. 每次阶段 review 后，Owner 一旦裁决某项为未来 `DEFER`，该阶段 closure 必须同步将其来源、边界与触发条件登记在本文件；近期阶段承接项继续留在本阶段，不因出现 `DEFER` 一词自动成为未来重构债。
5. 本账本不自动授权实现、部署或改变固定 d2core v0.1.1 合同。真正重构时须重新核对当时的代码、core 能力与威胁模型，不可机械照抄旧方案；任何冻结 V1 不变量的变更仍需正式 Owner 方向。

## PLAT-DEBT-001 — 无可信身份的 unknown create 缺少自动 no-effect 证明

- **Status:** `ACCEPTED DEBT` / `FUTURE RE-ARCHITECTURE`。
- **Source:** [A.4 FIX-05 Owner acceptance](validation/a4-repair.md)、[运维 Known Limitation 与 Owner Acceptance](operations.md)、[v1.0.2 安全合同](specs/v1.0.2-content-maintenance-validation.md)。
- **Current scope:** 固定 d2core v0.1.1 下，已发出、响应未知且没有可信 instance/operation ID 的 create 不能自动证明无副作用；延迟的原请求仍可能产生实例。I2 的 durable `operation-start` 后即使实际 core call 尚未发生，也先按 MAY-HAVE-STARTED 占容；[Owner execution-fence amendment](specs/v1.0.2-execution-fence-amendment.md)明确保留此边界。
- **Why not fixed now:** Owner 已接受 V1 安全优先的可用性残余限制；自动回收需要另起版本设计并审查可信 recovery proof primitive，不能从空 `list`、重试拒绝或 history 过期推断无副作用。
- **Current safety boundary:** 原 `NodeJob`/key/冻结请求与 `Allocation` 历史不变；`unknown` 与 `quarantined` 占容。管理员审计隔离、Owner escape、Node Drain/退役操作路径保持可运营；若后来取得可信 ID，仍按原身份 stop 并在 d2core 确认 `reclaimed/stopped/complete` 后释放。
- **Future direction:** Core + Platform 共同定义可持久验证的 operation/no-effect 证明及其过期、重启、迟到响应语义，再设计安全的自动收敛；不得通过放宽现有容量释放谓词实现。
- **Revisit trigger:** core protocol/recovery proof primitive 升级、Core + Platform 重构，或长期 unknown 占容已构成不可接受的可用性问题。
- **Related current work:** L-D3 所述 marker 已持久但 core call 未发生且 history 过期时的审计人工 reconcile / quarantine / owner escape 属 I3/Operations 交付，不因本条延期。

## PLAT-DEBT-002 — Windows ownership lock 的 reparse-point / TOCTOU 深化防护

- **Status:** `DEFERRED` / `NON-BLOCKING HARDENING`。
- **Source:** Owner 本轮提供的 I2 focused Delta Review `L-D1` 裁决；仓库中的 [I2 validation](validation/v1.0.2-i2-controller-proof.md)、[execution-fence amendment](specs/v1.0.2-execution-fence-amendment.md)及当前 [Windows lock 实现](../internal/controller/ownership/lock_windows.go)核实其实现与信任边界。`L-D1` 原始独立审查文本未纳入本仓库；本条的延期裁决依据为本轮 Owner 明示，不伪称仓库中存有该报告。
- **Current scope:** `Path` 解析受控父目录，Windows `acquire` 先 `Lstat` 拒绝明显 symlink，再以 `os.OpenFile` 打开锁文件并对所得 handle 调用 `LockFileEx`。检查和打开分步进行；当前实现未做更严格的 reparse-point 句柄级核验。
- **Why not fixed now:** Node host operator 与 Node Secret holder 在当前威胁模型中是可信节点主体；ownership lock 的正式用途是防止 duplicate supervisor / restart overlap，I2 合同要求的 `LockFileEx` 已实现。更强的本地路径攻击防护属于 defense-in-depth。
- **Current safety boundary:** 锁路径来自本机 d2core data-dir 的受控父目录，不由 Admin/Web 指定；Controller 在 capability session/Runner 前取得排他锁并持有到进程退出。旧 Controller 升级遵循 stop old → confirm exit → start new 的运维顺序。
- **Future direction:** 评估基于 Windows `CreateFile` 选项、reparse-point handle 与最终文件身份的核验，消除检查到打开之间的路径替换窗口；保留 OS 排他锁语义。
- **Revisit trigger:** Node host operator 不再可信、节点目录权限模型变化、多 Controller 进程/宿主机所有权模型变化，或 Core + Platform 重构。
- **Related current work:** L-D2 的父目录存在、受控、可写及 Linux 非 world-writable 检查，和 L-D4 的实际 d2core data directory 唯一 NodeID/Controller 归属，均是 deployment/runbook prerequisite，必须按当前部署路径完成。

## PLAT-DEBT-003 — Store 内部写入 API 缺少已验证 capability context

- **Status:** `DEFERRED LOW` / `NON-BLOCKING HARDENING`。
- **Source:** [I2 validation 对 Reviewer B MEDIUM-2 的 Owner 裁决](validation/v1.0.2-i2-controller-proof.md)；当前 [Node HTTP 授权入口](../internal/platform/httpapi/node.go)和 `ReportJob`、`PrepareCreateWithManifest`、`ReportInstanceFact` Store API 核实调用边界。
- **Current scope:** 这些 Store 内部 API 的签名只接受 Node/Job/Allocation 数据，未携带经过 HTTP session fence 验证的 capability context；若未来新增调用路径，调用者需记得先做同等授权。
- **Why not fixed now:** I2 裁决确认现有 Node HTTP routes 已按本次请求的 capability 过滤或授权；进一步在 Store 类型/API 中编码已验证授权属于低优先级防御纵深，不是当前 I2 blocker。
- **Current safety boundary:** 当前 Node route 在受保护的请求中先执行 `authorizeJob` / `authorizeAllocation` 或等效 capability gate，再进入对应 Store 操作；`required_capability` 持久且不可降级。缺能力的旧 Controller 不可读取或推进 v1.0.2 Job/Allocation。
- **Future direction:** 设计只能由验证入口构造的 typed authorization context，或 capability-aware Store API，让新调用路径难以遗漏 capability 检查；同时保留 legacy Job 的兼容语义。
- **Revisit trigger:** 增加 Node API/后台调用入口、跨进程 Store 使用、Node API protocol v2、威胁模型改变或 Core + Platform 重构。
- **Related current work:** Reviewer A `L-01` 的 `/reconcile/complete` capability 区分是 `DEFER I3 LOW` 的近期 I3 工作，不属于本条 future debt。

## 边界核对：不进入未来债务池

| 项目 | 当前裁决 / 去向 |
| --- | --- |
| I1 `A-M01`、`B-M2` | [I1 adjudication](validation/v1.0.2-i1-schema-store.md) 已 `REJECTED`；保留历史，不登记为 debt。 |
| I1 `A-M02` | 已在 `f54ffeebbaa03f84bec467d5f0d5ab2a139b23c6` 修复，并有 PostgreSQL 回归及 exact-SHA CI；状态 `FIXED/CLOSED`。 |
| I2 `HIGH-1`、`HIGH-2`；规格复核 `H-01`、`H-02`、`M-01` | 前两项按 [Owner amendment 与 I2 remediation](validation/v1.0.2-i2-controller-proof.md) 修复/收窄合同；后三项由 [delta review](specs/v1.0.2-content-maintenance-delta-review.md) 判 `CLOSED`。不作为未修债务。 |
| `B-M1` | I3 / 任何中间部署的 `CURRENT HARD GATE`，逐 Node × Preset × formal content/template combination 有效 PASS 调度检查；不得因本文件延期。 |
| `L-D3`；Reviewer A `L-01` | 前者是 I3/Operations 的审计人工 escape；后者是 `/reconcile/complete` 的 `DEFER I3 LOW`。二者都是近期承接项。 |
| `L-D2`、`L-D4` | 分别为锁父目录部署条件与 data directory 的唯一 NodeID/Controller 归属；属于 deployment/runbook prerequisite。 |

## 待裁决线索（`UNADJUDICATED — DO NOT ENTER LEDGER`）

Owner 提供的 I1 review 检索线索——非终态 `ValidationRun` 更强的 DB transition monotonicity/transition graph、`maintenance.begin` retry/idempotency ergonomics、inventory duplicate scan 的更明确冲突语义、template binding update 的审计/追踪完整性——在本仓库 I1 validation、规格与 Git 历史中未找到逐项 Owner `DEFER` 裁决。现有资料描述了 I1 已实现的约束、request ID 幂等、重复 scan 冲突与事实 revision，但这些描述本身不能证明仍有被接受的未修债。原 review 若另存于仓库外，需在 Owner 明确逐项裁决并核对当前实现后，才可补入正式条目。
