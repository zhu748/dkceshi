# Android 2.7.1 请求对齐核对

核对对象：`com.deepseek.chat` 2.7.1，versionCode 287，minSdk 24，targetSdk 36。
APK SHA-256：`cd461f3d7c78f0c4ab6b8c4ff7270e16a766042170dfebead540b7f174948c5c`。
依据是该 APK 的 DEX 调用链和仓库已有脱敏 HAR 夹具；未进行真实账号登录、遥测注册或现场 TLS 抓包。

## 设备身份与 auto 模式

| 请求位置 | APK 取值 | 项目实现 |
| --- | --- | --- |
| 登录 JSON `device_id` | `Lsv3.a → Loy9.b → SmAntiFraud.getDeviceId()` | `device_id`；auto 模式缺少值时调用现有 SDK 生成器，写回 `device_id_type=auto` |
| `x-device-id` | `Lbb.j → Lov3.b`，安装标识 | 独立的 `installation_id` |
| `check_device.device_id` | `Lrv3.a → Lov3.c/b`，安装标识 | 与 `x-device-id` 相同 |
| `x-rangers-id` | 遥测 `getDid()`，优先 `bd_did`，其次 `device_id` | 只使用配置的 `rangers_id`，不再生成伪遥测数字 |
| `x-hif-dliq/leim` | 缓存的远端认证材料，响应带 `x-hif-ttl` | 仅转发配置的真实、未过期材料；没有本地伪造实现 |

`device_id` 的 SDK 生成路径保留。manual 模式使用既有 ID 或号池，auto 模式调用
`scripts/devid-generate.cjs`，依赖部署环境提供的 `data/devid` 扩展包。生成失败会返回错误，
不再用随机 44 字符字符串冒充 SDK 结果。Vercel 仍按已有规则使用 manual 模式。

`installation_id` 可直接配置，也可由本地设备档案生成：UTF-8 的 `android_id + "_" + device_brand`，
使用 `MD5("com.deepseek.chat")` 的 16 字节摘要作 AES key，AES-CBC / PKCS7 padding、零 IV，最后 Base64。
这是 APK 本地安装标识算法，不是设备认证。新档案保存 Android ID、品牌、机型和 API 等级；
只读部署使用按账号稳定派生的 Android ID。旧无类型的 44 字符 ID 保留为安装标识，登录时按所选模式获取 SDK ID。

网页 SDK 与 APK 原生 SDK 使用相同组织配置，并不证明二者的云端可信度或结果完全等价。
本改动没有替换既有数美生成器，也没有声称其已等价于原生 SDK。

### 扩展包兼容性验证

扩展包中的 `browser_env.js` 必须实际支持 Android 锚定和 `__siteOverride`，不能仅在
`profiles.json` 或注入变量中写入 Android。仓库内置生成器在运行 SDK、发送上报前检查实际可见的
UA、平台、Client Hints/mobile、高熵型号/版本、GPU、语言、时区，以及页面和上报站点；
不匹配时返回明确错误。`--selftest --anchor <设备配置.json>` 同样包括这些检查。
锚定文件读取或解析失败会报错，不会退回无锚定的桌面画像。

本次补充的 `devid (1).zip` 提供了缺失的 8 个运行文件。原包仅实现桌面环境，曾在
Android/站点检查中有 14 项失败。现在由 `scripts/devid-android-env.cjs` 按已知原文件
SHA-256 适配，在运行时加入 Android 平台、移动 Client Hints/高熵型号版本、触屏、
移动插件/MIME 模板、GPU 和 chat 站点覆盖；不再加载原 Windows 字体度量。
`scripts/devid-android-profile.cjs` 提供 19 个 vivo 公开硬件规格模板与其他型号的通用移动模板。
原始 SDK 的字节不变；这仍是 Web SDK 浏览器模拟，不等价于 APK 原生 SDK。

Android vivo X200 Pro 和 X100 Ultra 路径均已真实上报成功：HTTP 200、`code=1100`、
`detail.c=0`，返回 89 字符的带 `B` 前缀 ID，debug 本地解密检查未报告问题。
上报明文确认包含 Android Mobile UA 与移动 GPU。自检扩展至 88 项，覆盖运行时实际值，
仍不能证明 APK 登录接受该 ID；未验证账号登录、聊天或上传。
SDK 资源仍由部署方提供，未加入公开仓库；Go 编译本身不会打包 Node 或这些文件。
部署内置生成器时需同时保留 `scripts/devid-generate.cjs` 和同目录的
`devid-runtime-check.cjs`、`devid-android-env.cjs`、`devid-android-profile.cjs`。
安装、构建和精度限制见 [Android Web SDK 适配包](android-devid-package.md)。
显式配置的外部脚本或扩展包自带的 `generate.cjs` 优先于内置脚本，
需单独核对是否包含这些检查。

## 头部与时区

型号不再设置固定默认值；新账号自动分配后保存。vivo 已扩充为 20 种型号代码，
完整列表、API 预设及来源见 [Android 设备型号池](android-device-models.md)。

UA 使用 `DeepSeek/2.7.1 Android/{android_api_level}`；API level 是设备运行值，不能由 targetSdk 自动推定。
账号配置覆盖全局默认。`timezone` 可设置 IANA 名称（例如 `America/Los_Angeles`），
请求头偏移按当前时刻计算，SDK 锚定使用相同名称和分钟偏移，支持夏令时。
仅配置全局固定秒偏移时使用固定偏移时区，非整小时且无已知映射的偏移需要明确配置 `timezone`。

存档画像在同一设备的语言/时区变化后更新这些字段；机型或 Android 主版本变化时重建画像，
避免继续复用旧设备的 SDK 存档。账号 Android 机型、品牌应一起配置。

HIF 可通过账号配置导入 `hif_dliq` / `hif_leim` 及各自的 `hif_dliq_expires_at` /
`hif_leim_expires_at`（Unix 秒）。超过有效期不发送。当前没有自动获取 HIF 的网络流程；
配置材料必须来自实际设备认证，不能用随机值代替。客户端设置上报和遥测注册也未新增。

Go 默认有序 HTTP/2。默认模式发生网络错误时返回给业务重试，不再隐式改用标准库 TLS/头部；
显式 `DS2API_DEEPSEEK_USE_FRAMED_H2=0` 仍允许旧路径及其 fallback。
Vercel 流请求改用原生 HTTP/2，明确构造伪头与普通头，不经 fetch 注入 `accept-language: *` /
`sec-fetch-mode: cors`；支持 gzip SSE、取消和按 UTF-8 字节计算 Content-Length。
Node 仍使用运行时 TLS，与 Go 的 uTLS 预设均不代表实测 Android ClientHello。

## 文件上传

`Ljd2.z`、`Lwz1.j`、`Lli7.<init>` 和 Ktor 参数转义 `La85.a/b` 核实：

| 项目 | 行为 |
| --- | --- |
| 接口 | POST `/api/v0/file/upload_file` |
| PoW | 针对此上传路径获取，放在 `x-ds-pow-response` |
| 思考开关 | `x-thinking-enabled: 1/0`，沿用最终请求开关 |
| 模型 | `x-model-type: default`（现有 HAR 基准） |
| 文件大小 | `x-file-size` 是最终文件字节数，不含 multipart 包装 |
| Multipart | 单个 `file`，不发送 purpose/model_type 表单字段 |
| 分段 MIME | `application/octet-stream` |
| 参数 | 普通值 `name=file; filename=report.pdf`；特殊字符值加引号并转义 |
| Boundary | 新生成的 70 字符十六进制/连字符随机值，不重放捕获值 |
| 长度 | 文件流形态不发送 multipart 总 Content-Length、分段 Content-Length |
| 换行 | CRLF，末尾 closing boundary + CRLF |
| PDF/TXT/其他文档 | 保留原字节 |
| 异步结果 | 等待 fetch_files 成功才传给聊天，错误/取消终止等待 |
| 账号 | 上传、PoW、轮询、聊天引用保持同一账号身份和代理 |

## 图片上传

`Lf72.q` 读取 `photo_picker_compress_ratio`、`picture_compress_format`。
APK 的相册比例缺省是 NaN（不启用普通照片压缩）；相机压缩比例缺省为 0.8。
启用压缩时格式缺省为 WebP；API >= 30 使用 `WEBP_LOSSY`，并非 VP8L 无损 WebP。

项目相册上传使用以下配置，可在账号 Android 设备面板、配置文件或管理接口设置：

```json
{
  "device_id": "B...generated SDK result...",
  "device_id_type": "auto",
  "device_model": "",
  "device_brand": "",
  "android_api_level": "",
  "timezone": "Asia/Shanghai",
  "photo_compress_ratio": 0.8,
  "picture_compress_format": "webp"
}
```

- `photo_compress_ratio` 未设置或为 null：JPEG/PNG 保留原图，匹配 APK 相册缺省。
- 配置 0–1：取 `round(float32(ratio) * 100)` 为质量，编码有损 WebP 或 JPEG。
- 全局替代：`DS2API_DEEPSEEK_PHOTO_COMPRESS_RATIO` / `DS2API_DEEPSEEK_PICTURE_COMPRESS_FORMAT`；账号优先。
- 按 APK 的 `width * height * 4 > 30 MiB` 判定，采样倍率取 2 的幂。
- JPEG 应用 EXIF 方向；重编码后不保留原 EXIF，文件名为 `原名_随机数.webp/jpg`。
- 发送前重算后缀、MIME、字节数和 `x-file-size`；multipart 分段 MIME 仍为 octet-stream。
- `preserve_image=true` 优先于所有压缩配置。

这些规则用于 OpenAI、Responses、Claude、Gemini 等接口共用的原生上传逻辑，
文件引用和多账号绑定不变，原始 base64 不注入文本提示词。

仍有明确边界：HEIF/BMP 等格式未增加解码器；动画 GIF/APNG/WebP 保留原字节；
超过 2000 万像素保留原格式以限制 Go 全图解码内存。当前解码器先完整解码再缩放，
不同于 Android 原生采样解码；采样滤镜和编码器版本也不同，因此不保证同图字节一致。
编码器质量 0 保留作默认值，显式比例 0 使用最低可用质量 1，避免错误使用质量 75。
未自动拉取 App 远程压缩设置，需根据实际设置提供对应配置；不能仅由一份 APK 推定当前远程值。

## 验证与剩余工作

回归覆盖：AES 已知向量、auto 生成/复用/持久化、登录与设备检查分离、token 轮换、
画像时区一致性、有损 WebP、JPEG 格式、EXIF、默认原图、采样阈值、multipart 原始字节、
转换后的文件大小、HTTP/2 实际头部顺序、gzip SSE 和取消。

仍需真实设备网络样本核对：TLS ClientHello、HTTP/2 SETTINGS/窗口/HPACK、连接复用策略、
远程 client/settings、HIF 自动获取、遥测注册、HEIF/动画转换及 Android 编码器结果。
现有 HAR 只验证 HTTP 层样本，不能替代这些证据。
