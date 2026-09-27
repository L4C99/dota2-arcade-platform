# A.4 — Adjudicated Targeted Fixes & Regression

日期：2026-09-27。原 Candidate：`9bf6d7d6b3195ca8af954a702ed69827f22afc4d`。本次方案 A 续作 starting HEAD：`7497c9d6047318f7c066d0f7f7300ed5126409b4`；既有 FIX-01～04、`eea15d5` STOP 检查点及 `7497c9d` 反例记录全部保留，未 reset/rebase/squash/rewrite。

执行依据为 Owner 原 A.4 请求、A.3 §12 和 Owner 正式接受的 Supplemental Adjudication 方案 A。未修改原 A.3/补充裁决历史。固定 d2core v0.1.1 / `988720ad85af1f0d97bfe98ec4da4fcbb070beea` 未修改或升级。

## 修复闭环

| FIX | ADJ | Commit | Regression | Result |
|---|---|---|---|---|
| 01 | 012 | `644dc57` | terminal Allocation 后迟到 accepted/unknown/failed，重复报告及下一局 | PASS |
| 02 | 032 | `bb62032` | unknown 后仅独立 stop 可领取；同实例/未解决 create 依赖拒领 | PASS |
| 03 | 034 | `ca27c09` | succeeded create 后可信 failed instance 派 stop；未知身份继续占容 | PASS |
| 04 | 033 | `9568392` | 旧 failed stop worker 退出后重试；running stop 复用；本 Job operation 不替换 | PASS |
| 05 | 011 | `8af6da3` | 固定 Store 延迟 A/B 反例、拒绝码/空 list/expiry 不释放、在线隔离、Owner escape、迟到报告和最终 reclaim | PASS — operational mitigation / safety boundary accepted |
| 06 | 001 | `39f156a` | never-claimed pending 终结、claim 优先、并发唯一胜者；审计与历史保留 | PASS |
| 07 | 002/003 | `85b3158` | next-game 消费时 Party 人数、全局/地图/玩法维护、disabled preset/manual node；拒绝不回滚旧 reclaim/abandon | PASS |
| 08 | 013 | `eb814e7` | terminal create 后 JoinInfo 补报可达；identity/Ready/revision/port 校验、首次时间不变 | PASS |
| 09 | 035 | `0e3cb1c` | expected revision 缺失/陈旧拒绝验证及开放，无成功审计；新 revision 正常验证 | PASS |
| 10 | 004/005 | `a173062` | 专用可信代理头；伪造头忽略；20,000-key flood 不清空/逐出活跃惩罚，过期键回收 | PASS |
| 11 | 006 | `6f14d15` | PostgreSQL revoke UPDATE 故障：logout 503、不清 cookie；rotation 插入/撤销同事务回滚；UI 保留身份 | PASS |
| 12 | 014/036/031 | `9a6e039` | 挂载 Vue：offline 有效 JoinInfo 保留并警告；unavailable 重新选择只清旧指针；current 优先；计时明确为申请总时长 | PASS |
| 13 | 007/008 | `1ccc885` | 100 silent Ready 实例下两秒总预算；未查询/取消不制造失败；Job 独立 timeout | PASS |
| 14 | 009/010 | `9577e30` | 共享 Workshop/version grammar；边界/保留名称/路径字符、写前拒绝无审计；开发旧数据异常数 0 | PASS |
| 15 | 026 | `f3d2b0f` | resolved addon subtree 写前拒绝，Windows Junction/Linux symlink、合法前缀同级目录；无 release 写入 | PASS |
| 16 | 022/025 | `4b06c1b` | Windows ValidateOnly 缺 content-tool 拒绝、完整 fixture 通过；Linux jq 缺失预检 | PASS |
| 17 | 028/029 | `1a77518` | 挂载 Vue：dirty 草稿跨 refresh 保留、服务器变更要求同步；503 重试保留登录、401 重新登录 | PASS |

## FIX-05 的证明边界与 Owner acceptance

ADJ-011 原事实保留：unknown create 没有自动 no-effect proof。HIGH 的 V1 发布阻塞通过 fail-closed、audited quarantine、Owner escape、retained capacity/history、node retirement runbook、正式 regression、明确 Known Limitation 和 Owner acceptance 完成运营闭环。**Residual limitation accepted for V1.** 不称为“自动 no-effect recovery 已修复”。

无可信 IDs 的已发出 unknown create 不因 retry rejection、反复空 list、manager restart、相同路径/marker 或 history expiry 释放。禁止合成 `RECONCILED_NO_EFFECT`。旧 key 后来取得 IDs 时走原 identity → status/operation → stop/full reclaim。纯 pending 未 claim 的 reservation 使用 FIX-06 独立安全终结规则，不放宽 quarantine。

`tests/coreproof` 直接使用固定 core Store；确定性 barrier 模拟同 manager 中请求 A 已读入、Respond 前挂起，B 同 key 先返回 NO_PORT_AVAILABLE，随后 A 创建成功。它是正式长期保留的调度反例，**不是完整 IPC 或真实 Dota 测试**。Platform PG 回归独立证明 B 的拒绝不得释放旧 unknown Allocation。Runner/PG/既有 HTTP 回归覆盖 unknown 占容、在线 Admin 隔离、Owner/leader 二次确认与幂等 abandon、非 Owner 禁止、新独立 Request、迟到旧报告及最终三元 reclaim。

完整运营路径、节点禁止重注册恢复账面容量、备用节点/独立授权重建边界及 Owner 接受文字见 [operations](../operations.md)。本次未退役或重建任何节点。

## 协议、文件与迁移

主要实现位于 `internal/platform/store`、`internal/platform/httpapi`、`internal/controller/runner`、`internal/controller/network`、`internal/contenttool`、`internal/contracts/contentid`、`web/src` 和 `deploy`。Node API 增加受限 independent-stop claim 和 Ready JoinInfo fact；Admin API 增加 pending 终结及 expected entry revision，见正式 API 文档。

唯一新增 migration：`0019_a4_next_game_eligibility.sql`，给 next_game_intents 增加 nullable failure_reason。0001～0018 未编辑。18→19 升级回归保存既有合法 Request/Allocation/NodeJob JSON 快照、Catalog 与当前入口 attestation，并验证重复升级。开发 DB 在受限路径备份后升级到 19。二进制回滚不等于数据库回滚；不删除不可变版本或资源历史。

## 自动验证

- Windows `go test ./... -count=1`，启用独立 disposable PostgreSQL 16.4：PASS。Store 41.823s，HTTP 3.769s；完整 fresh migration、Store/HTTP integration 和 upgrade tests。新增历史快照升级回归另行 PASS。
- `go vet ./...`：PASS。Windows amd64 与 Linux amd64 三命令 build：PASS；Linux 本地交叉构建不冒充 Linux runtime 测试。
- `go -C tests/coreproof test -mod=readonly -count=1 ./...`：PASS。
- lockfile `npm ci`、lint、typecheck、27 项 Web 测试、production build：PASS。组件测试使用 jsdom 真实挂载 Vue。
- gofmt、git diff --check：PASS。Windows ValidateOnly fixture 不安装服务/任务。
- 实现 SHA `1a775188d9e9ed819dba4873e5f3dcb4cc809655` 的 [CI 36306537049](https://github.com/L4C99/dota2-arcade-platform/actions/runs/36306537049)：Ubuntu Go、Windows Go、Web、PostgreSQL 四项均 success，包括两 OS 的固定 core counterexample。
- 本报告与升级快照回归提交后的最终 SHA 必须再次四项 CI 全绿，再冻结；最终 SHA/run URL 在本次交付回执中记录，避免在提交内自引用 SHA。实现 SHA 结果不替代最终文档提交的 CI。

环境诊断如实保留：沙箱最初阻止 esbuild 子进程，获准执行后测试/build 通过；Go telemetry/stat-cache 有拒写警告但命令退出 0。npm ci 报告两项 moderate dependency advisories，未做跨版本强制升级。

## 真实开发专项

所有自动回归通过后才访问已记录的开发 Web/DB、Linux game node、Windows VM。初检两节点 occupied=0、open jobs=0、活动 Request=0，core list 各为空。两节点先 Drain；不改 firewall/NAT/router、App570、VPK、入口 revision 或固定 core。Catalog 既有 Workshop/version 非法记录计数均为 0。

开发 Platform 与两 Controller 使用干净实现 SHA `1a77518` 新文件名构建，传输 SHA256 一致；开发 DB 备份后迁移到19。开发 Web 未换包；前端修改由本地 clean build/组件测试及 CI 验证。没有生产部署。

| 环境 | Request / Allocation | Instance | 结果 |
|---|---|---|---|
| Linux 新图 custom | `d79b2a50-3dd1-4f91-821e-eede3d577b44` / `a3a1e65f-7e9c-46bf-b5d2-47523682d6a3` | `i_635f28412292d1b319f6c927a3fdb7b1` | 正式 Player create → Ready/JoinInfo；Owner 当次确认 connect 成功；Controller 重启后原 ID/PID 保持；正常 stop → full reclaim |
| Windows 旧图 n6 | `4855231a-449b-44ad-846c-3692ff1fe884` / `c2871a89-d494-4674-ae63-45f95e20cade` | `i_29c3a8896d461362e6a5c716d4126219` | create → Ready/内网 JoinInfo；Controller 重启后原 ID/PID 保持；正常 stop → full reclaim |

Windows 新图 binding 本来未开放，最初等待请求 `a2c3ac4c-5054-4359-9ccc-0929dc5959f0` 在无 Allocation 时取消，随后改用已开放旧图；没有放宽 admission 或切换内容。两实例均由固定 core 确认 `lifecycle=reclaimed/process=stopped/cleanup=complete`，create/stop Job 均 succeeded，core list 为空。最终 occupied/open jobs/活动 Request 为 `0/0/0`；reconcile Linux `15/15`、Windows `7/7`。原 Drain=false、desired=1 恢复，地图 binding 原值保留。

Linux Steam/steamchina 两套原 verified/enabled 均仍为 true；Windows 原四项 false 保持。两节点入口 revision 前后相同。临时测试管理员已 logout、disabled，活跃会话已撤销。Windows Controller 沿用原前台 SSH 运行方式，没有安装无人值守服务。

## 保留限制及阶段边界

- unknown 无自动 no-effect proof，容量可能永久占用：V1 安全优先限制已由 Owner 明确接受，不能写成已回收。未来 recovery proof primitive 需另起版本设计审查。
- 不破坏真实 d2core 状态来强行制造 stop failure；此边界由隔离 regression 覆盖。
- Windows 公网/真人连接、两真人 Party 验收、全新 Ubuntu 安装、Windows 无人值守重启、生产 TLS/恢复/部署未验证，仍属既有后续门槛。
- 本次没有 RC1、Tag、Release、生产操作、core API 或 d2core v0.1.2。
- 保留原 Candidate 历史。最终交付须工作树 clean、origin/main=HEAD，且该 SHA 四项 CI success，才成为 Post-A.4 Candidate。建议下一阶段是单独授权的 RC1；本 A.4 不开始它。
