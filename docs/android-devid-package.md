# Android Web SDK 适配包

本包使用用户提供的数美 **Web SDK**，将浏览器模拟层改为 Android 手机环境。
`fp-1.min.js` 与原包字节相同，没有实现 APK 的原生 `SmAntiFraud` SDK，也不提供设备认证。
成功签发 device_id 仅验证数美上报链路；App 登录、聊天和上传需要用账号另行验证。

## 安装与使用

需要 Node.js 22+。将压缩包解压到 dkceshi 项目根目录，保留目录结构：

- `scripts/`：生成器及三个配套模块，需一起部署。
- `data/devid/`：8 个 SDK 运行文件及校验清单。
- `examples/devid-android-anchor.json`：独立运行示例，不是默认机型设置。

项目 auto 模式会根据账号型号、Android 主版本、语言及时区创建锚定配置并调用生成器。
本包不包含账号、密码、token、已生成的 ID 或设备存档。Go 编译不会打包 Node 和这些文件，
运行时需要同时提供；当前仓库的 Docker 镜像还需另安装 Node、复制 scripts 和 data/devid。
如果配置了 `DS2API_DEVID_SCRIPT` 或另有 `data/devid/generate.cjs`，它们会覆盖仓库内置生成器，
需将覆盖项指向本包的 `scripts/devid-generate.cjs`。

在项目根目录执行本地自检（不联网）：

```sh
node scripts/devid-generate.cjs --selftest --anchor examples/devid-android-anchor.json
```

生成并持久化设备档案（会向数美上报，不需要 DeepSeek 账号密码）：

```sh
node scripts/devid-generate.cjs --anchor examples/devid-android-anchor.json --profile data/devid/profiles/test-device.json --timeout 35
```

stdout 只有一行结果 JSON，成功时包含 `device_id`、`smid_v2`、`environment=android-web-sdk`。
同一 `--profile` 复用设备种子和 smid；`--reset` 会重建。没有锚定文件时从 19 个 vivo 模板中
随机选择并保存，不设置固定 V2507A。账号已有型号继续优先使用，不会被独立示例覆盖。
`android_version` 是 Android 版本（如 15），不是 API level（如 35）。
重复上报可能得到不同的 `device_id`；项目在成功生成后保存并复用该 ID，不会在每次请求时重新生成。
上述适配和校验针对默认 VM 路径；旧 `--browser` / `--via-chromium` 实验通道未进行同等 Android 验证。

## 适配范围

- 移动 UA 使用 Chrome 的缩减形式 `Android 10; K`；实际型号和 Android 主版本放在高熵
  Client Hints 中，`platform=Android`、`mobile=true`、架构 `arm/64` 保持一致。
- 页面、Origin、Referer 使用 `chat.deepseek.com`；上报 UA、平台和 mobile 头来自相同档案。
- 触屏、coarse pointer、无 hover、空 PDF 插件/MIME 列表作为当前模拟模板。
- 不加载 Windows 字体度量或 Direct3D GPU；使用 Android 字体名称、移动 GPU 和屏幕模板。
- 语言、Accept-Language、时区同步更新；旧版没有适配版本的设备存档重建，避免沿用桌面 smid。
- 原始旧包在加载时按已知 SHA-256 适配；预先适配过的包不会重复修改。未知版本不盲目套补丁。
- 运行时检查 SDK 实际可见的值，失败时不会提交云端上报。

## 数据精度

19 个 vivo 型号代码的物理分辨率和 GPU 芯片名来自 vivo 官方参数页：X90/X100/X200 系列及
S19/S20 系列。来源链接和映射保存在 `scripts/devid-android-profile.cjs`。
这是公开规格模板；DPR、CSS 屏幕尺寸、完整 WebGL 驱动字符串、字体宽度、Canvas、存储配额、
电池和网络参数仍是模拟值，尚未用这些机型的真实 Android Chrome 抓包校准。

池外型号及尚无硬件表的型号（包括 V2507A、Pixel/Samsung）保留型号和 Android 版本，
使用明确标记的 `generic-mobile-template`，日志会提示，结果 `hardware_approximate=true`。
独立锚定 JSON 可用 `screen: [CSS宽,CSS高,DPR]`、`gpu: {vendor,renderer}` 覆盖；
也可以在扩展包 `profiles.json.modelScreens`、`gpus.json.modelGpus` 中补充真机参数。

参考：

- [Chrome 移动模拟与 Client Hints](https://developer.chrome.com/docs/chromedriver/mobile-emulation)
- [vivo X200 Pro 官方参数](https://www.vivo.com.cn/vivo/param/x200pro)
- [vivo X100 Ultra 官方参数](https://www.vivo.com.cn/vivo/param/x100ultra)

## 从原包重新构建

在包含 SDK 8 个原始文件的仓库根目录执行（需要 Python 3 和 Node.js）：

```sh
python3 scripts/build-android-devid.py --output /path/to/devid-android-websdk.zip
```

构建采用文件白名单，排除账号、anchors、profiles、生成结果和其他运行数据。
`data/devid/ANDROID-PACKAGE.json` 记录原资产与包内文件的 SHA-256，便于核对 SDK 未被替换。
