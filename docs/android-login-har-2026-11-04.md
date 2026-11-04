# 2026-11-04 登录、MFA 与发送消息抓包核对

## 范围和关键修正

输入为 `登录并且发信息.har`，120 条记录，实际采集时间为北京时间
2026-11-04 15:00:00.882–15:02:02.978。
其中 46 次 chat.deepseek.com API、2 次 HIF 查询、2 次原生数美 SDK 请求、
13 次火山遥测/注册、29 次 hCaptcha/资源请求、28 条 CONNECT。
业务 API 共 22 个端点、25 种头部形态；同端点可能随登录态或设置 token 改变头部。

本样本纠正了 [上一轮四份 HAR 核对](android-har-alignment-2026-11-04.md) 对安装头默认策略的推广：
此前 216 次业务请求不带 x-device-id；本次 46 次中有 45 次携带，包含登录、设备检查和聊天。
仅最早的 ip_to_country_code 不带，随后尚未登录的 check_client_update/settings 已携带。
因此该头的出现不能简单等同于“已登录”，也不能认为新版 App 永远不发送。
先前样本为何省略这个头，当前证据仍不能确定。

最终策略恢复为：默认保留已知安装标识；`DS2API_DEEPSEEK_SEND_INSTALLATION_HEADER=0/false/no/off`
显式省略；`1/true/yes/on` 和未设置均保留。大小写/两端空白不影响开关。
本地安装标识和 SDK 登录 ID 仍分开，连接池继续在省略头时保留本地安装身份隔离。

45 次安装头全部相同，且逐条与 check_device.device_id 相等。
本次 36 次携带 Bearer 的业务请求使用同一个 token，均与 MFA 验证响应中的 user.token 相同。
check_device 返回 rotate=null，本样本没有实际 token 轮换。

## 登录时序和身份来源

以下时间均为北京时间；只记录字段结构，不保存认证值。

| 时间 | 请求/结果 | 观察与含义 |
| --- | --- | --- |
| 15:00:00 | ip_to_country_code | 无安装头、无授权 |
| 15:00:02 | check_client_update；settings main/provider/model | 已有安装头与 Rangers ID，无授权 |
| 登录前 | 数美 cloudconf / deviceprofile | 原生 SDK HTTP/1.1，Dalvik Android 16 UA；不同于 Web SDK 上报 |
| 15:00:12 | users/login | HTTP 200/code=0/**biz_code=14**，biz_data 只含 mfa_session_id |
| 登录验证阶段 | hCaptcha 页面、挑战与提交 | 存在完整挑战交互，不能把 HTTP 200 当作登录完成 |
| 15:01:00 | users/create_guest_challenge | 请求体 target_path，返回 guest_challenge |
| 15:01:01 | mfa/create_verification_code | 携带访客 PoW、mfa_session_id、数美 ID 和验证结果，返回发送窗口 |
| 15:01:14 | mfa/check_verification_code | 正文 mfa_session_id/verification_code；返回 user.token 与 mfa_ticket |
| 15:01:16 | check_device；users/current | 已有授权；check_device 使用安装 ID，rotate=null |
| 15:01:16 | trusted_device/register | 正文 mfa_ticket/mfa_public_key/mfa_algorithm，算法 **ES256** |
| 15:01:18 | trusted_device/verify | 正文 mfa_ticket/mfa_signature，返回 credential_id/expire_at |
| 15:01:21–15:01:41 | 3 次 completion、1 次上传 | 使用上述授权和相同安装头；请求结构沿用已有规则 |

首次 password login 没有直接返回 user.token，实际登录态由验证码校验取得。
仅有“发送验证码成功”或 mfa_session_id 不构成登录成功。
本次 3 个 completion 均为首轮，不能用它验证 MFA 后的同会话追问；上一轮样本覆盖追问。

## 加密登录请求与项目差距

| 项目 | 本次实测 | 当前项目 |
| --- | --- | --- |
| Content-Type | application/json | application/json |
| 实际 body | HAR 标记 base64，解码后 **192 字节**，不能解析为 JSON | 明文序列化的登录 JSON |
| x-request-encryption-key | 存在，344 字符 | 本轮支持其正确线序；没有实现 App 密钥封装/正文加密 |
| 头部位置 | user-agent → accept → x-request-encryption-key → accept-charset | Go/Node 传输构造器已对齐 |
| 密码登录结果 | MFA 待验证 | 现在显式返回 captcha_required，提示完成官方客户端验证并配置 token |
| 后续 MFA | 访客挑战、验证码校验、可信设备注册/签名 | 需要独立交互与密钥生命周期，当前未实现完整业务编排 |

`Content-Type: application/json` 不代表线上 body 是明文 JSON。
192 字节与 344 字符是观测长度，不能据此确定对称算法、模式、IV/nonce/tag 或密钥封装算法。
未解密或保存用户密码密文；传输测试使用合成不透明字节，只证明发送与头序正确。

登录错误处理已识别 **biz_code=14 且存在非空 mfa_session_id** 的组合：
返回需要验证的 RequestFailure，未产生登录 token，不执行 check_device，
不包装 auth.ErrLoginRejected，避免号池把未完成 MFA 计作明确凭据拒绝。
其他错误码继续按既有错误规则处理，不把所有 biz_code=14 响应盲目当成已登录。

## 请求头的新增差异

UA、平台、版本、locale、bundle、时区、机型仍为已有 2.7.1/Android 36 样本值。
除安装头外，新增需要区分的顺序如下：

| 形态 | 观察到的特殊线序/字段 | 本轮处理 |
| --- | --- | --- |
| 加密 password login | accept 后 x-request-encryption-key，再 accept-charset | Go/Node 构造器对齐 |
| MFA 发送验证码 | user-agent 后 x-ds-guest-pow-response，再 accept | Go/Node 构造器对齐 |
| check_device / trusted register / trusted verify | authorization 位于全部身份字段前 | 三条路径均按此前置 |
| settings GET | 登录前无授权、登录后有授权；scope/did 查询键 | 分别纳入头序回归 |
| settings/report | 第二次多出 x-settings-token，位于安装头后、UA 前 | 已有位置正确，新增两个形态回归 |
| telemetry | 加密键/压缩头前置；无 Rangers ID；accept-charset 先于 accept=*/* | 新增独立头序，正文为 octet-stream |
| 上传/聊天 | 安装头在 Rangers ID 后、UA 前；上传仍无总 Content-Length | 原有普通业务线序适用 |

构造器的顺序支持不代表新增了 MFA、遥测或客户端设置上报业务。
本次对 25 种形态用本地 Go/Node HTTP/2 服务收包，发送的是合成 ID、正文和认证占位值。
Node 将加密键、访客证明和设置 token 加入敏感头列表；HAR 无法验证 App 的 HPACK 索引策略，
本轮线序测试也不证明两者的压缩编码字节相同。

## SDK、HIF 与远程设置

| 项目 | 本次证据 | 对齐边界 |
| --- | --- | --- |
| 数美上报 | fp-sa-it-acc.fengkongcloud.com；deviceprofile/v4 code=1100，cloudconf code=1901 | 证实原生 SDK 的域名/UA/结构；不证明当前 Web SDK 生成器与原生结果等价 |
| Rangers 注册 | 2 次 device_register，返回 bd_did/device_id/install_id 等 | 当前仍使用配置的真实 Rangers ID，没有新增自动注册 |
| HIF-leim | 2 次 GET /query，HTTP/code/biz_code 均成功，biz_data 含 value，x-hif-ttl=600 | 证明存在认证材料查询及 TTL，未保存材料或新增自动签发流程 |
| HIF 请求头 | 46 次业务均无 x-hif-dliq/leim | 查询存在不等于后续业务一定携带；不能从这份 HAR 推断缺省原因 |
| HIF-dliq | 只看到 CONNECT | 不算成功签发样本 |
| 登录前 main 设置 | should_use_sm_device_id=true、hcaptcha_enabled=true | 支持本次实际 SDK/MFA 验证链路；非所有账号永久配置 |
| 登录后 main 设置 | photo_picker_compress_ratio=0.8，picture_compress_format=webp | 既有远程设置解析已覆盖，不设为全局固定默认 |
| 文件轮询/预取 | query_files_time_interval=1000；pow_prefetch=true/count=1；session_prefetch=true | 支持现有轮询/PoW 预取方向；不宣称调度和 App 完全一致 |

整个样本没有原始 TLS ClientHello、HTTP/2 SETTINGS/HPACK 数据，仍不能确认 JA3/JA4 或
Android 原生 TLS 的逐字节一致性。不同域名同时存在 App、Dalvik SDK 和 hCaptcha WebView
网络流量，不能为所有域名套用一套 App 请求头/传输身份。

## 验证与数据处理

新增脱敏夹具 `tests/fixtures/android_app_2_7_1_login_2026_11_04.json`，保留头序、公开静态值、
状态码、字段名、时间、计数和合成 MFA 响应。密码、验证码、手机号、邮件、SDK/安装/Rangers ID、
session/ticket、token、签名/密钥材料、密文及 hCaptcha 挑战材料均不写入夹具。
核对与回归在本地完成，未使用抓包凭据重放上游请求。

本地验证结果：

- `go test ./internal/deepseek/... ./internal/auth`：client、protocol、transport、auth 四包通过。
- `node --test tests/node/gif-har-headers.test.js tests/node/app-transport.test.js tests/node/deepseek-wire.test.js`：16 项通过，包含新增 25 种登录样本头序的实际发送。
- `./tests/scripts/check-refactor-line-gate.sh`、Go 格式化及 `git diff --check` 通过。

以上验证不包含真实上游登录、App 加密算法或 MFA 全流程联调。

推送前补充：完整 Go/Node 单测（65 个 Go 包成功、617 项 Node 测试）、WebUI 构建和
增量 Lint 通过。全仓格式门禁仍有 12 个既有 Go 文件未格式化，本次保持其原内容。
