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
