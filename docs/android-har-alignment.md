# Android App 2.7.1 抓包对齐

本次基于三份用户提供的 HAR（共 226 个条目）检查 `chat.deepseek.com` 的请求：

- `app注册.har`（83）
- `gator.volces.com_2026_10_28_12_33_05.har`（73）
- `发送文件删除对话.har`（70）

仓库只保留脱敏后的协议特征 `tests/fixtures/android_app_2_7_1.json`，用于自动化回归；
没有复制 HAR 中的认证 token、密码、验证码、设备标识和文件正文。
新增的原始 HAR 复核覆盖另一组 262 条记录，见
[2026-10-29 补充](android-har-reference-2026-10-29.md)。

## 请求头与设备配置

| 字段 | 默认值 / 来源 |
| --- | --- |
| User-Agent | `DeepSeek/2.7.1 Android/36` |
| x-client-platform | `android` |
| x-client-version | `2.7.1` |
| x-client-locale | `zh_CN` |
| x-client-bundle-id | `com.deepseek.chat` |
| x-client-timezone-offset | `28800`，秒 |
| x-device-model | 历史样本为 `V2507A`；当前按账号选择型号，无固定默认值 |
| x-device-id | 账号 `installation_id`，与登录 SDK `device_id` 分离 |
| x-rangers-id | 账号 `rangers_id` 或显式全局值，未注册时不生成伪值 |
| Accept / accept-charset / Accept-Encoding | `application/json` / `UTF-8` / `gzip` |
| authorization | 当前账号 token，刷新后使用新 token |
| x-ds-pow-response | 按当前账号、接口路径计算的一次性 PoW |

登录的数美 `device_id` 与请求头安装标识 `x-device-id` 已分开；后者与 check_device 使用同一来源。
安装标识使用 APK 的 Android ID / 品牌 AES-CBC 派生算法；数美 SDK ID 独立生成并保存。
需要真实安装信息时，为每个账号配置自己的真实值，不复用其他设备的认证材料。
账号的设备配置支持配置导入、导出、创建和 `PUT /admin/accounts/{identifier}` 更新。

```json
{
  "accounts": [{
    "email": "example@example.com",
    "password": "your-password",
    "device_model": "",
    "device_id": "",
    "rangers_id": ""
  }]
}
```

全局覆盖保留 `DS2API_DEEPSEEK_CLIENT_NAME`、`CLIENT_PLATFORM`、`CLIENT_VERSION`、
`ANDROID_API_LEVEL`、`CLIENT_LOCALE`、`CLIENT_TIMEZONE_OFFSET`、`RANGERS_ID`，
并增加 `DS2API_DEEPSEEK_DEVICE_MODEL`、`DS2API_DEEPSEEK_DEVICE_ID`。
前述缩写均使用完整前缀 `DS2API_DEEPSEEK_`。账号字段优先于相应全局覆盖。

## HTTP/2 头部顺序

Go 默认启用有序 HTTP/2，普通字段小写，伪头顺序为
`:method, :authority, :path, :scheme`。普通客户端身份字段依次为：

`x-client-platform, x-client-version, x-client-locale, x-client-bundle-id, x-client-timezone-offset, x-device-model, x-rangers-id, x-device-id`。

之后是 `user-agent, authorization, accept, accept-charset` 和内容相关字段，最后 `accept-encoding`。
各接口有以下差异：

| 接口 | 差异 |
| --- | --- |
| chat/completion | PoW 在身份字段之前；JSON 带 Content-Length |
| file/upload_file | 前置 PoW、thinking、file-size、model-type；无总 Content-Length |
| file/fetch_files 等 GET | 不带 Content-Type、Content-Length |
| chat_session/create | 空请求体，Content-Length 为 0 |
| users/auth_token/check_device | authorization 在身份字段之前 |
| client/settings/report | x-settings-token 在 device ID 后、User-Agent 前 |

后两项为头部构造器支持的已观察顺序；未增加客户端设置上报；设备检查已在登录后执行。
额外头部采用稳定排序，去重并过滤 HTTP/2 禁止的连接头，不注入浏览器 Origin/Referer 等字段。

## 2.7.1 APK 静态对齐补充（逆向核实）

以下结论来自对 deepseek.apk（versionCode 287）的反编译核对，与 HAR 观察互相印证：

| 项 | App 实现 | 项目实现 |
| --- | --- | --- |
| 登录 JSON 字段序 | `email, password, device_id, os`（UsersLoginWithEmailAndPwdRequest）；手机号为 `mobile, area_code, password, device_id, os` | MarshalAppJSON 按 `/api/v0/users/login` 定序输出同序 |
| check_device 请求体 | `device_model`（Build.MODEL）、`device_id`，登录成功与启动 check_update 流程均调用；响应 `biz_data.rotate.token` 非空时轮换本地 token | 登录成功后调用，轮换 token 优先于登录响应返回并持久化；失败仅告警不阻断登录 |
| x-device-model | `Build.MODEL`（真实机型） | 账号 device_model（同语义） |
| x-hif-dliq / x-hif-leim | 从两个 HIF query 端点拉取认证材料，缓存受 x-hif-ttl 与远程设置控制；可缺失 | 仅发送配置中真实、未过期的材料；没有自动获取流程 |
| 数美组织 | `P9usCUBauxft8eAmUXaZ` / appId `default`（fengkongcloud.com 双区域端点） | 网页端 SDK 同组织同 appId（fp-it-acc.portal101.cn 签发），相同组织配置不证明网页与原生 SDK 等价 |
| 安装标识 | AES-CBC(MD5(packageName) 为 key、全零 IV, android_id + "_" + Build.BRAND) | 相同 AES 派生算法；缺少真实 Android ID 时使用按账号派生的输入，不能视为真机身份 |

有序发送使用 fhttp 的 HTTP/2 实现处理 DATA 流控、分片和取消，替代原有手写帧循环。
JSON、SSE、GET 和上传都经过所选账号的客户端，不再绕过账号 SOCKS 代理。
无账号代理时遵循系统代理配置。`DS2API_DEEPSEEK_USE_FRAMED_H2=0` 可回退旧路径；
该回退路径及仅旧路径允许标准库 fallback；该路径不保证头部顺序一致。

## 照片和文件

| 项目 | 行为 |
| --- | --- |
| Multipart | 单个 `file` 分段，不添加 purpose、model_type 表单字段 |
| 分段 MIME | 均为 `application/octet-stream`，包括照片、PDF、TXT |
| boundary | 每次上传新生成 70 字符的小写十六进制/连字符边界，CRLF 结尾 |
| x-thinking-enabled | 随本次模型/请求的最终思考开关发送 `1` 或 `0`，不再固定 0 |
| x-model-type | `default` |
| x-file-size | 实际文件字节数，图片转码后重新计算；不含 multipart 包装 |
| PNG / JPEG 照片 | 相册压缩比例缺省时保留；显式设置比例后使用有损 WebP/JPEG，应用 JPEG EXIF 方向 |
| 已有 WebP、GIF、APNG、其他格式 | 保留原始字节，不丢弃动画帧 |
| PDF、TXT、HISTORY.txt、TOOLS.txt | 保留文件正文；走相同的 multipart 格式和思考开关 |
| 异步处理 | PENDING 后轮询 fetch_files，成功才用于聊天；失败状态直接报错，取消中断等待 |
| 聊天引用 | 文件 ID 传入 ref_file_ids，支持一次引用多张照片 |

转码策略已按 APK 静态代码修正：启用压缩时使用有损编码，并按 30 MiB 解码预算采样；见下方补充。
超过 2000 万像素或 WebP 尺寸上限的图片保留原始格式，以限制解码开销。
HAR 无法推断 App 的压缩质量、缩图阈值和具体编码器，因此不宣称转码结果与 App 文件字节相同。
`/v1/files` 可传 `preserve_image=true` 保留原图，内联文件请求也支持此布尔字段。
本地文件内容缓存保留调用方上传的原始文件和 MIME。

OpenAI Chat / Responses 的 data URL 和内联文件、Claude 的 base64 image/document、
Gemini 的 inlineData/inline_data 共用上传逻辑。原始 base64 在上传后替换为文件引用，
不会作为正文注入提示词。这里不自动下载远程图片 URL。
上传内联文件或携带文件引用的托管请求固定当前账号，避免中途换号混用文件 ID；同账号 token 刷新仍可进行。
通过 `/v1/files` 单独上传后，在多账号池中再次使用该 ID 时，应根据返回的 `account_id`，
在后续请求携带 `X-Ds2-Target-Account`，确保引用同一个账号的文件。

显式文件上传的思考开关从 `model` 解析；可用表单 `thinking_enabled=1/0` 或
`X-Thinking-Enabled: 1/0` 覆盖。`-nothinking` 模型保持关闭。
HISTORY/TOOLS 的上传直接沿用标准请求的 Thinking 值。

## 聊天 JSON

在发送边界序列化为：

`chat_session_id, parent_message_id, prompt, ref_file_ids, thinking_enabled, search_enabled, audio_id, preempt, model_type, action`。

缺省 audio_id 补 null；首轮 model_type 为 default，已有 parent_message_id 的续聊为 null，
与样本一致。删除单个对话发送 `chat_session_id` 和 `chat_session_ids: null`。
Go 与 Node 采用相同的字段顺序，不修改共享提示词和重试使用的标准请求对象。

## 无法由 HAR 验证的部分

HAR 不含原始 TLS ClientHello、HTTP/2 SETTINGS、WINDOW_UPDATE 和 HPACK 索引字节，
无法用这些文件验证 JA3/JA4、Huffman、never-indexed 或连接复用策略。
当前 TLS 只使用已有 Android OkHttp 预设，不再在握手失败时轮换 Chrome/Safari/随机预设；
这个预设并非从 Android 36 的原始握手测得，连接复用策略也没有宣称复刻 App。
Vercel Node 出站 fetch 共享账号头部和 JSON 规则，但 TLS 和头部的实际线序由 Node 平台控制。

注册抓包中的请求加密、游客 PoW、验证码以及 gator/hif 的遥测/设备认证材料不属于静态请求头。
本次没有硬编码这些材料、重放注册请求，或伪造其加密实现；项目原有登录逻辑保持独立。

## 验证

脱敏 HAR fixture 检查各请求形态的头部顺序和静态值。
实际 HTTP/2 测试检查伪头、普通头部、空请求、GET 和 1 MB 小窗口上传；
另外覆盖图片解码/方向/透明度、转码后大小、账号设备隔离、异步文件状态、
Claude/Gemini 原生上传、多个文件引用及 Go/Node JSON 一致性。

## 账号设备档案补充

托管账号首次使用时自动保存型号、品牌、Android API 等级及安装标识；登录 SDK ID 按 auto/manual 模式获取；账号字段覆盖环境默认。管理界面可查看、修改当前页档案。休眠和配置更新保留这些 Android 字段。只读部署使用按账号稳定派生的后备值。详见 [功能对齐说明](function-alignment.md)。

旧 OkHttp 预设没有 ALPN 扩展时会补入并重新构建 ClientHello；有序 H2 路径协商 h2，标准库旧路径和 TTS WebSocket 只协商 HTTP/1.1。这是协议兼容修正，未用 HAR 推断实测 TLS 字节。

## 本次 APK 复核修正

登录 SDK ID、安装标识、auto 生成路径、遥测头、时区、上传参数转义、图片压缩和 Node HTTP/2
已进一步核对；上文保留的是历史 HAR 背景。当前实现及明确边界以 [APK 请求对齐核对](apk-request-alignment.md) 为准。
