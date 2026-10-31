# API → 纯文本上下文兼容说明

本项目最初基于 `ouqiting20/ds2api` 的 `f9d2085` 对齐提示词、工具调用、历史拆分和模型目录；当前消息保真与任务拆分规则以本文为准。请求头、登录及上游传输继续使用本项目原有实现。

## 统一请求链路

OpenAI Chat、Responses、Claude 和 Gemini 先转换为标准消息，再通过
`internal/promptcompat` 统一生成提示词。协议适配器负责输入和输出格式，工具规则、
思考注入和历史拆分使用共享逻辑。Vercel Node 使用 Go prepare 返回的最终提示词，
并同步使用相同的工具标记及解析规则。

最终上下文保留 system、developer、user、assistant、tool 各自的角色与独立轮次，
相邻同角色消息不再合并。每份转录使用由消息内容派生、且不出现在正文中的
`[EndMessage:…]` 结束标记；开头说明角色标签只有位于消息边界时才表示真实角色。
用户粘贴的角色标签、Markdown 图片、正文中的 EPSE 及工具参数 CDATA 均保持原文。
转录前言说明 system、developer、user 的优先级及同角色后续指令的更新关系。
这是一套纯文本转录协议，不能赋予字符串原生 API 的角色权限。
转录前言明确结束标记只属于输入元数据，不应由模型输出。若正文仍生成
`[EndMessage:` 加十六位十六进制 ID 的完整标记，Go/Node 的共享内容策略在该处结束本轮，
丢弃标记及其后虚构的续写；跨 SSE 分片时先暂存可能的前缀，防止半截标记进入正文。
CDATA、双引号字符串、Markdown 围栏和行内代码里的字面量保留。

历史 reasoning 以 `[reasoning_content]` 块保留。工具结果的 `tool_call_id`、名称及
可用的 `is_error` 放入消息元数据；历史 invoke 携带 `call_id`，按 ID 配对而不是猜测顺序。
字符串使用 CDATA，非字符串参数使用 JSON 字面量，完整保留 null、空字符串、空对象、
空数组、嵌套数组和大整数文本。普通发送和 HISTORY.txt 使用同一转录器及元数据。
已完成的 assistant 消息结束后生成新的 assistant 轮次；仅显式 `prefix: true`
视为预填充续写，底部提醒放在预填充之前。

相关实现：

- `internal/prompt/messages.go`：角色标记和最终文本。
- `internal/promptcompat/prompt_build.go`：统一提示词组装。
- `internal/promptcompat/thinking_injection.go`：可选思考注入及重建。
- `internal/httpapi/claude/standard_request.go`、`internal/httpapi/gemini/convert_request.go`：协议归一化。

## 工具提示词和调用格式

工具说明来自本次请求：`Tool`、`Description`、`Parameters`（JSON Schema）。
说明后附统一格式规范、调用决策和参数类型规则。请求组装只注入与工具名无关的语法骨架；
不再按工具名称套用固定参数示例，避免同名工具 schema 不同导致误导。

```xml
<|EPSE|tool_calls>
  <|EPSE|invoke name="read_file">
    <|EPSE|parameter name="path"><![CDATA[src/main.go]]></|EPSE|parameter>
  </|EPSE|invoke>
</|EPSE|tool_calls>
```

字符串必须使用 CDATA；对象和数组优先采用 JSON 字面量，也兼容嵌套元素和重复 `<item>`；数字、布尔值和
null 保持字面量。schema 允许的空值合法；不得捏造未知参数、输出占位符或 Markdown 围栏。
提示词明确区分已执行的历史调用与当前待完成的任务，避免重复执行成功操作，
同时允许为了新信息、不同参数或失败重试再次调用同一工具。

完整规范并入首条 system 消息；底部格式提醒以独立 system 消息放在接近生成点的位置。
`bottom_format_injection.enabled` 默认 true，`prompt` 留空使用内置提醒，可关闭或自定义。
规范和默认提醒明确仅约束客户端提供的工具，并禁止以 DSML 前缀生成这些调用；
每个 invoke 都必须包含其 schema 声明的全部必填参数，包括同一批次中的各个调用。
已保存为显式文本的旧内置默认提醒（此前本项目版本及上游有/无作用域版本）按新默认解释，
仅匹配已知完整原文；真正的自定义文本及关闭状态保留。
只有有工具的请求才注入这些规则；OpenAI/Gemini 还兼容客户端在 system 文本中声明工具的形态。
Claude 的工具声明只采用顶层 tools，避免普通 system 文本误触发工具协议。

`EPSE` 是内部规范关键字。生成工具协议时替换成按调用者稳定派生的六位标识，
替换发生在合并用户内容之前，跳过 CDATA；返回文本仅把标签前缀恢复为 EPSE，
正文、路径及 CDATA 内的同名字符串保持原样。流式归一化保留跨分片的标签/CDATA 状态，
等待标识边界后再替换，避免改写包含标识的更长单词。历史文件、TOOLS.txt、
思考注入后的重建及修复提示词也沿用同一标识。
服务端密钥自动生成并保存，也可通过 `DS2API_TOOL_MARKER_SECRET` 固定。

API Key 配置中的 `force_disable_tools` 会在工具检测前清除工具定义并阻止格式注入。
账号管理页的新建/编辑 Key 弹窗读取 `credential_flag_specs`，显示同一开关；Key 列表显示生效徽标，
新增和编辑均将开关值提交给后端。
工具选择策略由标准请求持有：OpenAI Chat 和 Responses 解析 auto/none/required/指定工具，及标准 `{type:"allowed_tools", mode, tools}` 子集。Claude 的 auto/none/any/tool 与 disable_parallel_tool_use、Gemini 的 functionCallingConfig 先映射成同一策略。
Chat 的 Go、Vercel Node 和 Responses 流式路径对受约束调用延迟交付，在收尾校验 none、必需调用及指定工具；
违反策略返回 tool_choice_violation，不能先输出禁止的调用再报错。工具执行器仍应验证工具名和参数。
required/指定工具约束不会把上游空响应误判成策略违规：空响应继续走原有重试、审核及上游错误分类；
实际返回了普通答案却没有要求的调用时，仍返回 tool_choice_violation。

OpenAI Chat/Responses 的 `parallel_tool_calls: false` 会转换为共享单调用约束：
提示词要求每轮最多一个调用，收尾也检查数量；不合规时返回错误，不静默截掉其余调用。
提供正式工具目录时，auto 模式也检查工具名，名称区分大小写；没有正式目录的 system 文本工具仍保留兼容检测。JSON Schema 在共享层编译和校验（含本地 $ref、required、enum、额外字段和嵌套结构），不符合参数约束返回 invalid_tool_arguments。外部 $ref 不读取网络或文件。受约束调用在完整校验前不会交付。
参数解析不再静默丢弃未闭合或未命名的兄弟 parameter 节点，即使它们不是 schema 必填项；
invoke 内的 JSON 兼容包必须使用对象类型的 input 或 parameters，不能把错误类型或未知包结构变成空参数。
完整 JSON 字符串内的工具标签作为字面值保留，不参与标签匹配和格式归一化。
LLM 格式修复在占位符还原前检查工具名、参数名及节点顺序，并绑定原始参数值与占位符所属参数；
修复结果还必须全部解析成功。修复不能通过改路径、删参数、增加调用或只保留部分调用来“成功”。
若残余坏码明确包含本次声明的客户端工具调用，且收尾无法安全修复，
不交付残缺调用或作为正常正文释放。共享 completion runtime 会使用独立的工具纠正提示词，在同一账号补偿生成一次；
仍然失败时，托管账号可使用原始请求在另一个匹配账号的新会话重新生成一次，最终耗尽恢复机会后返回 HTTP 422 / 对应流式错误帧。
生成重试与格式修复分别计数，结束时复用已得到的错误，不重复发起同一轮格式修复。
流式补偿仅允许在未交付正文或工具调用时进行；正式 tools 的调用在完整参数和策略校验后才交付，不能通过提前发出参数片段绕过校验。
每次补偿清除未交付的正文、工具片段、工具检测思考、上游消息 ID 及错误状态，已流出的可见思考保留。
Claude、Chat、Responses 和 Gemini 的非流式调用复用共享错误分类；Claude、Chat 和 Responses 的 Go 流式路径与 Vercel Node 路径保持同一恢复边界。
临时服务故障可重试或换号，审核、超长输入、无效输入请求和客户端取消不会被当作生成失败反复提交。
换号与首次取号共用有容量限制的等待队列，单次换号等待最多 30 秒，且服从请求取消；唤醒按账号和工具池匹配。

Go 流式消费每次尝试拥有独立的读取生命周期。主动截断、超时或终止会取消该次后台读取并关闭上游正文，
收尾及下一次补偿继续使用调用方的有效上下文，避免旧读取协程滞留，也避免连带取消新的尝试。

上游 PoW 预取按账号或直通 token 的不可逆摘要与目标 endpoint 隔离，直通请求不会共用空账号键。
已缓存与正在预取的条目合计最多 256 项；写入时回收过期项，满载时不淘汰正在预取的条目。
缓存键不保存原始直通 token，缓存未命中或满载时仍可走同步获取。
孤立的结束标签不代表失败的调用；EPSE 与 DSML 的结束壳层均单独移除，后续正文和调用继续处理。
指定工具若被同一请求的 `allowed_tools` 排除，在归一化阶段直接报错，不能静默扩大允许集合。

## Agent 工具循环

服务负责生成和转换工具调用，实际工具仍由调用端执行。调用端执行后携带原调用 ID
回传结果，下一次新建上游会话时重新打包任务、调用和结果。提示词要求只并行发出互不依赖、
参数已经确定的调用；有依赖的步骤等待真实结果后再继续，不提前编造结果或宣称成功。
失败后的后续动作应参考实际错误，不能无条件重复相同失败调用。

- Chat Go/Node 的流式工具 index 在整个响应内递增，不因新的工具块重新从 0 开始。
- Responses 保留整轮的 item ID、call ID 和 output index，事件中的调用与最终 completed 对象保持一致，供下一轮结果回填。
- Claude/Gemini 优先使用显式 ID。兼容缺少 ID 的同名结果时，按未返回调用的顺序配对；已匹配项移出队列，合成 ID 避开本请求中已有的 ID。
- 缺少 ID 且结果乱序时不能可靠还原配对关系，调用端应传回原 ID。
- Responses 的结果错误标志和显式 null 会保留，避免下一轮把失败当成成功或把 null 当成空字符串。
- Node 与 Go 一样，仅在正文为空时尝试从推理通道恢复工具调用；关闭推理展示时，该内部检测不产生 reasoning_content 输出。

相关实现：`internal/toolcall/tool_prompt.go`、`internal/promptcompat/tool_prompt.go`、
`internal/toolcall/marker.go`。

### DSH 辅助插件

本项目可以配合 [ouqiting/dsh-2api](https://github.com/ouqiting/dsh-2api) 的
`@ds2api/dsh-epse-regeneration-guard` 使用。插件安装在 DSH 客户端；服务端没有 Cordis
插件加载入口。已检查插件 v0.8.0 / `300de60`，并用 DSH 0.2.0-rc.2 的真实 Session
及原插件 listener 完成离线联动验证，未连接真实上游或执行工具。

| 所处位置 | 标签或交付形式 |
|---|---|
| 服务端内部格式规范 | `EPSE` |
| 发往模型的提示词及上游原始输出 | 按 API Key 派生的标识，例如 `NTNAFD` |
| Go / Node 扫描与解析输入 | 动态标识先归一化回 `EPSE`，支持跨分片 |
| 成功返回给 DSH 的调用 | OpenAI 结构化 `tool_calls`，由客户端执行 |
| 未作为调用交付的格式残片 | 按共享校验规则报错或保留文本；文本中的协议前缀已经归一化 |

响应记录优先保存上游原文，因此面板中的 `NTNAFD` 不代表 DSH 收到的文本也使用该前缀。
无需把所有 API Key 改成固定标识。原插件虽然只匹配 `EPSE`，仍能识别服务端归一化后
作为正文保留的成对标签；正常的 `tool-calls` 完成事件不会触发其重生成。

在 DSH 所在机器安装后重启 DSH：

```sh
dsh plugin --profile web add github:ouqiting/dsh-2api
```

可用插件已有的 `targetProviders` 限定当前 DS2API provider 的实际名称，
`maxRegenerationsPerTurn` 默认为每个 step 两次。插件的配置由 DSH 管理。

原插件仅把成功文本中的标签泄漏 / 自定义触发词转为请求失败并重生成；它不认领
`invalid_tool_arguments` 等服务端错误码。把这些字样填入触发词也不会让错误响应自动重试。
可确定的参数外壳由本项目在 schema 校验前恢复；真正缺参或无法安全恢复的调用仍返回
HTTP 422 / 对应 SSE 错误事件，交给 DSH 的请求错误处理流程，不伪装成成功文本。
离线验证覆盖动态标识的所有双分片切分、残缺 description 的本地恢复、原插件对归一化
正文的重生成、正常工具调用放行及 422 错误原样委托；不代表已经完成用户部署的端到端联调。

## 工具输出解析与修复

Go 和 Node 均使用结构化扫描，容忍受支持的分隔符漂移、大小写、引号和部分缺失标签。
旧 canonical XML 保留兼容解析，但提示词和历史只示范带标记的格式。
Markdown 代码围栏/行内代码里的工具示例不会执行；完整却无法解释的坏块作为普通文本保留。
CDATA 中的标签示例及原始参数内容应逐字保真，不能被确定性修复吞掉或重排。
当上游把参数开标签误写为 `<|EPSE|description">`、`<|EPSE|old_string">` 这类
「字段名 + 尾引号」形式时，Go 和 Node 扫描器将其恢复为具名 parameter。仅接受带工具前缀、
ASCII 字段名和单个尾引号的明确结构，字段名保持大小写；仍须有匹配的 parameter 闭标签。
正文、CDATA、JSON 字符串及 Markdown 示例保持原文；不补造缺失字段或参数值，正式 schema
校验继续生效。该恢复发生在常规解析阶段，不增加模型请求；LLM 兜底也将这些字段纳入来源一致性校验。
流式捕获在完整调用之外遇到孤立或重复的工具闭合标签时，仅丢弃该闭合标签并继续处理后文，
不会把它作为失败调用正文释放，也不会吞掉随后的真实调用。收尾清理同时移除残留的带前缀闭合标签。
Node 的扫描、匹配和 CDATA 修复使用保留空白的原文字符串，工具名等语义字段单独 trim；
禁止先 trim 待扫描文本再使用返回的偏移切割原文，否则前导换行会使标签边界错位。

流式识别出工具块后输出协议原生工具增量，避免原始标签泄漏到正文；收尾阶段还会检测
仅思维链中存在的工具调用，并按客户端 thinking 开关决定是否显示思维链。
当存在残余工具意图时，收尾先执行本地恢复，再决定是否发起一次 LLM 格式修复。
收尾先尝试保守的本地 DSML 恢复：仅接受完整的 `calls/invoke/parameter` 层级，
每个参数必须明确声明 `string="true"` 或 `string="false"`。前者逐字保留原始内容，
后者只接受合法 JSON 字面量并保留大整数精度；未知属性、重复参数、截断、混合残余意图均不猜测。
本地恢复仍经过相同的工具选择和参数 schema 校验。无法本地恢复时才进入已有 LLM 修复；
常规 parser/sieve 对 DSML 的识别与隐藏规则不变，Node 的正式 tools 路径复用 Go 收尾校验端点。
LLM 修复复用同一账号、新建会话、关闭思考，继承客户端取消和期限，最多十秒。会话删除进入固定容量队列（128 项、2 个 worker），使用凭据快照，独立十秒期限，不阻塞工具交付；队列满或删除失败写日志。
参数通过 DS_SLOT 占位保护；修复失败或占位被破坏时保留原始残片，不输出半修复内容。
已流式发出的结构化调用不会回滚并重修。

疑似坏工具块缓存在独立区域，完成解析/修复后才决定是否作为正文释放，避免先发送坏代码再发送修复后的调用。正文截断规则及流式解析在 Go 和 Node 保持一致，包括闭标签复读和虚构 `[Tool]:` 角色；CDATA、引号字符串、Markdown 代码中的同名文本保留，跨分片状态不重扫全文。
详见 [toolcall-semantics.md](toolcall-semantics.md)。

续答快照的去重在 Go（字符串与 Builder 路径）和 Node 中采用同一规则：
只有至少 32 个 Unicode 字符的片段才参与前缀快照判断；更长的快照只追加新后缀，
相同或较短的旧快照不重复追加。短片段即使包含 emoji 或重复前缀也按正常增量保留。
长度判断最多扫描 32 个字符，避免对大段文本先做一次无条件的全文计数。

非流式收集在明确结束信号之前发生底层读取错误时，保留原始部分输出供诊断，
通过共享 runtime 返回 HTTP 502 / `upstream_read_error`；部分正文或已解析工具不能将其改写为成功，
也不会把断连误当成空输出触发补偿。收到 `FINISHED` 后仍可继续收集迟到的引用元数据，
此阶段的连接关闭不影响已完成的结果。工具格式修复和保活同样不得把读取失败记为成功。

响应记录的 `messages` 保留纯工具调用 assistant 轮次、空工具结果、原调用 ID、名称、
错误标志、reasoning 和 prefix；正式 tools schema、规范化工具选择策略及调用者标识也单独归档。
这些字段为 JSON 快照并独立复制，避免调用方修改原请求或读取结果后污染记录。旧记录可继续读取，
已有 `content` 仍是历史展示使用的原始正文，不代表最终交付给客户端的结构化 tool_calls。
本地回放方法和本轮样本分析见 [tool-call-history-audit.md](tool-call-history-audit.md)。
与 `ouqiting20/ds2api` 的提示词位置、DSML 提醒和兜底实现对比见
[upstream-toolcall-comparison.md](upstream-toolcall-comparison.md)。

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

- 以最近一次 user 任务（包括连续 user 输入）及之后完整的 assistant/tool 链作为活动上下文，保留在 live prompt。
- 如果活动结果引用了更早的工具调用，切分点回退到对应调用，避免跨文件拆散调用和结果。
- 更早的消息上传 HISTORY.txt；此前的 system/developer 指令按原顺序保留在文件中，并在主提示词中重复固定，避免只在附件中生效。
- 只有首个任务且已进入 assistant/tool 循环时，不为拆分强行把当前工具链搬进附件。
- 单条长首轮 user 仍可外置到 HISTORY.txt，主提示词明确其位置；只有前置指令时，兼容外置指令同时保留最新 user 和指令的行为。
- 文件按「本次生效的思考强度系统提示词（若启用）→ `# HISTORY.txt` 与先前历史说明 → 历史消息」排列，编号角色标题和独立消息边界与 live prompt 相同。
- 工具 schema 单独上传 TOOLS.txt，主提示词明确命名两个文件、它们的用途、当前任务位置和调用结果配对规则。
- 图片和文件在原消息位置保留附件引用；不能用空正文判断并删除工具调用、空工具结果或图片轮次。
- 生成文件按当前账号上传；账号切换后按原字节重传，不再次改写正文。usage 按文件和主提示词完整上下文计算。

思考注入仍由现有开关控制，仅在本次请求启用 thinking 时生效。它作为独立的生成系统前缀
放在最终提示词的最开头，`Reasoning Effort: ...` 紧跟 `<｜System｜>` 标签，先于搜索说明、
转录规则、工具说明、调用者系统指令及所有用户消息。自定义思考提示词也使用同一位置。
客户端 Messages 和 HISTORY.txt 中的用户原文保持不变；注入文本由 StandardRequest 的
ThinkingInjectionPrompt 单独携带。历史外置时，同一系统前缀也放到 HISTORY.txt 最开头，先于历史说明与续写指引；
实时续写保留此前缀。上传文件、本地 HistoryText 与换号重传保持相同字节，token 统计同时计入文件和实时提示词中的实际内容。
重复应用不会叠加；关闭开关或本轮未启用 thinking 时不添加前缀。
默认文本现为以下原文；已保存的旧内置默认值自动按新默认值解释，自定义文本保留：

```text
Reasoning Effort: Beyond maximum — exhaustive, relentless, and uncompromising.
You MUST reason with the utmost depth and rigor, leaving absolutely nothing to chance: exhaustively decompose the problem into its most fundamental components, trace every causal chain to its root, and resolve the underlying cause rather than any surface symptom. Do not stop reasoning until you have independently verified the solution from multiple angles and are certain that no assumption remains unchecked and no error remains undiscovered.
```

该文本是提示词，不会创建上游不存在的推理预算档位，也不代表模型推理质量得到保证。

旧 `history_split` 与 `current_input_file` 配置键均忽略且不再写回。
旧配置需要手动将 `current_input_file` 改名为 `input_file`。
模型后缀 `-forcehistory` 不再触发拆分。

`expert_prompt_segment` 保留配置兼容，但 enabled 强制为 false，与基准项目的停用状态一致；
旧配置不能重新打开发送后停止的分段路径。所有规范模型均映射为 default 模式。

相关实现：`internal/promptcompat/history_transcript.go`、
`internal/contextfile/current_input_file.go`、`internal/completionruntime`。

## 图片、文件与发送边界

三份 Android App 2.7.1 HAR 对齐的请求头、设备配置、HTTP/2 线序和上传细节见
[android-har-alignment.md](android-har-alignment.md)。
OpenAI Chat/Responses、Claude base64 image/document 与 Gemini inlineData 共用文件上传边界，
文件 ID 进入标准请求的 RefFileIDs；base64 不进入纯文本 prompt。
内联文件及显式文件引用的请求固定当前账号，防止重试中混用跨账号文件 ID。
通过 `/v1/files` 单独上传后，多账号环境需用返回的 account_id 固定后续请求的目标账号。
PNG/JPEG 按显式或远程压缩配置处理；没有有效比例时保留原文件，文档内容保持原样。
同一请求的内联文件先上传，再合并轮询，所有文件成功后才用于聊天。
OpenAI Chat/Responses 剥离已上传的图片块时保留带 file_id 的文本附件引用及用户轮次；纯图片首轮不会
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
搜索通过 search_enabled 传给上游，并为启用搜索的模型注入上游内置联网提示词。

### 搜索模型的内置联网提示词

移植自 `ouqiting20/ds2api` 的 `a2c51de`，提示词原文统一保存在
`internal/promptcompat/search_prompt.go` 的 `BuiltinSearchPrompt`。
按别名解析后的 `StandardRequest.Search` 开关生效，包括 `deepseek-v4.1-flash-search`、
`deepseek-v4.1-flash-search-nothinking` 和映射到它们的别名；普通模型不注入。

提示词说明原生 `search({"queries":"...||..."})`、每次最多 5 条短关键词查询、
每条中文查询最多 30 个中文字符，以及 `open({"open_list":[{"cursor":0,"id":0}]})`。
搜索轮次和结果编号从 0 开始，只能引用实际返回的条目；独立查询可并行，依赖查询根据前一轮结果调整，
最终按实际来源整合答案。参数示例不代表已经执行的搜索结果。

`ApplySearchPrompt` 在各协议的思考注入之后、历史文件处理之前加入独立 system 搜索说明，
并在历史外置重建后复用。启用思考注入时，其 `<｜System｜>` 前缀始终位于搜索说明之前。
OpenAI Chat、Responses、Claude、Gemini 共用这一份文本；Vercel Node 使用 Go prepare / switch
生成的提示词。消息重建和历史文件续写保留说明，计入 PromptTokenText，重复应用不叠加。

内置 search/open 由上游原生执行，不加入客户端 ToolsRaw / ToolNames，也不改写原始 Messages 或号池路由。
客户端即使声明同名工具，其 schema、工具选择策略和调用格式仍只约束该客户端工具。
EPSE-only 与 DSML 禁用规则继续限定在客户端工具范围，生成说明中的协议名称沿用调用者标识，
不改写用户正文。`force_disable_tools` 控制客户端工具，内置搜索仍由模型能力开关决定。

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

## Responses 多轮状态与 custom tools

`previous_response_id` 按调用者检索输入和输出的可重放上下文，在本次上传与账号分配前恢复；不存在、过期或跨调用者引用返回错误，不启动上游请求。只继承对话输入和 output items，不继承上一轮顶层 `instructions`；本轮应重发 instructions 与 tools。`store:false` 不保存 Responses 对象或续接状态（独立的响应日志策略仍由其配置控制）。

状态存储是进程内缓存，TTL 使用 responses store 配置（默认 15 分钟），最多 256 项、序列化预算 128 MiB，超额淘汰较早条目；服务重启、多实例未共享状态或缓存淘汰后，请携带完整历史。写入和读取深拷贝嵌套数据，克隆工作不持有全局读取锁。未支持的 Conversations API 明确拒绝。

标准 `reasoning` items 的 content/summary 作为 assistant reasoning 恢复，不再降级为 user 文本；不解密 encrypted_content。custom_tool_call 与 custom_tool_call_output 保留 assistant/tool 角色和 call_id。`type:custom` 的自由文本在内部工具协议中使用唯一字符串参数 input，输出还原为 custom_tool_call/input 和 custom_tool_call_input 的 delta/done 事件。format/grammar 描述进入提示词；不宣称原生 CFG 约束解码。当前服务器不执行 OpenAI 托管工具。

Vercel prepare 声明 validate_tools 能力时，Node 将原始候选文本交给带租约校验的 Go 内部端点，复用同一参数校验、修复和工具策略，再发送 arguments 字符串；JavaScript 不重新解析该字符串中的大整数。校验端点不可用时返回错误，不交付未经校验的调用。

工具参数在输入解码、历史渲染、输出解析和 Responses 回译中保持数字值；不能被 float64 无损重新编码的数值保留为 json.Number。CDATA 字符串不作数字推断。Claude 工具 ID 使用随机 UUID，避免同秒多轮重名；Gemini functionCall 同样返回 ID，供原样回填。

## 共享实现的归属

HISTORY.txt / TOOLS.txt 处理统一位于 `internal/contextfile`，供各协议和 `completionruntime` 直接调用。空输出 / 工具输出重试提示词与用量累计辅助位于 `internal/completionruntime/empty_retry.go`；输出清洗和流式累积分别位于 `textclean`、`assistantturn`。这些迁移不改变提示词、上传顺序或重试规则，避免让某个 HTTP 协议目录成为共享业务的所有者。

## 输出侧 SSE 元数据边界

Go `sse/paths.go` 与 Node `sse_paths.js` 对齐片段状态的绝对、response 相对及集合相对路径；嵌套 patch 在提取正文之前执行相同过滤，防止 `WIP` / `FINISHED` 或累计 token 字段混入可见答案。`token_usage` 在共享常量中声明为元数据，上游累计用量仍不替代内部估算。此变更不调整输入提示词、工具规则或历史转录。

## Node 租约与断流收尾

Node prepare 后持有的账号租约不改变输入归一化 / 历史文件语义；但必须在 prepare 异常或客户端断开时释放。401 内部恢复沿用 Go 的共享凭据恢复步骤，`auth_failed` 不得在控制请求编码时丢失。续答失败的部分输出先冲刷有效片段，再发送错误、`[DONE]` 和 HTTP 结束信号，避免连接悬挂或错误帧后的迟到正文。
