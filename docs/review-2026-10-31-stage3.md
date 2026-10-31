# 第三阶段全面审查与修复记录

本阶段承接[第一阶段](review-2026-10-31.md)（第 1～7 轮）与[第二阶段](review-2026-10-31-followup.md)（第 8～14 轮），完成第 15～17 轮。
审查基线 `4691132`（分支 `ceshi`），全部修改保留在当前工作区。审查方式：五个方向并行逐文件精读
（账号池/鉴权、上游客户端与传输、存储层、工具调用与提示词兼容层、HTTP surface 与流式引擎），对确认的问题先补回归再修复，
最后执行全量门禁。未使用真实 DeepSeek 账号。

## 覆盖范围与方法

| 方向 | 模块 | 结论 |
| --- | --- | --- |
| 账号池与鉴权 | `internal/account`、`internal/auth` | 修复 token 刷新标记泄漏、admin 密码分布偏差；轮转队列改环形 |
| 上游客户端 | `internal/deepseek/{client,transport,protocol}` | 修复 TTS nil 守卫；网络错误补齐退避；身份头热路径缓存 |
| 存储层 | `internal/chathistory`、`internal/usagestats`、`internal/responsehistory`、`internal/devcapture` | 修复重启中断条目 revision 不变、损坏账本被覆盖、抓包 Clear 不释放内存 |
| 工具调用与提示词 | `internal/toolcall`、`internal/toolstream`、`internal/promptcompat`、`internal/prompt` | 修复 required/forced 附加规则与格式规范的编号冲突 |
| HTTP surface 与流式引擎 | `internal/stream`、`internal/sse`、`internal/completionruntime`、`internal/httpapi/*` | 修复 Claude 初始错误状态码改写、分段路径鉴权误判、Gemini 取消/重生成路径、Claude/Responses/Gemini 取消时的历史终态 |

本轮另核对过并确认无问题的关键面（不重复排查清单）：waiter 唤醒接力链、inUse/inUseTotal 计数配对、锁序单向、
限速/每日限额定时器身份校验、SSE 泵 goroutine 退出路径、CDATA slot 机制、数字保真链路、`TrimContinuationOverlap` 零拷贝。

## 第 15 轮：并行深度审查

五路审查共产出 **22 个确定 bug、30+ 疑似问题、25+ 性能优化点** 的结构化清单，随后逐项人工复核代码确认（排除误报），
按「确定且可安全修复 → 性能热点 → 需产品决策」分级进入后续轮次。

## 第 16 轮：确定 bug 修复（批次 A）

| # | 位置 | 原问题与影响 | 修复 |
| --- | --- | --- | --- |
| 1 | `chathistory/store.go` `markInterruptedStreamingLocked` | 重启时把残留 streaming 条目改写为 error，但不递增 revision；管理端 ETag 轮询（If-None-Match）持续 304，已打开的页面永远停留在转圈状态——`4691132` 想消除的现象被架空 | 每个被改写条目执行 `item.Revision = s.nextRevisionLocked()`；回归 `TestRestartInterruptedStreamingBumpsRevision`（修复前失败） |
| 2 | `httpapi/claude/handler_messages.go`（两条流式入口） | 初始上游非 200 一律改写为 500：429 被报告为 `api_error` 而非 `rate_limit_error`，客户端放弃重试；且与历史记录写入的真实状态码不一致 | 透传 `resp.StatusCode` 并复用 `writeClaudeError` 的 Anthropic 状态映射；回归 `TestClaudeStreamInitialUpstreamErrorPreservesStatus`（429/403/502 三例） |
| 3 | `completionruntime/segments.go` | 分段（expert prompt）路径把 CreateSession 网络失败误判为 `account_unauthorized`：直传 token 客户端收到误导性 401，托管账号触发无谓强制重登 | 改用 `sessionOutputError(a, err)`，与单发路径对齐（网络抖动 → 502 `upstream_unavailable` 可换号） |
| 4 | `auth/auth_failure.go` `ForgetAccount` | 删除账号只清 `authFailures`，`tokenRefreshedAt` 条目永久残留（只增不减） | 同步 `delete(r.tokenRefreshedAt, identifier)` |
| 5 | `usagestats/store.go` 加载失败 | 账本 JSON 损坏（或暂时性读失败）后以空账本启动，第一次 Record 的 flush 原子覆盖原文件——全部历史无备份丢失 | 启动加载失败时先把原文件改名为 `.corrupt-<unix>` 隔离，再开始新账本；回归 `TestCorruptLedgerIsQuarantinedNotOverwritten`（修复前失败） |
| 6 | `devcapture/store.go` `Clear()` | `items[:0]` 只改 len，底层数组继续引用每条最多 5MB 的请求/响应体，GC 无法回收直到被新条目逐个覆盖 | 重新分配 `make([]Entry, 0, limit)` |
| 7 | `auth/admin.go` `GenerateAdminPassword` | 字符表 57 个字符，`256 % 57 = 28`：取模映射使前 28 个字符概率高约 25%，有效熵降低 | 拒绝采样（丢弃 ≥228 的字节），分布均匀 |
| 8 | `deepseek/client/client_tts.go` 两处 | `c.Auth.SwitchAccount` 缺 nil 守卫（其余 6 处同类调用都有）；`NewClient(store, nil)` + captcha 场景会 panic | 补 `c.Auth != nil` 判断 |
| 9 | `httpapi/gemini/handler_stream_runtime.go` 取消路径 | 客户端断开后 `ConsumeSSE` 不触发 OnFinalize，Gemini 缺取消检查 → 把截断内容写成 `history.Success(200)` 并向已死连接写终帧 | 补 `OnContextDone`（历史落 `stopped`）+ `ctx.Err()` 早退返回 Terminal |
| 10 | 同上 + `httpapi/claude` + `openai/responses` 取消路径 | 三处表面客户端取消时历史条目永久停留 `streaming`、用量漏记（chat 表面有完整 `stopped` 处理，其余表面没有） | `responsehistory.Session` 新增 `Stopped(thinking, content, finishReason)`：按已生成内容估算用量入账并落 `stopped` 终态；三个表面在取消路径调用 |
| 11 | `httpapi/gemini` `OnRegenerate` 缺失 | 空输出重试/换号重生成时上一轮 RawText/RawThinking 原样保留，新答案拼接在其后，工具检测在两轮拼接文本上运行 | `assistantturn.Accumulator` 新增 `ResetForRegeneration()`；Gemini 注册 `OnRegenerate` 钩子（对齐 chat/claude） |
| 12 | `promptcompat/tool_prompt.go` | required/forced 附加规则使用 `7)`/`8)` 编号，与格式规范既有规则 7（数字保持纯文本）、8（仅用 schema 参数名）冲突，同一提示词出现矛盾编号 | 改为独立 `R1)`/`R2)` 编号段 |

## 第 17 轮：性能优化（批次 B，均带 A/B 基准）

| # | 位置 | 优化 | 实测（同环境 3 次中位数） |
| --- | --- | --- | --- |
| 1 | `account` 轮转队列 | `bumpQueue` 原为全切片 memmove（5000 账号 ≈80KB/次取号）；改为 `queueHead` 环形缓冲，队首命中 O(1)，中位命中环形搬移。全部 6 处迭代点改走逻辑序（`forEachQueueAccount`/`queueRingIDs`） | 轮转原语 2886 → 8.3 ns/op（**347×**）；完整 Acquire+Release 3257 → 360 ns/op（**9×**），均 0 分配。`TestBumpQueueRingRotationSemantics` 用旧实现做参照逐_pick 对拍顺序一致 |
| 2 | `deepseek/client/client_identity.go` + `config/device_profile.go` | `time.LoadLocation` 每次读盘（每条 completion 链调用 5-8 次）→ 包级 `sync.Map` 缓存；`androidProfiles()` 每次重建 63 元素切片 → 包级 var；完整设备画像走短路（所有赋值都被"为空才填"守卫，完整画像重算纯属浪费） | `BaseHeadersForAuth` 8086 → 1421 ns/op（**5.7×**），9980 → 766 B/op（**13×**），25 → 9 allocs |
| 3 | `deepseek/client/client_continue.go` | 续答流每行 2 次全文 `string()` 拷贝（TrimSpace + HasSuffix）→ 全程 `[]byte`；`observe` 改收 `[]byte` 免除每行一次 JSON 前拷贝 | 长流每行省 2 次全文拷贝（与既有 SSE 行处理优化同主题） |
| 4 | `deepseek/client` 7 个重试点 | 网络层错误（连接重置/超时/握手失败）原为零退避立即重发，第 13 轮的 `transientFailureBackoffs` 只覆盖有状态码的 429/5xx；外层重试再乘一层 | 新增 `retryAfterNetworkError`（同退避表；ctx 取消/错误本身是取消时立即停止不再烧剩余 attempts）；CreateSession/GetPow 顺带保留底层错误详情（原返回泛化文案） |
| 5 | `account/pool_limits.go` + `pool_rate_limit.go` + `pool_acquire.go` | `acquireChecks` 增加 `now` 快照（原每跳过一个候选读一次时钟）；成功取号记账复用同一 checks 快照（原每次多 2 次存储 RLock + 日期重滚） | 5000 账号单线程 Acquire ~360 ns/op；多线程争用下存储锁压力收敛 |

## 验证结果

| 检查 | 结果 |
| --- | --- |
| `go vet ./...` | 通过 |
| 全量 Go 单测 | 57 个有测试包全部通过 |
| Go race | `account`、`auth`、`chathistory`、`usagestats`、`completionruntime`、`httpapi/claude`、`httpapi/gemini`、`httpapi/openai/responses`、`responsehistory`、`deepseek/client`、`assistantturn`、`devcapture` 全部通过 |
| `gofmt -l` | 0 个文件待格式化 |
| Node 门禁 | `check-node-split-syntax.sh` 102 项通过；`chat-stream.test.js` 109/109 |
| `git diff --check` | 通过 |
| 新增回归 | `TestRestartInterruptedStreamingBumpsRevision`、`TestCorruptLedgerIsQuarantinedNotOverwritten`、`TestClaudeStreamInitialUpstreamErrorPreservesStatus`、`TestBumpQueueRingRotationSemantics`、`TestBumpQueueRingSingleAccount`；基准 `BenchmarkBumpQueueLegacyVsRing`、`BenchmarkAcquireRotationLargePool`、`BenchmarkBaseHeadersForAuthCompleteProfile` |

验证环境：Go 1.26.0、Node 24.22.1、Linux。

## 已确认但未在本轮处理的事项（后续候选）

按风险/收益排序，需要产品决策或更大改动面，本轮不盲改：

1. `account/elastic.go` 验证码冷却把在途账号硬禁用（分支顺序与 busy 保护文档矛盾）——改动会影响代理桥重启行为。
2. 同一账号并发 401 无 single-flight 重登（可能并发多次真实登录放大风控暴露）。
3. `toolcall` JSON 修复层 `repairPathLikeControlChars` 会改写合法 JSON 中含真实换行的 path 类参数值（需重新设计触发条件）。
4. `sse` 非流式收集链路无服务端总时限（上游挂死时占用槽位；需配置项设计）。
5. 流式 sieve 捕获期对整个 capture 全量重扫（O(chunks×size)），需增量扫描改造并先量化。
6. 等待者匹配在有等待者时 `Release` 退化为 O(waiters×N)（需队列元素缓存账号快照）。
7. `usagestats` 无保留策略、`chathistory` 孤儿 detail 文件不清理（磁盘增长类）。

## 尚未验证的范围

真实上游账号联调、生产请求分布下的 p50/p95/p99、真实代理链路、macOS/Windows 实机运行未执行。
本轮修复均以「修复前失败的回归测试 + 全量门禁」验证，不构成「项目无任何 bug 或无进一步优化空间」的证明。
