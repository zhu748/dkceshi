# 运行时可靠性与开销优化

基于 `ada8a46` 的审查结果，修复响应探测超时、Node 传输恢复和失败会话清理，并减少输出处理与历史记录的重复工作。

## 修改及行为

| 范围 | 当前行为 |
| --- | --- |
| 首段探测 | Go 和 Node 的首字节 / JSON 响应探测继承调用方取消，最长不超过连接超时；Node 同时受空闲和请求总预算约束 |
| 错误响应 | Node 错误体最多读取 64 KiB，读取中断或超时进入共享错误处理；探测重放的 SSE 字节不重复计入输出限制 |
| 故障恢复 | Node 将连接重置、TLS / 传输异常和 HTTP 临时故障放在同一入口恢复；替代请求使用 Go 重建的 payload 和凭据，已交付正文 / 工具调用后不再重放 |
| 预算耗尽 | 响应头等待也受请求总截止时间控制；预算耗尽直接报错并释放租约，不给账号增加限流冷却 |
| 会话清理 | 所有直接协议出口在请求释放时执行 single/all 策略；会话创建后 PoW 失败、换号失败、晚到的资源注册同样清理，保存原所属账号、路由及最新 Token |
| 重复删除 | 换号回收与最终收尾原子取得回调；共享运行时已回收的会话不会再进入 Chat 的旧清理路径 |
| 非流式输出 | 每次尝试收集完传输结果、合并思考后，只构建与校验一次 turn；不完整输出保留诊断证据，禁止工具修复与执行 |
| 历史进度 | 沿用五秒合并窗口，思考及正文不变时跳过写盘；失败快照可用相同内容重试，终态仍立即保存 |
| 图表加载 | 移除模型行对图表引擎的同步引用，图表进入视区前 200 px 时才加载 / 初始化，保留占位高度；卸载后忽略晚到的观察回调 |
| 工程维护 | 修复 12 个既有 Go 格式问题；同步中英文 README 的思考开关说明，并收紧职责拆分后文件的行数上限 |

详细协议约束见 [prompt-compatibility.md](prompt-compatibility.md)。自动删除策略在会话创建时读取一次；替换会话仅删除被替代的单个会话，最终收尾才应用 all 策略。

## 回归覆盖

- Node 覆盖连接失败后采用替代账号凭据、无首字节、半截 JSON、半截错误体、响应头超时、预算耗尽、旧桥恢复上限、已交付后禁止重放及字节只计一次。
- Go 覆盖首字节 / JSON 探测超时、换号后新会话 PoW 失败、最终 single/all 清理、退休与最终回调竞争、释放后晚到的资源注册。
- 四个直接协议出口的流式 / 非流式与成功 / PoW 失败组合，共 16 种情形验证会话只清理一次、账号槽位释放完整。
- 历史覆盖相同内容不增加 revision、终态不能被合并、写盘失败后相同快照可重试并在重启后读取。
- 图表测试使用真实 React 组件，验证视区外占位、进入后初始化、重复观察不重新初始化，以及卸载后回调失效。
- 冷却时间测试以整个请求的观察区间验证两分钟窗口，包含 Unix 秒取整，避免 -race 下因替代账号较晚失败而误判。

## 本地性能测量

`BenchmarkNonStreamFinalize` 使用相同的约 54 KiB 合成正文 / 工具参数，调用实际非流式执行入口和模拟上游。旧入口由 Git 基线通过 Go overlay 加载，其他实现及依赖一致。每组预热后固定执行三次，重复三轮，使用单个 Go CPU；下表列每次请求的分配量。

| 样本 | 修改前 B/op | 修改后 B/op | 分配量下降 |
| --- | ---: | ---: | ---: |
| 普通正文 | 14,639,578 | 7,527,717 | 48.6% |
| 工具调用 | 14,583,002 | 7,499,429 | 48.6% |

两类样本的分配次数也约减半。该测量只覆盖本地执行、解析、校验和计数，不能代表真实上游延迟或整机峰值内存。图表优化减少离屏实例和同步依赖，未声称缩小 ECharts 引擎本身的体积。

验证入口：

```sh
./scripts/lint.sh
./tests/scripts/check-refactor-line-gate.sh
./tests/scripts/run-unit-all.sh
npm run build --prefix webui
go test -race ./internal/auth ./internal/completionruntime ./internal/deepseek/client ./internal/responsehistory
go test -race ./internal/httpapi/openai -run 'TestUnavailableEmptyOutputCoolsAccountAndSwitchesWithoutSameAccountRetry|TestEveryProtocolFinalizesOwnedSessionsOnSuccessAndStartupFailure'
go test ./internal/completionruntime -run '^$' -bench '^BenchmarkNonStreamFinalize$' -benchtime=3x -count=3 -cpu=1
```

本轮验证使用模拟上游，未调用真实 DeepSeek 账号。依赖真实 SDK / VM 的两项既有 Node 测试仍跳过。
