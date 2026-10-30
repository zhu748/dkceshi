# API → 纯文本上下文兼容说明

本项目的提示词、工具调用、历史拆分和模型目录对齐
`ouqiting20/ds2api` 的 `f9d2085`。请求头、登录及上游传输继续使用本项目原有实现。

## 统一请求链路

OpenAI Chat、Responses、Claude 和 Gemini 先转换为标准消息，再通过
`internal/promptcompat` 统一生成提示词。协议适配器负责输入和输出格式，工具规则、
思考注入和历史拆分使用共享逻辑。Vercel Node 使用 Go prepare 返回的最终提示词，
并同步使用相同的工具标记及解析规则。

最终上下文按 `[System]:`、`[User]:`、`[Assistant]:`、`[Tool]:` 拼接，
不再插入旧分支的特殊起止 token 或 Output integrity guard。
历史 reasoning 以 `[reasoning_content]` 块保留，工具调用历史重新渲染为规范工具块。

相关实现：

- `internal/prompt/messages.go`：角色标记和最终文本。
- `internal/promptcompat/prompt_build.go`：统一提示词组装。
- `internal/promptcompat/thinking_injection.go`：可选思考注入及重建。
- `internal/httpapi/claude/standard_request.go`、`internal/httpapi/gemini/convert_request.go`：协议归一化。

## 工具提示词和调用格式

工具说明来自本次请求：`Tool`、`Description`、`Parameters`（JSON Schema）。
说明后附统一格式规范、调用决策、参数类型规则和正反例。正确示例只选取当前请求中
实际声明且有已知参数形态的工具。

```xml
<|EPSE|tool_calls>
  <|EPSE|invoke name="read_file">
    <|EPSE|parameter name="path"><![CDATA[src/main.go]]></|EPSE|parameter>
  </|EPSE|invoke>
</|EPSE|tool_calls>
```

字符串必须使用 CDATA；对象支持嵌套元素，数组支持重复 `<item>`，数字、布尔值和
null 保持字面量。模型不得输出空参数、占位符或 Markdown 围栏。
提示词明确区分已执行的历史调用与当前待完成的任务，避免重复执行成功操作，
同时允许为了新信息、不同参数或失败重试再次调用同一工具。

完整规范并入首条 system 消息；底部格式提醒以独立 system 消息放在接近生成点的位置。
`bottom_format_injection.enabled` 默认 true，`prompt` 留空使用内置提醒，可关闭或自定义。
只有有工具的请求才注入这些规则；OpenAI/Gemini 还兼容客户端在 system 文本中声明工具的形态。
Claude 的工具声明只采用顶层 tools，避免普通 system 文本误触发工具协议。

`EPSE` 是内部规范关键字。出站时统一替换成按调用者稳定派生的六位标识，
返回文本在解析前恢复为 EPSE；流式归一化支持标识跨分片。历史文件、TOOLS.txt、
思考注入后的重建及修复提示词也沿用同一标识。
服务端密钥自动生成并保存，也可通过 `DS2API_TOOL_MARKER_SECRET` 固定。

API Key 配置中的 `force_disable_tools` 会在工具检测前清除工具定义并阻止格式注入。
账号管理页的新建/编辑 Key 弹窗读取 `credential_flag_specs`，显示同一开关；Key 列表显示生效徽标，
新增和编辑均将开关值提交给后端。
工具选择策略由标准请求持有：Responses 支持 required/指定工具；Chat 沿用基准项目的
默认 auto 策略，忽略客户端 tool_choice。工具执行器仍应自行验证工具名和参数。

相关实现：`internal/toolcall/tool_prompt.go`、`internal/promptcompat/tool_prompt.go`、
`internal/toolcall/marker.go`。

## 工具输出解析与修复

Go 和 Node 均使用结构化扫描，容忍受支持的分隔符漂移、大小写、引号和部分缺失标签。
旧 canonical XML 保留兼容解析，但提示词和历史只示范带标记的格式。
Markdown 代码围栏/行内代码里的工具示例不会执行；完整却无法解释的坏块作为普通文本保留。
CDATA 中的标签示例及原始参数内容应逐字保真，不能被确定性修复吞掉或重排。

流式识别出工具块后输出协议原生工具增量，避免原始标签泄漏到正文；收尾阶段还会检测
仅思维链中存在的工具调用，并按客户端 thinking 开关决定是否显示思维链。
当存在无法确定性恢复的残余工具意图时，收尾可发起一次 LLM 格式修复：
复用同一账号、新建会话、关闭思考、十秒超时，并在结束后清理修复会话。
参数通过 DS_SLOT 占位保护；修复失败或占位被破坏时保留原始残片，不输出半修复内容。
已流式发出的结构化调用不会回滚并重修。

正文截断规则及流式解析在 Go 和 Node 保持一致，包括闭标签复读和虚构 `[Tool]:` 角色。
详见 [toolcall-semantics.md](toolcall-semantics.md)。

## 历史拆分

```json
{
  "input_file": {"enabled": false, "min_chars": 0},
  "thinking_injection": {"enabled": false, "prompt": ""},
  "bottom_format_injection": {"enabled": true, "prompt": ""}
}
```

配置文件按声明的 JSON 类型读取开关、文本和整数；例如字符串 `"false"` 不等于布尔 `false`。
配置中的 null 表示未设置，整数字段的已存零值沿用声明默认值；HTTP 设置更新继续按接口规则校验。

`input_file` 默认关闭。启用后，以完整 FinalPrompt 的 Unicode 字符数检查阈值，
而非只统计最新一条输入。

- 已有对话且末条有效消息为 user 时：前面的消息上传到固定文件名 `HISTORY.txt`，
  最新 user 留在 live prompt 中。其余情况序列化完整上下文，长首轮输入仍能拆分。
- 文件以 `# HISTORY.txt` 和 `=== N. ROLE ===` 分段，包含继续当前任务的说明。
- 工具名称、描述和参数 schema 单独上传为 `TOOLS.txt`；live prompt 保留格式及策略，
  并明确工具 schema 的权威来源是 TOOLS.txt。
- live prompt 使用 system 角色的“继续会话”，不会伪造 user 消息。
- 每次请求按当前账号上传文件；账号切换后重新上传，避免跨账号复用 file_id。
- 上传文件和 live prompt 使用同一调用者标识，usage 按完整上下文计算。

旧 `history_split` 与 `current_input_file` 配置键均忽略且不再写回。
旧配置需要手动将 `current_input_file` 改名为 `input_file`。
模型后缀 `-forcehistory` 不再触发拆分。

`expert_prompt_segment` 保留配置兼容，但 enabled 强制为 false，与基准项目的停用状态一致；
旧配置不能重新打开发送后停止的分段路径。所有规范模型均映射为 default 模式。

相关实现：`internal/promptcompat/history_transcript.go`、
`internal/httpapi/openai/history/current_input_file.go`、`internal/completionruntime`。

## 图片、文件与发送边界

三份 Android App 2.7.1 HAR 对齐的请求头、设备配置、HTTP/2 线序和上传细节见
[android-har-alignment.md](android-har-alignment.md)。
OpenAI Chat/Responses、Claude base64 image/document 与 Gemini inlineData 共用文件上传边界，
文件 ID 进入标准请求的 RefFileIDs；base64 不进入纯文本 prompt。
内联文件及显式文件引用的请求固定当前账号，防止重试中混用跨账号文件 ID。
通过 `/v1/files` 单独上传后，多账号环境需用返回的 account_id 固定后续请求的目标账号。
PNG/JPEG 按显式或远程压缩配置处理；没有有效比例时保留原文件，文档内容保持原样。
同一请求的内联文件先上传，再合并轮询，所有文件成功后才用于聊天。
OpenAI Chat/Responses 剥离已上传的图片块时保留空文本用户轮次；纯图片首轮不会
因 messages 变空而返回 400，纯图片追问也不会退回上一条 assistant 的轮次。
Responses 支持图片 content part 数组、message 数组和单对象 input；引用先汇总到 RefFileIDs。

上游 Client 的共享上传层对内联文件、`/v1/files` 和 HISTORY/TOOLS 使用同一套进程内复用缓存。
键包含调用者、上游账号/凭证、安装身份、原文件 SHA-256、文件名/MIME/purpose、思考与压缩配置。
同一作用域的重复请求可复用 file_id；不同账号不混用，原有显式账号绑定不变。
缓存不改变号池路由；请求被分到另一个账号时，需要在该账号独立上传。
哈希发生在图片转码及随机命名之前，命中时免去重新压缩、上传 PoW 和 multipart 传输。
默认缓存 900 秒、最多 256 项；`DS2API_UPLOAD_CACHE_TTL_SECONDS` 和
`DS2API_UPLOAD_CACHE_MAX_ENTRIES` 可覆盖，任一为 0 时禁用；上限分别为 86400 秒和 4096 项。
缓存有效期不因命中而延长，重启后清空，不保存文件正文、原始响应或签名下载地址。

每次非并发命中先查询 fetch_files；不存在、HTTP 404/410 或失败状态会淘汰旧 ID，
使用本次输入重传。网络、鉴权或限流错误不会被当作文件失效；托管账号可在原账号刷新 token。
损坏 JSON、HTML 或缺少文件列表的异常响应不代表文件缺失，不触发重新上传。
文件就绪轮询遇到鉴权或永久请求错误立即停止；托管账号可刷新原账号一次，仍失败则返回原始错误类型。
超时、限流和服务端临时错误仍在原有总时限内重试。内联上传、批量等待和 `/v1/files` 对鉴权失败使用相同状态码。
并发相同上传/查询合并；等待方取消不影响发起方，发起方取消后仍有效的等待方可重试。
凭证/账号在重试中变化的结果不共享。成功删除任意会话后保守清除该凭证的复用项，
并阻止删除期间的在途请求重新填充缓存；新 HAR 并未证明删除后的旧文件 ID 仍可引用。
显式传入的 file_id 没有原始字节，不能自动恢复已失效文件，仍由调用方重新上传。

本地文件正文缓存与上述 ID 缓存分开：正文保留 30 分钟，单文件上限 100 MiB，
总上限 256 MiB / 256 项。写入时回收过期项，容量不足时淘汰最接近到期的项；
读写均复制字节，调用方修改自己的缓冲区不会改变缓存。正文被淘汰不等于上游 file_id 失效。
远程上传设置缓存也有 128 项硬上限；全部槽位正在查询时使用显式配置/默认值，避免继续扩张。

动画 GIF/APNG/WebP 保留原字节；GIF 回归验证 multipart 不丢动画帧、延时或循环信息。
最新抓包事实、复用边界和请求指纹验证见 [图片复用与 GIF 核对](upload-reuse-gif-alignment.md)。
HISTORY/TOOLS 与其他上传均使用
本次标准请求的最终思考开关。生成的 HISTORY/TOOLS 保留原有的按账号重建路径。
聊天 JSON 的字段顺序、缺省 audio_id 和续聊 model_type 的 null 处理仅在发送边界完成，
共享提示词组装、模型别名与工具调用规则继续按基准项目执行。

## 模型列表

规范模型共四个：

- `deepseek-v4.1-flash`
- `deepseek-v4.1-flash-nothinking`
- `deepseek-v4.1-flash-search`
- `deepseek-v4.1-flash-search-nothinking`

旧 DeepSeek flash/pro/vision 名称及内置 OpenAI、Claude、Gemini alias 按基准规则解析。
自定义 alias 支持最多八跳。仅保留 -nothinking 行为后缀；旧的 -autodelete、-forcehistory、
-thinkinginject、-v4f 不再暴露或解析。思考注入、上下文文件和自动删除由各自配置控制。
搜索通过 search_enabled 传给上游，本项目不注入独立的联网搜索提示词。

实现及回归：`internal/config/models.go`、`internal/config/model_alias_test.go`，
API 测试页面的默认模型和说明同步更新。

## 验证

运行 Go 全量测试、Node 工具解析/流式兼容测试、lint、重构行数检查和 WebUI 构建。
核心回归覆盖格式规则、跨分片标记、CDATA 修复、历史最后一条 user 保留、
重建时格式开关保真、配置部分更新及 OpenAI/Responses/Claude/Gemini 的协议输出。

### 账号与用量功能对齐

共享提示词、工具历史、拆分和文件注入继续在标准请求路径中处理。各协议的历史完成回调同时写入独立用量账本；关闭聊天记录不会关闭用量统计。后台设置改由注册表统一读写和校验，原有注入提示词、工具开关及阈值仍通过共享配置访问。新增账号调度、保活和语音功能的范围见 [功能对齐说明](function-alignment.md)。

## Android APK 上传规则补充

共享上传层保留数美 auto 登录 ID，并使用独立安装标识构造上传/轮询/聊天请求头。
相册 JPEG/PNG 优先使用请求/账号/环境显式配置，再由账号远程 settings 补足；
没有有效 photo_compress_ratio 或远程比例时保留原图；启用时按 APK 的有损 WebP/JPEG、
30 MiB 采样预算、EXIF 和随机文件名规则处理，随后以实际字节计算 x-file-size。
preserve_image=true 保留原图；文档、文件引用、账号绑定及纯文本提示词组装语义不变。
详见 [APK 请求对齐核对](apk-request-alignment.md)，包括尚未覆盖的格式和远程设置边界。
本轮原始抓包证据及批量轮询规则见 [三份 HAR 补充](android-har-reference-2026-10-29.md)。
