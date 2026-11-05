# 代理 IP：协议、智能识别与批量导入

「代理 IP」支持 HTTP、HTTPS、SOCKS4、SOCKS4A、SOCKS5、SOCKS5H、TURN、TURNS 和 SSTP。
账号绑定后，登录、聊天/流式输出、文件上传、TTS WebSocket 和设备上报沿用该代理出口。
代理配置无效、转换器缺失或代理连接失败时返回错误，不自动改走直连或环境代理。
未绑定代理的账号继续遵循原来的环境代理和显式直连设置。

## 添加与识别

先从「代理类型」下拉框选择协议，再粘贴节点。输入带前缀时，前缀决定协议；
没有前缀的记录使用面板当前选择。批量内容可以混合不同协议。
**端口不会改变协议**：选择 TURN 后粘贴 `relay.example:5349` 仍为 TURN，TLS 请明确选择 TURNS。

支持这些格式（示例协议在面板选择）：

```text
host.example:1080:user:password
user:password:host.example:1080
user:password@host.example:1080
host.example:1080@user:password
host.example|1080|user|password
host.example 1080 user password
host.example:1080
socks5h://user:password@host.example:1080
sstp://dXNlcjpwYXNzd29yZA==@vpn.example:443
turn:relay.example:3478?transport=tcp&username=user&password=password
turns://user:password@[2001:db8::1]:5349#备用
```

单条输入自动填入表单，可以继续补齐字段后保存；仅有用户名和密码不能确定节点地址。
SSTP 纯地址标记为「待补齐凭据」，可以先填入主机和端口，保存前必须补齐用户名和密码。
不支持的参数、非法编码等错误不会自动丢弃后填入表单；需要修改粘贴内容或清空输入后才能保存。
识别后手动补齐的内容不会因协议自动切换或界面语言变化被重复识别覆盖。
多条输入显示逐行预览，点击「导入有效节点」保存可导入项。错误行留在预览中，
重复节点自动跳过。每批最多 200 条、256 KiB。

URL / 冒号格式的凭据保留字符（`@ : # ? & %` 等）应进行 URL 编码。
`|` 四字段格式保留凭据中的内部及首尾空格；空格分隔格式仅适用于不含空格的凭据。
这两种格式不进行 URL 解码；JSON 字段和手动填写也无需 URL 编码。
IPv6 地址带端口时使用方括号。无法确定顺序的四段格式优先按「主机:端口:用户:密码」解释，
有歧义时请使用 `协议://用户:密码@主机:端口`。
用户名、密码首尾空格保留；控制字符被拒绝。

连接配置完全相同的节点视为重复，名称不同不会重复添加。
同一地址使用不同凭据或连接参数时可以分别保存。批量导入不覆盖已有节点及账号绑定。
编辑时密码留空保留原密码；勾选「清空已保存的密码」才能清空。SSTP 不能清空必需凭据。

### 节点 JSON 和 ICE JSON

支持单个对象、对象/链接混合数组，以及每行一个节点对象：

```json
{"type":"sstp","server":"vpn.example","server_port":443,
 "username":"domain\\user","password":"password","name":"备用 VPN",
 "options":{"sni":"cert.example","mtu":1280}}
```

`scheme/host/port` 分别可替代 `type/server/server_port`，同一字段不能重复使用两种名称。
高级参数也可通过顶层 `tls_server_name`、`ca_file`、`insecure`、`realm`、`mtu` 传入。
重复 JSON 字段、重复高级参数或未知节点字段被拒绝。高级文本参数必须为字符串，开关与 MTU 也分别校验类型。

```json
{"iceServers":[
 {"urls":"stun:discovery.example"},
 {"urls":["turn:relay.example:3478?transport=tcp","turns:relay.example:5349"],
  "username":"user","credential":"password"}
]}
```

STUN 被跳过，不能作为 TCP 代理。ICE 凭据按原文读取，仅支持 password 凭据，不支持 OAuth。
ICE 数组中的单条错误不阻止其他有效节点导入。
单行 ICE JSON 可以与普通链接混合逐行导入；空节点数组会明确提示没有节点。
字段必须为有效 UTF-8；格式不正确的百分号编码或无效 UTF-8 凭据会被拒绝。

## 协议与高级设置

| 协议 | 默认端口 | 认证 / DNS / 要求 |
|---|---:|---|
| HTTP / HTTPS | 80 / 443 | HTTP CONNECT；可选 Basic 认证；HTTPS 表示先以 TLS 连接代理 |
| SOCKS4 / SOCKS4A | 1080 | 仅用户 ID，无密码认证；4 本机解析 IPv4，4A 把域名交给代理；不支持 IPv6 目标 |
| SOCKS5 / SOCKS5H | 1080 | 可选用户名密码；5 本机解析，5H 远端解析 |
| TURN / TURNS | 3478 / 5349 | RFC 6062 TCP allocation；标准认证或匿名；TURNS 使用 TLS |
| SSTP | 443 | MS-CHAPv2；IPv4 IPCP；用户态 TCP/IP，无需 TUN/PPP 或 NET_ADMIN |

TURN 的 `transport=tcp` 只说明客户端到服务器使用 TCP；服务器还必须支持 RFC 6062 TCP 中继。
仅支持 UDP allocation 的 WebRTC TURN 服务器不能代理 HTTPS 请求。
SSTP 不支持 PAP-only、EAP、SSO/MFA 或 IPv6CP。
TURN/SSTP 目标 DNS 在转换器本机执行，不提供隧道内远端 DNS。
服务器出口策略仍可能限制 DeepSeek 等目标；临时 TURN 凭据过期后需要自行更新。

| 设置 | 适用协议 | 默认 |
|---|---|---|
| SNI / `sni` | HTTPS、TURNS、SSTP | 代理主机名 |
| CA 文件 / `ca` | HTTPS、TURNS、SSTP | 系统信任；路径必须在服务器或容器内可读 |
| 跳过证书校验 / `insecure` | HTTPS、TURNS、SSTP | false，需要手动开启 |
| `realm` | TURN、TURNS | 使用服务器返回值 |
| `mtu` | SSTP | 1400；接受 576–9000，内部链路上限 1400 |
| `transport`（链接参数） | TURN、TURNS | tcp，拒绝 udp |

SSTP 自定义 CA 当前需要 Linux。证书文件不随配置导出自动复制，Docker 使用者须自行挂载。

## 部署

Docker 的源码构建、预构建发布镜像和带 devid 的镜像均携带 `ds2api-proxy-bridge`。
发布压缩包把转换器和主程序放在同一目录。普通 Go 主模块保持原有版本，
独立转换器使用 Go 1.27.3；从源码构建：

```bash
mkdir -p bin
go -C proxybridge build -trimpath -o ../bin/ds2api-proxy-bridge .
go build -o bin/ds2api ./cmd/ds2api
```

Windows 二进制文件名增加 `.exe`。查找顺序：`DS2API_PROXY_BRIDGE_BIN`、主程序同目录、
当前目录的 `bin/`、PATH。Vercel 等限制子进程或长连接的平台不能保证 TURN/SSTP 运行。
保存和解析配置不访问远端，也不要求转换器已安装。

同一连接配置复用转换进程；启动超时或取消时清理子进程；空闲 30 分钟回收，
正在使用的连接不会被空闲回收；进程退出后下次使用会重新启动。最多保留 128 个转换实例。
同一配置启动失败后退避 3 秒，避免并发请求反复启动进程；修改凭据或连接参数后立即重新尝试。
启动失败的实例立即释放缓存名额；停止服务会取消尚未完成的启动。
主服务通过生命周期管道通知转换器退出，父进程异常退出时管道关闭，转换器也会停止。
本地仅监听 `127.0.0.1`，凭据通过私密临时文件传入，启动结束后删除；
不会把密码放入命令行或把子进程原始输出回传给面板。
启动失败通过受控错误码显示证书、CA 文件、SSTP 认证或握手等原因，其他原始诊断仍不对外返回。

转换器改编自 zhu748/vertex2openai 的 `proxybridge`（d58bfb3），包含其 MIT 许可和依赖 NOTICE。
本地回归包含实际 TURN/TURNS 服务器数据往返、匿名分配、TLS 校验、并发及取消。
完整 SSTP 联调需要私密服务器配置，见 `proxybridge/sstp_integration_test.go`。


## 连接与请求边界

HTTP/HTTPS、SOCKS4/4A、SOCKS5/5H 的握手有独立的 15 秒预算，包含本机 DNS（需要时）、
连接与认证；建立后的长连接不受该握手预算限制。取消与超时保留对应上下文错误，避免被当成普通代理故障重试。
HTTP CONNECT 握手响应头总量最多 64 KiB，支持临时响应；限制只作用于响应头，不截断隧道数据。
有序 HTTP/2 和 TTS WebSocket 的环境 HTTP/HTTPS 代理共用相同的 CONNECT 超时及响应头限制。
隧道建立后，连接目标的 TLS 握手另有 15 秒预算；握手完成后的长流式响应不受此预算限制。

代理接口要求 JSON 对象，拒绝 `null`、重复字段、未知字段和不匹配的字段类型。
解析输入文本仍限制为 256 KiB；外层 JSON 请求最多 2 MiB，为转义后的文本预留空间。
批量保存先验证每个提交项，再做去重，不会把非法参数当成已有节点而跳过验证。
等价 IPv6 写法按同一连接识别，既有代理 ID 不受影响。


## 账号绑定、代理桥与配置刷新

账号代理接口必须明确提供 `proxy_id`，空字符串才表示直连；使用 `mode=auto` 时不得同时指定非空代理 ID。
空对象、无效 mode 或无关字段不会修改账号绑定。测试已保存代理时只传 `proxy_id`，临时测试则填写完整代理字段。

`mihomo-` 是代理桥保留的 ID 前缀。托管节点可在代理 IP 页面测试，修改或删除请使用代理桥页面。
导入备份恢复托管节点时，仅重建当前系统的本地 SOCKS5 端点，不沿用备份中的外部代理凭据或 TLS 参数。
代理或完整配置改变时只清理连接配置已变化、已删除节点的连接池，保留其他节点的连接复用。
刷新与慢初始化并发时，不会把旧配置重新插回缓存。
客户端关闭后不会再创建新的代理连接池。调用方取消请求不计入节点故障；真实连接失败仍参与健康统计。

SOCKS4/5 本机 DNS 会尝试返回的其他可用地址，并在各候选间分配剩余握手预算；SOCKS4 仅尝试 IPv4。
SOCKS5H 继续使用远端 DNS。匿名 TURN 的握手支持提前取消，不必等到 socket deadline。
转换实例达到 128 个时优先回收最久未用的空闲实例，全部在用时才返回上限错误。

保存、删除、测试与导入会合并重复操作；关闭页面或切换登录来源后，不采用旧响应。
旧保存或导入的完成回调不会关闭新打开的弹窗。修改已提交但列表刷新失败时，明确提示「已保存，刷新失败」，避免误以为保存失败而重复提交。

## 连通性测试与自动故障转移

添加或编辑时可点击「测试当前配置」，无需先保存；编辑时密码留空使用已保存密码，勾选清空则测试空密码。
测试不会修改节点或账号绑定。「测试全部」最多并发测试 4 个节点，并汇总成功数；结果显示最近测试时间。

真实请求的健康反馈覆盖响应体读取：收到响应头后继续观察流式网络错误，用户主动取消不计失败。
手动配置的节点仍由用户管理，自动迁移只适用于代理桥自动绑定的账号。

自动调度需要变更监听时，暂缓新的账号请求与后台保活，等待当前在途请求结束后再应用配置；
排队请求继续遵守原有排队上限和取消规则。更新完成或服务停止时恢复调度，避免持续并发阻止故障转移。
写回绑定前再次核对账号当前选择，用户并发设置的手动代理、直连及新绑定不会被旧调度结果覆盖。
