# 2026-11-04 Android 请求头与指纹核对

后续 [登录与 MFA 抓包](android-login-har-2026-11-04.md) 提供了带安装头的反例，
已纠正初轮默认省略策略：当前默认保留已知安装 ID，`=0` 显式省略。下文统计仍只针对初轮四份 HAR。

## 项目主链路

DS2API 用 Go 将 OpenAI Chat/Responses、Claude、Gemini 请求归一为标准轮次，
经共享 prompt/file/completion runtime 调用 DeepSeek，再由各协议适配器输出结果。
React WebUI 管理账号、设备、代理和运行设置；Vercel 的 Node 桥接处理流式输出。

本次核对的代码入口：

| 层 | 入口 | 职责 |
| --- | --- | --- |
| 默认客户端资料 | `internal/deepseek/protocol/constants_shared.json` | Go/Node 同源的版本、平台、UA、语言、时区默认值 |
| 账号身份 | `internal/deepseek/client/client_identity.go` | 机型/API level、安装 ID、Rangers ID、有效 HIF 材料 |
| 请求正文 | `internal/deepseek/protocol/app_json.go`、`internal/js/shared/deepseek-wire.js` | JSON 字段顺序、首轮/追问字段值 |
| Go 传输 | `internal/deepseek/transport/ordered_headers.go`、`h2_framed.go` | 有序 HTTP/2、内容头、multipart 长度处理 |
| 连接隔离 | `internal/deepseek/transport/h2_pool.go` | 按 origin、凭证、安装身份隔离可复用连接 |
| Node 传输 | `internal/js/shared/app-transport.js` | Vercel HTTP/2 头部、发送和 gzip 流生命周期 |
| 文件 | `internal/deepseek/client/client_upload.go` | PoW、multipart、上传/轮询；各接口共享 |
| SDK | `internal/devid`、`scripts/devid-*` | 登录 SDK ID 生成与账号设备档案锚定 |

## 样本范围与时间

输入为本次上传的 4 份 HAR 和 `deepseek(2).apk`。
HAR 文件名的导出时间不等于采集时间；以下时间由 `startedDateTime` 转为北京时间（UTC+8）。

| HAR | 记录数 | 实际采集时间（北京时间） | 业务 API / 下载 / CONNECT |
| --- | ---: | --- | --- |
| `chat.deepseek.com_2026_11_04_14_41_52.har` | 59 | 10-30 00:22:59–00:24:20 | 54 / 0 / 5 |
| `chat.deepseek.com_2026_11_04_14_42_11.har` | 49 | 10-29 23:56:22–23:58:51 | 49 / 0 / 0 |
| `上传图片md pdf docx(1).har` | 52 | 11-03 09:14:09–09:16:40 | 49 / 0 / 3 |
| `chat.deepseek.com_2026_11_04_14_47_02.har` | 68 | 11-04 14:44:15–14:46:57 | 64 / 3 / 1 |
| 合计 | 228 | — | 216 / 3 / 9 |

上传 APK SHA-256 为
`cd461f3d7c78f0c4ab6b8c4ff7270e16a766042170dfebead540b7f174948c5c`，
与 [已有 APK 静态分析](apk-request-alignment.md) 完全相同；DEX 中也有 `2.7.1`
和既有客户端头字段。本次没有发现新的 APK 版本，沿用 Android 2.7.1/versionCode 287 基准。

仓库仅保存脱敏协议夹具 `tests/fixtures/android_app_2_7_1_2026_11_04.json`：
端点与头部名称/顺序、允许公开的静态字段、计数、JSON 键序及上传格式约束。
认证凭据、Rangers/安装 ID、文件名/正文、提示词、会话 ID、签名 URL 均不写入夹具。
本次核对采用离线解析和本地 HTTP/2 服务，没有使用 HAR 凭据向上游重放。

## 请求头逐项对照

| 字段 | 216 次业务请求观察 | 项目对齐结论 |
| --- | --- | --- |
| HTTP 版本 | 全部 `HTTP/2.0` | Go 有序 HTTP/2 / Node 原生 HTTP/2 |
| 伪头顺序 | `:method, :authority, :path, :scheme` | 一致 |
| `x-client-platform` | `android` | 一致 |
| `x-client-version` | `2.7.1` | 一致 |
| `x-client-locale` | `zh_CN` | 默认一致；支持账号覆盖 |
| `x-client-bundle-id` | `com.deepseek.chat` | 一致 |
| `x-client-timezone-offset` | `28800`，单位秒 | 默认一致；账号时区按当前偏移计算 |
| `x-device-model` | `V2507A` | 使用每个账号的设备档案；样本机型不设为全局默认 |
| `x-rangers-id` | 216 次出现 | 使用账号真实配置；没有注册数据时仍缺省 |
| `x-device-id` | 0 次出现 | 本批用显式 `=0` 对齐；后续登录样本携带该头，默认保留已知 ID |
| `x-hif-dliq/leim` | 均未出现 | 不补造；配置真实且未过期材料时才转发 |
| `user-agent` | `DeepSeek/2.7.1 Android/36` | 默认一致；Android API level 随账号档案 |
| `authorization` | 215 次；仅国家查询未携带 | 鉴权由端点/账号决定，token 不固定 |
| `accept` / `accept-charset` | `application/json` / `UTF-8` | 一致 |
| `accept-encoding` | `gzip` | 一致，保留 gzip 解码 |
| 浏览器头 | 无 Origin/Referer/Client Hints/Fetch Metadata | 业务默认不注入；保留已有浏览器默认头过滤 |

普通业务头顺序为：

```text
x-client-platform, x-client-version, x-client-locale, x-client-bundle-id,
x-client-timezone-offset, x-device-model, x-rangers-id,
user-agent, authorization, accept, accept-charset,
[content-type, content-length], accept-encoding
```

| 端点（省略 `/api/v0`） | 次数 | 前置头 / 内容头与正文 |
| --- | ---: | --- |
| GET `chat/history_messages` | 26 | 无 Content-Type/Length；查询键含 scenario，部分附 cache_version/cache_reset_at |
| POST `chat/create_pow_challenge` | 60 | JSON、精确字节长度；正文只含 target_path |
| POST `chat/completion` | 26 | 前置 x-ds-pow-response；JSON、精确字节长度 |
| POST `chat_session/create` | 18 | Content-Type 为 JSON，空 body，Content-Length=0 |
| GET `file/fetch_files` | 43 | 无内容头；38 次单 file_ids、5 次重复 file_ids 键批量查询 |
| POST `file/upload_file` | 32 | 前置 PoW/thinking/file-size/model-type；multipart，无总 Content-Length |
| POST `chat_session/delete` | 7 | JSON 键序 chat_session_id → chat_session_ids |
| GET `chat_session/fetch_page` | 1 | 无内容头 |
| GET `users/current` | 1 | 无内容头 |
| GET `chat/tts/voices` | 1 | 无内容头 |
| GET `ip_to_country_code` | 1 | 无 authorization，无内容头 |

所有 11 种业务头形态均加入 Go 和 Node 的本地实际 HTTP/2 收包回归。
对项目暂未主动调用的端点，验证的是通用传输构造能力，不代表新增了该业务流程。
含安装头和省略安装头的 HAR 分别通过保留/省略模式回归；check_device 的 authorization 前置顺序保留。

## 本次实现修正

`DS2API_DEEPSEEK_SEND_INSTALLATION_HEADER` 在默认有序 Go HTTP/2 和 Node 中的语义调整为：

| 配置 | 发送 `x-device-id` |
| --- | --- |
| 未设置、空值 | 是（已知 ID 存在时；后续登录样本纠正了初轮省略策略） |
| `0/false/no/off` | 否 |
| `1/true/yes/on` | 是，使用本地安装标识 |
| 其他值 | 是（沿用默认保留规则） |

值忽略两端空白和大小写。显式旧配置 0/1 含义保持一致。
本地 `installation_id` 仍用于 check_device JSON、SDK/上传设置与缓存隔离；
登录正文中的 SDK `device_id` 仍是独立来源。
连接池额外纳入原始请求中的安装标识，防止省略线上的头后，不同安装共用同一连接池键。
显式旧传输 `DS2API_DEEPSEEK_USE_FRAMED_H2=0` 不提供本轮线序保证。

实际 Node 发送测试发现 GET/HEAD 默认会在 `session.request()` 时结束请求流，旧代码
随后 `stream.end('')` 会触发 `ERR_STREAM_WRITE_AFTER_END`。
现按有无请求体显式控制 endStream；有正文才调用 end，空会话创建和 GET 都能正确完成。

## JSON、PoW 与上传指纹

26 次 completion 的键序完全一致：

```text
chat_session_id, parent_message_id, prompt, ref_file_ids,
thinking_enabled, search_enabled, audio_id, preempt, model_type, action
```

首轮 parent_message_id 为 null、model_type 为 default；追问 parent 非空、model_type 为 null。
audio_id/action 均为 null，preempt 为 false；思考为 true；最新样本同时覆盖搜索开/关。
文件引用数覆盖 0–3，追问既可不带文件，也可追加文件。
Go/Node 用合成会话、文件 ID 和文本回归这些观察组合，不保存真实请求正文。

58 个 PoW 请求头的 Base64 JSON 键序均为
`algorithm, challenge, salt, signature, answer, target_path`，target_path 与实际端点一致。
60 次挑战请求中，32 次用于上传、28 次用于 completion；数量差异不能视为协议错误，
App/项目都有预取行为。每次证明仍使用当前挑战，不复用抓包材料。

32 次上传包括 16 WebP、3 GIF、4 PDF、3 DOCX/ZIP 和 6 文本/其他文件。
逐条解码 HAR 的 base64 或文本 multipart 后验证：

- 32/32 文件部分字节数等于 x-file-size，包含一次 status=0 的未完成请求。
- 单个 file 分段，分段 MIME 为 application/octet-stream；没有分段 Content-Length。
- boundary 均为 70 字符十六进制/连字符，尾部为 closing boundary + CRLF。
- 无 multipart 总 Content-Length；x-thinking-enabled=1、x-model-type=default。
- 31 次上传 HTTP 200，1 次 status=0、timings 未完成；不能当成服务端业务拒绝。

项目已有实现满足这些约束；思考头仍随实际请求取 0/1，搜索开关仍保留调用方语义。
图片压缩由账号/远程设置驱动，不能从 HAR 推出 Android 编码器质量或逐字节等价。

## 下载域名与仍需补充的证据

最新 HAR 另有 3 次 `files.deepseeksvc.com/api/file` GET，查询键为 file_id/state/ty。
其线序是伪头后先 referer，再平台/版本/语言/bundle-id、UA、accept-charset、accept、gzip；
`referer=https://chat.deepseek.com`、`accept=*/*`，没有 authorization、时区、机型或 Rangers ID。
这是独立的签名资源下载形态，已单列脱敏观察。项目当前 `/v1/files` 内容读取依赖本地内容存储，
没有新增上游签名下载流程，也没有把业务凭据发送到该域名。

| 指纹层 | 本次可确认 | 仍未确认 |
| --- | --- | --- |
| HTTP 语义 | 头名称、顺序、静态值、JSON 键序、multipart 与长度 | 本批缺少启动/登录/上报；后续补充见登录专题 |
| TLS | Go 使用 HelloAndroid_11_OkHttp 兼容预设；Node 使用运行时 TLS | Android 36 实机 ClientHello、扩展顺序、JA3/JA4 一致性 |
| HTTP/2 帧 | 本地实线校验本项目发送头序及正文 | App 的 SETTINGS、窗口、PRIORITY、HPACK 索引策略与帧分片 |
| 身份与认证 | SDK ID/安装 ID/Rangers/HIF 的来源区分 | Web SDK 与原生 SDK 的云端可信度等价、HIF 签发和遥测注册 |

HAR 是 Reqable 导出的 HTTP 层记录，包含 CONNECT 也不代表包含原始 TLS 握手或 HTTP/2 帧。
因此本轮完成的是可观察请求层对齐，不能宣称 Android 全栈指纹已经完全复现。

## 本地验证结果

- `go test ./internal/deepseek/...`：client、protocol、transport 三个包通过。
- `node --test tests/node/gif-har-headers.test.js tests/node/app-transport.test.js tests/node/deepseek-wire.test.js`：16 项通过。
- Go/Node 均验证 11 类最新业务请求的实际 HTTP/2 头序、静态值和正文发送；Node 另覆盖空 HEAD。
- Go 连接池测试验证省略安装头后，同 token 不同安装身份仍使用独立连接。
- `./tests/scripts/check-refactor-line-gate.sh`：通过；变更 Go 文件已 gofmt，`git diff --check` 通过。

推送前补充：完整 Go/Node 单测、WebUI 构建和增量 Lint 均通过。全仓格式门禁被 12 个既有、
本次未修改的 Go 文件阻断；本次变更 Go 文件均已格式化。未做真实上游账号联调。
