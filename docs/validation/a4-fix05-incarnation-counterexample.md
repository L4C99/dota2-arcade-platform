# FIX-05：同 manager incarnation 下的延迟请求反例

日期：2026-09-27。续作起点：`eea15d5eebae8b8c4662bb416220503055b1757d`，工作树干净。
本次仅验证 Owner 新给出的 FIX-05 安全契约，不是新的开放式代码审查。

结论：**BLOCKED / STOP**。同一 incarnation 解决了跨 manager 存储连续性的歧义，但不能证明更早的超时 create 请求不会在当前拒绝之后产生副作用。
不能据此生产 `RECONCILED_NO_EFFECT`，没有修改生产实现或新增 migration。

## 固定源码依据

全部来自 d2core v0.1.1，commit `988720ad85af1f0d97bfe98ec4da4fcbb070beea`：

- `internal/localipc/ipc.go:147` 的 Serve 为每个连接启动独立 goroutine。在 readLine 成功后调用 handler；没有将客户端断开或请求 deadline 传入 handler，也没有跨连接 FIFO 保证。
- `internal/core/manager.go:143` 的 Respond 在进入时获取 mutex。这保证同一时间只有一个 dispatch，却不保证独立连接按发送时间执行。goroutine 可以在成功读完请求后、进入 Respond 前暂停。
- `internal/records/records.go:323` 的 CreateChecked 在资源拒绝前查 key；`NO_PORT_AVAILABLE` / `PORT_IN_USE` 不持久化该 key 的拒绝终态。
- `internal/localipc/ipc.go:208` 的 Call 超时会结束客户端连接；这不会取消已经读入服务端的 handler 工作。`docs/local-api.md` 也明确管理端会继续处理已受理操作。

## 具体交错

全过程保持同一 manager incarnation E1、同一内存 Store、同一 frozen key/template/port/fingerprint，且远在 history expiry 之前。

1. Controller 发出 create A。服务端 goroutine 已读到完整请求，在进入 Respond 之前暂停。
2. A 的客户端超时，Controller 向 Platform 持久报告 unknown/E1，无 instance/operation IDs。
3. Controller 后续 list 可先执行；它不对 A 构成跨连接执行屏障。
4. Controller 用同一请求重试 B，B 先进入 Respond。key 尚不存在，端口池当前无空闲，返回 `NO_PORT_AVAILABLE@validate`，无 IDs。
5. 按拟议契约，Ecurrent=E1 且命中 post-lookup whitelist，此时会释放 Allocation。
6. 另一个实例正常回收或其他端口占用结束，使端口可用。
7. A 的 goroutine 恢复，在同一 manager 中执行 CreateChecked。key 仍不存在，于是成功持久化 instance、operation、key，并可能继续启动。

第 5 步已经释放、第 7 步却产生副作用。这个反例不要求 manager restart、存储替换、恶意篡改或不同 key。
即便对 endpoint 做调用前后双重核对，E1 始终不变，仍无法排除该交错。
mutex 串行化与跨连接 FIFO 是不同保证；固定 client/API 没有暴露旧请求已经完成或被取消的 fence。

## 执行证据及边界

在工作树忽略目录 `.local/a4-incarnation-proof` 创建独立测试 module，引用只读的固定 d2core module；没有改动其源码。
测试名：`TestSameIncarnationRejectionDoesNotFenceEarlierRequest`。
完整已执行测试源另存为 [incarnation_test.go](a4-fix05-incarnation-counterexample.go.txt)，供复核；`.txt` 后缀避免让主 module 编译依赖上游 internal 包的隔离证明。
复现时使用独立 module `github.com/L4C99/dota2-arcade-dedicated-core/a4proof`，require 固定 core v0.1.1 与平台锁定的 x/sys v0.48.0，replace core 到已核实 Origin.Hash 的只读本地 module 路径，并将测试源保存为 `incarnation_test.go`。
使用真实 `records.Open`、`config.Template`、`Store.CreateChecked` 和磁盘持久化；使用 TCP listener 暂时占用单端口池。
用 channel barrier 确定性模拟第一个 handler 在进入 manager critical section 前被延迟，用 mutex 保持串行 dispatch。

运行：

```text
go -C .local/a4-incarnation-proof test -mod=mod -v -count=1 ./...
=== RUN   TestSameIncarnationRejectionDoesNotFenceEarlierRequest
same store, same frozen key/request: retry returned NO_PORT_AVAILABLE@validate without IDs
after that rejection, delayed original request persisted instance + operation + key; same Store, no restart
--- PASS: TestSameIncarnationRejectionDoesNotFenceEarlierRequest (0.04s)
PASS
```

PASS 表示成功观察到反例，**不是 FIX-05 修复通过**。
随后重新 `records.Open`，确认迟到原请求的 key/instance 已持久化。
测试不运行实际 IPC、Manager worker 或 Dota；并发可达性来自上述固定 Serve/Respond 源码，barrier 只是确定性模拟合法调度暂停。
没有连接 PG、真实节点、生产服务，没有修改防火墙或内容。

初次运行尝试拉取未缓存的 x/sys v0.10.0，因网络受限失败；测试 module 随后使用平台已有锁定依赖 x/sys v0.48.0（与本项目实际构建固定 core 时一致），运行通过。

## 恢复工作的必要条件

需要可证明较早同 key 请求均已执行完毕或已被阻止继续执行的额外条件。
单纯同 incarnation、再次 list、固定等待时间、重复端口拒绝都没有提供这一保证。
本轮不自行修改 d2core、不设计更弱证明、不将 FIX-05 deferred，也不越过 STOP 启动 FIX-06～17。
需 Owner 对这一具体反例给出后续技术契约；既有 FIX-01～04 与五个本地提交不变。
