# A.4 定向修复检查点 — BLOCKED

日期：2026-09-27。A.4 **未完成**，没有 Post-A.4 Candidate，没有进入 RC1。

原 Candidate、starting HEAD、执行 `git fetch` 后的 origin/main 均为
`9bf6d7d6b3195ca8af954a702ed69827f22afc4d`；起始工作树干净。
唯一 finding 输入是 Owner 提供的 `adjudication-9bf6d7d6.md` §12。
没有修改原裁定、历史 validation、migration 0001–0018 或固定 d2core。

## 修复闭环

下表 PASS 表示该 FIX 已实施且所列本地定向回归通过，不代表完整 A.4、双平台 CI 或真实节点验收通过。
后续项 BLOCKED 表示在 FIX-05 停止边界之后未启动，并非重新裁决或降级 deferred。

| FIX | ADJ | Commit | Regression | Result |
|---|---|---|---|---|
| FIX-01 | 012 | `644dc5721019eb5d5bdf8bc83afdfd9d34636b93` | PG：reclaim 后 late accepted/unknown/failed、重复报告、有/无下一局；released attempt 后新 attempt 与旧报告 | PASS |
| FIX-02 | 032 | `bb62032ae09a3dc01c967621e9a83b6379c9be58` | Runner：过期 unknown 只领取独立 stop；PG：同实例依赖拒领、无关 stop 回收、原 unknown 不变 | PASS |
| FIX-03 | 034 | `ca27c094ba853b3cdc9ef110b55e5b186e53e9d5` | Runner：成功 create 后 failed/stopped、loading、unknown identity、reclaimed；PG：重复 failed fact 只派一个 stop 且继续占容 | PASS |
| FIX-04 | 033 | `9568392829258b06d8c5ea38e67029e75842ec9b` | Runner：旧 failed worker 暂留后重试成功、running stop 复用、本 job failure 不换 operation；完整 Runner 套件 | PASS |
| FIX-05 | 011 | — | 固定 core 源码核对；既有 unknown 保守回归通过；缺 storage-continuity 生产证明 | BLOCKED |
| FIX-06 | 001 | — | 未执行，停止边界 | BLOCKED |
| FIX-07 | 002、003 | — | 未执行，停止边界 | BLOCKED |
| FIX-08 | 013 | — | 未执行，停止边界 | BLOCKED |
| FIX-09 | 035 | — | 未执行，停止边界 | BLOCKED |
| FIX-10 | 004、005 | — | 未执行，停止边界 | BLOCKED |
| FIX-11 | 006 | — | 未执行，停止边界 | BLOCKED |
| FIX-12 | 014、036、031 | — | 未执行，停止边界 | BLOCKED |
| FIX-13 | 007、008 | — | 未执行，停止边界 | BLOCKED |
| FIX-14 | 009、010 | — | 未执行，停止边界 | BLOCKED |
| FIX-15 | 026 | — | 未执行，停止边界 | BLOCKED |
| FIX-16 | 022、025 | — | 未执行，停止边界 | BLOCKED |
| FIX-17 | 028、029 | — | 未执行，停止边界 | BLOCKED |

## FIX-05：未建立存储连续性证明

Owner 要求：同一 frozen key/request、明确 post-key-lookup、可信当前 storage continuity、可信 history window、无既有 core IDs，全部成立后才能生产 `RECONCILED_NO_EFFECT`。

本机 Go module 的 Origin.Hash 核实为固定
`988720ad85af1f0d97bfe98ec4da4fcbb070beea`，不是 main。
核对的固定源码：

- `internal/records/records.go:323`：CreateChecked 先 ValidateKey/Fingerprint，再查 State.Keys，随后加载模板、check、分配端口。某些拒绝确实位于 key lookup 之后，不能扩大到 protocol/path 预检拒绝。
- `internal/core/storage.go:28`：list.storage 只有 checkedAt、freeBytes、minFreeBytes、historyDays、error；没有 durable storage generation、manager identity 或历史完整性证明。
- `client/client.go:40` 与 `internal/localipc/ipc.go:208`：每次 Call 重新读取 endpoint 并连接，每连接一个请求/响应；持有同一个 Go Client 对象不能证明两次请求来自同一个 manager/storage。
- `docs/local-api.md:9` 与 `internal/localipc/ipc.go:52`：endpoint.json 是文档化的传输发现信息，manager 创建随机地址；文档明确它不是信任凭据。它可能成为进一步研究的 manager 连续性 witness，但当前客户端没有将所调用的 endpoint identity 与响应一起返回，当前冻结 execution 也未保存这样的证据。不能将简单路径相等或两次读取相等未经论证地当作存储连续性证明。

必须区分两个历史：A 为首次调用从未产生实例，B 为首次调用产生实例但响应丢失、随后原存储丢失/替换。
没有连续性证据时，两者都可能在当前空库存和重试的 post-lookup 拒绝中呈现相同 API 结果。
因此同 key/fingerprint + 当前 historyDays + 无已知 IDs 本身不足以排除 B。

本轮没有实现白名单释放、list 空集释放、手动强制释放，也没有修改 core 或猜测进程身份。
这是当前 **API-only 证明方案**的阻塞，不宣称所有可能的旁路连续性 witness 都已被证明不可行。
按 Owner 的技术冲突 STOP 条款，需先明确并论证允许的连续性证据契约，再继续 FIX-05；不能把该项改为 deferred 或宣告 PASS。

## 实际检查

- FIX-01 先写回归：旧实现 6 个子用例全部按预期失败，出现 occupied 0→1 或下一局 owner 唯一约束冲突；修复后通过。
- 新建独立本地 PostgreSQL 16.4 cluster，loopback 独立端口；没有连接历史、开发服务或生产数据库。每个测试使用并清理独立 schema。
- 各 FIX 提交前所列 targeted regression：PASS。
- `go test -count=1 ./...`，设置 disposable PG 的 `PLATFORM_TEST_DATABASE_URL`：PASS；含 Store、HTTP integration 与已有 migration upgrade 测试。Store 35.427s，HTTP 3.383s。
- `go vet ./...`：PASS。
- Windows 三命令 `go build ./cmd/platform-server ./cmd/node-controller ./cmd/content-tool`：PASS。
- Linux amd64 同三命令 cross-build：PASS。不是 Linux runtime tests。
- 已修改 Go 文件经 gofmt。
- Go build 曾输出只读 module stat-cache 写入警告，但命令退出码为 0；不把此警告隐藏为无诊断执行。

## 文件、协议和迁移

主要实现：`internal/platform/store/node_jobs_api.go`、`reconcile.go`、
`internal/platform/httpapi/node.go`、`internal/controller/runner/runner.go`、
`internal/controller/platformclient/client.go`。
新增正式测试：`a4_terminal_test.go`、`a4_stop_claim_test.go`、`a4_recovery_test.go`。
Node API v1 增加可选 `independentStop=true` claim 筛选，见 `docs/node-protocol.md`。
没有新增 migration；latest 仍为 18。没有 Web、Admin API 或 d2core 协议变更。

## NOT VERIFIED / 停止状态

- FIX-05～17 未完成；没有完整 A.4 regression 结论。
- 未执行本轮 Web clean npm ci/lint/typecheck/tests/build；Web 未改动。
- 没有本检查点的 GitHub Actions 证据，也没有最终修复 SHA CI 全绿结论。
- 未连接真实 Linux game node、Windows VM 或生产；没有真实 Dota/JoinInfo/URI/stop failure 专项。mock 与 PG 不替代这些检查。
- 没有新的 18→19 migration，故没有该升级证据；已有 migration tests 通过不代表未来 migration 已验证。
- 原 Candidate 保留。修复提交仅本地保存，未 push；origin/main 仍为原 Candidate，因此 origin/main != HEAD。
- 本报告作为独立文档提交保存；最终检查点 SHA 与 git status 见会话完成报告，避免文档自引用 commit SHA。
- 下一步是解决 FIX-05 的连续性证明契约，然后继续原 A.4 顺序；不是 RC1、部署、Tag 或 Release。
