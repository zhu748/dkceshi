# API → 纯文本上下文兼容说明

本项目最初基于 `ouqiting20/ds2api` 的 `f9d2085` 对齐提示词、工具调用、历史拆分和模型目录；当前消息保真与任务拆分规则以本文为准。请求头、登录及上游传输继续使用本项目原有实现。

## 2026-11-06：原生工具历史与文本块复查

- Claude 的工具 ID 别名及 Gemini 的 id/call_id/callId/tool_call_id 在适配前校验类型与一致性，
  不通过字段优先级掩盖冲突。Claude tool_use 支持 tool_call_id，并与结果使用同一身份；
  tool_result 的 is_error 必须是布尔值，显式 input/args 必须是参数对象，缺省仍兼容空对象。
  校验同时覆盖直接入口和转接 OpenAI 的路径，在取用上游账号之前返回输入错误。
- Responses output/content 与 Gemini response/output 的并行结果入口必须保持相同 JSON 数据，
  显式 null 也参与比较，不再回退覆盖。Gemini 单独的 null 结果保留为 null，和媒体 parts
  同时存在时同样保留；嵌套 parts 的错误形状与文本别名也在展开之前拒绝。
  工具历史 arguments/input 的别名检查同样区分显式 null 与未提供字段。
- Responses 独立 text/input_text/output_text 按完整内容块进入共享序列化，保留 annotations、
  citations 和空白；refusal 块及 Gemini systemInstruction.text 的兼容入口也保留上下文元数据。
  即时 prompt 与 HISTORY.txt 共用这些规则。
- Go 与 Node 的嵌套 XML 参数扫描同时保护混合正文顺序、注释和处理指令。此类内容不能无损
  转为对象时保留完整原文，禁止拼接为 _text 或丢掉注释再作为对象执行；字符串 Schema 接受
  原文，对象 Schema 进入既有纠正生成流程。仅有普通子节点的 XML 兼容对象解析保持可用，
  JSON 字符串及 CDATA 的原有保真规则继续适用。

## 2026-11-06：上下文完整性与流式收尾修复

- 图片清理只替换图片载荷，保留原消息边界、refusal、音频转录/引用及身份元数据。
  当前消息带图片时，不能因为历史 assistant 的 content 为 null 或空串而删掉整条历史。
  HTTPS 等外部图片 URL 尚不支持上传，现返回明确的 400，要求使用 data URL 或已上传的
  file_id；不能在没有附件引用的情况下移除图片后继续生成。已有 file_id 不依赖旧 URL 重传。
- Go/Node 的嵌套兼容 XML 遇到属性或命名空间时保留完整参数字符串，包括实体和内部空白，
  不将它缩减成丢掉属性的对象。字符串 Schema 可以接受原文；对象 Schema 返回参数错误，
  进入现有纠正生成流程。格式修复不能通过删除属性来满足 Schema，JSON 字符串里的 XML
  示例仍是普通 JSON 数据。
- Responses 原生调用的调用 ID 依次读取 call_id、tool_call_id、兼容 id。调用和结果对齐后，
  历史拆分保留它们的依赖关系。call_id/tool_call_id 冲突仍在输入转换前拒绝；原生项 id
  与 call_id 是不同身份字段，不把二者强制解释成相同别名。
- 已知文本块中的 text/content，以及思考块中的 text/thinking/content 同时出现时，必须逐
  字节一致，否则返回输入错误。这项校验也覆盖原生 system 容器及 Responses 思考摘要条目。
  思考块的 annotations 等上下文元数据跟随正文进入即时 prompt 和 HISTORY.txt，Claude、
  Gemini 及 Responses 共同保留；Responses 的独立 text 与 content/summary 共同保留。
- 业务数据中的 signature/base64 字段按所在容器的协议类型判断，不再仅凭字段名抹去普通
  业务文本。明确的媒体载荷、附件自身签名、思考签名与 encrypted_content 仍按原有规则
  省略；嵌套业务对象不会继承附件或思考的签名判定。
- Vercel prepare 用共享输出策略返回 validate_prose，required/forced 工具策略及结构化输出
  仍需 Go 收尾校验。普通 auto 回答不会仅因携带工具目录而依赖额外校验请求，也不整体缓冲
  正文。真实工具候选、残缺协议前缀和未解决的工具捕获仍交给 Go 原子校验；校验不可用时不
  交付工具调用。已有可见正文时，思考中的工具计划不触发额外校验，与共享检测规则一致。
- Chat（含 Vercel prepare）、Claude/Gemini 直接入口在 Key 开关和附件预处理之前捕获输入
  字段摘要。私有类型的请求内快照在后续重建与附件重放时沿用，客户端 JSON 不能伪造它。
  Responses 继续在续接展开前捕获原输入摘要；内部协议转接后的适配器摘要仍描述其实际入口。
- context_manifest 的标准消息字段增加 omitted_reason，并增加 omissions 汇总，分别说明
  transport_hint、field_has_no_prompt_serializer 和未转录消息。历史外置后仍汇总原标准消息
  的省略原因，不保存字段值。它是标准消息层面的诊断，不是嵌套字段的自动覆盖证明，也不代表
  上游模型已经读取附件。

回归覆盖图文请求中的拒绝/音频历史、未上传的外部图片、调用 ID 别名与历史依赖、XML 属性
及格式修复保真、业务字段清洗、四种协议的思考元数据、文本别名冲突，以及无工具回答和工具
候选在校验服务不可用时的不同收尾行为。

## 2026-11-06：第二轮来源保真与续接校验

- 格式修复的参数值比较以 CDATA 的原始字符串为准，不把它先解释成 JSON 再比较。
  将原始 boolean/null/number/object 或带引号 JSON 字符串包进 CDATA、导致最终参数类型或
  字符串内容变化的修复结果会被拒绝。原有 CDATA 占位符绑定、原字节恢复及 Schema 校验继续适用。
- Responses 的 `instructions`、原生 reasoning 项、工具名称/ID 别名及 assistant 的 refusal、
  audio 文本和 reasoning_content 形状，在输入项转换前校验。HTTP 入口在读取续接存储和获取
  上游账号前执行相同校验，不能让无效来源在 `previous_response_id` 展开过程中消失。
  instructions 必须是字符串；reasoning content/summary 必须是包含 string text 的对象数组。
  非空兼容 messages 与 input 同时出现、错误类型的 messages/input 也返回 400。
- Chat/Responses 工具历史和 Responses 原生调用若同时提供多个参数入口，必须具有一致的
  JSON 表示；名称、call_id/tool_call_id 别名不能矛盾。参数比较保留大整数，解码失败的历史
  参数字符串按原文比较。只有一个参数入口时不额外解码或编码整份参数。
- Responses 的“调用 A → 新思考 → 调用 B”保留为按源顺序排列的独立 assistant 消息，
  不为合并批次把新思考搬到调用 A 之前。没有后续思考的连续匿名工具项仍可合并。
- 原生 function/custom 调用、输出、reasoning 和附件项即使带有兼容 role 字段，也按其 type
  提取载荷；role 与原生项语义矛盾时返回 400。显式 role 不能把工具 output 转成 content:null。
  内置工具及未知 typed records 保留完整类型数据和已有角色，不降成空的普通 assistant 消息。
- Responses 的 input_fields 摘要在 HTTP 入口展开续接、上传附件之前计算，再随标准请求和
  附件重放沿用；因此原始 input 的证据不会被内部展开后的 messages 摘要覆盖。逐条标准消息
  与最终文件/提示词摘要仍分别反映各自实际阶段，不将摘要清单解释为模型已理解上下文的证明。

## 2026-11-06：消息边界、内容顺序与异常扫描

- 显式空 assistant 正文、仅带 id/name/prefix 的 assistant 消息保留消息边界；prefix 同时进入
  转录元数据，只有最后一条显式 prefix=true 的 assistant 作为续写前缀。Claude/Gemini 的空
  content/parts 数组也保留对应消息。HISTORY.txt、即时 prompt 和提示词重建使用相同规则。
- Responses 只合并没有独立正文或消息元数据的相邻工具项。id、name、prefix（含 false）、
  refusal、audio 等字段阻止合并；用户、system/developer 和工具结果的消息身份字段继续传入
  共享转录。原生 function_call 的调用 ID 仍保存在各调用上，不被误当作消息 ID。
- Claude/Gemini 在新的思考块到来前提交此前正文；Chat/Responses 的 assistant 内容数组按
  原始块顺序转录思考与正文，不把所有思考搬到最前。独立 reasoning_content 在正文前保留，
  与内容数组中的思考逐字节相同时只保留数组表示，避免重复。文本块内部空白保持原样。
- Chat/Responses 在共享历史校验中检查已知 text/input_text/output_text/thinking/reasoning/
  refusal 内容块的文本类型；缺失文本或错误类型返回输入错误，不能在合法邻居旁静默消失。
  兼容字符串数组、未知块及普通工具结果 JSON 对象不按内容块 Schema 解释。
- 工具策略明确为 none 或允许集合为空时，共享流式/非流式收尾跳过格式修复，不发起无效
  修复模型请求；原有工具策略和完整性校验仍拒绝不允许的调用。
- Go/Node 的失败标签探测跳过已排除的连续开标签前缀，仅重新访问最后一个开标签，以保留
  其 CDATA/注释边界；结构预算、候选规范化和修复来源扫描共用此规则。宽松标签解析不能
  把这些字面量当成任意前缀。未完成字面量检查只对
  不超过固定标识长度的尾部比较大小写，避免在每个 `<` 上转换整段后缀。
- context_manifest 增加 input_fields，记录消息适配器入口的 system/systemInstruction、
  instructions、messages/input/contents、tools/functions 字段摘要，去向为 adapter_input。
  这些摘要在消息转换前计算且不包含正文；附件预处理可能已先于适配器运行。raw_request
  继续表示原始 HTTP 字节，source_messages/live_messages 和 prompt/history/tools 摘要表示
  后续阶段。各阶段摘要用于定位变化，不是自动的端到端覆盖证明，也不代表模型已读取附件。

回归包括空消息与预填充、元数据合并边界、混合思考顺序、错误文本类型、禁用工具纠错、
连续开标签中的 CDATA/注释及完整流式筛分。扫描基准：
`go test ./internal/toolcall -run '^$' -bench BenchmarkProtocolPrefixDelimiterRun -benchmem`。

## 工具解析预算与字段去向

- 空输出、工具参数错误和提示词回显的纠正重试，在原 prompt 后追加指令时保留原有全部字节，
  包括首尾空白、CRLF、制表符及 assistant 预填充缩进；Go/Node 的重试 usage 计数采用实际发送文本。
- Claude/Gemini 在转换前检查消息、内容数组、已知文本字段和顶层 system 容器；无效元素返回
  输入错误，不因旁边还有合法消息而跳过。Gemini 的同一 part 携带多个互斥载荷也拒绝，避免只取
  text 后丢掉 functionCall。未知的对象块继续按原有文本清洗规则保留。
- assistant 的独立 reasoning_content 与内容块中的思考正文共同保留；Responses reasoning item
  的 content 与 summary 也共同保留。只有不同字段间逐字节相同的替代表示去重；同一字段内的重复
  块和不同轮次不去重。普通 prompt、HISTORY.txt 和响应记录使用相同的思考文本提取器。
- 嵌套兼容 XML 的普通元素也检查 128 层及 32768 标签预算；检查覆盖递归解析及其本地 fallback，
  超限的结构化参数不能转成部分调用或提交给格式修复模型。CDATA 内的深层 XML 示例保持字符串
  原文，不因兼容解析而变成深层对象。JSON 字符串内的字面量标签继续不计深度。
- 旧式工具历史在请求入口只归一化一次；同名工具按顺序返回结果时，队列移除头部不再反复搬移
  剩余 ID。乱序结果、已完成 ID 再次使用及部分历史的既有配对语义保留。

- Go 与 Node 的 XML 元素提取使用一次配对扫描，避免每个未闭合节点重新扫描剩余全文。
  工具批次在本地解析和格式修复前检查结构预算：最多 128 层工具标签、32768 个标签，
  调用数量沿用 `DS2API_STREAM_MAX_TOOL_CALLS`（默认 128）。预算按源节点计算，损坏调用也计数；
  CDATA、完整 JSON 字符串和代码示例中的标签不计数。超限批次整体拒绝，不交付已解析的部分，
  也不向修复模型提交超限内容。
- Chat/Responses 的标准工具历史先验证形状，再进行旧式兼容转换。非对象消息、错误类型的
  `tool_calls`、缺少名称的调用、冲突名称或同时携带新旧调用包会返回输入错误；不再静默删除。
  Claude `tool_use` 和 Gemini `functionCall` 的缺失名称也进入同一共享校验。
- parameter 的 `name` 属性仅解码一层实体，保留首尾空白和显式空字符串键；缺少属性仍拒绝。
  历史转录、普通解析、DSML 恢复及格式修复使用相同规则，不靠改名满足 Schema。
- 工具定义中的 `input_examples` 与名称、描述、Schema 一起进入普通 prompt 和 TOOLS.txt，
  例子中的字符串按 JSON 原义保存。示例用于模型上下文，不代替共享参数 Schema 校验。
- JSON 入口和共享参数解码拒绝未配对的 UTF-16 转义，避免 Go 将其替换为 `�` 后继续交付。
  Node 本地严格解析使用相同规则。合法代理对、原本的 `�`、转义后的反斜杠，以及 CDATA
  中作为普通文本展示的 `\ud800` 示例均保留。
  历史 arguments 字符串若包含非法 Unicode 转义，以原文转录，不由标准解码器替换字符。
- 普通业务字符串、MCP 历史输出和未知块中的 Data URL 文本不按外观删除。只有明确的
  文件/图片/音频载荷字段及不透明签名使用省略标记；图片 URL 容器和 Chat 音频容器中的
  二进制仍清洗，避免把未上传的 Base64 当聊天正文发送。
- `context_manifest` 的每条标准消息增加 `fields`，按字段名排序记录字节长度、SHA-256
  和去向：`content_serializer`、`message_metadata`、`transport_only`、`unrendered` 或
  `omitted_message`。历史外置前保存这些记录；生成诊断快照时复制字段列表。
  记录不包含字段值，只描述标准消息字段进入哪个处理器；适配器入口证据另见 input_fields，
  也不保证序列化器内每个嵌套值都保留。原始 HTTP 摘要及最终传输摘要继续分别记录。

## 参数与输入上下文完整性补充

- 同名 parameter 形成数组时，逐个记录并恢复 CDATA 的原始字符串，保留空白、引号、
  `null` 外观和每次出现的顺序。数组的本地引用、组合和位置 Schema 仍决定类型；
  明确要求整数或对象的项继续走原有兼容解析，并由完整 Schema 最终校验。
- 嵌套 XML 的真实 `_text` 子节点不能被混合正文覆盖。发生冲突时保留完整源文本，
  对象 Schema 会拒绝这种歧义输入，再由原有纠正流程处理；Node 本地扫描也不丢弃
  子节点前后的非空白正文，最终交付仍采用 Go 的共享校验结果。
- 原模型的工具格式修复同样检查内容过滤和内容策略截断；已取得完整工具块也不能
  覆盖异常终态，不交付被过滤或截断的修复结果。
- Node 入口读取原始请求流，不访问平台的懒加载 JSON body getter。兼容 Vercel 已恢复的
  data/end 事件流，保留大整数、重复键和原始 JSON 字节，再交由 Go 判定输入是否合法。
  只有已解析对象而没有原始流/字节的调用返回明确错误，禁止重新序列化后冒充原始输入。
- 文件上传仅替换二进制传输内容。文档标题、context、citations 及其他非载荷元数据
  随附件引用进入 prompt 和 HISTORY.txt；显式二进制与不透明签名继续使用省略标记。
  Claude 顶层 system 文本块和 Gemini 文本 part 的上下文元数据使用共享文本渲染规则。
- `context_manifest.raw_request` 保存 JSON/UTF-8 入口验证成功后的原始 HTTP body 长度和
  SHA-256，不保存正文；该摘要在 Node prepare/release 和换号重放间沿用原请求。
  每条标准消息增加 `prompt_content` 摘要，未进入转录的消息附带 `omitted_reason`。
  这些记录分别覆盖原始输入、标准消息、可见转录内容及最终传输，不能直接以不同阶段
  的摘要相等证明语义完整，也不代表上游模型已读取或理解附件。旧记录不补造摘要。
- Responses 的 instructions、历史 reasoning/thinking 以及当前用户消息拆分只用空白
  判断内容是否为空，不修改正文的首尾空格、缩进和 CRLF；合并思考块时逐块保留原文。
  原生附件转换与上传共用上下文元数据保留规则，媒体 display_name 在转换后仍可用于
  匹配工具结果中的引用。Node 读取请求体时若连接提前关闭，立即释放监听器并结束读取。

## 上下文与纠错保真

已闭合 CDATA 的原文是确定的字符串边界。即使其中出现 `</parameter>`、`<invoke>`、
另一个 `<![CDATA[`，也不能通过插入闭合符增加参数或调用。旧的“调用/参数数量更多优先”
本地边界恢复已移除。格式修复对这类闭合内容采用完整的贪婪占位符，禁止切换到边界受限
解释；确实缺少关闭符且原文未形成闭合字面量时，仍保留原有窄修复。
缺失必填字段由共享 Schema 校验和纠正生成处理，不能靠拆分字符串补出字段。

JSON 对象的重复键（含转义后同名键）在 API 输入和工具结构化参数中拒绝，避免默认解码
静默覆盖前值。CDATA 内作为普通字符串引用的重复键示例不受影响；大整数仍无损保存。

`output_text` / 普通文本块附带的 annotations、citations 等上下文元数据保存在正文旁；
Claude 转换与 HISTORY.txt 共用同一渲染器。assistant 历史已有的 `audio.transcript` 及音频 ID
作为转录/引用保留，不从远程音频 ID 推测内容。Responses 的内置工具历史及未知 typed items
以完整、标明类型的数据块保留，不转换为新的可执行客户端调用。
Responses `tool_choice.type=custom` 映射为共享指定工具策略。

响应详情增加 `context_manifest`：记录标准化 API 消息的逐条字节长度与 SHA-256、
实际 live 消息、最终 prompt / HISTORY.txt / 工具目录摘要及附件 ID。历史外置前保存原消息
摘要，使诊断能比较拆分前后。摘要范围是标准化上下文，不代表原始 HTTP 报文字节，也不能
证明上游模型已读取或理解文件；它不重复保存正文或二进制载荷。

Schema 编译缓存同时限制为 128 项及 8 MiB 源数据，单项最高 1 MiB。
较大目录可以复用编译结果，超限项继续按需编译，并发同项编译仍合并。

## Responses 历史合并边界

Responses 的同一 assistant 工具批次可由多个独立 `reasoning` / `function_call` 项组成。
归一化合并工具调用时，按出现顺序拼接每段思考内容；已有思考内容不构成丢弃后续内容的理由。
这条规则同时作用于即时 prompt、HISTORY.txt 和 Responses 续聊上下文。不同角色或正文消息
仍保留消息边界，不跨工具结果合并。

Claude 与 Gemini 的请求适配器和 OpenAI 使用同一账号错误分类：账号暂不可用或日额度耗尽
返回 429，各自渲染相应协议错误体；鉴权失败继续返回 401。未知文本块与已知文本块共同保留完整文本，由请求总大小和外置文件传输控制容量。

## 统一请求链路

### 文本、检索结果和工具媒体的完整性

Claude 的 `document.source.type=text` 按纯文本保留 `source.data`，包括首尾空白、长正文和
形似 Base64 / Data URL 的普通内容。`search_result.content` 及 `document.source.type=content`
中的已知文本块递归保留完整正文；普通 prompt 与 HISTORY.txt 使用同一内容块序列化。
已知及未知内容块均不按单块长度截断，也不根据字符串长度或 Base64 外观猜测二进制。
普通 `data` / `bytes` 业务字段完整保留；显式媒体类型、Base64 编码对象、文件载荷、
签名和加密内容仍使用省略标记。Claude、Gemini 和 OpenAI 共用此规则。

Gemini 的 `functionResponse.parts` 与 JSON `response` 保存在同一标准 tool 消息中。
其中的媒体经共享文件预处理上传，结果保留调用 ID、附件顺序、文件引用和 `display_name`，
用于对应 JSON 响应中的 `$ref`。历史拆分和换号重放沿用相同附件信息。
附件上传与引用发现共用容器遍历规则；工具结果的普通 JSON 对象、函数参数和文本块
内部数据不作为附件容器。文件识别需要明确类型或文件/图片载荷字段，不能仅凭
`name` / `filename` 与 `data` 将业务结果替换为文件。

Go 与 Node 的宽松 JSON 修复只在引号外补键名引号和对象列表的数组括号，按原始位置插入
结构字符，保留字符串、转义、Unicode 和正文空白。未闭合字符串不执行这种结构修复。
XML 重复节点按独立出现次数累积，区分原值中的数组与重复节点列表；单个 `<item>[1,2]</item>`
保留为 `[[1,2]]`，多个数组 item 保留完整层级，不展平。两种运行时共用完整性测试样例，
Node 同时检查所有流式切分位置。

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

Claude 的已知 `tool_result.content` 文本块保留完整原文，包括长文件正文、首尾空白和
看似 Base64 的普通字符串；不再套用未知内容块的 1024 字节截断或 Base64 猜测。
未知块中的明确二进制继续清洗，普通文本不截断。旧式历史
`assistant.function_call` / `role:function` 先转换为标准调用和结果，分配的兼容 ID
避开整个请求已有 ID，转换不修改调用者的原消息。

文本形式的历史工具块只有在整批完整解析、无残余调用时才重建；不完整批次保持原文，
不能因为解析出一个合法调用就删除其他损坏调用。已有 invoke `call_id` 在归一化和
重新渲染时保留；这类历史元数据不作为新响应的交付 ID。读取保护优先识别明确错误标志，
兼容文本状态只匹配短的独立状态消息，不扫描完整源码中的 `error`、`cached` 等词。

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

流式交付按正文和工具协议分别处理：auto/none 模式下，工具边界之前的正文可立即交付，
工具批次始终等待共享收尾解析、必要修复、策略与 Schema 校验；required/指定工具模式继续
暂存正文，保留失败后的重新生成能力。规范允许在工具块之前输出简短必要说明，块内及块后
仍不得夹带解释或编造工具结果。

未闭合引文中的普通可见前缀可以增量预览。原始引文仍完整留在筛分器中，工具标签、JSON、
代码及控制标识的歧义部分不预览；清洗层确认上下文后按已预览的可见前缀长度只扣除一次，
避免重复显示，也不提前放行嵌套在待确认 HTML/JSON/Markdown 单元中的引文。工具捕获进度
只在整个 wrapper 可能结束时调度完整解析，内部 parameter/invoke 的闭合不再反复重扫批次。

Vercel Node 的 prepare/PoW/换号请求继承客户端取消信号，并使用同一请求的剩余时间；
部署预算取配置总时长与 vercel.json 函数上限的较小值，预留最多 20 秒给最终校验和清理。
校验返回完整合法 usage 时直接采用 Go 的计数，包括零值；缺少权威数据时才本地估算。
租约保存 Go 收到 prepare 的原始时间，release 记录该时间开始的耗时、原始错误状态和错误码；
内部结构化日志记录首段正文、工具识别、上游结束及累计校验时间，不新增公开协议事件。

四种 Go 流式适配器使用共享 streamwriter：每个 SSE 帧单次写入，检查短写、Flush 和序列化
错误，支持时设置并清理写入期限；输出失败后停止消费，不对失效连接继续修复或重新生成。
API 测试页在后续失败时保留已显示正文，并分别展示错误；按 index 累积工具调用的名称、ID
和参数片段，只展示调用，不执行工具。

OpenAI Chat/Responses 的 `parallel_tool_calls: false` 会转换为共享单调用约束：
提示词要求每轮最多一个调用，收尾也检查数量；不合规时返回错误，不静默截掉其余调用。
提供正式工具目录时，auto 模式也检查工具名，名称区分大小写；没有正式目录的 system 文本工具仍保留兼容检测。JSON Schema 在共享层编译和校验（含本地 $ref、required、enum、额外字段和嵌套结构），不符合参数约束返回 invalid_tool_arguments。外部 $ref 不读取网络或文件。受约束调用在完整校验前不会交付。
参数解析不再静默丢弃未闭合或未命名的兄弟 parameter 节点，即使它们不是 schema 必填项；
invoke 内的 JSON 兼容包必须使用对象类型的 input 或 parameters，不能把错误类型或未知包结构变成空参数。
兼容包只允许一个 input 或 parameters 字段；同时提供两个参数包或夹带其他字段时拒绝解析，
避免静默丢掉模型生成的参数。空包 `{}` 继续表示无参数调用。Go 和 Node 使用相同规则。
旧 canonical XML 的未解析具名 invoke 与 EPSE 残片一样进入修复和校验，
不能因同一批次已解析出其他调用就将其忽略；代码示例和孤立闭标签继续排除。
完整 JSON 字符串内的工具标签作为字面值保留，不参与标签匹配和格式归一化。
合法 JSON 参数优先解析，保留换行、反斜杠和实体字面值；不能根据 path/file 等参数名
把合法 JSON 的控制字符猜测成 Windows 路径并改写。非 CDATA 的 XML 参数只解码一层实体，
`&amp;amp;` 保留为 `&amp;`；CDATA 中的 JSON 数组及兼容的逗号分隔对象也不解码实体。
提示词要求对象和数组直接使用完整 JSON 字面量，避免把数组末尾的 `]` 与 CDATA 的 `]]>` 混用。
共享 schema 归一化在字符串不符合 schema、且 schema 没有声明字符串分支时，可恢复数组末尾缺失的
容器闭合符：原值必须以 `[` 开始、以完整对象或子数组结束，按已有嵌套栈追加唯一的闭合序列，
例如提问数组的末尾选项对象之后缺少 `]}]`。扫描保留字符串和转义上下文，嵌套最多 128 层，
括号错配、未闭合字符串、缺值、尾逗号或其他损坏均不修复。补齐后必须是完整 JSON，
并通过该字段的全部 schema 约束。不会补写字段或参数值，也不会删除、重排已有内容；
字符串字段原文保留，无法确定恢复的值继续返回 `invalid_tool_arguments`。Go 各协议和 Node 的
Go 最终校验入口共用此规则，不需要额外 LLM 格式修复请求。
Go/Node 对标准 XML 实体及十进制、十六进制数字实体执行相同的单次解码规则。
LLM 格式修复在占位符还原前检查工具名、参数名及节点顺序，并绑定原始参数值与占位符所属参数；
修复结果还必须全部解析成功。修复不能通过改路径、删参数、增加调用或只保留部分调用来“成功”。
若工具调用存在残余坏码、参数不符合 schema 或违反工具策略，且收尾无法安全修复，
不交付残缺调用或作为正常正文释放。共享 completion runtime 会使用独立的工具纠正提示词，在同一账号补偿生成一次；
仍然失败时，托管账号可使用原始请求在另一个匹配账号的新会话重新生成一次，最终耗尽恢复机会后返回 HTTP 422 / 对应流式错误帧。
生成重试与格式修复分别计数，结束时复用已得到的错误，不重复发起同一轮格式修复。
纠正生成会收到本次校验的错误码、具体原因（如参数路径、缺参或类型不匹配）及原始错误输出，
以 JSON 数据附加。错误消息最多 512 个 Unicode 字符（截断时追加省略号）；`invalid_output`
最多 2048 个 Unicode 字符，较长输出保留首尾并设置 `invalid_output_truncated=true`。
纠正提示明确上次调用未交付执行，反馈是待纠正的数据，应遵循当前工具格式重新生成，调用工具时最终正文仅含完整工具块；
该约束仅作为工具失败后的纠正重试附加指令，不改变首次调用允许前置说明的规范。
不代替模型猜测参数，也不放宽 schema 校验。流式和非流式补偿使用同一反馈构造规则，
正文为空时可从内部工具检测思考取得原始错误片段。
重试用量逐次累计实际发送的 follow-up prompt，包含本次诊断，避免多次重试递归重复计入旧上下文。
流式补偿仅允许在未交付非空白正文或工具调用时进行。工具目录、名称、历史调用和 auto/none 策略
不再触发整轮正文暂存；普通说明和工具执行后的最终回答经过增量筛分后实时交付。
候选工具块及其后的文字继续隔离，完整批次必须通过共享格式、参数、名称和策略校验后才能交付执行。
仅 required/forced 模式暂存整轮正文：缺少所要求的调用也是策略失败，保留纠正生成的机会。
已经交付正文后若后续调用失败，返回对应错误并保留此前正文，不静默撤回、替换或重放整轮。
Go 与 Node 保持一致；Node 的 validate_tools 只要求共享校验，不代表正文必须暂存。
每次补偿清除未交付的正文、工具片段、工具检测思考、上游消息 ID 及错误状态，已流出的可见思考保留。
流式交付判断只认实际交付的非空白正文或结构化工具调用；前导空白暂存到有效正文出现才一起发送，
失败轮的空白在重试时清空，不提前关闭思考或创建文本块。Chat、Responses、Claude、Gemini 和 Vercel Node 使用相同边界，
已经交付实际正文或工具调用时仍结束并报错，防止重复执行。
Claude、Chat、Responses 和 Gemini 的非流式调用复用共享错误分类；Claude、Chat 和 Responses 的 Go 流式路径与 Vercel Node 路径保持同一恢复边界。
临时服务故障可重试或换号，审核、超长输入、无效输入请求和客户端取消不会被当作生成失败反复提交。
换号与首次取号共用有容量限制的等待队列，单次换号等待最多 30 秒，且服从请求取消；唤醒按账号和工具池匹配。

Go 流式消费每次尝试拥有独立的读取生命周期。主动截断、超时或终止会取消该次后台读取并关闭上游正文，
收尾及下一次补偿继续使用调用方的有效上下文，避免旧读取协程滞留，也避免连带取消新的尝试。
只有响应级 `status` / `quasi_status=FINISHED` 或 `[DONE]` 才表示完整结束；没有结束标记的 EOF
作为断流处理，片段级 FINISHED 仍只结束对应片段。最后一批数据同时包含正文和结束状态时保留末段正文。
Chat、Responses、Claude、Gemini 的 Go 流式路径在已有部分输出及消息 ID 时通过共享 runtime
继续同一个上游消息，保留累积内容和正文/思考通道；没有可续答 ID 或恢复失败时报告中断，不能另起生成冒充续答。
Claude 流式保留内容过滤状态，不把审核空输出当成服务临时故障重试或换号。
非流式单次收集默认空闲 300 秒、总耗时 1800 秒；收到完成状态后最多再等待 250 毫秒收集引用。
时间边界或调用方取消会关闭读取及自动续答背后的上游正文；已完成的答案不因引用收集超时报错。
三个时间参数的环境变量见部署指南，工具格式修复收集同时服从其既有的修复请求上下文。

托管账号登录/刷新按账号合并并发调用，迟到的旧请求复用已刷新凭据，不清掉新 token。
等待者取消不终止仍有效的登录；登录发起者取消时，其他有效请求可以重新发起，避免共享取消结果。

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
- Claude/Gemini 优先使用显式 ID。Gemini 调用和结果对 `id`、`call_id`、`callId`、`tool_call_id` 使用相同优先级，显式乱序结果不能退回同名 FIFO 配对。兼容缺少 ID 的同名结果时，按未返回调用的顺序配对；已匹配项移出队列，合成 ID 避开本请求中已有的 ID。
- Chat、Responses、Claude、Gemini 在标准化后共同校验工具历史：拒绝重复的未完成调用 ID、已知 ID 与结果名称冲突以及同一已知调用重复返回结果。显式乱序结果和缺失原始调用的部分历史继续兼容；合成 ID 预留整个请求中已有的显式 ID。
- 缺少 ID 且结果乱序时不能可靠还原配对关系，调用端应传回原 ID。
- Responses 的结果错误标志和显式 null 会保留，避免下一轮把失败当成成功或把 null 当成空字符串。
- Node 与 Go 一样，仅在正文为空时尝试从推理通道恢复工具调用；默认只接受纯工具格式，混有计划、否定说明、注释或代码示例的思考不产生可执行调用。服务端 `tool_safety.allow_reasoning_prose` 可显式恢复旧版兼容行为，客户端请求不能覆盖该设置。关闭推理展示时，内部检测不产生 reasoning_content 输出。

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

工具块与 invoke 内的结构必须完整消费：子节点之间只允许空白和完整注释。
未闭合 invoke、写错的调用标签、`<param name="...">` 等未知参数节点和额外正文不能因已有
成功调用或 schema 将字段设为可选而静默消失。残余中有具名调用时，正式工具与无目录续聊
都按同一规则修复；无法安全恢复时返回 `invalid_tool_arguments`，不交付部分批次。

部分成功时，格式修复接收去除 Markdown 示例后的完整原始批次，包括已解析的调用。
CDATA 值继续用占位符保护；完整性检查绑定全部调用、参数、值及原始顺序。
成功后整批替换，不能按 name+input 去重，也不能把修好的前置调用追加到批次末尾。
LLM 修复输出重新解析后还必须通过协议残余校验；即使有名调用全部恢复，额外的未闭合
EPSE、canonical 或已确认的未知协议外壳也会使整次修复失败。
完整 DSML 可与完整 canonical 调用交错恢复，保持源顺序；任何未恢复的具名兄弟调用使本地恢复失败。
有歧义的无围栏嵌套 CDATA 若会留下孤立 `]]>`，拒绝格式修复并交给生成纠正，避免接受被截断的值。

Go/Node 流式层将传输外壳识别与可执行语法解析分开：除了已知 EPSE/DSML 前缀，
`<|任意标识|本地名 ...>` 也会作为显示隔离候选，本地名不必已被 parser 支持。
因此调用者标识和标签名同时写错时，也不能在小分片中先泄漏、在整段解析时才被拒绝。
尚未分类的标签名前缀不会因超过 128 UTF-8 字节而转为正文；128 字节只用于将长前缀
移入捕获区、复用增量进度调度，不是放行上限。普通 HTML/特殊 token 在边界明确后原样放行。
只有半截未知 token（如 `<｜System`）而没有工具外壳证据时，EOF 按普通字面量保留；
完整的未知外壳仍须最终校验，不能把该规则用于释放已经确认的坏调用。
未知外壳中仍能提取具名调用时，接入已有格式修复流程；工具/参数身份、数量、顺序和参数值
必须通过来源完整性检查。候选识别本身不增加可执行语法，也不会猜测工具名称。
坏的 canonical 前缀与 EPSE 使用相同的 deferred 区域。Node 的所有候选工具调用均等到
Go 收尾校验后再交付，包括没有正式工具目录、仅从历史继续调用的请求；内部校验请求携带原始输出，
不能使用 JavaScript 已解析的数值作为参数来源，确保 `9007199254740993` 等大整数不丢精度。
Chat、Responses、Claude 的 Go 流式路径也将完整批次保留到共享收尾，包含无正式目录的兼容调用；
纯正文仍可实时流出。已检测到工具调用后丢弃同轮的尾随正文，失败重生成清除待交付调用状态。

可见输出清理由 Go `textclean.StreamCleaner` / Node `VisibleCleaner` 按通道保持状态，
生产工具筛分器在发送文本事件前统一调用；适配器不再对已分类的单个 delta 重跑无状态格式删除。
普通正文和围栏代码实时输出；跨片段的标签、行内代码、引号字符串、CDATA、注释和 JSON
待边界确定后分类。代码示例（含空 JSON 围栏）保留原文，不会因切片方式改变清理结果。
历史 wire 工具数组/结果仅在完整 JSON 结构成立时隐藏，字段顺序、嵌套和转义字符串由 JSON
解析处理，不通过正则截取。原始正文、原始思考和工具参数继续独立保存，不经过显示清理。
`think` 和 agent 显示外壳按原文坐标与同名嵌套深度寻找结束位置，跳过引号、代码、注释及
CDATA 中的同名结束标签；不能对整段 Unicode 文本转小写后复用偏移。未闭合 `think` 继续隐藏。
超过 4 KiB 的待定显示单元也采用增量边界扫描与分段缓冲；JSON 记录括号深度、引号和转义状态，
避免小分片反复复制、解析累计前缀。边界扫描仅调度完整解析，EOF 仍处理全部缓冲内容。
重新生成重建筛分与清理状态；续答沿用同一状态。正文和思考均覆盖该规则。
工具筛分层对未闭合的双引号字符串、注释和普通 CDATA 同样增量检查新增分片；
转义状态和跨分片结束标记保留，可能闭合、非法控制字符或 EOF 才重新运行完整解析。
普通 CDATA 按自身第一个结束标记闭合，不能借用后续工具参数的结束标记吞掉真实调用。
只有紧接参数开始标签的 CDATA 值沿用嵌套代码示例的容错边界，保持已有参数保真行为。

长工具块超过 4 KiB 后，以增量扫描的新闭合结构决定何时再次运行完整解析器；普通参数内容
不再触发每片重扫。该调度只提供解析时机，CDATA、Markdown 和最终执行判定仍由完整解析器负责。
Node 直接扫描新增片段并限制调度器的临时标签缓冲，避免访问累计字符串而反复展开整条字符串。
Claude 的原始工具片段和隐藏工具检测思考都计入上游活动，持续规划工具不会误触无内容超时。


Go 和 Node 均使用结构化扫描，容忍受支持的分隔符漂移、大小写、引号和部分缺失标签。
旧 canonical XML 保留兼容解析，但提示词和历史只示范带标记的格式。
Markdown 代码围栏/行内代码里的工具示例不会执行。生产路径隔离无法解释的工具块；
完整 EPSE 标识后的未知标签名、未闭合外壳和截断 canonical 调用即使尚无工具名，
也不能作为成功正文释放。共享收尾无法安全恢复时返回 `invalid_tool_arguments`，无工具目录续轮同样适用。
已解析调用旁的截断兄弟块也纳入残余校验，禁止静默交付部分批次。孤立闭合标签继续仅清理，不触发调用。
流式 Markdown 状态必须消费纯空白分片：独立的换行、CRLF、空格和缩进都会影响后续围栏是否位于行首。
不能因为分片没有可见字符就跳过状态更新，否则可能漏掉真正的开始围栏、把结束围栏反判成开始，
进而将后续真实调用作为代码示例透传，或将代码示例反向识别为可执行调用。Go/Node 使用相同规则。
2026-10-31 的新导出样本确认，某轮 8 个调用既进入了结构化 tool_calls，也完整出现在下一轮的
assistant 正文中；完整文本回放可以解析全部调用，问题出在流式围栏状态，并非参数 schema 或搜索提示词。
回归覆盖单独空白分片、反引号/波浪线围栏、示例保留、8 调用流式交付及 CDATA 参数字节保真。
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

流式识别出的工具块在完整批次收尾校验后输出协议原生工具增量，避免原始标签泄漏到正文；收尾阶段还会检测
仅思维链中存在的工具调用，并按客户端 thinking 开关决定是否显示清理后的思维链。
所有协议及 Node 镜像的可见思考均经过独立的有状态工具可见性 guard，
完整调用、DSML、未知 EPSE 标签和截断调用不会以 reasoning/thinking 增量泄漏标签或参数；
原始思考仍保留给最终检测、格式修复与诊断，不在思考展示过滤中执行工具。
已确认的反引号/波浪线围栏、行内代码、JSON 字符串、CDATA 和注释示例保留，
未完成的展示字面量等待分片补全后分类。正文生产筛分也保留这段字面量上下文，
避免逐字符到达的 JSON/CDATA 示例被误识别为调用、继而丢失示例正文。
续答沿用 guard，重新生成清除旧 guard 的暂存内容。
前缀拦截使用与解析器相同的大小写/全角字符规则，不依赖单一 ASCII `<|EPSE` 子串；
全角 DSML 起始符的截断尾巴也会清理。
隐藏通道中的完整 DSML 或坏工具语法同样进入共享恢复；正文已有工具意图时，不能使用思维链中的另一组调用掩盖其失败。
当存在残余工具意图时，收尾先执行本地恢复，再决定是否发起一次 LLM 格式修复。
收尾先尝试保守的本地 DSML 恢复：仅接受完整的 `calls/invoke/parameter` 层级，
每个参数必须明确声明 `string="true"` 或 `string="false"`。前者逐字保留原始内容，
后者只接受合法 JSON 字面量并保留大整数精度；未知属性、重复参数、截断及无法完整解释的混合残余意图均不猜测。
本地恢复仍经过相同的工具选择和参数 schema 校验。无法本地恢复时才进入已有 LLM 修复；
常规 parser/sieve 对 DSML 的识别与隐藏规则不变，Node 的全部候选工具调用复用 Go 收尾校验端点。
LLM 修复默认跟随本次标准请求的实际 `ResolvedModel`，复用同一账号、新建会话、开启思考。管理面板可改用自定义 OpenAI、Claude 或 Gemini API；自定义请求独立发送格式修复提示词，不继承主对话历史、文件或上游鉴权。默认十秒，可配置 1–300 秒，始终继承客户端取消和更短的期限。会话删除进入固定容量队列（128 项、2 个 worker），使用凭据快照，独立十秒期限，不阻塞工具交付；队列满或删除失败写日志。
原生修复响应按思考模式分离通道，自定义 API 分离 `content` 与 `reasoning_content`，思考内容单独记入管理端响应归档；仅最终正文进入格式解析与保真校验，思考中的示例或候选调用不能作为修复结果。修复请求仍只携带格式说明、约束和错误工具批次，不附带主对话上下文。
参数通过 DS_SLOT 占位保护；修复失败或占位被破坏时保留内部原始残片供诊断，
向客户端返回格式错误，不输出半修复内容或原始工具代码。
已流式发出的结构化调用不会回滚并重修。

疑似坏工具块缓存在独立区域，完成解析/修复后才决定是否作为正文释放，避免先发送坏代码再发送修复后的调用。正文截断规则及流式解析在 Go 和 Node 保持一致，包括闭标签复读和虚构 `[Tool]:` 角色；CDATA、引号字符串、Markdown 代码中的同名文本保留；正文长参数沿用增量扫描调度。
详见 [toolcall-semantics.md](toolcall-semantics.md)。

续答快照的去重在 Go（字符串与 Builder 路径）和 Node 中采用同一规则：
只有明确标记的 SET 更新或带 fragments 的重放快照才进入去重；普通 APPEND / 增量数据无论多长都按原文追加，两次相同的工具调用保留为两个独立出现。同一上游流按 fragment ID、索引及补丁路径保留各字段原文，嵌套 BATCH 中的 SET 先转换成新后缀，再进入思考识别与显示筛分；修改已交付字段的既有前缀会终止响应。首次重放包中相邻同通道片段合并后执行跨传输轮的前缀比较，避免只去掉第一个片段。显式快照不受 32 字符门槛限制；无来源标记的旧文本启发式助手仍保留该门槛。
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
文件预处理失败（内联附件上传、引用查询、HISTORY/TOOLS 生成）也写入响应记录，
保留当前输入、账号、失败阶段、HTTP 状态、底层错误及准备耗时；不计为模型输出或成功用量。
Vercel 内部 prepare 在上传阶段失败时直接记录，无需等待不会发生的 release；
成功 prepare 仍由 release 归档，避免重复记录。直接 token 鉴权失败仍不留响应记录。
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
- 校验与拆分共用按出现顺序建立的调用实例依赖；已完成 ID 再次使用时，后面的同 ID 调用不会覆盖较早结果的归属。
- 更早的消息上传 HISTORY.txt；此前的 system/developer 指令按原顺序保留在文件中，并在主提示词中重复固定，避免只在附件中生效。
- 只有首个任务且已进入 assistant/tool 循环时，不为拆分强行把当前工具链搬进附件。
- 单条长首轮 user 仍可外置到 HISTORY.txt，主提示词明确其位置；有前置指令且首轮 user 正文合计超过 32768 个 Unicode 字符时，同样外置完整首轮转录并保留实时指令，较短首轮兼容外置指令同时保留最新 user 和指令的行为。
- 文件按「本次生效的思考强度系统提示词（若启用）→ `# HISTORY.txt` 与先前历史说明 → 历史消息」排列，编号角色标题和独立消息边界与 live prompt 相同。
- 工具 schema 单独上传 TOOLS.txt，主提示词明确命名两个文件、它们的用途、当前任务位置和调用结果配对规则。
- 图片和文件在原消息位置保留附件引用；不能用空正文判断并删除工具调用、空工具结果或图片轮次。
- 生成文件按当前账号上传；账号切换后按原字节重传，不再次改写正文。usage 按文件和主提示词完整上下文计算。

历史拆分用于绕开输入框输入限制，由开关与字符阈值控制，不以 token 节省、压缩率或
拆分后实时提示词是否缩短作为触发条件，也不自动总结、裁剪或重写客户端正文。
客户端附件和生成的 HISTORY/TOOLS 共用每请求 50 个文件引用上限；上传前检查组合数量，
超限返回 HTTP 400，不丢弃附件、不上传一半后才发现超限。上传返回空结果或空 ID 也明确报错。

管理台对当前带结束标记的历史使用对应转录解析器，按本文件声明的消息结束标记切分，
保留 developer 角色、结果元数据、空消息和正文空白。正文中的角色标签或编号标题不会
产生新消息。旧历史继续使用旧格式解析；当前格式损坏时不退回宽松的裸角色扫描。
合并历史时只去掉已知完整的服务端续聊提示，用户自行输入的相似文本保留。
拆分后的提示词通过 StandardRequest 重建，沿用入口的工具目录来源约束；Claude
禁止从 system 文本推断工具的规则不会因历史文件续聊而重新开启。

思考注入仍由现有开关控制，仅在本次请求启用 thinking 时生效。它作为独立的生成系统前缀
放在最终提示词的最开头，`Reasoning Effort: ...` 紧跟 `<｜System｜>` 标签，先于搜索说明、
转录规则、工具说明、调用者系统指令及所有用户消息。自定义思考提示词也使用同一位置。

生成前缀只属于输入。若上游正文从本次实际发送的思考系统前缀，或会话转录元数据的引言重新开始，
Go 与 Vercel Node 会在正文首次交付前缓冲并识别这一回显，阻止它进入正文和成功历史。
检测仅针对回答开头且绑定本次提示词；代码块、引号、回答中的后续引用和不同的普通文本保持原文。
未确认的短前缀在 EOF 释放，避免正常短回答被吞掉。该规则不移动 thinking/response 通道，
也不改变 HISTORY.txt 的切分位置或客户端消息内容。
命中后使用 `upstream_prompt_echo` 分类并按共享重试预算重新生成，向模型明确反馈不要复述输入、
继续最新真实任务；重试耗尽返回 502，不将它误报为正常 stop 或账号限流，也不从该次思考规划中执行工具。
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
multipart 文件名统一按 `/` 移除路径前缀，反斜杠与引号作为文件名字符转义保留；
线上字段不使用宿主系统的路径解析规则，避免部署到 Windows 后改变原文件名。
内联文件及显式文件引用的请求固定当前账号，防止重试中混用跨账号文件 ID。
通过 `/v1/files` 单独上传后，多账号环境需用返回的 account_id 固定后续请求的目标账号。
PNG/JPEG 按显式或远程压缩配置处理；没有有效比例时保留原文件，文档内容保持原样。
同一请求的内联文件先上传，再合并轮询，所有文件成功后才用于聊天。
OpenAI Chat/Responses 剥离已上传的图片块时保留带 file_id 的文本附件引用及用户轮次；纯图片首轮不会
因 messages 变空而返回 400，纯图片追问也不会退回上一条 assistant 的轮次。
Responses 支持图片 content part 数组、message 数组和单对象 input；引用先汇总到 RefFileIDs。

上游 Client 的共享上传层对内联文件、`/v1/files` 和 HISTORY/TOOLS 使用同一套进程内复用缓存。
上游 HTTP 200 中的 `code/biz_code=40029`、`TOO_MANY_REQUESTS` 与 HTTP 429
统一识别为文件限流。没有托管号池（例如直传 token）时，上传在原账号按 1、2、4…秒（最高 16 秒）退避，
存在 `Retry-After` 时取较长等待时间，耗尽本次上传重试次数后向调用方返回 HTTP 429。
文件就绪查询遇到该业务限流也可退避重试，仍受原有总等待时限约束。
没有托管号池时，复用缓存命中后的校验、显式 file_id 查询也在共享元数据入口最多尝试 3 次，
遵守 `Retry-After` 并保留文件所属账号与原缓存，不因查询限流重传文件。
就绪轮询只调用单轮查询，继续使用自身的轮询预算，避免两层重试相乘。
等待支持取消，不把限流当作 token 失效、文件不存在或允许盲目重传的证据。
托管号池识别限流后直接暂停该账号 120 秒，交由保留原始输入的边界换号重建；具体规则见文末。
这些策略不保证避开上游的账号或时间窗口配额。
键包含调用者、上游账号/凭证、安装身份、原文件 SHA-256、文件名/MIME/purpose、思考与压缩配置。
同一作用域的重复请求可复用 file_id；不同账号不混用，原有显式账号绑定不变。
缓存不改变号池路由；请求被分到另一个账号时，需要在该账号独立上传。
哈希发生在图片转码及随机命名之前，命中时免去重新压缩、上传 PoW 和 multipart 传输。
默认缓存 900 秒、最多 256 项；`DS2API_UPLOAD_CACHE_TTL_SECONDS` 和
`DS2API_UPLOAD_CACHE_MAX_ENTRIES` 可覆盖，任一为 0 时禁用；上限分别为 86400 秒和 4096 项。
缓存有效期不因命中而延长，重启后清空，不保存文件正文、原始响应或签名下载地址。

文件用量在共享文件预处理层解析：上传响应与 fetch_files 的 `token_usage` 优先用于
`RefFileTokens`，包括明确的 0；null、缺失、负数及非整数不作为真实计数。
结果和缓存复制保留独立的 token 元数据，不依赖 Raw，也不与调用方共享可修改指针。
内联上传完成后统一等待就绪；显式 file_id 按当前账号批量获取元数据并等待处理完成，
不重新上传、不计算上传 PoW。每个唯一 ID 每次请求只计入一次，包括历史消息中的重复引用。
未知用量时仅对该文件按原始内联字节数（或查询到的文件大小）/3 估算，不叠加真实值。
文件引用与内联文件共享每次请求最多 50 个唯一 ID 的限制；内联块数仍沿用原有 50 项限制。
文件缺失或批量查询只返回部分文件时返回 404；处理失败返回 422；鉴权失败保留原有语义。
文件查询保留上游 HTTP 429/5xx；等待超时映射为 504，损坏或超限的上游元数据映射为 502。
请求已明确携带超过 50 个唯一引用时，在上传任何内联文件之前拒绝，避免无效请求消耗上传资源。
内联文件与 count_tokens 共用增量 SHA-256，直接读取原字节，不再为哈希复制整份文件；
原有 MIME、文件名分隔方式与缓存键保持一致。
文件查询遇到托管凭证失效时仅刷新原账号一次，不换号。JSON 不能伪造共享层的已解析用量。
OpenAI Chat/Responses、Claude、Gemini 的标准请求、流式及非流式 usage 使用同一文件计数。
Claude count_tokens 对已有 ID 查询真实元数据，对内联内容只本地估算，不实际上传文件。
文本和思考输出仍按本地 tokenizer/估算口径统计；SSE accumulated_token_usage 是会话累计值，
不直接作为本轮输出，也不因文件 ID 免重传而省略文件输入用量。

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
搜索仅通过 search_enabled 传给上游，不再额外注入搜索提示词。

### 搜索模型的 App 原生能力

原生搜索按别名解析后的 `StandardRequest.Search` 开关生效，包括 `deepseek-v4.1-flash-search`、
`deepseek-v4.1-flash-search-nothinking` 和映射到它们的别名；普通模型保持关闭。
此前移植及精简过的内置搜索提示词已删除，包括各协议入口、提示词重建、历史文件续写和 Vercel prepare 路径。

原生搜索和网页读取由上游 App 在推理过程中内部执行并回填结果，
不需要模型在回复正文输出客户端工具调用。SSE 中的 `TOOL_SEARCH`、`TOOL_OPEN`
进度归入思考通道，最终答案归入正文；关闭思考展示不会将搜索进度改为正文或关闭原生搜索。
服务端不再手写原生调用接口、参数 schema、`||` 查询示例、轮次编号协议或搜索能力提醒。
调用者自己的 system/developer/user 指令保持原文，不通过字符串过滤删除其中的搜索要求。

OpenAI Chat、Responses、Claude、Gemini 和 Vercel Node 共用模型能力开关；搜索开关不改变提示词内容。
思考注入仍由独立配置控制，通过 `WithThinkingInstruction` 复用于 live prompt 和上传历史，
保留前置位置、幂等性及 PromptTokenText 统计，移除搜索注入不会移除已配置的思考注入。

内置 search/open 由上游原生执行，不加入客户端 ToolsRaw / ToolNames，也不改写原始 Messages 或号池路由。
客户端即使声明同名工具，其 schema、工具选择策略和调用格式仍只约束该客户端工具。
历史外置时，TOOLS.txt 的权威目录与允许列表明确只约束客户端工具，不覆盖上游原生能力。
EPSE-only 与 DSML 禁用规则继续限定在客户端工具范围，生成说明中的协议名称沿用调用者标识，
不改写用户正文。`force_disable_tools` 控制客户端工具，内置搜索仍由模型能力开关决定。

2026-10-31 的导出记录中，两次 `web_search.queries` 被生成为 `||` 拼接字符串，
但客户端 schema 要求字符串数组；另一次将上游 `search` 输出成了客户端调用，触发工具允许列表校验。
三次失败的正文都以两个换行开头。离线流式回归复现了这些换行被误计为已交付正文、
导致直接输出 422 错误帧并跳过纠正重试的问题；修复后仅输出空白的失败尝试也可进入原有有界补偿流程。
面板 `content` 保存上游原始标签，成功记录需结合 `finish_reason` 和下一轮回填的结构化调用判断，
不能仅凭原始标签认定发生正文透传。离线验证不代表已完成真实上游原生搜索联调。

实现及回归：`internal/config/models.go`、`internal/config/model_alias_test.go`，
API 测试页面的默认模型和说明同步更新。

## 验证

运行 Go 全量测试、Node 工具解析/流式兼容测试、lint、重构行数检查和 WebUI 构建。
核心回归覆盖格式规则、跨分片标记、CDATA 修复、历史最后一条 user 保留、
重建时格式开关保真、配置部分更新及 OpenAI/Responses/Claude/Gemini 的协议输出。

### 账号与用量功能对齐

共享提示词、工具历史、拆分和文件注入继续在标准请求路径中处理。各协议的历史完成回调通过共享请求记账器写入独立用量账本；关闭聊天记录不会关闭用量统计。账本按日期、实际模型、调用方和最终处理账号聚合，重试切号后使用终态账号并保证一次请求只记一次；账号只保留哈希和脱敏标签。旧账本保持未知账号归属，已有账本不重复补账。流式取消通过共享用量估算器读取当前归一化请求的完整提示词、重试上下文与引用文件计数；新版 Vercel 桥在现有 release 请求中由 Go 共享计算器结算成功及取消用量，保留原始 HISTORY/TOOLS、引用附件和实际重试提示词；旧桥继续保留 Node 已提供的取消用量。导入/清空/首次补账写盘失败不会发布内存变更。有效调用次数、Token 和估算费用口径及前端联动筛选见功能对齐说明。后台设置改由注册表统一读写和校验，原有注入提示词、工具开关及阈值仍通过共享配置访问。新增账号调度、保活和语音功能的范围见 [功能对齐说明](function-alignment.md)。

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

续接上下文同时保存顶层 file_ids、ref_file_ids 和 attachments；下一轮合并新旧文件输入，
ID 去重、列表保持平坦，避免多轮累积出嵌套数组。附件在内联预处理改写前保存原始输入，
使 Base64 附件仍可遵守原有号池上传规则；显式文件 ID 仍要求绑定所属账号。
这些文件输入与消息在同一不可变 JSON 快照中保存、恢复，并计入 Responses 缓存容量，
遵守相同调用者隔离与过期规则。store:false 跳过用于持久化的文件和消息深拷贝。

状态存储是进程内缓存，TTL 使用 responses store 配置（默认 15 分钟）；`responses.max_entries` 默认 256 项、`max_bytes_mib` 默认 128 MiB，`max_caller_entries` 默认每调用者 128 项。调用者先在自身配额内淘汰旧条目，再执行全局容量淘汰；超大替换不能删除原有有效条目。流式与非流式使用同一保存回调；写入超出容量或无法编码时返回 `response_storage_failed`（非流式 HTTP 507，流式 `response.failed`），不会以 `response.completed` 声称保存成功，也不会在保存失败之前发送待交付工具调用。上述容量在后台设置中可调整，配置更新保留已有状态和有效期；服务重启、多实例未共享状态或缓存淘汰后，请携带完整历史。写入和读取深拷贝嵌套数据，克隆工作不持有全局读取锁。未支持的 Conversations API 明确拒绝。

标准 `reasoning` items 的 content/summary 作为 assistant reasoning 恢复，不再降级为 user 文本；不解密 encrypted_content。custom_tool_call 与 custom_tool_call_output 保留 assistant/tool 角色和 call_id。`type:custom` 的自由文本在内部工具协议中使用唯一字符串参数 input，输出还原为 custom_tool_call/input 和 custom_tool_call_input 的 delta/done 事件。format/grammar 描述进入提示词；不宣称原生 CFG 约束解码。当前服务器不执行 OpenAI 托管工具。

Vercel prepare 声明 validate_tools 能力时，Node 将原始候选文本交给带租约校验的 Go 内部端点，复用同一参数校验、修复和工具策略，再发送 arguments 字符串；JavaScript 不重新解析该字符串中的大整数。校验端点不可用时返回错误，不交付未经校验的调用。

工具参数在输入解码、历史渲染、输出解析和 Responses 回译中保持数字值；不能被 float64 无损重新编码的数值保留为 json.Number。CDATA 字符串不作数字推断。Claude 工具 ID 使用随机 UUID，避免同秒多轮重名；Gemini functionCall 同样返回 ID，供原样回填。

## 共享实现的归属

HISTORY.txt / TOOLS.txt 处理统一位于 `internal/contextfile`，供各协议和 `completionruntime` 直接调用。空输出 / 工具输出重试提示词与用量累计辅助位于 `internal/completionruntime/empty_retry.go`；输出清洗和流式累积分别位于 `textclean`、`assistantturn`。这些迁移不改变提示词、上传顺序或重试规则，避免让某个 HTTP 协议目录成为共享业务的所有者。

## 输出侧 SSE 元数据边界

Go `sse/paths.go` 与 Node `sse_paths.js` 对齐片段状态的绝对、response 相对及集合相对路径；嵌套 patch 在提取正文之前执行相同过滤，防止 `WIP` / `FINISHED` 或累计 token 字段混入可见答案。`token_usage` 在共享常量中声明为元数据，上游累计用量仍不替代内部估算。此变更不调整输入提示词、工具规则或历史转录。

## Node 租约与断流收尾

Node prepare 后持有的账号租约不改变输入归一化 / 历史文件语义；但必须在 prepare 异常或客户端断开时释放。401 内部恢复沿用 Go 的共享凭据恢复步骤，`auth_failed` 不得在控制请求编码时丢失。续答失败的部分输出先冲刷有效片段，再发送错误、`[DONE]` 和 HTTP 结束信号，避免连接悬挂或错误帧后的迟到正文。
续答返回失败状态或耗尽续答次数时，同样以中断结束，不能把部分内容标成正常 stop。
续答判断依据原始正文、原始思考和隐藏工具检测内容，包含带片段的快照包，不能依赖已显示的文本。
读流异常与 EOF 走相同的续接分支，先释放旧 reader；仅生命周期取消才表示客户端断开，
上游独立的 AbortError 仍是传输失败。续接请求异常、失败状态和无终态的空续接均不能触发工具校验交付；
空续接保留同一回答已有内容的状态，继续尝试至配置上限，再返回 upstream_interrupted。

## Gemini 无目录续轮的流式输出

原生 Gemini 流式路径对无正式工具目录的 agent 续轮也启用共享 toolstream 筛分，跨分片的调用者标记不会作为正文泄漏。工具批次统一在 assistantturn 收尾校验后转换为 functionCall，保留参数数字精度与调用顺序。普通正文继续实时输出；正文已经交付后不再重新生成，永久错误不触发空输出重试。替代生成重置筛分与待交付批次，继续生成保留已有累积。


## 工具输出完整性与计数

SSE 解码保留正文与隐藏思考的源文本。`<think>`、`CONTENT_FILTER`、引用标记及模型控制 token
若位于工具参数中，保持原值；不能先删除再把改写后的字符串交给 Schema 校验。
显示清理在工具筛分后执行，跨分片维护引号、CDATA、注释及 Markdown 上下文。
结构化状态字段决定上游内容过滤；文本中的普通 `CONTENT_FILTER` 提及保留，已知泄漏的
`CONTENT_FILTERblocked` / 中文拒绝后缀仅在非字面量显示通道隐藏。
旧版 `</think>` 恢复逻辑跨分片保留字面量上下文，参数中的同名标签不结束思考阶段。

明确的上游错误与内容过滤状态阻止整个待交付工具批次，不执行格式修复，也不因已解析出
合法兄弟调用而消除错误。Chat、Responses、Claude、Gemini 与 Node 使用同一终态规则。
无效的显式工具目录（名称缺失、结构错误、重名、名称冲突或不支持的类型）在入口返回 400；
无目录的历史兼容请求仍按原有工具策略处理。Gemini 的无效声明不会在适配时静默丢失。

参数类型归一化复用本地编译的 Schema 引用图，支持 `$ref`、`$defs`、递归、allOf 与数组项。
anyOf / oneOf 已有合法类型保持原值；只采用能通过完整 Schema 且结果唯一的转换，外部引用仍禁止。
长工具块的增量扫描跳过 CDATA / 引号 / 注释 / Markdown 内的字面量闭标签，避免写入协议文档时反复全文解析。

输出 Token 按生成源文本估算，包括工具名、参数及隐藏思考；同一思考同时出现在原始与检测
通道时合并计数，从思考恢复的调用不重复计费。Node 工具校验响应携带共享层的输出用量，
完成帧优先复用该结果。Node 校验和历史归档携带原始思考，客户端思考增量继续通过显示 guard。普通输出仍沿用各运行时的估算器，所有数值仍是估算，非上游账单。


## 第二轮工具与 Agent 边界修复

- 旧版 `</think>` 兼容恢复保留跨 SSE 帧的未完成标签和字面量状态，标签前缀最多暂存 256 字节；确认失败或 EOF 时作为思考原文释放。扫描仅检查当前有界候选，不再对每个 `<` 搜索整个剩余片段。
- 闭标签复读检测跳过 CDATA、引号、注释及 Markdown 字面量，参数中的重复 `</parameter>` 不再导致提前截断。真实结构性闭标签复读仍按原阈值终止。
- Schema 字符串归一化同时遍历 properties 和全部匹配的 patternProperties；只有两者均未覆盖字段时才使用 additionalProperties。多个约束仍由完整 Schema 最终验证，不能因转换绕过相互冲突的约束。
- 回归用例覆盖同值 APPEND、嵌套 SET、多片段快照、每个思考闭标签切分点、CDATA 参数保真、存储失败和 store:false、跨协议工具历史冲突。性能基准：`go test ./internal/sse -run '^$' -bench BenchmarkThinkingBoundaryMarkup -benchmem`。


## 工具安全与 Agent 续接补充（2026-11-01）

- Markdown 围栏中的工具代码是字面量示例，包括包含 CDATA 的单行/多行示例和未闭合围栏。只有围栏开始前已经处于真实工具参数内部时，才允许恢复被参数正文中的未闭合围栏遮住的结束标签。
- 文本参数中的 CDATA 保留原始字节。对任意参数名，Schema 中的 string（含本地 `$ref`、组合、patternProperties、additionalProperties）优先恢复 CDATA 原文，避免 JSON 外观的字符串丢失引号、缩进、空白，或把文本 `null` 变成空值。无类型目录中的旧式对象/数组参数兼容解析继续保留；custom 工具的 input 始终保留原字符串。
- 断流恢复和自动续答属于同一条 assistant 回答，必须保留正文防泄漏器的待定标记、CDATA、引号和 Markdown 状态。只有重新生成或更换账号后重放请求才清空状态。
- 正文与可见思考均对本次实际生成的前导说明执行增量 Prompt Echo 检测；共享收尾也校验隐藏思考，回显内容不能恢复为工具调用。正常思考、引用、代码示例和未确认的短前缀保持原行为。
- 省略 `tools` 时，默认继续兼容无正式目录的 Agent 续轮；显式 `tools: []` 表示没有可调用工具。后台“工具调用安全”的 `require_tool_catalog` 默认 false，开启后无正式目录的调用也会被拒绝。此设置由服务端控制，请求体不能覆盖。
- Go 和 Node 使用 `internal/streambudget/defaults.json` 中一致的输出与续答默认预算。超限返回明确错误，整批工具调用不作部分交付，也不会截断参数后继续执行。

可使用下列环境变量调整预算（正整数；无效值回退默认值）：

| 环境变量 | 默认值 | 含义 |
| --- | --- | --- |
| `DS2API_STREAM_MAX_LINE_BYTES` | 8388608 | 单行 SSE 最大字节数，含换行 |
| `DS2API_STREAM_MAX_OUTPUT_BYTES` | 33554432 | 上游读取/累积输出的字节预算；Node 跨续答和重试累计 |
| `DS2API_STREAM_MAX_REPAIR_BYTES` | 1048576 | 参数槽位替换后发送给修复模型的提示词大小 |
| `DS2API_STREAM_MAX_TOOL_CALLS` | 128 | 单个已校验工具批次的最大调用数 |
| `DS2API_STREAM_AUTO_CONTINUE_MAX_ROUNDS` | 8 | Go/Node 自动续答轮数上限 |
| `DS2API_STREAM_KEEPALIVE_MS` | 5000 | Node SSE 注释心跳间隔 |
| `DS2API_STREAM_IDLE_TIMEOUT_MS` | 300000 | Node 上游读取空闲超时 |
| `DS2API_STREAM_CONNECT_TIMEOUT_MS` | 30000 | Node 上游连接/响应头、PoW 控制请求及下游写入等待超时 |
| `DS2API_STREAM_MAX_DURATION_MS` | 1800000 | Node 上游流处理总时长预算，以及包含上传/分段的 prepare、switch 控制请求超时 |

Node 仅在 SSE 响应开始后发送心跳，结束或断开时清理定时器；下游背压期间暂停读取上游并跳过心跳，等待 drain 或结束连接。隐藏思考、工具参数片段仍属于上游活动。工具批次继续在 Go 共享校验完成后交付，心跳不代表工具已经执行。

## 工具交付与 Agent 续轮修正（2026-11-02）

- 工具轮的可见正文按原始调用位置截断，保留调用前的说明；DSML 本地恢复和最终格式修复不再让调用后的虚构结果进入正文。源码定位沿用与流式筛分一致的字面量规则，引用、CDATA 和代码围栏中的示例不作为边界。无目录请求在结束筛分时也保留隐藏 DSML 的边界信号，结构化调用仍只在共享校验成功后交付。
- 取消由工具目录或历史触发的 `HoldOutput`。auto/none 模式中的普通正文增量交付，候选工具批次仍在共享校验后交付；required/forced 模式保留整轮暂存。只有尚未交付非空白正文或工具调用时才允许纠正生成。此调整覆盖 Go 各协议与 Node；Gemini 的可见思考也在工具上下文中实时发送。
- 纯文本目录不再由 `tool` / `functions` 子串触发。识别 `Available tools: read_file`、`# Tools` 后的具名条目、`可用工具：read_file` 或 `Tool:` / `Parameters:` 结构；普通工具讨论、否定指令和空目录不触发。`tools: []` 明确关闭此兼容分支。更自由的目录格式应改用正式 `tools` 数组或上述明确结构。路由、提示词和历史拆分复用该判定，Gemini 原生入口在归一化 systemInstruction 后补齐路由判断。
- Responses 的前导纯空白在出现有效正文前暂存，不提前关闭 reasoning item 或创建 message item。重新生成丢弃失败轮的空白；有效正文保留自己的前导空白。已流出的思考继续在同一个未关闭的思考项中追加，最终只关闭一次。
- Responses 只合并无独立正文、元数据或后续新思考的相邻工具消息；显式 `content` 保留消息边界。reasoning 按源位置保留，避免覆盖、丢失或移到更早的调用之前。
- Responses 流式完成对象及其保存对象复用共享 `turn.Usage`，包含工具参数和隐藏思考，不再按清洗后的可见正文重新确定输出用量。


## 工具调用诊断记录

管理面板新增「工具调用诊断」，在共享收尾记录直接解析、本地恢复、模型格式修复、
最终策略与 Schema 校验，并关联各次纠正生成和账号切换。格式修复返回文本不代表
最终校验成功；正常中间收集不提交诊断，终止的读取错误会保留工具候选，避免遗漏失败现场。

普通记录保留最近 20/50 条；未解决失败自动锁定，后续重试成功保留原始失败现场。
诊断独立于响应历史，通过现有管理员鉴权查看。Node 流式验证与 release 沿用租约
关联生成尝试。记录失败不会改变工具解析或重试行为。详见 [工具调用诊断](tool-call-diagnostics.md)。

工具调用诊断的修复配置独立于采集开关。默认使用标准请求解析后的上游模型；各 Go
协议和 Node 租约路径传递相同的原模型元数据及配置快照。修复请求阶段记录实际提供商与
模型，配置密钥不回显到浏览器、不进入诊断导出或 Node prepare 数据。Node 校验超时
随配置调整并受整体剩余期限约束。自定义 API 的额外参数不允许覆盖模型、上下文或协议。

修复 API 的三种线格式在 provider adapter 中分别组装，并归一化成最终文本和思考文本，
再复用同一批次解析、占位符保真与 Schema 校验。Claude 使用 Messages，Gemini 使用
原生 generateContent；各自的思考块不作为最终调用。面板可以按所选协议拉取模型列表，
使用临时草稿及匹配的已保存密钥，分页读取、筛选并保存选择。拉取不会修改运行配置；
跨协议不复用隐藏的已保存密钥，过期拉取结果不覆盖新选择。


共享诊断收尾覆盖非流式、Go 流式的中断、取消、无法续传和续传失败，保留原始候选，
不执行截断内容的格式修复。校验后 Responses 存储失败仍标为未交付；Schema 展示复用
实际逐调用校验结果。Node 回调绑定生成尝试，迟到的旧尝试不能污染新尝试。

首个候选、校验、失败和最终结果同步保存；中间更新按 100 ms 窗口合并，关键检查点
刷新全部排队证据。索引提交失败回滚内存淘汰决策，修订号只在成功提交后前进。
修复地址仅识别当前协议的完整后缀，保留网关路径；保存返回脱敏规范配置，旧轮询
及离开页面后的迟到响应不能覆盖当前会话。后台暂停轮询，返回时立即刷新。
OpenAI 格式修复只接收最终文本，结构化调用与明确拒绝不会作为有效修复输出。

### 工具兼容与诊断优化（2026-11-02）

- system/developer 文本工具目录在明确的目录标题下支持 Markdown 行内代码、粗体、子标题、无序与编号列表，例如 `# Tools` 后的 ``- `read_file(path)` ``。普通工具讨论及显式空目录仍不激活工具分支；路由与提示注入共用此识别结果。
- 所有 Go 流式出口复用共享候选进度观察，仅保存生成中证据；最终解析、修复、Schema 和工具策略检查仍先于工具交付。Node 的租约进度回调同样只记录证据，错误及取消收尾复用共享失败观察并保留隐藏思考通道。
- Node 最终校验按生成尝试缓存，重复收尾不会对同一输出再次发起修复；重新生成清除缓存，continue/resume 保持当前回答状态。
- 修复模型可使用未保存的草稿配置发送固定样例测试；生产修复与测试共享 API 协议适配和格式保真检查，测试不读取主请求上下文。详见 `docs/tool-call-diagnostics.md`。


## 自定义修复 API 的重试与参数完整性

自定义修复支持 `retry_count`（默认额外重试 3 次，允许 0–10）及 `reasoning_effort`、`thinking_mode`。
重试覆盖网络/临时服务失败与模型格式修复失败，整个修复过程共用原配置的超时与客户端取消信号。
每次重试重新执行占位符恢复、调用身份和参数值保真检查；不会因已有部分成功而交付部分批次。
无法确认源参数边界时跳过模型修复，进入既有纠正生成流程；资源超限不再执行修复。
XML 结构化参数及宽松恢复必须保留完整源内容，不能忽略提前闭合临时根节点后的片段。
三种外部协议的思考强度由修复 API 适配器转换为原生字段；共享运行时只处理最终正文，思考仍单独归档。
上述配置不改变主请求模型、工具策略、主对话历史及文件处理。详见 [工具调用诊断](tool-call-diagnostics.md)。


修复模型或本地格式校验在取消之后完成时，结果仍被丢弃；取消优先于晚到的语法成功。
CDATA 与尾随普通文本混合的参数必须完整保留文本，并区分 CDATA 内外的实体解码规则，Go/Node 保持一致。
已保存修复配置无效时，只暂停模型修复调用，不触发其他模型替代；确定性本地恢复继续执行，诊断采集与管理端修正配置仍可用。

## 账号调度与限流换号

`runtime.account_sticky_enabled` 默认关闭，管理页“运行时设置 → 账号粘性模式”可切换。
关闭时按号池轮询；开启时优先用同一账号，直到它的并发槽位满，再按号池顺序使用下一个，
并发降低后优先回用靠前且仍可用的账号。该选项不是按会话或 API Key 绑定账号，
所有请求继续遵守工具池类型、账号/全局并发、队列和本地分钟/日额度。

两种模式均识别 HTTP 429 和 HTTP 200 中的 40029 / TOO_MANY_REQUESTS，
把受限托管账号的 `cooldown_until` 设为至少当前时间加 120 秒（已有更晚的冷却不缩短），
暂停新的调度；定时到期恢复。USER_IS_BANNED 标记 Banned / Disabled 并移出可用号池。
只把明确的封禁视作封禁，普通 403 不凭状态码禁用账号。
文件上传/查询、生成 HISTORY/TOOLS、会话/PoW 和回复启动都保留可换号的失败类型，
准备阶段在下个账号重新上传完整文件组、创建会话再请求回复，不向客户端交付失败账号的中间结果。
原始内联附件的部分上传失败时保留原请求，在新账号重建整个批次，不混用两账号的 ID。
明确的 `503 / upstream_unavailable / Upstream service is unavailable and returned no output.`
也纳入 120 秒账号冷却策略，流式和非流式均直接换号，不先在原账号合成一次空输出重试。
该规则按状态、错误码和上述明确消息匹配；普通网络错误和其他 503 保持原有重试策略。

手动指定目标账号、只有已上传 file_id 而无原始字节的引用不能跨账号自动恢复；
受限/封禁状态仍会更新。已经向客户端交付正文或工具调用后，不重放已交付操作。
每个请求通过 TriedAccounts 排除已失败账号，耗尽可用账号或请求取消时终止，避免循环换号。
配置可写入文件并通过导入/导出保留；关闭粘性开关只改变调度顺序，不关闭限流/封禁换号。

### 换号资源重建与鉴权恢复（v1.0.1）

- `TargetAccount` 只表示用户明确指定的账号；操作期间用可嵌套的临时账号固定保护会话、PoW 和上传批次。只有无法重放的文件引用才使用持久的文件账号绑定。
- 原始内联附件在当前请求或 Node 租约存活期间保留可重放快照。附件上传成功后，在上下文上传、创建会话、PoW 或回复阶段遇到可恢复账号失败，仍可在新账号重新上传完整附件组，再通过原协议适配器归一化输入、重建 HISTORY/TOOLS 和会话。禁止对用户正文做文件 ID 的全局字符串替换。已有 `file_id` 且没有原始字节的请求继续固定账号。
- 首次启动与换号重启复用同一执行入口；纠正生成的 PoW 也固定当前会话账号。明确限流、禁言、封禁和鉴权失败交给外层恢复，不再带旧会话或旧文件跨账号发送，也不在明确失败时复用旧 PoW。
- 上下文和附件准备支持托管账号鉴权恢复：每账号每请求最多强制刷新一次，刷新失败后允许切换健康账号；同一次已拒绝刷新不会被内外两层重复计入连续鉴权失败。普通网络失败不会因此标记为限流冷却。
- Node 检查每一次首发、纠正生成与换号响应中的账号错误。HTTP 429、40029、禁言和封禁 JSON 在进入 SSE 解析前处理，避免封禁被误判为空输出而只冷却两分钟。禁言未返回时间戳时与 Go 一致兜底十分钟。
- prepare 返回实际重建后的标准请求，Node 租约、后续重试与实际发送的附件、提示词保持一致。重放回调及原始附件快照属于服务端请求状态，不序列化到诊断记录或 prepare JSON。


### 2026-11-04：恢复预算、凭据与用量一致性

- Completion 的 HTTP 200 JSON 也检查 `40001/40002/40003` 和既有鉴权消息，明确映射托管/直传鉴权错误。封禁、禁言和限流优先于鉴权文本；普通 403 不直接推断为封禁或鉴权失效。Go 与 Node 使用同一组契约样本回归。
- `__stream_pow` 在刷新成功后同时返回当前 Token、基础请求头及其到期信息；Node 原子采用同一账号的凭据与 PoW。账号错误继续由外层重建会话与附件，已交付正文/工具调用不重放。
- 每次托管请求最多调度三个替代账号，上传、HISTORY/TOOLS、会话、PoW 和 completion 共用此额度；同账号刷新不消耗替代账号额度。换号的排队及取 Token 累计预算为 30 秒，客户端更早的截止时间优先。正常生成时间不扣减此等待预算。新 Node 桥由 Go 执行统一预算，旧 Go 桥仍沿用 Node 的本地上限。
- 开启 `auto_delete` 时，在更换账号、同账号建立替代会话及回收过期租约前，将已登记会话交给现有固定大小的异步清理队列。删除使用离开前快照的所属账号凭据；队列满或删除失败记录日志，不阻塞输出。最终会话继续执行原有 single/all 策略。
- Prepare 下发包含引用文件的初始 `prompt_tokens`。Node 只回传实际发送的 `retry_prompts`；换号后清空上一账号的重试列表。Go 校验和收尾以租约持有的 `PromptTokenText`、附件计数与这些提示词调用共享计算器，确保 HISTORY/TOOLS 和附件仅计一次。收尾响应返回同一份 usage，Node 复用为最终 SSE usage，无新增控制请求。返回 usage 后再执行远端清理，避免慢删除延迟最终帧。
- Release 失败时 Node 保留 prepare 提供的完整输入 token 基数并估算重试增量；旧 Go/Node 组合继续支持原有回退。错误请求不写消费账本。


## 参数与请求转录保真完善（2026-11-04）

- 完整合法的 JSON 参数保留原有类型与对象外层，包括仅含 `item` 或与参数同名键的对象。
  XML 的 `<item>` 列表与旧式不完整 JSON 容错继续保留，不能用其规则拆解已经合法的 JSON。
- 顶层 parameter 和嵌套文本均支持连续 CDATA 分段；历史渲染对 `]]>` 的标准分段可无损回读，
  保留 CR/LF、缩进、实体字面量及分段后的文本。原有参数内工具代码示例的兼容处理继续保留。
- OpenAI 顶层 `functions` / `function_call` 在路由、工具策略及提示词构建前按兼容路径处理；
  旧声明归一化为 `tools` / `tool_choice`，不修改调用者原数据。新旧声明冲突返回 400。
  输出继续使用该接口的现代 `tool_calls` 形态；旧客户端需要识别现代响应字段。
- OpenAI、Claude 和 Gemini 的已知正文文本块按顺序直接拼接，不裁剪原文或额外插入换行。
  OpenAI refusal 块以标签保留全文；未识别块作为有标记的数据转录，显式二进制字段与
  不透明签名省略，降级 JSON 保留完整文本，由请求总大小和外置传输控制容量。助手拒绝回答不会消失。
- 共享 turn 在原始工具边界截断可见正文。四种非流式协议与流式协议均保留调用前的说明，
  不返回调用后编造的结果；Responses 的正文项排在工具项之前。

### 采样参数与上游请求体边界（2026-11-04）

上游 completion 请求体的键集与真实 App 严格一致：`chat_session_id`、`parent_message_id`、
`prompt`、`ref_file_ids`、`thinking_enabled`、`search_enabled`、`audio_id`、`preempt`、
`model_type`、`action`。此前 OpenAI Chat / Responses 与 Gemini 出口会把 `temperature`、
`top_p`、`max_tokens`、`max_completion_tokens`、`presence_penalty`、`frequency_penalty`、
`stop`（Gemini 侧来自 `generationConfig` 的 `temperature`/`topP`/`maxOutputTokens`/
`stopSequences`）合并进请求体，Claude 转换层也会复制 `temperature`/`top_p`/`stop`。
这些键在真实客户端中不存在，属于请求形状偏差，现已全部移除：

- 各出口仍兼容接收这些字段（含 `reasoning_effort`、`thinking` 等思考强度形态），请求不会报错；
  `StandardRequest` 不再保留透传通道，`CompletionPayload` 只输出 App 真实键集。
- 思考开关完全由模型名决定：`-nothinking` 后缀关闭，无后缀开启（`config.GetModelConfig`
  按后缀返回开关）。请求里的 `thinking` / `reasoning` / `reasoning_effort` 不再折叠成
  `thinking_enabled` 布尔——App 的真实开关就是所选模型，请求参数无法改变它；文件上传
  路径的表单 `thinking_enabled` 与 `X-Thinking-Enabled` 请求头同样仅兼容接收。Claude
  代理转发体不再注入 thinking 标记，Gemini 代理不再改写 `thinkingConfig`，响应侧是否
  暴露思考块均按转发模型的开关决定。
- 出站请求头继续全部由服务端按账号档案重建，客户端请求头不进入上游；`max_tokens`
  的 Claude 缺省补齐仅用于协议层校验，不发送上游。
- 工具调用修复链路是例外：它调用的是管理员配置的外部 OpenAI/Claude/Gemini 兼容 API，
  `reasoning_effort`/`thinking`/`max_tokens` 按该 API 的语义正常组装，不属于上游透传。

### 文本输出格式约束

本服务反代 App，不提供原生温度、采样参数或思考强度控制；这些字段在各出口仅做兼容接收，
一律不进入上游请求体。思考开关完全由模型名决定（`-nothinking` 后缀关闭，无后缀开启），
与 App 内选模型的行为一致；提示词注入设置沿用现有行为。输出格式采用提示词约束及最终校验实现，
不声称上游具有原生 JSON / Schema 模式，也不靠格式修复猜测或修改答案内容。

- Chat 接受 `response_format` 的 `text`、`json_object`、`json_schema`；Responses 接受 `text.format`。
- Gemini 接受 `generationConfig.responseMimeType=application/json`，以及 `responseSchema`
  （适配其大写类型）或 `responseJsonSchema`；两种 Schema 同时提供或与 text/plain 冲突时返回 400。
- Claude 的 `output_config.format` 可使用 `json_schema`。格式约束独立保存在标准请求中，
  思考注入、附件重放及 HISTORY/TOOLS 外置后重建提示词仍保留，客户端消息本体不被改写。
- JSON 答案必须为一个完整 JSON 值；json_object 必须是对象，json_schema 再验证全部 Schema。
  编译复用有界缓存，只允许本地引用，不访问网络或文件。请求 Schema 无效时返回 400。
- 工具调用先经过原有完整批次、名称、策略及参数校验；输出格式约束只检查无工具的最终答案，
  允许 Agent 先调用工具取得信息。格式不合规返回 `invalid_response_format`（HTTP 422 / SSE 错误），
  不把不合规文本作为成功答案交付，也不针对这类错误自动重新生成。
- 仅对有输出格式约束的请求暂存正文，完成共享校验后再交付；思考仍可按现有策略实时显示。
  Go 四协议和 Vercel Node 使用相同校验，Node prepare 的 validate_output 控制正文暂存，
  已通过 Go 校验的正文原文用于交付。普通文本与既有工具请求继续沿用原先的交付边界。


## 2026-11-04：响应预检、恢复预算与会话结束

- Go completion/分段 completion 的响应预检同样受 `DS2API_STREAM_IDLE_TIMEOUT_MS` 与 `DS2API_STREAM_MAX_DURATION_MS` 约束；取消或超时会关闭阻塞 reader。预检成功后移交原始前缀及余流，并停止预检计时器，后续流由既有 SSE 消费器管理。
- Node 首块/JSON 预检使用请求的剩余总预算与空闲超时；JSON 超过 1 MiB 的检查上限时移交已读前缀和未读后缀。HTTP 错误正文最多保留 64 KiB 并标记截断，读取仍受超时和取消约束。
- 未交付可见答案或可执行工具调用时，Node 的连接失败可经共享 Go 账号恢复入口重建请求；每次恢复循环设有本地上限，账号切换继续服从 Go 请求级预算。已交付答案后禁止换号重放。
- 非流式请求切号后，替换账号在文件重建、会话或 PoW 阶段失败，会按同一账号切换预算继续恢复；旧账号的文件引用仍不得跨账号复用。
- Chat、Responses、Claude、Gemini 的远端会话统一在请求结束时执行清理。`single` 删除当前会话，`all` 在最终结束时删除账号会话；中途退役的会话始终只删除该会话。清理快照保留原账号身份，优先读取其刷新后的 Token，并经固定队列后台执行。`none` 不触发自动删除。重复结束不会重复提交同一清理。
- 工具批次继续等待共享最终校验；连接复用、预检与重试不会提前交付未经验证的工具调用。

## 2026-11-04：工具来源完整性与协议转换一致性

- 格式修复的来源清单包含未命名参数和 invoke 内无法归属的数据。未知节点、游离正文、引文或 CDATA 不能被修复模型删除后当作成功；这类歧义交给共享生成纠正流程。重复的 XML 属性（包括工具名、参数名和调用 ID）拒绝解析，Go 与 Node 一致，不能静默选择最后一个值。
- 文本形式的历史工具块重新渲染时保留原始 CDATA 字符串，包括看似布尔值、null、对象和数组的文本。无参数 Schema 的兼容调用也保留这些字符串；明确的 Schema 仍负责约束和恢复参数类型。
- Claude/Gemini 的 Vercel prepare 与兼容转发使用原生请求适配器和同一标准消息模型，再序列化为内部 Chat 请求。工具选择、并行限制、允许工具子集、嵌套 Schema、调用/结果 ID、is_error 和输出格式约束沿用共享校验。release 不归一化对话，继续按原租约释放。
- Gemini 带 thoughtSignature 的普通文本仍是 assistant 正文；只有明确的 thought:true 标为思考。assistant 顶层 refusal 保留为拒绝内容；Responses 兼容消息保留 id、name 和显式 prefix，不因空正文而降成 user。
- 图片和文件在归一化中保持结构化及原位置，含 Claude tool_result 内的附件；共享上传完成后再转录为附件引用。上传前的提示词预览仍清除二进制数据。普通文本工具结果保持原文和空白。
- 启用 input_file 且达到 min_chars 后，没有 assistant/tool 循环的首轮 user 正文合计超过 32768 个 Unicode 字符时，即使存在前置 system/developer 指令，也将完整首轮转录放入 HISTORY.txt，指令继续固定在实时提示词。较短首轮及已开始的活动工具循环沿用原规则；不会总结或裁剪正文。
- 上游客户端已有按账号、凭据、上传配置与内容摘要隔离的上传复用缓存。历史文件分步上传失败时不发布部分拆分状态；缓存文件可能同时被其他请求引用，不能在局部失败时直接删除复用文件。

### 第二轮上下文与 Schema 边界完善

- 附件收集只把明确的 ID 列表中的字符串，以及附件内容块的 file_id/id 当作文件引用。普通消息或工具结果中的字符串数组、profile 等未知块的业务 ID、工具结果普通对象的 file_id 不再进入上游 ref_file_ids；顶层 files 与 attachments 使用相同的显式引用规则。
- Responses 的思考合并保留拒绝信息、显式空正文、预填充和命名消息边界。兼容的纯思考内容块仍可与紧随的函数调用组成一轮；字符串兼容输入保留首尾空白，普通内容与 reasoning 项按源顺序转录。
- 空工具名的 invoke 不交给格式修复模型补写或删除；与未命名参数、未归属正文共用来源完整性拒绝规则。
- 没有字段类型约束的工具 Schema 不把 CDATA 字符串改成布尔、null 或容器值；明确的非字符串字段仍沿用既有类型恢复，并在共享收尾验证。
- Gemini Schema 的 nullable 包含 enum 与组合 Schema，使用显式 null 分支，保留本地引用的根定义。SDK 字符串形式的长度/数量上下界转成整数，数值类型枚举转成保留精度的 JSON 数值；非法边界仍由共享 Schema 校验明确拒绝。规则来自 [Google Gen AI SDK Schema 定义](https://googleapis.github.io/js-genai/release_docs/interfaces/types.Schema.html)，同时用于工具参数与响应格式。
- 历史文件外置保存本次原始 user 输入作为本地归档元数据，响应记录继续显示真实用户任务，避免误用续聊系统指令作为输入标题；该字段不进入上游提示词或公开请求。
