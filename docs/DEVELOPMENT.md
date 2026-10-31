# DS2API 开发者速查

语言 / Language: 中文

本文面向维护者和贡献者，用于快速判断“从哪里看、改哪里、跑什么”。架构细节仍以 [ARCHITECTURE.md](./ARCHITECTURE.md) 为准，接口行为以 [API.md](../API.md) 为准。

## 1. 本地入口

常用启动与检查：

```bash
# 后端
go run ./cmd/ds2api

# WebUI 开发服务器
npm run dev --prefix webui

# WebUI 生产构建
npm run build --prefix webui
```

PR 前固定门禁：

```bash
./scripts/lint.sh
./tests/scripts/check-refactor-line-gate.sh
./tests/scripts/run-unit-all.sh
npm run build --prefix webui
```

修改 Go 文件后先运行：

```bash
gofmt -w <changed-go-files>
```

## 2. 代码定位

优先从这些入口顺着调用链看：

| 目标 | 入口 |
| --- | --- |
| 总路由、CORS、健康检查 | `internal/server/router.go` |
| OpenAI Chat / Responses | `internal/httpapi/openai/chat`、`internal/httpapi/openai/responses` |
| Claude / Gemini 兼容入口 | `internal/httpapi/claude`、`internal/httpapi/gemini` |
| API 请求归一到网页纯文本上下文 | `internal/promptcompat`、`docs/prompt-compatibility.md` |
| 工具调用解析与流式防泄漏 | `internal/toolcall`、`internal/toolstream`、`docs/toolcall-semantics.md` |
| DeepSeek 上游调用、登录、PoW、代理 | `internal/deepseek/client`、`internal/deepseek/transport` |
| 账号池、并发槽位、等待队列 | `internal/account` |
| Admin API | `internal/httpapi/admin` |
| WebUI 页面 | `webui/src/layout/DashboardShell.jsx`、`webui/src/features/*` |
| 服务器端对话记录 | `internal/chathistory`、`internal/httpapi/admin/history` |

## 3. 常见改动建议

- 改接口行为时，同时检查 `API.md` / `API.en.md` 是否需要同步。
- 改 prompt 兼容链路时，必须同步 `docs/prompt-compatibility.md`。
- 改 tool call 语义时，同时检查 Go、Node sieve 和 `docs/toolcall-semantics.md`。
- 改 WebUI 配置项时，同时检查 `webui/src/features/settings`、语言包和 `config.example.json`。
- 拆分大文件时，保持对外函数签名稳定，并跑 `./tests/scripts/check-refactor-line-gate.sh`。

## 4. 故障定位

接口请求先看路由入口，再看协议适配层，最后看共享 runtime：

1. 路由是否命中：`internal/server/router.go` 和对应 `RegisterRoutes`。
2. 鉴权与账号选择：`internal/auth`、`internal/account`。
3. 请求归一化：`internal/promptcompat` 或协议转换包。
4. 上游请求：`internal/deepseek/client`。
5. 流式输出：`internal/stream`、`internal/sse`、`internal/toolstream`。
6. 响应格式：主路径看 `internal/assistantturn` 与 `internal/format/*`；`internal/translatorcliproxy` 只用于 Vercel/fallback/test 桥接。

对话记录页面问题优先检查：

- Admin API：`/admin/chat-history`、`/admin/chat-history/{id}`。
- 后端存储：`internal/chathistory/store.go`。
- 输出归档：`internal/responsehistory` 在协议回译/裁剪前记录 DeepSeek 上游 assistant text / thinking；即使工具调用已被对外响应转成结构化 `tool_calls` 并从可见正文剔除，后台历史仍应保留原始 DSML / XML 片段，方便排查格式漂移。
- 前端轮询和 ETag：`webui/src/features/chatHistory/ChatHistoryContainer.jsx`。

Tool call 问题优先跑：

```bash
go test -v ./internal/toolcall ./internal/toolstream -count=1
./tests/scripts/run-unit-node.sh
```

### 运行时和管理台边界

- 每分钟限速热更新会按当前窗口计数重新判断状态；下调阈值立即生效，旧窗口/旧账号条目的定时器不能清空新窗口。
- 全局并发已满时，取号和等待者匹配立即停止，不再遍历所有候选账号。
- Responses 缓存保留不可变 JSON 快照；输入编码失败保留原记录，TTL 更新仅影响后续写入。
- 管理台登录恢复、列表和配置读取需防止迟到响应提交状态；共享请求世代控制位于 `webui/src/utils/latestRequest.js`。
- 会话 token 与过期时间必须来自同一存储位置，当前标签页会话优先；退出或新登录会取消旧验证，旧 token 的 401 不能清除新会话。
- 代理订阅超过 16 MiB 时返回错误，不能把截断的有效 YAML 前缀作为完整订阅使用。

补充微基准：

```bash
go test ./internal/account -run '^$' -bench BenchmarkAcquireSaturatedGlobalLimit -benchmem -count=3
go test ./internal/httpapi/openai/responses -run '^$' -bench BenchmarkResponseStore -benchmem -count=3
go test ./internal/sse -run '^$' -bench BenchmarkParsedLinePump -benchmem -count=3
```

## 5. 测试选择

小范围 Go 改动：

```bash
go test ./internal/<package> -count=1
```

前端改动：

```bash
npm run build --prefix webui
```

高风险协议或流式改动：

```bash
./tests/scripts/run-unit-all.sh
```

发布或真实账号链路验证：

```bash
./tests/scripts/run-live.sh
```

端到端测试产物默认写入 `artifacts/testsuite/`。分享日志前需要清理 token、密码、cookie 和原始请求响应内容。

## 6. 维护代码分类

共享业务层不得引用 `internal/httpapi`。当前输入文件服务在 `internal/contextfile`；输出清洗在 `internal/textclean`；流式累积和工具检测在 `internal/assistantturn`；重试提示词及 payload 在 `internal/completionruntime`。增加协议时直接依赖这些模块，不把业务放回 OpenAI 目录。

账号管理保留 `useAccountActions` 作为页面装配入口，内部职责为：

| Hook | 职责 |
| --- | --- |
| `useAccountEditor` | 账号新增、编辑、删除及表单状态 |
| `useCredentialActions` | API Key 操作，组合手动 token Hook |
| `useManualTokenActions` / `manualTokenResource` | token 弹窗状态、凭据读取、保存与会话取消 |
| `useAccountTesting` | 单账号 / 批量测试和会话操作 |
| `useAccountRuntime` | 代理绑定、启停、封禁清理和弹性池 |

密钥和账号表单的忙碌状态分别维护，由装配入口汇总为 `loading`，避免一个表单完成时提前清除另一个在途操作的状态。

行数门禁在旧计划清单缺失时发现当前全部 Go / JS / JSX / TS 源码（包含未提交新文件），测试文件排除。默认后端上限 300 行、前端 500 行、轻量入口 120 行。`tests/refactor-line-baseline.tsv` 冻结尚未拆分的大文件以及本轮迁移实现的实际大小；这些条目不允许继续增长，拆分时下调或移除条目。不要通过增加新例外绕开职责拆分。显式传入的目标清单缺失、目标文件缺失和零检查数均会失败。

完整审查、改动和验证结果见 [代码分类与可维护性审查](maintainability-review-2026-10-31.md)。

## 7. 配置注册与历史存储定位

配置沿用同一套注册表与锁，文件按职责组织：

| 文件 | 职责 |
| --- | --- |
| `config/settings.go`、`sections.go` | 声明类型和元数据契约 |
| `config/settings_defaults.go` | 默认设置和页面卡片的注册顺序 |
| `config/settings_registry.go`、`settings_cards.go`、`sections_registry.go` | 注册、查询和声明校验 |
| `config/settings_values.go`、`sections_values.go` | 读取有效值、应用和合并更新 |
| `config/settings_validation.go` | 标量转换、配置和间隔校验 |
| `config/sections_codec.go`、`sections_fields.go` | section 编解码生命周期、字段构造器 |

卡片注册先验证完整声明再提交成员归属；拒绝未知、隐藏或重复成员后，注册表保持原样。

历史存储中，`models.go` 定义持久化结构，`store.go` 负责初始化和查询，`mutations.go` 负责写入操作，`load.go` / `legacy.go` 负责加载恢复及旧格式迁移，`persistence.go` 负责原子写盘，`index.go` / `detail_changes.go` 负责索引与待写详情，`clone.go` 负责副本隔离。所有 `Locked` 方法均要求持有 `Store.mu`，数据格式、revision 递增与写盘顺序沿用原实现。

手动 token 弹窗使用独立会话身份：切换、关闭、测试结果直接提供凭据和组件卸载都会作废旧请求；旧的读取或保存结果不能更新当前弹窗。即使重开同一账号，也属于新的会话。提交期间忽略重复提交；已被服务端接受的旧保存请求无法由关闭弹窗撤回，会话保护只约束当前 UI 状态。

## 8. 音频与 SSE 定位

音频入口 `audio/handler.go` 仅串联 HTTP 流程；`request.go` / `plan.go` 负责参数归一和完整校验，`segments.go` 负责文本切分，`workers.go` / `account_gate.go` 负责取号与并发上限，`synthesis.go` 负责分段生成，`recite.go` 负责共享 completion 调用，`response.go` 编码音频，`cleanup.go` 清理临时会话。段数校验必须在取号之前；后台清理只能使用每段保存的路由身份，不能保留 worker 的可变鉴权对象。子清理超时须继承整个任务的 deadline。

Go SSE 按 `parser.go`、`paths.go`、`fragments.go`、`extract_recursive.go`、`thinking.go`、`content_filter.go` 分类；Node 对应 `sse_parse_impl.js`、`sse_paths.js`、`sse_extract.js`、`sse_thinking.js`、`sse_content_filter.js` 和 `sse_text.js`。协调入口统一创建结果和执行收尾；叶子模块不反向引用入口。片段级 status 与响应级 status 不可混用，修改时优先在 `tests/compat/fixtures/sse_chunks` 与 `expected` 添加 Go / Node 共用的回归夹具。

## 9. Node 流式生命周期与批量测试

Node `stream_lifecycle.js` 在 prepare 前监听断开；拿到租约 ID 后立即登记释放器，prepare 字段不完整也要释放。断开时取消上游 reader；如果 Go 的控制请求已在进行，则等它结束、释放租约，不再启动新的 completion 或换号。释放器沿用幂等和独立清理超时。`stream_retry_policy.js`、`stream_continue.js`、`stream_failure.js`、`upstream_inspect.js` 分别承载重试规则、续答状态、错误输出和 JSON 探测；普通协调流程只调用它们。

`http_internal.js` 共用一条 prepare / PoW / switch 请求编解码路径。401 必须显式传 `auth_failed: true`，让 Go 按共享规则先恢复凭据，避免退化为普通换号。JSON 探测器拥有源 reader，完整 JSON、读取失败和首块超限时均须取消 / 解锁；非 JSON 则把 reader 的所有权交给重放流。

账号批量测试由 `batchAccountTests.js` 管理一个批次及其 AbortSignal；`useAccountTesting` 负责 React 接线和文案，`AccountTestControls` 负责操作按钮与进度。批次开始时固定账号标识；重复启动复用在途批次，取消 / 卸载后不提交迟到结果、不启动下一个账号。主动取消保留已完成结果；卸载清理保持静默，不能写 React 状态。已经由服务端完成的 Token 刷新不会因取消撤回。
