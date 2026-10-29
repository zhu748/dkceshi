# APK 与三份原始 HAR 的请求对齐补充

核对 APK：`com.deepseek.chat` 2.7.1（287），SHA-256
`cd461f3d7c78f0c4ab6b8c4ff7270e16a766042170dfebead540b7f174948c5c`。
本轮直接读取以下三份附件，共 262 条记录：

- `gator.volces.com_2026_10_28_12_33_05.har`：73 条。
- `gator.volces.com_2026_10_28_16_50_45.har`：119 条。
- `发送文件删除对话.har`：70 条。

脱敏协议事实保存在 `tests/fixtures/android_app_2_7_1_reference.json`。
原始 HAR、认证头、账号、设备 ID、会话 ID、提示词和文件内容不进入仓库。

## 聊天、PoW 与续写

12 条 completion 均使用既有 10 字段顺序；首轮 `model_type=default`，
续轮 `model_type=null`。远程 `model_configs` 中 default 启用，expert/vision 关闭；
项目原有 default 映射保持适用。对外模型名称不等于服务端权重版本证明。

PoW 头按 APK `AuthedPowSolution` 和抓包的原始顺序输出：
`algorithm, challenge, salt, signature, answer, target_path`，再做标准 Base64。
`target_path` 从发起挑战的请求路径取得，不依赖响应可选字段。
抓包 `expire_at` 为毫秒时间戳，`expire_after=300000` 为毫秒时长。
缓存优先采用实际 `expire_after`，缺省时支持毫秒/秒绝对到期时间，
使用前保留 APK 的 20000ms 余量；没有有效期的结果不缓存。
预取固定原账号；清除后的在途结果不会写回缓存。

新增的 continue 抓包没有 `x-ds-pow-response`。Go 和 Node 续写均不发送该头，
Node 不为 continue 额外刷新挑战；completion 和真正的重试仍使用其独立 PoW。
continue 与 stop_stream 的 JSON 字段顺序在发送边界固定。
Go JSON 同 APK/Node 一样直接输出 U+2028/U+2029，调用者的字面 `\\u2028` 保持原义。

## 删除会话

四条单条删除请求均为 POST `/api/v0/chat_session/delete`，
JSON 为 `chat_session_id` 后接 `chat_session_ids:null`。
全部删除的空体规则来自 APK `vy0` 调用点；本轮 HAR 没有 delete_all 样本。
项目该接口发送空请求体，不附加 JSON Content-Type，长度为 0。

单条 token 删除和两个全部删除入口均检查 HTTP 状态、外层 code 与内层 biz_code；
业务拒绝不再返回成功。会话删除只允许原账号刷新 token，不跨账号重试同一会话 ID。
管理页本地历史删除与上游会话删除仍是不同操作。

## 文件与图片

9 次上传为 6 张 WebP、1 个 PDF、2 个 TXT。逐个解码 HAR 的 base64 multipart 后确认：
单个 file 分段、octet-stream MIME、70 字符新边界、CRLF 结尾；
`x-file-size` 等于实际文件部分的字节数，无 multipart 总/分段 Content-Length。
不能因为上传内容是 WebP，就推断所有图片原始格式或证明每张图片都被压缩。

APK 静态的相册压缩比例缺省是 NaN，普通 JPEG/PNG 默认保留原图；
本轮原始 settings 响应则出现 `photo_picker_compress_ratio=0.8`、
`picture_compress_format=webp`、`query_files_time_interval=1000`。
因此运行时按实际配置执行，不能把 0.8 固定成所有设备的默认值。

上传前按账号身份和代理读取 `/api/v0/client/settings?scope=main&did=...`，
仅消费图片比例、图片格式和文件轮询间隔。按账号凭证、安装标识、DID、语言隔离缓存；
成功缓存 5 分钟、失败短暂退避，已有值或 APK 缺省用于降级。
稀疏响应没有某个字段时保留该字段的既有值，与 APK 的本地设置缓存语义一致。
这里只实现上传相关设置读取，没有实现全部 settings 生命周期或 report 上报。

优先级：`preserve_image=true` 始终保留原图；否则请求/账号显式配置优先，
全局环境覆盖次之，远程设置补足缺省；三者都未提供比例时保留原图。
特殊文件名通过 Ktor 风格的引号/反斜线转义发送，不用下划线替换原字符。

多文件内联输入共用两阶段流程：先上传得到文件 ID，再以重复的
`file_ids=a&file_ids=b` 合并轮询；成功文件移出下一次查询，
所有文件就绪后才用于聊天，失败/取消终止处理。OpenAI、Responses、Claude、Gemini
均使用这一共享路径。单文件 `/v1/files` 仍等待就绪后返回。
轮询采用实际设置间隔，抓包缺省为 1 秒；本项目的 60 次 / 65 秒等待上限仍保留。

## 验证与边界

回归包含原始顺序/字节比较、毫秒有效期、可选 target_path、删除业务拒绝、
continue 无 PoW、重复 file_ids 与 PENDING/PARSING/SUCCESS、批量等待失败传播、
远程配置缓存并发与隔离、原图覆盖及特殊文件名。

仍不能宣称与真机完全相同：Go/Node TLS 和 HTTP2 SETTINGS/HPACK、连接复用策略
尚无本轮原始网络证据；Vercel Node 流没有账号 SOCKS 代理支持。
Web SDK 不等价于 APK 原生数美 SDK，HIF 自动获取和遥测注册尚未实现。
HEIF/动画/超大图与 Android 原生编码器字节一致性仍未实现。
批量会话删除、fork_file_task 等没有新增公共业务入口。
